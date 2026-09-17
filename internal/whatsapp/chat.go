package whatsapp

// ChatItem represents a conversation in the chat list sidebar
type ChatItem struct {
	JID             string `json:"jid"`
	Name            string `json:"name"`
	LastMessage     string `json:"lastMessage"`
	LastMessageTime int64  `json:"lastMessageTime"` // Unix timestamp
	UnreadCount     int    `json:"unreadCount"`
	IsGroup         bool   `json:"isGroup"`
	Avatar          string `json:"avatar,omitempty"` // Base64 encoded
	// These values make same-second history chunks deterministic. They stay
	// internal so the frontend continues to receive the public ChatItem shape.
	LastMessageID    string `json:"-"`
	LastMessageOrder uint64 `json:"-"`
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
	IsPTT         bool   `json:"isPtt,omitempty"`         // Push-to-talk (voice note)
	// DeliveryStatus applies only to outgoing messages: sent, delivered, read.
	DeliveryStatus string `json:"deliveryStatus,omitempty"`
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

// InitialSyncEvent is emitted to signal initial sync lifecycle state
type InitialSyncEvent struct {
	State   string `json:"state"` // "running" | "done" | "failed"
	Message string `json:"message,omitempty"`
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
