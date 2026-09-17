package whatsapp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	waStore "go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"

	"whatsapp-desktop/internal/store"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func init() {
	// Set device name that appears in WhatsApp "Linked Devices" list
	waStore.SetOSInfo("Wamio", [3]uint32{1, 0, 0})
}

// WhatsAppService manages the whatsmeow client and exposes methods to the frontend via Wails bindings
type WhatsAppService struct {
	ctx    context.Context
	client *whatsmeow.Client
	db     *store.Database
	state  ConnectionState
	mu     sync.RWMutex
	log    waLog.Logger

	// Chat data (in-memory cache)
	chats    map[string]*ChatItem     // keyed by JID string
	messages map[string][]MessageItem // keyed by chat JID string
	chatMu   sync.RWMutex

	// Persistent chat store (SQLite)
	chatStore *store.ChatStore

	// Pairing history is asynchronous. Keep its lifecycle separate from a
	// normal reconnect, where the local cache is already authoritative.
	awaitingInitialSync bool
	initialSyncTimer    *time.Timer
	pendingOlder        map[string]bool
	filterMu            sync.RWMutex
	chatFilter          map[string]bool

	// Media cache
	mediaCache *MediaCache
}

// NewWhatsAppService creates a new WhatsAppService instance
func NewWhatsAppService() *WhatsAppService {
	// Determine media cache base path
	configDir, _ := os.UserConfigDir()
	mediaBase := filepath.Join(configDir, "Wamio")

	return &WhatsAppService{
		state:        StateDisconnected,
		log:          waLog.Stdout("Wamio", "INFO", true),
		chats:        make(map[string]*ChatItem),
		messages:     make(map[string][]MessageItem),
		pendingOlder: make(map[string]bool),
		chatFilter:   make(map[string]bool),
		mediaCache:   NewMediaCache(mediaBase),
	}
}

// SetContext sets the Wails runtime context (called from app.startup)
func (s *WhatsAppService) SetContext(ctx context.Context) {
	s.ctx = ctx
}

// Connect initiates the connection to WhatsApp.
// If the device is not yet registered, it will emit QR code events.
// If already registered, it reconnects using the stored session.
func (s *WhatsAppService) Connect(accountID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Set state to connecting
	s.setState(StateConnecting)

	// Resolve the database path for this account
	dbPath, err := store.GetDefaultDBPath(accountID)
	if err != nil {
		s.setState(StateDisconnected)
		return fmt.Errorf("failed to get DB path: %w", err)
	}

	// Initialize the database
	db, err := store.NewDatabase(dbPath)
	if err != nil {
		s.setState(StateDisconnected)
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	s.db = db

	// Initialize chat store for persistence
	chatStore, err := store.NewChatStore(db.GetSQLDB())
	if err != nil {
		s.log.Warnf("Failed to initialize chat store: %v", err)
	} else {
		s.chatStore = chatStore
		// Load persisted chats into memory
		s.loadChatsFromDB()
	}

	// Get or create device store
	deviceStore, err := db.Container.GetFirstDevice(context.Background())
	if err != nil {
		s.setState(StateDisconnected)
		return fmt.Errorf("failed to get device store: %w", err)
	}

	// Create whatsmeow client
	clientLog := waLog.Stdout("Client", "WARN", true)
	s.client = whatsmeow.NewClient(deviceStore, clientLog)
	s.awaitingInitialSync = s.client.Store.ID == nil

	// Detect whether local cache is empty for logging/diagnostics.
	s.chatMu.RLock()
	needSync := len(s.chats) == 0
	s.chatMu.RUnlock()
	if needSync {
		s.log.Infof("Local chat cache empty at startup")
	}

	// A newly paired device has no trustworthy Wamio cache. It must wait for
	// WhatsApp's RECENT/INITIAL_BOOTSTRAP payload; reconnects can use cache.
	if s.awaitingInitialSync {
		s.emitInitialSync("running")
	}

	// Register event handler
	s.client.AddEventHandler(s.handleEvent)

	// Check if we need to pair (QR scan) or just reconnect
	if s.client.Store.ID == nil {
		// New device: need QR code pairing
		return s.connectWithQR()
	}

	// Existing device: reconnect
	return s.reconnect()
}

// connectWithQR starts the QR code pairing process
func (s *WhatsAppService) connectWithQR() error {
	qrChan, _ := s.client.GetQRChannel(context.Background())

	if err := s.client.Connect(); err != nil {
		s.setState(StateDisconnected)
		return fmt.Errorf("failed to connect: %w", err)
	}

	// Process QR events in a goroutine
	go func() {
		for evt := range qrChan {
			switch evt.Event {
			case "code":
				s.setState(StateQRReady)
				s.emitEvent("wa:qr-code", QRCodeEvent{
					Code:  evt.Code,
					Event: "code",
				})
			case "success":
				s.setState(StateConnected)
				s.emitEvent("wa:qr-code", QRCodeEvent{
					Event: "success",
				})
				s.emitEvent("wa:connection", ConnectionStatusEvent{
					State:   StateConnected,
					Message: "Login berhasil!",
				})
			case "timeout":
				s.emitEvent("wa:qr-code", QRCodeEvent{
					Event: "timeout",
				})
			}
		}
	}()

	return nil
}

// reconnect connects using an existing session
func (s *WhatsAppService) reconnect() error {
	if err := s.client.Connect(); err != nil {
		s.setState(StateDisconnected)
		return fmt.Errorf("failed to reconnect: %w", err)
	}

	s.setState(StateConnected)
	s.emitEvent("wa:connection", ConnectionStatusEvent{
		State:   StateConnected,
		Message: "Terhubung kembali",
	})

	return nil
}

// Disconnect disconnects from WhatsApp without clearing the session
func (s *WhatsAppService) Disconnect() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil {
		s.client.Disconnect()
	}
	s.setState(StateDisconnected)
}

// Logout disconnects and clears the session (requires re-scanning QR)
func (s *WhatsAppService) Logout() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.initialSyncTimer != nil {
		s.initialSyncTimer.Stop()
		s.initialSyncTimer = nil
	}

	if s.client != nil {
		if err := s.client.Logout(context.Background()); err != nil {
			// Still disconnect even if logout request fails
			s.client.Disconnect()
			s.log.Warnf("Logout request failed: %v", err)
		}
		s.client.Disconnect()
		s.client = nil
	}

	if s.db != nil {
		// Logout is an explicit reset: discard only the app's derived message
		// cache before closing the shared WhatsApp database.
		if s.chatStore != nil {
			if err := s.chatStore.ClearCache(); err != nil {
				s.log.Warnf("Failed to clear Wamio chat cache: %v", err)
			}
		}
		s.db.Close()
		s.db = nil
		s.chatStore = nil
	}

	// Clear chat data
	s.chatMu.Lock()
	s.chats = make(map[string]*ChatItem)
	s.messages = make(map[string][]MessageItem)
	s.pendingOlder = make(map[string]bool)
	s.chatMu.Unlock()

	s.setState(StateLoggedOut)
	s.emitEvent("wa:connection", ConnectionStatusEvent{
		State:   StateLoggedOut,
		Message: "Logged out",
	})

	return nil
}

// IsLoggedIn returns true if the client has an active session
func (s *WhatsAppService) IsLoggedIn() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.client != nil && s.client.Store.ID != nil && s.client.IsConnected()
}

// GetConnectionState returns the current connection state
func (s *WhatsAppService) GetConnectionState() ConnectionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// GetUserInfo returns information about the logged-in user
func (s *WhatsAppService) GetUserInfo() (*UserInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.client == nil || s.client.Store.ID == nil {
		return nil, fmt.Errorf("not logged in")
	}

	jid := s.client.Store.ID
	pushName := s.client.Store.PushName
	phoneNumber := jid.User
	platform := s.client.Store.Platform

	return &UserInfo{
		JID:         jid.String(),
		PushName:    pushName,
		PhoneNumber: phoneNumber,
		Platform:    platform,
	}, nil
}

// ============================================================
// Chat & Message Methods
// ============================================================

// GetChats returns the list of conversations sorted by last message time
func (s *WhatsAppService) GetChats() []ChatItem {
	s.chatMu.RLock()
	defer s.chatMu.RUnlock()

	result := make([]ChatItem, 0, len(s.chats))
	for _, chat := range s.chats {
		result = append(result, *chat)
	}

	// Sort by last message time (newest first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].LastMessageTime > result[j].LastMessageTime
	})

	return result
}

// GetMessages returns messages for a specific chat, limited to the given count
func (s *WhatsAppService) GetMessages(chatJID string, limit int) []MessageItem {
	s.chatMu.RLock()
	msgs, ok := s.messages[chatJID]
	s.chatMu.RUnlock()

	// Fall back to database if not in memory
	if (!ok || len(msgs) == 0) && s.chatStore != nil {
		dbMsgs, err := s.chatStore.GetMessages(chatJID, limit)
		if err == nil && len(dbMsgs) > 0 {
			result := make([]MessageItem, len(dbMsgs))
			for i, m := range dbMsgs {
				result[i] = messageItemFromRow(m)
			}
			// Cache in memory
			s.chatMu.Lock()
			s.messages[chatJID] = result
			s.chatMu.Unlock()
			return result
		}
		return []MessageItem{}
	}

	if limit <= 0 || limit > len(msgs) {
		limit = len(msgs)
	}

	// Return the most recent messages
	start := len(msgs) - limit
	if start < 0 {
		start = 0
	}
	result := make([]MessageItem, limit)
	copy(result, msgs[start:])
	return result
}

// SendMessage sends a text message to the specified chat
func (s *WhatsAppService) SendMessage(chatJID string, text string) error {
	s.mu.RLock()
	client := s.client
	s.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return fmt.Errorf("not connected")
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("message cannot be empty")
	}

	// Parse the JID
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("invalid JID: %w", err)
	}

	// Send the message
	msg := &waProto.Message{
		Conversation: &text,
	}

	resp, err := client.SendMessage(context.Background(), jid, msg)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	// Add message to local cache
	sentMsg := MessageItem{
		ID:             resp.ID,
		ChatJID:        chatJID,
		SenderJID:      client.Store.ID.String(),
		Content:        text,
		Timestamp:      resp.Timestamp.Unix(),
		IsFromMe:       true,
		IsRead:         true,
		DeliveryStatus: "sent",
	}

	s.addMessageToCache(chatJID, sentMsg)

	// Update chat's last message
	lastMsgPreview := previewForMessage(sentMsg, jid.Server == types.GroupServer)
	s.updateChatLastMessage(chatJID, lastMsgPreview, resp.Timestamp.Unix())

	// Emit to frontend
	s.emitEvent("wa:message", MessageEvent{
		ChatJID: chatJID,
		Message: sentMsg,
	})

	return nil
}

// DownloadMedia downloads a media message and returns its base64 content
func (s *WhatsAppService) DownloadMedia(chatJID string, messageID string) (string, error) {
	s.mu.RLock()
	client := s.client
	s.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return "", fmt.Errorf("not connected")
	}

	filePath, err := s.mediaCache.SaveMedia(client, chatJID, messageID)
	if err != nil {
		return "", err
	}

	base64Data, err := s.mediaCache.ReadMediaAsBase64(filePath)
	if err != nil {
		return "", err
	}

	return base64Data, nil
}

// OpenDocument downloads a document and opens it with the default OS application
func (s *WhatsAppService) OpenDocument(chatJID string, messageID string) error {
	s.mu.RLock()
	client := s.client
	s.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return fmt.Errorf("not connected")
	}

	filePath, err := s.mediaCache.SaveMedia(client, chatJID, messageID)
	if err != nil {
		return err
	}

	return OpenFile(filePath)
}

// getContactName resolves a JID to a display name
func (s *WhatsAppService) getContactName(jid types.JID) string {
	if s.client == nil {
		return jid.User
	}

	// Try contact store
	contact, err := s.client.Store.Contacts.GetContact(context.Background(), jid)
	if err == nil {
		if contact.FullName != "" {
			return contact.FullName
		}
		if contact.PushName != "" {
			return contact.PushName
		}
		if contact.BusinessName != "" {
			return contact.BusinessName
		}
	}

	// For groups, try to get the group info
	if jid.Server == types.GroupServer {
		groupInfo, err := s.client.GetGroupInfo(context.Background(), jid)
		if err == nil && groupInfo.Name != "" {
			return groupInfo.Name
		}
	}

	// Fallback: use phone number
	if jid.User != "" {
		return "+" + jid.User
	}
	return jid.String()
}

// canonicalChatJID collapses WhatsApp's LID and phone-number aliases into a
// single cache key whenever the device has received a mapping for that user.
func (s *WhatsAppService) canonicalChatJID(jid types.JID) types.JID {
	if jid.Server != types.HiddenUserServer || s.client == nil || s.client.Store.LIDs == nil {
		return jid
	}
	pn, err := s.client.Store.LIDs.GetPNForLID(context.Background(), jid)
	if err == nil && !pn.IsEmpty() {
		return pn.ToNonAD()
	}
	return jid
}

// isExcludedChat keeps Status, Channels and community containers out of the
// conversation list. Normal community subgroups remain ordinary group chats.
func (s *WhatsAppService) isExcludedChat(jid types.JID) bool {
	if jid == types.StatusBroadcastJID || jid.Server == types.NewsletterServer {
		return true
	}
	if jid.Server != types.GroupServer || s.client == nil {
		return false
	}
	s.filterMu.RLock()
	cached, known := s.chatFilter[jid.String()]
	s.filterMu.RUnlock()
	if known {
		return cached
	}
	group, err := s.client.GetGroupInfo(context.Background(), jid)
	if err != nil {
		return false
	}
	excluded := group.IsParent || (group.IsDefaultSubGroup && group.IsAnnounce)
	s.filterMu.Lock()
	s.chatFilter[jid.String()] = excluded
	s.filterMu.Unlock()
	return excluded
}

func deliveryStatusForMessage(isFromMe bool) string {
	if isFromMe {
		return "sent"
	}
	return ""
}

func previewForMessage(msg MessageItem, isGroup bool) string {
	if isGroup {
		return msg.SenderName + ": " + msg.Content
	}
	if msg.IsFromMe {
		return "Anda: " + msg.Content
	}
	return msg.Content
}

func (s *WhatsAppService) refreshChatName(jid types.JID) {
	jid = s.canonicalChatJID(jid)
	if s.isExcludedChat(jid) {
		return
	}
	name := s.getContactName(jid)
	if name == "" {
		return
	}
	s.chatMu.Lock()
	chat, ok := s.chats[jid.String()]
	if ok {
		chat.Name = name
	}
	s.chatMu.Unlock()
	if !ok {
		return
	}
	s.persistAndEmitChat(jid.String())
}

func (s *WhatsAppService) refreshGroupChat(jid types.JID) {
	s.filterMu.Lock()
	delete(s.chatFilter, jid.String())
	s.filterMu.Unlock()
	if s.isExcludedChat(jid) {
		s.removeChat(jid.String())
		return
	}
	s.refreshChatName(jid)
}

func (s *WhatsAppService) persistAndEmitChat(chatJID string) {
	s.chatMu.RLock()
	chat, ok := s.chats[chatJID]
	if !ok {
		s.chatMu.RUnlock()
		return
	}
	copy := *chat
	s.chatMu.RUnlock()
	if s.chatStore != nil {
		_ = s.chatStore.UpsertChat(chatRowFromItem(copy))
	}
	s.emitEvent("wa:chat-update", ChatUpdateEvent{Chat: copy})
}

func (s *WhatsAppService) removeChat(chatJID string) {
	s.chatMu.Lock()
	delete(s.chats, chatJID)
	delete(s.messages, chatJID)
	s.chatMu.Unlock()
	if s.chatStore != nil {
		_ = s.chatStore.DeleteChatCache(chatJID)
	}
	s.emitEvent("wa:chats-sync", s.GetChats())
}

func chatRowFromItem(chat ChatItem) store.ChatRow {
	return store.ChatRow{JID: chat.JID, Name: chat.Name, LastMessage: chat.LastMessage, LastMessageTime: chat.LastMessageTime,
		UnreadCount: chat.UnreadCount, IsGroup: chat.IsGroup, LastMessageID: chat.LastMessageID, LastMessageOrder: chat.LastMessageOrder}
}

func (s *WhatsAppService) handleReceipt(receipt *events.Receipt) {
	if receipt == nil || len(receipt.MessageIDs) == 0 {
		return
	}
	status := ""
	switch receipt.Type {
	case types.ReceiptTypeDelivered:
		status = "delivered"
	case types.ReceiptTypeRead, types.ReceiptTypePlayed:
		status = "read"
	default:
		return
	}
	chatJID := s.canonicalChatJID(receipt.Chat).String()
	ids := make([]string, len(receipt.MessageIDs))
	for i, id := range receipt.MessageIDs {
		ids[i] = string(id)
	}
	s.chatMu.Lock()
	for i := range s.messages[chatJID] {
		message := &s.messages[chatJID][i]
		if !message.IsFromMe || !containsMessageID(ids, message.ID) {
			continue
		}
		if status == "read" || message.DeliveryStatus != "read" {
			message.DeliveryStatus = status
			message.IsRead = status == "read"
		}
	}
	s.chatMu.Unlock()
	if s.chatStore != nil {
		_ = s.chatStore.UpdateMessageReceipt(chatJID, ids, status)
	}
	s.emitEvent("wa:message-receipt", MessageReceiptEvent{ChatJID: chatJID, MessageIDs: ids, DeliveryStatus: status})
}

func containsMessageID(ids []string, id string) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

// addMessageToCache adds a message to the in-memory cache (thread-safe)
func (s *WhatsAppService) addMessageToCache(chatJID string, msg MessageItem) {
	s.chatMu.Lock()
	defer s.chatMu.Unlock()

	for _, existing := range s.messages[chatJID] {
		if existing.ID == msg.ID && msg.ID != "" {
			return
		}
	}
	s.messages[chatJID] = append(s.messages[chatJID], msg)
	sort.SliceStable(s.messages[chatJID], func(i, j int) bool {
		if s.messages[chatJID][i].Timestamp != s.messages[chatJID][j].Timestamp {
			return s.messages[chatJID][i].Timestamp < s.messages[chatJID][j].Timestamp
		}
		return s.messages[chatJID][i].ID < s.messages[chatJID][j].ID
	})

	// Limit cache to 500 messages per chat
	if len(s.messages[chatJID]) > 500 {
		s.messages[chatJID] = s.messages[chatJID][len(s.messages[chatJID])-500:]
	}

	// Persist to SQLite
	if s.chatStore != nil {
		_ = s.chatStore.UpsertMessage(store.MessageRow{
			ID:             msg.ID,
			ChatJID:        msg.ChatJID,
			SenderJID:      msg.SenderJID,
			SenderName:     msg.SenderName,
			Content:        msg.Content,
			Timestamp:      msg.Timestamp,
			IsFromMe:       msg.IsFromMe,
			IsRead:         msg.IsRead,
			MediaType:      msg.MediaType,
			MediaDuration:  msg.MediaDuration,
			FileName:       msg.FileName,
			Mimetype:       msg.Mimetype,
			IsPTT:          msg.IsPTT,
			DeliveryStatus: msg.DeliveryStatus,
		})
	}
}

// updateChatLastMessage updates a chat's preview text and timestamp
func (s *WhatsAppService) updateChatLastMessage(chatJID string, text string, timestamp int64) {
	s.chatMu.Lock()
	defer s.chatMu.Unlock()

	chat, ok := s.chats[chatJID]
	if !ok {
		// Create the chat entry if it doesn't exist yet
		chat = &ChatItem{
			JID:  chatJID,
			Name: chatJID,
		}
		s.chats[chatJID] = chat
	}

	if timestamp >= chat.LastMessageTime {
		chat.LastMessage = text
		chat.LastMessageTime = timestamp
		chat.LastMessageID = ""
		chat.LastMessageOrder = 0
	}

	// Persist to SQLite
	if s.chatStore != nil {
		_ = s.chatStore.UpsertChat(store.ChatRow{
			JID:              chat.JID,
			Name:             chat.Name,
			LastMessage:      chat.LastMessage,
			LastMessageTime:  chat.LastMessageTime,
			UnreadCount:      chat.UnreadCount,
			IsGroup:          chat.IsGroup,
			LastMessageID:    chat.LastMessageID,
			LastMessageOrder: chat.LastMessageOrder,
		})
	}
}

// ensureChatExists creates a chat entry if it doesn't already exist
func (s *WhatsAppService) ensureChatExists(jid types.JID, name string, isGroup bool) {
	s.chatMu.Lock()
	defer s.chatMu.Unlock()

	jidStr := jid.String()
	if _, ok := s.chats[jidStr]; !ok {
		s.chats[jidStr] = &ChatItem{
			JID:     jidStr,
			Name:    name,
			IsGroup: isGroup,
		}
	} else if name != "" && s.chats[jidStr].Name == jidStr {
		// Update name if we only had a JID before
		s.chats[jidStr].Name = name
	}
}

// loadChatsFromDB loads persisted chats and messages from SQLite into memory
func (s *WhatsAppService) loadChatsFromDB() {
	if s.chatStore == nil {
		return
	}

	chatRows, err := s.chatStore.GetAllChats()
	if err != nil {
		s.log.Warnf("Failed to load chats from DB: %v", err)
		return
	}

	loaded := make(map[string]*ChatItem, len(chatRows))
	for _, c := range chatRows {
		originalJID, parseErr := types.ParseJID(c.JID)
		if parseErr != nil {
			continue
		}
		if s.isExcludedChat(originalJID) {
			_ = s.chatStore.DeleteChatCache(c.JID)
			continue
		}
		canonical := s.canonicalChatJID(originalJID)
		if canonical.String() != c.JID {
			// Preserve the newest source metadata before deleting the LID row.
			c.JID = canonical.String()
			if c.Name == "" || !c.IsGroup {
				c.Name = s.getContactName(canonical)
			}
			_ = s.chatStore.UpsertChat(c)
			_ = s.chatStore.MergeChatCache(originalJID.String(), canonical.String())
		} else if !c.IsGroup {
			// Contact actions are already stored by whatsmeow. Refresh Wamio's
			// denormalized label at startup instead of preserving the old snapshot.
			if currentName := s.getContactName(canonical); currentName != "" && currentName != c.Name {
				c.Name = currentName
				_ = s.chatStore.UpsertChat(c)
			}
		}
		item := &ChatItem{
			JID:              c.JID,
			Name:             c.Name,
			LastMessage:      c.LastMessage,
			LastMessageTime:  c.LastMessageTime,
			UnreadCount:      c.UnreadCount,
			IsGroup:          c.IsGroup,
			LastMessageID:    c.LastMessageID,
			LastMessageOrder: c.LastMessageOrder,
		}
		if existing, exists := loaded[c.JID]; !exists || previewIsNewer(item.LastMessageTime, item.LastMessageOrder, item.LastMessageID, existing) {
			loaded[c.JID] = item
		}
	}
	s.chatMu.Lock()
	s.chats = loaded
	s.chatMu.Unlock()

	s.log.Infof("Loaded %d chats from database", len(chatRows))
}

// extractMessageContent extracts text content from a whatsmeow message proto
func extractMessageContent(msg *waProto.Message) (text string, mediaType string) {
	if msg == nil {
		return "", ""
	}
	if msg.Conversation != nil {
		return *msg.Conversation, ""
	}
	if msg.ExtendedTextMessage != nil && msg.ExtendedTextMessage.Text != nil {
		return *msg.ExtendedTextMessage.Text, ""
	}
	if msg.ImageMessage != nil {
		caption := ""
		if msg.ImageMessage.Caption != nil {
			caption = *msg.ImageMessage.Caption
		}
		if caption != "" {
			return "📷 " + caption, "image"
		}
		return "📷 Foto", "image"
	}
	if msg.VideoMessage != nil {
		caption := ""
		if msg.VideoMessage.Caption != nil {
			caption = *msg.VideoMessage.Caption
		}
		if caption != "" {
			return "🎥 " + caption, "video"
		}
		return "🎥 Video", "video"
	}
	if msg.AudioMessage != nil {
		if msg.AudioMessage.GetPTT() {
			return "🎤 Pesan suara", "audio"
		}
		return "🎵 Audio", "audio"
	}
	if msg.DocumentMessage != nil {
		fileName := "Dokumen"
		if msg.DocumentMessage.FileName != nil {
			fileName = *msg.DocumentMessage.FileName
		}
		return "📎 " + fileName, "document"
	}
	if msg.StickerMessage != nil {
		return "🖼️ Stiker", "sticker"
	}
	if msg.ContactMessage != nil {
		return "👤 Kontak", "contact"
	}
	if msg.LocationMessage != nil {
		return "📍 Lokasi", "location"
	}
	return "", ""
}

// ============================================================
// Event Handlers
// ============================================================

// handleEvent is the main event handler for whatsmeow events
func (s *WhatsAppService) handleEvent(evt interface{}) {
	switch v := evt.(type) {
	case *events.Connected:
		s.log.Infof("Connected to WhatsApp")
		s.mu.Lock()
		s.setState(StateConnected)
		s.mu.Unlock()
		s.emitEvent("wa:connection", ConnectionStatusEvent{
			State:   StateConnected,
			Message: "Terhubung",
		})

		if s.awaitingInitialSync {
			s.startInitialSyncTimeout()
			return
		}

		// A reconnect deliberately renders the existing local cache immediately.
		s.loadChatsFromDB()
		s.emitEvent("wa:chats-sync", s.GetChats())
		s.emitInitialSync("done")

	case *events.Disconnected:
		s.log.Infof("Disconnected from WhatsApp")
		s.mu.Lock()
		s.setState(StateDisconnected)
		s.mu.Unlock()
		s.emitEvent("wa:connection", ConnectionStatusEvent{
			State:   StateDisconnected,
			Message: "Terputus",
		})

	case *events.LoggedOut:
		s.log.Infof("Logged out from WhatsApp: %v", v.Reason)
		s.mu.Lock()
		s.setState(StateLoggedOut)
		s.mu.Unlock()
		s.emitEvent("wa:connection", ConnectionStatusEvent{
			State:   StateLoggedOut,
			Message: fmt.Sprintf("Logged out: %v", v.Reason),
		})

	case *events.PushNameSetting:
		s.log.Infof("Push name updated: %s", v.Action.GetName())

	case *events.Contact:
		s.refreshChatName(v.JID)

	case *events.PushName:
		s.refreshChatName(v.JID)

	case *events.BusinessName:
		s.refreshChatName(v.JID)

	case *events.GroupInfo:
		s.refreshGroupChat(v.JID)

	case *events.Receipt:
		s.handleReceipt(v)

	case *events.Message:
		s.handleIncomingMessage(v)

	case *events.HistorySync:
		s.handleHistorySync(v)
	}
}

// handleIncomingMessage processes a real-time incoming message
func (s *WhatsAppService) handleIncomingMessage(v *events.Message) {
	info := v.Info
	if s.isExcludedChat(info.Chat) || info.IsNewsletterStatus {
		return
	}
	chatJIDValue := s.canonicalChatJID(info.Chat)

	// Determine chat JID
	chatJID := chatJIDValue.String()
	isGroup := chatJIDValue.Server == types.GroupServer
	isFromMe := info.IsFromMe

	// Resolve sender name (use PushName for group members/non-contacts)
	senderName := ""
	if isFromMe {
		senderName = "Anda"
	} else if info.PushName != "" {
		senderName = info.PushName
	} else {
		senderName = s.getContactName(info.Sender)
	}

	// Resolve chat name
	chatName := s.getContactName(chatJIDValue)

	// Extract message content
	content, mediaType := extractMessageContent(v.Message)
	if content == "" {
		// Skip messages with no extractable content (reactions, protocol msgs, etc.)
		return
	}

	// Ensure chat exists in cache
	s.ensureChatExists(chatJIDValue, chatName, isGroup)

	// Extract media metadata
	var mediaDuration uint32
	var fileName, mimetype string
	var isPTT bool

	if v.Message.GetImageMessage() != nil {
		mimetype = v.Message.GetImageMessage().GetMimetype()
	} else if v.Message.GetStickerMessage() != nil {
		mimetype = v.Message.GetStickerMessage().GetMimetype()
		if mimetype == "" {
			mimetype = "image/webp"
		}
	} else if v.Message.GetAudioMessage() != nil {
		mimetype = v.Message.GetAudioMessage().GetMimetype()
		mediaDuration = v.Message.GetAudioMessage().GetSeconds()
		isPTT = v.Message.GetAudioMessage().GetPTT()
	} else if v.Message.GetVideoMessage() != nil {
		mimetype = v.Message.GetVideoMessage().GetMimetype()
		mediaDuration = v.Message.GetVideoMessage().GetSeconds()
	} else if v.Message.GetDocumentMessage() != nil {
		mimetype = v.Message.GetDocumentMessage().GetMimetype()
		fileName = v.Message.GetDocumentMessage().GetFileName()
	}

	// Cache raw message proto for media download
	if mediaType != "" {
		s.mediaCache.StoreRawMessage(chatJID, info.ID, v.Message)
	}

	// Create message item
	msgItem := MessageItem{
		ID:             info.ID,
		ChatJID:        chatJID,
		SenderJID:      info.Sender.String(),
		SenderName:     senderName,
		Content:        content,
		Timestamp:      info.Timestamp.Unix(),
		IsFromMe:       isFromMe,
		IsRead:         isFromMe,
		MediaType:      mediaType,
		MediaDuration:  mediaDuration,
		FileName:       fileName,
		Mimetype:       mimetype,
		IsPTT:          isPTT,
		DeliveryStatus: deliveryStatusForMessage(isFromMe),
	}

	// Add to cache
	s.addMessageToCache(chatJID, msgItem)

	lastMsgPreview := previewForMessage(msgItem, isGroup)
	s.updateChatLastMessage(chatJID, lastMsgPreview, info.Timestamp.Unix())

	// Update unread count for non-own messages
	if !isFromMe {
		s.chatMu.Lock()
		if chat, ok := s.chats[chatJID]; ok {
			chat.UnreadCount++
		}
		s.chatMu.Unlock()
	}

	// Emit to frontend
	s.emitEvent("wa:message", MessageEvent{
		ChatJID: chatJID,
		Message: msgItem,
	})

	// Also emit chat update for sidebar
	s.chatMu.RLock()
	if chat, ok := s.chats[chatJID]; ok {
		s.emitEvent("wa:chat-update", ChatUpdateEvent{
			Chat: *chat,
		})
	}
	s.chatMu.RUnlock()

	// Emit notification for incoming messages from others
	if !isFromMe {
		s.emitEvent("wa:notification", NotificationEvent{
			ChatJID:    chatJID,
			ChatName:   chatName,
			SenderName: senderName,
			Content:    content,
			IsGroup:    isGroup,
		})
	}
}

// handleHistorySync processes history sync events from WhatsApp
func (s *WhatsAppService) handleHistorySync(v *events.HistorySync) {
	data := v.Data
	if data == nil {
		return
	}
	syncType := data.GetSyncType()
	if syncType == waHistorySync.HistorySync_FULL {
		// The server controls delivery, but Wamio never turns FULL history into
		// its local cache. This keeps initial rendering recent-first.
		s.log.Infof("Ignoring FULL history sync (%d conversations)", len(data.GetConversations()))
		return
	}
	if syncType == waHistorySync.HistorySync_INITIAL_BOOTSTRAP ||
		syncType == waHistorySync.HistorySync_RECENT ||
		syncType == waHistorySync.HistorySync_ON_DEMAND {
		s.log.Infof("Processing %s history sync: %d conversations", syncType, len(data.GetConversations()))
		s.emitEvent("wa:history-sync-progress", map[string]interface{}{
			"count": len(data.GetConversations()), "progress": data.GetProgress(), "type": syncType.String(),
		})
		for _, conv := range data.GetConversations() {
			s.processHistoryConversation(syncType, conv)
		}
		if syncType == waHistorySync.HistorySync_RECENT && s.awaitingInitialSync && data.GetProgress() >= 100 {
			s.finishInitialSync()
		}
		return
	}
	return

	s.log.Infof("Processing history sync: %d conversations", len(data.GetConversations()))

	// Emit sync progress count to frontend
	s.emitEvent("wa:history-sync-progress", map[string]interface{}{
		"count": len(data.GetConversations()),
	})

	for _, conv := range data.GetConversations() {
		jidStr := conv.GetID()
		if jidStr == "" {
			continue
		}

		jid, err := types.ParseJID(jidStr)
		if err != nil {
			continue
		}

		isGroup := jid.Server == types.GroupServer
		chatName := s.getContactName(jid)

		// Ensure chat metadata exists in cache (messages loaded on-demand via GetMessagesPage)
		s.chatMu.Lock()
		chat, exists := s.chats[jidStr]
		if !exists {
			chat = &ChatItem{
				JID:     jidStr,
				Name:    chatName,
				IsGroup: isGroup,
			}
			s.chats[jidStr] = chat
		} else if chat.Name == "" || chat.Name == jidStr {
			chat.Name = chatName
		}
		chat.UnreadCount = int(conv.GetUnreadCount())

		// Extract last message timestamp and content from history sync
		ts := int64(conv.GetConversationTimestamp())
		if ts > 0 {
			chat.LastMessageTime = ts
		}

		if len(conv.GetMessages()) > 0 {
			// Find the newest message timestamp across all synced messages in the chunk
			for _, mObj := range conv.GetMessages() {
				if mObj != nil && mObj.GetMessage() != nil {
					msgTs := int64(mObj.GetMessage().GetMessageTimestamp())
					if msgTs > chat.LastMessageTime {
						chat.LastMessageTime = msgTs
					}
				}
			}

			// Index 0 in whatsmeow HistorySync conversation represents the newest message
			lastMsgObj := conv.GetMessages()[0]
			if lastMsgObj != nil && lastMsgObj.GetMessage() != nil && lastMsgObj.GetMessage().GetMessage() != nil {
				webMsg := lastMsgObj.GetMessage()
				content, _ := extractMessageContent(webMsg.GetMessage())
				if content != "" {
					// Prepend sender name if it's a group chat
					if isGroup {
						if webMsg.GetKey().GetFromMe() {
							content = "Anda: " + content
						} else {
							senderJID := webMsg.GetKey().GetParticipant()
							if senderJID != "" {
								if parsedSender, err := types.ParseJID(senderJID); err == nil {
									content = s.getContactName(parsedSender) + ": " + content
								}
							}
						}
					}
					chat.LastMessage = content
				}
			}
		}

		// Persist the synced chat metadata to SQLite so it remains sorted on next launch!
		if s.chatStore != nil {
			_ = s.chatStore.UpsertChat(store.ChatRow{
				JID:             chat.JID,
				Name:            chat.Name,
				LastMessage:     chat.LastMessage,
				LastMessageTime: chat.LastMessageTime,
				UnreadCount:     chat.UnreadCount,
				IsGroup:         chat.IsGroup,
			})
		}
		s.chatMu.Unlock()
	}

	// Emit full chat list update to frontend
	s.emitEvent("wa:chats-sync", s.GetChats())

	// Preload recent-chat messages (7 days, 20 messages each)
	recentEntries := s.GetRecentChatsWithMessages(7, 20)
	s.emitEvent("wa:recent-chat-messages", RecentChatMessagesEvent{Entries: recentEntries})

	s.emitInitialSync("done")
}

type orderedHistoryMessage struct {
	item  MessageItem
	order uint64
}

// processHistoryConversation accepts only the small, explicitly supported
// history streams. It never relies on the slice order supplied by WhatsApp.
func (s *WhatsAppService) processHistoryConversation(syncType waHistorySync.HistorySync_HistorySyncType, conv *waHistorySync.Conversation) {
	if conv == nil || conv.GetID() == "" {
		return
	}
	sourceJID, err := types.ParseJID(conv.GetID())
	if err != nil {
		return
	}
	if s.isExcludedChat(sourceJID) {
		s.removeChat(sourceJID.String())
		return
	}
	jid := s.canonicalChatJID(sourceJID)
	chatJID := jid.String()
	client := s.GetClient()
	if client == nil {
		return
	}

	items := make([]orderedHistoryMessage, 0, len(conv.GetMessages()))
	for _, historyMsg := range conv.GetMessages() {
		if historyMsg == nil || historyMsg.GetMessage() == nil {
			continue
		}
		parsed, parseErr := client.ParseWebMessage(sourceJID, historyMsg.GetMessage())
		if parseErr != nil {
			s.log.Debugf("Skipping malformed history message in %s: %v", chatJID, parseErr)
			continue
		}
		item, ok := s.messageItemFromEvent(parsed)
		if !ok {
			continue
		}
		item.ChatJID = chatJID
		items = append(items, orderedHistoryMessage{item: item, order: historyMsg.GetMsgOrderID()})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].item.Timestamp != items[j].item.Timestamp {
			return items[i].item.Timestamp < items[j].item.Timestamp
		}
		if items[i].order != items[j].order {
			return items[i].order < items[j].order
		}
		return items[i].item.ID < items[j].item.ID
	})

	for _, entry := range items {
		s.addMessageToCache(chatJID, entry.item)
	}
	if syncType != waHistorySync.HistorySync_ON_DEMAND && s.chatStore != nil {
		// The initial cache is intentionally small. Older messages are persisted
		// only after an explicit scroll-triggered request.
		_ = s.chatStore.TrimMessages(chatJID, 20)
		s.reloadMessagesFromDB(chatJID, 20)
	}

	var newest *orderedHistoryMessage
	if len(items) > 0 {
		candidate := items[len(items)-1]
		newest = &candidate
	}
	s.upsertHistoryChat(jid, conv, newest)

	if syncType == waHistorySync.HistorySync_ON_DEMAND {
		canRequestOlder := !historyTransferExhausted(conv)
		if s.chatStore != nil {
			state := 1
			if !canRequestOlder {
				state = 2
			}
			_ = s.chatStore.SetHistoryState(chatJID, state)
		}
		s.chatMu.Lock()
		delete(s.pendingOlder, chatJID)
		s.chatMu.Unlock()
		result := make([]MessageItem, 0, len(items))
		for _, entry := range items {
			result = append(result, entry.item)
		}
		s.emitEvent("wa:history-page", HistoryPageEvent{ChatJID: chatJID, Messages: result, CanRequestOlder: canRequestOlder})
	}
}

func historyTransferExhausted(conv *waHistorySync.Conversation) bool {
	return conv.GetEndOfHistoryTransferType() == waHistorySync.Conversation_COMPLETE_AND_NO_MORE_MESSAGE_REMAIN_ON_PRIMARY ||
		conv.GetEndOfHistoryTransferType() == waHistorySync.Conversation_COMPLETE_ON_DEMAND_SYNC_WITH_MORE_MSG_ON_PRIMARY_BUT_NO_ACCESS
}

func (s *WhatsAppService) messageItemFromEvent(v *events.Message) (MessageItem, bool) {
	if v == nil || v.Message == nil {
		return MessageItem{}, false
	}
	content, mediaType := extractMessageContent(v.Message)
	if content == "" {
		return MessageItem{}, false
	}
	info := v.Info
	senderName := s.getContactName(info.Sender)
	if info.IsFromMe {
		senderName = "Anda"
	} else if info.PushName != "" {
		senderName = info.PushName
	}
	item := MessageItem{ID: info.ID, ChatJID: info.Chat.String(), SenderJID: info.Sender.String(), SenderName: senderName,
		Content: content, Timestamp: info.Timestamp.Unix(), IsFromMe: info.IsFromMe, IsRead: info.IsFromMe, MediaType: mediaType,
		DeliveryStatus: deliveryStatusForMessage(info.IsFromMe)}
	if image := v.Message.GetImageMessage(); image != nil {
		item.Mimetype = image.GetMimetype()
	} else if audio := v.Message.GetAudioMessage(); audio != nil {
		item.Mimetype, item.MediaDuration, item.IsPTT = audio.GetMimetype(), audio.GetSeconds(), audio.GetPTT()
	} else if video := v.Message.GetVideoMessage(); video != nil {
		item.Mimetype, item.MediaDuration = video.GetMimetype(), video.GetSeconds()
	} else if document := v.Message.GetDocumentMessage(); document != nil {
		item.Mimetype, item.FileName = document.GetMimetype(), document.GetFileName()
	}
	return item, true
}

func (s *WhatsAppService) upsertHistoryChat(jid types.JID, conv *waHistorySync.Conversation, newest *orderedHistoryMessage) {
	chatJID := jid.String()
	s.chatMu.Lock()
	chat, exists := s.chats[chatJID]
	if !exists {
		chat = &ChatItem{JID: chatJID, Name: s.getContactName(jid), IsGroup: jid.Server == types.GroupServer}
		s.chats[chatJID] = chat
	}
	chat.UnreadCount = int(conv.GetUnreadCount())
	if newest != nil && previewIsNewer(newest.item.Timestamp, newest.order, newest.item.ID, chat) {
		preview := previewForMessage(newest.item, chat.IsGroup)
		chat.LastMessage, chat.LastMessageTime = preview, newest.item.Timestamp
		chat.LastMessageID, chat.LastMessageOrder = newest.item.ID, newest.order
	} else if chat.LastMessageTime == 0 && conv.GetConversationTimestamp() > 0 {
		// Metadata has no trustworthy text; keep the preview blank rather than
		// associating an old message with a newer conversation timestamp.
		chat.LastMessageTime = int64(conv.GetConversationTimestamp())
	}
	row := store.ChatRow{JID: chat.JID, Name: chat.Name, LastMessage: chat.LastMessage, LastMessageTime: chat.LastMessageTime,
		UnreadCount: chat.UnreadCount, IsGroup: chat.IsGroup, LastMessageID: chat.LastMessageID, LastMessageOrder: chat.LastMessageOrder}
	s.chatMu.Unlock()
	if s.chatStore != nil {
		_ = s.chatStore.UpsertChat(row)
	}
}

func previewIsNewer(timestamp int64, order uint64, id string, current *ChatItem) bool {
	if timestamp != current.LastMessageTime {
		return timestamp > current.LastMessageTime
	}
	if order != current.LastMessageOrder {
		return order > current.LastMessageOrder
	}
	return id > current.LastMessageID
}

func (s *WhatsAppService) reloadMessagesFromDB(chatJID string, limit int) {
	if s.chatStore == nil {
		return
	}
	rows, err := s.chatStore.GetMessages(chatJID, limit)
	if err != nil {
		return
	}
	items := make([]MessageItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, messageItemFromRow(row))
	}
	s.chatMu.Lock()
	s.messages[chatJID] = items
	s.chatMu.Unlock()
}

func messageItemFromRow(m store.MessageRow) MessageItem {
	return MessageItem{ID: m.ID, ChatJID: m.ChatJID, SenderJID: m.SenderJID, SenderName: m.SenderName,
		Content: m.Content, Timestamp: m.Timestamp, IsFromMe: m.IsFromMe, IsRead: m.IsRead,
		MediaType: m.MediaType, MediaDuration: m.MediaDuration, FileName: m.FileName,
		Mimetype: m.Mimetype, IsPTT: m.IsPTT, DeliveryStatus: m.DeliveryStatus}
}

// GetRecentChatsWithMessages returns preloaded messages for chats active within `days`.
// Used to emit wa:recent-chat-messages after history sync so the frontend can
// hydrate recent conversations without extra round-trips.
func (s *WhatsAppService) GetRecentChatsWithMessages(days int, msgLimit int) []ChatWithMessages {
	if s.chatStore == nil {
		return nil
	}

	since := time.Now().Unix() - int64(days*86400)
	recentChats, err := s.chatStore.GetRecentChats(since)
	if err != nil {
		s.log.Warnf("Failed to load recent chats: %v", err)
		return nil
	}

	result := make([]ChatWithMessages, 0, len(recentChats))
	for _, c := range recentChats {
		dbMsgs, err := s.chatStore.GetMessages(c.JID, msgLimit)
		if err != nil {
			continue
		}
		msgs := make([]MessageItem, len(dbMsgs))
		for i, m := range dbMsgs {
			msgs[i] = messageItemFromRow(m)
		}
		chat := ChatItem{
			JID:             c.JID,
			Name:            c.Name,
			LastMessage:     c.LastMessage,
			LastMessageTime: c.LastMessageTime,
			UnreadCount:     c.UnreadCount,
			IsGroup:         c.IsGroup,
		}
		result = append(result, ChatWithMessages{Chat: chat, Messages: msgs})
	}
	return result
}

// MarkChatRead resets the unread count for a chat
func (s *WhatsAppService) MarkChatRead(chatJID string) {
	s.chatMu.Lock()
	defer s.chatMu.Unlock()

	if chat, ok := s.chats[chatJID]; ok {
		chat.UnreadCount = 0
	}

	// Also emit update
	if chat, ok := s.chats[chatJID]; ok {
		s.emitEvent("wa:chat-update", ChatUpdateEvent{
			Chat: *chat,
		})
	}
}

// CheckRegistered checks if a phone number is registered on WhatsApp
func (s *WhatsAppService) CheckRegistered(phone string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.client == nil || !s.client.IsConnected() {
		return false, fmt.Errorf("not connected")
	}

	jid := types.NewJID(phone, types.DefaultUserServer)
	resp, err := s.client.IsOnWhatsApp(context.Background(), []string{jid.User})
	if err != nil {
		return false, err
	}

	for _, r := range resp {
		if r.IsIn {
			return true, nil
		}
	}
	return false, nil
}

// GetProfilePicture fetches the profile picture preview CDN URL for a JID
func (s *WhatsAppService) GetProfilePicture(chatJID string) (string, error) {
	s.mu.RLock()
	client := s.client
	s.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return "", fmt.Errorf("not connected")
	}

	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return "", err
	}

	info, err := client.GetProfilePictureInfo(context.Background(), jid, &whatsmeow.GetProfilePictureParams{
		Preview: true,
	})
	if err != nil {
		return "", err
	}

	if info == nil || info.URL == "" {
		return "", fmt.Errorf("no profile picture found")
	}

	return info.URL, nil
}

// setState updates the connection state (must be called with lock held or from emitEvent)
func (s *WhatsAppService) setState(state ConnectionState) {
	s.state = state
}

// emitEvent sends an event to the Wails frontend
func (s *WhatsAppService) emitEvent(name string, data interface{}) {
	if s.ctx != nil {
		runtime.EventsEmit(s.ctx, name, data)
	}
}

// GetClient returns the underlying whatsmeow client (for internal use only)
func (s *WhatsAppService) GetClient() *whatsmeow.Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client
}

// emitInitialSync emits the wa:initial-sync lifecycle event
func (s *WhatsAppService) emitInitialSync(state string, message ...string) {
	text := ""
	if len(message) > 0 {
		text = message[0]
	}
	s.emitEvent("wa:initial-sync", InitialSyncEvent{State: state, Message: text})
}

// GetMessagesPage returns a local page. Once it is exhausted the caller can
// ask RequestOlderMessages, which contacts the primary device.
func (s *WhatsAppService) GetMessagesPage(chatJID string, limit int, beforeTimestamp int64) MessagePage {
	return s.getMessagesPage(chatJID, limit, beforeTimestamp)
}

func (s *WhatsAppService) getMessagesPage(chatJID string, limit int, beforeTimestamp int64) MessagePage {
	if limit <= 0 {
		limit = 100
	}
	if limit > 100 {
		limit = 100
	}

	if beforeTimestamp <= 0 {
		messages := s.GetMessages(chatJID, limit)
		return MessagePage{Messages: messages, HasMoreLocal: len(messages) >= limit, CanRequestOlder: s.canRequestOlder(chatJID)}
	}

	if s.chatStore == nil {
		return MessagePage{Messages: []MessageItem{}}
	}

	rows, err := s.chatStore.GetMessagesBefore(chatJID, beforeTimestamp, limit)
	if err != nil {
		return MessagePage{Messages: []MessageItem{}}
	}

	result := make([]MessageItem, len(rows))
	for i, m := range rows {
		result[i] = messageItemFromRow(m)
	}
	return MessagePage{Messages: result, HasMoreLocal: len(result) >= limit, CanRequestOlder: s.canRequestOlder(chatJID)}
}

func (s *WhatsAppService) canRequestOlder(chatJID string) bool {
	if s.chatStore == nil {
		return false
	}
	state, err := s.chatStore.GetHistoryState(chatJID)
	return err == nil && state != 2 && state != 3
}

// RequestOlderMessages asks the primary phone for up to 50 messages preceding
// the supplied oldest local message. The result is delivered by wa:history-page.
func (s *WhatsAppService) RequestOlderMessages(chatJID, oldestMessageID string, oldestMessageFromMe bool, oldestTimestamp int64, limit int) error {
	if oldestMessageID == "" || oldestTimestamp <= 0 {
		return fmt.Errorf("an oldest local message is required")
	}
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	if !s.canRequestOlder(chatJID) {
		return fmt.Errorf("older history is unavailable")
	}
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("invalid chat JID: %w", err)
	}
	s.mu.RLock()
	client := s.client
	s.mu.RUnlock()
	if client == nil || !client.IsConnected() {
		return fmt.Errorf("not connected")
	}
	s.chatMu.Lock()
	if s.pendingOlder[chatJID] {
		s.chatMu.Unlock()
		return nil
	}
	s.pendingOlder[chatJID] = true
	s.chatMu.Unlock()

	info := &types.MessageInfo{MessageSource: types.MessageSource{Chat: jid, IsFromMe: oldestMessageFromMe}, ID: oldestMessageID, Timestamp: time.Unix(oldestTimestamp, 0)}
	if _, err = client.SendPeerMessage(context.Background(), client.BuildHistorySyncRequest(info, limit)); err != nil {
		s.chatMu.Lock()
		delete(s.pendingOlder, chatJID)
		s.chatMu.Unlock()
		return fmt.Errorf("failed to request older history: %w", err)
	}
	time.AfterFunc(30*time.Second, func() {
		s.chatMu.Lock()
		if !s.pendingOlder[chatJID] {
			s.chatMu.Unlock()
			return
		}
		delete(s.pendingOlder, chatJID)
		s.chatMu.Unlock()
		if s.chatStore != nil {
			_ = s.chatStore.SetHistoryState(chatJID, 3)
		}
		s.emitEvent("wa:history-page", HistoryPageEvent{ChatJID: chatJID, CanRequestOlder: false, Error: "Ponsel tidak merespons permintaan pesan lama."})
	})
	return nil
}

func (s *WhatsAppService) startInitialSyncTimeout() {
	if s.initialSyncTimer != nil {
		s.initialSyncTimer.Stop()
	}
	s.initialSyncTimer = time.AfterFunc(120*time.Second, func() {
		if !s.awaitingInitialSync {
			return
		}
		s.awaitingInitialSync = false
		s.emitInitialSync("failed", "Sinkronisasi pesan terbaru tidak selesai. Tautkan ulang perangkat ini.")
	})
}

func (s *WhatsAppService) finishInitialSync() {
	if !s.awaitingInitialSync {
		return
	}
	s.awaitingInitialSync = false
	if s.initialSyncTimer != nil {
		s.initialSyncTimer.Stop()
		s.initialSyncTimer = nil
	}
	s.emitEvent("wa:chats-sync", s.GetChats())
	s.emitInitialSync("done", "")
}

// Utility: get current time helper (for testing)
func now() time.Time {
	return time.Now()
}
