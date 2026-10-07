package server

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestServidorHTTPAplicaLimites comprueba los campos de límite de red del
// http.Server (SEC-02) sin abrir puertos: por eso la construcción está
// extraída en nuevoHTTPServer, para poder mirarla directamente desde aquí.
func TestServidorHTTPAplicaLimites(t *testing.T) {
	srv := nuevoHTTPServer(http.NewServeMux())

	if srv.MaxHeaderBytes != 16<<10 {
		t.Errorf("MaxHeaderBytes = %d, se esperaba %d (16 KiB)", srv.MaxHeaderBytes, 16<<10)
	}
	if srv.ReadTimeout <= 0 {
		t.Error("ReadTimeout debe ser mayor que cero: sin él el slowloris mantiene la goroutine ocupada para siempre")
	}
	if srv.WriteTimeout != 0 {
		// Lo contrario rompería el WebSocket y las respuestas amplias; la
		// explicación completa vive en el comentario de nuevoHTTPServer.
		t.Error("WriteTimeout no debe estar fijado: mataría el WebSocket")
	}
	if srv.ReadHeaderTimeout <= 0 {
		t.Error("ReadHeaderTimeout debe seguir activo")
	}
	if srv.IdleTimeout <= 0 {
		t.Error("IdleTimeout debe seguir activo")
	}
}

// TestLimiteDeClientesWebSocket cubre el techo de paneles conectados (SEC-16):
// con el mapa lleno websocketLimiteSuperado debe devolver true y handleWebSocket
// responderá un 503 antes del handshake; con una entrada menos, false.
// Hoy el techo es maxClientesWS = 32.
func TestLimiteDeClientesWebSocket(t *testing.T) {
	s := &Server{wsClients: make(map[*wsClient]bool)}

	// Una entrada menos que el techo: todavía hay hueco.
	for i := 0; i < maxClientesWS-1; i++ {
		s.wsClients[&wsClient{}] = true
	}
	if s.websocketLimiteSuperado() {
		t.Errorf("con %d clientes no debe estar lleno", maxClientesWS-1)
	}

	// Con el techo exacto ya no entra nadie más.
	s.wsClients[&wsClient{}] = true
	if !s.websocketLimiteSuperado() {
		t.Errorf("con %d clientes el techo debe estar lleno", maxClientesWS)
	}
}

// TestLimiteDeCuerpoDeAPI cubre el techo de 1 MiB por petición (SEC-02) en dos
// niveles:
//
//	(a) el helper s.limiteDeCuerpo corta la lectura del cuerpo en
//	    maxCuerpoAPI, y
//	(b) el camino real hasta handleIPCDownload nunca procesa un cuerpo
//	    claramente mayor que 1 MiB.
func TestLimiteDeCuerpoDeAPI(t *testing.T) {
	s := nuevoServidorDePrueba()

	// (a) Lectura directa: con 3 MiB delante, el handler no puede llevárselos
	// todos. MaxBytesHandler devuelve un *http.MaxBytesError al superar el
	// techo, en lugar de entregar el cuerpo completo.
	var errLectura error
	var leidos int
	lector := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		datos, err := io.ReadAll(r.Body)
		leidos = len(datos)
		errLectura = err
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/api/encuesta",
		strings.NewReader(strings.Repeat("a", 3<<20)))
	w := httptest.NewRecorder()
	s.limiteDeCuerpo(lector).ServeHTTP(w, req)

	if errLectura == nil {
		t.Fatal("el handler no debía poder leer el cuerpo completo: falta el techo de 1 MiB")
	}
	var demasiadoGrande *http.MaxBytesError
	if !errors.As(errLectura, &demasiadoGrande) {
		t.Errorf("se esperaba un error de tipo MaxBytesError, got %v", errLectura)
	}
	if leidos > maxCuerpoAPI {
		t.Errorf("se leyeron %d bytes, el techo es %d", leidos, maxCuerpoAPI)
	}

	// (b) Camino completo: loopback, cabecera propia y Content-Type correctos,
	// pero con un JSON válido de más de 2 MiB. Sin el techo, el decoder
	// terminaría de leer el objeto, vería la URL y EnqueueURL la rechazaría
	// con 400 (el control de abajo). Con el techo, MaxBytesHandler corta la
	// lectura a mitad y el decoder falla antes de completar el JSON, de modo
	// que handleIPCDownload responde su error habitual, el 422 de «URL
	// requerida», sin haber procesado nada. Lo que se comprueba es que una
	// petición con cuerpo gigante nunca se procesa (jamás responde 200).
	gigante := `{"url":"https://example.com/esto-no-es-telegram","pad":"` +
		strings.Repeat("a", 2<<20) + `"}`
	reqG := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8000/api/ipc/download",
		strings.NewReader(gigante))
	reqG.Header.Set("Content-Type", "application/json")
	reqG.Header.Set(ownRequestHeader, "1")
	reqG.RemoteAddr = "127.0.0.1:51010"
	wG := httptest.NewRecorder()
	s.Handler().ServeHTTP(wG, reqG)

	if wG.Code == http.StatusOK {
		t.Fatal("un cuerpo de más de 2 MiB no debe procesarse: la respuesta no puede ser 200")
	}
	if wG.Code != http.StatusUnprocessableEntity {
		t.Errorf("se esperaba el 422 habitual del handler al no poder leer el cuerpo, got %d", wG.Code)
	}

	// Control: el mismo cuerpo en tamaño pequeño sí llega hasta el final del
	// decode y hasta EnqueueURL, que descarta la URL con 400. Este 400 frente
	// al 422 de arriba es lo que demuestra que la única diferencia la marca el
	// techo de cuerpo.
	pequeno := `{"url":"https://example.com/esto-no-es-telegram"}`
	reqP := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8000/api/ipc/download",
		strings.NewReader(pequeno))
	reqP.Header.Set("Content-Type", "application/json")
	reqP.Header.Set(ownRequestHeader, "1")
	reqP.RemoteAddr = "127.0.0.1:51011"
	wP := httptest.NewRecorder()
	s.Handler().ServeHTTP(wP, reqP)

	if wP.Code != http.StatusBadRequest {
		t.Errorf("control: un cuerpo pequeño debe llegar a EnqueueURL (400), got %d", wP.Code)
	}
}
