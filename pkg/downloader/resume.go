package downloader

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/tg"
)

const downloadPartSize int64 = 512 * 1024

// attemptsPerBlock es cuántas veces se reintenta un bloque antes de dar la
// descarga entera por perdida.
const attemptsPerBlock = 5

// maxFloodWait es el tope de lo que se espera cuando Telegram pide una
// pausa. Más allá de eso se prefiere fallar y que el usuario reintente, en
// lugar de dejar una tarea colgada media hora.
const maxFloodWait = 5 * time.Minute

// floodWaitDuration lee cuánto pide esperar Telegram cuando responde
// FLOOD_WAIT_x.
//
// Se saca del texto del error a propósito: así no hace falta importar nada
// nuevo, y funciona igual cuando el error viene envuelto por capas de arriba.
// El código de Telegram es estable y no va a cambiar de nombre.
func floodWaitDuration(err error) (time.Duration, bool) {
	if err == nil {
		return 0, false
	}
	const prefix = "FLOOD_WAIT_"
	text := err.Error()
	pos := strings.Index(text, prefix)
	if pos < 0 {
		return 0, false
	}
	rest := text[pos+len(prefix):]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	seconds, convErr := strconv.Atoi(rest[:end])
	if convErr != nil || seconds <= 0 {
		return 0, false
	}
	wait := time.Duration(seconds) * time.Second
	if wait > maxFloodWait {
		return 0, false
	}
	return wait, true
}

// downloadMissingParts descarga únicamente las partes que no aparecen en
// completed. UploadGetFile acepta offsets arbitrarios, por lo que no vuelve a
// transferir las partes que ya están en el temporal.
func downloadMissingParts(
	ctx context.Context,
	client interface {
		UploadGetFile(context.Context, *tg.UploadGetFileRequest) (tg.UploadFileClass, error)
	},
	location tg.InputFileLocationClass,
	output *progressWriterAt,
	totalBytes int64,
	workers int,
	completed map[int64]struct{},
) error {
	if totalBytes <= 0 {
		return fmt.Errorf("invalid file size for resume")
	}
	if workers < 1 {
		workers = 1
	}

	jobs := make(chan int64)
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex
	setErr := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		if firstErr == nil {
			firstErr = err
			cancel()
		}
		errMu.Unlock()
	}

	worker := func() {
		defer wg.Done()
		for offset := range jobs {
			// Telegram requiere que el límite sea múltiplo de 4096 (4KB).
			// Para el último fragmento, pedimos el tamaño de parte completo;
			// Telegram simplemente devolverá los bytes restantes del archivo.
			limit := int(downloadPartSize)

			var data []byte
			var err error
			for attempt := 0; attempt < attemptsPerBlock; attempt++ {
				if workCtx.Err() != nil {
					return
				}
				response, requestErr := client.UploadGetFile(workCtx, &tg.UploadGetFileRequest{
					Location: location,
					Offset:   offset,
					Limit:    limit,
				})
				if requestErr == nil {
					file, ok := response.(*tg.UploadFile)
					if !ok {
						requestErr = fmt.Errorf("unexpected Telegram response: %T", response)
					} else {
						data = file.Bytes
						if len(data) == 0 || len(data) > limit {
							requestErr = fmt.Errorf("Telegram returned %d bytes for a block of %d", len(data), limit)
						}
					}
				}
				if requestErr == nil {
					// Limpiar el error del intento anterior es imprescindible:
					// sin esto, un fallo pasajero seguido de un reintento
					// correcto salía del bucle with err distinto de nil y la
					// comprobación de abajo mataba la descarga entera aunque el
					// bloque se hubiera traído bien.
					err = nil
					break
				}
				err = requestErr
				if attempt == attemptsPerBlock-1 {
					break
				}

				// El FLOOD_WAIT ya no se trata aquí: lo absorbe el cliente
				// envoltorio de blockclient.go, que además para a todos los
				// workers a la vez. Lo que queda es el reintento corto de los
				// errores normales de red.
				wait := time.Duration(attempt+1) * 300 * time.Millisecond
				select {
				case <-workCtx.Done():
					return
				case <-time.After(wait):
				}
			}
			if err != nil {
				setErr(fmt.Errorf("error downloading offset %d: %w", offset, err))
				return
			}
			if _, err = output.WriteAt(data, offset); err != nil {
				setErr(fmt.Errorf("error writing offset %d: %w", offset, err))
				return
			}
		}
	}

	if workers > 32 {
		workers = 32
	}
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go worker()
	}

	sentJobs := true
	for offset := int64(0); offset < totalBytes; offset += downloadPartSize {
		if _, alreadyDone := completed[offset/downloadPartSize]; alreadyDone {
			continue
		}
		select {
		case jobs <- offset:
		case <-workCtx.Done():
			sentJobs = false
		}
		if !sentJobs {
			break
		}
	}
	close(jobs)
	wg.Wait()

	errMu.Lock()
	defer errMu.Unlock()
	if firstErr != nil {
		return firstErr
	}
	return workCtx.Err()
}
