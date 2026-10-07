package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"tgdown/pkg/config"
	"tgdown/pkg/downloader"
	"tgdown/pkg/listener"
	"tgdown/pkg/telegram"
)

// newRetokenTestServer monta un Server con el enrutado completo, el
// upgrader de NewServer (incluido CheckOrigin) y motores ligeros, para
// probar la rotación de token sobre un WebSocket real de verdad (SEC-35).
//
// Los constructores son inocuos en este entorno:
//
//   - downloader.NewEngine(cm, nil, cfg) no arranca ninguna goroutine
//     porque los bucles de persistencia se lanzan solo con st != nil
//     (pkg/downloader/engine.go, condición `if st != nil`);
//   - listener.NewListenerEngine(cm, nil, eng, cfg) tampoco: solo normaliza
//     la config, registra callbacks en el ClientManager y en el motor de
//     descargas; el arranque de red es de Start(), que no se llama aquí.
//
// cachedDisk se precarga para que buildStateSnapshot no consulte el espacio
// libre del disco real de la máquina que ejecuta la prueba.
func newRetokenTestServer(t *testing.T) *Server {
	t.Helper()
	cm := &telegram.ClientManager{}
	eng := downloader.NewEngine(cm, nil, config.Config{})
	le := listener.NewListenerEngine(cm, nil, eng, config.Config{})

	s := &Server{
		mux:        http.NewServeMux(),
		apiToken:   "token-de-prueba-viejo-0123456789abcdef",
		downloader: eng,
		listener:   le,
		wsClients:  make(map[*wsClient]bool),
		// Réplica exacta del upgrader de NewServer: sin cabecera Origin el
		// cliente es nativo y se admite; con Origin se valida igual que en
		// producción.
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				return origin == "" || isOriginAllowed(origin, r)
			},
		},
		cachedDisk: &downloader.DiskInfo{Status: "green"},
		stopCh:     make(chan struct{}),
	}
	s.registerRoutes(s.mux)
	return s
}

// wsDialer entrega un dialer que autentica por subprotocolos, igual que el
// panel: tgdl-v1 y, como segundo, el token. HandshakeTimeout de seguridad
// para que ninguna fase del test pueda quedarse colgada.
func wsDialer(token string) websocket.Dialer {
	return websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
		Subprotocols:     []string{wsSubprotocol, token},
	}
}

// TestTokenRotationForcesWebSocketReconnect cubre SEC-35 de extremo a
// extremo: al rotar el token, el WebSocket conectado recibe el cierre de
// aplicación 4401, el token viejo ya no abre conexiones nuevas y el token
// nuevo sí. Todo sobre loopback (httptest), sin disco ni red externa.
func TestTokenRotationForcesWebSocketReconnect(t *testing.T) {
	s := newRetokenTestServer(t)
	ts := httptest.NewServer(s.WebHandler())
	t.Cleanup(ts.Close)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws"
	tokenViejo := s.APIToken()

	// 1. Conexión con el token vigente: el handshake pasa (101)...
	dialer := wsDialer(tokenViejo)
	conexion, resp, err := dialer.Dial(wsURL, nil)
	if err != nil {
		codigo := 0
		if resp != nil {
			codigo = resp.StatusCode
		}
		t.Fatalf("el handshake con el token vigente debe funcionar (HTTP %d): %v", codigo, err)
	}
	t.Cleanup(func() {
		// Cerrar de nuevo en la limpieza es inofensivo: si la rotación ya lo
		// cerró, Close solo devuelve un error que aquí no interesa a nadie.
		_ = conexion.Close()
	})

	// ...y llega el snapshot inicial: es la señal de que el servidor ya
	// registró el cliente en s.wsClients (el registro va antes del envío en
	// handleWebSocket), así que la rotación de justo a continuación lo tiene
	// que incluir en su snapshot.
	if err := conexion.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("no se pudo fijar la fecha límite de lectura: %v", err)
	}
	_, mensaje, err := conexion.ReadMessage()
	if err != nil {
		t.Fatalf("se esperaba el snapshot inicial tras el handshake: %v", err)
	}
	var inicial map[string]any
	if err := json.Unmarshal(mensaje, &inicial); err != nil {
		t.Fatalf("el snapshot inicial debe ser JSON: %v", err)
	}
	if inicial["type"] != "state" {
		t.Fatalf("el primer mensaje debe ser el snapshot de estado, got %q", inicial["type"])
	}

	// 2. Rotación: devuelve un token nuevo y distinto del viejo.
	tokenNuevo := s.RegenerateToken()
	if tokenNuevo == "" || tokenNuevo == tokenViejo {
		// No se imprimen los valores: ni siquiera un token de prueba debe
		// acabar en el registro.
		t.Fatal("la rotación debe devolver un token nuevo y distinto del anterior")
	}
	if s.APIToken() != tokenNuevo {
		t.Fatal("APIToken() debe reflejar el token recién generado")
	}

	// 3. El cliente conectado se desconecta (cierre 4401 si llega a leer el
	// frame; si el corte llega primero, basta con que la lectura falle).
	if err := conexion.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("no se pudo fijar la fecha límite de lectura: %v", err)
	}
	if _, _, err = conexion.ReadMessage(); err == nil {
		t.Fatal("tras la rotación el WebSocket conectado debe cerrarse")
	} else {
		var cierre *websocket.CloseError
		if errors.As(err, &cierre) {
			t.Logf("cliente desconectado con cierre de aplicación: code=%d text=%q", cierre.Code, cierre.Text)
			if cierre.Code != wsCloseTokenRotated {
				t.Fatalf("el cliente debe recibir el código de aplicación %d, got %d (%s)",
					wsCloseTokenRotated, cierre.Code, cierre.Text)
			}
		}
	}

	// 4. El token viejo ya no abre conexiones nuevas: handshake rechazado
	// con 401 (authMiddleware) y nunca un 101.
	dialerViejo := wsDialer(tokenViejo)
	_, respViejo, errViejo := dialerViejo.Dial(wsURL, nil)
	if respViejo != nil && respViejo.Body != nil {
		// El cuerpo del 401 es minúsculo; se cierra siempre para no dejar
		// la conexión HTTP colgada en la limpieza del cliente.
		_ = respViejo.Body.Close()
	}
	if errViejo == nil {
		t.Fatal("el token viejo no debe poder abrir nuevas conexiones tras la rotación")
	}
	if respViejo == nil || respViejo.StatusCode != http.StatusUnauthorized {
		codigo := 0
		if respViejo != nil {
			codigo = respViejo.StatusCode
		}
		t.Fatalf("la conexión con el token viejo debe morir en 401, got HTTP %d (%v)", codigo, errViejo)
	}

	// 5. El token nuevo sí abre conexión (101): la reconexión con la nueva
	// credencial funciona.
	dialerNuevo := wsDialer(tokenNuevo)
	conexionNueva, respNuevo, errNuevo := dialerNuevo.Dial(wsURL, nil)
	if errNuevo != nil {
		codigo := 0
		if respNuevo != nil {
			codigo = respNuevo.StatusCode
		}
		t.Fatalf("el handshake con el token nuevo debe funcionar (HTTP %d): %v", codigo, errNuevo)
	}
	t.Cleanup(func() {
		// Igual que arriba: el cierre repetido es inofensivo.
		_ = conexionNueva.Close()
	})
	if respNuevo.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("se esperaba un 101 con el token nuevo, got %d", respNuevo.StatusCode)
	}
}
