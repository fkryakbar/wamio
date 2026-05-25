package whatsapp

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// MediaCache manages downloaded media files and raw message protos
type MediaCache struct {
	basePath  string
	rawMsgs   map[string]*waProto.Message // keyed by "chatJID:messageID"
	mu        sync.RWMutex
	log       waLog.Logger
}

// NewMediaCache creates a new media cache at the given base directory
func NewMediaCache(basePath string) *MediaCache {
	return &MediaCache{
		basePath: basePath,
		rawMsgs:  make(map[string]*waProto.Message),
		log:      waLog.Stdout("Media", "INFO", true),
	}
}

// StoreRawMessage caches the raw proto message for later media download
func (mc *MediaCache) StoreRawMessage(chatJID, messageID string, msg *waProto.Message) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	key := chatJID + ":" + messageID
	mc.rawMsgs[key] = msg

	// Limit cache size (keep last 2000 raw messages)
	if len(mc.rawMsgs) > 2000 {
		// Simple eviction: remove oldest entries (map doesn't guarantee order, but it's fine)
		count := 0
		for k := range mc.rawMsgs {
			if count >= 200 {
				break
			}
			delete(mc.rawMsgs, k)
			count++
		}
	}
}

// GetRawMessage retrieves a cached raw message
func (mc *MediaCache) GetRawMessage(chatJID, messageID string) *waProto.Message {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	key := chatJID + ":" + messageID
	return mc.rawMsgs[key]
}

// getMediaDir returns the directory for a chat's media files
func (mc *MediaCache) getMediaDir(chatJID string) string {
	return filepath.Join(mc.basePath, "media", sanitizeJID(chatJID))
}

// GetCachedPath returns the path if the media file already exists
func (mc *MediaCache) GetCachedPath(chatJID, messageID, ext string) string {
	path := filepath.Join(mc.getMediaDir(chatJID), messageID+ext)
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

// SaveMedia downloads media from the raw message proto and saves to disk
func (mc *MediaCache) SaveMedia(client *whatsmeow.Client, chatJID, messageID string) (string, error) {
	rawMsg := mc.GetRawMessage(chatJID, messageID)
	if rawMsg == nil {
		return "", fmt.Errorf("no raw message found for %s:%s", chatJID, messageID)
	}

	// Determine downloadable and extension
	var downloadable whatsmeow.DownloadableMessage
	var ext string

	if rawMsg.GetImageMessage() != nil {
		downloadable = rawMsg.GetImageMessage()
		ext = getExtFromMime(rawMsg.GetImageMessage().GetMimetype(), ".jpg")
	} else if rawMsg.GetAudioMessage() != nil {
		downloadable = rawMsg.GetAudioMessage()
		ext = getExtFromMime(rawMsg.GetAudioMessage().GetMimetype(), ".ogg")
	} else if rawMsg.GetVideoMessage() != nil {
		downloadable = rawMsg.GetVideoMessage()
		ext = getExtFromMime(rawMsg.GetVideoMessage().GetMimetype(), ".mp4")
	} else if rawMsg.GetDocumentMessage() != nil {
		downloadable = rawMsg.GetDocumentMessage()
		ext = getExtFromMime(rawMsg.GetDocumentMessage().GetMimetype(), ".bin")
		// Try to use original filename extension
		if fn := rawMsg.GetDocumentMessage().GetFileName(); fn != "" {
			if fext := filepath.Ext(fn); fext != "" {
				ext = fext
			}
		}
	} else if rawMsg.GetStickerMessage() != nil {
		downloadable = rawMsg.GetStickerMessage()
		ext = ".webp"
	} else {
		return "", fmt.Errorf("no downloadable media in message")
	}

	// Check if already cached
	if cached := mc.GetCachedPath(chatJID, messageID, ext); cached != "" {
		return cached, nil
	}

	// Download
	data, err := client.Download(context.Background(), downloadable)
	if err != nil {
		return "", fmt.Errorf("failed to download media: %w", err)
	}

	// Save to disk
	dir := mc.getMediaDir(chatJID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create media directory: %w", err)
	}

	filePath := filepath.Join(dir, messageID+ext)
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write media file: %w", err)
	}

	mc.log.Infof("Downloaded media: %s (%d bytes)", filePath, len(data))
	return filePath, nil
}

// ReadMediaAsBase64 reads a cached media file and returns it as base64
func (mc *MediaCache) ReadMediaAsBase64(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read media file: %w", err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// OpenFile opens a file with the OS default application
func OpenFile(filePath string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", filePath)
	case "darwin":
		cmd = exec.Command("open", filePath)
	default: // linux
		cmd = exec.Command("xdg-open", filePath)
	}
	return cmd.Start()
}

// sanitizeJID converts a JID to a safe directory name
func sanitizeJID(jid string) string {
	safe := ""
	for _, c := range jid {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
			safe += string(c)
		default:
			safe += "_"
		}
	}
	return safe
}

// getExtFromMime returns a file extension from a MIME type
func getExtFromMime(mime, fallback string) string {
	switch mime {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "audio/ogg; codecs=opus", "audio/ogg":
		return ".ogg"
	case "audio/mpeg":
		return ".mp3"
	case "audio/mp4":
		return ".m4a"
	case "video/mp4":
		return ".mp4"
	case "video/3gpp":
		return ".3gp"
	case "application/pdf":
		return ".pdf"
	default:
		return fallback
	}
}
