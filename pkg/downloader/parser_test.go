package downloader

import "testing"

func TestParseURLChannelSingleMessage(t *testing.T) {
	res, err := ParseURL("https://t.me/c/2121902112/31449")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsChannelID {
		t.Fatal("expected IsChannelID = true")
	}
	if res.ChatID != -1002121902112 {
		t.Fatalf("incorrect ChatID: got %d, want -1002121902112", res.ChatID)
	}
	if res.StartMsgID != 31449 || res.EndMsgID != 31449 {
		t.Fatalf("incorrect range: start=%d end=%d", res.StartMsgID, res.EndMsgID)
	}
}

func TestParseURLChannelRange(t *testing.T) {
	res, err := ParseURL("https://t.me/c/2121902112/31449-31455")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.StartMsgID != 31449 || res.EndMsgID != 31455 {
		t.Fatalf("incorrect range: start=%d end=%d", res.StartMsgID, res.EndMsgID)
	}
}

func TestParseURLRangeInvertedFails(t *testing.T) {
	_, err := ParseURL("https://t.me/c/2121902112/31455-31449")
	if err == nil {
		t.Fatal("expected error when end message is smaller than start message")
	}
}

func TestParseURLRangeTooLargeFails(t *testing.T) {
	url := "https://t.me/c/2121902112/1-600"
	_, err := ParseURL(url)
	if err == nil {
		t.Fatal("expected error when range exceeds MaxMessagesPerJob")
	}
}

func TestParseURLRangeAtLimitSucceeds(t *testing.T) {
	// 500 mensajes exactos (1..500) debe ser válido, el límite es inclusivo.
	_, err := ParseURL("https://t.me/c/2121902112/1-500")
	if err != nil {
		t.Fatalf("a range of exactly %d messages should not fail: %v", MaxMessagesPerJob, err)
	}
}

func TestParseURLBotFormat(t *testing.T) {
	res, err := ParseURL("https://t.me/b/mybot/50")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ChatUsername != "mybot" {
		t.Fatalf("incorrect username: %q", res.ChatUsername)
	}
	if res.IsChannelID {
		t.Fatal("/b/ format should not be marked as IsChannelID")
	}
	if res.StartMsgID != 50 || res.EndMsgID != 50 {
		t.Fatalf("incorrect range: start=%d end=%d", res.StartMsgID, res.EndMsgID)
	}
}

func TestParseURLUsernameFormat(t *testing.T) {
	res, err := ParseURL("https://t.me/canal/100")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ChatUsername != "canal" {
		t.Fatalf("incorrect username: %q", res.ChatUsername)
	}
	if res.StartMsgID != 100 {
		t.Fatalf("incorrect StartMsgID: %d", res.StartMsgID)
	}
}

func TestParseURLNormalizesTelegramMeDomain(t *testing.T) {
	res, err := ParseURL("telegram.me/canal/7")
	if err != nil {
		t.Fatalf("telegram.me should normalize to t.me: %v", err)
	}
	if res.ChatUsername != "canal" || res.StartMsgID != 7 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestParseURLAddsSchemeWhenMissing(t *testing.T) {
	res, err := ParseURL("t.me/canal/9")
	if err != nil {
		t.Fatalf("URL without scheme should be accepted prepending https://: %v", err)
	}
	if res.ChatUsername != "canal" || res.StartMsgID != 9 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestParseURLEmptyFails(t *testing.T) {
	if _, err := ParseURL(""); err == nil {
		t.Fatal("empty URL should return error")
	}
	if _, err := ParseURL("   "); err == nil {
		t.Fatal("spaces-only URL should return error")
	}
}

func TestParseURLGarbageFails(t *testing.T) {
	if _, err := ParseURL("https://example.com/not-telegram"); err == nil {
		t.Fatal("non-Telegram URL should return error")
	}
}

func TestParseURLChannelWithTopic(t *testing.T) {
	res, err := ParseURL("https://t.me/c/2121902112/57/31449")
	if err != nil {
		t.Fatalf("topic group link should be accepted: %v", err)
	}
	if res.ChatID != -1002121902112 {
		t.Fatalf("incorrect ChatID: got %d, want -1002121902112", res.ChatID)
	}
	if res.TopicID != 57 {
		t.Fatalf("incorrect TopicID: got %d, want 57", res.TopicID)
	}
	// El último número siempre es el mensaje: es lo único que hace falta para
	// descargar, porque el ID de mensaje es único dentro del chat.
	if res.StartMsgID != 31449 || res.EndMsgID != 31449 {
		t.Fatalf("incorrect range: start=%d end=%d", res.StartMsgID, res.EndMsgID)
	}
}

func TestParseURLChannelWithTopicRange(t *testing.T) {
	res, err := ParseURL("https://t.me/c/2121902112/57/31449-31455")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TopicID != 57 || res.StartMsgID != 31449 || res.EndMsgID != 31455 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestParseURLChannelWithTopicTrailingSlash(t *testing.T) {
	res, err := ParseURL("https://t.me/c/2121902112/57/31449/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TopicID != 57 || res.StartMsgID != 31449 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestParseURLChannelWithTopicAndQuery(t *testing.T) {
	res, err := ParseURL("https://t.me/c/2121902112/57/31449?single")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TopicID != 57 || res.StartMsgID != 31449 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestParseURLWithoutTopicKeepsTopicZero(t *testing.T) {
	res, err := ParseURL("https://t.me/c/2121902112/31449")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TopicID != 0 {
		t.Fatalf("link without topic should not invent one: got %d", res.TopicID)
	}
}

func TestParseURLUsernameWithTopic(t *testing.T) {
	res, err := ParseURL("https://t.me/migrupo/12/345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ChatUsername != "migrupo" {
		t.Fatalf("incorrect username: %q", res.ChatUsername)
	}
	if res.TopicID != 12 || res.StartMsgID != 345 || res.EndMsgID != 345 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestParseURLTopicRangeTooLargeStillFails(t *testing.T) {
	if _, err := ParseURL("https://t.me/c/2121902112/57/1-600"); err == nil {
		t.Fatal("message limit should still apply with topics")
	}
}
