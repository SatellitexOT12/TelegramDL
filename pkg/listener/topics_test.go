package listener

import (
	"testing"

	"github.com/gotd/td/tg"

	"tgdown/pkg/config"
)

func newEngineWithChats(chats ...config.ListenerChat) *ListenerEngine {
	cfg := config.DefaultConfig()
	cfg.ListenerChats = chats
	cfg = config.NormalizeConfig(cfg)

	le := &ListenerEngine{
		config:         cfg,
		items:          make(map[string]*ListenerItem),
		chatMap:        make(map[int64][]config.ListenerChat),
		topicNameTries: make(map[string]bool),
	}
	le.updateChatMap(cfg)
	return le
}

func topicMessage(topicID int, viaReply bool) *tg.Message {
	msg := &tg.Message{ID: 1000}
	if topicID <= 0 {
		return msg
	}
	header := &tg.MessageReplyHeader{ForumTopic: true}
	if viaReply {
		// Respuesta dentro del tema: reply_to_top_id apunta al tema.
		header.SetReplyToTopID(topicID)
		header.SetReplyToMsgID(topicID + 5)
	} else {
		// Mensaje escrito directamente en el tema.
		header.SetReplyToMsgID(topicID)
	}
	msg.ReplyTo = header
	return msg
}

func TestMessageTopicIDReadsForumHeader(t *testing.T) {
	if got := messageTopicID(topicMessage(57, false)); got != 57 {
		t.Fatalf("mensaje escrito en el tema: got %d, want 57", got)
	}
	if got := messageTopicID(topicMessage(57, true)); got != 57 {
		t.Fatalf("respuesta dentro del tema: got %d, want 57", got)
	}
	if got := messageTopicID(topicMessage(0, false)); got != 0 {
		t.Fatalf("mensaje sin tema: got %d, want 0", got)
	}
}

func TestMessageTopicIDIgnoresNonForumReplies(t *testing.T) {
	// Una respuesta normal en un grupo sin temas NO debe interpretarse como
	// tema: si no, cualquier respuesta parecería pertenecer a un tema.
	header := &tg.MessageReplyHeader{}
	header.SetReplyToMsgID(42)
	msg := &tg.Message{ID: 1000, ReplyTo: header}

	if got := messageTopicID(msg); got != 0 {
		t.Fatalf("una respuesta normal no es un tema: got %d", got)
	}
}

func TestMatchChatOnlyAcceptsMonitoredTopic(t *testing.T) {
	le := newEngineWithChats(config.ListenerChat{
		ID:        -1002121902112,
		TopicID:   config.TopicPointer(57),
		TopicName: "Películas",
		Name:      "Mi Grupo",
	})

	if _, ok := le.matchChat(-1002121902112, 2121902112, 57); !ok {
		t.Fatal("el tema vigilado debe aceptarse")
	}
	if _, ok := le.matchChat(-1002121902112, 2121902112, 99); ok {
		t.Fatal("un tema distinto del vigilado no debe aceptarse")
	}
	if _, ok := le.matchChat(-1002121902112, 2121902112, 0); ok {
		t.Fatal("el tema General no debe colarse cuando se vigila otro tema")
	}
}

func TestMatchChatEntireGroupAcceptsAnyTopic(t *testing.T) {
	le := newEngineWithChats(config.ListenerChat{ID: -1002121902112, Name: "Mi Grupo"})

	for _, topic := range []int64{0, 1, 57, 99} {
		if _, ok := le.matchChat(-1002121902112, 2121902112, topic); !ok {
			t.Fatalf("vigilando el grupo entero debe entrar el tema %d", topic)
		}
	}
}

func TestMatchChatPrefersSpecificTopicEntry(t *testing.T) {
	le := newEngineWithChats(
		config.ListenerChat{ID: -1001, Name: "Grupo", AutoDownload: false},
		config.ListenerChat{ID: -1001, TopicID: config.TopicPointer(57), TopicName: "Pelis", Name: "Grupo", AutoDownload: true},
	)

	chat, ok := le.matchChat(-1001, 0, 57)
	if !ok {
		t.Fatal("el mensaje del tema 57 debe emparejarse")
	}
	if !chat.AutoDownload || chat.Topic() != 57 {
		t.Fatalf("debe ganar la entrada específica del tema: %+v", chat)
	}

	otro, ok := le.matchChat(-1001, 0, 12)
	if !ok {
		t.Fatal("los demás temas los recoge la entrada del grupo entero")
	}
	if otro.HasTopic() {
		t.Fatalf("se esperaba la entrada del grupo entero: %+v", otro)
	}
}

func TestMatchChatGeneralTopic(t *testing.T) {
	// Telegram no marca los mensajes del tema «General», llegan sin tema (0).
	le := newEngineWithChats(config.ListenerChat{
		ID:      -1001,
		TopicID: config.TopicPointer(config.GeneralTopicID),
		Name:    "Grupo",
	})

	if _, ok := le.matchChat(-1001, 0, 0); !ok {
		t.Fatal("un mensaje sin tema debe contar como tema General")
	}
	if _, ok := le.matchChat(-1001, 0, 57); ok {
		t.Fatal("un mensaje de otro tema no es del General")
	}
}

func TestMatchChatAlternativeIDs(t *testing.T) {
	// El chat puede estar configurado con el ID interno del canal en vez del
	// canónico con prefijo -100.
	le := newEngineWithChats(config.ListenerChat{ID: 2121902112, Name: "Grupo"})

	if _, ok := le.matchChat(-1002121902112, 2121902112, 0); !ok {
		t.Fatal("debe reconocerse el canal aunque esté anotado con el ID interno")
	}
}

func TestMatchChatUnmonitoredChat(t *testing.T) {
	le := newEngineWithChats(config.ListenerChat{ID: -1001, Name: "Grupo"})

	if _, ok := le.matchChat(-1009, 0, 0); ok {
		t.Fatal("un chat que no está en la lista no debe emparejarse")
	}
}
