package notifier

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// roundTripFunc permite sustituir el transporte del http.Client del notifier
// para forzar un fallo de red sin salir de la máquina.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestRedactErrorHidesTokenInURLError cubre SEC-10 a nivel de helper: un
// *url.Error simulado con el token en la URL no puede aparecer en el texto
// final renderizado, aunque el error original siga disponible vía Unwrap.
func TestRedactErrorHidesTokenInURLError(t *testing.T) {
	const token = "1234567890:AAFakeTokenForTests_abcdefghijklmnop"
	inner := &url.Error{
		Op:  "Post",
		URL: "https://api.telegram.org/bot" + token + "/sendMessage",
		Err: errors.New("dial tcp: lookup api.telegram.org: no such host"),
	}
	// Mismo envoltorio que aplican sendSync y DetectChat al salir.
	err := RedactError(fmt.Errorf("HTTP request failed: %w", inner), token)

	if err == nil {
		t.Fatal("se esperaba un error")
	}
	if strings.Contains(err.Error(), token) {
		t.Errorf("el token aparece en el texto del error: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "***") {
		t.Errorf("falta la marca de redacción en el error: %q", err.Error())
	}

	// El unwrap se conserva: errores.Is/As siguen viendo el *url.Error original.
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		t.Errorf("errors.As debía encontrar el *url.Error original, got %v", err)
	}
}

// TestSendSyncRedactsTokenFromNetworkError verifica el camino real: con el
// transporte caído (sin conexión, timeout, DNS), sendSync debe devolver un
// error cuyo mensaje no contenga el token, porque ese texto es el que llega a
// logbus vía SendMessage.
func TestSendSyncRedactsTokenFromNetworkError(t *testing.T) {
	const token = "1234567890:AAFakeTokenForTests_abcdefghijklmnop"
	n := NewNotifier(token, 42, nil)
	n.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial tcp: connect: connection refused")
	})}

	err := n.sendSync(context.Background(), "test message")
	if err == nil {
		t.Fatal("sendSync debía fallar con el transporte caído")
	}
	if strings.Contains(err.Error(), token) {
		t.Errorf("el token aparece en el texto del error: %q", err.Error())
	}
}
