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

func TestMediaCacheStoresOutgoingMedia(t *testing.T) {
	cache := NewMediaCache(t.TempDir())
	source := filepath.Join(t.TempDir(), "source.pdf")
	payload := []byte("outgoing document")
	if err := os.WriteFile(source, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.StoreOutgoingMedia("123@s.whatsapp.net", "outgoing", "application/pdf", "document", "report.pdf", source); err != nil {
		t.Fatal(err)
	}
	actual, found, err := cache.ReadCachedMedia("123@s.whatsapp.net", "outgoing", "application/pdf", "document", "report.pdf")
	if err != nil || !found || actual != base64.StdEncoding.EncodeToString(payload) {
		t.Fatalf("outgoing cache found=%v err=%v data=%q", found, err, actual)
	}
}
