package whatsapp

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type stagedAttachment struct {
	AttachmentDraft
	path string
}

// StageAttachments copies user-selected paths into Wamio's staging area. A
// staged copy makes retries safe even if the original file moves afterwards.
func (s *WhatsAppService) StageAttachments(paths []string, requestedKind string) ([]AttachmentDraft, error) {
	if len(paths) == 0 {
		return []AttachmentDraft{}, nil
	}
	result := make([]AttachmentDraft, 0, len(paths))
	for _, path := range paths {
		draft, err := s.stageFile(path, requestedKind, false)
		if err != nil {
			return result, err
		}
		result = append(result, draft)
	}
	return result, nil
}

// StageDataURL is used only for media captured in the webview (camera and
// microphone). The bytes are immediately moved into the same private staging
// area as normal selected files.
func (s *WhatsAppService) StageDataURL(dataURL, fileName, requestedKind string, isPTT bool) (AttachmentDraft, error) {
	parts := strings.SplitN(dataURL, ",", 2)
	if len(parts) != 2 || !strings.Contains(parts[0], ";base64") {
		return AttachmentDraft{}, fmt.Errorf("invalid captured media payload")
	}
	data, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return AttachmentDraft{}, fmt.Errorf("invalid captured media: %w", err)
	}
	mediaType := ""
	if strings.HasPrefix(parts[0], "data:") {
		mediaType = strings.TrimPrefix(strings.Split(parts[0], ";")[0], "data:")
	}
	if fileName == "" {
		fileName = "capture" + extensionForMIME(mediaType)
	}
	if err := os.MkdirAll(s.draftDir, 0755); err != nil {
		return AttachmentDraft{}, err
	}
	tempPath := filepath.Join(s.draftDir, newDraftID()+filepath.Ext(fileName))
	if err := os.WriteFile(tempPath, data, 0600); err != nil {
		return AttachmentDraft{}, fmt.Errorf("failed to stage captured media: %w", err)
	}
	draft, err := s.stageFile(tempPath, requestedKind, isPTT)
	_ = os.Remove(tempPath)
	if err == nil {
		draft.FileName = fileName
		if isPTT {
			// The browser-side encoder always produces OGG/Opus. Preserve the
			// codec parameter because WhatsApp uses it to identify a voice note.
			draft.Mimetype = "audio/ogg; codecs=opus"
		}
		s.draftMu.Lock()
		staged := s.drafts[draft.ID]
		staged.AttachmentDraft = draft
		s.drafts[draft.ID] = staged
		s.draftMu.Unlock()
	}
	return draft, err
}

func (s *WhatsAppService) stageFile(source, requestedKind string, isPTT bool) (AttachmentDraft, error) {
	info, err := os.Stat(source)
	if err != nil {
		return AttachmentDraft{}, fmt.Errorf("file tidak dapat dibuka: %w", err)
	}
	if !info.Mode().IsRegular() {
		return AttachmentDraft{}, fmt.Errorf("hanya file biasa yang dapat dikirim")
	}
	input, err := os.Open(source)
	if err != nil {
		return AttachmentDraft{}, err
	}
	defer input.Close()
	probe := make([]byte, 512)
	count, _ := input.Read(probe)
	mimetype := mime.TypeByExtension(strings.ToLower(filepath.Ext(info.Name())))
	if mimetype == "" || mimetype == "application/octet-stream" {
		mimetype = http.DetectContentType(probe[:count])
	}
	mimetype = strings.Split(mimetype, ";")[0]
	kind, err := classifyAttachment(requestedKind, mimetype)
	if err != nil {
		return AttachmentDraft{}, err
	}
	if _, err = input.Seek(0, io.SeekStart); err != nil {
		return AttachmentDraft{}, err
	}
	if err = os.MkdirAll(s.draftDir, 0755); err != nil {
		return AttachmentDraft{}, err
	}
	id := newDraftID()
	extension := filepath.Ext(info.Name())
	if extension == "" {
		extension = extensionForMIME(mimetype)
	}
	destination := filepath.Join(s.draftDir, id+extension)
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return AttachmentDraft{}, err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(destination)
		if copyErr != nil {
			return AttachmentDraft{}, fmt.Errorf("gagal menyalin file: %w", copyErr)
		}
		return AttachmentDraft{}, closeErr
	}
	draft := AttachmentDraft{ID: id, Kind: kind, FileName: info.Name(), Mimetype: mimetype, FileSize: uint64(info.Size()), IsPTT: isPTT}
	s.draftMu.Lock()
	s.drafts[id] = stagedAttachment{AttachmentDraft: draft, path: destination}
	s.draftMu.Unlock()
	return draft, nil
}

func classifyAttachment(requestedKind, mimetype string) (string, error) {
	if requestedKind == "document" {
		return "document", nil
	}
	if requestedKind == "audio" {
		if !strings.HasPrefix(mimetype, "audio/") {
			return "", fmt.Errorf("pilih file audio")
		}
		return "audio", nil
	}
	if requestedKind == "media" {
		if strings.HasPrefix(mimetype, "image/") {
			return "image", nil
		}
		if strings.HasPrefix(mimetype, "video/") {
			return "video", nil
		}
		return "", fmt.Errorf("pilih foto atau video")
	}
	switch {
	case strings.HasPrefix(mimetype, "image/"):
		return "image", nil
	case strings.HasPrefix(mimetype, "video/"):
		return "video", nil
	case strings.HasPrefix(mimetype, "audio/"):
		return "audio", nil
	default:
		return "document", nil
	}
}

func extensionForMIME(mimetype string) string {
	if extensions, _ := mime.ExtensionsByType(mimetype); len(extensions) > 0 {
		return extensions[0]
	}
	return ".bin"
}

func newDraftID() string {
	return fmt.Sprintf("draft-%d", time.Now().UnixNano())
}

func (s *WhatsAppService) draft(id string) (stagedAttachment, error) {
	s.draftMu.RLock()
	draft, ok := s.drafts[id]
	s.draftMu.RUnlock()
	if !ok {
		return stagedAttachment{}, fmt.Errorf("lampiran sudah tidak tersedia")
	}
	return draft, nil
}

// GetDraftPreview lazily returns previews, avoiding file paths in the browser.
func (s *WhatsAppService) GetDraftPreview(id string) (string, error) {
	draft, err := s.draft(id)
	if err != nil {
		return "", err
	}
	if draft.Kind != "image" && draft.Kind != "video" {
		return "", nil
	}
	const maxPreviewBytes = 16 << 20
	if draft.FileSize > maxPreviewBytes {
		return "", nil
	}
	data, err := os.ReadFile(draft.path)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// SendAttachment uploads a staged draft then emits the canonical WhatsApp
// message. The clientRequestID is returned in the event for optimistic UI
// reconciliation.
func (s *WhatsAppService) SendAttachment(chatJID, draftID, caption, clientRequestID string) error {
	draft, err := s.draft(draftID)
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
	data, err := os.ReadFile(draft.path)
	if err != nil {
		return fmt.Errorf("failed to read staged media: %w", err)
	}
	uploadType := whatsmeow.MediaDocument
	switch draft.Kind {
	case "image":
		uploadType = whatsmeow.MediaImage
	case "video":
		uploadType = whatsmeow.MediaVideo
	case "audio":
		uploadType = whatsmeow.MediaAudio
	}
	upload, err := client.Upload(context.Background(), data, uploadType)
	if err != nil {
		return fmt.Errorf("failed to upload media: %w", err)
	}
	message := buildOutgoingMedia(draft, caption, upload)
	resp, err := client.SendMessage(context.Background(), jid, message)
	if err != nil {
		return fmt.Errorf("failed to send media: %w", err)
	}
	_, _ = s.mediaCache.StoreOutgoingMedia(chatJID, resp.ID, draft.Mimetype, draft.Kind, draft.FileName, draft.path)
	s.mediaCache.StoreRawMessage(chatJID, resp.ID, message)
	sent := MessageItem{ID: resp.ID, ChatJID: chatJID, SenderJID: client.Store.ID.String(), Content: caption, Caption: caption, Timestamp: resp.Timestamp.Unix(), IsFromMe: true, IsRead: true, MediaType: draft.Kind, FileName: draft.FileName, Mimetype: draft.Mimetype, FileSize: draft.FileSize, IsPTT: draft.IsPTT, DeliveryStatus: "sent", ClientRequestID: clientRequestID}
	s.addMessageToCache(chatJID, sent)
	s.updateChatLastMessage(chatJID, sent, jid.Server == types.GroupServer)
	s.persistAndEmitChat(chatJID)
	s.emitEvent("wa:message", MessageEvent{ChatJID: chatJID, Message: sent})

	// Success has a durable cache copy; failed uploads deliberately retain the
	// staged file so the UI can retry it.
	s.draftMu.Lock()
	delete(s.drafts, draftID)
	s.draftMu.Unlock()
	_ = os.Remove(draft.path)
	return nil
}

func buildOutgoingMedia(draft stagedAttachment, caption string, upload whatsmeow.UploadResponse) *waProto.Message {
	base := func() (url, directPath *string, fileLength *uint64, mediaKey, fileHash, encHash []byte) {
		return &upload.URL, &upload.DirectPath, proto.Uint64(upload.FileLength), upload.MediaKey, upload.FileSHA256, upload.FileEncSHA256
	}
	url, directPath, fileLength, mediaKey, fileHash, encHash := base()
	switch draft.Kind {
	case "image":
		return &waProto.Message{ImageMessage: &waE2E.ImageMessage{URL: url, DirectPath: directPath, Mimetype: proto.String(draft.Mimetype), Caption: proto.String(caption), FileLength: fileLength, MediaKey: mediaKey, FileSHA256: fileHash, FileEncSHA256: encHash}}
	case "video":
		return &waProto.Message{VideoMessage: &waE2E.VideoMessage{URL: url, DirectPath: directPath, Mimetype: proto.String(draft.Mimetype), Caption: proto.String(caption), FileLength: fileLength, MediaKey: mediaKey, FileSHA256: fileHash, FileEncSHA256: encHash}}
	case "audio":
		return &waProto.Message{AudioMessage: &waE2E.AudioMessage{URL: url, DirectPath: directPath, Mimetype: proto.String(draft.Mimetype), FileLength: fileLength, MediaKey: mediaKey, FileSHA256: fileHash, FileEncSHA256: encHash, PTT: proto.Bool(draft.IsPTT)}}
	default:
		return &waProto.Message{DocumentMessage: &waE2E.DocumentMessage{URL: url, DirectPath: directPath, Mimetype: proto.String(draft.Mimetype), Title: proto.String(draft.FileName), FileName: proto.String(draft.FileName), Caption: proto.String(caption), FileLength: fileLength, MediaKey: mediaKey, FileSHA256: fileHash, FileEncSHA256: encHash}}
	}
}
