package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIsPublicAPIPath(t *testing.T) {
	// El envío del token tiene que responder sin token: quien pulsa el botón es
	// justo quien no lo tiene.
	for _, path := range []string{publicAPIPath, tokenSendAPIPath} {
		if !isPublicAPIPath(path) {
			t.Fatalf("%s debería ser público", path)
		}
	}
	// Cualquier otra cosa sigue exigiendo token.
	for _, path := range []string{"/api/settings", "/api/auth/token", "/api/auth/token/regenerate", "/api/downloads"} {
		if isPublicAPIPath(path) {
			t.Fatalf("%s NO debería ser público", path)
		}
	}
}

func TestBuildTokenMessageDejaElTokenEnSuPropiaLinea(t *testing.T) {
	const token = "0123456789abcdef0123456789abcdef"
	msg := buildTokenMessage(token)

	lines := strings.Split(msg, "\n")
	found := false
	for _, line := range lines {
		if line == token {
			found = true
			break
		}
	}
	if !found {
		// Si el token comparte línea con otro texto, el formato de código lo
		// abarcaría de más y copiarlo de un toque dejaría de funcionar.
		t.Fatalf("el token debe ocupar una línea entera:\n%s", msg)
	}
	if strings.Count(msg, token) != 1 {
		t.Fatalf("el token no debe aparecer dos veces:\n%s", msg)
	}
}

func TestRequestOriginIgnoraCabecerasDeProxy(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, tokenSendAPIPath, nil)
	r.RemoteAddr = "192.168.1.40:51234"
	// Falsificable por quien llama: no debe influir en lo que se registra.
	r.Header.Set("X-Forwarded-For", "8.8.8.8")

	if got := requestOrigin(r); got != "192.168.1.40" {
		t.Fatalf("origen incorrecto: got %q, want %q", got, "192.168.1.40")
	}
}

func TestSendTokenRechazaMetodoIncorrecto(t *testing.T) {
	s := &Server{}
	w := httptest.NewRecorder()

	s.handleSendTokenToTelegram(w, httptest.NewRequest(http.MethodGet, tokenSendAPIPath, nil))

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("un GET debe rechazarse: got %d", w.Code)
	}
}

func TestSendTokenAplicaLimiteDeFrecuencia(t *testing.T) {
	s := &Server{apiToken: "abc"}
	// Turno ya consumido: el siguiente intento no debe llegar a Telegram.
	s.tokenSendNextAt = time.Now().Add(30 * time.Second)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, tokenSendAPIPath, nil)
	r.RemoteAddr = "10.0.0.5:1234"
	r.Header.Set(cabeceraPeticionPropia, "1")
	s.handleSendTokenToTelegram(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("se esperaba 429, got %d", w.Code)
	}

	var body struct {
		Status     string `json:"status"`
		RetryAfter int    `json:"retry_after"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if body.Status != "rate_limited" {
		t.Fatalf("estado inesperado: %q", body.Status)
	}
	if body.RetryAfter < 1 || body.RetryAfter > 31 {
		t.Fatalf("la espera devuelta no es razonable: %d", body.RetryAfter)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("falta la cabecera Retry-After")
	}
	// La respuesta nunca debe filtrar el token.
	if strings.Contains(w.Body.String(), "abc") {
		t.Fatalf("la respuesta no debe incluir el token: %s", w.Body.String())
	}
}

func TestSendTokenSinTokenGenerado(t *testing.T) {
	s := &Server{}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, tokenSendAPIPath, nil)
	r.RemoteAddr = "10.0.0.5:1234"
	r.Header.Set(cabeceraPeticionPropia, "1")

	s.handleSendTokenToTelegram(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin token generado se esperaba 503, got %d", w.Code)
	}
}

// Este endpoint responde sin token, así que sin más protección cualquier página
// abierta en el navegador del usuario podría dispararlo: la petición no lleva
// cuerpo ni Content-Type, y eso la convierte en una «simple request» que el
// navegador manda sin consultar antes con CORS. Exigir una cabecera propia
// obliga al preflight, que CORS rechaza.
func TestSendTokenExigeLaCabeceraPropia(t *testing.T) {
	s := &Server{apiToken: "abc"}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, tokenSendAPIPath, nil)
	r.RemoteAddr = "10.0.0.5:1234"
	// Sin r.Header.Set(cabeceraPeticionPropia, ...): es lo que se comprueba.

	s.handleSendTokenToTelegram(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("sin la cabecera propia se esperaba 403, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "abc") {
		t.Fatalf("la respuesta no debe incluir el token: %s", w.Body.String())
	}
}

// El límite global por sí solo dejaba que cualquiera en la red llenase los
// mensajes guardados del usuario a razón de uno por minuto, día y noche.
//
// El servidor se deja sin token a propósito: así la petición que SÍ debe pasar
// el límite se detiene justo después, en la comprobación del token, y el test
// no necesita una sesión de Telegram para llegar hasta ahí.
func TestSendTokenLimitaTambienPorOrigen(t *testing.T) {
	s := &Server{
		tokenSendPorIP: map[string]time.Time{"10.0.0.5": time.Now().Add(5 * time.Minute)},
	}
	// El límite global está libre: el que debe frenar es el del origen.
	s.tokenSendNextAt = time.Time{}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, tokenSendAPIPath, nil)
	r.RemoteAddr = "10.0.0.5:1234"
	r.Header.Set(cabeceraPeticionPropia, "1")

	s.handleSendTokenToTelegram(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("un origen que ya pidió el token hace poco debe recibir 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("falta la cabecera Retry-After")
	}

	// Otro origen distinto no debe verse afectado por el límite del primero:
	// llega hasta la comprobación del token, que es la siguiente.
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodPost, tokenSendAPIPath, nil)
	r2.RemoteAddr = "10.0.0.9:1234"
	r2.Header.Set(cabeceraPeticionPropia, "1")

	s.handleSendTokenToTelegram(w2, r2)

	if w2.Code == http.StatusTooManyRequests {
		t.Fatal("el límite de un origen no debe aplicarse a otro distinto")
	}
	if w2.Code != http.StatusServiceUnavailable {
		t.Fatalf("se esperaba llegar a la comprobación del token (503), got %d", w2.Code)
	}
}

// recordTokenSendByIP limpia las entradas caducadas, para que el mapa no crezca
// sin límite si alguien insiste desde muchas direcciones.
func TestRecordTokenSendByIPCleansExpired(t *testing.T) {
	s := &Server{tokenSendPorIP: map[string]time.Time{
		"1.1.1.1": time.Now().Add(-time.Hour), // caducada
		"2.2.2.2": time.Now().Add(time.Hour),  // vigente
	}}

	s.recordTokenSendByIP("3.3.3.3", time.Now().Add(time.Hour))

	if _, hay := s.tokenSendPorIP["1.1.1.1"]; hay {
		t.Error("la entrada caducada debería haberse borrado")
	}
	if _, hay := s.tokenSendPorIP["2.2.2.2"]; !hay {
		t.Error("la entrada vigente no debería tocarse")
	}
	if _, hay := s.tokenSendPorIP["3.3.3.3"]; !hay {
		t.Error("no se registró el origen nuevo")
	}
}

// Un mapa sin inicializar no debe hacer que reviente nada.
func TestRecordTokenSendByIPWithNilMap(t *testing.T) {
	s := &Server{}
	s.recordTokenSendByIP("10.0.0.1", time.Now().Add(time.Minute))
	if len(s.tokenSendPorIP) != 1 {
		t.Fatalf("se esperaba una entrada, hay %d", len(s.tokenSendPorIP))
	}
}
