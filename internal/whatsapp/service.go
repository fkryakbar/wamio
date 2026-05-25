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
	chats    map[string]*ChatItem    // keyed by JID string
	messages map[string][]MessageItem // keyed by chat JID string
	chatMu   sync.RWMutex

	// Persistent chat store (SQLite)
	chatStore *store.ChatStore

	// Media cache
	mediaCache *MediaCache
}

// NewWhatsAppService creates a new WhatsAppService instance
func NewWhatsAppService() *WhatsAppService {
	// Determine media cache base path
	configDir, _ := os.UserConfigDir()
	mediaBase := filepath.Join(configDir, "Wamio")

	return &WhatsAppService{
		state:      StateDisconnected,
		log:        waLog.Stdout("Wamio", "INFO", true),
		chats:      make(map[string]*ChatItem),
		messages:   make(map[string][]MessageItem),
		mediaCache: NewMediaCache(mediaBase),
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

	// Detect whether local cache is empty for logging/diagnostics.
	s.chatMu.RLock()
	needSync := len(s.chats) == 0
	s.chatMu.RUnlock()
	if needSync {
		s.log.Infof("Local chat cache empty at startup")
	}

	// Signal initial sync start
	s.emitInitialSync("running")

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
		s.db.Close()
		s.db = nil
	}

	// Clear chat data
	s.chatMu.Lock()
	s.chats = make(map[string]*ChatItem)
	s.messages = make(map[string][]MessageItem)
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
				result[i] = MessageItem{
					ID:            m.ID,
					ChatJID:       m.ChatJID,
					SenderJID:     m.SenderJID,
					SenderName:    m.SenderName,
					Content:       m.Content,
					Timestamp:     m.Timestamp,
					IsFromMe:      m.IsFromMe,
					IsRead:        m.IsRead,
					MediaType:     m.MediaType,
					MediaDuration: m.MediaDuration,
					FileName:      m.FileName,
					Mimetype:      m.Mimetype,
					IsPTT:         m.IsPTT,
				}
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
		ID:        resp.ID,
		ChatJID:   chatJID,
		SenderJID: client.Store.ID.String(),
		Content:   text,
		Timestamp: resp.Timestamp.Unix(),
		IsFromMe:  true,
		IsRead:    true,
	}

	s.addMessageToCache(chatJID, sentMsg)

	// Update chat's last message
	s.updateChatLastMessage(chatJID, text, resp.Timestamp.Unix())

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

// addMessageToCache adds a message to the in-memory cache (thread-safe)
func (s *WhatsAppService) addMessageToCache(chatJID string, msg MessageItem) {
	s.chatMu.Lock()
	defer s.chatMu.Unlock()

	s.messages[chatJID] = append(s.messages[chatJID], msg)

	// Limit cache to 500 messages per chat
	if len(s.messages[chatJID]) > 500 {
		s.messages[chatJID] = s.messages[chatJID][len(s.messages[chatJID])-500:]
	}

	// Persist to SQLite
	if s.chatStore != nil {
		_ = s.chatStore.UpsertMessage(store.MessageRow{
			ID:            msg.ID,
			ChatJID:       msg.ChatJID,
			SenderJID:     msg.SenderJID,
			SenderName:    msg.SenderName,
			Content:       msg.Content,
			Timestamp:     msg.Timestamp,
			IsFromMe:      msg.IsFromMe,
			IsRead:        msg.IsRead,
			MediaType:     msg.MediaType,
			MediaDuration: msg.MediaDuration,
			FileName:      msg.FileName,
			Mimetype:      msg.Mimetype,
			IsPTT:         msg.IsPTT,
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

	chat.LastMessage = text
	chat.LastMessageTime = timestamp

	// Persist to SQLite
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

	s.chatMu.Lock()
	for _, c := range chatRows {
		s.chats[c.JID] = &ChatItem{
			JID:             c.JID,
			Name:            c.Name,
			LastMessage:     c.LastMessage,
			LastMessageTime: c.LastMessageTime,
			UnreadCount:     c.UnreadCount,
			IsGroup:         c.IsGroup,
		}
	}
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
		// Signal initial sync done for reconnect cases (no HistorySync)
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

	case *events.Message:
		s.handleIncomingMessage(v)

	case *events.HistorySync:
		s.handleHistorySync(v)
	}
}

// handleIncomingMessage processes a real-time incoming message
func (s *WhatsAppService) handleIncomingMessage(v *events.Message) {
	info := v.Info

	// Determine chat JID
	chatJID := info.Chat.String()
	isGroup := info.Chat.Server == types.GroupServer
	isFromMe := info.IsFromMe

	// Resolve sender name
	senderName := ""
	if isFromMe {
		senderName = "Anda"
	} else {
		senderName = s.getContactName(info.Sender)
	}

	// Resolve chat name
	chatName := s.getContactName(info.Chat)

	// Extract message content
	content, mediaType := extractMessageContent(v.Message)
	if content == "" {
		// Skip messages with no extractable content (reactions, protocol msgs, etc.)
		return
	}

	// Ensure chat exists in cache
	s.ensureChatExists(info.Chat, chatName, isGroup)

	// Extract media metadata
	var mediaDuration uint32
	var fileName, mimetype string
	var isPTT bool

	if v.Message.GetImageMessage() != nil {
		mimetype = v.Message.GetImageMessage().GetMimetype()
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
		ID:            info.ID,
		ChatJID:       chatJID,
		SenderJID:     info.Sender.String(),
		SenderName:    senderName,
		Content:       content,
		Timestamp:     info.Timestamp.Unix(),
		IsFromMe:      isFromMe,
		IsRead:        isFromMe,
		MediaType:     mediaType,
		MediaDuration: mediaDuration,
		FileName:      fileName,
		Mimetype:      mimetype,
		IsPTT:         isPTT,
	}

	// Add to cache
	s.addMessageToCache(chatJID, msgItem)
	s.updateChatLastMessage(chatJID, content, info.Timestamp.Unix())

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

	s.log.Infof("Processing history sync: %d conversations", len(data.GetConversations()))

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
		}
		chat.UnreadCount = int(conv.GetUnreadCount())
		s.chatMu.Unlock()
	}

	// Emit full chat list update to frontend
	s.emitEvent("wa:chats-sync", s.GetChats())

	// Preload recent-chat messages (7 days, 20 messages each)
	recentEntries := s.GetRecentChatsWithMessages(7, 20)
	s.emitEvent("wa:recent-chat-messages", RecentChatMessagesEvent{Entries: recentEntries})

	s.emitInitialSync("done")
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
			msgs[i] = MessageItem{
				ID:            m.ID,
				ChatJID:       m.ChatJID,
				SenderJID:     m.SenderJID,
				SenderName:    m.SenderName,
				Content:       m.Content,
				Timestamp:     m.Timestamp,
				IsFromMe:      m.IsFromMe,
				IsRead:        m.IsRead,
				MediaType:     m.MediaType,
				MediaDuration: m.MediaDuration,
				FileName:      m.FileName,
				Mimetype:      m.Mimetype,
				IsPTT:         m.IsPTT,
			}
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
func (s *WhatsAppService) emitInitialSync(state string) {
	s.emitEvent("wa:initial-sync", InitialSyncEvent{State: state})
}

// GetMessagesPage returns messages for a chat with cursor-based pagination.
// If beforeTimestamp is <= 0, returns the most recent messages (limit defaults to 100, max 100).
func (s *WhatsAppService) GetMessagesPage(chatJID string, limit int, beforeTimestamp int64) []MessageItem {
	if limit <= 0 {
		limit = 100
	}
	if limit > 100 {
		limit = 100
	}

	if beforeTimestamp <= 0 {
		return s.GetMessages(chatJID, limit)
	}

	if s.chatStore == nil {
		return []MessageItem{}
	}

	rows, err := s.chatStore.GetMessagesBefore(chatJID, beforeTimestamp, limit)
	if err != nil {
		return []MessageItem{}
	}

	result := make([]MessageItem, len(rows))
	for i, m := range rows {
		result[i] = MessageItem{
			ID:            m.ID,
			ChatJID:       m.ChatJID,
			SenderJID:     m.SenderJID,
			SenderName:    m.SenderName,
			Content:       m.Content,
			Timestamp:     m.Timestamp,
			IsFromMe:      m.IsFromMe,
			IsRead:        m.IsRead,
			MediaType:     m.MediaType,
			MediaDuration: m.MediaDuration,
			FileName:      m.FileName,
			Mimetype:      m.Mimetype,
			IsPTT:         m.IsPTT,
		}
	}
	return result
}

// Utility: get current time helper (for testing)
func now() time.Time {
	return time.Now()
}
