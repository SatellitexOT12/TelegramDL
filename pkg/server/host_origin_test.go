package server

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"tgdown/pkg/config"
)

// themeRequest arma un GET al endpoint público /api/theme (no necesita token
// ni storage) con el Host forzado y, si se indica, la cabecera Origin: es la
// petición mínima con la que se observa por separado el middleware de Host
// (SEC-08) y el de CORS.
func themeRequest(host, origin string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, publicAPIPath, nil)
	req.Host = host
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	return req
}

// SEC-08: un Host de dominio punteado que no nombra a este servidor recibe 421
// Misdirected Request y NINGÚN handler se alcanza. Es el caso del DNS
// rebinding: evil.com resuelve a 127.0.0.1, el Origin y el Host coinciden
// entre sí, pero ninguno de los dos nombrea a este servidor.
func TestHostRejectedForForeignDottedDomain(t *testing.T) {
	s := nuevoServidorDePrueba()

	// Endpoint público: sin el middleware respondería 200.
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, themeRequest("evil.com:8000", ""))
	if w.Code == http.StatusOK {
		t.Fatal("evil.com no debe llegar al handler: la respuesta no puede ser 200")
	}
	if w.Code != http.StatusMisdirectedRequest {
		t.Fatalf("Host de dominio ajeno: se esperaba 421, got %d", w.Code)
	}

	// Endpoint protegido: sin el 421 devolvería 401 (o 403 en otro guardia).
	// Se comprueba explícitamente que no aparece ninguno de esos códigos,
	// para demostrar que el corte ocurre ANTES de la autenticación.
	reqAuth := httptest.NewRequest(http.MethodGet, "/api/downloads", nil)
	reqAuth.Host = "evil.com:8000"
	wAuth := httptest.NewRecorder()
	s.Handler().ServeHTTP(wAuth, reqAuth)
	if wAuth.Code == http.StatusUnauthorized || wAuth.Code == http.StatusForbidden {
		t.Fatalf("el 421 de SEC-08 debe llegar antes que la autenticación: got %d", wAuth.Code)
	}
	if wAuth.Code != http.StatusMisdirectedRequest {
		t.Fatalf("Host de dominio ajeno sobre endpoint protegido: se esperaba 421, got %d", wAuth.Code)
	}

	// WebHandler (modo web y acceso remoto) corta igual que Handler.
	wWeb := httptest.NewRecorder()
	s.WebHandler().ServeHTTP(wWeb, themeRequest("evil.com:8000", ""))
	if wWeb.Code != http.StatusMisdirectedRequest {
		t.Fatalf("WebHandler con Host ajeno: se esperaba 421, got %d", wWeb.Code)
	}

	// Rebinding en su forma completa: Host y Origin iguales entre sí
	// (evil.com y http://evil.com). El «mismo origen» no salva al Host.
	wRebind := httptest.NewRecorder()
	s.Handler().ServeHTTP(wRebind, themeRequest("evil.com:8000", "http://evil.com:8000"))
	if wRebind.Code != http.StatusMisdirectedRequest {
		t.Fatalf("rebinding con Host y Origin coincidentes: se esperaba 421, got %d", wRebind.Code)
	}
}

// SEC-08: la lista de Host que sí se admiten. Cubre la matriz de «no romper»:
// loopback, LAN, Tailscale (IP y MagicDNS), mDNS, la ventana de Wails, los
// nombres cortos de LAN y la ausencia de cabecera Host en las llamadas
// directas a los handlers.
func TestHostValidationAllowsExpectedHosts(t *testing.T) {
	permitidos := []string{
		"localhost:8000",
		"127.0.0.1:8000",
		"192.168.1.40:8000",
		"100.64.0.5:8000",
		"wails.localhost:34115",
		"wails.localhost",
		"machine.tailnet.ts.net:8000",
		"mispeluquin:8000",
		"mipc.local:8000",
		"[::1]:8000",
		"", // sin cabecera Host (solo posible llamando al handler a mano)
	}

	for _, host := range permitidos {
		s := nuevoServidorDePrueba()
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, themeRequest(host, ""))
		if w.Code == http.StatusMisdirectedRequest {
			t.Errorf("Host %q debería estar permitido, pero ha recibido 421", host)
			continue
		}
		if w.Code != http.StatusOK {
			t.Errorf("Host %q debería llegar al handler (200), got %d", host, w.Code)
		}
	}
}

// SEC-08: el host concreto de la configuración de escucha (bind_host) se
// admite aunque sea un dominio punteado. La variable de entorno es la vía que
// lee config.GetServerHost() por delante de la base de datos, y t.Setenv la
// restaura al terminar el test.
func TestHostValidationAllowsConfiguredBindHost(t *testing.T) {
	t.Setenv("TGDL_BIND_HOST", "panel.example.com")

	if !hostAllowed("panel.example.com:8000") {
		t.Error("el bind_host concreto debe estar permitido con su puerto")
	}
	if !hostAllowed("panel.example.com") {
		t.Error("el bind_host concreto debe estar permitido sin puerto")
	}
	// Cualquier otro dominio punteado sigue fuera, aunque el bind_host lo sea.
	if hostAllowed("otro-ejemplo.net:8000") {
		t.Error("un dominio distinto del bind_host no debe estar permitido")
	}
}

// SEC-08: mismo origen remoto reflejado en Access-Control-Allow-Origin. Es la
// matriz de «no romper» de REMOTE_ACCESS.md: el móvil entrando por
// http://192.168.x.x:8000 ve el panel servido desde esa misma dirección.
func TestCorsReflectsRemoteSameOrigin(t *testing.T) {
	s := nuevoServidorDePrueba()

	origin := "http://192.168.1.40:8000"
	w := httptest.NewRecorder()
	s.WebHandler().ServeHTTP(w, themeRequest("192.168.1.40:8000", origin))

	if w.Code != http.StatusOK {
		t.Fatalf("mismo origen remoto debe llegar al handler (200), got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Errorf("Access-Control-Allow-Origin = %q, se esperaba el reflejo %q", got, origin)
	}
}

// SEC-08: sin Access-Control-Allow-Origin cuando el Origin no es este
// servidor. Se comprueba el dominio ajeno (que el Host sí deja pasar) y el
// loopback con un puerto que no es el del servidor: era el agujero que daba
// CORS de lectura a cualquier proceso local montado en un puerto cualquiera.
func TestCorsOmitsAllowOriginForForeignOrigins(t *testing.T) {
	s := nuevoServidorDePrueba()

	// Origin de dominio ajeno con Host permitido: la petición pasa (sin
	// Origin no habría CORS; con Origin desechado se omite la cabecera), lo
	// que demuestra que el corte es del CORS y no del middleware de Host.
	wForeign := httptest.NewRecorder()
	s.WebHandler().ServeHTTP(wForeign, themeRequest("localhost:8000", "http://evil.com"))
	if wForeign.Code != http.StatusOK {
		t.Fatalf("Host permitido con Origin ajeno debe seguir respondiendo (200), got %d", wForeign.Code)
	}
	if got := wForeign.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Origin http://evil.com no debe recibir ACAO, got %q", got)
	}

	// Loopback con puerto que no corresponde al servidor (ni al 8080 de Vite).
	otroPuerto := config.GetServerPort() + 1
	if otroPuerto == viteDevPort {
		otroPuerto++
	}
	originRara := "http://127.0.0.1:" + strconv.Itoa(otroPuerto)
	wLoopback := httptest.NewRecorder()
	s.WebHandler().ServeHTTP(wLoopback, themeRequest("localhost:8000", originRara))
	if wLoopback.Code != http.StatusOK {
		t.Fatalf("Host permitido con puerto de loopback ajeno debe seguir respondiendo (200), got %d", wLoopback.Code)
	}
	if got := wLoopback.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Origin %s no debe recibir ACAO, got %q", originRara, got)
	}
}

// SEC-08: los orígenes legítimos de loopback con puerto concreto siguen
// recibiendo ACAO: el desarrollo con Vite (localhost:8080 con proxy
// changeOrigin), el propio puerto del servidor y la forma exacta del
// WebSocket de la ventana de escritorio (Host 127.0.0.1:8000 + Origin
// http://wails.localhost:<puerto dinámico>, App.vue:862-865). Esa misma
// isOriginAllowed es la que valida el CheckOrigin del upgrade de /api/ws.
func TestCorsAllowsViteAndWailsOrigins(t *testing.T) {
	s := nuevoServidorDePrueba()

	casos := []struct {
		nombre string
		host   string
		origin string
	}{
		{"desarrollo vite", "127.0.0.1:8000", "http://localhost:" + strconv.Itoa(viteDevPort)},
		{"puerto del servidor", "127.0.0.1:8000",
			"http://localhost:" + strconv.Itoa(config.GetServerPort())},
		{"websocket de escritorio wails", "127.0.0.1:8000", "http://wails.localhost:34115"},
	}

	for _, caso := range casos {
		w := httptest.NewRecorder()
		s.WebHandler().ServeHTTP(w, themeRequest(caso.host, caso.origin))
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != caso.origin {
			t.Errorf("%s: Access-Control-Allow-Origin = %q, se esperaba %q",
				caso.nombre, got, caso.origin)
		}
	}
}

// SEC-08 + SEC-07: la respuesta 421 sale de dentro de
// securityHeadersMiddleware, así que lleva las mismas cabeceras de protección
// que el resto de respuestas del servidor (CSP con frame-ancestors 'none',
// nosniff, etc.). Si el encadenamiento estuviera al revés, el 421 sería la
// única respuesta sin ellas.
func TestHostRejectionKeepsSecurityHeaders(t *testing.T) {
	s := nuevoServidorDePrueba()

	for nombre, handler := range map[string]http.Handler{
		"Handler":    s.Handler(),
		"WebHandler": s.WebHandler(),
	} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, themeRequest("evil.com:8000", ""))

		if w.Code != http.StatusMisdirectedRequest {
			t.Fatalf("%s: se esperaba 421, got %d", nombre, w.Code)
		}
		if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'none'") {
			t.Errorf("%s: la respuesta 421 no lleva la CSP de SEC-07: %q", nombre, csp)
		}
		if w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: la respuesta 421 no lleva X-Content-Type-Options: %q",
				nombre, w.Header().Get("X-Content-Type-Options"))
		}
		if w.Header().Get("X-Frame-Options") != "DENY" {
			t.Errorf("%s: la respuesta 421 no lleva X-Frame-Options: %q",
				nombre, w.Header().Get("X-Frame-Options"))
		}
		// La ruta es de /api/, así que también debe heredar no-store.
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s: la respuesta 421 de /api/ no lleva no-store: %q",
				nombre, w.Header().Get("Cache-Control"))
		}
	}
}

// Matriz de «no romper»: el CLI `tgdown <url>` y curl no mandan cabecera
// Origin. Sin Origin solo se omite el CORS de siempre (nada de 421 ni de 403),
// siempre que el Host sí sea este servidor.
func TestRequestWithoutOriginIsNotBlocked(t *testing.T) {
	s := nuevoServidorDePrueba()

	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, themeRequest("192.168.1.40:8000", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("una petición sin Origin no debe dejar de funcionar: se esperaba 200, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("sin Origin no debe haber Access-Control-Allow-Origin, got %q", got)
	}
}

// El host que se escribe en el registro viene de una cabecera, así que se
// sanea: sin CR/LF no se puede fabricar una línea falsa en el log y con el
// tope de hostLogMaxLen no se coloca un texto de tamaño arbitrario.
func TestSanitizeLogHostStripsControlCharsAndTruncates(t *testing.T) {
	inyectado := "evil.com\r\n[ERROR] falso aviso\r\n" + strings.Repeat("a", 300)

	limpio := sanitizeLogHost(inyectado)

	if strings.ContainsAny(limpio, "\r\n") {
		t.Errorf("el host saneado aún contiene CR/LF: %q", limpio)
	}
	if len(limpio) > hostLogMaxLen {
		t.Errorf("el host saneado mide %d bytes, el tope es %d", len(limpio), hostLogMaxLen)
	}
	if !strings.HasPrefix(limpio, "evil.com") {
		t.Errorf("el host saneado debe conservar el valor depurado: %q", limpio)
	}
}
