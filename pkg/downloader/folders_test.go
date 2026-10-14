package downloader

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestChatFolderNameNormalTitles(t *testing.T) {
	cases := []struct {
		title    string
		expected string
	}{
		{"Canal de Pruebas", "Canal de Pruebas"},
		{"  Películas   HD  ", "Películas HD"},
		{"Новости дня", "Новости дня"},
		{"中文频道", "中文频道"},
		{"Ελληνικά", "Ελληνικά"},
		{"قناة الأفلام", "قناة الأفلام"},
		{"Serie 2024 (temporada 3)", "Serie 2024 (temporada 3)"},
	}

	for _, tc := range cases {
		if got := ChatFolderName(tc.title, -1001234567890); got != tc.expected {
			t.Errorf("ChatFolderName(%q) = %q, want %q", tc.title, got, tc.expected)
		}
	}
}

func TestChatFolderNameRemovesDecoration(t *testing.T) {
	cases := []struct {
		title    string
		expected string
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

	for _, tc := range cases {
		if got := ChatFolderName(tc.title, -1001234567890); got != tc.expected {
			t.Errorf("ChatFolderName(%q) = %q, want %q", tc.title, got, tc.expected)
		}
	}
}

func TestChatFolderNameFallsBackToID(t *testing.T) {
	const id = -1001234567890
	cases := []string{
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

	for _, title := range cases {
		got := ChatFolderName(title, id)
		if got != "chat_1001234567890" {
			t.Errorf("ChatFolderName(%q) = %q, expected fallback to ID", title, got)
		}
	}
}

func TestChatFolderNameRemovesInvisibleMarks(t *testing.T) {
	// U+202E le da la vuelta al texto en el Explorador: es el truco clásico
	// para disfrazar un ejecutable. Tiene que desaparecer.
	got := ChatFolderName("Fotos\u202egpj.exe", 123)
	if strings.ContainsRune(got, '\u202e') {
		t.Fatalf("RLO mark survived in %q", got)
	}
	if strings.ContainsRune(ChatFolderName("Ca\u200bnal Serio", 123), '\u200b') {
		t.Error("zero-width space survived")
	}
}

func TestChatFolderNameLimitsZalgo(t *testing.T) {
	title := "C" + strings.Repeat("\u0301", 40) + "anal"
	got := ChatFolderName(title, 123)
	tildes := strings.Count(got, "\u0301")
	if tildes > 1 {
		t.Errorf("got %d combining marks in %q, expected at most 1", tildes, got)
	}
	if !strings.Contains(got, "anal") {
		t.Errorf("base text was lost: %q", got)
	}
}

func TestChatFolderNameRespectsDiskLimits(t *testing.T) {
	longName := strings.Repeat("Canal muy largo ", 20)
	got := ChatFolderName(longName, 123)
	if n := len([]rune(got)); n > maxFolderRunes {
		t.Errorf("name has %d runes, limit is %d", n, maxFolderRunes)
	}

	// Caracteres que rompen una ruta y nombres reservados de Windows.
	if got := ChatFolderName(`Canal/Serie\Temporada`, 123); strings.ContainsAny(got, `/\`) {
		t.Errorf("path separators left in %q", got)
	}
	if got := ChatFolderName("NUL", 123); got == "NUL" {
		t.Error("NUL was kept as is, which is reserved in Windows")
	}
	if got := ChatFolderName("Canal Oficial.", 123); strings.HasSuffix(got, ".") {
		t.Errorf("%q ends with dot, not supported by Windows", got)
	}
}

func TestTopicFolderName(t *testing.T) {
	if got := TopicFolderName("Capítulos nuevos", 42); got != "Capítulos nuevos" {
		t.Errorf("= %q", got)
	}
	if got := TopicFolderName("", 42); got != "tema_42" {
		t.Errorf("= %q, want tema_42", got)
	}
	if got := TopicFolderName("🔥", 7); got != "tema_7" {
		t.Errorf("= %q, want tema_7", got)
	}
}

func TestFolderNameWithID(t *testing.T) {
	got := FolderNameWithID("Noticias", -1009876543210)
	if got != "Noticias (1009876543210)" {
		t.Errorf("= %q", got)
	}

	// Un nombre ya en el límite tiene que dejar sitio al ID: es lo único que
	// distingue una carpeta de la otra.
	longName := strings.Repeat("a", 80)
	got = FolderNameWithID(longName, 55)
	if n := len([]rune(got)); n > maxFolderRunes {
		t.Errorf("name with ID has %d runes, limit is %d", n, maxFolderRunes)
	}
	if !strings.Contains(got, "(55)") {
		t.Errorf("ID was excluded from %q", got)
	}
}

func TestSafeRelativePath(t *testing.T) {
	cases := []struct {
		input    string
		expected string
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

	for _, tc := range cases {
		if got := SafeRelativePath(tc.input); got != tc.expected {
			t.Errorf("SafeRelativePath(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}

	// Sea cual sea la entrada, el resultado nunca puede ser absoluto ni
	// contener un salto hacia arriba.
	for _, input := range []string{"../x", "/x", `C:\Windows\System32`, "..\\..\\x", "x/../../y"} {
		got := SafeRelativePath(input)
		if filepath.IsAbs(got) {
			t.Errorf("SafeRelativePath(%q) returned an absolute path: %q", input, got)
		}
		for _, part := range strings.Split(filepath.ToSlash(got), "/") {
			if part == ".." {
				t.Errorf("SafeRelativePath(%q) passed a «..»: %q", input, got)
			}
		}
	}
}

func TestChatFolderText(t *testing.T) {
	text := ChatFolderText("🔥🔥🔥", -1001234567890, "Capítulos")
	if !strings.Contains(text, "-1001234567890") {
		t.Error("ID missing from folder text file")
	}
	if !strings.Contains(text, "Capítulos") {
		t.Error("topic missing from folder text file")
	}
}
