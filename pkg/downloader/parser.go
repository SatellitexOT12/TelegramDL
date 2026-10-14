package downloader

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"tgdown/pkg/i18n"
)

type ParsedURL struct {
	ChatUsername string
	ChatID       int64
	IsChannelID  bool
	// TopicID es el tema del grupo cuando el enlace lo lleva
	// (https://t.me/c/CHAT/TEMA/MENSAJE), o 0 si no aplica. Para descargar no
	// hace falta —el ID de mensaje es único dentro del chat—, pero se conserva
	// para poder mostrarlo y registrarlo.
	TopicID    int
	StartMsgID int
	EndMsgID   int
}

// Los enlaces de un grupo con temas llevan un número extra en medio:
// https://t.me/c/2121902112/57/31449. El grupo (?:(\d+)/)? lo captura de forma
// opcional, de modo que los enlaces de siempre siguen funcionando igual.
var (
	channelRegex  = regexp.MustCompile(`^https://t\.me/c/(\d+)/(?:(\d+)/)?(\d+)(?:-(\d+))?(?:/)?(?:[?#].*)?$`)
	botRegex      = regexp.MustCompile(`^https://t\.me/b/([A-Za-z0-9_]{1,64})/(?:(\d+)/)?(\d+)(?:-(\d+))?(?:/)?(?:[?#].*)?$`)
	usernameRegex = regexp.MustCompile(`^https://t\.me/([A-Za-z0-9_]{1,64})/(?:(\d+)/)?(\d+)(?:-(\d+))?(?:/)?(?:[?#].*)?$`)
)

const MaxMessagesPerJob = 500

// parseMsgRange interpreta los campos de inicio/fin de mensaje extraídos por
// las expresiones regulares de arriba y valida que formen un rango razonable.
// Centraliza una lógica que antes estaba duplicada en las tres ramas de
// ParseURL (canal, bot y username).
func parseMsgRange(startStr, endStr string) (startID, endID int, err error) {
	startID, _ = strconv.Atoi(startStr)
	endID = startID
	if endStr != "" {
		endID, _ = strconv.Atoi(endStr)
	}
	if endID < startID {
		return 0, 0, errors.New(i18n.T("parser.endLessThanStart"))
	}
	if endID-startID+1 > MaxMessagesPerJob {
		return 0, 0, fmt.Errorf(i18n.T("parser.rangeExceedsMax"), MaxMessagesPerJob)
	}
	return startID, endID, nil
}

func submatchOrEmpty(match []string, idx int) string {
	if len(match) > idx {
		return match[idx]
	}
	return ""
}

func ParseURL(url string) (*ParsedURL, error) {
	clean := strings.TrimSpace(url)
	if clean == "" {
		return nil, errors.New(i18n.T("parser.emptyURL"))
	}

	clean = strings.Replace(clean, "http://telegram.me/", "https://t.me/", 1)
	clean = strings.Replace(clean, "https://telegram.me/", "https://t.me/", 1)
	clean = strings.Replace(clean, "telegram.me/", "https://t.me/", 1)

	if strings.HasPrefix(clean, "http://") {
		clean = "https://" + strings.TrimPrefix(clean, "http://")
	} else if !strings.HasPrefix(clean, "https://") && !strings.HasPrefix(clean, "tg://") {
		clean = "https://" + clean
	}

	if match := channelRegex.FindStringSubmatch(clean); match != nil {
		topicID, _ := strconv.Atoi(submatchOrEmpty(match, 2))
		startID, endID, err := parseMsgRange(match[3], submatchOrEmpty(match, 4))
		if err != nil {
			return nil, err
		}

		// En MTProto de Telegram, los IDs de canales privados llevan prefijo -100
		tgChatID, _ := strconv.ParseInt(fmt.Sprintf("-100%s", match[1]), 10, 64)

		return &ParsedURL{
			ChatID:      tgChatID,
			IsChannelID: true,
			TopicID:     topicID,
			StartMsgID:  startID,
			EndMsgID:    endID,
		}, nil
	}

	if match := botRegex.FindStringSubmatch(clean); match != nil {
		username := match[1]
		topicID, _ := strconv.Atoi(submatchOrEmpty(match, 2))
		startID, endID, err := parseMsgRange(match[3], submatchOrEmpty(match, 4))
		if err != nil {
			return nil, err
		}

		return &ParsedURL{
			ChatUsername: username,
			IsChannelID:  false,
			TopicID:      topicID,
			StartMsgID:   startID,
			EndMsgID:     endID,
		}, nil
	}

	if match := usernameRegex.FindStringSubmatch(clean); match != nil {
		username := match[1]
		topicID, _ := strconv.Atoi(submatchOrEmpty(match, 2))
		startID, endID, err := parseMsgRange(match[3], submatchOrEmpty(match, 4))
		if err != nil {
			return nil, err
		}

		return &ParsedURL{
			ChatUsername: username,
			IsChannelID:  false,
			TopicID:      topicID,
			StartMsgID:   startID,
			EndMsgID:     endID,
		}, nil
	}

	return nil, errors.New(i18n.T("parser.invalidURLFormat"))
}
