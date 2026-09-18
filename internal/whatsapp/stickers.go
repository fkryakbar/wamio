package whatsapp

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"whatsapp-desktop/internal/store"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

const maxRecentStickers = 200

// rememberRecentSticker builds the same metadata record from an actual sticker
// message. Some linked-device history payloads omit RecentStickers entirely,
// while the recent conversation window still contains sticker messages.
func (s *WhatsAppService) rememberRecentSticker(sticker *waE2E.StickerMessage, timestamp int64) {
	if sticker == nil || sticker.GetIsLottie() {
		return
	}
	mimetype := sticker.GetMimetype()
	if mimetype == "" {
		mimetype = "image/webp"
	}
	s.syncRecentStickers([]*waHistorySync.StickerMetadata{{
		URL:               proto.String(sticker.GetURL()),
		FileSHA256:        sticker.GetFileSHA256(),
		FileEncSHA256:     sticker.GetFileEncSHA256(),
		MediaKey:          sticker.GetMediaKey(),
		Mimetype:          proto.String(mimetype),
		Height:            proto.Uint32(sticker.GetHeight()),
		Width:             proto.Uint32(sticker.GetWidth()),
		DirectPath:        proto.String(sticker.GetDirectPath()),
		FileLength:        proto.Uint64(sticker.GetFileLength()),
		LastStickerSentTS: proto.Int64(timestamp),
		IsLottie:          proto.Bool(sticker.GetIsLottie()),
	}})
}

func stickerID(metadata interface {
	GetImageHash() string
	GetFileSHA256() []byte
}) string {
	if hash := metadata.GetImageHash(); hash != "" {
		return hash
	}
	return hex.EncodeToString(metadata.GetFileSHA256())
}

func (s *WhatsAppService) syncRecentStickers(stickers []*waHistorySync.StickerMetadata) {
	if len(stickers) == 0 {
		return
	}
	s.stickerMu.Lock()
	changed := false
	for _, sticker := range stickers {
		if sticker == nil || sticker.GetIsLottie() || sticker.GetMimetype() == "" {
			continue
		}
		if id := stickerID(sticker); id != "" {
			s.recentStickers[id] = sticker
			changed = true
			if s.chatStore != nil {
				_ = s.chatStore.UpsertRecentSticker(store.StickerRow{ID: id, URL: sticker.GetURL(), FileSHA256: sticker.GetFileSHA256(), FileEncSHA256: sticker.GetFileEncSHA256(), MediaKey: sticker.GetMediaKey(), Mimetype: sticker.GetMimetype(), Width: sticker.GetWidth(), Height: sticker.GetHeight(), DirectPath: sticker.GetDirectPath(), FileLength: sticker.GetFileLength(), LastUsedAt: sticker.GetLastStickerSentTS(), IsLottie: sticker.GetIsLottie()})
			}
		}
	}
	if len(s.recentStickers) > maxRecentStickers {
		ordered := make([]*waHistorySync.StickerMetadata, 0, len(s.recentStickers))
		for _, sticker := range s.recentStickers {
			ordered = append(ordered, sticker)
		}
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].GetLastStickerSentTS() > ordered[j].GetLastStickerSentTS() })
		for _, stale := range ordered[maxRecentStickers:] {
			delete(s.recentStickers, stickerID(stale))
		}
		if s.chatStore != nil {
			_ = s.chatStore.TrimRecentStickers(maxRecentStickers)
		}
	}
	s.stickerMu.Unlock()

	// RecentStickers is not guaranteed to be sent on every linked-device
	// session. Notify an open picker when history/live stickers add a record.
	if changed && s.ctx != nil {
		s.emitEvent("wa:stickers-update", s.GetRecentStickers())
	}
}

func (s *WhatsAppService) loadRecentStickersFromDB() {
	if s.chatStore == nil {
		return
	}
	rows, err := s.chatStore.GetRecentStickers(maxRecentStickers)
	if err != nil {
		s.log.Warnf("Failed to load recent stickers: %v", err)
		return
	}
	s.stickerMu.Lock()
	defer s.stickerMu.Unlock()
	for _, row := range rows {
		if row.IsLottie || row.ID == "" {
			continue
		}
		s.recentStickers[row.ID] = &waHistorySync.StickerMetadata{URL: proto.String(row.URL), FileSHA256: row.FileSHA256, FileEncSHA256: row.FileEncSHA256, MediaKey: row.MediaKey, Mimetype: proto.String(row.Mimetype), Width: proto.Uint32(row.Width), Height: proto.Uint32(row.Height), DirectPath: proto.String(row.DirectPath), FileLength: proto.Uint64(row.FileLength), LastStickerSentTS: proto.Int64(row.LastUsedAt), IsLottie: proto.Bool(row.IsLottie), ImageHash: proto.String(row.ID)}
	}
}

// GetRecentStickers exposes only non-secret metadata. The contents are
// downloaded lazily so a large history sync never blocks the initial inbox.
func (s *WhatsAppService) GetRecentStickers() []StickerItem {
	s.stickerMu.RLock()
	items := make([]StickerItem, 0, len(s.recentStickers))
	for id, sticker := range s.recentStickers {
		items = append(items, StickerItem{ID: id, Mimetype: sticker.GetMimetype(), Width: sticker.GetWidth(), Height: sticker.GetHeight(), LastUsedAt: sticker.GetLastStickerSentTS(), Available: true})
	}
	s.stickerMu.RUnlock()
	sort.Slice(items, func(i, j int) bool { return items[i].LastUsedAt > items[j].LastUsedAt })
	return items
}

func (s *WhatsAppService) stickerPath(id string) string {
	return filepath.Join(s.mediaCache.basePath, "stickers", hex.EncodeToString([]byte(id))+".webp")
}

func (s *WhatsAppService) stickerMetadata(id string) (*waHistorySync.StickerMetadata, error) {
	s.stickerMu.RLock()
	metadata := s.recentStickers[id]
	s.stickerMu.RUnlock()
	if metadata == nil {
		return nil, fmt.Errorf("stiker tidak tersedia; tunggu sinkronisasi WhatsApp")
	}
	return metadata, nil
}

func (s *WhatsAppService) GetStickerPreview(id string) (string, error) {
	metadata, err := s.stickerMetadata(id)
	if err != nil {
		return "", err
	}
	path := s.stickerPath(id)
	if data, readErr := os.ReadFile(path); readErr == nil {
		return base64.StdEncoding.EncodeToString(data), nil
	}
	s.mu.RLock()
	client := s.client
	s.mu.RUnlock()
	if client == nil || !client.IsConnected() {
		return "", fmt.Errorf("not connected")
	}
	data, err := client.Download(context.Background(), metadata)
	if err != nil {
		return "", fmt.Errorf("failed to download sticker: %w", err)
	}
	if err = os.MkdirAll(filepath.Dir(path), 0755); err == nil {
		err = os.WriteFile(path, data, 0644)
	}
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func (s *WhatsAppService) SendRecentSticker(chatJID, id, clientRequestID string) error {
	metadata, err := s.stickerMetadata(id)
	if err != nil {
		return err
	}
	s.mu.RLock()
	client := s.client
	s.mu.RUnlock()
	if client == nil || !client.IsConnected() {
		return fmt.Errorf("not connected")
	}
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("invalid JID: %w", err)
	}
	message := &waProto.Message{StickerMessage: &waE2E.StickerMessage{URL: proto.String(metadata.GetURL()), DirectPath: proto.String(metadata.GetDirectPath()), Mimetype: proto.String(metadata.GetMimetype()), FileSHA256: metadata.GetFileSHA256(), FileEncSHA256: metadata.GetFileEncSHA256(), MediaKey: metadata.GetMediaKey(), FileLength: proto.Uint64(metadata.GetFileLength()), Height: proto.Uint32(metadata.GetHeight()), Width: proto.Uint32(metadata.GetWidth()), StickerSentTS: proto.Int64(time.Now().Unix())}}
	resp, err := client.SendMessage(context.Background(), jid, message)
	if err != nil {
		return fmt.Errorf("failed to send sticker: %w", err)
	}
	// Best effort: a cached preview makes the sent sticker immediately visible.
	if data, previewErr := s.GetStickerPreview(id); previewErr == nil {
		if bytes, decodeErr := base64.StdEncoding.DecodeString(data); decodeErr == nil {
			_ = os.MkdirAll(filepath.Dir(s.mediaCache.mediaPath(chatJID, resp.ID, metadata.GetMimetype(), "sticker", "")), 0755)
			_ = os.WriteFile(s.mediaCache.mediaPath(chatJID, resp.ID, metadata.GetMimetype(), "sticker", ""), bytes, 0644)
		}
	}
	s.mediaCache.StoreRawMessage(chatJID, resp.ID, message)
	sent := MessageItem{ID: resp.ID, ChatJID: chatJID, SenderJID: client.Store.ID.String(), Timestamp: resp.Timestamp.Unix(), IsFromMe: true, IsRead: true, MediaType: "sticker", Mimetype: metadata.GetMimetype(), DeliveryStatus: "sent", ClientRequestID: clientRequestID}
	s.addMessageToCache(chatJID, sent)
	s.updateChatLastMessage(chatJID, sent, jid.Server == types.GroupServer)
	s.persistAndEmitChat(chatJID)
	s.emitEvent("wa:message", MessageEvent{ChatJID: chatJID, Message: sent})
	return nil
}
