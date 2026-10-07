package downloader

// Pruebas del ciclo de vida de Shutdown (SEC-36): los canales de persistencia
// deben cerrarse, sus dos bucles deben terminar antes de devolver y ningún
// envío concurrente con ese cierre puede producir un "send on closed channel".

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tgdown/pkg/config"
	"tgdown/pkg/storage"
)

// newShutdownEngine monta un motor con SQLite real: los bucles de persistencia
// solo arrancan cuando hay storage, que es justo lo que estos tests miden. Sin
// cliente de Telegram porque aquí no se descarga nada.
func newShutdownEngine(t *testing.T) (*Engine, *storage.Storage) {
	t.Helper()
	st, err := storage.NewStorage(filepath.Join(t.TempDir(), "shutdown.db"))
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := st.Close(); closeErr != nil {
			t.Errorf("storage Close: %v", closeErr)
		}
	})
	cfg := config.DefaultConfig()
	cfg.DownloadFolder = t.TempDir()
	return NewEngine(nil, st, cfg), st
}

// SEC-36: tras un Shutdown con contexto amplio, los bucles de persistencia
// deben terminar (canales cerrados y drenados) y un envío posterior a
// enqueuePersist/requestChunkFlush no debe reventar.
func TestShutdownClosesAndDrainsPersistenceLoops(t *testing.T) {
	eng, st := newShutdownEngine(t)

	// Estado encolado antes de apagar: si el drenaje fallara, no llegaría
	// nunca a SQLite y la comprobación final lo delataría.
	eng.enqueuePersist(storage.DownloadItem{
		ID:        "drained",
		ChatID:    1,
		MessageID: 1,
		FileName:  "drained.bin",
		Status:    "queued",
	})

	if err := eng.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	// Los dos bucles deben haber salido de su `range` ANTES de que Shutdown
	// devolviera: si siguieran vivos, este Wait no terminaría y el tiempo
	// daría el fallo.
	loopsDone := make(chan struct{})
	go func() {
		eng.loopsWG.Wait()
		close(loopsDone)
	}()
	select {
	case <-loopsDone:
	case <-time.After(3 * time.Second):
		t.Fatal("los bucles de persistencia no terminaron tras Shutdown")
	}

	eng.mu.RLock()
	closedFlag := eng.persistClosed
	eng.mu.RUnlock()
	if !closedFlag {
		t.Fatal("Shutdown no marcó persistClosed")
	}

	// Lo que estaba encolado se drenó a SQLite antes de cerrar el canal.
	saved, err := st.LoadDownloads("")
	if err != nil {
		t.Fatalf("LoadDownloads: %v", err)
	}
	if _, ok := saved["drained"]; !ok {
		t.Fatal("el estado encolado no se drenó a SQLite antes de cerrar")
	}

	// Un envío con los canales ya cerrados no debe hacer panic: se descarta.
	eng.enqueuePersist(storage.DownloadItem{ID: "after-shutdown", Status: "queued"})
	eng.requestChunkFlush("after-shutdown")
}

// SEC-36: envíos concurrentes con el cierre de Shutdown no deben producir
// "send on closed channel". Los workers encolan ANTES, DURANTE y DESPUÉS del
// Shutdown (siguen hasta que se les da la orden de parar, ya con los canales
// cerrados), que es exactamente el cruce peligroso.
func TestConcurrentEnqueueDuringShutdownDoesNotPanic(t *testing.T) {
	eng, _ := newShutdownEngine(t)

	const workers = 4
	var started atomic.Int32
	var panics atomic.Int32
	stop := make(chan struct{})
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; ; i++ {
				func() {
					// Un panic aquí tumbaría el proceso entero en la
					// aplicación real: se recoge y se cuenta para fallar el
					// test de forma explícita.
					defer func() {
						if r := recover(); r != nil {
							panics.Add(1)
							t.Logf("panic en worker %d: %v", worker, r)
						}
					}()
					id := fmt.Sprintf("race-%d-%d", worker, i)
					eng.enqueuePersist(storage.DownloadItem{ID: id, Status: "queued"})
					eng.requestChunkFlush(id)
				}()
				if i == 0 {
					started.Add(1)
				}
				select {
				case <-stop:
					return
				default:
				}
			}
		}(w)
	}

	// Garantiza que hay envíos ANTES de que Shutdown empiece a cerrar.
	waitForCondition(t, "los workers han arrancado", func() bool {
		return started.Load() == workers
	})

	if err := eng.Shutdown(context.Background()); err != nil {
		// No es Fatal: los workers siguen girando y hay que pararlos abajo.
		t.Errorf("Shutdown: %v", err)
	}

	// Los workers siguen encolando con los canales ya cerrados: ese es el
	// envío que antes habría reventado el proceso.
	close(stop)
	wg.Wait()

	if n := panics.Load(); n > 0 {
		t.Fatalf("%d envíos hicieron panic con los canales cerrados", n)
	}

	// Caso más directo, ya en caliente y en la goroutine del test.
	eng.enqueuePersist(storage.DownloadItem{ID: "post-shutdown", Status: "queued"})
	eng.requestChunkFlush("post-shutdown")
}
