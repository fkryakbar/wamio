package whatsapp

import (
	"testing"
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
	msgs := svc.GetMessagesPage("chat@jid", 0, 12345)
	if msgs == nil {
		t.Fatal("expected non-nil slice")
	}
	// no chatStore => empty result
	if len(msgs) != 0 {
		t.Errorf("expected empty slice without chatStore, got %d", len(msgs))
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
	msgs := svc.GetMessagesPage("chat@jid", 10, 0)
	if msgs == nil {
		t.Fatal("expected non-nil slice")
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
