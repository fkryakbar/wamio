package whatsapp

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestMediaCacheReadsDownloadedImageWithoutRawMessage(t *testing.T) {
	cache := NewMediaCache(t.TempDir())
	chatJID, messageID := "123@s.whatsapp.net", "image-id"
	directory := cache.getMediaDir(chatJID)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := []byte("already-downloaded")
	if err := os.WriteFile(filepath.Join(directory, messageID+".jpg"), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	actual, found, err := cache.ReadCachedMedia(chatJID, messageID, "image/jpeg", "image", "")
	if err != nil || !found {
		t.Fatalf("ReadCachedMedia found=%v err=%v", found, err)
	}
	if actual != base64.StdEncoding.EncodeToString(payload) {
		t.Fatalf("unexpected media payload: %q", actual)
	}
}
