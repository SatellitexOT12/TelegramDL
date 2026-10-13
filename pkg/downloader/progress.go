package downloader

// Calculo de progreso y limitacion de velocidad (throttle) de las
// descargas en curso.

import (
	"fmt"
	"io"
	"math"
	"os"
	"time"

	"tgdown/pkg/config"
)

// formatETA pasa los segundos que faltan a algo que se lee de un vistazo.
// El tope de un día es a propósito: con la descarga casi parada la cuenta se
// dispara a cifras que no dicen nada, y es más honesto decir que no se sabe.
func formatETA(segundos float64) string {
	if math.IsNaN(segundos) || math.IsInf(segundos, 0) || segundos < 0 {
		return ""
	}
	if segundos < 1 {
		return "menos de 1 s"
	}
	if segundos >= 24*3600 {
		return "más de 1 día"
	}

	total := int(segundos + 0.5)
	horas := total / 3600
	minutos := (total % 3600) / 60
	segs := total % 60

	switch {
	case horas > 0:
		return fmt.Sprintf("%d h %d min", horas, minutos)
	case minutos > 0:
		return fmt.Sprintf("%d min %d s", minutos, segs)
	default:
		return fmt.Sprintf("%d s", segs)
	}
}

// maxCreditSec limita cuánto crédito puede acumular el limitador de
// velocidad, medido en segundos al límite configurado.
//
// El contador (throttleTime/bytesSince) es de toda la vida del motor y no se
// reinicia nunca, a propósito: así el ritmo medio a largo plazo sale exacto.
// El problema era que tampoco tenía tope. Cada rato sin descargar sumaba
// crédito, porque el reloj seguía corriendo y los bytes no: tras diez minutos
// parado con un límite de 5 MB/s había 3 GB de crédito, y la siguiente descarga
// corría sin ningún freno hasta gastarlos. Eso es lo que se ve como "empieza
// rapidísimo y luego se desploma".
//
// Con el tope, como mucho se permite la ráfaga de un segundo al límite. La
// deuda, en cambio, sigue sin tope: lo que se pasa de rosca se paga entero.
const maxCreditSec = 1.0

func (e *Engine) throttle(bytesCount int64) {
	e.mu.RLock()
	sp := e.config.SpeedLimit
	e.mu.RUnlock()

	if sp.Value <= 0 {
		return
	}

	mult := config.SpeedMultipliers[sp.Unit]
	limitBps := sp.Value * mult
	if limitBps <= 0 {
		return
	}

	e.throttleMu.Lock()
	ahora := time.Now()
	if e.throttleTime.IsZero() {
		e.throttleTime = ahora
		e.bytesSince = 0
	}
	e.bytesSince += bytesCount
	elapsed := ahora.Sub(e.throttleTime).Seconds()
	expectedTime := float64(e.bytesSince) / limitBps

	if elapsed-expectedTime > maxCreditSec {
		// Vamos muy por delante del reloj: se adelanta la marca de tiempo para
		// dejar el crédito justo en el tope, en lugar de reiniciar el contador,
		// que perdonaría también la deuda pendiente.
		elapsed = expectedTime + maxCreditSec
		e.throttleTime = ahora.Add(-time.Duration(elapsed * float64(time.Second)))
	}

	sleepSec := expectedTime - elapsed
	e.throttleMu.Unlock()

	if sleepSec > 0.005 {
		time.Sleep(time.Duration(sleepSec * float64(time.Second)))
	}
}

type progressWriterAt struct {
	file   *os.File
	itemID string
	engine *Engine
	total  int64
}

func (pw *progressWriterAt) WriteAt(p []byte, off int64) (int, error) {
	n, err := pw.file.WriteAt(p, off)
	if n == len(p) && err == nil {
		pw.engine.onProgress(pw.itemID, int64(n), pw.total, off)
		pw.engine.throttle(int64(n))
	} else if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	return n, err
}

func (e *Engine) onProgress(itemID string, bytesWritten int64, totalBytes int64, offset int64) {
	e.mu.Lock()
	item, ok := e.downloads[itemID]
	if !ok || item.Status != "downloading" {
		e.mu.Unlock()
		return
	}

	now := time.Now()
	// Sumar siempre los bytes escritos al progreso actual para evitar que se detenga
	item.CurrentBytes += bytesWritten

	if totalBytes > 0 {
		item.TotalBytes = totalBytes
		progressVal := (float64(item.CurrentBytes) / float64(totalBytes)) * 100.0
		if progressVal >= 100.0 {
			item.Progress = 100.0
		} else {
			item.Progress = math.Min(99.9, progressVal)
		}
		item.TotalStr = config.FormatBytes(float64(totalBytes))
	}
	item.CurrentStr = config.FormatBytes(float64(item.CurrentBytes))
	item.UpdatedAt = float64(now.Unix())

	// Deduplicación solo para la base de datos (para no saturar con miles de inserts)
	chunkIndex := offset / downloadPartSize
	chunks := e.seenChunks[itemID]
	if chunks == nil {
		chunks = make(map[int64]struct{})
		e.seenChunks[itemID] = chunks
	}
	if _, seen := chunks[chunkIndex]; !seen {
		chunks[chunkIndex] = struct{}{}
		e.pendingChunks[itemID] = append(e.pendingChunks[itemID], chunkIndex)
	}

	lastTime := e.lastProgressTimes[itemID]
	lastBytes := e.lastProgressBytes[itemID]
	if !lastTime.IsZero() && bytesWritten > 0 {
		elapsed := now.Sub(lastTime).Seconds()
		bytesDelta := item.CurrentBytes - lastBytes
		if elapsed > 0 && bytesDelta >= 0 {
			instantSpeed := float64(bytesDelta) / elapsed
			// Suavizado exponencial basado en tiempo: las ráfagas de varios
			// workers no generan picos artificiales de velocidad.
			const smoothingWindow = 2.0
			alpha := 1 - math.Exp(-elapsed/smoothingWindow)
			speedVal := e.itemSpeeds[itemID] + alpha*(instantSpeed-e.itemSpeeds[itemID])
			item.Speed = config.FormatBytes(speedVal) + "/s"
			e.itemSpeeds[itemID] = speedVal
		}
	}

	// Tiempo restante, calculado con el mismo ritmo suavizado que la velocidad
	// que se muestra al lado: así las dos cifras cuentan lo mismo y no se
	// contradicen. Aquí el estado siempre es "downloading" (arriba hay una
	// salida temprana para el resto).
	item.ETA = ""
	if ritmo := e.itemSpeeds[itemID]; ritmo > 0 && totalBytes > 0 {
		if restante := totalBytes - item.CurrentBytes; restante > 0 {
			item.ETA = formatETA(float64(restante) / ritmo)
		}
	}
	if bytesWritten > 0 {
		e.lastProgressTimes[itemID] = now
		e.lastProgressBytes[itemID] = item.CurrentBytes
	}

	lastBroadcast, hasBroadcast := e.lastBroadcastTimes[itemID]
	shouldBroadcast := !hasBroadcast || now.Sub(lastBroadcast) >= 200*time.Millisecond
	if shouldBroadcast {
		e.lastBroadcastTimes[itemID] = now
	}

	lastSave, hasSave := e.lastSaveTimes[itemID]
	shouldSave := !hasSave || now.Sub(lastSave) >= 1*time.Second
	if shouldSave {
		e.lastSaveTimes[itemID] = now
	}

	cp := *item
	e.mu.Unlock()

	if shouldSave && e.storage != nil {
		e.enqueuePersist(cp)
		// Aviso en vez de llamada directa: persistSeenChunks abre una
		// transacción en SQLite y toma el candado global de Storage, y aquí
		// estamos en la goroutine del worker, la que debería estar pidiendo el
		// siguiente bloque a Telegram. Con la base ocupada (otro guardado, un
		// VACUUM al limpiar historial) el worker se quedaba esperando.
		e.requestChunkFlush(itemID)
	}

	if shouldBroadcast {
		e.notifyState(cp)
	}
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
