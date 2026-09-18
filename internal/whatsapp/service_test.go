package whatsapp

import (
	"strings"
	"testing"

	waCommon "go.mau.fi/whatsmeow/proto/waCommon"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	waWeb "go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
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

func TestNewAccountIDIsOpaqueAndUnique(t *testing.T) {
	first, err := newAccountID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := newAccountID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first, "account-") || first == second {
		t.Fatalf("account IDs must be opaque and unique: %q, %q", first, second)
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

func TestFindInitialConversationAliasUsesConversationIdentity(t *testing.T) {
	svc := NewWhatsAppService()
	jid := types.NewJID("628123456789", types.DefaultUserServer)
	conversation := &waHistorySync.Conversation{ID: proto.String(jid.String())}
	// The initial map key can be a pre-mapping LID while the sidebar has
	// already switched to the canonical phone-number JID.
	svc.initialConversations = map[string]*waHistorySync.Conversation{
		"legacy@lid": conversation,
	}

	if got := svc.findInitialConversationAlias(jid.String()); got != conversation {
		t.Fatal("expected initial conversation to be found by its conversation ID")
	}
}

func TestSplitInitialPreloadUsesFiveForegroundAndThirtyTotal(t *testing.T) {
	conversations := make([]*waHistorySync.Conversation, 32)
	for i := range conversations {
		conversations[i] = &waHistorySync.Conversation{}
	}

	foreground, background := splitInitialPreload(conversations)
	if len(foreground) != initialForegroundChats {
		t.Fatalf("foreground count = %d, want %d", len(foreground), initialForegroundChats)
	}
	if len(background) != initialPreloadLimit-initialForegroundChats {
		t.Fatalf("background count = %d, want %d", len(background), initialPreloadLimit-initialForegroundChats)
	}
	if foreground[0] != conversations[0] || background[0] != conversations[initialForegroundChats] {
		t.Fatal("preload split changed newest-first conversation order")
	}
}

func TestSplitInitialPreloadAcceptsShortHistory(t *testing.T) {
	conversations := make([]*waHistorySync.Conversation, 3)
	foreground, background := splitInitialPreload(conversations)
	if len(foreground) != 3 || len(background) != 0 {
		t.Fatalf("short history split = %d foreground, %d background", len(foreground), len(background))
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

func TestHistoryMetadataRestoresArchivedAndPinnedChat(t *testing.T) {
	svc := NewWhatsAppService()
	chatID := "628123456789@s.whatsapp.net"
	svc.syncHistoryConversationMetadata(&waHistorySync.Conversation{
		ID:       proto.String(chatID),
		Archived: proto.Bool(true),
		Pinned:   proto.Uint32(1),
	})

	chat, ok := svc.chats[chatID]
	if !ok {
		t.Fatal("history metadata did not create chat")
	}
	if !chat.IsArchived || !chat.IsPinned {
		t.Fatalf("history metadata state archived=%v pinned=%v, want both true", chat.IsArchived, chat.IsPinned)
	}

	// A later/authoritative app-state result must not be replaced by an older
	// history snapshot received after it.
	chat.ArchiveKnown, chat.PinKnown = true, true
	chat.IsArchived, chat.IsPinned = false, false
	svc.syncHistoryConversationMetadata(&waHistorySync.Conversation{
		ID:       proto.String(chatID),
		Archived: proto.Bool(true),
		Pinned:   proto.Uint32(1),
	})
	if chat.IsArchived || chat.IsPinned {
		t.Fatal("older history metadata overwrote authoritative settings")
	}
}

func TestHistoryConversationNameUsesPayloadBeforeFallback(t *testing.T) {
	if got := historyConversationName(&waHistorySync.Conversation{
		Name:        proto.String("Tim Produk"),
		DisplayName: proto.String("Nama Tampilan"),
	}); got != "Tim Produk" {
		t.Fatalf("history group name = %q, want payload Name", got)
	}
	if got := historyConversationName(&waHistorySync.Conversation{
		DisplayName: proto.String("Nama Tampilan"),
	}); got != "Nama Tampilan" {
		t.Fatalf("history display name = %q", got)
	}
}

func TestHistoryConversationExclusionDoesNotRequireGroupLookup(t *testing.T) {
	group := types.NewJID("120363000000000000", types.GroupServer)
	if !isExcludedHistoryConversation(group, &waHistorySync.Conversation{
		IsParentGroup: proto.Bool(true),
	}) {
		t.Fatal("community parent from history must be excluded locally")
	}
	if isExcludedHistoryConversation(group, &waHistorySync.Conversation{
		Name: proto.String("Grup biasa"),
	}) {
		t.Fatal("ordinary group from history must not be excluded")
	}
}

func TestLatestHistoryPreviewUsesNewestPayloadMessage(t *testing.T) {
	preview := latestHistoryPreview(&waHistorySync.Conversation{
		Messages: []*waHistorySync.HistorySyncMsg{
			{
				Message: &waWeb.WebMessageInfo{
					Key:              &waCommon.MessageKey{ID: proto.String("older")},
					Message:          &waE2E.Message{Conversation: proto.String("pesan lama")},
					MessageTimestamp: proto.Uint64(10),
					PushName:         proto.String("Dina"),
				},
				MsgOrderID: proto.Uint64(1),
			},
			{
				Message: &waWeb.WebMessageInfo{
					Key:              &waCommon.MessageKey{ID: proto.String("newer"), FromMe: proto.Bool(true)},
					Message:          &waE2E.Message{Conversation: proto.String("pesan terbaru")},
					MessageTimestamp: proto.Uint64(20),
					PushName:         proto.String("Rina"),
					Status:           waWeb.WebMessageInfo_READ.Enum(),
				},
				MsgOrderID: proto.Uint64(2),
			},
		},
	}, true)
	if preview == nil || preview.item.Content != "pesan terbaru" || preview.item.DeliveryStatus != "read" {
		t.Fatalf("preview = %#v, want newest group message", preview)
	}
}

func TestDeliveryStatusForHistoryMessagePreservesWhatsAppReceipts(t *testing.T) {
	tests := []struct {
		name     string
		fromMe   bool
		status   waWeb.WebMessageInfo_Status
		expected string
	}{
		{name: "outgoing without receipt", fromMe: true, status: waWeb.WebMessageInfo_SERVER_ACK, expected: "sent"},
		{name: "outgoing delivered", fromMe: true, status: waWeb.WebMessageInfo_DELIVERY_ACK, expected: "delivered"},
		{name: "outgoing read", fromMe: true, status: waWeb.WebMessageInfo_READ, expected: "read"},
		{name: "outgoing played", fromMe: true, status: waWeb.WebMessageInfo_PLAYED, expected: "read"},
		{name: "incoming ignores outgoing receipt", fromMe: false, status: waWeb.WebMessageInfo_READ, expected: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if actual := deliveryStatusForHistoryMessage(test.fromMe, test.status); actual != test.expected {
				t.Fatalf("deliveryStatusForHistoryMessage(%t, %v) = %q, want %q", test.fromMe, test.status, actual, test.expected)
			}
		})
	}
}
