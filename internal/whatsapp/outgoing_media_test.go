package whatsapp

import (
	"bytes"
	"encoding/json"
	"testing"

	"go.mau.fi/whatsmeow"
)

func TestBuildOutgoingVoiceNoteCarriesPTTMetadata(t *testing.T) {
	waveform := []uint32{4, 18, 63, 100}
	draft := stagedAttachment{AttachmentDraft: AttachmentDraft{
		Kind: "audio", Mimetype: "audio/ogg; codecs=opus", FileSize: 321,
		IsPTT: true, MediaDuration: 17, Waveform: waveform,
	}}
	message := buildOutgoingMedia(draft, "", whatsmeow.UploadResponse{
		URL: "https://example.invalid/media", DirectPath: "/media/path", FileLength: 321,
	})
	if message.AudioMessage == nil {
		t.Fatal("voice note was not encoded as AudioMessage")
	}
	audio := message.AudioMessage
	if !audio.GetPTT() || audio.GetSeconds() != 17 {
		t.Fatalf("PTT metadata missing: ptt=%v seconds=%d", audio.GetPTT(), audio.GetSeconds())
	}
	if audio.GetMimetype() != "audio/ogg; codecs=opus" || !bytes.Equal(audio.GetWaveform(), []byte{4, 18, 63, 100}) {
		t.Fatalf("voice format metadata mismatch: mimetype=%q waveform=%v", audio.GetMimetype(), audio.GetWaveform())
	}
}

func TestMessageWaveformUsesNumericJSONAcrossWailsBoundary(t *testing.T) {
	encoded, err := json.Marshal(MessageItem{Waveform: []uint32{4, 18, 63}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"waveform":[4,18,63]`)) {
		t.Fatalf("waveform must be a JSON number array, got %s", encoded)
	}
}
