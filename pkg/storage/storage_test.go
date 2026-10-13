package storage

import (
	"database/sql"
	"path/filepath"
	"testing"

	"tgdown/pkg/config"
)

func newTestStorage(t *testing.T) *Storage {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.sqlite3")
	st, err := NewStorage(dbPath)
	if err != nil {
		t.Fatalf("no se pudo crear el storage de prueba: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestAPITokenRoundTrip(t *testing.T) {
	st := newTestStorage(t)

	tok, err := st.GetAPIToken()
	if err != nil {
		t.Fatalf("error inesperado leyendo token vacío: %v", err)
	}
	if tok != "" {
		t.Fatalf("se esperaba token vacío antes de guardar nada, got %q", tok)
	}

	if err := st.SaveAPIToken("abc123"); err != nil {
		t.Fatalf("error guardando token: %v", err)
	}

	tok, err = st.GetAPIToken()
	if err != nil {
		t.Fatalf("error leyendo token: %v", err)
	}
	if tok != "abc123" {
		t.Fatalf("token incorrecto: got %q, want %q", tok, "abc123")
	}

	// Regenerar debe sobrescribir el valor anterior, no duplicarlo.
	if err := st.SaveAPIToken("def456"); err != nil {
		t.Fatalf("error regenerando token: %v", err)
	}
	tok, err = st.GetAPIToken()
	if err != nil {
		t.Fatalf("error leyendo token regenerado: %v", err)
	}
	if tok != "def456" {
		t.Fatalf("token regenerado incorrecto: got %q, want %q", tok, "def456")
	}
}

func TestCredentialsRoundTrip(t *testing.T) {
	st := newTestStorage(t)

	if err := st.SaveCredentials("12345", "hashvalue"); err != nil {
		t.Fatalf("error guardando credenciales: %v", err)
	}

	apiID, apiHash, err := st.GetCredentials()
	if err != nil {
		t.Fatalf("error leyendo credenciales: %v", err)
	}
	if apiID != "12345" || apiHash != "hashvalue" {
		t.Fatalf("credenciales incorrectas: id=%q hash=%q", apiID, apiHash)
	}
}

func TestSaveAndLoadDownloadRoundTrip(t *testing.T) {
	st := newTestStorage(t)

	item := DownloadItem{
		ID:         "item-1",
		JobID:      "job-1",
		MessageID:  10,
		ChatID:     -100123,
		FileName:   "video.mp4",
		Status:     "queued",
		Progress:   0,
		Kind:       "video",
		Source:     "manual",
		TotalBytes: 1000,
	}
	if err := st.SaveDownload(item); err != nil {
		t.Fatalf("error guardando descarga: %v", err)
	}

	loaded, err := st.LoadDownloads("")
	if err != nil {
		t.Fatalf("error cargando descargas: %v", err)
	}
	got, ok := loaded["item-1"]
	if !ok {
		t.Fatal("la descarga guardada no aparece al recargar")
	}
	if got.FileName != "video.mp4" || got.ChatID != -100123 || got.MessageID != 10 {
		t.Fatalf("datos incorrectos tras recargar: %+v", got)
	}

	// Actualizar (upsert) no debe crear una segunda fila.
	item.Status = "completed"
	item.Progress = 100
	if err := st.SaveDownload(item); err != nil {
		t.Fatalf("error actualizando descarga: %v", err)
	}
	loaded, err = st.LoadDownloads("")
	if err != nil {
		t.Fatalf("error recargando tras update: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("se esperaba 1 registro tras el upsert, hay %d", len(loaded))
	}
	if loaded["item-1"].Status != "completed" {
		t.Fatalf("el estado no se actualizó: %+v", loaded["item-1"])
	}
}

func TestChunksLifecycle(t *testing.T) {
	st := newTestStorage(t)

	// download_chunks tiene FOREIGN KEY(download_id) REFERENCES downloads(id),
	// así que la descarga padre debe existir antes de insertar sus chunks.
	if err := st.SaveDownload(DownloadItem{
		ID:       "dl-1",
		FileName: "video.mp4",
		Status:   "downloading",
	}); err != nil {
		t.Fatalf("error creando la descarga padre: %v", err)
	}

	if err := st.AddChunks("dl-1", []int64{0, 1, 2, 1}); err != nil {
		t.Fatalf("error agregando chunks: %v", err)
	}

	chunks, err := st.Chunks("dl-1")
	if err != nil {
		t.Fatalf("error leyendo chunks: %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("se esperaban 3 chunks únicos (0,1,2), hay %d: %+v", len(chunks), chunks)
	}

	if err := st.DeleteChunks("dl-1"); err != nil {
		t.Fatalf("error borrando chunks: %v", err)
	}
	chunks, err = st.Chunks("dl-1")
	if err != nil {
		t.Fatalf("error releyendo chunks tras borrar: %v", err)
	}
	if len(chunks) != 0 {
		t.Fatalf("se esperaban 0 chunks tras borrar, hay %d", len(chunks))
	}
}

func TestClearFinishedDownloadsKeepsActiveOnes(t *testing.T) {
	st := newTestStorage(t)

	statuses := map[string]string{
		"active-1":    "downloading",
		"queued-1":    "queued",
		"done-1":      "completed",
		"failed-1":    "failed",
		"cancelled-1": "cancelled",
	}
	for id, status := range statuses {
		if err := st.SaveDownload(DownloadItem{ID: id, Status: status, FileName: id}); err != nil {
			t.Fatalf("error guardando %s: %v", id, err)
		}
	}

	removed, err := st.ClearFinishedDownloads()
	if err != nil {
		t.Fatalf("error limpiando historial: %v", err)
	}
	if removed != 3 {
		t.Fatalf("se esperaban 3 registros eliminados (done/failed/cancelled), se eliminaron %d", removed)
	}

	remaining, err := st.LoadDownloads("")
	if err != nil {
		t.Fatalf("error recargando tras limpiar: %v", err)
	}
	if _, ok := remaining["active-1"]; !ok {
		t.Fatal("una descarga activa no debería borrarse al limpiar el historial")
	}
	if _, ok := remaining["queued-1"]; !ok {
		t.Fatal("una descarga en cola no debería borrarse al limpiar el historial")
	}
	if len(remaining) != 2 {
		t.Fatalf("se esperaban 2 descargas restantes, hay %d", len(remaining))
	}
}

func TestListenerChatsRoundTripWithTopics(t *testing.T) {
	st := newTestStorage(t)

	cfg := config.DefaultConfig()
	cfg.ListenerChats = []config.ListenerChat{
		{ID: -1001, Name: "Mi Grupo", FPhotos: true, FVideos: true, FAudios: true, FDocs: true, FStickers: true},
		{ID: -1001, TopicID: config.TopicPointer(57), TopicName: "Películas", Name: "Mi Grupo", AutoDownload: true, FVideos: true},
		{ID: -1001, TopicID: config.TopicPointer(9), TopicName: "Música", Name: "Mi Grupo", FAudios: true},
	}
	cfg = config.NormalizeConfig(cfg)

	if err := st.SaveConfig(cfg); err != nil {
		t.Fatalf("error guardando la configuración: %v", err)
	}

	loaded, err := st.LoadConfig(config.DefaultConfig(), "")
	if err != nil {
		t.Fatalf("error recargando la configuración: %v", err)
	}
	if len(loaded.ListenerChats) != 3 {
		t.Fatalf("un grupo debe poder tener varias entradas por tema, hay %d: %+v",
			len(loaded.ListenerChats), loaded.ListenerChats)
	}

	byKey := make(map[string]config.ListenerChat, len(loaded.ListenerChats))
	for _, c := range loaded.ListenerChats {
		byKey[c.Key()] = c
	}

	whole, ok := byKey["-1001"]
	if !ok || whole.HasTopic() {
		t.Fatalf("falta la entrada del grupo entero: %+v", loaded.ListenerChats)
	}

	pelis, ok := byKey["-1001:57"]
	if !ok {
		t.Fatalf("falta la entrada del tema 57: %+v", loaded.ListenerChats)
	}
	if pelis.TopicName != "Películas" || !pelis.AutoDownload {
		t.Fatalf("la entrada del tema no se guardó completa: %+v", pelis)
	}
	if got, want := pelis.DisplayName(), "Películas · Mi Grupo"; got != want {
		t.Fatalf("nombre visible incorrecto: got %q, want %q", got, want)
	}

	if _, ok := byKey["-1001:9"]; !ok {
		t.Fatalf("falta la entrada del tema 9: %+v", loaded.ListenerChats)
	}
}

func TestMigrationFromSchemaWithoutTopics(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.sqlite3")

	// Base de datos tal y como la dejaban las versiones anteriores: una sola
	// fila por grupo, con chat_id como clave primaria.
	legacy, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("no se pudo crear la base de datos antigua: %v", err)
	}
	_, err = legacy.Exec(`
		CREATE TABLE listener_chats (
			chat_id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			auto_download INTEGER NOT NULL DEFAULT 0,
			f_photos INTEGER NOT NULL DEFAULT 1,
			f_videos INTEGER NOT NULL DEFAULT 1,
			f_audios INTEGER NOT NULL DEFAULT 1,
			f_docs INTEGER NOT NULL DEFAULT 1,
			f_stickers INTEGER NOT NULL DEFAULT 1
		);
		INSERT INTO listener_chats VALUES(-1001, 'Grupo Antiguo', 1, 1, 0, 1, 1, 0);
	`)
	if err != nil {
		t.Fatalf("no se pudo preparar la tabla antigua: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("no se pudo cerrar la base de datos antigua: %v", err)
	}

	st, err := NewStorage(dbPath)
	if err != nil {
		t.Fatalf("la migración no debería impedir abrir la base de datos: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	loaded, err := st.LoadConfig(config.DefaultConfig(), "")
	if err != nil {
		t.Fatalf("error cargando la configuración migrada: %v", err)
	}
	if len(loaded.ListenerChats) != 1 {
		t.Fatalf("se esperaba conservar el chat existente: %+v", loaded.ListenerChats)
	}

	chat := loaded.ListenerChats[0]
	if chat.ID != -1001 || chat.Name != "Grupo Antiguo" {
		t.Fatalf("el chat migrado perdió datos: %+v", chat)
	}
	if chat.HasTopic() {
		t.Fatalf("un chat antiguo vigila el grupo entero, no un tema: %+v", chat)
	}
	if !chat.AutoDownload || chat.FVideos || chat.FStickers || !chat.FPhotos {
		t.Fatalf("los filtros no sobrevivieron a la migración: %+v", chat)
	}

	// Tras migrar, el mismo grupo admite entradas por tema.
	cfg := loaded
	cfg.ListenerChats = append(cfg.ListenerChats, config.ListenerChat{
		ID: -1001, TopicID: config.TopicPointer(57), TopicName: "Series", Name: "Grupo Antiguo",
	})
	if err := st.SaveConfig(config.NormalizeConfig(cfg)); err != nil {
		t.Fatalf("no se pudo guardar una entrada por tema tras migrar: %v", err)
	}
	again, err := st.LoadConfig(config.DefaultConfig(), "")
	if err != nil {
		t.Fatalf("error recargando: %v", err)
	}
	if len(again.ListenerChats) != 2 {
		t.Fatalf("se esperaban 2 entradas para el mismo grupo: %+v", again.ListenerChats)
	}
}
