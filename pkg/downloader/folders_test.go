package downloader

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestChatFolderNameNormalTitles(t *testing.T) {
	casos := []struct {
		titulo   string
		esperado string
	}{
		{"Canal de Pruebas", "Canal de Pruebas"},
		{"  Películas   HD  ", "Películas HD"},
		{"Новости дня", "Новости дня"},
		{"中文频道", "中文频道"},
		{"Ελληνικά", "Ελληνικά"},
		{"قناة الأفلام", "قناة الأفلام"},
		{"Serie 2024 (temporada 3)", "Serie 2024 (temporada 3)"},
	}

	for _, caso := range casos {
		if got := ChatFolderName(caso.titulo, -1001234567890); got != caso.esperado {
			t.Errorf("ChatFolderName(%q) = %q, se esperaba %q", caso.titulo, got, caso.esperado)
		}
	}
}

func TestChatFolderNameRemovesDecoration(t *testing.T) {
	casos := []struct {
		titulo   string
		esperado string
	}{
		{"🔥 Películas HD 🔥", "Películas HD"},
		{"✅ Canal Oficial ✅", "Canal Oficial"},
		{"『 Anime 』", "『 Anime 』"}, // los corchetes CJK son legales en disco: se dejan
		{"Canal ⚡ Rápido", "Canal Rápido"},
		// Letras decorativas: se traducen en vez de tirarse.
		{"𝓒𝓪𝓷𝓪𝓵 𝓟𝓻𝓲𝓿𝓪𝓭𝓸", "Canal Privado"},
		{"𝗠𝗲𝗴𝗮 𝟰𝗞", "Mega 4K"},
		{"𝕯𝖆𝖙𝖔𝖘", "Datos"},
	}

	for _, caso := range casos {
		if got := ChatFolderName(caso.titulo, -1001234567890); got != caso.esperado {
			t.Errorf("ChatFolderName(%q) = %q, se esperaba %q", caso.titulo, got, caso.esperado)
		}
	}
}

func TestChatFolderNameFallsBackToID(t *testing.T) {
	const id = -1001234567890
	casos := []string{
		"🔥🔥🔥",
		"✅",
		"",
		"    ",
		"...",
		"😀😀 😀😀",
		"→←↑↓",
		"▓▒░",
		"A", // una sola letra no identifica nada
	}

	for _, titulo := range casos {
		got := ChatFolderName(titulo, id)
		if got != "chat_1001234567890" {
			t.Errorf("ChatFolderName(%q) = %q, se esperaba el respaldo por ID", titulo, got)
		}
	}
}

func TestChatFolderNameRemovesInvisibleMarks(t *testing.T) {
	// U+202E le da la vuelta al texto en el Explorador: es el truco clásico
	// para disfrazar un ejecutable. Tiene que desaparecer.
	got := ChatFolderName("Fotos\u202egpj.exe", 123)
	if strings.ContainsRune(got, '\u202e') {
		t.Fatalf("la marca RLO sobrevivió en %q", got)
	}
	if strings.ContainsRune(ChatFolderName("Ca\u200bnal Serio", 123), '\u200b') {
		t.Error("el espacio de ancho cero sobrevivió")
	}
}

func TestChatFolderNameLimitsZalgo(t *testing.T) {
	titulo := "C" + strings.Repeat("\u0301", 40) + "anal"
	got := ChatFolderName(titulo, 123)
	tildes := strings.Count(got, "\u0301")
	if tildes > 1 {
		t.Errorf("quedaron %d marcas combinantes en %q, se esperaba una como mucho", tildes, got)
	}
	if !strings.Contains(got, "anal") {
		t.Errorf("se perdió el texto base: %q", got)
	}
}

func TestChatFolderNameRespectsDiskLimits(t *testing.T) {
	largo := strings.Repeat("Canal muy largo ", 20)
	got := ChatFolderName(largo, 123)
	if n := len([]rune(got)); n > maxCarpetaRunas {
		t.Errorf("el nombre tiene %d runas, el tope es %d", n, maxCarpetaRunas)
	}

	// Caracteres que rompen una ruta y nombres reservados de Windows.
	if got := ChatFolderName(`Canal/Serie\Temporada`, 123); strings.ContainsAny(got, `/\`) {
		t.Errorf("quedaron separadores de ruta en %q", got)
	}
	if got := ChatFolderName("NUL", 123); got == "NUL" {
		t.Error("NUL se dejó tal cual, y en Windows es el dispositivo nulo")
	}
	if got := ChatFolderName("Canal Oficial.", 123); strings.HasSuffix(got, ".") {
		t.Errorf("%q termina en punto, Windows no lo admite", got)
	}
}

func TestTopicFolderName(t *testing.T) {
	if got := TopicFolderName("Capítulos nuevos", 42); got != "Capítulos nuevos" {
		t.Errorf("= %q", got)
	}
	if got := TopicFolderName("", 42); got != "tema_42" {
		t.Errorf("= %q, se esperaba tema_42", got)
	}
	if got := TopicFolderName("🔥", 7); got != "tema_7" {
		t.Errorf("= %q, se esperaba tema_7", got)
	}
}

func TestFolderNameWithID(t *testing.T) {
	got := FolderNameWithID("Noticias", -1009876543210)
	if got != "Noticias (1009876543210)" {
		t.Errorf("= %q", got)
	}

	// Un nombre ya en el límite tiene que dejar sitio al ID: es lo único que
	// distingue una carpeta de la otra.
	largo := strings.Repeat("a", 80)
	got = FolderNameWithID(largo, 55)
	if n := len([]rune(got)); n > maxCarpetaRunas {
		t.Errorf("el nombre con ID tiene %d runas, el tope es %d", n, maxCarpetaRunas)
	}
	if !strings.Contains(got, "(55)") {
		t.Errorf("el ID se quedó fuera de %q", got)
	}
}

func TestSafeRelativePath(t *testing.T) {
	casos := []struct {
		entrada  string
		esperado string
	}{
		{"", ""},
		{"   ", ""},
		{"Canal", "Canal"},
		{"Canal/Tema 5", filepath.Join("Canal", "Tema 5")},
		{`Canal\Tema 5`, filepath.Join("Canal", "Tema 5")},
		{"Canal//Tema", filepath.Join("Canal", "Tema")},
		{"./Canal", "Canal"},
		// Nada de salirse de la carpeta de descargas.
		{"../../etc", ""},
		{"..", ""},
		{"Canal/../../..", ""},
		{"/etc/passwd", ""},
		{`C:\Windows\System32`, ""},
		// Como mucho dos niveles: grupo y tema.
		{"a/b/c/d", filepath.Join("a", "b")},
	}

	for _, caso := range casos {
		if got := SafeRelativePath(caso.entrada); got != caso.esperado {
			t.Errorf("SafeRelativePath(%q) = %q, se esperaba %q", caso.entrada, got, caso.esperado)
		}
	}

	// Sea cual sea la entrada, el resultado nunca puede ser absoluto ni
	// contener un salto hacia arriba.
	for _, entrada := range []string{"../x", "/x", `C:\Windows\System32`, "..\\..\\x", "x/../../y"} {
		got := SafeRelativePath(entrada)
		if filepath.IsAbs(got) {
			t.Errorf("SafeRelativePath(%q) devolvió una ruta absoluta: %q", entrada, got)
		}
		for _, parte := range strings.Split(filepath.ToSlash(got), "/") {
			if parte == ".." {
				t.Errorf("SafeRelativePath(%q) dejó pasar un «..»: %q", entrada, got)
			}
		}
	}
}

func TestChatFolderText(t *testing.T) {
	texto := ChatFolderText("🔥🔥🔥", -1001234567890, "Capítulos")
	if !strings.Contains(texto, "-1001234567890") {
		t.Error("falta el ID en el archivito de la carpeta")
	}
	if !strings.Contains(texto, "Capítulos") {
		t.Error("falta el tema en el archivito de la carpeta")
	}
}
