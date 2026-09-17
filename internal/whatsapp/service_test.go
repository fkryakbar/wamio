package whatsapp

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestNewWhatsAppService(t *testing.T) {
	svc := NewWhatsAppService()

	if svc == nil {
		t.Fatal("NewWhatsAppService returned nil")
	}

	if svc.state != StateDisconnected {
		t.Errorf("Initial state should be disconnected, got %s", svc.state)
	}

	if svc.client != nil {
		t.Error("Client should be nil before Connect")
	}

	if svc.db != nil {
		t.Error("DB should be nil before Connect")
	}
}

func TestGetConnectionState(t *testing.T) {
	svc := NewWhatsAppService()

	state := svc.GetConnectionState()
	if state != StateDisconnected {
		t.Errorf("Expected disconnected, got %s", state)
	}
}

func TestIsLoggedIn_NotConnected(t *testing.T) {
	svc := NewWhatsAppService()

	if svc.IsLoggedIn() {
		t.Error("Should not be logged in when not connected")
	}
}

func TestGetUserInfo_NotLoggedIn(t *testing.T) {
	svc := NewWhatsAppService()

	_, err := svc.GetUserInfo()
	if err == nil {
		t.Error("GetUserInfo should return error when not logged in")
	}
}

func TestDisconnect_NoClient(t *testing.T) {
	svc := NewWhatsAppService()

	// Should not panic when no client
	svc.Disconnect()

	if svc.GetConnectionState() != StateDisconnected {
		t.Error("State should be disconnected after Disconnect")
	}
}

func TestLogout_NoClient(t *testing.T) {
	svc := NewWhatsAppService()

	// Should not panic when no client
	err := svc.Logout()
	if err != nil {
		t.Errorf("Logout should not error when no client: %v", err)
	}

	if svc.GetConnectionState() != StateLoggedOut {
		t.Error("State should be logged_out after Logout")
	}
}

func TestSetState(t *testing.T) {
	svc := NewWhatsAppService()

	tests := []struct {
		state ConnectionState
	}{
		{StateConnecting},
		{StateQRReady},
		{StateConnected},
		{StateDisconnected},
		{StateLoggedOut},
	}

	for _, tt := range tests {
		svc.setState(tt.state)
		if svc.state != tt.state {
			t.Errorf("Expected state %s, got %s", tt.state, svc.state)
		}
	}
}

func TestGetMessagesPage_DefaultLimit(t *testing.T) {
	svc := NewWhatsAppService()
	page := svc.GetMessagesPage("chat@jid", 0, 12345)
	if page.Messages == nil {
		t.Fatal("expected non-nil message slice")
	}
	// no chatStore => empty result
	if len(page.Messages) != 0 {
		t.Errorf("expected empty slice without chatStore, got %d", len(page.Messages))
	}
}

func TestGetMessagesPage_ClampsLimit(t *testing.T) {
	svc := NewWhatsAppService()
	_ = svc.GetMessagesPage("chat@jid", 200, 12345)
	_ = svc.GetMessagesPage("chat@jid", -1, 12345)
	// Should not panic
}

func TestGetMessagesPage_NoCursor(t *testing.T) {
	svc := NewWhatsAppService()
	page := svc.GetMessagesPage("chat@jid", 10, 0)
	if page.Messages == nil {
		t.Fatal("expected non-nil message slice")
	}
}

func TestConnectionStateConstants(t *testing.T) {
	// Verify constant values are as expected
	if StateDisconnected != "disconnected" {
		t.Error("StateDisconnected value mismatch")
	}
	if StateConnecting != "connecting" {
		t.Error("StateConnecting value mismatch")
	}
	if StateQRReady != "qr_ready" {
		t.Error("StateQRReady value mismatch")
	}
	if StateConnected != "connected" {
		t.Error("StateConnected value mismatch")
	}
	if StateLoggedOut != "logged_out" {
		t.Error("StateLoggedOut value mismatch")
	}
}

func TestPreviewIsNewerRejectsLateOldHistoryChunk(t *testing.T) {
	chat := &ChatItem{LastMessageTime: 1000, LastMessageOrder: 20, LastMessageID: "new"}
	if previewIsNewer(1000, 10, "old", chat) {
		t.Fatal("an older history message in the same conversation timestamp must not replace preview")
	}
	if !previewIsNewer(1000, 21, "newer", chat) {
		t.Fatal("a newer message order should replace preview")
	}
}

func TestChatFilterExcludesStatusAndChannels(t *testing.T) {
	svc := NewWhatsAppService()
	if !svc.isExcludedChat(types.StatusBroadcastJID) {
		t.Fatal("status broadcast must not become a chat")
	}
	if !svc.isExcludedChat(types.NewJID("123", types.NewsletterServer)) {
		t.Fatal("newsletter must not become a chat")
	}
}

func TestPreviewDistinguishesOwnMessage(t *testing.T) {
	if got := previewForMessage(MessageItem{Content: "halo", IsFromMe: true}, false); got != "halo" {
		t.Fatalf("own preview = %q", got)
	}
	if got := previewForMessage(MessageItem{Content: "halo", IsFromMe: true, SenderName: "Anda"}, true); got != "halo" {
		t.Fatalf("own group preview = %q", got)
	}
	if got := previewForMessage(MessageItem{Content: "halo", SenderName: "Budi"}, true); got != "Budi: halo" {
		t.Fatalf("group preview = %q", got)
	}
}

func TestAdvanceDeliveryStatusNeverRegresses(t *testing.T) {
	if got := advanceDeliveryStatus("sent", "delivered"); got != "delivered" {
		t.Fatalf("sent -> delivered = %q", got)
	}
	if got := advanceDeliveryStatus("read", "delivered"); got != "read" {
		t.Fatalf("read receipt regressed to %q", got)
	}
}

func TestApplyHistoryReadStateUsesUnreadSuffix(t *testing.T) {
	items := []orderedHistoryMessage{
		{item: MessageItem{ID: "old", IsFromMe: false}},
		{item: MessageItem{ID: "mine", IsFromMe: true}},
		{item: MessageItem{ID: "new", IsFromMe: false}},
	}
	applyHistoryReadState(items, 1)
	if !items[0].item.IsRead {
		t.Fatal("incoming message before the unread boundary must be read")
	}
	if !items[1].item.IsRead {
		t.Fatal("own message must always be read")
	}
	if items[2].item.IsRead {
		t.Fatal("newest unread incoming message must remain unread")
	}
}
