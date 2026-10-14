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
		t.Fatalf("invalid characters remained: %q", got)
	}
}

func TestSanitizeFileNameEmptyFallsBackToDefault(t *testing.T) {
	if got := SanitizeFileName(""); got != "file" {
		t.Fatalf("expected 'file', got %q", got)
	}
	if got := SanitizeFileName("   "); got != "file" {
		t.Fatalf("spaces-only name should fall back to default, got %q", got)
	}
}

func TestSanitizeFileNameTrimsDotsAndSpaces(t *testing.T) {
	got := SanitizeFileName("  video.mp4  ")
	if got != "video.mp4" {
		t.Fatalf("expected 'video.mp4' without spaces, got %q", got)
	}
}

func TestSanitizeFileNameTruncatesLongNamesKeepingExtension(t *testing.T) {
	longStem := strings.Repeat("a", 250)
	got := SanitizeFileName(longStem + ".mp4")
	if len(got) > 200 {
		t.Fatalf("name was not truncated to 200 chars: len=%d", len(got))
	}
	if !strings.HasSuffix(got, ".mp4") {
		t.Fatalf("lost extension when truncating: %q", got)
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
		t.Fatal("expected MediaInfo, got nil")
	}
	if info.FileName != "reporte.pdf" {
		t.Fatalf("incorrect filename: %q", info.FileName)
	}
	if info.Kind != KindFile {
		t.Fatalf("incorrect kind: %q", info.Kind)
	}
	if info.FileSize != 12345 {
		t.Fatalf("incorrect size: %d", info.FileSize)
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
		t.Fatal("expected MediaInfo, got nil")
	}
	if info.Kind != KindVideo {
		t.Fatalf("incorrect kind: %q", info.Kind)
	}
	if info.FileName != "video_99.mp4" {
		t.Fatalf("incorrect generated filename: %q", info.FileName)
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
		t.Fatal("expected MediaInfo, got nil")
	}
	if info.Kind != KindSong {
		t.Fatalf("incorrect kind: %q", info.Kind)
	}
	if info.FileName != "Artista - Cancion.mp3" {
		t.Fatalf("incorrect generated filename: %q", info.FileName)
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
		t.Fatal("expected MediaInfo, got nil")
	}
	if info.FileName != "voice_3.ogg" {
		t.Fatalf("incorrect generated filename: %q", info.FileName)
	}
}

func TestExtractMediaInfoNoMediaReturnsNil(t *testing.T) {
	if info := ExtractMediaInfo(&tg.Message{ID: 1}); info != nil {
		t.Fatalf("message without Media should return nil, got %+v", info)
	}
	if info := ExtractMediaInfo(nil); info != nil {
		t.Fatal("nil message should return nil")
	}
}

func TestExtensionFromMimeKnownAndUnknown(t *testing.T) {
	if ext := extensionFromMime("application/zip"); ext != ".zip" {
		t.Fatalf("incorrect extension for zip: %q", ext)
	}
	if ext := extensionFromMime("application/x-made-up"); ext != "" {
		t.Fatalf("unknown mime should return empty string, got %q", ext)
	}
}

// --- Regresiones de seguridad -------------------------------------------

// Un nombre con una «extensión» de más de 200 caracteres hacía que el truncado
// calculara un índice negativo y reventara el proceso. El nombre lo elige quien
// sube el archivo a Telegram, y la escucha lo procesa sola, así que bastaba un
// mensaje para tumbar la aplicación.
func TestSanitizeFileNameNoPanicOnLongExtension(t *testing.T) {
	entries := []string{
		"x." + strings.Repeat("y", 250),
		"." + strings.Repeat("z", 300),
		strings.Repeat(".", 300),
		strings.Repeat("a", 5000),
		strings.Repeat("😀", 300) + ".mp4",
	}
	for _, entry := range entries {
		got := SanitizeFileName(entry)
		if got == "" {
			t.Errorf("returned an empty name for an entry of %d bytes", len(entry))
		}
		if len(got) > 200 {
			t.Errorf("did not respect limit of 200 bytes: len=%d", len(got))
		}
		if !utf8.ValidString(got) {
			t.Errorf("split a UTF-8 character: %q", got)
		}
	}
}

// «&», «^» y «%» los interpreta cmd.exe. Aunque openInSystem ya no pasa por
// el shell, el nombre no debe llevarlos de todas formas.
func TestSanitizeFileNameRemovesShellChars(t *testing.T) {
	if got := SanitizeFileName("video&calc.mp4"); strings.ContainsAny(got, "&^%") {
		t.Fatalf("character interpreted by cmd.exe survived: %q", got)
	}
}

func TestSanitizeFileNameAvoidsReservedWindowsNames(t *testing.T) {
	for _, entry := range []string{"NUL.mp4", "con.txt", "COM1.dat", "lpt9"} {
		if isReservedWindowsName(SanitizeFileName(entry)) {
			t.Errorf("%q is still a reserved name", entry)
		}
	}
	if got := SanitizeFileName("CONcierto.mp4"); got != "CONcierto.mp4" {
		t.Errorf("a legitimate name was altered: %q", got)
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
		t.Fatalf("expected DCID 4, got: %+v", infoDoc)
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
		t.Fatalf("expected DCID 2 for photo, got: %+v", infoPhoto)
	}
}

func TestDetectFileMigrate(t *testing.T) {
	errTg := &tgerr.Error{
		Code:     303,
		Type:     "FILE_MIGRATE",
		Argument: 4,
	}
	if dc := detectFileMigrate(errTg); dc != 4 {
		t.Fatalf("expected DC 4 from tgerr, got %d", dc)
	}

	errStr := errors.New("get file: get next chunk: invoke pool: rpcDoRequest: rpc error code 303: FILE_MIGRATE (4)")
	if dc := detectFileMigrate(errStr); dc != 4 {
		t.Fatalf("expected DC 4 from formatted error, got %d", dc)
	}

	errUnderscore := errors.New("rpc error: FILE_MIGRATE_3")
	if dc := detectFileMigrate(errUnderscore); dc != 3 {
		t.Fatalf("expected DC 3 from underscore error, got %d", dc)
	}

	errOther := errors.New("connection reset by peer")
	if dc := detectFileMigrate(errOther); dc != 0 {
		t.Fatalf("expected DC 0 for unrelated error, got %d", dc)
	}
}
