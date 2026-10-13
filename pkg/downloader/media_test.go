package downloader

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

func TestSanitizeFileNameReplacesInvalidChars(t *testing.T) {
	got := SanitizeFileName(`a/b\c:d"e<f>g|h?i*j`)
	if strings.ContainsAny(got, `/\:"<>|?*`) {
		t.Fatalf("quedaron caracteres inválidos: %q", got)
	}
}

func TestSanitizeFileNameEmptyFallsBackToDefault(t *testing.T) {
	if got := SanitizeFileName(""); got != "archivo" {
		t.Fatalf("se esperaba 'archivo', got %q", got)
	}
	if got := SanitizeFileName("   "); got != "archivo" {
		t.Fatalf("un nombre de solo espacios debería caer al valor por defecto, got %q", got)
	}
}

func TestSanitizeFileNameTrimsDotsAndSpaces(t *testing.T) {
	got := SanitizeFileName("  video.mp4  ")
	if got != "video.mp4" {
		t.Fatalf("se esperaba 'video.mp4' sin espacios, got %q", got)
	}
}

func TestSanitizeFileNameTruncatesLongNamesKeepingExtension(t *testing.T) {
	longStem := strings.Repeat("a", 250)
	got := SanitizeFileName(longStem + ".mp4")
	if len(got) > 200 {
		t.Fatalf("el nombre no se truncó a 200 caracteres: len=%d", len(got))
	}
	if !strings.HasSuffix(got, ".mp4") {
		t.Fatalf("se perdió la extensión al truncar: %q", got)
	}
}

func TestExtractMediaInfoDocumentWithFilename(t *testing.T) {
	msg := &tg.Message{
		ID: 42,
		Media: &tg.MessageMediaDocument{
			Document: &tg.Document{
				MimeType: "application/pdf",
				Size:     12345,
				Attributes: []tg.DocumentAttributeClass{
					&tg.DocumentAttributeFilename{FileName: "reporte.pdf"},
				},
			},
		},
	}

	info := ExtractMediaInfo(msg)
	if info == nil {
		t.Fatal("se esperaba MediaInfo, se obtuvo nil")
	}
	if info.FileName != "reporte.pdf" {
		t.Fatalf("nombre de archivo incorrecto: %q", info.FileName)
	}
	if info.Kind != KindFile {
		t.Fatalf("kind incorrecto: %q", info.Kind)
	}
	if info.FileSize != 12345 {
		t.Fatalf("tamaño incorrecto: %d", info.FileSize)
	}
}

func TestExtractMediaInfoVideoWithoutFilenameGeneratesOne(t *testing.T) {
	msg := &tg.Message{
		ID: 99,
		Media: &tg.MessageMediaDocument{
			Document: &tg.Document{
				MimeType: "video/mp4",
				Size:     999,
				Attributes: []tg.DocumentAttributeClass{
					&tg.DocumentAttributeVideo{},
				},
			},
		},
	}

	info := ExtractMediaInfo(msg)
	if info == nil {
		t.Fatal("se esperaba MediaInfo, se obtuvo nil")
	}
	if info.Kind != KindVideo {
		t.Fatalf("kind incorrecto: %q", info.Kind)
	}
	if info.FileName != "video_99.mp4" {
		t.Fatalf("nombre generado incorrecto: %q", info.FileName)
	}
}

func TestExtractMediaInfoAudioWithTitleAndPerformer(t *testing.T) {
	msg := &tg.Message{
		ID: 7,
		Media: &tg.MessageMediaDocument{
			Document: &tg.Document{
				MimeType: "audio/mpeg",
				Size:     321,
				Attributes: []tg.DocumentAttributeClass{
					&tg.DocumentAttributeAudio{Title: "Cancion", Performer: "Artista"},
				},
			},
		},
	}

	info := ExtractMediaInfo(msg)
	if info == nil {
		t.Fatal("se esperaba MediaInfo, se obtuvo nil")
	}
	if info.Kind != KindSong {
		t.Fatalf("kind incorrecto: %q", info.Kind)
	}
	if info.FileName != "Artista - Cancion.mp3" {
		t.Fatalf("nombre generado incorrecto: %q", info.FileName)
	}
}

func TestExtractMediaInfoVoiceMessage(t *testing.T) {
	msg := &tg.Message{
		ID: 3,
		Media: &tg.MessageMediaDocument{
			Document: &tg.Document{
				MimeType: "audio/ogg",
				Size:     50,
				Attributes: []tg.DocumentAttributeClass{
					&tg.DocumentAttributeAudio{Voice: true},
				},
			},
		},
	}

	info := ExtractMediaInfo(msg)
	if info == nil {
		t.Fatal("se esperaba MediaInfo, se obtuvo nil")
	}
	if info.FileName != "voice_3.ogg" {
		t.Fatalf("nombre generado incorrecto: %q", info.FileName)
	}
}

func TestExtractMediaInfoNoMediaReturnsNil(t *testing.T) {
	if info := ExtractMediaInfo(&tg.Message{ID: 1}); info != nil {
		t.Fatalf("un mensaje sin Media debería devolver nil, se obtuvo %+v", info)
	}
	if info := ExtractMediaInfo(nil); info != nil {
		t.Fatal("un mensaje nil debería devolver nil")
	}
}

func TestExtensionFromMimeKnownAndUnknown(t *testing.T) {
	if ext := extensionFromMime("application/zip"); ext != ".zip" {
		t.Fatalf("extensión incorrecta para zip: %q", ext)
	}
	if ext := extensionFromMime("application/x-made-up"); ext != "" {
		t.Fatalf("un mime desconocido debería devolver cadena vacía, got %q", ext)
	}
}

// --- Regresiones de seguridad -------------------------------------------

// Un nombre con una «extensión» de más de 200 caracteres hacía que el truncado
// calculara un índice negativo y reventara el proceso. El nombre lo elige quien
// sube el archivo a Telegram, y la escucha lo procesa sola, así que bastaba un
// mensaje para tumbar la aplicación.
func TestSanitizeFileNameNoPanicOnLongExtension(t *testing.T) {
	entradas := []string{
		"x." + strings.Repeat("y", 250),
		"." + strings.Repeat("z", 300),
		strings.Repeat(".", 300),
		strings.Repeat("a", 5000),
		strings.Repeat("😀", 300) + ".mp4",
	}
	for _, entrada := range entradas {
		got := SanitizeFileName(entrada)
		if got == "" {
			t.Errorf("devolvió un nombre vacío para una entrada de %d bytes", len(entrada))
		}
		if len(got) > 200 {
			t.Errorf("no se respetó el límite de 200 bytes: len=%d", len(got))
		}
		if !utf8.ValidString(got) {
			t.Errorf("se partió un carácter UTF-8: %q", got)
		}
	}
}

// «&», «^» y «%» los interpreta cmd.exe. Aunque openInSystem ya no pasa por
// el shell, el nombre no debe llevarlos de todas formas.
func TestSanitizeFileNameRemovesShellChars(t *testing.T) {
	if got := SanitizeFileName("video&calc.mp4"); strings.ContainsAny(got, "&^%") {
		t.Fatalf("sobrevivió un carácter que interpreta cmd.exe: %q", got)
	}
}

func TestSanitizeFileNameAvoidsReservedWindowsNames(t *testing.T) {
	for _, entrada := range []string{"NUL.mp4", "con.txt", "COM1.dat", "lpt9"} {
		if isReservedWindowsName(SanitizeFileName(entrada)) {
			t.Errorf("%q sigue siendo un nombre reservado", entrada)
		}
	}
	if got := SanitizeFileName("CONcierto.mp4"); got != "CONcierto.mp4" {
		t.Errorf("se alteró un nombre legítimo: %q", got)
	}
}

func TestExtractMediaInfoDCID(t *testing.T) {
	msgDoc := &tg.Message{
		ID: 42,
		Media: &tg.MessageMediaDocument{
			Document: &tg.Document{
				MimeType: "application/zip",
				Size:     5000,
				DCID:     4,
				Attributes: []tg.DocumentAttributeClass{
					&tg.DocumentAttributeFilename{FileName: "game.zip.001"},
				},
			},
		},
	}
	infoDoc := ExtractMediaInfo(msgDoc)
	if infoDoc == nil || infoDoc.DCID != 4 {
		t.Fatalf("se esperaba DCID 4, se obtuvo: %+v", infoDoc)
	}

	msgPhoto := &tg.Message{
		ID: 43,
		Media: &tg.MessageMediaPhoto{
			Photo: &tg.Photo{
				ID:   123,
				DCID: 2,
				Sizes: []tg.PhotoSizeClass{
					&tg.PhotoSize{Type: "y", Size: 100},
				},
			},
		},
	}
	infoPhoto := ExtractMediaInfo(msgPhoto)
	if infoPhoto == nil || infoPhoto.DCID != 2 {
		t.Fatalf("se esperaba DCID 2 para foto, se obtuvo: %+v", infoPhoto)
	}
}

func TestDetectFileMigrate(t *testing.T) {
	errTg := &tgerr.Error{
		Code:     303,
		Type:     "FILE_MIGRATE",
		Argument: 4,
	}
	if dc := detectFileMigrate(errTg); dc != 4 {
		t.Fatalf("se esperaba DC 4 desde tgerr, se obtuvo %d", dc)
	}

	errStr := errors.New("get file: get next chunk: invoke pool: rpcDoRequest: rpc error code 303: FILE_MIGRATE (4)")
	if dc := detectFileMigrate(errStr); dc != 4 {
		t.Fatalf("se esperaba DC 4 desde error formateado, se obtuvo %d", dc)
	}

	errUnderscore := errors.New("rpc error: FILE_MIGRATE_3")
	if dc := detectFileMigrate(errUnderscore); dc != 3 {
		t.Fatalf("se esperaba DC 3 desde error con guión bajo, se obtuvo %d", dc)
	}

	errOther := errors.New("connection reset by peer")
	if dc := detectFileMigrate(errOther); dc != 0 {
		t.Fatalf("se esperaba DC 0 para error no relacionado, se obtuvo %d", dc)
	}
}
