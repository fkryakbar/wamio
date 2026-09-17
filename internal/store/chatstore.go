package store

import (
	"database/sql"
	"fmt"
	"strings"
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
			is_group INTEGER NOT NULL DEFAULT 0,
			last_message_id TEXT NOT NULL DEFAULT '',
			last_message_order INTEGER NOT NULL DEFAULT 0,
			history_state INTEGER NOT NULL DEFAULT 0
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
			delivery_status TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (id, chat_jid)
		);

		CREATE INDEX IF NOT EXISTS idx_messages_chat_ts ON wamio_messages(chat_jid, timestamp);
	`)
	if err != nil {
		return err
	}
	// Existing installations have the original table shape. SQLite has no
	// ADD COLUMN IF NOT EXISTS, so duplicate-column errors are intentionally
	// ignored here.
	for _, column := range []string{
		"last_message_id TEXT NOT NULL DEFAULT ''",
		"last_message_order INTEGER NOT NULL DEFAULT 0",
		"history_state INTEGER NOT NULL DEFAULT 0",
	} {
		if _, alterErr := cs.db.Exec("ALTER TABLE wamio_chats ADD COLUMN " + column); alterErr != nil && !strings.Contains(alterErr.Error(), "duplicate column name") {
			return alterErr
		}
	}
	if _, alterErr := cs.db.Exec("ALTER TABLE wamio_messages ADD COLUMN delivery_status TEXT NOT NULL DEFAULT ''"); alterErr != nil && !strings.Contains(alterErr.Error(), "duplicate column name") {
		return alterErr
	}
	return nil
}

// ChatRow represents a chat stored in the database
type ChatRow struct {
	JID              string
	Name             string
	LastMessage      string
	LastMessageTime  int64
	UnreadCount      int
	IsGroup          bool
	LastMessageID    string
	LastMessageOrder uint64
	// HistoryState: 0 unknown, 1 primary has more, 2 exhausted, 3 unavailable.
	HistoryState int
}

// MessageRow represents a message stored in the database
type MessageRow struct {
	ID             string
	ChatJID        string
	SenderJID      string
	SenderName     string
	Content        string
	Timestamp      int64
	IsFromMe       bool
	IsRead         bool
	MediaType      string
	MediaDuration  uint32
	FileName       string
	Mimetype       string
	IsPTT          bool
	DeliveryStatus string
}

// UpsertChat inserts or updates a chat entry
func (cs *ChatStore) UpsertChat(c ChatRow) error {
	_, err := cs.db.Exec(`
		INSERT INTO wamio_chats (jid, name, last_message, last_message_time, unread_count, is_group, last_message_id, last_message_order, history_state)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(jid) DO UPDATE SET
			name = CASE WHEN excluded.name != '' THEN excluded.name ELSE wamio_chats.name END,
			last_message = CASE WHEN excluded.last_message_time > wamio_chats.last_message_time
				OR (excluded.last_message_time = wamio_chats.last_message_time AND excluded.last_message_order >= wamio_chats.last_message_order)
				THEN excluded.last_message ELSE wamio_chats.last_message END,
			last_message_time = CASE WHEN excluded.last_message_time > wamio_chats.last_message_time
				OR (excluded.last_message_time = wamio_chats.last_message_time AND excluded.last_message_order >= wamio_chats.last_message_order)
				THEN excluded.last_message_time ELSE wamio_chats.last_message_time END,
			last_message_id = CASE WHEN excluded.last_message_time > wamio_chats.last_message_time
				OR (excluded.last_message_time = wamio_chats.last_message_time AND excluded.last_message_order >= wamio_chats.last_message_order)
				THEN excluded.last_message_id ELSE wamio_chats.last_message_id END,
			last_message_order = CASE WHEN excluded.last_message_time > wamio_chats.last_message_time
				OR (excluded.last_message_time = wamio_chats.last_message_time AND excluded.last_message_order >= wamio_chats.last_message_order)
				THEN excluded.last_message_order ELSE wamio_chats.last_message_order END,
			unread_count = excluded.unread_count,
			is_group = excluded.is_group,
			history_state = CASE WHEN excluded.history_state != 0 THEN excluded.history_state ELSE wamio_chats.history_state END
	`, c.JID, c.Name, c.LastMessage, c.LastMessageTime, c.UnreadCount, boolToInt(c.IsGroup), c.LastMessageID, c.LastMessageOrder, c.HistoryState)
	return err
}

// UpsertMessage inserts a message (ignores if already exists)
func (cs *ChatStore) UpsertMessage(m MessageRow) error {
	_, err := cs.db.Exec(`
		INSERT OR IGNORE INTO wamio_messages 
		(id, chat_jid, sender_jid, sender_name, content, timestamp, is_from_me, is_read, media_type, media_duration, file_name, mimetype, is_ptt, delivery_status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, m.ID, m.ChatJID, m.SenderJID, m.SenderName, m.Content, m.Timestamp,
		boolToInt(m.IsFromMe), boolToInt(m.IsRead), m.MediaType, m.MediaDuration,
		m.FileName, m.Mimetype, boolToInt(m.IsPTT), m.DeliveryStatus)
	return err
}

// GetAllChats returns all chats sorted by last message time (newest first)
func (cs *ChatStore) GetAllChats() ([]ChatRow, error) {
	rows, err := cs.db.Query(`
		SELECT jid, name, last_message, last_message_time, unread_count, is_group, last_message_id, last_message_order, history_state
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
		if err := rows.Scan(&c.JID, &c.Name, &c.LastMessage, &c.LastMessageTime, &c.UnreadCount, &isGroup, &c.LastMessageID, &c.LastMessageOrder, &c.HistoryState); err != nil {
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
		SELECT jid, name, last_message, last_message_time, unread_count, is_group, last_message_id, last_message_order, history_state
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
		if err := rows.Scan(&c.JID, &c.Name, &c.LastMessage, &c.LastMessageTime, &c.UnreadCount, &isGroup, &c.LastMessageID, &c.LastMessageOrder, &c.HistoryState); err != nil {
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
		       is_from_me, is_read, media_type, media_duration, file_name, mimetype, is_ptt, delivery_status
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
			&m.FileName, &m.Mimetype, &isPTT, &m.DeliveryStatus); err != nil {
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
		       is_from_me, is_read, media_type, media_duration, file_name, mimetype, is_ptt, delivery_status
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
			&m.FileName, &m.Mimetype, &isPTT, &m.DeliveryStatus); err != nil {
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

// UpdateMessageReceipt advances outgoing message delivery state without
// allowing an older receipt to downgrade a newer one.
func (cs *ChatStore) UpdateMessageReceipt(chatJID string, ids []string, status string) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		_, err := cs.db.Exec(`
			UPDATE wamio_messages SET
				delivery_status = CASE
					WHEN ? = 'read' THEN 'read'
					WHEN ? = 'delivered' AND delivery_status != 'read' THEN 'delivered'
					WHEN delivery_status = '' THEN 'sent'
					ELSE delivery_status END,
				is_read = CASE WHEN ? = 'read' THEN 1 ELSE is_read END
			WHERE chat_jid = ? AND id = ? AND is_from_me = 1
		`, status, status, status, chatJID, id)
		if err != nil {
			return err
		}
	}
	return nil
}

// DeleteChatCache removes a filtered chat and all derived messages.
func (cs *ChatStore) DeleteChatCache(jid string) error {
	_, err := cs.db.Exec(`DELETE FROM wamio_messages WHERE chat_jid = ?; DELETE FROM wamio_chats WHERE jid = ?`, jid, jid)
	return err
}

// MergeChatCache moves a LID-keyed cache into its phone-number key.
func (cs *ChatStore) MergeChatCache(fromJID, toJID string) error {
	if fromJID == toJID {
		return nil
	}
	tx, err := cs.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT OR IGNORE INTO wamio_messages
		(id, chat_jid, sender_jid, sender_name, content, timestamp, is_from_me, is_read, media_type, media_duration, file_name, mimetype, is_ptt, delivery_status)
		SELECT id, ?, sender_jid, sender_name, content, timestamp, is_from_me, is_read, media_type, media_duration, file_name, mimetype, is_ptt, delivery_status
		FROM wamio_messages WHERE chat_jid = ?`, toJID, fromJID); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM wamio_messages WHERE chat_jid = ?`, fromJID); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM wamio_chats WHERE jid = ?`, fromJID); err != nil {
		return err
	}
	return tx.Commit()
}

// SetHistoryState records whether an older-message request can be made.
func (cs *ChatStore) SetHistoryState(jid string, state int) error {
	_, err := cs.db.Exec(`UPDATE wamio_chats SET history_state = ? WHERE jid = ?`, state, jid)
	return err
}

func (cs *ChatStore) GetHistoryState(jid string) (int, error) {
	var state int
	err := cs.db.QueryRow(`SELECT history_state FROM wamio_chats WHERE jid = ?`, jid).Scan(&state)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return state, err
}

// TrimMessages keeps the newest max messages for an initial RECENT sync.
func (cs *ChatStore) TrimMessages(chatJID string, max int) error {
	if max <= 0 {
		return nil
	}
	_, err := cs.db.Exec(`
		DELETE FROM wamio_messages
		WHERE chat_jid = ? AND id NOT IN (
			SELECT id FROM wamio_messages WHERE chat_jid = ? ORDER BY timestamp DESC, id DESC LIMIT ?
		)
	`, chatJID, chatJID, max)
	return err
}

// ClearCache removes only Wamio's derived chat cache, never WhatsApp protocol data.
func (cs *ChatStore) ClearCache() error {
	_, err := cs.db.Exec(`DELETE FROM wamio_messages; DELETE FROM wamio_chats;`)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
