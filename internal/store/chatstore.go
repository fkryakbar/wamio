package store

import (
	"database/sql"
	"fmt"
)

// ChatStore handles persistence of chat list and messages in SQLite
type ChatStore struct {
	db *sql.DB
}

// NewChatStore creates a new ChatStore using the existing database connection
// and creates the required tables if they don't exist
func NewChatStore(db *sql.DB) (*ChatStore, error) {
	cs := &ChatStore{db: db}
	if err := cs.createTables(); err != nil {
		return nil, fmt.Errorf("failed to create chat tables: %w", err)
	}
	return cs, nil
}

func (cs *ChatStore) createTables() error {
	_, err := cs.db.Exec(`
		CREATE TABLE IF NOT EXISTS wamio_chats (
			jid TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			last_message TEXT NOT NULL DEFAULT '',
			last_message_time INTEGER NOT NULL DEFAULT 0,
			unread_count INTEGER NOT NULL DEFAULT 0,
			is_group INTEGER NOT NULL DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS wamio_messages (
			id TEXT NOT NULL,
			chat_jid TEXT NOT NULL,
			sender_jid TEXT NOT NULL DEFAULT '',
			sender_name TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL DEFAULT '',
			timestamp INTEGER NOT NULL DEFAULT 0,
			is_from_me INTEGER NOT NULL DEFAULT 0,
			is_read INTEGER NOT NULL DEFAULT 0,
			media_type TEXT NOT NULL DEFAULT '',
			media_duration INTEGER NOT NULL DEFAULT 0,
			file_name TEXT NOT NULL DEFAULT '',
			mimetype TEXT NOT NULL DEFAULT '',
			is_ptt INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (id, chat_jid)
		);

		CREATE INDEX IF NOT EXISTS idx_messages_chat_ts ON wamio_messages(chat_jid, timestamp);
	`)
	return err
}

// ChatRow represents a chat stored in the database
type ChatRow struct {
	JID             string
	Name            string
	LastMessage     string
	LastMessageTime int64
	UnreadCount     int
	IsGroup         bool
}

// MessageRow represents a message stored in the database
type MessageRow struct {
	ID            string
	ChatJID       string
	SenderJID     string
	SenderName    string
	Content       string
	Timestamp     int64
	IsFromMe      bool
	IsRead        bool
	MediaType     string
	MediaDuration uint32
	FileName      string
	Mimetype      string
	IsPTT         bool
}

// UpsertChat inserts or updates a chat entry
func (cs *ChatStore) UpsertChat(c ChatRow) error {
	_, err := cs.db.Exec(`
		INSERT INTO wamio_chats (jid, name, last_message, last_message_time, unread_count, is_group)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(jid) DO UPDATE SET
			name = CASE WHEN excluded.name != '' THEN excluded.name ELSE wamio_chats.name END,
			last_message = CASE WHEN excluded.last_message_time >= wamio_chats.last_message_time 
				THEN excluded.last_message ELSE wamio_chats.last_message END,
			last_message_time = MAX(wamio_chats.last_message_time, excluded.last_message_time),
			unread_count = excluded.unread_count,
			is_group = excluded.is_group
	`, c.JID, c.Name, c.LastMessage, c.LastMessageTime, c.UnreadCount, boolToInt(c.IsGroup))
	return err
}

// UpsertMessage inserts a message (ignores if already exists)
func (cs *ChatStore) UpsertMessage(m MessageRow) error {
	_, err := cs.db.Exec(`
		INSERT OR IGNORE INTO wamio_messages 
		(id, chat_jid, sender_jid, sender_name, content, timestamp, is_from_me, is_read, media_type, media_duration, file_name, mimetype, is_ptt)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, m.ID, m.ChatJID, m.SenderJID, m.SenderName, m.Content, m.Timestamp,
		boolToInt(m.IsFromMe), boolToInt(m.IsRead), m.MediaType, m.MediaDuration,
		m.FileName, m.Mimetype, boolToInt(m.IsPTT))
	return err
}

// GetAllChats returns all chats sorted by last message time (newest first)
func (cs *ChatStore) GetAllChats() ([]ChatRow, error) {
	rows, err := cs.db.Query(`
		SELECT jid, name, last_message, last_message_time, unread_count, is_group
		FROM wamio_chats
		ORDER BY last_message_time DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chats []ChatRow
	for rows.Next() {
		var c ChatRow
		var isGroup int
		if err := rows.Scan(&c.JID, &c.Name, &c.LastMessage, &c.LastMessageTime, &c.UnreadCount, &isGroup); err != nil {
			return nil, err
		}
		c.IsGroup = isGroup != 0
		chats = append(chats, c)
	}
	return chats, rows.Err()
}

// GetRecentChats returns chats whose last_message_time >= sinceTimestamp
func (cs *ChatStore) GetRecentChats(sinceTimestamp int64) ([]ChatRow, error) {
	rows, err := cs.db.Query(`
		SELECT jid, name, last_message, last_message_time, unread_count, is_group
		FROM wamio_chats
		WHERE last_message_time >= ?
		ORDER BY last_message_time DESC
	`, sinceTimestamp)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chats []ChatRow
	for rows.Next() {
		var c ChatRow
		var isGroup int
		if err := rows.Scan(&c.JID, &c.Name, &c.LastMessage, &c.LastMessageTime, &c.UnreadCount, &isGroup); err != nil {
			return nil, err
		}
		c.IsGroup = isGroup != 0
		chats = append(chats, c)
	}
	return chats, rows.Err()
}

// GetMessages returns messages for a chat, ordered by timestamp, limited to `limit`
func (cs *ChatStore) GetMessages(chatJID string, limit int) ([]MessageRow, error) {
	rows, err := cs.db.Query(`
		SELECT id, chat_jid, sender_jid, sender_name, content, timestamp, 
		       is_from_me, is_read, media_type, media_duration, file_name, mimetype, is_ptt
		FROM wamio_messages
		WHERE chat_jid = ?
		ORDER BY timestamp DESC
		LIMIT ?
	`, chatJID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []MessageRow
	for rows.Next() {
		var m MessageRow
		var isFromMe, isRead, isPTT int
		if err := rows.Scan(&m.ID, &m.ChatJID, &m.SenderJID, &m.SenderName, &m.Content,
			&m.Timestamp, &isFromMe, &isRead, &m.MediaType, &m.MediaDuration,
			&m.FileName, &m.Mimetype, &isPTT); err != nil {
			return nil, err
		}
		m.IsFromMe = isFromMe != 0
		m.IsRead = isRead != 0
		m.IsPTT = isPTT != 0
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Reverse to get chronological order (oldest first)
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

// GetMessagesBefore returns messages older than beforeTimestamp for a chat.
// Results are returned in chronological order (oldest first).
func (cs *ChatStore) GetMessagesBefore(chatJID string, beforeTimestamp int64, limit int) ([]MessageRow, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := cs.db.Query(`
		SELECT id, chat_jid, sender_jid, sender_name, content, timestamp,
		       is_from_me, is_read, media_type, media_duration, file_name, mimetype, is_ptt
		FROM wamio_messages
		WHERE chat_jid = ? AND timestamp < ?
		ORDER BY timestamp DESC
		LIMIT ?
	`, chatJID, beforeTimestamp, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []MessageRow
	for rows.Next() {
		var m MessageRow
		var isFromMe, isRead, isPTT int
		if err := rows.Scan(&m.ID, &m.ChatJID, &m.SenderJID, &m.SenderName, &m.Content,
			&m.Timestamp, &isFromMe, &isRead, &m.MediaType, &m.MediaDuration,
			&m.FileName, &m.Mimetype, &isPTT); err != nil {
			return nil, err
		}
		m.IsFromMe = isFromMe != 0
		m.IsRead = isRead != 0
		m.IsPTT = isPTT != 0
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

// UpdateChatUnread updates the unread count for a chat
func (cs *ChatStore) UpdateChatUnread(jid string, unreadCount int) error {
	_, err := cs.db.Exec(`UPDATE wamio_chats SET unread_count = ? WHERE jid = ?`, unreadCount, jid)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
