package logbus

import (
	"strings"
	"testing"
)

// TestEmitNeutralizesLineBreaks comprueba (SEC-28) que un message/detail
// forjado con CR/LF no puede introducir líneas físicas falsas en la salida del
// registro (falsificación de nivel/categoría, p. ej. "\n[ERROR] [FAKE] ...").
func TestEmitNeutralizesLineBreaks(t *testing.T) {
	var out strings.Builder
	b := NewBus(16)
	b.mirror = &out

	e := b.Emit(LevelInfo, CatSystem,
		"arranque ok\r\n[ERROR] [FAKE] inyectado",
		"detalle\r\n[WARN] [FAKE] otro")

	if n := strings.Count(out.String(), "\n"); n != 1 {
		t.Fatalf("la salida debe tener exactamente 1 salto de línea (el terminador), tiene %d: %q", n, out.String())
	}
	if strings.ContainsRune(out.String(), '\r') {
		t.Errorf("la salida contiene CR sin normalizar: %q", out.String())
	}
	if strings.ContainsAny(e.Message, "\r\n") || strings.ContainsAny(e.Detail, "\r\n") {
		t.Errorf("la entrada en memoria aún contiene saltos: message=%q detail=%q", e.Message, e.Detail)
	}
	// El texto no se trunca: solo cambia el salto por su marca literal.
	if !strings.Contains(e.Message, `\n[ERROR] [FAKE] inyectado`) {
		t.Errorf("el texto debe conservarse íntegro con el salto neutralizado: %q", e.Message)
	}
	if !strings.Contains(e.Detail, `\n[WARN] [FAKE] otro`) {
		t.Errorf("el detalle debe conservarse íntegro con el salto neutralizado: %q", e.Detail)
	}
}

// TestSanitizeLogField comprueba que CRLF, LF y CR sueltos se neutralizan sin
// truncar el texto.
func TestSanitizeLogField(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a\r\nb", `a\nb`},
		{"a\nb", `a\nb`},
		{"a\rb", `a\nb`},
		{"sin saltos", "sin saltos"},
	}
	for _, c := range cases {
		if got := sanitizeLogField(c.in); got != c.want {
			t.Errorf("sanitizeLogField(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
