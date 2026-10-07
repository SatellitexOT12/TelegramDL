package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// peticionLogout manda POST /api/auth/logout con la forma del panel: Host
// permitido (SEC-08), Content-Type JSON y la cabecera propia (necesaria para
// que la puerta de setup admita la petición sin token). Si token no está
// vacío se añade Authorization Bearer; con token vacío la petición va sin él.
// body nil replica a las llamadas antiguas (AuthWizard): van sin cuerpo.
func peticionLogout(s *Server, body io.Reader, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", body)
	// Host permitido: httptest.NewRequest deja r.Host = "example.com" y
	// hostValidationMiddleware (SEC-08) respondería 421.
	req.Host = "localhost:8000"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(ownRequestHeader, "1")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	return w
}

// compruebaTokenSinRotar falla si el token vigente dejó de ser el esperado.
func compruebaTokenSinRotar(t *testing.T, s *Server, original string) {
	t.Helper()
	if s.APIToken() != original {
		t.Fatalf("el token no debe cambiar: original %q, vigente %q", original, s.APIToken())
	}
}

// SEC-27 (caso 1): con sesión de trabajo, revoke_token: true y un token
// válido en la cabecera, el logout gira el token y el viejo queda muerto.
//
// Camino ejercitado: authMiddleware (token correcto) → handleAuthLogout →
// clientMgr.Logout devuelve nil con un ClientManager vacío (misma costura
// que usa TestLogoutRespondeOkCuandoNoHayError; pkg/telegram/client.go
// devuelve siempre nil y no consulta haySesion, por eso no hace falta
// trucar la sesión para llegar al éxito) → condiciones (b) y (c) cumplidas →
// RegenerateToken (storage nil, sin clientes WS: solo gira el token).
func TestLogoutConRevokeTokenGiraElToken(t *testing.T) {
	s := nuevoServidorDePrueba()
	tokenViejo := s.APIToken()

	w := peticionLogout(s, strings.NewReader(`{"revoke_token":true}`), tokenViejo)
	if w.Code != http.StatusOK {
		t.Fatalf("se esperaba 200, got %d", w.Code)
	}
	if s.APIToken() == tokenViejo {
		t.Fatal("revoke_token autenticado debe girar el token")
	}
	if s.APIToken() == "" {
		t.Fatal("el token nuevo no puede quedar vacío")
	}
	// SEC-25: el token nuevo jamás viaja en la respuesta.
	if strings.Contains(w.Body.String(), s.APIToken()) {
		t.Fatalf("la respuesta no debe contener el token nuevo: %s", w.Body.String())
	}

	// El token viejo ya no abre nada: el endpoint protegido responde 401.
	req := httptest.NewRequest(http.MethodGet, "/api/downloads", nil)
	req.Host = "localhost:8000"
	req.Header.Set("Authorization", "Bearer "+tokenViejo)
	w2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(w2, req)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("el token viejo debe dar 401, got %d", w2.Code)
	}
}

// SEC-27 (caso 2): sin el campo revoke_token el token no cambia. «sin
// cuerpo» es la llamada real de AuthWizard (cancela o retrocede el flujo de
// login) y «{}» un cuerpo válido sin el flag; ambos deben responder 200 con
// {"status":"ok"}, igual que antes de tocar el handler (regresión del
// asistente).
func TestLogoutSinFlagNoGiraElToken(t *testing.T) {
	casos := map[string]io.Reader{
		"sin cuerpo":      nil,
		"cuerpo vacío":    strings.NewReader(""),
		"objeto sin flag": strings.NewReader(`{}`),
	}
	for nombre, cuerpo := range casos {
		t.Run(nombre, func(t *testing.T) {
			s := nuevoServidorDePrueba()
			tokenOriginal := s.APIToken()

			w := peticionLogout(s, cuerpo, tokenOriginal)
			if w.Code != http.StatusOK {
				t.Fatalf("se esperaba 200, got %d", w.Code)
			}
			if !strings.Contains(w.Body.String(), `"status":"ok"`) {
				t.Fatalf("respuesta inesperada: %s", w.Body.String())
			}
			compruebaTokenSinRotar(t, s, tokenOriginal)
		})
	}
}

// SEC-27 (caso 3): revoke_token: true pero SIN token y sin sesión de
// Telegram: la petición pasa por la puerta de setup, el flag se ignora y el
// token legítimo sigue intacto. Sin esta condición (c), un atacante sin
// token podría invalidar el token legítimo (DoS de rotación no autenticada).
func TestLogoutConFlagSinTokenNoGiraElToken(t *testing.T) {
	s := nuevoServidorDePrueba() // haySesion → false: puerta de setup abierta
	tokenOriginal := s.APIToken()

	w := peticionLogout(s, strings.NewReader(`{"revoke_token":true}`), "")
	if w.Code != http.StatusOK {
		t.Fatalf("la puerta de setup debe dejar pasar la petición (200), got %d", w.Code)
	}
	compruebaTokenSinRotar(t, s, tokenOriginal)
}

// SEC-27 (caso 4): revoke_token: true con un token INVÁLIDO. Con sesión
// guardada la puerta de setup remite a authMiddleware (código 0 → 401) y ni
// siquiera se llega al handler; sin sesión, la propia puerta lo admite y el
// flag se ignora por la misma condición (c). En ninguno de los dos caminos
// gira el token.
func TestLogoutConFlagYTokenInvalidoNoGiraElToken(t *testing.T) {
	t.Run("con sesión guardada responde 401", func(t *testing.T) {
		s := nuevoServidorDePrueba()
		s.haySesion = func() bool { return true }
		tokenOriginal := s.APIToken()

		w := peticionLogout(s, strings.NewReader(`{"revoke_token":true}`), "token-invalido")
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("token inválido con sesión debe dar 401, got %d", w.Code)
		}
		compruebaTokenSinRotar(t, s, tokenOriginal)
	})

	t.Run("sin sesión la puerta lo admite pero no gira", func(t *testing.T) {
		s := nuevoServidorDePrueba()
		tokenOriginal := s.APIToken()

		w := peticionLogout(s, strings.NewReader(`{"revoke_token":true}`), "token-invalido")
		if w.Code != http.StatusOK {
			t.Fatalf("sin sesión la puerta responde 200, got %d", w.Code)
		}
		compruebaTokenSinRotar(t, s, tokenOriginal)
	})
}

// SEC-27 (caso 5): revoke_token: false explícito con token válido: el flag
// manda y el token no cambia (el cliente pidió cerrar sesión sin revocar).
func TestLogoutConFlagExplicitoFalseNoGiraElToken(t *testing.T) {
	s := nuevoServidorDePrueba()
	tokenOriginal := s.APIToken()

	w := peticionLogout(s, strings.NewReader(`{"revoke_token":false}`), tokenOriginal)
	if w.Code != http.StatusOK {
		t.Fatalf("se esperaba 200, got %d", w.Code)
	}
	compruebaTokenSinRotar(t, s, tokenOriginal)
}
