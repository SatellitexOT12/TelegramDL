package config

import "testing"

// TestEsLoopback fija la tabla de decisiones de EsLoopback (SEC-05): qué
// direcciones dejan el panel únicamente en este equipo y cuáles lo exponen a
// la red. Un fallo aquí significa que el aviso de exposición de server.Start
// se emitiría (o no) sobre un criterio equivocado.
func TestEsLoopback(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		// Loopback: IP literal, nombre estándar, IPv6 y cualquier 127.x.x.x.
		{"127.0.0.1", true},
		{"localhost", true},
		{"::1", true},
		{"127.5.5.5", true},
		{" 127.0.0.1 ", true}, // espacios sobrantes: se recortan
		// No loopback: vacío (todas las interfaces en server.Start),
		// unspecified, IP de red y nombres no reconocidos.
		{"", false},
		{"0.0.0.0", false},
		{"::", false},
		{"192.168.1.40", false},
		{"example.com", false},
	}
	for _, c := range cases {
		if got := EsLoopback(c.host); got != c.want {
			t.Errorf("EsLoopback(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

// TestDefaultBindHostEsLoopback inmoviliza el valor por defecto de bind: si
// alguien vuelve a exponer el panel sin querer (regresión a 0.0.0.0), este
// test rompe antes que el aviso de arranque (SEC-05).
func TestDefaultBindHostEsLoopback(t *testing.T) {
	if DefaultBindHost != "127.0.0.1" {
		t.Errorf("DefaultBindHost = %q, want %q", DefaultBindHost, "127.0.0.1")
	}
	if !EsLoopback(DefaultBindHost) {
		t.Errorf("EsLoopback(%q) = false, want true", DefaultBindHost)
	}
}
