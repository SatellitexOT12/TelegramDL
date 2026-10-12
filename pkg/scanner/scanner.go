package scanner

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gotd/td/tg"

	"tgdown/pkg/config"
	"tgdown/pkg/downloader"
	"tgdown/pkg/storage"
	"tgdown/pkg/telegram"
)

type ScanRequest struct {
	Target    string `json:"target"`
	StartDate string `json:"start_date"` // YYYY-MM-DD
	EndDate   string `json:"end_date"`   // YYYY-MM-DD
	FPhotos   bool   `json:"f_photos"`
	FVideos   bool   `json:"f_videos"`
	FAudios   bool   `json:"f_audios"`
	FDocs     bool   `json:"f_docs"`
	FStickers bool   `json:"f_stickers"`
	MinSize   int64  `json:"min_size"` // Bytes (0 = sin límite)
	MaxSize   int64  `json:"max_size"` // Bytes (0 = sin límite)
	MaxScan   int    `json:"max_scan"` // Máximo de mensajes a inspeccionar
}

type ScanItem struct {
	ID               string `json:"id"`
	ChatID           int64  `json:"chat_id"`
	ChatName         string `json:"chat_name"`
	MessageID        int64  `json:"message_id"`
	FileName         string `json:"file_name"`
	CaptionFileName  string `json:"caption_file_name,omitempty"`
	OriginalFileName string `json:"original_file_name,omitempty"`
	Kind             string `json:"kind"`
	FileSize         int64  `json:"file_size"`
	FormattedSize    string `json:"formatted_size"`
	Date             int64  `json:"date"`
	DateStr          string `json:"date_str"`
}

type ScanResponse struct {
	Status       string     `json:"status"`
	ScannedCount int        `json:"scanned_count"`
	MatchCount   int        `json:"match_count"`
	Items        []ScanItem `json:"items"`
	ReachedLimit bool       `json:"reached_limit"`
	Message      string     `json:"message,omitempty"`
}

type ResolvedTargetInfo struct {
	ChatID   int64  `json:"chat_id"`
	ChatName string `json:"chat_name"`
	Type     string `json:"type"`
	Username string `json:"username,omitempty"`
}

type ScannerService struct {
	clientMgr *telegram.ClientManager
	storage   *storage.Storage
	engine    *downloader.Engine
}

func NewScannerService(cm *telegram.ClientManager, st *storage.Storage, eng *downloader.Engine) *ScannerService {
	return &ScannerService{
		clientMgr: cm,
		storage:   st,
		engine:    eng,
	}
}

// ResolveTarget identifica un chat, bot o canal ingresado por el usuario.
func (s *ScannerService) ResolveTarget(ctx context.Context, target string) (ResolvedTargetInfo, error) {
	if s.clientMgr == nil {
		return ResolvedTargetInfo{}, errors.New("cliente de Telegram no disponible")
	}
	if err := s.clientMgr.WaitReady(ctx); err != nil {
		return ResolvedTargetInfo{}, fmt.Errorf("cliente de Telegram no listo: %w", err)
	}

	clean := strings.TrimSpace(target)
	if clean == "" {
		return ResolvedTargetInfo{}, errors.New("objetivo vacío")
	}

	var chatID int64
	var username string

	if strings.HasPrefix(clean, "http://") || strings.HasPrefix(clean, "https://") || strings.HasPrefix(clean, "t.me/") {
		parsed, err := downloader.ParseURL(clean)
		if err == nil {
			chatID = parsed.ChatID
			username = parsed.ChatUsername
		}
	}

	if chatID == 0 {
		if id, err := strconv.ParseInt(clean, 10, 64); err == nil {
			chatID = id
		} else {
			username = strings.TrimPrefix(clean, "@")
		}
	}

	if chatID == 0 && username != "" {
		resolvedID, err := s.clientMgr.ResolveUsername(ctx, username)
		if err != nil {
			return ResolvedTargetInfo{}, fmt.Errorf("no se pudo resolver @%s: %w", username, err)
		}
		chatID = resolvedID
	}

	if chatID == 0 {
		return ResolvedTargetInfo{}, errors.New("no se pudo identificar el chat")
	}

	chatName := s.clientMgr.GetChatName(chatID)
	if chatName == "" {
		chatName = strconv.FormatInt(chatID, 10)
	}

	return ResolvedTargetInfo{
		ChatID:   chatID,
		ChatName: chatName,
		Type:     s.determineType(chatID),
		Username: username,
	}, nil
}

func (s *ScannerService) determineType(chatID int64) string {
	if chatID > 0 {
		return "user_or_bot"
	}
	sID := strconv.FormatInt(chatID, 10)
	if strings.HasPrefix(sID, "-100") {
		return "channel"
	}
	return "group"
}

// resolveInputPeer construye el InputPeerClass de MTProto recuperando access hash si es necesario.
func (s *ScannerService) resolveInputPeer(ctx context.Context, chatID int64) (tg.InputPeerClass, string, error) {
	raw := s.clientMgr.RawClient()
	if raw == nil {
		return nil, "", errors.New("cliente no conectado")
	}

	chatName := s.clientMgr.GetChatName(chatID)

	// Usuario / Bot (ID positivo)
	if chatID > 0 {
		accessHash, found := s.clientMgr.GetUserAccessHash(chatID)
		if !found {
			_ = s.clientMgr.FetchDialogs(ctx)
			accessHash, found = s.clientMgr.GetUserAccessHash(chatID)
		}
		if found {
			if chatName == "" {
				chatName = fmt.Sprintf("Usuario %d", chatID)
			}
			return &tg.InputPeerUser{UserID: chatID, AccessHash: accessHash}, chatName, nil
		}

		// Probar si era ID de canal positivo
		accessHashCh, foundCh := s.clientMgr.GetChannelAccessHash(chatID)
		if !foundCh {
			_ = s.clientMgr.FetchDialogs(ctx)
			accessHashCh, foundCh = s.clientMgr.GetChannelAccessHash(chatID)
		}
		if foundCh {
			if chatName == "" {
				chatName = fmt.Sprintf("Canal %d", chatID)
			}
			return &tg.InputPeerChannel{ChannelID: chatID, AccessHash: accessHashCh}, chatName, nil
		}

		return nil, "", fmt.Errorf("sin información de acceso para ID %d", chatID)
	}

	// Canal / Supergrupo / Grupo
	sID := strconv.FormatInt(chatID, 10)
	if strings.HasPrefix(sID, "-100") && len(sID) > 4 {
		channelID, err := strconv.ParseInt(sID[4:], 10, 64)
		if err != nil {
			return nil, "", fmt.Errorf("ID de canal inválido: %s", sID)
		}
		accessHash, found := s.clientMgr.GetChannelAccessHash(channelID)
		if !found {
			_ = s.clientMgr.FetchDialogs(ctx)
			accessHash, found = s.clientMgr.GetChannelAccessHash(channelID)
		}
		if !found {
			return nil, "", fmt.Errorf("sin acceso al canal %d", channelID)
		}
		if chatName == "" {
			chatName = fmt.Sprintf("Canal %d", channelID)
		}
		return &tg.InputPeerChannel{ChannelID: channelID, AccessHash: accessHash}, chatName, nil
	}

	// Grupo básico
	groupChatID := -chatID
	if chatName == "" {
		chatName = fmt.Sprintf("Grupo %d", groupChatID)
	}
	return &tg.InputPeerChat{ChatID: groupChatID}, chatName, nil
}

// Scan busca en el historial de Telegram filtrando por fechas, tipo de medio y tamaño.
func (s *ScannerService) Scan(ctx context.Context, req ScanRequest) (*ScanResponse, error) {
	if s.clientMgr == nil {
		return nil, errors.New("cliente de Telegram no disponible")
	}
	if err := s.clientMgr.WaitReady(ctx); err != nil {
		return nil, fmt.Errorf("cliente de Telegram no listo: %w", err)
	}

	raw := s.clientMgr.RawClient()
	if raw == nil {
		return nil, errors.New("cliente de Telegram no conectado")
	}

	// Resolver target a ChatID si hace falta
	targetInfo, err := s.ResolveTarget(ctx, req.Target)
	if err != nil {
		return nil, err
	}
	chatID := targetInfo.ChatID

	peer, chatName, err := s.resolveInputPeer(ctx, chatID)
	if err != nil {
		return nil, err
	}

	// Parsear fechas
	var startTS int64
	var endTS int64

	if req.StartDate != "" {
		if t, err := time.ParseInLocation("2006-01-02", req.StartDate, time.Local); err == nil {
			startTS = t.Unix()
		}
	}
	if req.EndDate != "" {
		if t, err := time.ParseInLocation("2006-01-02", req.EndDate, time.Local); err == nil {
			// Ajustar al final del día (23:59:59)
			endTS = t.Add(23*time.Hour + 59*time.Minute + 59*time.Second).Unix()
		}
	}
	if endTS == 0 {
		endTS = time.Now().Unix() + 86400
	}

	maxScan := req.MaxScan
	if maxScan <= 0 || maxScan > 5000 {
		maxScan = 1000
	}

	batchLimit := 100
	if maxScan < batchLimit {
		batchLimit = maxScan
	}

	offsetID := 0
	scannedTotal := 0
	var items []ScanItem
	reachedLimit := false
	var floodMsg string

	for scannedTotal < maxScan {
		// Pausa preventiva entre peticiones para evitar FLOOD_WAIT
		if scannedTotal > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(350 * time.Millisecond):
			}
		}

		reqBatch := &tg.MessagesGetHistoryRequest{
			Peer:     peer,
			OffsetID: offsetID,
			Limit:    batchLimit,
		}

		res, err := raw.MessagesGetHistory(ctx, reqBatch)
		if err != nil {
			if floodDur, ok := parseFloodWait(err); ok {
				downloader.SetFloodWait(floodDur)
				waitSeconds := int(floodDur.Seconds())
				if waitSeconds > 10 {
					floodMsg = fmt.Sprintf("Telegram solicitó una pausa de %d segundos (FLOOD_WAIT). Se devuelven los resultados obtenidos hasta el momento.", waitSeconds)
					break
				}
				time.Sleep(floodDur)
				continue
			}
			return nil, fmt.Errorf("error al consultar el historial de Telegram: %w", err)
		}

		var msgClasses []tg.MessageClass
		switch m := res.(type) {
		case *tg.MessagesMessages:
			msgClasses = m.Messages
		case *tg.MessagesMessagesSlice:
			msgClasses = m.Messages
		case *tg.MessagesChannelMessages:
			msgClasses = m.Messages
		}

		if len(msgClasses) == 0 {
			break
		}

		for _, mc := range msgClasses {
			msg, ok := mc.(*tg.Message)
			if !ok {
				continue
			}

			scannedTotal++
			offsetID = msg.ID

			msgDate := int64(msg.Date)

			// Si el mensaje es más reciente que el límite superior (EndDate), omitir
			if endTS > 0 && msgDate > endTS {
				continue
			}

			// Si el mensaje es más antiguo que StartDate, detener búsqueda (historial descendente)
			if startTS > 0 && msgDate < startTS {
				reachedLimit = true
				break
			}

			// Extraer información multimedia
			mediaInfo := downloader.ExtractMediaInfo(msg)
			if mediaInfo == nil {
				continue
			}

			// Filtros por tipo
			allowed := false
			switch mediaInfo.Kind {
			case downloader.KindPhoto:
				allowed = req.FPhotos
			case downloader.KindVideo:
				allowed = req.FVideos
			case downloader.KindSong:
				allowed = req.FAudios
			case downloader.KindSticker:
				allowed = req.FStickers
			case downloader.KindFile:
				allowed = req.FDocs
			}
			if !allowed {
				continue
			}

			// Filtros por tamaño
			if req.MinSize > 0 && mediaInfo.FileSize < req.MinSize {
				continue
			}
			if req.MaxSize > 0 && mediaInfo.FileSize > req.MaxSize {
				continue
			}

			tMsg := time.Unix(msgDate, 0)
			dateStr := tMsg.Format("2006-01-02 15:04:05")

			item := ScanItem{
				ID:               fmt.Sprintf("scanner:%d:%d", chatID, msg.ID),
				ChatID:           chatID,
				ChatName:         chatName,
				MessageID:        int64(msg.ID),
				FileName:         mediaInfo.FileName,
				CaptionFileName:  mediaInfo.CaptionFileName,
				OriginalFileName: mediaInfo.OriginalFileName,
				Kind:             string(mediaInfo.Kind),
				FileSize:         mediaInfo.FileSize,
				FormattedSize:    config.FormatBytes(float64(mediaInfo.FileSize)),
				Date:             msgDate,
				DateStr:          dateStr,
			}
			items = append(items, item)
		}

		if reachedLimit {
			break
		}
	}

	return &ScanResponse{
		Status:       "ok",
		ScannedCount: scannedTotal,
		MatchCount:   len(items),
		Items:        items,
		ReachedLimit: reachedLimit || scannedTotal >= maxScan,
		Message:      floodMsg,
	}, nil
}

// QueueItems añade los elementos escaneados a la cola de descargas.
func (s *ScannerService) QueueItems(items []ScanItem) int {
	queued := 0
	now := float64(time.Now().UnixNano()) / 1e9

	for _, item := range items {
		dlItem := storage.DownloadItem{
			ID:               item.ID,
			JobID:            fmt.Sprintf("scanner:%d", item.ChatID),
			MessageID:        item.MessageID,
			ChatID:           item.ChatID,
			FileName:         item.FileName,
			CaptionFileName:  item.CaptionFileName,
			OriginalFileName: item.OriginalFileName,
			Status:           "queued",
			Kind:             item.Kind,
			TotalStr:         item.FormattedSize,
			TotalBytes:       item.FileSize,
			Source:           "scanner",
			CreatedAt:        now,
			UpdatedAt:        now,
		}

		if s.storage != nil {
			if err := s.storage.SaveDownload(dlItem); err != nil {
				log.Printf("[SCANNER] Error guardando descarga en storage: %v", err)
			}
		}
		if s.engine != nil {
			s.engine.QueueItem(dlItem)
			queued++
		}
	}
	return queued
}

func parseFloodWait(err error) (time.Duration, bool) {
	if err == nil {
		return 0, false
	}
	texto := err.Error()
	const marca = "FLOOD_WAIT_"
	pos := strings.Index(texto, marca)
	if pos < 0 {
		return 0, false
	}
	resto := texto[pos+len(marca):]
	fin := 0
	for fin < len(resto) && resto[fin] >= '0' && resto[fin] <= '9' {
		fin++
	}
	if fin == 0 {
		return 0, false
	}
	segundos, convErr := strconv.Atoi(resto[:fin])
	if convErr != nil || segundos <= 0 {
		return 0, false
	}
	return time.Duration(segundos) * time.Second, true
}

