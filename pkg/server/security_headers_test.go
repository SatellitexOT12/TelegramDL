package server

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// grabadorHijack es un ResponseWriter de test que implementa http.Hijacker,
// igual que el que entrega net/http al servidor real y del que depende el
// upgrade del WebSocket (/api/ws).
type grabadorHijack struct {
	*httptest.ResponseRecorder
}

// Hijack cumple la interfaz http.Hijacker sin abrir conexiones reales: en
// estos tests solo interesa que la interfaz siga siendo alcanzable a través
// del writer que recibe el handler.
func (g *grabadorHijack) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, errors.New("hijack de prueba: no abre conexiones reales")
}

// TestCabecerasSeguridadEnRespuestaEstatica cubre SEC-07 sobre una respuesta
// del panel: CSP con el mínimo exigido, anti-clickjacking, nosniff, referrer
// y permisos. La CSP jamás puede admitir unsafe-eval ni script con
// unsafe-inline: son las dos vías que convierten un XSS en robo del token de
// localStorage.
func TestCabecerasSeguridadEnRespuestaEstatica(t *testing.T) {
	s := nuevoServidorDePrueba()

	// WebHandler es el camino del modo web/remoto, donde se sirve el panel.
	w := httptest.NewRecorder()
	s.WebHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	// Handler es el camino que Wails deriva a la API en modo escritorio:
	// también debe llevar las cabeceras (el asset server de Wails solo cubre
	// los ficheros estáticos, que quedan fuera de este alcance).
	wHandler := httptest.NewRecorder()
	s.Handler().ServeHTTP(wHandler, httptest.NewRequest(http.MethodGet, "/", nil))

	for nombre, rec := range map[string]*httptest.ResponseRecorder{
		"WebHandler": w,
		"Handler":    wHandler,
	} {
		csp := rec.Header().Get("Content-Security-Policy")
		if csp == "" {
			t.Fatalf("%s: falta Content-Security-Policy", nombre)
		}
		for _, esperada := range []string{
			"default-src 'self'",
			"script-src 'self'",
			"frame-ancestors 'none'",
			"base-uri 'none'",
			"object-src 'none'",
		} {
			if !strings.Contains(csp, esperada) {
				t.Errorf("%s: la CSP no contiene %q: %s", nombre, esperada, csp)
			}
		}
		if strings.Contains(csp, "unsafe-eval") {
			t.Errorf("%s: la CSP no debe admitir unsafe-eval: %s", nombre, csp)
		}
		for _, directiva := range strings.Split(csp, ";") {
			directiva = strings.TrimSpace(directiva)
			if strings.HasPrefix(directiva, "script-src") && strings.Contains(directiva, "unsafe-inline") {
				t.Errorf("%s: script-src no debe admitir unsafe-inline: %q", nombre, directiva)
			}
		}

		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: X-Content-Type-Options = %q, se esperaba nosniff",
				nombre, rec.Header().Get("X-Content-Type-Options"))
		}
		if rec.Header().Get("X-Frame-Options") != "DENY" {
			t.Errorf("%s: X-Frame-Options = %q, se esperaba DENY",
				nombre, rec.Header().Get("X-Frame-Options"))
		}
		if rec.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%s: Referrer-Policy = %q, se esperaba no-referrer",
				nombre, rec.Header().Get("Referrer-Policy"))
		}
		if rec.Header().Get("Permissions-Policy") != "camera=(), microphone=(), geolocation=()" {
			t.Errorf("%s: Permissions-Policy = %q, se esperaban las tres cláusulas vacías",
				nombre, rec.Header().Get("Permissions-Policy"))
		}
	}
}

// TestCacheControlNoStoreEnAPI: /api/ transporta PII (teléfono y nombre en
// /api/auth/*) y el token de acceso, así que esas respuestas no deben
// quedarse en la caché del navegador. Se usa el endpoint público /api/theme,
// que responde sin token y sin storage.
func TestCacheControlNoStoreEnAPI(t *testing.T) {
	s := nuevoServidorDePrueba()

	w := httptest.NewRecorder()
	s.WebHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, publicAPIPath, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("se esperaba 200 de %s, got %d", publicAPIPath, w.Code)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q, se esperaba no-store en /api/", w.Header().Get("Cache-Control"))
	}

	// El mismo criterio en el camino de Handler (API en modo escritorio).
	wHandler := httptest.NewRecorder()
	s.Handler().ServeHTTP(wHandler, httptest.NewRequest(http.MethodGet, publicAPIPath, nil))
	if wHandler.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Handler: Cache-Control = %q, se esperaba no-store en /api/",
			wHandler.Header().Get("Cache-Control"))
	}
}

// TestRespuestaEstaticaSinNoStore: no-store es solo para /api/. Los assets
// estáticos (JS, CSS, fuentes, theme-init.js) deben seguir cacheándose; si
// llevasen no-store, cada recarga re-descargaría el panel entero.
func TestRespuestaEstaticaSinNoStore(t *testing.T) {
	s := nuevoServidorDePrueba()

	w := httptest.NewRecorder()
	s.WebHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
		t.Errorf("la respuesta estática no debe llevar no-store, Cache-Control = %q",
			w.Header().Get("Cache-Control"))
	}
	// Aun sin no-store, el resto de cabeceras de protección sí aplican.
	if w.Header().Get("Content-Security-Policy") == "" {
		t.Error("falta la CSP en la respuesta estática")
	}
}

// TestCabecerasSeguridadNoRompeElWebSocket verifica la condición que hace
// seguro el middleware para /api/ws: delega SIEMPRE el mismo ResponseWriter
// (nada de envoltorios) y solo toca w.Header() antes de llamar al handler,
// sin escribir la respuesta por su cuenta. Así el http.Hijacker que usa
// gorilla para secuestrar la conexión sigue siendo el mismo objeto, y el 101
// del handshake lo escribe gorilla a mano (las cabeceras del mapa se
// descartan al secuestrar), sin riesgo de respuesta duplicada.
func TestCabecerasSeguridadNoRompeElWebSocket(t *testing.T) {
	w := &grabadorHijack{ResponseRecorder: httptest.NewRecorder()}

	var recibido http.ResponseWriter
	interceptor := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		recibido = rw
		if _, ok := rw.(http.Hijacker); !ok {
			t.Error("el middleware envolvió el ResponseWriter y rompió http.Hijacker (upgrade de /api/ws)")
		}
	})

	securityHeadersMiddleware(interceptor).ServeHTTP(w,
		httptest.NewRequest(http.MethodGet, "/api/ws", nil))

	if recibido != w {
		t.Error("el middleware debe delegar el mismo ResponseWriter, no una envoltura")
	}
	if w.Header().Get("Content-Security-Policy") == "" {
		t.Error("falta la CSP en el mapa de cabeceras del handshake del WebSocket")
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q: /api/ws entra en /api/, debe llevar no-store",
			w.Header().Get("Cache-Control"))
	}
}
