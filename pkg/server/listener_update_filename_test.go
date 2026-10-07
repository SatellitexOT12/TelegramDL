package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"tgdown/pkg/config"
	"tgdown/pkg/downloader"
	"tgdown/pkg/listener"
	"tgdown/pkg/storage"
)

// listenerFilenameItemID es el ID del item de escucha sembrado en SQLite para
// que handleListenerUpdateFilename tenga algo real que actualizar.
const listenerFilenameItemID = "listener-item-sec29"

// newFilenameTestServer monta un servidor mínimo con storage real y un item de
// escucha ya cargado en memoria. El ListenerEngine solo puebla su mapa de items
// al construirse (leyendo de SQLite las filas con source='listener' y estado
// disponible/cancelado), por eso la siembra se hace ANTES de crearlo.
func newFilenameTestServer(t *testing.T) (*Server, *storage.Storage) {
	t.Helper()

	cfg := config.DefaultConfig()
	cfg.DownloadFolder = t.TempDir()

	st, err := storage.NewStorage(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatalf("no se pudo crear el storage de prueba: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("no se pudo cerrar el storage de prueba: %v", err)
		}
	})

	if err := st.SaveDownload(storage.DownloadItem{
		ID:       listenerFilenameItemID,
		FileName: "original.mp4",
		Status:   "available",
		Source:   "listener",
	}); err != nil {
		t.Fatalf("no se pudo sembrar el item de escucha: %v", err)
	}

	engine := downloader.NewEngine(nil, nil, cfg)
	le := listener.NewListenerEngine(nil, st, engine, cfg)

	return &Server{
		config:     cfg,
		storage:    st,
		downloader: engine,
		listener:   le,
	}, st
}

// postUpdateFilename helper que llama al handler con un payload JSON armado con
// json.Marshal, para que los \r\n del nombre viajen escapados como en un
// request real.
func postUpdateFilename(t *testing.T, s *Server, id, fileName string) *httptest.ResponseRecorder {
	t.Helper()

	payload, err := json.Marshal(map[string]string{"id": id, "file_name": fileName})
	if err != nil {
		t.Fatalf("no se pudo serializar el payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/listener/update-filename",
		strings.NewReader(string(payload)))
	w := httptest.NewRecorder()
	s.handleListenerUpdateFilename(w, req)
	return w
}

// TestHandleListenerUpdateFilenameSanitizesInput cubre SEC-29: el nombre que
// llega en file_name es entrada no confiable y no puede llegar a SQLite con
// CR/LF (inyección de log) ni con separadores de ruta.
func TestHandleListenerUpdateFilenameSanitizesInput(t *testing.T) {
	s, st := newFilenameTestServer(t)

	const raw = "../evil\r\n[ERROR] inyectado.mp4"
	w := postUpdateFilename(t, s, listenerFilenameItemID, raw)
	if w.Code != http.StatusOK {
		t.Fatalf("se esperaba 200, got %d: %s", w.Code, w.Body.String())
	}

	// Lo que queda persistido en SQLite debe coincidir con la versión saneada.
	saved, err := st.LoadDownloads("")
	if err != nil {
		t.Fatalf("no se pudo releer SQLite: %v", err)
	}
	item, ok := saved[listenerFilenameItemID]
	if !ok {
		t.Fatalf("el item %q ya no está en SQLite", listenerFilenameItemID)
	}

	want := downloader.SanitizeFileName(strings.TrimSpace(raw))
	if item.FileName != want {
		t.Errorf("file_name guardado = %q, se esperaba %q", item.FileName, want)
	}
	if strings.ContainsAny(item.FileName, "\r\n") {
		t.Errorf("file_name guardado aún contiene CR/LF: %q", item.FileName)
	}
	if strings.ContainsAny(item.FileName, `/\`) {
		t.Errorf("file_name guardado aún contiene separadores de ruta: %q", item.FileName)
	}
	if item.FileName == strings.TrimSpace(raw) {
		t.Error("el nombre crudo llegó sin sanitizar a SQLite")
	}

	// El item en memoria del escuchador (el que se registra en log) también.
	for _, live := range s.listener.GetItems() {
		if live.ID != listenerFilenameItemID {
			continue
		}
		if live.FileName != want {
			t.Errorf("FileName en memoria = %q, se esperaba %q", live.FileName, want)
		}
	}
}

// TestHandleListenerUpdateFilenameKeepsEmptyNameBadRequest comprueba que el 400
// actual se conserva: la vacíez se evalúa sobre el valor crudo, antes de
// sanitizar (si no, SanitizeFileName devolvería "archivo" y el caso pasaría a
// ser un 200).
func TestHandleListenerUpdateFilenameKeepsEmptyNameBadRequest(t *testing.T) {
	s, _ := newFilenameTestServer(t)

	for _, fileName := range []string{"", "   "} {
		w := postUpdateFilename(t, s, listenerFilenameItemID, fileName)
		if w.Code != http.StatusBadRequest {
			t.Errorf("file_name=%q: se esperaba 400, got %d: %s",
				fileName, w.Code, w.Body.String())
		}
	}

	// Sin ID también sigue siendo 400.
	w := postUpdateFilename(t, s, "", "nombre.mp4")
	if w.Code != http.StatusBadRequest {
		t.Errorf("id vacío: se esperaba 400, got %d: %s", w.Code, w.Body.String())
	}
}
