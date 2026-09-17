package whatsapp

// ChatItem represents a conversation in the chat list sidebar
type ChatItem struct {
	JID             string `json:"jid"`
	Name            string `json:"name"`
	LastMessage     string `json:"lastMessage"`
	LastMessageTime int64  `json:"lastMessageTime"` // Unix timestamp
	UnreadCount     int    `json:"unreadCount"`
	IsGroup         bool   `json:"isGroup"`
	// LastMessageFromMe and LastMessageStatus let the sidebar render WhatsApp
	// receipt ticks without embedding presentation text such as "Anda:" in the
	// stored preview.
	LastMessageFromMe bool   `json:"lastMessageFromMe"`
	LastMessageStatus string `json:"lastMessageStatus"`
	IsArchived        bool   `json:"isArchived"`
	IsPinned          bool   `json:"isPinned"`
	IsMuted           bool   `json:"isMuted"`
	// MutedUntil is a Unix timestamp. A negative value represents a permanent
	// mute, matching WhatsApp's app-state representation.
	MutedUntil int64    `json:"mutedUntil"`
	ListIDs    []string `json:"listIds,omitempty"`
	Avatar     string   `json:"avatar,omitempty"` // Base64 encoded
	// These values make same-second history chunks deterministic. They stay
	// internal so the frontend continues to receive the public ChatItem shape.
	LastMessageID    string `json:"-"`
	LastMessageOrder uint64 `json:"-"`
	LastReadAt       int64  `json:"-"`
	// ArchiveKnown is set once the archive state comes from WhatsApp app state.
	// It prevents a later history chunk from restoring an older archive value.
	ArchiveKnown bool `json:"-"`
}

// MessageItem represents a single message in a chat conversation
type MessageItem struct {
	ID            string `json:"id"`
	ChatJID       string `json:"chatJid"`
	SenderJID     string `json:"senderJid"`
	SenderName    string `json:"senderName"`
	Content       string `json:"content"`
	Timestamp     int64  `json:"timestamp"` // Unix timestamp
	IsFromMe      bool   `json:"isFromMe"`
	IsRead        bool   `json:"isRead"`
	MediaType     string `json:"mediaType,omitempty"`     // "image", "video", "audio", "document", "sticker", ""
	MediaURL      string `json:"mediaUrl,omitempty"`      // Local cached file path
	MediaDuration uint32 `json:"mediaDuration,omitempty"` // Duration in seconds (audio/video)
	FileName      string `json:"fileName,omitempty"`      // Original filename (documents)
	Mimetype      string `json:"mimetype,omitempty"`      // MIME type
	Thumbnail     string `json:"thumbnail,omitempty"`     // Base64 JPEG preview for media
	IsPTT         bool   `json:"isPtt,omitempty"`         // Push-to-talk (voice note)
	// DeliveryStatus applies only to outgoing messages: sent, delivered, read.
	DeliveryStatus string `json:"deliveryStatus,omitempty"`
	// ReplyTo is a snapshot of the quoted message supplied by WhatsApp. Keeping
	// the snapshot lets replies render even when the referenced page isn't open.
	ReplyTo     *MessageReference `json:"replyTo,omitempty"`
	IsForwarded bool              `json:"isForwarded,omitempty"`
	IsDeleted   bool              `json:"isDeleted,omitempty"`
	Caption     string            `json:"caption,omitempty"`
	FileSize    uint64            `json:"fileSize,omitempty"`
	Kind        string            `json:"kind,omitempty"` // "" (message) or "call"
	Call        *CallInfo         `json:"call,omitempty"`
	Reactions   []MessageReaction `json:"reactions,omitempty"`
}

// CallInfo is the presentation-safe call metadata used by a system call card.
// It intentionally contains no signaling or media secrets.
type CallInfo struct {
	CallID     string `json:"callId"`
	Outcome    string `json:"outcome"`
	Duration   int64  `json:"duration,omitempty"`
	IsVideo    bool   `json:"isVideo"`
	IsIncoming bool   `json:"isIncoming"`
}

// ChatList is a WhatsApp-synchronized, read-only chat-list definition.
type ChatList struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Order    int32  `json:"order"`
	IsActive bool   `json:"isActive"`
}

// DocumentState reports whether a document is already present in Wamio's
// durable local cache. The internal cache path is deliberately not exposed.
type DocumentState struct {
	Downloaded bool   `json:"downloaded"`
	FileName   string `json:"fileName"`
	FileSize   uint64 `json:"fileSize"`
}

// RestoreSessionResult tells the UI whether an automatic restore attempt was
// started. Connection success/failure continues to arrive through wa:connection.
type RestoreSessionResult struct {
	Attempted bool   `json:"attempted"`
	AccountID string `json:"accountId,omitempty"`
}

// MessageReference identifies a message and contains the small display
// snapshot needed by a reply bubble.
type MessageReference struct {
	ID         string `json:"id"`
	ChatJID    string `json:"chatJid"`
	SenderJID  string `json:"senderJid"`
	SenderName string `json:"senderName"`
	Content    string `json:"content"`
	MediaType  string `json:"mediaType,omitempty"`
	IsFromMe   bool   `json:"isFromMe"`
}

// MessageReaction is grouped by emoji for rendering. FromMe reports whether
// the current account is among the reactors for this emoji.
type MessageReaction struct {
	Emoji  string `json:"emoji"`
	Count  int    `json:"count"`
	FromMe bool   `json:"fromMe"`
}

// ChatProfile is the intentionally small, read-only profile panel exposed to
// the desktop UI. Group-only fields are omitted for personal chats.
type ChatProfile struct {
	JID              string `json:"jid"`
	Name             string `json:"name"`
	Avatar           string `json:"avatar,omitempty"`
	IsGroup          bool   `json:"isGroup"`
	PhoneNumber      string `json:"phoneNumber,omitempty"`
	Description      string `json:"description,omitempty"`
	ParticipantCount int    `json:"participantCount,omitempty"`
}

// ForwardResult reports every destination independently because WhatsApp
// sends cannot be rolled back after one destination accepts a message.
type ForwardResult struct {
	ChatJID string `json:"chatJid"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// MessageEvent is emitted to the frontend when a new message arrives
type MessageEvent struct {
	ChatJID string      `json:"chatJid"`
	Message MessageItem `json:"message"`
}

// ChatUpdateEvent is emitted when a chat list item changes
type ChatUpdateEvent struct {
	Chat ChatItem `json:"chat"`
}

type MessageReceiptEvent struct {
	ChatJID        string   `json:"chatJid"`
	MessageIDs     []string `json:"messageIds"`
	DeliveryStatus string   `json:"deliveryStatus"`
}

type MessageReactionEvent struct {
	ChatJID   string            `json:"chatJid"`
	MessageID string            `json:"messageId"`
	Reactions []MessageReaction `json:"reactions"`
}

type MessageDeleteEvent struct {
	ChatJID   string `json:"chatJid"`
	MessageID string `json:"messageId"`
	ForMe     bool   `json:"forMe"`
}

// InitialSyncEvent is emitted to signal initial sync lifecycle state
type InitialSyncEvent struct {
	State   string `json:"state"` // "running" | "ready" | "degraded"
	Message string `json:"message,omitempty"`
}

// SyncProgressEvent reports actual pairing/history progress. Progress is -1
// when WhatsApp has not supplied a meaningful percentage yet.
type SyncProgressEvent struct {
	Phase          string `json:"phase"` // "initial" | "background" | "complete" | "degraded"
	Progress       int32  `json:"progress"`
	ProcessedChats int    `json:"processedChats"`
	PreparedChats  int    `json:"preparedChats"`
	Message        string `json:"message,omitempty"`
}

// PresenceEvent describes the latest server-provided status for a personal chat.
type PresenceEvent struct {
	ChatJID     string `json:"chatJid"`
	Online      bool   `json:"online"`
	LastSeen    int64  `json:"lastSeen,omitempty"`
	Unavailable bool   `json:"unavailable"`
}

// ChatPresenceEvent is emitted when a contact starts or stops typing.
type ChatPresenceEvent struct {
	ChatJID string `json:"chatJid"`
	Typing  bool   `json:"typing"`
}

// MessagePage is a local page plus whether older history may be requested
// from the primary phone once the local cache is exhausted.
type MessagePage struct {
	Messages        []MessageItem `json:"messages"`
	HasMoreLocal    bool          `json:"hasMoreLocal"`
	CanRequestOlder bool          `json:"canRequestOlder"`
}

// HistoryPageEvent delivers asynchronous ON_DEMAND history to the frontend.
type HistoryPageEvent struct {
	ChatJID         string        `json:"chatJid"`
	Messages        []MessageItem `json:"messages"`
	CanRequestOlder bool          `json:"canRequestOlder"`
	Error           string        `json:"error,omitempty"`
}

// NotificationEvent is emitted for native OS notifications
type NotificationEvent struct {
	ChatJID    string `json:"chatJid"`
	ChatName   string `json:"chatName"`
	SenderName string `json:"senderName"`
	Content    string `json:"content"`
	IsGroup    bool   `json:"isGroup"`
}

// ChatWithMessages bundles a chat with its recent messages for preloading
type ChatWithMessages struct {
	Chat     ChatItem      `json:"chat"`
	Messages []MessageItem `json:"messages"`
}

// RecentChatMessagesEvent is emitted after history sync with preloaded messages
// for chats active in the last 7 days
type RecentChatMessagesEvent struct {
	Entries []ChatWithMessages `json:"entries"`
}
