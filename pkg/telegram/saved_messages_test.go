package telegram

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestCodeEntitiesUsaDesplazamientosUTF16(t *testing.T) {
	// El emoji ocupa dos unidades UTF-16 pero cuatro bytes y una sola runa: si
	// se midiera en bytes o en runas, Telegram resaltaría el tramo equivocado.
	const token = "abc123"
	text := "🔐 TelegramDL · token de acceso remoto\n\n" + token + "\n\nfin"

	entities := codeEntities(text, []string{token})
	if len(entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(entities))
	}

	code, ok := entities[0].(*tg.MessageEntityCode)
	if !ok {
		t.Fatalf("unexpected entity type: %T", entities[0])
	}
	if code.Offset != 40 {
		t.Fatalf("incorrect offset: got %d, want 40", code.Offset)
	}
	if code.Length != len(token) {
		t.Fatalf("incorrect length: got %d, want %d", code.Length, len(token))
	}
}

func TestCodeEntitiesIgnoraFragmentosAusentes(t *testing.T) {
	entities := codeEntities("hola mundo", []string{"", "no está", "mundo"})
	if len(entities) != 1 {
		t.Fatalf("only present fragment should generate entity: %+v", entities)
	}

	code := entities[0].(*tg.MessageEntityCode)
	if code.Offset != 5 || code.Length != 5 {
		t.Fatalf("incorrect entity: offset=%d length=%d", code.Offset, code.Length)
	}
}

func TestMessageRandomIDDoesNotRepeat(t *testing.T) {
	vistos := make(map[int64]bool, 64)
	for i := 0; i < 64; i++ {
		id, err := messageRandomID()
		if err != nil {
			t.Fatalf("error generating identifier: %v", err)
		}
		if vistos[id] {
			t.Fatalf("repeated identifier: %d", id)
		}
		vistos[id] = true
	}
}

func TestSendToSavedMessagesFailsWithoutClient(t *testing.T) {
	// Se construye a mano en vez de con NewClientManager para no tocar
	// ~/.tgdown ni la sesión real del usuario durante los tests.
	cm := &ClientManager{}

	if err := cm.SendToSavedMessages(t.Context(), "hola"); err == nil {
		t.Fatal("without configured client should return error")
	}
	if err := cm.SendToSavedMessages(t.Context(), "   "); err == nil {
		t.Fatal("empty message should be rejected")
	}
}
