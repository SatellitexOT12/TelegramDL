package server

import (
	"sync"
	"testing"

	"tgdown/pkg/config"
)

// TestGetConfigDevuelveConfigActualizada verifica que GetConfig lee la fuente
// viva (SEC-40): tras escribir s.config bajo candado —la misma pauta que usan
// los handlers de guardado— el getter debe devolver el valor nuevo, no el que
// tenía el servidor al crearse.
func TestGetConfigDevuelveConfigActualizada(t *testing.T) {
	s := &Server{config: config.DefaultConfig()}

	// Escritura bajo candado, igual que handleSettings.
	s.mu.Lock()
	s.config.DownloadFolder = "descargas-nuevas"
	s.mu.Unlock()

	got := s.GetConfig()
	if got.DownloadFolder != "descargas-nuevas" {
		t.Errorf("DownloadFolder = %q, se esperaba %q", got.DownloadFolder, "descargas-nuevas")
	}

	// La copia es por valor: tocar lo devuelto no debe alterar la config viva.
	got.DownloadFolder = "tocada-fuera"
	if s.GetConfig().DownloadFolder != "descargas-nuevas" {
		t.Error("GetConfig debe devolver una copia, no un alias de s.config")
	}
}

// TestGetConfigBajoCandadoConcurrente ejercita el getter frente a una
// escritura concurrente protegida por el mismo candado RWMutex. Sin -race
// localmente no detecta la carrera de datos, pero deja a la vista cualquier
// deadlock o panic si el candado faltara.
func TestGetConfigBajoCandadoConcurrente(t *testing.T) {
	s := &Server{config: config.DefaultConfig()}

	var wg sync.WaitGroup
	wg.Add(2)

	// Escritor: misma pauta que handleSettings/handleSpeedLimit.
	go func() {
		defer wg.Done()
		for i := 1; i <= 200; i++ {
			s.mu.Lock()
			s.config.MaxConcurrentDownloads = i
			s.mu.Unlock()
		}
	}()

	// Lector: GetConfig debe poder llamarse desde varias goroutines a la vez.
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			cfg := s.GetConfig()
			if cfg.MaxConcurrentDownloads < 0 {
				t.Errorf("MaxConcurrentDownloads negativo: %d", cfg.MaxConcurrentDownloads)
			}
		}
	}()

	wg.Wait()
}
