package updater

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// La comparación de versiones sustituye a la que traía go-selfupdate, así que
// conviene que quede fijada: de ella depende que la aplicación no se ofrezca a
// sí misma una actualización, ni se salte una real.
func TestCompararVersiones(t *testing.T) {
	casos := []struct {
		a, b     string
		esperado int
	}{
		{"2.4.2", "2.4.2", 0},
		{"v2.4.2", "2.4.2", 0},
		{"V2.4.2", "v2.4.2", 0},
		{"2.4.3", "2.4.2", 1},
		{"2.4.2", "2.4.3", -1},
		{"2.5.0", "2.4.9", 1},
		{"3.0.0", "2.99.99", 1},
		{"2.10.0", "2.9.0", 1}, // comparación numérica, no alfabética
		{"2.4", "2.4.0", 0},
		{"2.4.2", "2.4", 1},
		{"2.5.0-rc1", "2.5.0", -1}, // una versión de prueba es anterior a la final
		{"2.5.0", "2.5.0-rc1", 1},
		{"2.5.0-rc2", "2.5.0-rc1", 1},
		{"2.5.0+build7", "2.5.0", 0}, // los metadatos de compilación no cuentan
		{"2.4.2a", "2.4.2", 0},       // basura al final: se ignora en vez de romper
	}

	for _, caso := range casos {
		if got := compararVersiones(caso.a, caso.b); got != caso.esperado {
			t.Errorf("compararVersiones(%q, %q) = %d, se esperaba %d", caso.a, caso.b, got, caso.esperado)
		}
	}
}

// assetParaEsteSistema tiene que quedarse con el archivo de esta plataforma y,
// sobre todo, no confundir el checksums.txt con la aplicación.
func TestAssetParaEsteSistemaIgnoraLosArchivosQueNoSonLaApp(t *testing.T) {
	sistema := systemKeywords()[0]
	arquitectura := archKeywords()[0]

	assets := []assetGitHub{
		{Name: "checksums.txt", URL: "https://example.test/checksums.txt", Size: 200},
		{Name: fmt.Sprintf("TelegramDL_%s_%s.zip.sig", sistema, arquitectura), URL: "https://example.test/sig", Size: 100},
		{Name: fmt.Sprintf("TelegramDL_%s_%s.zip", sistema, arquitectura), URL: "https://example.test/app", Size: 1000},
	}

	elegido := assetParaEsteSistema(assets)
	if elegido == nil {
		t.Fatalf("no se eligió ningún archivo para %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if elegido.DownloadURL != "https://example.test/app" {
		t.Fatalf("se eligió %q en vez del ejecutable", elegido.Name)
	}
}

// Con varios archivos del mismo sistema gana el de esta arquitectura.
func TestAssetParaEsteSistemaPrefiereLaArquitecturaCorrecta(t *testing.T) {
	sistema := systemKeywords()[0]
	arquitectura := archKeywords()[0]

	otraArquitectura := "arm64"
	if arquitectura == "arm64" || arquitectura == "aarch64" {
		otraArquitectura = "amd64"
	}

	assets := []assetGitHub{
		{Name: fmt.Sprintf("TelegramDL_%s_%s.zip", sistema, otraArquitectura), URL: "https://example.test/otra", Size: 1000},
		{Name: fmt.Sprintf("TelegramDL_%s_%s.zip", sistema, arquitectura), URL: "https://example.test/mia", Size: 1000},
	}

	elegido := assetParaEsteSistema(assets)
	if elegido == nil || elegido.DownloadURL != "https://example.test/mia" {
		t.Fatalf("se esperaba el archivo de %s, se eligió %+v", arquitectura, elegido)
	}
}

func TestAssetParaEsteSistemaSinNadaCompatible(t *testing.T) {
	assets := []assetGitHub{
		{Name: "TelegramDL_plan9_mips.tar.gz", URL: "https://example.test/nada", Size: 10},
	}
	if runtime.GOOS == "plan9" {
		t.Skip("el caso de prueba usa plan9 como sistema imposible")
	}
	if elegido := assetParaEsteSistema(assets); elegido != nil {
		t.Fatalf("no debería haber elegido nada, eligió %q", elegido.Name)
	}
}

// aplicarBinario es el reemplazo de selfupdate.Apply: tiene que dejar la nueva
// versión en su sitio y no perder la anterior si algo sale mal.
func TestAplicarBinarioSustituyeElEjecutable(t *testing.T) {
	dir := t.TempDir()
	destino := filepath.Join(dir, "TelegramDL")
	origen := filepath.Join(dir, "descargado")

	if err := os.WriteFile(destino, []byte("version vieja"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(origen, []byte("version nueva"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := aplicarBinario(origen, destino); err != nil {
		t.Fatalf("aplicarBinario devolvió error: %v", err)
	}

	contenido, err := os.ReadFile(destino)
	if err != nil {
		t.Fatal(err)
	}
	if string(contenido) != "version nueva" {
		t.Fatalf("el ejecutable no se sustituyó: %q", contenido)
	}

	// El binario nuevo tiene que quedar ejecutable aunque el descargado no lo
	// fuera (en Windows el modo no significa nada y la comprobación se salta).
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(destino)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm()&0o100 == 0 {
			t.Fatalf("el ejecutable quedó sin permiso de ejecución: %v", fi.Mode())
		}
	}

	// No debe quedar ningún temporal suelto en la carpeta.
	entradas, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entrada := range entradas {
		if len(entrada.Name()) > 0 && entrada.Name()[0] == '.' {
			t.Fatalf("quedó un archivo temporal sin limpiar: %s", entrada.Name())
		}
	}
}

func TestAplicarBinarioSinOrigenNoTocaElDestino(t *testing.T) {
	dir := t.TempDir()
	destino := filepath.Join(dir, "TelegramDL")
	if err := os.WriteFile(destino, []byte("version vieja"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := aplicarBinario(filepath.Join(dir, "no-existe"), destino); err == nil {
		t.Fatal("se esperaba un error al no encontrar la nueva versión")
	}

	contenido, err := os.ReadFile(destino)
	if err != nil {
		t.Fatalf("el ejecutable original desapareció: %v", err)
	}
	if string(contenido) != "version vieja" {
		t.Fatalf("el ejecutable original cambió: %q", contenido)
	}
}

func TestUrlDelChecksum(t *testing.T) {
	got, err := urlDelChecksum("https://github.com/infinityxgame/tgdown/releases/download/v2.5.0/TelegramDL_windows_amd64.zip?token=x")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://github.com/infinityxgame/tgdown/releases/download/v2.5.0/checksums.txt"
	if got != want {
		t.Fatalf("urlDelChecksum = %q, se esperaba %q", got, want)
	}
}
