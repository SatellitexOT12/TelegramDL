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
func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b     string
		expected int
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

	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.expected {
			t.Errorf("compareVersions(%q, %q) = %d, expected %d", c.a, c.b, got, c.expected)
		}
	}
}

// assetForThisSystem tiene que quedarse con el archivo de esta plataforma y,
// sobre todo, no confundir el checksums.txt con la aplicación.
func TestAssetForThisSystemIgnoresNonAppFiles(t *testing.T) {
	system := systemKeywords()[0]
	architecture := archKeywords()[0]

	assets := []assetGitHub{
		{Name: "checksums.txt", URL: "https://example.test/checksums.txt", Size: 200},
		{Name: fmt.Sprintf("TelegramDL_%s_%s.zip.sig", system, architecture), URL: "https://example.test/sig", Size: 100},
		{Name: fmt.Sprintf("TelegramDL_%s_%s.zip", system, architecture), URL: "https://example.test/app", Size: 1000},
	}

	chosen := assetForThisSystem(assets)
	if chosen == nil {
		t.Fatalf("no se eligió ningún archivo para %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if chosen.DownloadURL != "https://example.test/app" {
		t.Fatalf("se eligió %q en vez del ejecutable", chosen.Name)
	}
}

// Con varios archivos del mismo sistema gana el de esta arquitectura.
func TestAssetForThisSystemPrefersCorrectArch(t *testing.T) {
	system := systemKeywords()[0]
	architecture := archKeywords()[0]

	otherArchitecture := "arm64"
	if architecture == "arm64" || architecture == "aarch64" {
		otherArchitecture = "amd64"
	}

	assets := []assetGitHub{
		{Name: fmt.Sprintf("TelegramDL_%s_%s.zip", system, otherArchitecture), URL: "https://example.test/otra", Size: 1000},
		{Name: fmt.Sprintf("TelegramDL_%s_%s.zip", system, architecture), URL: "https://example.test/mia", Size: 1000},
	}

	chosen := assetForThisSystem(assets)
	if chosen == nil || chosen.DownloadURL != "https://example.test/mia" {
		t.Fatalf("se esperaba el archivo de %s, se eligió %+v", architecture, chosen)
	}
}

func TestAssetForThisSystemNoCompatibleAsset(t *testing.T) {
	assets := []assetGitHub{
		{Name: "TelegramDL_plan9_mips.tar.gz", URL: "https://example.test/nada", Size: 10},
	}
	if runtime.GOOS == "plan9" {
		t.Skip("el caso de prueba usa plan9 como sistema imposible")
	}
	if chosen := assetForThisSystem(assets); chosen != nil {
		t.Fatalf("no debería haber elegido nada, eligió %q", chosen.Name)
	}
}

// applyBinary es el reemplazo de selfupdate.Apply: tiene que dejar la nueva
// versión en su sitio y no perder la anterior si algo sale mal.
func TestApplyBinarySubstitutesExecutable(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "TelegramDL")
	source := filepath.Join(dir, "descargado")

	if err := os.WriteFile(target, []byte("version vieja"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("version nueva"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := applyBinary(source, target); err != nil {
		t.Fatalf("applyBinary devolvió error: %v", err)
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "version nueva" {
		t.Fatalf("el ejecutable no se sustituyó: %q", content)
	}

	// El binario nuevo tiene que quedar ejecutable aunque el descargado no lo
	// fuera (en Windows el modo no significa nada y la comprobación se salta).
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm()&0o100 == 0 {
			t.Fatalf("el ejecutable quedó sin permiso de ejecución: %v", fi.Mode())
		}
	}

	// No debe quedar ningún temporal suelto en la carpeta.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if len(entry.Name()) > 0 && entry.Name()[0] == '.' {
			t.Fatalf("quedó un archivo temporal sin limpiar: %s", entry.Name())
		}
	}
}

func TestApplyBinaryNoSourceDoesNotTouchTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "TelegramDL")
	if err := os.WriteFile(target, []byte("version vieja"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := applyBinary(filepath.Join(dir, "no-existe"), target); err == nil {
		t.Fatal("se esperaba un error al no encontrar la nueva versión")
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("el ejecutable original desapareció: %v", err)
	}
	if string(content) != "version vieja" {
		t.Fatalf("el ejecutable original cambió: %q", content)
	}
}

func TestChecksumURL(t *testing.T) {
	got, err := checksumURL("https://github.com/infinityxgame/tgdown/releases/download/v2.5.0/TelegramDL_windows_amd64.zip?token=x")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://github.com/infinityxgame/tgdown/releases/download/v2.5.0/checksums.txt"
	if got != want {
		t.Fatalf("checksumURL = %q, se esperaba %q", got, want)
	}
}
