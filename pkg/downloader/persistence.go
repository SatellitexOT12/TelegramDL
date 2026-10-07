package downloader

// Persistencia en segundo plano del estado de las descargas: aqui vive el
// bucle que escribe en SQLite sin bloquear el hilo que actualiza el progreso.

import (
	"log"

	"tgdown/pkg/i18n"
	"tgdown/pkg/storage"
)

func (e *Engine) persistenceLoop() {
	for item := range e.persistCh {
		if e.storage != nil {
			if err := e.storage.SaveDownload(item); err != nil {
				log.Printf("[DOWNLOADER] %s", i18n.T("downloader.saveStateError", err))
			}
		}
		e.persistWG.Done()
	}
}

func (e *Engine) enqueuePersist(item storage.DownloadItem) {
	if e.storage == nil {
		return
	}
	// El candado cubre a la vez el flag, el Add y el envío: Shutdown marca
	// persistClosed y cierra persistCh con ese mismo candado tomado, así que o
	// este envío entra antes (y el ítem se drena al cerrar) o se encuentra el
	// flag y se desecha. Un Add hecho bajo candado también garantiza que no
	// haya Add concurrente con el Wait que hace Shutdown. El envío es no
	// bloqueante, así que el candado no se retiene esperando sitio.
	e.mu.Lock()
	if e.persistClosed {
		e.mu.Unlock()
		// Motor apagando: el estado sigue en memoria y ya no interesa
		// persistirlo. Nunca se envía a un canal cerrado.
		return
	}
	e.persistWG.Add(1)
	select {
	case e.persistCh <- item:
		e.mu.Unlock()
	default:
		e.persistWG.Done()
		e.mu.Unlock()
		// El estado actual permanece en memoria y el siguiente tick lo
		// volverá a persistir; nunca frenamos una escritura de Telegram.
	}
}

// chunkFlushLoop vuelca los chunks pendientes fuera de la goroutine que está
// descargando. Los avisos llegan por chunkFlushCh.
func (e *Engine) chunkFlushLoop() {
	for itemID := range e.chunkFlushCh {
		e.persistSeenChunks(itemID)
	}
}

// requestChunkFlush pide ese volcado sin bloquear nunca. Si el volcador está
// ocupado el aviso se descarta y no pasa nada: los chunks pendientes siguen en
// memoria y el siguiente tick, o el volcado síncrono del final de la tarea, los
// recoge igual.
func (e *Engine) requestChunkFlush(itemID string) {
	if e.storage == nil {
		return
	}
	// Mismo candado y misma regla que enqueuePersist: el envío y el cierre de
	// Shutdown son mutuamente excluyentes, así que jamás se envía a un canal
	// cerrado. El select es no bloqueante, así que el candado no se retiene.
	e.mu.Lock()
	if e.persistClosed {
		e.mu.Unlock()
		// Motor apagando: los chunks siguen en memoria y ya no se les va a
		// pedir volcado por esta vía; el proceso va a terminar.
		return
	}
	select {
	case e.chunkFlushCh <- itemID:
	default:
	}
	e.mu.Unlock()
}

// persistSeenChunks es la versión síncrona. Se sigue llamando tal cual al
// terminar, pausar o cancelar una tarea, que son los momentos en los que sí
// hace falta que los chunks estén en disco antes de seguir.
func (e *Engine) persistSeenChunks(itemID string) {
	if e.storage == nil {
		return
	}
	e.mu.Lock()
	chunks := e.pendingChunks[itemID]
	e.pendingChunks[itemID] = nil
	e.mu.Unlock()

	if len(chunks) > 0 {
		if err := e.storage.AddChunks(itemID, chunks); err != nil {
			log.Printf("[DOWNLOADER] %s", i18n.T("downloader.saveChunksError", err))
		}
	}
}
