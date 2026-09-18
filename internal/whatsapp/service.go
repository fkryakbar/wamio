package whatsapp

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	waWeb "go.mau.fi/whatsmeow/proto/waWeb"
	waStore "go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"whatsapp-desktop/internal/store"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func init() {
	// Set device name that appears in WhatsApp "Linked Devices" list
	waStore.SetOSInfo("Wamio", [3]uint32{1, 0, 0})
	// Ask WhatsApp for a bounded payload on a newly linked device. The server
	// remains authoritative, but this prevents every chat from being eagerly
	// materialized before the recent inbox is usable.
	waStore.DeviceProps.HistorySyncConfig.InitialSyncMaxMessagesPerChat = proto.Uint32(uint32(initialPreloadMessages))
}

const (
	initialForegroundChats = 5
	initialPreloadLimit    = 30
	initialPreloadBatch    = 5
	initialPreloadMessages = 20
)

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
	chatStore       *store.ChatStore
	activeAccountID string
	pendingAccount  *pendingAccount

	// Pairing history is asynchronous. Keep its lifecycle separate from a
	// normal reconnect, where the local cache is already authoritative.
	awaitingInitialSync      bool
	initialSyncTimer         *time.Timer
	initialReadyTimer        *time.Timer
	initialSyncReady         bool
	initialMu                sync.Mutex
	initialConversations     map[string]*waHistorySync.Conversation
	initialMaterialized      map[string]bool
	initialMaterializing     map[string]chan struct{}
	initialPreloadGeneration uint64
	initialProcessedChats    int
	pendingOlder             map[string]bool
	filterMu                 sync.RWMutex
	chatFilter               map[string]bool

	// Media cache
	mediaCache *MediaCache

	// Attachment staging is intentionally private to the backend. The UI only
	// receives opaque draft IDs and cannot ask WhatsApp to upload arbitrary
	// paths after the user has selected/dropped a file.
	draftMu  sync.RWMutex
	drafts   map[string]stagedAttachment
	draftDir string

	stickerMu      sync.RWMutex
	recentStickers map[string]*waHistorySync.StickerMetadata
}

type pendingAccount struct {
	ID         string
	Label      string
	PreviousID string
}

// NewWhatsAppService creates a new WhatsAppService instance
func NewWhatsAppService() *WhatsAppService {
	// Determine media cache base path
	configDir, _ := os.UserConfigDir()
	mediaBase := filepath.Join(configDir, "Wamio")

	return &WhatsAppService{
		state:                StateDisconnected,
		log:                  waLog.Stdout("Wamio", "INFO", true),
		chats:                make(map[string]*ChatItem),
		messages:             make(map[string][]MessageItem),
		pendingOlder:         make(map[string]bool),
		chatFilter:           make(map[string]bool),
		initialConversations: make(map[string]*waHistorySync.Conversation),
		initialMaterialized:  make(map[string]bool),
		initialMaterializing: make(map[string]chan struct{}),
		mediaCache:           NewMediaCache(mediaBase),
		drafts:               make(map[string]stagedAttachment),
		draftDir:             filepath.Join(mediaBase, "staging"),
		recentStickers:       make(map[string]*waHistorySync.StickerMetadata),
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
	return s.connect(accountID, true)
}

// RestoreLastSession connects the last successfully linked account without
// showing QR. Failure is reported through the normal connection event and
// leaves the login screen available for manual pairing.
func (s *WhatsAppService) RestoreLastSession() (RestoreSessionResult, error) {
	accountID, err := store.LoadLastAccountID()
	if err != nil {
		return RestoreSessionResult{}, err
	}
	if accountID == "" {
		return RestoreSessionResult{Attempted: false}, nil
	}
	if err := s.connect(accountID, false); err != nil {
		return RestoreSessionResult{Attempted: true, AccountID: accountID}, err
	}
	return RestoreSessionResult{Attempted: true, AccountID: accountID}, nil
}

// GetAccounts returns linked accounts for the profile switcher. Session keys
// are deliberately not exposed: this is only display metadata.
func (s *WhatsAppService) GetAccounts() []AccountInfo {
	records, err := store.LoadAccounts()
	if err != nil {
		s.log.Warnf("Failed to load account registry: %v", err)
		return []AccountInfo{}
	}
	s.mu.RLock()
	activeID := s.activeAccountID
	s.mu.RUnlock()
	accounts := make([]AccountInfo, 0, len(records))
	for _, record := range records {
		accounts = append(accounts, AccountInfo{
			ID: record.ID, Label: record.Label, IsActive: record.ID == activeID,
			UserInfo: UserInfo{JID: record.JID, PushName: record.PushName, PhoneNumber: record.PhoneNumber, Platform: record.Platform},
		})
	}
	return accounts
}

// BeginAddAccount creates an isolated, temporary session and starts QR
// pairing. It does not add the account to the switcher until WhatsApp reports
// a successful connection.
func (s *WhatsAppService) BeginAddAccount(label string) error {
	label = strings.TrimSpace(label)
	if label == "" {
		return fmt.Errorf("nama akun wajib diisi")
	}
	// Existing installations may have a linked session from before the account
	// registry existed. Register it before replacing its client so it remains
	// selectable after the newly paired account becomes active.
	s.persistActiveAccount()
	for _, account := range s.GetAccounts() {
		if strings.EqualFold(account.Label, label) {
			return fmt.Errorf("nama akun sudah digunakan")
		}
	}
	accountID, err := newAccountID()
	if err != nil {
		return err
	}
	s.mu.Lock()
	previousID := s.activeAccountID
	if s.pendingAccount != nil {
		s.mu.Unlock()
		return fmt.Errorf("penambahan akun sedang berlangsung")
	}
	s.pendingAccount = &pendingAccount{ID: accountID, Label: label, PreviousID: previousID}
	s.mu.Unlock()
	if err := s.connect(accountID, true); err != nil {
		s.mu.Lock()
		if s.pendingAccount != nil && s.pendingAccount.ID == accountID {
			s.pendingAccount = nil
		}
		s.mu.Unlock()
		_ = store.RemoveAccountSession(accountID)
		// A failed QR setup must not strand the account that was open before
		// the user pressed Tambah akun.
		if previousID != "" {
			if restoreErr := s.connect(previousID, false); restoreErr == nil {
				_ = store.TouchAccount(previousID)
				_ = store.SaveLastAccountID(previousID)
			}
		}
		return err
	}
	return nil
}

// SwitchAccount preserves the old linked session and activates the selected
// one. Wamio intentionally keeps one live WhatsApp connection at a time.
func (s *WhatsAppService) SwitchAccount(accountID string) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return fmt.Errorf("account ID is required")
	}
	found := false
	for _, account := range s.GetAccounts() {
		if account.ID == accountID {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("akun tidak ditemukan")
	}
	s.mu.RLock()
	alreadyActive := s.activeAccountID == accountID && s.client != nil && s.client.IsConnected()
	s.mu.RUnlock()
	if alreadyActive {
		return nil
	}
	s.mu.Lock()
	s.pendingAccount = nil
	s.mu.Unlock()
	if err := s.connect(accountID, false); err != nil {
		return err
	}
	_ = store.TouchAccount(accountID)
	_ = store.SaveLastAccountID(accountID)
	s.emitAccountsChanged()
	return nil
}

// CancelAddAccount discards only the unlinked temporary session and restores
// the account that was active before the Add account flow began.
func (s *WhatsAppService) CancelAddAccount() error {
	s.mu.Lock()
	pending := s.pendingAccount
	if pending != nil {
		s.pendingAccount = nil
	}
	s.mu.Unlock()
	if pending == nil {
		return fmt.Errorf("tidak ada penambahan akun yang dapat dibatalkan")
	}

	var restoreErr error
	if pending.PreviousID != "" {
		restoreErr = s.connect(pending.PreviousID, false)
		if restoreErr == nil {
			_ = store.TouchAccount(pending.PreviousID)
			_ = store.SaveLastAccountID(pending.PreviousID)
		}
	} else {
		s.mu.Lock()
		s.closeActiveSessionLocked()
		s.resetAccountDataLocked()
		s.activeAccountID = ""
		s.setState(StateDisconnected)
		s.mu.Unlock()
		s.emitEvent("wa:connection", ConnectionStatusEvent{State: StateDisconnected})
	}
	_ = store.RemoveAccountSession(pending.ID)
	if restoreErr == nil {
		s.emitAccountsChanged()
	}
	return restoreErr
}

func newAccountID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to create account ID: %w", err)
	}
	return "account-" + hex.EncodeToString(bytes), nil
}

func (s *WhatsAppService) connect(accountID string, allowPair bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Every activation owns its own sqlite handle and websocket. Close the
	// previous one first, but never call Logout: account switching must retain
	// its linked WhatsApp session.
	s.closeActiveSessionLocked()
	s.resetAccountDataLocked()
	s.activeAccountID = ""

	// Set state to connecting
	s.setState(StateConnecting)
	s.emitEvent("wa:connection", ConnectionStatusEvent{State: StateConnecting, Message: "Memuat akun"})

	// Resolve the database path for this account
	dbPath, err := store.GetDefaultDBPath(accountID)
	if err != nil {
		s.setState(StateDisconnected)
		return fmt.Errorf("failed to get DB path: %w", err)
	}
	if !allowPair {
		if _, statErr := os.Stat(dbPath); statErr != nil {
			s.setState(StateDisconnected)
			return fmt.Errorf("saved session is unavailable")
		}
	}

	// Initialize the database
	db, err := store.NewDatabase(dbPath)
	if err != nil {
		s.setState(StateDisconnected)
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	s.db = db
	s.activeAccountID = accountID
	// Media and decrypted raw-message caches are account-scoped as well. A
	// contact JID can exist in several accounts, so a shared cache would leak
	// attachments across an account switch.
	s.mediaCache = NewMediaCache(filepath.Join(filepath.Dir(s.draftDir), "accounts", accountID))

	// Initialize chat store for persistence
	chatStore, err := store.NewChatStore(db.GetSQLDB())
	if err != nil {
		s.log.Warnf("Failed to initialize chat store: %v", err)
	} else {
		s.chatStore = chatStore
		// Load persisted chats into memory
		s.loadChatsFromDB()
		s.loadRecentStickersFromDB()
	}

	// Get or create device store
	deviceStore, err := db.Container.GetFirstDevice(context.Background())
	if err != nil {
		s.closeActiveSessionLocked()
		s.resetAccountDataLocked()
		s.setState(StateDisconnected)
		return fmt.Errorf("failed to get device store: %w", err)
	}

	// Create whatsmeow client
	clientLog := waLog.Stdout("Client", "WARN", true)
	s.client = whatsmeow.NewClient(deviceStore, clientLog)
	client := s.client
	// Chat settings live in whatsmeow's account store rather than Wamio's
	// derived cache. Hydrate labels as soon as the client exists so a
	// reconnect is accurate even before another history chunk arrives.
	s.hydrateChatSettings()
	s.awaitingInitialSync = s.client.Store.ID == nil
	s.initialSyncReady = !s.awaitingInitialSync
	s.initialMu.Lock()
	s.initialProcessedChats = 0
	s.initialConversations = make(map[string]*waHistorySync.Conversation)
	s.initialMaterialized = make(map[string]bool)
	s.initialMaterializing = make(map[string]chan struct{})
	s.initialPreloadGeneration++
	s.initialMu.Unlock()

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
		s.emitSyncProgress("initial", -1, 0, 0, "Menyiapkan sinkronisasi chat terbaru")
	}

	// Register event handler
	client.AddEventHandler(func(evt interface{}) { s.handleClientEvent(client, evt) })

	// Check if we need to pair (QR scan) or just reconnect
	if client.Store.ID == nil {
		if !allowPair {
			s.closeActiveSessionLocked()
			s.resetAccountDataLocked()
			s.activeAccountID = ""
			s.setState(StateDisconnected)
			return fmt.Errorf("saved session has expired")
		}
		// New device: need QR code pairing
		if err := s.connectWithQR(client); err != nil {
			s.closeActiveSessionLocked()
			s.resetAccountDataLocked()
			s.activeAccountID = ""
			return err
		}
		return nil
	}

	// Existing device: reconnect
	if err := s.reconnect(client); err != nil {
		s.closeActiveSessionLocked()
		s.resetAccountDataLocked()
		s.activeAccountID = ""
		return err
	}
	return nil
}

func (s *WhatsAppService) closeActiveSessionLocked() {
	if s.client != nil {
		s.client.Disconnect()
		s.client = nil
	}
	if s.db != nil {
		_ = s.db.Close()
		s.db = nil
	}
	s.chatStore = nil
	s.invalidateInitialPreload()
}

func (s *WhatsAppService) resetAccountDataLocked() {
	s.chatMu.Lock()
	s.chats = make(map[string]*ChatItem)
	s.messages = make(map[string][]MessageItem)
	s.pendingOlder = make(map[string]bool)
	s.chatMu.Unlock()
	s.draftMu.Lock()
	s.drafts = make(map[string]stagedAttachment)
	s.draftMu.Unlock()
}

// connectWithQR starts the QR code pairing process
func (s *WhatsAppService) connectWithQR(client *whatsmeow.Client) error {
	qrChan, _ := client.GetQRChannel(context.Background())

	if err := client.Connect(); err != nil {
		s.setState(StateDisconnected)
		return fmt.Errorf("failed to connect: %w", err)
	}

	// Process QR events in a goroutine
	go func() {
		for evt := range qrChan {
			if !s.isCurrentClient(client) {
				return
			}
			switch evt.Event {
			case "code":
				s.mu.Lock()
				s.setState(StateQRReady)
				s.mu.Unlock()
				s.emitEvent("wa:qr-code", QRCodeEvent{
					Code:  evt.Code,
					Event: "code",
				})
			case "success":
				s.mu.Lock()
				s.setState(StateConnected)
				s.mu.Unlock()
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
func (s *WhatsAppService) reconnect(client *whatsmeow.Client) error {
	if err := client.Connect(); err != nil {
		s.setState(StateDisconnected)
		return fmt.Errorf("failed to reconnect: %w", err)
	}

	return nil
}

func (s *WhatsAppService) isCurrentClient(client *whatsmeow.Client) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return client != nil && s.client == client
}

func (s *WhatsAppService) handleClientEvent(client *whatsmeow.Client, evt interface{}) {
	if !s.isCurrentClient(client) {
		return
	}
	s.handleEvent(evt)
}

// Disconnect disconnects from WhatsApp without clearing the session
func (s *WhatsAppService) Disconnect() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil {
		s.client.Disconnect()
	}
	s.setState(StateDisconnected)
	s.invalidateInitialPreload()
}

func (s *WhatsAppService) invalidateInitialPreload() {
	s.initialMu.Lock()
	s.initialPreloadGeneration++
	s.initialMu.Unlock()
}

// Logout disconnects and clears the session (requires re-scanning QR)
func (s *WhatsAppService) Logout() error {
	s.mu.Lock()
	activeAccountID := s.activeAccountID
	if s.initialSyncTimer != nil {
		s.initialSyncTimer.Stop()
		s.initialSyncTimer = nil
	}
	if s.initialReadyTimer != nil {
		s.initialReadyTimer.Stop()
		s.initialReadyTimer = nil
	}
	s.awaitingInitialSync = false
	s.initialSyncReady = false
	s.initialMu.Lock()
	s.initialConversations = make(map[string]*waHistorySync.Conversation)
	s.initialMaterialized = make(map[string]bool)
	s.initialMaterializing = make(map[string]chan struct{})
	s.initialPreloadGeneration++
	s.initialMu.Unlock()

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
	s.activeAccountID = ""
	s.pendingAccount = nil

	// Clear chat data
	s.chatMu.Lock()
	s.chats = make(map[string]*ChatItem)
	s.messages = make(map[string][]MessageItem)
	s.pendingOlder = make(map[string]bool)
	s.chatMu.Unlock()

	s.setState(StateLoggedOut)
	s.mu.Unlock()

	if activeAccountID != "" {
		_ = store.RemoveAccount(activeAccountID)
		_ = store.RemoveAccountSession(activeAccountID)
	}
	accounts := s.GetAccounts()
	if len(accounts) > 0 {
		// The registry is sorted by most recently used, so logout seamlessly
		// returns to the last available account without logging it out.
		if err := s.SwitchAccount(accounts[0].ID); err != nil {
			s.emitEvent("wa:connection", ConnectionStatusEvent{State: StateDisconnected, Message: "Gagal membuka akun lain"})
			return err
		}
		return nil
	}
	_ = store.ClearLastAccountID()
	s.emitAccountsChanged()
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

	return userInfoFromClient(s.client), nil
}

func userInfoFromClient(client *whatsmeow.Client) *UserInfo {
	if client == nil || client.Store == nil || client.Store.ID == nil {
		return nil
	}
	jid := client.Store.ID
	return &UserInfo{JID: jid.String(), PushName: client.Store.PushName, PhoneNumber: jid.User, Platform: client.Store.Platform}
}

func (s *WhatsAppService) persistActiveAccount() {
	s.mu.RLock()
	accountID, client := s.activeAccountID, s.client
	label := accountID
	if s.pendingAccount != nil && s.pendingAccount.ID == accountID {
		label = s.pendingAccount.Label
	}
	s.mu.RUnlock()
	info := userInfoFromClient(client)
	if accountID == "" || info == nil {
		return
	}
	if label == accountID {
		for _, account := range s.GetAccounts() {
			if account.ID == accountID {
				label = account.Label
				break
			}
		}
	}
	if err := store.UpsertAccount(store.AccountRecord{ID: accountID, Label: label, JID: info.JID, PushName: info.PushName, PhoneNumber: info.PhoneNumber, Platform: info.Platform, LastUsedAt: time.Now().Unix()}); err != nil {
		s.log.Warnf("Failed to persist account %s: %v", accountID, err)
		return
	}
	_ = store.SaveLastAccountID(accountID)
	s.mu.Lock()
	if s.pendingAccount != nil && s.pendingAccount.ID == accountID {
		s.pendingAccount = nil
	}
	s.mu.Unlock()
	s.emitAccountsChanged()
}

func (s *WhatsAppService) emitAccountsChanged() {
	s.emitEvent("wa:accounts-changed", s.GetAccounts())
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

	// Pinned conversations stay above the normal newest-first ordering.
	sort.Slice(result, func(i, j int) bool {
		if result[i].IsPinned != result[j].IsPinned {
			return result[i].IsPinned
		}
		return result[i].LastMessageTime > result[j].LastMessageTime
	})

	return result
}

// GetCallLog returns call records supplied by WhatsApp history sync.
func (s *WhatsAppService) GetCallLog() []CallLogEntry {
	if s.chatStore == nil {
		return []CallLogEntry{}
	}
	rows, err := s.chatStore.GetCallLog(200)
	if err != nil {
		s.log.Warnf("Failed to load call log: %v", err)
		return []CallLogEntry{}
	}
	result := make([]CallLogEntry, 0, len(rows))
	for _, row := range rows {
		result = append(result, CallLogEntry{
			ChatJID: row.ChatJID, ChatName: row.ChatName, Timestamp: row.Timestamp,
			Outcome: row.Outcome, Duration: row.Duration, IsVideo: row.IsVideo, IsIncoming: row.IsIncoming,
		})
	}
	return result
}

// GetChatLists returns synchronized list definitions. Built-in filters are
// rendered by the client; custom list memberships are persisted locally.
func (s *WhatsAppService) GetChatLists() []ChatList {
	result := []ChatList{{ID: "all", Name: "All", Type: "all", Order: 0, IsActive: true}, {ID: "unread", Name: "Unread", Type: "unread", Order: 1, IsActive: true}, {ID: "groups", Name: "Groups", Type: "groups", Order: 2, IsActive: true}}
	if s.chatStore == nil {
		return result
	}
	rows, err := s.chatStore.GetChatLists()
	if err != nil {
		return result
	}
	for _, row := range rows {
		result = append(result, ChatList{ID: row.ID, Name: row.Name, Type: row.Type, Order: row.Order, IsActive: row.IsActive})
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Order < result[j].Order })
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
				s.attachReactions(&result[i])
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

// GetMessagesAround returns a persisted local context around a reply target.
// If it is empty, callers may request older history from the primary device.
func (s *WhatsAppService) GetMessagesAround(chatJID, messageID string, limit int) MessagePage {
	if s.chatStore == nil || chatJID == "" || messageID == "" {
		return MessagePage{Messages: []MessageItem{}}
	}
	rows, err := s.chatStore.GetMessagesAround(chatJID, messageID, limit)
	if err != nil {
		return MessagePage{Messages: []MessageItem{}}
	}
	items := make([]MessageItem, 0, len(rows))
	for _, row := range rows {
		item := messageItemFromRow(row)
		s.attachReactions(&item)
		items = append(items, item)
	}
	return MessagePage{Messages: items, HasMoreLocal: true, CanRequestOlder: s.canRequestOlder(chatJID)}
}

// SendMessage sends a text message to the specified chat.
func (s *WhatsAppService) SendMessage(chatJID string, text string, clientRequestID string) error {
	_, err := s.sendTextMessage(chatJID, text, nil, false, clientRequestID)
	return err
}

// SendReply sends text while preserving WhatsApp's native quoted-message
// context so other linked devices can render it as a reply too.
func (s *WhatsAppService) SendReply(chatJID, text string, replyTo MessageReference, clientRequestID string) error {
	if replyTo.ID == "" || replyTo.ChatJID != chatJID {
		return fmt.Errorf("a message from this chat is required for reply")
	}
	_, err := s.sendTextMessage(chatJID, text, &replyTo, false, clientRequestID)
	return err
}

func (s *WhatsAppService) sendTextMessage(chatJID, text string, replyTo *MessageReference, forwarded bool, clientRequestID string) (MessageItem, error) {
	s.mu.RLock()
	client := s.client
	s.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return MessageItem{}, fmt.Errorf("not connected")
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return MessageItem{}, fmt.Errorf("message cannot be empty")
	}

	// Parse the JID
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return MessageItem{}, fmt.Errorf("invalid JID: %w", err)
	}

	var msg *waProto.Message
	if replyTo == nil && !forwarded {
		msg = &waProto.Message{Conversation: &text}
	} else {
		contextInfo := &waProto.ContextInfo{}
		if replyTo != nil {
			contextInfo.StanzaID = proto.String(replyTo.ID)
			contextInfo.RemoteJID = proto.String(replyTo.ChatJID)
			if replyTo.SenderJID != "" {
				contextInfo.Participant = proto.String(replyTo.SenderJID)
			}
		}
		if forwarded {
			contextInfo.IsForwarded = proto.Bool(true)
			contextInfo.ForwardingScore = proto.Uint32(1)
		}
		msg = &waProto.Message{ExtendedTextMessage: &waProto.ExtendedTextMessage{Text: &text, ContextInfo: contextInfo}}
	}

	resp, err := client.SendMessage(context.Background(), jid, msg)
	if err != nil {
		return MessageItem{}, fmt.Errorf("failed to send message: %w", err)
	}

	// Add message to local cache
	sentMsg := MessageItem{
		ID:              resp.ID,
		ChatJID:         chatJID,
		SenderJID:       client.Store.ID.String(),
		Content:         text,
		Timestamp:       resp.Timestamp.Unix(),
		IsFromMe:        true,
		IsRead:          true,
		DeliveryStatus:  "sent",
		ReplyTo:         replyTo,
		IsForwarded:     forwarded,
		ClientRequestID: clientRequestID,
	}

	s.addMessageToCache(chatJID, sentMsg)

	// Update chat's last message
	s.updateChatLastMessage(chatJID, sentMsg, jid.Server == types.GroupServer)
	s.persistAndEmitChat(chatJID)

	// Emit to frontend
	s.emitEvent("wa:message", MessageEvent{
		ChatJID: chatJID,
		Message: sentMsg,
	})

	return sentMsg, nil
}

// ReactToMessage adds, replaces, or removes (empty emoji) the current user's
// reaction on a message. The server response is accepted before UI state moves.
func (s *WhatsAppService) ReactToMessage(chatJID, messageID, emoji string) error {
	target, err := s.findMessage(chatJID, messageID)
	if err != nil || target.IsDeleted {
		return fmt.Errorf("message is unavailable")
	}
	client := s.GetClient()
	if client == nil || !client.IsConnected() {
		return fmt.Errorf("not connected")
	}
	chat, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("invalid chat JID: %w", err)
	}
	sender, err := types.ParseJID(target.SenderJID)
	if err != nil {
		return fmt.Errorf("invalid message sender: %w", err)
	}
	if _, err = client.SendMessage(context.Background(), chat, client.BuildReaction(chat, sender, types.MessageID(messageID), emoji)); err != nil {
		return fmt.Errorf("failed to react: %w", err)
	}
	if s.chatStore != nil {
		ownJID := client.Store.ID.String()
		_ = s.chatStore.UpsertReaction(store.ReactionRow{ChatJID: chatJID, MessageID: messageID, ReactorJID: ownJID, Emoji: emoji, Timestamp: time.Now().UnixMilli()})
	}
	reactions := s.refreshCachedReactions(chatJID, messageID)
	s.emitEvent("wa:message-reaction", MessageReactionEvent{ChatJID: chatJID, MessageID: messageID, Reactions: reactions})
	return nil
}

// ForwardMessages forwards selected text messages to each selected chat. Each
// destination returns independently because this operation is not atomic.
func (s *WhatsAppService) ForwardMessages(sourceChatJID string, messageIDs, targetChatJIDs []string) []ForwardResult {
	results := make([]ForwardResult, 0, len(targetChatJIDs))
	if len(messageIDs) == 0 || len(targetChatJIDs) == 0 {
		return results
	}
	messages := make([]MessageItem, 0, len(messageIDs))
	for _, id := range messageIDs {
		message, err := s.findMessage(sourceChatJID, id)
		if err != nil || message.IsDeleted || message.MediaType != "" {
			return []ForwardResult{{Success: false, Error: "Hanya pesan teks yang masih tersedia dapat diteruskan."}}
		}
		messages = append(messages, message)
	}
	for _, target := range targetChatJIDs {
		result := ForwardResult{ChatJID: target, Success: true}
		for _, message := range messages {
			if _, err := s.sendTextMessage(target, message.Content, nil, true, ""); err != nil {
				result.Success, result.Error = false, err.Error()
				break
			}
		}
		results = append(results, result)
	}
	return results
}

func (s *WhatsAppService) DeleteMessageForEveryone(chatJID, messageID string) error {
	target, err := s.findMessage(chatJID, messageID)
	if err != nil || !target.IsFromMe || target.IsDeleted {
		return fmt.Errorf("only your available messages can be deleted for everyone")
	}
	client := s.GetClient()
	if client == nil || !client.IsConnected() {
		return fmt.Errorf("not connected")
	}
	chat, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("invalid chat JID: %w", err)
	}
	if _, err = client.SendMessage(context.Background(), chat, client.BuildRevoke(chat, types.EmptyJID, types.MessageID(messageID))); err != nil {
		return fmt.Errorf("failed to delete message: %w", err)
	}
	s.applyMessageDeleted(chatJID, messageID, false)
	return nil
}

func (s *WhatsAppService) DeleteMessageForMe(chatJID, messageID string) error {
	target, err := s.findMessage(chatJID, messageID)
	if err != nil {
		return fmt.Errorf("message is unavailable")
	}
	client := s.GetClient()
	if client == nil || !client.IsConnected() {
		return fmt.Errorf("not connected")
	}
	chat, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("invalid chat JID: %w", err)
	}
	sender, _ := types.ParseJID(target.SenderJID)
	participant := "0"
	if !target.IsFromMe && !sender.IsEmpty() && chat.Server == types.GroupServer {
		participant = sender.String()
	}
	patch := appstate.PatchInfo{Type: appstate.WAPatchRegularHigh, Mutations: []appstate.MutationInfo{{
		Index:   []string{appstate.IndexDeleteMessageForMe, chat.String(), messageID, boolString(target.IsFromMe), participant},
		Version: 3,
		Value: &waSyncAction.SyncActionValue{DeleteMessageForMeAction: &waSyncAction.DeleteMessageForMeAction{
			DeleteMedia: proto.Bool(false), MessageTimestamp: proto.Int64(target.Timestamp),
		}},
	}}}
	if err = client.SendAppState(context.Background(), patch); err != nil {
		return fmt.Errorf("failed to delete message for me: %w", err)
	}
	s.applyMessageDeleted(chatJID, messageID, true)
	return nil
}

func boolString(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func (s *WhatsAppService) findMessage(chatJID, messageID string) (MessageItem, error) {
	s.chatMu.RLock()
	for _, message := range s.messages[chatJID] {
		if message.ID == messageID {
			s.chatMu.RUnlock()
			return message, nil
		}
	}
	s.chatMu.RUnlock()
	if s.chatStore != nil {
		row, err := s.chatStore.GetMessage(chatJID, messageID)
		if err == nil {
			message := messageItemFromRow(row)
			s.attachReactions(&message)
			return message, nil
		}
	}
	return MessageItem{}, fmt.Errorf("message not found")
}

func (s *WhatsAppService) applyMessageDeleted(chatJID, messageID string, forMe bool) {
	if s.chatStore != nil {
		if forMe {
			_ = s.chatStore.DeleteMessageForMe(chatJID, messageID)
		} else {
			_ = s.chatStore.MarkMessageDeleted(chatJID, messageID)
		}
	}
	s.chatMu.Lock()
	messages := s.messages[chatJID]
	for i := 0; i < len(messages); i++ {
		if messages[i].ID == messageID {
			if forMe {
				messages = append(messages[:i], messages[i+1:]...)
			} else {
				messages[i].IsDeleted = true
				messages[i].Content, messages[i].MediaType, messages[i].FileName, messages[i].Mimetype = "", "", "", ""
				messages[i].Thumbnail, messages[i].Caption, messages[i].FileSize = "", "", 0
				messages[i].Kind, messages[i].Call = "", nil
				messages[i].Reactions = nil
			}
			break
		}
	}
	if !forMe {
		for i := range messages {
			if messages[i].ReplyTo != nil && messages[i].ReplyTo.ChatJID == chatJID && messages[i].ReplyTo.ID == messageID {
				messages[i].ReplyTo.Content = "Pesan ini telah dihapus"
			}
		}
	}
	s.messages[chatJID] = messages
	s.chatMu.Unlock()
	s.refreshChatPreview(chatJID)
	s.emitEvent("wa:message-delete", MessageDeleteEvent{ChatJID: chatJID, MessageID: messageID, ForMe: forMe})
}

func (s *WhatsAppService) refreshChatPreview(chatJID string) {
	items := s.GetMessages(chatJID, 100)
	var newest *MessageItem
	for i := len(items) - 1; i >= 0; i-- {
		candidate := items[i]
		newest = &candidate
		break
	}
	s.chatMu.Lock()
	chat, ok := s.chats[chatJID]
	if ok {
		if newest == nil {
			chat.LastMessage, chat.LastMessageTime, chat.LastMessageID = "", 0, ""
			chat.LastMessageFromMe, chat.LastMessageStatus = false, ""
		} else {
			chat.LastMessage = previewForMessage(*newest, chat.IsGroup)
			chat.LastMessageTime, chat.LastMessageID = newest.Timestamp, newest.ID
			chat.LastMessageFromMe = newest.IsFromMe
			if newest.IsFromMe {
				chat.LastMessageStatus = newest.DeliveryStatus
			} else {
				chat.LastMessageStatus = ""
			}
		}
	}
	s.chatMu.Unlock()
	if ok {
		s.persistAndEmitChat(chatJID)
	}
}

// GetCachedMedia returns a previously-downloaded media payload when present.
func (s *WhatsAppService) GetCachedMedia(chatJID, messageID string) (string, error) {
	message, err := s.findMessage(chatJID, messageID)
	if err != nil {
		return "", err
	}
	data, found, err := s.mediaCache.ReadCachedMedia(chatJID, messageID, message.Mimetype, message.MediaType, message.FileName)
	if err != nil {
		return "", err
	}
	if !found {
		return "", nil
	}
	return data, nil
}

// DownloadMedia downloads a media message and returns its base64 content.
// It checks the durable disk cache before requiring a connected client.
func (s *WhatsAppService) DownloadMedia(chatJID string, messageID string) (string, error) {
	if cached, err := s.GetCachedMedia(chatJID, messageID); err == nil && cached != "" {
		return cached, nil
	}
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
	_, err := s.downloadDocument(chatJID, messageID)
	if err != nil {
		return err
	}
	message, err := s.findMessage(chatJID, messageID)
	if err != nil {
		return err
	}
	path := s.mediaCache.CachedMediaPath(chatJID, messageID, message.Mimetype, message.MediaType, message.FileName)
	if path == "" {
		return fmt.Errorf("document cache is unavailable")
	}
	return OpenFile(path)
}

func (s *WhatsAppService) GetDocumentState(chatJID string, messageID string) (DocumentState, error) {
	message, err := s.findMessage(chatJID, messageID)
	if err != nil {
		return DocumentState{}, err
	}
	if message.MediaType != "document" {
		return DocumentState{}, fmt.Errorf("message is not a document")
	}
	path := s.mediaCache.CachedMediaPath(chatJID, messageID, message.Mimetype, message.MediaType, message.FileName)
	state := DocumentState{Downloaded: path != "", FileName: message.FileName, FileSize: message.FileSize}
	if path != "" {
		if info, statErr := os.Stat(path); statErr == nil {
			state.FileSize = uint64(info.Size())
		}
	}
	return state, nil
}

func (s *WhatsAppService) DownloadDocument(chatJID string, messageID string) (DocumentState, error) {
	return s.downloadDocument(chatJID, messageID)
}

func (s *WhatsAppService) downloadDocument(chatJID string, messageID string) (DocumentState, error) {
	state, err := s.GetDocumentState(chatJID, messageID)
	if err != nil || state.Downloaded {
		return state, err
	}
	s.mu.RLock()
	client := s.client
	s.mu.RUnlock()
	if client == nil || !client.IsConnected() {
		return state, fmt.Errorf("not connected")
	}
	filePath, err := s.mediaCache.SaveMedia(client, chatJID, messageID)
	if err != nil {
		return state, err
	}
	if info, statErr := os.Stat(filePath); statErr == nil {
		state.FileSize = uint64(info.Size())
	}
	state.Downloaded = true
	return state, nil
}

func (s *WhatsAppService) SaveDocumentAs(chatJID string, messageID string) (bool, error) {
	if _, err := s.downloadDocument(chatJID, messageID); err != nil {
		return false, err
	}
	message, err := s.findMessage(chatJID, messageID)
	if err != nil {
		return false, err
	}
	source := s.mediaCache.CachedMediaPath(chatJID, messageID, message.Mimetype, message.MediaType, message.FileName)
	if source == "" {
		return false, fmt.Errorf("document cache is unavailable")
	}
	name := message.FileName
	if name == "" {
		name = "document" + filepath.Ext(source)
	}
	target, err := runtime.SaveFileDialog(s.ctx, runtime.SaveDialogOptions{Title: "Simpan dokumen", DefaultFilename: name})
	if err != nil || target == "" {
		return false, err
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return false, err
	}
	if err = os.WriteFile(target, data, 0644); err != nil {
		return false, err
	}
	return true, nil
}

// OpenExternalURL validates chat links before handing them to the OS browser.
func (s *WhatsAppService) OpenExternalURL(raw string) error {
	if strings.HasPrefix(raw, "www.") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("invalid web link")
	}
	runtime.BrowserOpenURL(s.ctx, parsed.String())
	return nil
}

// lookupContactName resolves a human-facing name without ever treating the
// phone-number fallback as an authoritative contact update. During the first
// history stream, LID-to-PN mappings and contacts can arrive in either order.
func (s *WhatsAppService) lookupContactName(jid types.JID) (string, bool) {
	if s.client == nil {
		return "", false
	}
	aliases := []types.JID{jid}
	if canonical := s.canonicalChatJID(jid); canonical != jid {
		aliases = append(aliases, canonical)
	}
	for _, alias := range aliases {
		contact, err := s.client.Store.Contacts.GetContact(context.Background(), alias)
		if err != nil {
			continue
		}
		if contact.FullName != "" {
			return contact.FullName, true
		}
		if contact.PushName != "" {
			return contact.PushName, true
		}
		if contact.BusinessName != "" {
			return contact.BusinessName, true
		}
	}

	// For groups, try to get the group info
	if jid.Server == types.GroupServer {
		groupInfo, err := s.client.GetGroupInfo(context.Background(), jid)
		if err == nil && groupInfo.Name != "" {
			return groupInfo.Name, true
		}
	}
	return "", false
}

// getContactName resolves a JID to a display name while retaining a harmless
// fallback for a genuinely unknown contact.
func (s *WhatsAppService) getContactName(jid types.JID) string {
	if name, ok := s.lookupContactName(jid); ok {
		return name
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

// deliveryStatusForHistoryMessage retains WhatsApp's persisted receipt state
// when it is present in a history-sync message. A parsed events.Message only
// exposes MessageInfo and loses WebMessageInfo.Status, which previously made
// every outgoing message loaded after login look like it had only one tick.
func deliveryStatusForHistoryMessage(isFromMe bool, status waWeb.WebMessageInfo_Status) string {
	if !isFromMe {
		return ""
	}
	switch status {
	case waWeb.WebMessageInfo_READ, waWeb.WebMessageInfo_PLAYED:
		return "read"
	case waWeb.WebMessageInfo_DELIVERY_ACK:
		return "delivered"
	default:
		// PENDING, SERVER_ACK, and absent statuses have not yet reached the
		// recipient (or do not contain a more specific receipt state).
		return "sent"
	}
}

func previewForMessage(msg MessageItem, isGroup bool) string {
	if msg.IsDeleted {
		return "Pesan ini telah dihapus"
	}
	content := msg.Content
	if msg.MediaType != "" {
		switch msg.MediaType {
		case "image":
			content = "Foto"
		case "video":
			content = "Video"
		case "audio":
			if msg.IsPTT {
				content = "Pesan suara"
			} else {
				content = "Audio"
			}
		case "document":
			content = "Dokumen: " + msg.FileName
		case "sticker":
			content = "Stiker"
		}
		if msg.Caption != "" {
			content = msg.Caption
		}
	}
	if msg.IsFromMe {
		return content
	}
	if isGroup && msg.SenderName != "" {
		return msg.SenderName + ": " + content
	}
	return content
}

// refreshChatName updates every local alias for a contact. Initial history can
// arrive with a LID while contact app-state arrives with the phone-number JID
// (or the reverse), so updating only one map key leaves the sidebar stuck on
// a number after a fresh login.
func (s *WhatsAppService) refreshChatName(jid types.JID) {
	if jid.IsEmpty() {
		return
	}
	if s.isExcludedChat(s.canonicalChatJID(jid)) {
		return
	}
	name, resolved := s.lookupContactName(jid)
	if !resolved {
		return
	}
	s.refreshChatsForContact(jid, name)
}

func (s *WhatsAppService) refreshChatsForContact(contactJID types.JID, name string) {
	if name == "" {
		return
	}
	canonicalContact := s.canonicalChatJID(contactJID)
	updated := make([]string, 0, 1)
	s.chatMu.Lock()
	for chatJID, chat := range s.chats {
		if chat.IsGroup {
			continue
		}
		candidate, err := types.ParseJID(chatJID)
		if err != nil {
			continue
		}
		if candidate != contactJID && s.canonicalChatJID(candidate) != canonicalContact {
			continue
		}
		if chat.Name != name {
			chat.Name = name
			updated = append(updated, chatJID)
		}
	}
	s.chatMu.Unlock()
	for _, chatJID := range updated {
		s.persistAndEmitChat(chatJID)
	}
}

// reconcileContactNames catches the initial-sync ordering where contact
// entries and LID-to-PN mappings are committed after chat history. Contact
// events handle ordinary updates; these delayed passes handle that one-time
// bootstrap race without turning a phone number fallback into a final name.
func (s *WhatsAppService) reconcileContactNames() {
	s.chatMu.RLock()
	chatJIDs := make([]string, 0, len(s.chats))
	for chatJID, chat := range s.chats {
		if !chat.IsGroup {
			chatJIDs = append(chatJIDs, chatJID)
		}
	}
	s.chatMu.RUnlock()
	for _, chatJID := range chatJIDs {
		jid, err := types.ParseJID(chatJID)
		if err != nil {
			continue
		}
		if name, resolved := s.lookupContactName(jid); resolved {
			s.refreshChatsForContact(jid, name)
		}
	}
}

func (s *WhatsAppService) scheduleContactNameReconciliation() {
	s.mu.RLock()
	accountID := s.activeAccountID
	client := s.client
	s.mu.RUnlock()
	for _, delay := range []time.Duration{1500 * time.Millisecond, 5 * time.Second, 15 * time.Second} {
		time.AfterFunc(delay, func() {
			s.mu.RLock()
			stillCurrent := s.activeAccountID == accountID && s.client == client && s.state == StateConnected
			s.mu.RUnlock()
			if stillCurrent {
				s.reconcileContactNames()
			}
		})
	}
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
		UnreadCount: chat.UnreadCount, IsGroup: chat.IsGroup, LastMessageID: chat.LastMessageID, LastMessageOrder: chat.LastMessageOrder,
		LastReadAt: chat.LastReadAt, LastMessageFromMe: chat.LastMessageFromMe, LastMessageStatus: chat.LastMessageStatus,
		IsArchived: chat.IsArchived, IsPinned: chat.IsPinned, MutedUntil: chat.MutedUntil}
}

func (s *WhatsAppService) handleReceipt(receipt *events.Receipt) {
	if receipt == nil || len(receipt.MessageIDs) == 0 {
		return
	}
	status := ""
	switch receipt.Type {
	case types.ReceiptTypeDelivered:
		status = "delivered"
	case types.ReceiptTypeRead, types.ReceiptTypePlayed, types.ReceiptTypeReadSelf:
		status = "read"
	default:
		return
	}
	chatJID := s.canonicalChatJID(receipt.Chat).String()
	ids := make([]string, len(receipt.MessageIDs))
	for i, id := range receipt.MessageIDs {
		ids[i] = string(id)
	}
	previewChanged := false
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
	if chat, ok := s.chats[chatJID]; ok && chat.LastMessageFromMe && containsMessageID(ids, chat.LastMessageID) {
		nextStatus := advanceDeliveryStatus(chat.LastMessageStatus, status)
		if nextStatus != chat.LastMessageStatus {
			chat.LastMessageStatus = nextStatus
			previewChanged = true
		}
	}
	s.chatMu.Unlock()
	if s.chatStore != nil {
		_ = s.chatStore.UpdateMessageReceipt(chatJID, ids, status)
		if status == "read" {
			if incoming, err := s.chatStore.HasIncomingMessageIDs(chatJID, ids); err == nil && incoming {
				s.applyIncomingRead(chatJID, ids, receipt.Timestamp.Unix())
			}
		}
	}
	if previewChanged {
		s.persistAndEmitChat(chatJID)
	}
	s.emitEvent("wa:message-receipt", MessageReceiptEvent{ChatJID: chatJID, MessageIDs: ids, DeliveryStatus: status})
}

func advanceDeliveryStatus(current, next string) string {
	rank := map[string]int{"": 0, "sent": 1, "delivered": 2, "read": 3}
	if rank[next] > rank[current] {
		return next
	}
	return current
}

func containsMessageID(ids []string, id string) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

func messageRowFromItem(msg MessageItem) store.MessageRow {
	row := store.MessageRow{ID: msg.ID, ChatJID: msg.ChatJID, SenderJID: msg.SenderJID, SenderName: msg.SenderName,
		Content: msg.Content, Timestamp: msg.Timestamp, IsFromMe: msg.IsFromMe, IsRead: msg.IsRead,
		MediaType: msg.MediaType, MediaDuration: msg.MediaDuration, FileName: msg.FileName,
		Mimetype: msg.Mimetype, Thumbnail: msg.Thumbnail, IsPTT: msg.IsPTT, DeliveryStatus: msg.DeliveryStatus,
		IsForwarded: msg.IsForwarded, IsDeleted: msg.IsDeleted, Caption: msg.Caption, FileSize: msg.FileSize, Kind: msg.Kind}
	if msg.Call != nil {
		row.CallID, row.CallOutcome, row.CallDuration, row.CallIsVideo, row.CallIsIncoming = msg.Call.CallID, msg.Call.Outcome, msg.Call.Duration, msg.Call.IsVideo, msg.Call.IsIncoming
	}
	if msg.ReplyTo != nil {
		row.ReplyID, row.ReplyChatJID = msg.ReplyTo.ID, msg.ReplyTo.ChatJID
		row.ReplySenderJID, row.ReplySenderName = msg.ReplyTo.SenderJID, msg.ReplyTo.SenderName
		row.ReplyContent, row.ReplyMediaType, row.ReplyFromMe = msg.ReplyTo.Content, msg.ReplyTo.MediaType, msg.ReplyTo.IsFromMe
	}
	return row
}

func (s *WhatsAppService) attachReactions(item *MessageItem) {
	if item == nil || s.chatStore == nil || item.ID == "" {
		return
	}
	rows, err := s.chatStore.GetReactions(item.ChatJID, item.ID)
	if err != nil {
		return
	}
	client := s.GetClient()
	ownJID := ""
	if client != nil && client.Store != nil && client.Store.ID != nil {
		ownJID = client.Store.ID.String()
	}
	byEmoji := make(map[string]*MessageReaction)
	for _, reaction := range rows {
		group := byEmoji[reaction.Emoji]
		if group == nil {
			group = &MessageReaction{Emoji: reaction.Emoji}
			byEmoji[reaction.Emoji] = group
		}
		group.Count++
		if reaction.ReactorJID == ownJID {
			group.FromMe = true
		}
	}
	item.Reactions = make([]MessageReaction, 0, len(byEmoji))
	for _, reaction := range byEmoji {
		item.Reactions = append(item.Reactions, *reaction)
	}
	sort.Slice(item.Reactions, func(i, j int) bool { return item.Reactions[i].Emoji < item.Reactions[j].Emoji })
}

func (s *WhatsAppService) refreshCachedReactions(chatJID, messageID string) []MessageReaction {
	var item MessageItem
	found := false
	s.chatMu.RLock()
	for _, cached := range s.messages[chatJID] {
		if cached.ID == messageID {
			item, found = cached, true
			break
		}
	}
	s.chatMu.RUnlock()
	if !found {
		return nil
	}
	s.attachReactions(&item)
	s.chatMu.Lock()
	for i := range s.messages[chatJID] {
		if s.messages[chatJID][i].ID == messageID {
			s.messages[chatJID][i].Reactions = item.Reactions
			break
		}
	}
	s.chatMu.Unlock()
	return item.Reactions
}

// addMessageToCache adds a message to the in-memory cache (thread-safe)
func (s *WhatsAppService) addMessageToCache(chatJID string, msg MessageItem) {
	// A revoke may arrive before the original history record. The database
	// tombstone wins so a delayed sync cannot visibly resurrect the message.
	if s.chatStore != nil && msg.ID != "" {
		if existing, err := s.chatStore.GetMessage(chatJID, msg.ID); err == nil && existing.IsDeleted {
			msg = messageItemFromRow(existing)
		}
	}
	s.chatMu.Lock()
	for i := range s.messages[chatJID] {
		if s.messages[chatJID][i].ID == msg.ID && msg.ID != "" {
			// History sync can replay a message already restored from Wamio's
			// cache. Keep the richer cached fields, but merge its receipt so the
			// replayed WebMessageInfo.Status is not discarded.
			if msg.IsFromMe {
				s.messages[chatJID][i].DeliveryStatus = advanceDeliveryStatus(s.messages[chatJID][i].DeliveryStatus, msg.DeliveryStatus)
			}
			if msg.IsRead {
				s.messages[chatJID][i].IsRead = true
			}
			s.chatMu.Unlock()
			if s.chatStore != nil {
				_ = s.chatStore.UpsertMessage(messageRowFromItem(msg))
			}
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
	s.chatMu.Unlock()

	// Persist to SQLite
	if s.chatStore != nil {
		_ = s.chatStore.UpsertMessage(messageRowFromItem(msg))
	}
}

// updateChatLastMessage updates only the in-memory preview. The caller
// persists and emits it after all related state (such as unread count) has
// been updated, which keeps frontend events in a consistent order.
func (s *WhatsAppService) updateChatLastMessage(chatJID string, msg MessageItem, isGroup bool) {
	s.chatMu.Lock()
	defer s.chatMu.Unlock()

	chat, ok := s.chats[chatJID]
	if !ok {
		// Create the chat entry if it doesn't exist yet
		chat = &ChatItem{
			JID:     chatJID,
			Name:    chatJID,
			IsGroup: isGroup,
		}
		s.chats[chatJID] = chat
	}

	if msg.Timestamp >= chat.LastMessageTime {
		chat.LastMessage = previewForMessage(msg, isGroup)
		chat.LastMessageTime = msg.Timestamp
		chat.LastMessageID = msg.ID
		chat.LastMessageOrder = 0
		chat.LastMessageFromMe = msg.IsFromMe
		if msg.IsFromMe {
			chat.LastMessageStatus = msg.DeliveryStatus
		} else {
			chat.LastMessageStatus = ""
		}
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
		s.backfillPreviewMetadata(&c)
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
			if name, resolved := s.lookupContactName(originalJID); resolved {
				c.Name = name
			} else if c.Name == "" {
				c.Name = s.getContactName(canonical)
			}
			_ = s.chatStore.UpsertChat(c)
			_ = s.chatStore.MergeChatCache(originalJID.String(), canonical.String())
		} else if !c.IsGroup {
			// Contact actions are already stored by whatsmeow. Refresh Wamio's
			// denormalized label at startup, but never overwrite a persisted name
			// with a temporary phone-number fallback.
			if currentName, resolved := s.lookupContactName(canonical); resolved && currentName != c.Name {
				c.Name = currentName
				_ = s.chatStore.UpsertChat(c)
			}
		}
		item := &ChatItem{
			JID:               c.JID,
			Name:              c.Name,
			LastMessage:       c.LastMessage,
			LastMessageTime:   c.LastMessageTime,
			UnreadCount:       c.UnreadCount,
			IsGroup:           c.IsGroup,
			LastMessageID:     c.LastMessageID,
			LastMessageOrder:  c.LastMessageOrder,
			LastReadAt:        c.LastReadAt,
			LastMessageFromMe: c.LastMessageFromMe,
			LastMessageStatus: c.LastMessageStatus,
			IsArchived:        c.IsArchived,
			IsPinned:          c.IsPinned,
			MutedUntil:        c.MutedUntil,
			IsMuted:           isMutedUntil(c.MutedUntil),
		}
		if ids, listErr := s.chatStore.GetChatListIDs(c.JID); listErr == nil {
			item.ListIDs = ids
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

// backfillPreviewMetadata upgrades cached previews created before the sidebar
// tracked the sender and delivery state separately. Only a message at least as
// new as the cached preview may replace it, so incomplete local history cannot
// make a conversation appear older.
func (s *WhatsAppService) backfillPreviewMetadata(chat *store.ChatRow) {
	if s.chatStore == nil || chat == nil {
		return
	}
	rows, err := s.chatStore.GetMessages(chat.JID, 1)
	if err != nil || len(rows) == 0 {
		return
	}
	latest := messageItemFromRow(rows[len(rows)-1])
	if latest.Timestamp < chat.LastMessageTime {
		return
	}
	chat.LastMessage = previewForMessage(latest, chat.IsGroup)
	chat.LastMessageTime = latest.Timestamp
	chat.LastMessageID = latest.ID
	chat.LastMessageFromMe = latest.IsFromMe
	if latest.IsFromMe {
		chat.LastMessageStatus = latest.DeliveryStatus
		if chat.LastMessageStatus == "" {
			chat.LastMessageStatus = "sent"
		}
	} else {
		chat.LastMessageStatus = ""
	}
	if err := s.chatStore.UpsertChat(*chat); err != nil {
		s.log.Warnf("Failed to backfill preview metadata for %s: %v", chat.JID, err)
	}
}

// hydrateArchiveStates copies the authoritative account-level archive setting
// into Wamio's cache. It only reads whatsmeow's local store, so it is safe to
// run before the websocket connection is established.
func (s *WhatsAppService) hydrateChatSettings() {
	client := s.client
	if client == nil || client.Store == nil || client.Store.ChatSettings == nil {
		return
	}

	updates := make([]store.ChatRow, 0)
	s.chatMu.Lock()
	for chatJID, chat := range s.chats {
		jid, err := types.ParseJID(chatJID)
		if err != nil {
			continue
		}
		settings, err := client.Store.ChatSettings.GetChatSettings(context.Background(), jid)
		if err != nil || !settings.Found {
			continue
		}
		chat.IsArchived = settings.Archived
		chat.IsPinned = settings.Pinned
		chat.MutedUntil = settings.MutedUntil.Unix()
		chat.IsMuted = isMutedUntil(chat.MutedUntil)
		chat.ArchiveKnown = true
		chat.PinKnown = true
		updates = append(updates, chatRowFromItem(*chat))
	}
	s.chatMu.Unlock()

	if s.chatStore != nil {
		for _, row := range updates {
			if err := s.chatStore.UpsertChat(row); err != nil {
				s.log.Warnf("Failed to persist chat settings for %s: %v", row.JID, err)
			}
		}
	}
}

func isMutedUntil(until int64) bool { return until < 0 || until > time.Now().Unix() }

func messageContextInfo(msg *waProto.Message) *waProto.ContextInfo {
	if msg == nil {
		return nil
	}
	if message := msg.GetExtendedTextMessage(); message != nil {
		return message.GetContextInfo()
	}
	if message := msg.GetImageMessage(); message != nil {
		return message.GetContextInfo()
	}
	if message := msg.GetVideoMessage(); message != nil {
		return message.GetContextInfo()
	}
	if message := msg.GetDocumentMessage(); message != nil {
		return message.GetContextInfo()
	}
	if message := msg.GetAudioMessage(); message != nil {
		return message.GetContextInfo()
	}
	return nil
}

func (s *WhatsAppService) replyReferenceFromMessage(info types.MessageInfo, msg *waProto.Message) *MessageReference {
	contextInfo := messageContextInfo(msg)
	if contextInfo == nil || contextInfo.GetStanzaID() == "" {
		return nil
	}
	chatJID := contextInfo.GetRemoteJID()
	if chatJID == "" {
		chatJID = s.canonicalChatJID(info.Chat).String()
	}
	senderJID := contextInfo.GetParticipant()
	content, mediaType := extractMessageContent(contextInfo.GetQuotedMessage())
	if content == "" {
		content = "Pesan"
	}
	reference := &MessageReference{ID: contextInfo.GetStanzaID(), ChatJID: chatJID, SenderJID: senderJID,
		Content: content, MediaType: mediaType}
	if senderJID != "" {
		if sender, err := types.ParseJID(senderJID); err == nil {
			reference.SenderName = s.getContactName(sender)
			client := s.GetClient()
			reference.IsFromMe = client != nil && client.Store != nil && client.Store.ID != nil && sender.User == client.Store.ID.User
		}
	}
	if reference.IsFromMe {
		reference.SenderName = "Anda"
	}
	return reference
}

func (s *WhatsAppService) processMessageMutation(v *events.Message) bool {
	return s.processMessageMutationWithEmit(v, true)
}

func (s *WhatsAppService) processMessageMutationWithEmit(v *events.Message, emit bool) bool {
	if v == nil || v.Message == nil {
		return false
	}
	if reaction := v.Message.GetReactionMessage(); reaction != nil && reaction.GetKey().GetID() != "" {
		chatJID := s.canonicalChatJID(v.Info.Chat).String()
		messageID := reaction.GetKey().GetID()
		timestamp := reaction.GetSenderTimestampMS()
		if timestamp == 0 {
			timestamp = v.Info.Timestamp.UnixMilli()
		}
		if s.chatStore != nil {
			_ = s.chatStore.UpsertReaction(store.ReactionRow{ChatJID: chatJID, MessageID: messageID,
				ReactorJID: v.Info.Sender.String(), Emoji: reaction.GetText(), Timestamp: timestamp})
		}
		if emit {
			reactions := s.refreshCachedReactions(chatJID, messageID)
			s.emitEvent("wa:message-reaction", MessageReactionEvent{ChatJID: chatJID, MessageID: messageID, Reactions: reactions})
		}
		return true
	}
	if protocol := v.Message.GetProtocolMessage(); protocol != nil && protocol.GetType() == waE2E.ProtocolMessage_REVOKE && protocol.GetKey().GetID() != "" {
		s.applyMessageDeleted(s.canonicalChatJID(v.Info.Chat).String(), protocol.GetKey().GetID(), false)
		return true
	}
	return false
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
			return caption, "image"
		}
		return "Foto", "image"
	}
	if msg.VideoMessage != nil {
		caption := ""
		if msg.VideoMessage.Caption != nil {
			caption = *msg.VideoMessage.Caption
		}
		if caption != "" {
			return caption, "video"
		}
		return "Video", "video"
	}
	if msg.AudioMessage != nil {
		if msg.AudioMessage.GetPTT() {
			return "Pesan suara", "audio"
		}
		return "Audio", "audio"
	}
	if msg.DocumentMessage != nil {
		fileName := "Dokumen"
		if msg.DocumentMessage.FileName != nil {
			fileName = *msg.DocumentMessage.FileName
		}
		return fileName, "document"
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

func mediaThumbnail(msg *waProto.Message) string {
	if msg == nil {
		return ""
	}
	var thumbnail []byte
	switch {
	case msg.GetImageMessage() != nil:
		thumbnail = msg.GetImageMessage().GetJPEGThumbnail()
	case msg.GetVideoMessage() != nil:
		thumbnail = msg.GetVideoMessage().GetJPEGThumbnail()
	case msg.GetDocumentMessage() != nil:
		thumbnail = msg.GetDocumentMessage().GetJPEGThumbnail()
	}
	if len(thumbnail) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(thumbnail)
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
		s.persistActiveAccount()
		s.emitEvent("wa:connection", ConnectionStatusEvent{
			State:   StateConnected,
			Message: "Terhubung",
		})
		go func(client *whatsmeow.Client) { _ = client.SendPresence(context.Background(), types.PresenceAvailable) }(s.GetClient())

		if s.awaitingInitialSync {
			s.startInitialSyncTimeout()
			return
		}

		// A reconnect deliberately renders the existing local cache immediately.
		s.loadChatsFromDB()
		s.emitEvent("wa:chats-sync", s.GetChats())
		s.initialSyncReady = true
		s.emitInitialSync("ready")
		s.emitSyncProgress("complete", 100, 0, 0, "")

	case *events.Disconnected:
		s.log.Infof("Disconnected from WhatsApp")
		s.mu.Lock()
		s.setState(StateDisconnected)
		s.mu.Unlock()
		s.invalidateInitialPreload()
		s.emitEvent("wa:connection", ConnectionStatusEvent{
			State:   StateDisconnected,
			Message: "Terputus",
		})

	case *events.LoggedOut:
		s.log.Infof("Logged out from WhatsApp: %v", v.Reason)
		s.mu.Lock()
		s.setState(StateLoggedOut)
		s.mu.Unlock()
		s.invalidateInitialPreload()
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
		if !v.JIDAlt.IsEmpty() {
			s.refreshChatName(v.JIDAlt)
		}

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

	case *events.Archive:
		s.handleArchive(v)

	case *events.Pin:
		s.handlePin(v)

	case *events.Mute:
		s.handleMute(v)

	case *events.LabelEdit:
		s.handleLabelEdit(v)

	case *events.LabelAssociationChat:
		s.handleLabelAssociationChat(v)

	case *events.DeleteForMe:
		if v != nil {
			s.applyMessageDeleted(s.canonicalChatJID(v.ChatJID).String(), v.MessageID, true)
		}

	case *events.Presence:
		s.handlePresence(v)

	case *events.ChatPresence:
		s.handleChatPresence(v)
	}
}

// handleIncomingMessage processes a real-time incoming message
func (s *WhatsAppService) handleIncomingMessage(v *events.Message) {
	if v == nil || v.Message == nil {
		return
	}
	if s.processMessageMutation(v) {
		return
	}
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
	var fileName, mimetype, caption string
	var fileSize uint64
	var isPTT bool

	if v.Message.GetImageMessage() != nil {
		mimetype = v.Message.GetImageMessage().GetMimetype()
		caption = v.Message.GetImageMessage().GetCaption()
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
		caption = v.Message.GetVideoMessage().GetCaption()
	} else if v.Message.GetDocumentMessage() != nil {
		mimetype = v.Message.GetDocumentMessage().GetMimetype()
		fileName = v.Message.GetDocumentMessage().GetFileName()
		caption, fileSize = v.Message.GetDocumentMessage().GetCaption(), v.Message.GetDocumentMessage().GetFileLength()
	}

	// Cache raw message proto for media download
	if mediaType != "" {
		s.mediaCache.StoreRawMessage(chatJID, info.ID, v.Message)
	}
	if sticker := v.Message.GetStickerMessage(); sticker != nil {
		s.rememberRecentSticker(sticker, info.Timestamp.Unix())
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
		Thumbnail:      mediaThumbnail(v.Message),
		IsPTT:          isPTT,
		DeliveryStatus: deliveryStatusForMessage(isFromMe),
		ReplyTo:        s.replyReferenceFromMessage(info, v.Message),
		IsForwarded:    messageContextInfo(v.Message) != nil && messageContextInfo(v.Message).GetIsForwarded(),
		Caption:        caption,
		FileSize:       fileSize,
	}

	// Add to cache
	s.addMessageToCache(chatJID, msgItem)

	s.updateChatLastMessage(chatJID, msgItem, isGroup)

	// Update unread count for non-own messages
	if !isFromMe {
		s.chatMu.Lock()
		if chat, ok := s.chats[chatJID]; ok {
			chat.UnreadCount++
		}
		s.chatMu.Unlock()
	}

	// Persist after the unread count has changed; the previous order left a
	// stale badge in SQLite after a relaunch.
	s.persistAndEmitChat(chatJID)

	// Emit the message after its unread state and preview have been persisted.
	// The frontend can then safely acknowledge an incoming message in the
	// active conversation without racing the unread counter.
	s.emitEvent("wa:message", MessageEvent{
		ChatJID: chatJID,
		Message: msgItem,
	})

	// Muted and archived chats intentionally never produce desktop alerts.
	if !isFromMe && s.isInitialSyncReady() && !s.shouldSuppressNotification(chatJID) {
		s.emitEvent("wa:notification", NotificationEvent{
			ChatJID:    chatJID,
			ChatName:   chatName,
			SenderName: senderName,
			Content:    content,
			IsGroup:    isGroup,
		})
	}
}

func (s *WhatsAppService) shouldSuppressNotification(chatJID string) bool {
	s.chatMu.RLock()
	defer s.chatMu.RUnlock()
	chat := s.chats[chatJID]
	return chat != nil && (chat.IsArchived || chat.IsMuted || isMutedUntil(chat.MutedUntil))
}

// handleHistorySync processes history sync events from WhatsApp
func (s *WhatsAppService) handleHistorySync(v *events.HistorySync) {
	data := v.Data
	if data == nil {
		return
	}
	syncType := data.GetSyncType()
	// Call history is independent of conversation history and can be supplied
	// with any sync kind, including FULL (which we intentionally do not cache).
	s.syncCallLogRecords(data.GetCallLogRecords())
	s.syncRecentStickers(data.GetRecentStickers())
	if syncType == waHistorySync.HistorySync_FULL {
		// FULL can be very large, so it must not seed Wamio's message cache.
		// It is nevertheless the only initial payload that reliably contains
		// archived/pinned state for every conversation after a new QR login.
		for _, conv := range data.GetConversations() {
			s.syncHistoryConversationMetadata(conv)
		}
		s.hydrateChatSettings()
		s.emitEvent("wa:chats-sync", s.GetChats())
		s.log.Infof("Synced metadata from FULL history (%d conversations)", len(data.GetConversations()))
		return
	}
	if syncType == waHistorySync.HistorySync_ON_DEMAND {
		for _, conv := range data.GetConversations() {
			s.processHistoryConversation(syncType, conv)
		}
		return
	}
	if syncType != waHistorySync.HistorySync_INITIAL_BOOTSTRAP && syncType != waHistorySync.HistorySync_RECENT {
		return
	}
	if !s.awaitingInitialSync {
		// The cache/prepared recent window is already usable. Real-time Message
		// events keep it current; processing late history here would restart a
		// broad stale synchronization pass.
		return
	}
	for _, conv := range data.GetConversations() {
		s.stageInitialConversation(conv)
	}
	s.emitSyncProgress("initial", int32(data.GetProgress()), s.initialProcessedChats, 0, "Menerima chat terbaru")
	s.scheduleInitialReady()
	if data.GetProgress() >= 100 {
		s.completeInitialSync("ready", "")
	}
}

func (s *WhatsAppService) syncCallLogRecords(records []*waSyncAction.CallLogRecord) {
	client := s.GetClient()
	if client == nil || client.Store == nil || client.Store.ID == nil {
		return
	}
	for _, record := range records {
		if record == nil || record.GetCallID() == "" || record.GetStartTime() <= 0 {
			continue
		}
		target := record.GetGroupJID()
		if target == "" {
			for _, participant := range record.GetParticipants() {
				if participant.GetUserJID() != "" && participant.GetUserJID() != client.Store.ID.String() {
					target = participant.GetUserJID()
					break
				}
			}
		}
		if target == "" || target == client.Store.ID.String() {
			continue
		}
		jid, err := types.ParseJID(target)
		if err != nil {
			continue
		}
		jid = s.canonicalChatJID(jid)
		s.ensureChatExists(jid, s.getContactName(jid), jid.Server == types.GroupServer)
		outcome := record.GetCallResult().String()
		item := MessageItem{ID: "call:" + record.GetCallID(), ChatJID: jid.String(), SenderJID: client.Store.ID.String(), SenderName: "Anda", Timestamp: record.GetStartTime(), IsFromMe: !record.GetIsIncoming(), IsRead: true, Kind: "call", Content: "Panggilan suara", Call: &CallInfo{CallID: record.GetCallID(), Outcome: outcome, Duration: record.GetDuration(), IsVideo: record.GetIsVideo(), IsIncoming: record.GetIsIncoming()}}
		if record.GetIsVideo() {
			item.Content = "Panggilan video"
		}
		s.addMessageToCache(jid.String(), item)
		s.updateChatLastMessage(jid.String(), item, jid.Server == types.GroupServer)
		s.persistAndEmitChat(jid.String())
	}
	if len(records) > 0 {
		s.emitEvent("wa:call-log", s.GetCallLog())
	}
}

func conversationNewestTimestamp(conv *waHistorySync.Conversation) int64 {
	if conv == nil {
		return 0
	}
	latest := int64(conv.GetConversationTimestamp())
	for _, item := range conv.GetMessages() {
		if item != nil && item.GetMessage() != nil {
			if ts := int64(item.GetMessage().GetMessageTimestamp()); ts > latest {
				latest = ts
			}
		}
	}
	return latest
}

// stageInitialConversation records metadata references until the short
// recent-first window closes. The newest 30 are then materialized in bounded
// batches rather than eagerly persisting every conversation's messages.
func (s *WhatsAppService) stageInitialConversation(conv *waHistorySync.Conversation) {
	if conv == nil || conv.GetID() == "" {
		return
	}
	jid, err := types.ParseJID(conv.GetID())
	if err != nil || isExcludedHistoryConversation(jid, conv) {
		return
	}
	key := s.canonicalChatJID(jid).String()
	s.initialMu.Lock()
	defer s.initialMu.Unlock()
	if previous, ok := s.initialConversations[key]; !ok {
		s.initialConversations[key] = conv
		s.initialProcessedChats++
	} else if conversationNewestTimestamp(conv) >= conversationNewestTimestamp(previous) {
		s.initialConversations[key] = conv
	}
}

func (s *WhatsAppService) scheduleInitialReady() {
	if s.initialReadyTimer != nil {
		s.initialReadyTimer.Stop()
	}
	s.initialReadyTimer = time.AfterFunc(1200*time.Millisecond, func() { s.completeInitialSync("ready", "") })
}

func (s *WhatsAppService) completeInitialSync(state, message string) {
	if !s.awaitingInitialSync {
		return
	}
	s.awaitingInitialSync = false
	s.initialSyncReady = true
	if s.initialSyncTimer != nil {
		s.initialSyncTimer.Stop()
		s.initialSyncTimer = nil
	}
	if s.initialReadyTimer != nil {
		s.initialReadyTimer.Stop()
		s.initialReadyTimer = nil
	}
	s.initialMu.Lock()
	conversations := make([]*waHistorySync.Conversation, 0, len(s.initialConversations))
	for _, conv := range s.initialConversations {
		conversations = append(conversations, conv)
	}
	generation := s.initialPreloadGeneration
	s.initialMu.Unlock()
	sort.Slice(conversations, func(i, j int) bool {
		return conversationNewestTimestamp(conversations[i]) > conversationNewestTimestamp(conversations[j])
	})
	// Preserve every chat's state before reducing the expensive message import
	// to the small recent-first window.
	for _, conv := range conversations {
		s.syncHistoryConversationMetadata(conv)
	}
	foreground, background := splitInitialPreload(conversations)
	for _, conv := range foreground {
		s.materializeInitialConversationPayloadForGeneration(conv, generation)
	}
	// App-state mutations are stored by whatsmeow independently of history.
	// Run after chats exist so archive/pin mutations received during login are
	// applied even if their event preceded the corresponding conversation.
	s.hydrateChatSettings()
	s.emitEvent("wa:chats-sync", s.GetChats())
	s.scheduleContactNameReconciliation()
	s.emitInitialSync(state, message)

	prepared, target := len(foreground), len(foreground)+len(background)
	if len(background) == 0 {
		phase, progress := "complete", int32(100)
		if state == "degraded" {
			phase, progress = "degraded", -1
		}
		s.emitSyncProgress(phase, progress, prepared, target, message)
		return
	}

	s.emitSyncProgress("background", preloadProgress(prepared, target), prepared, target, "Menyiapkan chat terbaru di latar belakang")
	go s.preloadInitialConversations(generation, background, prepared, target, state, message)
}

// splitInitialPreload limits startup persistence to the 30 most recent chats.
// The first five are prepared before rendering; the remainder are processed
// in background batches so opening the sidebar remains responsive.
func splitInitialPreload(conversations []*waHistorySync.Conversation) (foreground, background []*waHistorySync.Conversation) {
	if len(conversations) > initialPreloadLimit {
		conversations = conversations[:initialPreloadLimit]
	}
	foregroundCount := initialForegroundChats
	if foregroundCount > len(conversations) {
		foregroundCount = len(conversations)
	}
	return conversations[:foregroundCount], conversations[foregroundCount:]
}

func preloadProgress(prepared, target int) int32 {
	if target <= 0 {
		return 100
	}
	return int32(prepared * 100 / target)
}

func (s *WhatsAppService) preloadInitialConversations(generation uint64, conversations []*waHistorySync.Conversation, prepared, target int, initialState, initialMessage string) {
	for start := 0; start < len(conversations); start += initialPreloadBatch {
		if !s.initialPreloadCurrent(generation) {
			return
		}
		end := start + initialPreloadBatch
		if end > len(conversations) {
			end = len(conversations)
		}
		for _, conversation := range conversations[start:end] {
			if !s.initialPreloadCurrent(generation) {
				return
			}
			s.materializeInitialConversationPayloadForGeneration(conversation, generation)
		}
		prepared += end - start
		s.emitEvent("wa:chats-sync", s.GetChats())
		phase := "background"
		progress := preloadProgress(prepared, target)
		if prepared >= target {
			phase, progress = "complete", 100
			if initialState == "degraded" {
				phase, progress = "degraded", -1
			}
		}
		statusMessage := "Menyiapkan chat terbaru di latar belakang"
		if phase != "background" {
			statusMessage = initialMessage
		}
		s.emitSyncProgress(phase, progress, prepared, target, statusMessage)
	}
}

func (s *WhatsAppService) initialPreloadCurrent(generation uint64) bool {
	s.initialMu.Lock()
	current := s.initialPreloadGeneration == generation
	s.initialMu.Unlock()
	if !current {
		return false
	}
	s.mu.RLock()
	connected := s.state == StateConnected && s.client != nil
	s.mu.RUnlock()
	return connected
}

func (s *WhatsAppService) isInitialSyncReady() bool { return s.initialSyncReady }

func (s *WhatsAppService) emitSyncProgress(phase string, progress int32, processed, prepared int, message string) {
	s.emitEvent("wa:sync-progress", SyncProgressEvent{Phase: phase, Progress: progress, ProcessedChats: processed, PreparedChats: prepared, Message: message})
}

func (s *WhatsAppService) handlePresence(v *events.Presence) {
	if v == nil {
		return
	}
	jid := s.canonicalChatJID(v.From).String()
	s.emitEvent("wa:presence", PresenceEvent{ChatJID: jid, Online: !v.Unavailable, LastSeen: v.LastSeen.Unix(), Unavailable: v.Unavailable})
}

func (s *WhatsAppService) handleChatPresence(v *events.ChatPresence) {
	if v == nil || v.IsFromMe {
		return
	}
	jid := s.canonicalChatJID(v.Chat).String()
	s.emitEvent("wa:chat-presence", ChatPresenceEvent{ChatJID: jid, Typing: v.State == types.ChatPresenceComposing})
}

// handleArchive keeps archive labels aligned with app-state updates made from
// the phone or another linked device.
func (s *WhatsAppService) handleArchive(v *events.Archive) {
	if v == nil || v.Action == nil {
		return
	}
	jid := s.canonicalChatJID(v.JID)
	if s.isExcludedChat(jid) {
		return
	}
	// App-state can arrive before its history conversation. Keep the state on
	// a lightweight placeholder instead of silently dropping it.
	s.ensureChatExists(jid, s.getContactName(jid), jid.Server == types.GroupServer)
	chatJID := jid.String()
	s.chatMu.Lock()
	chat := s.chats[chatJID]
	chat.IsArchived = v.Action.GetArchived()
	chat.ArchiveKnown = true
	s.chatMu.Unlock()
	s.persistAndEmitChat(chatJID)
}

func (s *WhatsAppService) handlePin(v *events.Pin) {
	if v == nil || v.Action == nil {
		return
	}
	jid := s.canonicalChatJID(v.JID)
	if s.isExcludedChat(jid) {
		return
	}
	// See handleArchive: app-state precedes history on some fresh pairings.
	s.ensureChatExists(jid, s.getContactName(jid), jid.Server == types.GroupServer)
	chatJID := jid.String()
	s.chatMu.Lock()
	chat := s.chats[chatJID]
	chat.IsPinned = v.Action.GetPinned()
	chat.PinKnown = true
	s.chatMu.Unlock()
	s.persistAndEmitChat(chatJID)
}

func (s *WhatsAppService) handleMute(v *events.Mute) {
	if v == nil || v.Action == nil {
		return
	}
	chatJID := s.canonicalChatJID(v.JID).String()
	updated := false
	s.chatMu.Lock()
	if chat, ok := s.chats[chatJID]; ok {
		if v.Action.GetMuted() {
			chat.MutedUntil = v.Action.GetMuteEndTimestamp()
		} else {
			chat.MutedUntil = 0
		}
		chat.IsMuted = isMutedUntil(chat.MutedUntil)
		updated = true
	}
	s.chatMu.Unlock()
	if updated {
		s.persistAndEmitChat(chatJID)
	}
}

func (s *WhatsAppService) handleLabelEdit(v *events.LabelEdit) {
	if v == nil || v.Action == nil || v.LabelID == "" {
		return
	}
	if s.chatStore != nil {
		_ = s.chatStore.UpsertChatList(store.ChatListRow{ID: v.LabelID, Name: v.Action.GetName(), Type: v.Action.GetType().String(), Order: v.Action.GetOrderIndex(), IsActive: v.Action.GetIsActive() && !v.Action.GetDeleted()})
	}
	s.emitEvent("wa:chat-lists", s.GetChatLists())
}

func (s *WhatsAppService) handleLabelAssociationChat(v *events.LabelAssociationChat) {
	if v == nil || v.Action == nil || v.LabelID == "" {
		return
	}
	chatJID := s.canonicalChatJID(v.JID).String()
	if s.chatStore != nil {
		_ = s.chatStore.SetChatListMembership(v.LabelID, chatJID, v.Action.GetLabeled())
	}
	s.chatMu.Lock()
	if chat, ok := s.chats[chatJID]; ok {
		ids := make([]string, 0, len(chat.ListIDs)+1)
		for _, id := range chat.ListIDs {
			if id != v.LabelID {
				ids = append(ids, id)
			}
		}
		if v.Action.GetLabeled() {
			ids = append(ids, v.LabelID)
		}
		chat.ListIDs = ids
	}
	s.chatMu.Unlock()
	s.persistAndEmitChat(chatJID)
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
	// INITIAL/RECENT conversations carry the community flags needed for local
	// filtering. Avoid GroupInfo lookups here: this function is also used by
	// lazy opening and must not turn one click into an unbounded network wait.
	// ON_DEMAND payloads may omit those flags, so retain the runtime check there.
	if isExcludedHistoryConversation(sourceJID, conv) ||
		(syncType == waHistorySync.HistorySync_ON_DEMAND && s.isExcludedChat(sourceJID)) {
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
	mutations := make([]*events.Message, 0)
	for _, historyMsg := range conv.GetMessages() {
		if historyMsg == nil || historyMsg.GetMessage() == nil {
			continue
		}
		parsed, parseErr := client.ParseWebMessage(sourceJID, historyMsg.GetMessage())
		if parseErr != nil {
			s.log.Debugf("Skipping malformed history message in %s: %v", chatJID, parseErr)
			continue
		}
		if parsed.Message.GetReactionMessage() != nil || parsed.Message.GetProtocolMessage() != nil {
			mutations = append(mutations, parsed)
			continue
		}
		item, ok := s.messageItemFromEvent(parsed)
		if !ok {
			continue
		}
		item.DeliveryStatus = deliveryStatusForHistoryMessage(item.IsFromMe, historyMsg.GetMessage().GetStatus())
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
	applyHistoryReadState(items, int(conv.GetUnreadCount()))

	for _, entry := range items {
		s.addMessageToCache(chatJID, entry.item)
	}
	// History often carries a reaction before its target message. Persist normal
	// messages first, then apply all mutations and rehydrate each reaction set.
	for _, mutation := range mutations {
		s.processMessageMutationWithEmit(mutation, false)
	}
	for i := range items {
		items[i].item.Reactions = s.refreshCachedReactions(chatJID, items[i].item.ID)
	}
	if syncType != waHistorySync.HistorySync_ON_DEMAND && s.chatStore != nil {
		// The initial cache is intentionally small. Older messages are persisted
		// only after an explicit scroll-triggered request.
		_ = s.chatStore.TrimMessages(chatJID, initialPreloadMessages)
		s.reloadMessagesFromDB(chatJID, initialPreloadMessages)
	}

	var newest *orderedHistoryMessage
	if len(items) > 0 {
		candidate := items[len(items)-1]
		newest = &candidate
	}
	s.upsertHistoryChat(jid, sourceJID, conv, newest)

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

// HistorySync carries an unread counter for the conversation, not an is-read
// bit per incoming message. In a chronological recent window, WhatsApp's
// unread messages form the newest incoming suffix. Reconstruct that boundary
// so messages already read on the phone do not appear unread on desktop.
func applyHistoryReadState(items []orderedHistoryMessage, unreadCount int) {
	if unreadCount < 0 {
		unreadCount = 0
	}
	remainingUnread := unreadCount
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].item.IsFromMe {
			items[i].item.IsRead = true
			continue
		}
		if remainingUnread > 0 {
			items[i].item.IsRead = false
			remainingUnread--
		} else {
			items[i].item.IsRead = true
		}
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
		DeliveryStatus: deliveryStatusForMessage(info.IsFromMe), ReplyTo: s.replyReferenceFromMessage(info, v.Message),
		IsForwarded: messageContextInfo(v.Message) != nil && messageContextInfo(v.Message).GetIsForwarded()}
	if image := v.Message.GetImageMessage(); image != nil {
		item.Mimetype, item.Caption = image.GetMimetype(), image.GetCaption()
	} else if audio := v.Message.GetAudioMessage(); audio != nil {
		item.Mimetype, item.MediaDuration, item.IsPTT = audio.GetMimetype(), audio.GetSeconds(), audio.GetPTT()
	} else if video := v.Message.GetVideoMessage(); video != nil {
		item.Mimetype, item.MediaDuration, item.Caption = video.GetMimetype(), video.GetSeconds(), video.GetCaption()
	} else if document := v.Message.GetDocumentMessage(); document != nil {
		item.Mimetype, item.FileName, item.Caption, item.FileSize = document.GetMimetype(), document.GetFileName(), document.GetCaption(), document.GetFileLength()
	}
	item.Thumbnail = mediaThumbnail(v.Message)
	if mediaType != "" {
		s.mediaCache.StoreRawMessage(s.canonicalChatJID(info.Chat).String(), info.ID, v.Message)
	}
	if sticker := v.Message.GetStickerMessage(); sticker != nil {
		s.rememberRecentSticker(sticker, info.Timestamp.Unix())
	}
	return item, true
}

func (s *WhatsAppService) upsertHistoryChat(jid, sourceJID types.JID, conv *waHistorySync.Conversation, newest *orderedHistoryMessage) {
	chatJID := jid.String()
	historyName := historyConversationName(conv)
	if newest == nil {
		newest = latestHistoryPreview(conv, jid.Server == types.GroupServer)
	}
	s.chatMu.Lock()
	chat, exists := s.chats[chatJID]
	if !exists {
		name := historyName
		if name == "" {
			if jid.Server == types.GroupServer {
				// A group name is normally carried by Conversation.Name. Do not
				// issue a network request per group during initial sync when it is
				// temporarily absent.
				name = "Grup WhatsApp"
			} else {
				// Keep the original LID available for contact lookup while storing
				// the canonical PN as the chat key.
				name = s.getContactName(sourceJID)
			}
		}
		chat = &ChatItem{JID: chatJID, Name: name, IsGroup: jid.Server == types.GroupServer}
		s.chats[chatJID] = chat
	} else if historyName != "" && (chat.IsGroup || chat.Name == "" || chat.Name == "+"+sourceJID.User || chat.Name == sourceJID.String()) {
		// History carries a display name before GroupInfo events are guaranteed
		// to arrive. Replace only an unresolved fallback, never a local contact
		// name for a personal chat.
		chat.Name = historyName
	}
	if !chat.ArchiveKnown && conv.Archived != nil {
		chat.IsArchived = conv.GetArchived()
	}
	if !chat.PinKnown && conv.Pinned != nil {
		chat.IsPinned = conv.GetPinned() != 0
	}
	// Never restore a badge from an older history chunk after this chat was
	// already read on another linked device.
	if int64(conv.GetConversationTimestamp()) > chat.LastReadAt {
		chat.UnreadCount = int(conv.GetUnreadCount())
	} else {
		chat.UnreadCount = 0
	}
	if newest != nil && previewIsNewer(newest.item.Timestamp, newest.order, newest.item.ID, chat) {
		preview := previewForMessage(newest.item, chat.IsGroup)
		chat.LastMessage, chat.LastMessageTime = preview, newest.item.Timestamp
		chat.LastMessageID, chat.LastMessageOrder = newest.item.ID, newest.order
		chat.LastMessageFromMe = newest.item.IsFromMe
		if newest.item.IsFromMe {
			chat.LastMessageStatus = newest.item.DeliveryStatus
		} else {
			chat.LastMessageStatus = ""
		}
	} else if chat.LastMessageTime == 0 && conv.GetConversationTimestamp() > 0 {
		// The history payload may contain only protocol messages, which do not
		// have a safe user-facing preview. Keep its order in the sidebar but make
		// the absence explicit rather than rendering a blank line.
		chat.LastMessage = "Pesan"
		chat.LastMessageTime = int64(conv.GetConversationTimestamp())
	}
	row := store.ChatRow{JID: chat.JID, Name: chat.Name, LastMessage: chat.LastMessage, LastMessageTime: chat.LastMessageTime,
		UnreadCount: chat.UnreadCount, IsGroup: chat.IsGroup, LastMessageID: chat.LastMessageID, LastMessageOrder: chat.LastMessageOrder,
		LastReadAt: chat.LastReadAt, LastMessageFromMe: chat.LastMessageFromMe, LastMessageStatus: chat.LastMessageStatus,
		IsArchived: chat.IsArchived, IsPinned: chat.IsPinned, MutedUntil: chat.MutedUntil}
	s.chatMu.Unlock()
	if s.chatStore != nil {
		_ = s.chatStore.UpsertChat(row)
	}
}

// syncHistoryConversationMetadata keeps the chat list complete without
// retaining the potentially huge message payload attached to FULL history.
func (s *WhatsAppService) syncHistoryConversationMetadata(conv *waHistorySync.Conversation) {
	if conv == nil || conv.GetID() == "" {
		return
	}
	sourceJID, err := types.ParseJID(conv.GetID())
	if err != nil || isExcludedHistoryConversation(sourceJID, conv) {
		return
	}
	s.upsertHistoryChat(s.canonicalChatJID(sourceJID), sourceJID, conv, nil)
}

// historyConversationName uses the label included in the history payload.
// In particular, Conversation.Name is the group subject. Resolving every
// group through GetGroupInfo here would make a new login perform a serial
// network request for every group before the inbox can be displayed.
func historyConversationName(conv *waHistorySync.Conversation) string {
	if conv == nil {
		return ""
	}
	if name := strings.TrimSpace(conv.GetName()); name != "" {
		return name
	}
	return strings.TrimSpace(conv.GetDisplayName())
}

// latestHistoryPreview reads only the already-decrypted payload in a history
// conversation. Unlike ParseWebMessage, it does not resolve contacts, write
// storage, or make a network request; it is therefore safe for all metadata
// rows in the initial sidebar.
func latestHistoryPreview(conv *waHistorySync.Conversation, isGroup bool) *orderedHistoryMessage {
	if conv == nil {
		return nil
	}
	var newest *orderedHistoryMessage
	for _, historyMsg := range conv.GetMessages() {
		if historyMsg == nil || historyMsg.GetMessage() == nil {
			continue
		}
		messageInfo := historyMsg.GetMessage()
		content, mediaType := extractMessageContent(messageInfo.GetMessage())
		if content == "" {
			continue
		}
		item := MessageItem{
			ID:         messageInfo.GetKey().GetID(),
			Content:    content,
			Timestamp:  int64(messageInfo.GetMessageTimestamp()),
			IsFromMe:   messageInfo.GetKey().GetFromMe(),
			MediaType:  mediaType,
			SenderName: messageInfo.GetPushName(),
		}
		item.DeliveryStatus = deliveryStatusForHistoryMessage(item.IsFromMe, messageInfo.GetStatus())
		candidate := orderedHistoryMessage{item: item, order: historyMsg.GetMsgOrderID()}
		if newest == nil || previewIsNewer(candidate.item.Timestamp, candidate.order, candidate.item.ID, &ChatItem{
			LastMessageTime:  newest.item.Timestamp,
			LastMessageOrder: newest.order,
			LastMessageID:    newest.item.ID,
		}) {
			copy := candidate
			newest = &copy
		}
	}
	if newest != nil {
		newest.item.Content = previewForMessage(newest.item, isGroup)
		newest.item.MediaType = ""
	}
	return newest
}

// isExcludedHistoryConversation keeps history processing local and bounded.
// The history payload already identifies community parent groups and default
// read-only announcement subgroups, so querying GroupInfo is unnecessary at
// this stage. Runtime group events still use isExcludedChat for validation.
func isExcludedHistoryConversation(jid types.JID, conv *waHistorySync.Conversation) bool {
	if jid == types.StatusBroadcastJID || jid.Server == types.NewsletterServer {
		return true
	}
	return jid.Server == types.GroupServer && conv != nil &&
		(conv.GetIsParentGroup() || (conv.GetIsDefaultSubgroup() && conv.GetReadOnly()))
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
		item := messageItemFromRow(row)
		s.attachReactions(&item)
		items = append(items, item)
	}
	s.chatMu.Lock()
	s.messages[chatJID] = items
	s.chatMu.Unlock()
}

func messageItemFromRow(m store.MessageRow) MessageItem {
	item := MessageItem{ID: m.ID, ChatJID: m.ChatJID, SenderJID: m.SenderJID, SenderName: m.SenderName,
		Content: m.Content, Timestamp: m.Timestamp, IsFromMe: m.IsFromMe, IsRead: m.IsRead,
		MediaType: m.MediaType, MediaDuration: m.MediaDuration, FileName: m.FileName,
		Mimetype: m.Mimetype, Thumbnail: m.Thumbnail, IsPTT: m.IsPTT, DeliveryStatus: m.DeliveryStatus,
		IsForwarded: m.IsForwarded, IsDeleted: m.IsDeleted, Caption: m.Caption, FileSize: m.FileSize, Kind: m.Kind}
	if m.CallID != "" {
		item.Call = &CallInfo{CallID: m.CallID, Outcome: m.CallOutcome, Duration: m.CallDuration, IsVideo: m.CallIsVideo, IsIncoming: m.CallIsIncoming}
	}
	if m.ReplyID != "" {
		item.ReplyTo = &MessageReference{ID: m.ReplyID, ChatJID: m.ReplyChatJID, SenderJID: m.ReplySenderJID,
			SenderName: m.ReplySenderName, Content: m.ReplyContent, MediaType: m.ReplyMediaType,
			IsFromMe: m.ReplyFromMe}
		if m.ReplyIsDeleted {
			item.ReplyTo.Content = "Pesan ini telah dihapus"
		}
	}
	return item
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
			s.attachReactions(&msgs[i])
		}
		chat := ChatItem{
			JID:               c.JID,
			Name:              c.Name,
			LastMessage:       c.LastMessage,
			LastMessageTime:   c.LastMessageTime,
			UnreadCount:       c.UnreadCount,
			IsGroup:           c.IsGroup,
			LastMessageFromMe: c.LastMessageFromMe,
			LastMessageStatus: c.LastMessageStatus,
			IsArchived:        c.IsArchived,
			IsPinned:          c.IsPinned,
			MutedUntil:        c.MutedUntil,
			IsMuted:           isMutedUntil(c.MutedUntil),
		}
		if ids, listErr := s.chatStore.GetChatListIDs(c.JID); listErr == nil {
			chat.ListIDs = ids
		}
		result = append(result, ChatWithMessages{Chat: chat, Messages: msgs})
	}
	return result
}

// OpenChat subscribes to presence and acknowledges every locally unread
// incoming message. The local badge is only changed after WhatsApp accepts
// the receipt, so a transient connection error cannot pretend a message was
// read on the phone.
func (s *WhatsAppService) OpenChat(chatJID string) error {
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("invalid chat JID: %w", err)
	}
	jid = s.canonicalChatJID(jid)
	client := s.GetClient()
	if client == nil || !client.IsConnected() {
		return fmt.Errorf("not connected")
	}
	if jid.Server == types.DefaultUserServer || jid.Server == types.HiddenUserServer {
		if err := client.SubscribePresence(context.Background(), jid); err != nil {
			s.emitEvent("wa:presence", PresenceEvent{ChatJID: jid.String(), Unavailable: true})
		}
	}
	if s.chatStore == nil {
		return nil
	}
	rows, err := s.chatStore.GetUnreadIncomingMessages(jid.String())
	if err != nil || len(rows) == 0 {
		return err
	}
	bySender := make(map[string][]store.MessageRow)
	for _, row := range rows {
		bySender[row.SenderJID] = append(bySender[row.SenderJID], row)
	}
	acknowledged := make([]string, 0, len(rows))
	var readAt int64
	for senderText, messages := range bySender {
		sender, parseErr := types.ParseJID(senderText)
		if parseErr != nil {
			return fmt.Errorf("invalid message sender: %w", parseErr)
		}
		ids := make([]types.MessageID, 0, len(messages))
		latest := int64(0)
		for _, row := range messages {
			ids = append(ids, types.MessageID(row.ID))
			acknowledged = append(acknowledged, row.ID)
			if row.Timestamp > latest {
				latest = row.Timestamp
			}
		}
		if err := client.MarkRead(context.Background(), ids, time.Unix(latest, 0), jid, sender); err != nil {
			return fmt.Errorf("failed to mark message read: %w", err)
		}
		if latest > readAt {
			readAt = latest
		}
	}
	s.applyIncomingRead(jid.String(), acknowledged, readAt)
	return nil
}

// MarkChatRead remains as a backwards-compatible Wails binding. New clients
// should use OpenChat so errors can be surfaced to the UI.
func (s *WhatsAppService) MarkChatRead(chatJID string) {
	if err := s.OpenChat(chatJID); err != nil {
		s.log.Warnf("Failed to mark %s read: %v", chatJID, err)
	}
}

func (s *WhatsAppService) applyIncomingRead(chatJID string, ids []string, readAt int64) {
	if readAt <= 0 {
		readAt = time.Now().Unix()
	}
	if s.chatStore != nil {
		if err := s.chatStore.MarkIncomingMessagesRead(chatJID, ids, readAt); err != nil {
			s.log.Warnf("Failed to persist read state for %s: %v", chatJID, err)
			return
		}
	}
	s.chatMu.Lock()
	for i := range s.messages[chatJID] {
		if !s.messages[chatJID][i].IsFromMe && containsMessageID(ids, s.messages[chatJID][i].ID) {
			s.messages[chatJID][i].IsRead = true
		}
	}
	var updated *ChatItem
	if chat, ok := s.chats[chatJID]; ok {
		chat.UnreadCount = 0
		if readAt > chat.LastReadAt {
			chat.LastReadAt = readAt
		}
		copy := *chat
		updated = &copy
	}
	s.chatMu.Unlock()
	if updated != nil {
		s.emitEvent("wa:chat-update", ChatUpdateEvent{Chat: *updated})
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

// GetChatProfile fetches the small set of information that WhatsApp exposes
// reliably for a contact or group. Missing avatars are intentionally nonfatal.
func (s *WhatsAppService) GetChatProfile(chatJID string) (ChatProfile, error) {
	client := s.GetClient()
	if client == nil || !client.IsConnected() {
		return ChatProfile{}, fmt.Errorf("not connected")
	}
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return ChatProfile{}, fmt.Errorf("invalid JID: %w", err)
	}
	jid = s.canonicalChatJID(jid)
	profile := ChatProfile{JID: jid.String(), Name: s.getContactName(jid), IsGroup: jid.Server == types.GroupServer}
	if profile.Name == "" {
		profile.Name = jid.User
	}
	if avatar, avatarErr := s.GetProfilePicture(jid.String()); avatarErr == nil {
		profile.Avatar = avatar
	}
	if profile.IsGroup {
		group, groupErr := client.GetGroupInfo(context.Background(), jid)
		if groupErr != nil {
			return profile, nil
		}
		if group.Name != "" {
			profile.Name = group.Name
		}
		profile.Description = group.Topic
		profile.ParticipantCount = group.ParticipantCount
		return profile, nil
	}
	if contact, contactErr := client.Store.Contacts.GetContact(context.Background(), jid); contactErr == nil && contact.RedactedPhone != "" {
		profile.PhoneNumber = contact.RedactedPhone
	} else if jid.Server == types.DefaultUserServer {
		profile.PhoneNumber = "+" + jid.User
	}
	return profile, nil
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
		if len(messages) == 0 && s.materializeInitialConversation(chatJID) {
			messages = s.GetMessages(chatJID, limit)
		}
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
		s.attachReactions(&result[i])
	}
	return MessagePage{Messages: result, HasMoreLocal: len(result) >= limit, CanRequestOlder: s.canRequestOlder(chatJID)}
}

// materializeInitialConversation lazily imports the bounded recent payload for
// a chat that was listed during a fresh pairing but was not among the initial
// preload set. This keeps startup fast while ensuring a selectable chat does
// not open as an unrecoverable blank view.
func (s *WhatsAppService) materializeInitialConversation(chatJID string) bool {
	s.initialMu.Lock()
	conversation := s.initialConversations[chatJID]
	s.initialMu.Unlock()
	if conversation == nil {
		conversation = s.findInitialConversationAlias(chatJID)
		if conversation != nil {
			// Keep the canonical key too. History can be received under a LID
			// before its LID-to-PN mapping is available, while the sidebar is
			// later keyed by the PN. Without this alias, the preview is visible
			// but opening the conversation finds no payload to materialize.
			s.initialMu.Lock()
			s.initialConversations[chatJID] = conversation
			s.initialMu.Unlock()
		}
	}
	if conversation == nil {
		return false
	}
	return s.materializeInitialConversationPayload(conversation)
}

// materializeInitialConversationPayload coordinates foreground, background,
// and click-triggered imports. A selected chat waits for an in-flight batch
// instead of observing an empty page, and each canonical conversation is
// parsed only once.
func (s *WhatsAppService) materializeInitialConversationPayload(conversation *waHistorySync.Conversation) bool {
	s.initialMu.Lock()
	generation := s.initialPreloadGeneration
	s.initialMu.Unlock()
	return s.materializeInitialConversationPayloadForGeneration(conversation, generation)
}

func (s *WhatsAppService) materializeInitialConversationPayloadForGeneration(conversation *waHistorySync.Conversation, generation uint64) bool {
	if conversation == nil || conversation.GetID() == "" {
		return false
	}
	jid, err := types.ParseJID(conversation.GetID())
	if err != nil {
		return false
	}
	key := s.canonicalChatJID(jid).String()

	s.initialMu.Lock()
	if s.initialPreloadGeneration != generation {
		s.initialMu.Unlock()
		return false
	}
	if s.initialMaterialized[key] {
		s.initialMu.Unlock()
		return true
	}
	if done := s.initialMaterializing[key]; done != nil {
		s.initialMu.Unlock()
		<-done
		return true
	}
	done := make(chan struct{})
	s.initialMaterializing[key] = done
	s.initialMu.Unlock()

	s.processHistoryConversation(waHistorySync.HistorySync_RECENT, conversation)

	s.initialMu.Lock()
	if s.initialPreloadGeneration == generation {
		s.initialMaterialized[key] = true
		delete(s.initialMaterializing, key)
	}
	close(done)
	s.initialMu.Unlock()
	return true
}

// findInitialConversationAlias resolves an initial payload by identity rather
// than its historical map key. WhatsApp may send the same individual chat as
// a LID in the history stream and as a phone-number JID after contact sync.
func (s *WhatsAppService) findInitialConversationAlias(chatJID string) *waHistorySync.Conversation {
	target, err := types.ParseJID(chatJID)
	if err != nil {
		return nil
	}
	canonicalTarget := s.canonicalChatJID(target)

	s.initialMu.Lock()
	conversations := make([]*waHistorySync.Conversation, 0, len(s.initialConversations))
	for _, candidate := range s.initialConversations {
		conversations = append(conversations, candidate)
	}
	s.initialMu.Unlock()

	for _, candidate := range conversations {
		if candidate == nil || candidate.GetID() == "" {
			continue
		}
		source, parseErr := types.ParseJID(candidate.GetID())
		if parseErr != nil {
			continue
		}
		if source == target || s.canonicalChatJID(source) == canonicalTarget {
			return candidate
		}
	}
	return nil
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
		// Notifications can arrive before history chunks. Keep the linked session
		// usable and surface a recoverable status instead of falsely demanding a
		// relink.
		s.completeInitialSync("degraded", "Riwayat terbaru belum selesai. Pesan baru tetap akan masuk.")
	})
}

func (s *WhatsAppService) finishInitialSync() {
	s.completeInitialSync("ready", "")
}

// Utility: get current time helper (for testing)
func now() time.Time {
	return time.Now()
}
