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

// InitialSyncEvent is emitted to signal initial sync lifecycle state
type InitialSyncEvent struct {
	State string `json:"state"` // "running" | "done"
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
