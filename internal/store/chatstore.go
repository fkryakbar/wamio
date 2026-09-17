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
			history_state INTEGER NOT NULL DEFAULT 0,
			last_read_at INTEGER NOT NULL DEFAULT 0,
			last_message_from_me INTEGER NOT NULL DEFAULT 0,
			last_message_status TEXT NOT NULL DEFAULT '',
			is_archived INTEGER NOT NULL DEFAULT 0,
			is_pinned INTEGER NOT NULL DEFAULT 0,
			muted_until INTEGER NOT NULL DEFAULT 0
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
			thumbnail TEXT NOT NULL DEFAULT '',
			is_ptt INTEGER NOT NULL DEFAULT 0,
			delivery_status TEXT NOT NULL DEFAULT '',
			reply_id TEXT NOT NULL DEFAULT '',
			reply_chat_jid TEXT NOT NULL DEFAULT '',
			reply_sender_jid TEXT NOT NULL DEFAULT '',
			reply_sender_name TEXT NOT NULL DEFAULT '',
			reply_content TEXT NOT NULL DEFAULT '',
			reply_media_type TEXT NOT NULL DEFAULT '',
			reply_from_me INTEGER NOT NULL DEFAULT 0,
			reply_is_deleted INTEGER NOT NULL DEFAULT 0,
			is_forwarded INTEGER NOT NULL DEFAULT 0,
			is_deleted INTEGER NOT NULL DEFAULT 0,
			caption TEXT NOT NULL DEFAULT '',
			file_size INTEGER NOT NULL DEFAULT 0,
			kind TEXT NOT NULL DEFAULT '',
			call_id TEXT NOT NULL DEFAULT '',
			call_outcome TEXT NOT NULL DEFAULT '',
			call_duration INTEGER NOT NULL DEFAULT 0,
			call_is_video INTEGER NOT NULL DEFAULT 0,
			call_is_incoming INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (id, chat_jid)
		);

		CREATE TABLE IF NOT EXISTS wamio_chat_lists (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			type TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0,
			is_active INTEGER NOT NULL DEFAULT 1
		);
		CREATE TABLE IF NOT EXISTS wamio_chat_list_memberships (
			list_id TEXT NOT NULL,
			chat_jid TEXT NOT NULL,
			PRIMARY KEY (list_id, chat_jid)
		);

		CREATE TABLE IF NOT EXISTS wamio_message_reactions (
			chat_jid TEXT NOT NULL,
			message_id TEXT NOT NULL,
			reactor_jid TEXT NOT NULL,
			emoji TEXT NOT NULL DEFAULT '',
			timestamp INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (chat_jid, message_id, reactor_jid)
		);

		CREATE INDEX IF NOT EXISTS idx_messages_chat_ts ON wamio_messages(chat_jid, timestamp);
		CREATE INDEX IF NOT EXISTS idx_reactions_message ON wamio_message_reactions(chat_jid, message_id);
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
		"last_read_at INTEGER NOT NULL DEFAULT 0",
		"last_message_from_me INTEGER NOT NULL DEFAULT 0",
		"last_message_status TEXT NOT NULL DEFAULT ''",
		"is_archived INTEGER NOT NULL DEFAULT 0",
		"is_pinned INTEGER NOT NULL DEFAULT 0",
		"muted_until INTEGER NOT NULL DEFAULT 0",
	} {
		if _, alterErr := cs.db.Exec("ALTER TABLE wamio_chats ADD COLUMN " + column); alterErr != nil && !strings.Contains(alterErr.Error(), "duplicate column name") {
			return alterErr
		}
	}
	for _, column := range []string{
		"delivery_status TEXT NOT NULL DEFAULT ''",
		"thumbnail TEXT NOT NULL DEFAULT ''",
		"reply_id TEXT NOT NULL DEFAULT ''",
		"reply_chat_jid TEXT NOT NULL DEFAULT ''",
		"reply_sender_jid TEXT NOT NULL DEFAULT ''",
		"reply_sender_name TEXT NOT NULL DEFAULT ''",
		"reply_content TEXT NOT NULL DEFAULT ''",
		"reply_media_type TEXT NOT NULL DEFAULT ''",
		"reply_from_me INTEGER NOT NULL DEFAULT 0",
		"reply_is_deleted INTEGER NOT NULL DEFAULT 0",
		"is_forwarded INTEGER NOT NULL DEFAULT 0",
		"is_deleted INTEGER NOT NULL DEFAULT 0",
		"caption TEXT NOT NULL DEFAULT ''",
		"file_size INTEGER NOT NULL DEFAULT 0",
		"kind TEXT NOT NULL DEFAULT ''",
		"call_id TEXT NOT NULL DEFAULT ''",
		"call_outcome TEXT NOT NULL DEFAULT ''",
		"call_duration INTEGER NOT NULL DEFAULT 0",
		"call_is_video INTEGER NOT NULL DEFAULT 0",
		"call_is_incoming INTEGER NOT NULL DEFAULT 0",
	} {
		if _, alterErr := cs.db.Exec("ALTER TABLE wamio_messages ADD COLUMN " + column); alterErr != nil && !strings.Contains(alterErr.Error(), "duplicate column name") {
			return alterErr
		}
	}
	return nil
}

// ChatRow represents a chat stored in the database
type ChatRow struct {
	JID               string
	Name              string
	LastMessage       string
	LastMessageTime   int64
	UnreadCount       int
	IsGroup           bool
	LastMessageID     string
	LastMessageOrder  uint64
	LastMessageFromMe bool
	LastMessageStatus string
	IsArchived        bool
	IsPinned          bool
	MutedUntil        int64
	// HistoryState: 0 unknown, 1 primary has more, 2 exhausted, 3 unavailable.
	HistoryState int
	// LastReadAt prevents a late history chunk from restoring an unread badge
	// for messages that were already acknowledged on another device.
	LastReadAt int64
}

// MessageRow represents a message stored in the database
type MessageRow struct {
	ID              string
	ChatJID         string
	SenderJID       string
	SenderName      string
	Content         string
	Timestamp       int64
	IsFromMe        bool
	IsRead          bool
	MediaType       string
	MediaDuration   uint32
	FileName        string
	Mimetype        string
	Thumbnail       string
	IsPTT           bool
	DeliveryStatus  string
	ReplyID         string
	ReplyChatJID    string
	ReplySenderJID  string
	ReplySenderName string
	ReplyContent    string
	ReplyMediaType  string
	ReplyFromMe     bool
	ReplyIsDeleted  bool
	IsForwarded     bool
	IsDeleted       bool
	Caption         string
	FileSize        uint64
	Kind            string
	CallID          string
	CallOutcome     string
	CallDuration    int64
	CallIsVideo     bool
	CallIsIncoming  bool
}

type ChatListRow struct {
	ID       string
	Name     string
	Type     string
	Order    int32
	IsActive bool
}

// ReactionRow stores one participant's latest reaction. An empty Emoji is a
// tombstone for a removed reaction, allowing delayed history to stay stale.
type ReactionRow struct {
	ChatJID    string
	MessageID  string
	ReactorJID string
	Emoji      string
	Timestamp  int64
}

const messageRowColumns = `id, chat_jid, sender_jid, sender_name, content, timestamp,
	is_from_me, is_read, media_type, media_duration, file_name, mimetype, thumbnail, is_ptt, delivery_status,
	reply_id, reply_chat_jid, reply_sender_jid, reply_sender_name, reply_content, reply_media_type,
	reply_from_me, reply_is_deleted, is_forwarded, is_deleted, caption, file_size, kind, call_id, call_outcome, call_duration, call_is_video, call_is_incoming`

func scanMessageRow(scanner interface{ Scan(...interface{}) error }, m *MessageRow) error {
	var isFromMe, isRead, isPTT, replyFromMe, replyIsDeleted, isForwarded, isDeleted, callIsVideo, callIsIncoming int
	err := scanner.Scan(&m.ID, &m.ChatJID, &m.SenderJID, &m.SenderName, &m.Content,
		&m.Timestamp, &isFromMe, &isRead, &m.MediaType, &m.MediaDuration,
		&m.FileName, &m.Mimetype, &m.Thumbnail, &isPTT, &m.DeliveryStatus, &m.ReplyID,
		&m.ReplyChatJID, &m.ReplySenderJID, &m.ReplySenderName, &m.ReplyContent,
		&m.ReplyMediaType, &replyFromMe, &replyIsDeleted, &isForwarded, &isDeleted, &m.Caption, &m.FileSize,
		&m.Kind, &m.CallID, &m.CallOutcome, &m.CallDuration, &callIsVideo, &callIsIncoming)
	if err != nil {
		return err
	}
	m.IsFromMe, m.IsRead, m.IsPTT = isFromMe != 0, isRead != 0, isPTT != 0
	m.ReplyFromMe, m.ReplyIsDeleted = replyFromMe != 0, replyIsDeleted != 0
	m.IsForwarded, m.IsDeleted = isForwarded != 0, isDeleted != 0
	m.CallIsVideo, m.CallIsIncoming = callIsVideo != 0, callIsIncoming != 0
	return nil
}

// UpsertChat inserts or updates a chat entry
func (cs *ChatStore) UpsertChat(c ChatRow) error {
	_, err := cs.db.Exec(`
		INSERT INTO wamio_chats (jid, name, last_message, last_message_time, unread_count, is_group, last_message_id, last_message_order, history_state, last_read_at, last_message_from_me, last_message_status, is_archived, is_pinned, muted_until)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
			last_message_from_me = CASE WHEN excluded.last_message_time > wamio_chats.last_message_time
				OR (excluded.last_message_time = wamio_chats.last_message_time AND excluded.last_message_order >= wamio_chats.last_message_order)
				THEN excluded.last_message_from_me ELSE wamio_chats.last_message_from_me END,
			last_message_status = CASE WHEN excluded.last_message_time > wamio_chats.last_message_time
				OR (excluded.last_message_time = wamio_chats.last_message_time AND excluded.last_message_order >= wamio_chats.last_message_order)
				THEN CASE
					WHEN excluded.last_message_id = wamio_chats.last_message_id
						AND (wamio_chats.last_message_status = 'read'
							OR (wamio_chats.last_message_status = 'delivered' AND excluded.last_message_status IN ('', 'sent')))
						THEN wamio_chats.last_message_status
					ELSE excluded.last_message_status END
				ELSE wamio_chats.last_message_status END,
			unread_count = CASE
				WHEN excluded.last_read_at > wamio_chats.last_read_at THEN 0
				WHEN excluded.last_message_time <= wamio_chats.last_read_at THEN 0
				ELSE excluded.unread_count END,
			is_group = excluded.is_group,
			history_state = CASE WHEN excluded.history_state != 0 THEN excluded.history_state ELSE wamio_chats.history_state END,
			last_read_at = MAX(wamio_chats.last_read_at, excluded.last_read_at),
			is_archived = excluded.is_archived,
			is_pinned = excluded.is_pinned,
			muted_until = excluded.muted_until
	`, c.JID, c.Name, c.LastMessage, c.LastMessageTime, c.UnreadCount, boolToInt(c.IsGroup), c.LastMessageID, c.LastMessageOrder, c.HistoryState, c.LastReadAt, boolToInt(c.LastMessageFromMe), c.LastMessageStatus, boolToInt(c.IsArchived), boolToInt(c.IsPinned), c.MutedUntil)
	return err
}

// UpsertMessage inserts a message (ignores if already exists)
func (cs *ChatStore) UpsertMessage(m MessageRow) error {
	// A revoke is a durable tombstone. Do not retain attachment metadata when a
	// revoke arrives before the original message or races with history sync.
	if m.IsDeleted {
		m.Content, m.MediaType, m.FileName, m.Mimetype, m.Thumbnail, m.Caption = "", "", "", "", "", ""
		m.MediaDuration, m.FileSize, m.IsPTT, m.Kind, m.CallID, m.CallOutcome, m.CallDuration, m.CallIsVideo, m.CallIsIncoming = 0, 0, false, "", "", "", 0, false, false
	}
	_, err := cs.db.Exec(`
		INSERT INTO wamio_messages
		(id, chat_jid, sender_jid, sender_name, content, timestamp, is_from_me, is_read, media_type, media_duration, file_name, mimetype, thumbnail, is_ptt, delivery_status, reply_id, reply_chat_jid, reply_sender_jid, reply_sender_name, reply_content, reply_media_type, reply_from_me, reply_is_deleted, is_forwarded, is_deleted, caption, file_size, kind, call_id, call_outcome, call_duration, call_is_video, call_is_incoming)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id, chat_jid) DO UPDATE SET
			is_read = CASE WHEN excluded.is_read = 1 THEN 1 ELSE wamio_messages.is_read END,
			delivery_status = CASE
				WHEN excluded.delivery_status = 'read' THEN 'read'
				WHEN excluded.delivery_status = 'delivered' AND wamio_messages.delivery_status != 'read' THEN 'delivered'
				WHEN wamio_messages.delivery_status = '' THEN excluded.delivery_status
				ELSE wamio_messages.delivery_status END,
			thumbnail = CASE WHEN excluded.thumbnail != '' THEN excluded.thumbnail ELSE wamio_messages.thumbnail END,
			reply_id = CASE WHEN excluded.reply_id != '' THEN excluded.reply_id ELSE wamio_messages.reply_id END,
			reply_chat_jid = CASE WHEN excluded.reply_id != '' THEN excluded.reply_chat_jid ELSE wamio_messages.reply_chat_jid END,
			reply_sender_jid = CASE WHEN excluded.reply_id != '' THEN excluded.reply_sender_jid ELSE wamio_messages.reply_sender_jid END,
			reply_sender_name = CASE WHEN excluded.reply_id != '' THEN excluded.reply_sender_name ELSE wamio_messages.reply_sender_name END,
			reply_content = CASE WHEN excluded.reply_id != '' THEN excluded.reply_content ELSE wamio_messages.reply_content END,
			reply_media_type = CASE WHEN excluded.reply_id != '' THEN excluded.reply_media_type ELSE wamio_messages.reply_media_type END,
			reply_from_me = CASE WHEN excluded.reply_id != '' THEN excluded.reply_from_me ELSE wamio_messages.reply_from_me END,
			reply_is_deleted = MAX(wamio_messages.reply_is_deleted, excluded.reply_is_deleted),
			is_forwarded = MAX(wamio_messages.is_forwarded, excluded.is_forwarded),
			is_deleted = MAX(wamio_messages.is_deleted, excluded.is_deleted),
			caption = CASE WHEN wamio_messages.is_deleted = 1 THEN '' WHEN excluded.caption != '' THEN excluded.caption ELSE wamio_messages.caption END,
			file_size = CASE WHEN wamio_messages.is_deleted = 1 THEN 0 WHEN excluded.file_size != 0 THEN excluded.file_size ELSE wamio_messages.file_size END,
			kind = CASE WHEN wamio_messages.is_deleted = 1 THEN '' WHEN excluded.kind != '' THEN excluded.kind ELSE wamio_messages.kind END,
			call_id = CASE WHEN wamio_messages.is_deleted = 1 THEN '' WHEN excluded.call_id != '' THEN excluded.call_id ELSE wamio_messages.call_id END,
			call_outcome = CASE WHEN wamio_messages.is_deleted = 1 THEN '' WHEN excluded.call_outcome != '' THEN excluded.call_outcome ELSE wamio_messages.call_outcome END,
			call_duration = CASE WHEN wamio_messages.is_deleted = 1 THEN 0 WHEN excluded.call_duration != 0 THEN excluded.call_duration ELSE wamio_messages.call_duration END,
			call_is_video = CASE WHEN wamio_messages.is_deleted = 1 THEN 0 WHEN excluded.call_is_video = 1 THEN 1 ELSE wamio_messages.call_is_video END,
			call_is_incoming = CASE WHEN wamio_messages.is_deleted = 1 THEN 0 WHEN excluded.call_is_incoming = 1 THEN 1 ELSE wamio_messages.call_is_incoming END
	`, m.ID, m.ChatJID, m.SenderJID, m.SenderName, m.Content, m.Timestamp,
		boolToInt(m.IsFromMe), boolToInt(m.IsRead), m.MediaType, m.MediaDuration,
		m.FileName, m.Mimetype, m.Thumbnail, boolToInt(m.IsPTT), m.DeliveryStatus,
		m.ReplyID, m.ReplyChatJID, m.ReplySenderJID, m.ReplySenderName, m.ReplyContent,
		m.ReplyMediaType, boolToInt(m.ReplyFromMe), boolToInt(m.ReplyIsDeleted),
		boolToInt(m.IsForwarded), boolToInt(m.IsDeleted), m.Caption, m.FileSize, m.Kind, m.CallID, m.CallOutcome,
		m.CallDuration, boolToInt(m.CallIsVideo), boolToInt(m.CallIsIncoming))
	return err
}

// GetAllChats returns all chats sorted by last message time (newest first)
func (cs *ChatStore) GetAllChats() ([]ChatRow, error) {
	rows, err := cs.db.Query(`
		SELECT jid, name, last_message, last_message_time, unread_count, is_group, last_message_id, last_message_order, history_state, last_read_at, last_message_from_me, last_message_status, is_archived, is_pinned, muted_until
		FROM wamio_chats
		ORDER BY is_pinned DESC, last_message_time DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chats []ChatRow
	for rows.Next() {
		var c ChatRow
		var isGroup, lastMessageFromMe, isArchived, isPinned int
		if err := rows.Scan(&c.JID, &c.Name, &c.LastMessage, &c.LastMessageTime, &c.UnreadCount, &isGroup, &c.LastMessageID, &c.LastMessageOrder, &c.HistoryState, &c.LastReadAt, &lastMessageFromMe, &c.LastMessageStatus, &isArchived, &isPinned, &c.MutedUntil); err != nil {
			return nil, err
		}
		c.IsGroup, c.LastMessageFromMe, c.IsArchived, c.IsPinned = isGroup != 0, lastMessageFromMe != 0, isArchived != 0, isPinned != 0
		chats = append(chats, c)
	}
	return chats, rows.Err()
}

// GetRecentChats returns chats whose last_message_time >= sinceTimestamp
func (cs *ChatStore) GetRecentChats(sinceTimestamp int64) ([]ChatRow, error) {
	rows, err := cs.db.Query(`
		SELECT jid, name, last_message, last_message_time, unread_count, is_group, last_message_id, last_message_order, history_state, last_read_at, last_message_from_me, last_message_status, is_archived, is_pinned, muted_until
		FROM wamio_chats
		WHERE last_message_time >= ?
		ORDER BY is_pinned DESC, last_message_time DESC
	`, sinceTimestamp)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chats []ChatRow
	for rows.Next() {
		var c ChatRow
		var isGroup, lastMessageFromMe, isArchived, isPinned int
		if err := rows.Scan(&c.JID, &c.Name, &c.LastMessage, &c.LastMessageTime, &c.UnreadCount, &isGroup, &c.LastMessageID, &c.LastMessageOrder, &c.HistoryState, &c.LastReadAt, &lastMessageFromMe, &c.LastMessageStatus, &isArchived, &isPinned, &c.MutedUntil); err != nil {
			return nil, err
		}
		c.IsGroup, c.LastMessageFromMe, c.IsArchived, c.IsPinned = isGroup != 0, lastMessageFromMe != 0, isArchived != 0, isPinned != 0
		chats = append(chats, c)
	}
	return chats, rows.Err()
}

// GetMessages returns messages for a chat, ordered by timestamp, limited to `limit`
func (cs *ChatStore) GetMessages(chatJID string, limit int) ([]MessageRow, error) {
	rows, err := cs.db.Query(`
		SELECT `+messageRowColumns+`
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
		if err := scanMessageRow(rows, &m); err != nil {
			return nil, err
		}
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
		SELECT `+messageRowColumns+`
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
		if err := scanMessageRow(rows, &m); err != nil {
			return nil, err
		}
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

// GetUnreadIncomingMessages returns local incoming messages that have not yet
// been acknowledged as read. Group callers must send one receipt per sender.
func (cs *ChatStore) GetUnreadIncomingMessages(chatJID string) ([]MessageRow, error) {
	rows, err := cs.db.Query(`
		SELECT `+messageRowColumns+`
		FROM wamio_messages
		WHERE chat_jid = ? AND is_from_me = 0 AND is_read = 0
		ORDER BY sender_jid, timestamp ASC, id ASC
	`, chatJID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []MessageRow
	for rows.Next() {
		var m MessageRow
		if err := scanMessageRow(rows, &m); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// MarkIncomingMessagesRead persists a read acknowledgement and clears the
// chat badge. A WhatsApp read receipt represents the visible read boundary,
// not merely the IDs included in the receipt stanza.
func (cs *ChatStore) MarkIncomingMessagesRead(chatJID string, ids []string, readAt int64) error {
	tx, err := cs.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if len(ids) > 0 {
		placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
		args := make([]interface{}, 0, len(ids)+1)
		args = append(args, chatJID)
		for _, id := range ids {
			args = append(args, id)
		}
		if _, err = tx.Exec(`UPDATE wamio_messages SET is_read = 1 WHERE chat_jid = ? AND is_from_me = 0 AND id IN (`+placeholders+`)`, args...); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`UPDATE wamio_chats SET unread_count = 0, last_read_at = MAX(last_read_at, ?) WHERE jid = ?`, readAt, chatJID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// HasIncomingMessageIDs distinguishes a read receipt generated by this
// account on another device from a recipient receipt for an outgoing message.
func (cs *ChatStore) HasIncomingMessageIDs(chatJID string, ids []string) (bool, error) {
	if len(ids) == 0 {
		return false, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]interface{}, 0, len(ids)+1)
	args = append(args, chatJID)
	for _, id := range ids {
		args = append(args, id)
	}
	var found int
	err := cs.db.QueryRow(`SELECT 1 FROM wamio_messages WHERE chat_jid = ? AND is_from_me = 0 AND id IN (`+placeholders+`) LIMIT 1`, args...).Scan(&found)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
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

// GetMessage finds one message for action validation and local state updates.
func (cs *ChatStore) GetMessage(chatJID, messageID string) (MessageRow, error) {
	var message MessageRow
	err := scanMessageRow(cs.db.QueryRow(`SELECT `+messageRowColumns+` FROM wamio_messages WHERE chat_jid = ? AND id = ?`, chatJID, messageID), &message)
	return message, err
}

// GetMessagesAround returns a chronological local context containing the
// target message. It is used to reveal the original message of a reply.
func (cs *ChatStore) GetMessagesAround(chatJID, messageID string, limit int) ([]MessageRow, error) {
	if limit <= 0 || limit > 50 {
		limit = 25
	}
	target, err := cs.GetMessage(chatJID, messageID)
	if err != nil {
		return nil, err
	}
	beforeLimit := limit / 2
	afterLimit := limit - beforeLimit - 1
	beforeRows, err := cs.db.Query(`
		SELECT `+messageRowColumns+` FROM wamio_messages
		WHERE chat_jid = ? AND (timestamp < ? OR (timestamp = ? AND id <= ?))
		ORDER BY timestamp DESC, id DESC LIMIT ?`, chatJID, target.Timestamp, target.Timestamp, target.ID, beforeLimit+1)
	if err != nil {
		return nil, err
	}
	defer beforeRows.Close()
	before := make([]MessageRow, 0, beforeLimit+1)
	for beforeRows.Next() {
		var row MessageRow
		if err := scanMessageRow(beforeRows, &row); err != nil {
			return nil, err
		}
		before = append(before, row)
	}
	if err := beforeRows.Err(); err != nil {
		return nil, err
	}
	for left, right := 0, len(before)-1; left < right; left, right = left+1, right-1 {
		before[left], before[right] = before[right], before[left]
	}
	afterRows, err := cs.db.Query(`
		SELECT `+messageRowColumns+` FROM wamio_messages
		WHERE chat_jid = ? AND (timestamp > ? OR (timestamp = ? AND id > ?))
		ORDER BY timestamp ASC, id ASC LIMIT ?`, chatJID, target.Timestamp, target.Timestamp, target.ID, afterLimit)
	if err != nil {
		return nil, err
	}
	defer afterRows.Close()
	for afterRows.Next() {
		var row MessageRow
		if err := scanMessageRow(afterRows, &row); err != nil {
			return nil, err
		}
		before = append(before, row)
	}
	if err := afterRows.Err(); err != nil {
		return nil, err
	}
	return before, nil
}

// UpsertReaction stores the last reaction made by one participant. An empty
// emoji is intentionally retained as a tombstone so an older history entry
// cannot restore a removed reaction.
func (cs *ChatStore) UpsertReaction(reaction ReactionRow) error {
	_, err := cs.db.Exec(`
		INSERT INTO wamio_message_reactions (chat_jid, message_id, reactor_jid, emoji, timestamp)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(chat_jid, message_id, reactor_jid) DO UPDATE SET
			emoji = excluded.emoji,
			timestamp = excluded.timestamp
		WHERE excluded.timestamp >= wamio_message_reactions.timestamp
	`, reaction.ChatJID, reaction.MessageID, reaction.ReactorJID, reaction.Emoji, reaction.Timestamp)
	return err
}

func (cs *ChatStore) GetReactions(chatJID, messageID string) ([]ReactionRow, error) {
	rows, err := cs.db.Query(`
		SELECT chat_jid, message_id, reactor_jid, emoji, timestamp
		FROM wamio_message_reactions
		WHERE chat_jid = ? AND message_id = ? AND emoji != ''
		ORDER BY emoji, reactor_jid
	`, chatJID, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var reactions []ReactionRow
	for rows.Next() {
		var reaction ReactionRow
		if err := rows.Scan(&reaction.ChatJID, &reaction.MessageID, &reaction.ReactorJID, &reaction.Emoji, &reaction.Timestamp); err != nil {
			return nil, err
		}
		reactions = append(reactions, reaction)
	}
	return reactions, rows.Err()
}

// MarkMessageDeleted retains a tombstone for a revoke, and invalidates any
// reply snapshots which refer to that now-deleted message.
func (cs *ChatStore) MarkMessageDeleted(chatJID, messageID string) error {
	tx, err := cs.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// A revoke can be delivered before the original history item. Keep a bare
	// row so the later item cannot resurrect its text or attachment.
	if _, err = tx.Exec(`INSERT OR IGNORE INTO wamio_messages (id, chat_jid, is_deleted) VALUES (?, ?, 1)`, messageID, chatJID); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE wamio_messages SET
		is_deleted = 1, content = '', media_type = '', media_duration = 0,
		file_name = '', mimetype = '', thumbnail = '', is_ptt = 0, caption = '',
		file_size = 0, kind = '', call_id = '', call_outcome = '', call_duration = 0,
		call_is_video = 0, call_is_incoming = 0
		WHERE chat_jid = ? AND id = ?`, chatJID, messageID); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM wamio_message_reactions WHERE chat_jid = ? AND message_id = ?`, chatJID, messageID); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE wamio_messages SET reply_is_deleted = 1 WHERE reply_chat_jid = ? AND reply_id = ?`, chatJID, messageID); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteMessageForMe removes a message from this account's local view while
// leaving any downloaded media file untouched.
func (cs *ChatStore) DeleteMessageForMe(chatJID, messageID string) error {
	tx, err := cs.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM wamio_message_reactions WHERE chat_jid = ? AND message_id = ?`, chatJID, messageID); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM wamio_messages WHERE chat_jid = ? AND id = ?`, chatJID, messageID); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteChatCache removes a filtered chat and all derived messages.
func (cs *ChatStore) DeleteChatCache(jid string) error {
	_, err := cs.db.Exec(`DELETE FROM wamio_message_reactions WHERE chat_jid = ?; DELETE FROM wamio_messages WHERE chat_jid = ?; DELETE FROM wamio_chats WHERE jid = ?`, jid, jid, jid)
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
		(id, chat_jid, sender_jid, sender_name, content, timestamp, is_from_me, is_read, media_type, media_duration, file_name, mimetype, thumbnail, is_ptt, delivery_status, reply_id, reply_chat_jid, reply_sender_jid, reply_sender_name, reply_content, reply_media_type, reply_from_me, reply_is_deleted, is_forwarded, is_deleted, caption, file_size, kind, call_id, call_outcome, call_duration, call_is_video, call_is_incoming)
		SELECT id, ?, sender_jid, sender_name, content, timestamp, is_from_me, is_read, media_type, media_duration, file_name, mimetype, thumbnail, is_ptt, delivery_status, reply_id, reply_chat_jid, reply_sender_jid, reply_sender_name, reply_content, reply_media_type, reply_from_me, reply_is_deleted, is_forwarded, is_deleted, caption, file_size, kind, call_id, call_outcome, call_duration, call_is_video, call_is_incoming
		FROM wamio_messages WHERE chat_jid = ?`, toJID, fromJID); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM wamio_messages WHERE chat_jid = ?`, fromJID); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT OR IGNORE INTO wamio_message_reactions (chat_jid, message_id, reactor_jid, emoji, timestamp)
		SELECT ?, message_id, reactor_jid, emoji, timestamp FROM wamio_message_reactions WHERE chat_jid = ?`, toJID, fromJID); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM wamio_message_reactions WHERE chat_jid = ?`, fromJID); err != nil {
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

// UpsertChatList persists a server-defined list. Lists are intentionally
// read-only in Wamio; mutations only originate from WhatsApp app-state sync.
func (cs *ChatStore) UpsertChatList(list ChatListRow) error {
	_, err := cs.db.Exec(`INSERT INTO wamio_chat_lists (id, name, type, sort_order, is_active)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, type=excluded.type,
		sort_order=excluded.sort_order, is_active=excluded.is_active`,
		list.ID, list.Name, list.Type, list.Order, boolToInt(list.IsActive))
	return err
}

func (cs *ChatStore) GetChatLists() ([]ChatListRow, error) {
	rows, err := cs.db.Query(`SELECT id, name, type, sort_order, is_active FROM wamio_chat_lists ORDER BY sort_order, name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lists []ChatListRow
	for rows.Next() {
		var list ChatListRow
		var active int
		if err := rows.Scan(&list.ID, &list.Name, &list.Type, &list.Order, &active); err != nil {
			return nil, err
		}
		list.IsActive = active != 0
		lists = append(lists, list)
	}
	return lists, rows.Err()
}

func (cs *ChatStore) SetChatListMembership(listID, chatJID string, member bool) error {
	if member {
		_, err := cs.db.Exec(`INSERT OR IGNORE INTO wamio_chat_list_memberships (list_id, chat_jid) VALUES (?, ?)`, listID, chatJID)
		return err
	}
	_, err := cs.db.Exec(`DELETE FROM wamio_chat_list_memberships WHERE list_id = ? AND chat_jid = ?`, listID, chatJID)
	return err
}

func (cs *ChatStore) GetChatListIDs(chatJID string) ([]string, error) {
	rows, err := cs.db.Query(`SELECT list_id FROM wamio_chat_list_memberships WHERE chat_jid = ? ORDER BY list_id`, chatJID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
