package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tgdown/pkg/telegram"
)

// nuevoServidorDePrueba monta el enrutado completo del servidor sin sesión
// real de Telegram, sin red y sin disco: solo el mux, un token de prueba y la
// costura haySesion sustituida. Los handlers que necesitan storage no se
// alcanzan en estos tests.
func nuevoServidorDePrueba() *Server {
	s := &Server{
		mux:       http.NewServeMux(),
		apiToken:  "token-de-prueba-0123456789abcdef",
		clientMgr: &telegram.ClientManager{},
		haySesion: func() bool { return false },
	}
	s.registerRoutes(s.mux)
	return s
}

// peticionSetup manda una petición de la forma exacta que manda el panel:
// POST con cuerpo JSON y la cabecera propia, sin token.
func peticionSetup(s *Server, metodo, destino, cuerpo string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(metodo, destino, strings.NewReader(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(ownRequestHeader, "1")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	return w
}

// TestSuperficiePublica fija la lista completa de lo que responde sin token
// (SEC-01 y test hueco de SEC-26): solo lookup exacto, nunca por prefijo.
func TestSuperficiePublica(t *testing.T) {
	publicos := []string{
		publicAPIPath,
		hasCredentialsAPIPath,
		tokenSendAPIPath,
		"/api/ipc/download",
	}
	if len(publicAPIPaths) != len(publicos) {
		t.Fatalf("la lista de públicos ha cambiado: hay %d entradas, se esperaban %d",
			len(publicAPIPaths), len(publicos))
	}
	for _, path := range publicos {
		if !isPublicAPIPath(path) {
			t.Errorf("%s debería ser público", path)
		}
	}

	privados := []string{
		// La puerta de setup: sin token directo, solo vía setupSinToken.
		"/api/auth/status",
		"/api/auth/credentials",
		"/api/auth/send-code",
		"/api/auth/verify-code",
		"/api/auth/verify-2fa",
		"/api/auth/logout",
		// El resto de la API exige token por el camino normal.
		"/api/settings",
		"/api/auth/token",
		"/api/downloads",
	}
	for _, path := range privados {
		if isPublicAPIPath(path) {
			t.Errorf("%s no debe ser público: exige token o la puerta de setup", path)
		}
	}
}

// SEC-04: sin la cabecera propia el navegador no hace el preflight de CORS,
// así que una página web podría mandar el POST en modo no-cors.
func TestSetupSinTokenExigeCabeceraPropia(t *testing.T) {
	s := nuevoServidorDePrueba()

	req := httptest.NewRequest(http.MethodPost, "/api/auth/credentials",
		strings.NewReader(`{"api_id":"1","api_hash":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	// Sin req.Header.Set(ownRequestHeader, ...): es lo que se comprueba.
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("sin la cabecera propia se esperaba 403, got %d", w.Code)
	}
}

// SEC-01: logout (y el resto de mutaciones) por GET o por <img> debe morir en
// la puerta con 405.
func TestSetupSinTokenExigePOST(t *testing.T) {
	s := nuevoServidorDePrueba()

	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/auth/logout", nil))

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("un GET sin token se esperaba 405, got %d", w.Code)
	}
}

// SEC-04: el decoder JSON ignora el Content-Type, por eso un text/plain en
// modo no-cors llegaba antes hasta el handler.
func TestSetupSinTokenExigeContentTypeJSON(t *testing.T) {
	s := nuevoServidorDePrueba()

	req := httptest.NewRequest(http.MethodPost, "/api/auth/credentials",
		strings.NewReader(`{"api_id":"1","api_hash":"x"}`))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set(ownRequestHeader, "1")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("con text/plain se esperaba 415, got %d", w.Code)
	}
}

// SEC-01: en cuanto hay sesión guardada, hasta una petición bien formada de
// la puerta exige token (401 estándar de authMiddleware).
func TestSetupRequiereTokenCuandoHaySesion(t *testing.T) {
	s := nuevoServidorDePrueba()
	s.haySesion = func() bool { return true }

	w := peticionSetup(s, http.MethodPost, "/api/auth/credentials", `{"api_id":"1","api_hash":"x"}`)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("con sesión guardada se esperaba 401, got %d", w.Code)
	}
}

// SEC-09: 5 intentos por minuto y origen; la sexta recibe 429 con Retry-After
// y un origen distinto no se ve afectado.
func TestSetupLimitaLaFrecuencia(t *testing.T) {
	s := nuevoServidorDePrueba()

	// Las cinco primeras pasan la puerta y llegan al handler, que responde
	// 422 porque el cuerpo no trae teléfono (nada de red ni de storage).
	for i := 1; i <= 5; i++ {
		w := peticionSetup(s, http.MethodPost, "/api/auth/send-code", `{}`)
		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("intento %d: se esperaba llegar al handler (422), got %d", i, w.Code)
		}
	}

	// La sexta en el mismo minuto se corta.
	w := peticionSetup(s, http.MethodPost, "/api/auth/send-code", `{}`)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("la sexta petición debe recibir 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("falta la cabecera Retry-After")
	}
	var cuerpo struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if cuerpo.Status != "rate_limited" {
		t.Fatalf("estado inesperado: %q", cuerpo.Status)
	}

	// Otro origen distinto sigue pasando.
	req := httptest.NewRequest(http.MethodPost, "/api/auth/send-code", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(ownRequestHeader, "1")
	req.RemoteAddr = "198.51.100.42:60000"
	w2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(w2, req)

	if w2.Code == http.StatusTooManyRequests {
		t.Fatal("el límite de un origen no debe aplicarse a otro distinto")
	}
	if w2.Code != http.StatusUnprocessableEntity {
		t.Fatalf("el segundo origen debe llegar al handler (422), got %d", w2.Code)
	}
}

// Primera arrancada: sin sesión guardada, una petición bien formada de la
// puerta llega al handler. Se usa logout porque su handler no necesita
// storage (el de /api/auth/status sí, y construirlo obligaría a disco en este
// test); el estado de status se comprueba aparte, directo sobre la puerta.
func TestSetupSinTokenAdmiteLaPrimeraConfiguracion(t *testing.T) {
	s := nuevoServidorDePrueba()

	w := peticionSetup(s, http.MethodPost, "/api/auth/logout", `{}`)
	if w.Code != http.StatusOK {
		t.Fatalf("sin sesión, una configuración bien formada debe llegar al handler: got %d", w.Code)
	}
	var cuerpo map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if cuerpo["status"] != "ok" {
		t.Fatalf("estado inesperado: %q", cuerpo["status"])
	}

	// /api/auth/status sin token pasa la puerta sin consumir cuota ni exigir
	// POST: sin sesión responde de inmediato el estado «UNCONFIGURED».
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
		ok, codigo, _, _ := s.setupSinToken(req)
		if !ok || codigo != 0 {
			t.Fatalf("status sin token debe pasar la puerta (intento %d): ok=%v codigo=%d", i+1, ok, codigo)
		}
	}
}

// SEC-03: /api/ipc/download sigue siendo público para el CLI, pero solo
// acepta loopback con la cabecera propia.
func TestIPCDownloadSoloLoopback(t *testing.T) {
	s := nuevoServidorDePrueba()

	// (a) Origen remoto (Host y RemoteAddr de fuera) → 401.
	req := httptest.NewRequest(http.MethodPost, "http://panel.example.com/api/ipc/download",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(ownRequestHeader, "1")
	req.RemoteAddr = "203.0.113.7:51000"
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("petición remota sin token: se esperaba 401, got %d", w.Code)
	}

	// (b) Loopback sin la cabecera propia → 403.
	req2 := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8000/api/ipc/download",
		strings.NewReader(`{}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.RemoteAddr = "127.0.0.1:51001"
	w2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(w2, req2)
	if w2.Code != http.StatusForbidden {
		t.Fatalf("loopback sin cabecera propia: se esperaba 403, got %d", w2.Code)
	}

	// (c) Loopback con cabecera: pasa todos los guardias y llega al decode;
	// con un cuerpo sin URL el propio handler responde 422.
	req3 := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8000/api/ipc/download",
		strings.NewReader(`{}`))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set(ownRequestHeader, "1")
	req3.RemoteAddr = "127.0.0.1:51002"
	w3 := httptest.NewRecorder()
	s.Handler().ServeHTTP(w3, req3)
	if w3.Code != http.StatusUnprocessableEntity {
		t.Fatalf("loopback bien formado debe llegar al decode (422), got %d", w3.Code)
	}
}

// Quien trae token no pasa por la puerta de setup, ni siquiera con sesión
// guardada: el camino del token no puede verse afectado.
func TestConTokenNoPasaPorLaPuertaDeSetup(t *testing.T) {
	s := nuevoServidorDePrueba()
	s.haySesion = func() bool { return true }

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+s.apiToken)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("con token se esperaba llegar al handler (200), got %d", w.Code)
	}
}

// SEC-24: camino de éxito de Logout. El de error no se puede forzar sin
// trucos mientras ClientManager.Logout devuelva nil sin más
// (pkg/telegram/client.go); cuando empiece a propagar fallos, este mismo test
// debe poder ampliarse con un manejador que falle.
func TestLogoutRespondeOkCuandoNoHayError(t *testing.T) {
	s := &Server{clientMgr: &telegram.ClientManager{}}
	w := httptest.NewRecorder()

	s.handleAuthLogout(w, httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("sin error se esperaba 200, got %d", w.Code)
	}
	var cuerpo map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if cuerpo["status"] != "ok" {
		t.Fatalf("estado inesperado: %q", cuerpo["status"])
	}
}
