package store

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"
)

func setupTestChatStore(t *testing.T) *ChatStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test_chatstore.db")
	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cs, err := NewChatStore(db.GetSQLDB())
	if err != nil {
		t.Fatalf("NewChatStore failed: %v", err)
	}
	return cs
}

func TestChatStoreRecentStickersAreOrderedAndBounded(t *testing.T) {
	cs := setupTestChatStore(t)
	for _, row := range []StickerRow{
		{ID: "older", Mimetype: "image/webp", LastUsedAt: 10, MediaKey: []byte{1}},
		{ID: "newer", Mimetype: "image/webp", LastUsedAt: 20, MediaKey: []byte{2}},
	} {
		if err := cs.UpsertRecentSticker(row); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := cs.GetRecentStickers(10)
	if err != nil || len(rows) != 2 || rows[0].ID != "newer" {
		t.Fatalf("GetRecentStickers rows=%+v err=%v", rows, err)
	}
	if err := cs.TrimRecentStickers(1); err != nil {
		t.Fatal(err)
	}
	rows, err = cs.GetRecentStickers(10)
	if err != nil || len(rows) != 1 || rows[0].ID != "newer" {
		t.Fatalf("TrimRecentStickers rows=%+v err=%v", rows, err)
	}
}

func seedMessages(t *testing.T, cs *ChatStore, chatJID string) {
	t.Helper()
	timestamps := []int64{100, 200, 300, 400, 500}
	for _, ts := range timestamps {
		err := cs.UpsertMessage(MessageRow{
			ID:        "msg-" + chatJID + "-" + string(rune(ts)),
			ChatJID:   chatJID,
			Timestamp: ts,
			Content:   "test content",
		})
		if err != nil {
			t.Fatalf("UpsertMessage failed: %v", err)
		}
	}
}

func TestChatStore_GetMessagesBefore(t *testing.T) {
	cs := setupTestChatStore(t)
	chatJID := "test@s.whatsapp.net"
	seedMessages(t, cs, chatJID)

	t.Run("returns oldest-first within page", func(t *testing.T) {
		rows, err := cs.GetMessagesBefore(chatJID, 500, 2)
		if err != nil {
			t.Fatalf("GetMessagesBefore failed: %v", err)
		}
		if len(rows) != 2 {
			t.Fatalf("expected 2 rows, got %d", len(rows))
		}
		if rows[0].Timestamp != 300 || rows[1].Timestamp != 400 {
			t.Errorf("expected [300, 400], got [%d, %d]", rows[0].Timestamp, rows[1].Timestamp)
		}
	})

	t.Run("respects beforeTimestamp cursor", func(t *testing.T) {
		rows, err := cs.GetMessagesBefore(chatJID, 300, 5)
		if err != nil {
			t.Fatalf("GetMessagesBefore failed: %v", err)
		}
		if len(rows) != 2 {
			t.Fatalf("expected 2 rows before 300, got %d", len(rows))
		}
		for _, r := range rows {
			if r.Timestamp >= 300 {
				t.Errorf("timestamp %d should be < 300", r.Timestamp)
			}
		}
	})

	t.Run("respects limit", func(t *testing.T) {
		rows, err := cs.GetMessagesBefore(chatJID, 999, 1)
		if err != nil {
			t.Fatalf("GetMessagesBefore failed: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("expected 1 row, got %d", len(rows))
		}
	})

	t.Run("returns empty when no older messages", func(t *testing.T) {
		rows, err := cs.GetMessagesBefore(chatJID, 50, 10)
		if err != nil {
			t.Fatalf("GetMessagesBefore failed: %v", err)
		}
		if len(rows) != 0 {
			t.Errorf("expected 0 rows, got %d", len(rows))
		}
	})

	t.Run("defaults limit when zero or negative", func(t *testing.T) {
		rows, err := cs.GetMessagesBefore(chatJID, 999, 0)
		if err != nil {
			t.Fatalf("GetMessagesBefore with limit=0 failed: %v", err)
		}
		if len(rows) != 5 {
			t.Errorf("expected all 5 rows with default limit, got %d", len(rows))
		}
	})
}

func TestChatStore_GetMessagesBeforeCursorKeepsSameSecondMessages(t *testing.T) {
	cs := setupTestChatStore(t)
	chatJID := "cursor@s.whatsapp.net"
	for _, row := range []MessageRow{
		{ID: "a", ChatJID: chatJID, Timestamp: 100, MessageOrder: 10, Content: "same second"},
		{ID: "b", ChatJID: chatJID, Timestamp: 100, MessageOrder: 20, Content: "same second"},
		{ID: "c", ChatJID: chatJID, Timestamp: 100, MessageOrder: 20, Content: "same second"},
		{ID: "d", ChatJID: chatJID, Timestamp: 101, MessageOrder: 1, Content: "newer"},
	} {
		if err := cs.UpsertMessage(row); err != nil {
			t.Fatal(err)
		}
	}

	rows, err := cs.GetMessagesBeforeCursor(chatJID, MessageCursor{Timestamp: 100, Order: 20, MessageID: "c"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ID != "a" || rows[1].ID != "b" {
		t.Fatalf("expected canonical cursor to return [a b], got %#v", rows)
	}
}

func TestChatStore_MarkIncomingMessagesReadOnlyMarksVisibleIDs(t *testing.T) {
	cs := setupTestChatStore(t)
	chatJID := "visible@s.whatsapp.net"
	if err := cs.UpsertChat(ChatRow{JID: chatJID, UnreadCount: 3}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"visible", "below-fold", "outgoing"} {
		if err := cs.UpsertMessage(MessageRow{
			ID: id, ChatJID: chatJID, Timestamp: 100, IsFromMe: id == "outgoing", IsRead: id == "outgoing",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := cs.MarkIncomingMessagesRead(chatJID, []string{"visible", "outgoing"}, 123); err != nil {
		t.Fatal(err)
	}

	unread, err := cs.GetUnreadIncomingMessages(chatJID)
	if err != nil {
		t.Fatal(err)
	}
	if len(unread) != 1 || unread[0].ID != "below-fold" {
		t.Fatalf("expected only below-fold to remain unread, got %#v", unread)
	}
	unreadCount, err := cs.GetUnreadIncomingCount(chatJID)
	if err != nil || unreadCount != 1 {
		t.Fatalf("durable unread count = %d, err=%v", unreadCount, err)
	}
	chats, err := cs.GetAllChats()
	if err != nil || len(chats) != 1 {
		t.Fatalf("GetAllChats: rows=%d err=%v", len(chats), err)
	}
	if chats[0].UnreadCount != 1 || chats[0].LastReadAt != 123 {
		t.Fatalf("unexpected read checkpoint: %#v", chats[0])
	}
}

func TestChatStore_HistoryFailureBackoffAndReconnectReset(t *testing.T) {
	cs := setupTestChatStore(t)
	chatJID := "retry@s.whatsapp.net"
	if err := cs.UpsertChat(ChatRow{JID: chatJID, LastMessageID: "anchor"}); err != nil {
		t.Fatal(err)
	}
	first, err := cs.SetHistoryFailure(chatJID, "timeout", 100)
	if err != nil {
		t.Fatal(err)
	}
	second, err := cs.SetHistoryFailure(chatJID, "timeout again", 100)
	if err != nil {
		t.Fatal(err)
	}
	if first.RetryAt != 105 || second.RetryAt != 110 || second.FailureCount != 2 {
		t.Fatalf("unexpected retry schedule: first=%#v second=%#v", first, second)
	}
	if err := cs.ResetHistoryRetry(chatJID); err != nil {
		t.Fatal(err)
	}
	reset, err := cs.GetHistoryStatus(chatJID)
	if err != nil {
		t.Fatal(err)
	}
	if reset.State != HistoryMoreOnPrimary || reset.RetryAt != 0 || reset.FailureCount != 0 || reset.LastError != "" {
		t.Fatalf("retry state was not reset: %#v", reset)
	}
}

func TestChatStore_MessageContextPreservesThumbnail(t *testing.T) {
	cs := setupTestChatStore(t)
	chatJID := "test@s.whatsapp.net"
	for index := 1; index <= 5; index++ {
		row := MessageRow{ID: fmt.Sprintf("message-%d", index), ChatJID: chatJID, Timestamp: int64(index * 100), Content: "content"}
		if index == 3 {
			row.MediaType, row.Mimetype, row.Thumbnail, row.Waveform = "audio", "audio/ogg; codecs=opus", "thumb-data", []byte{3, 20, 80}
			row.MediaDuration, row.IsPTT = 12, true
		}
		if err := cs.UpsertMessage(row); err != nil {
			t.Fatalf("UpsertMessage: %v", err)
		}
	}
	context, err := cs.GetMessagesAround(chatJID, "message-3", 5)
	if err != nil {
		t.Fatalf("GetMessagesAround: %v", err)
	}
	if len(context) != 5 || context[2].ID != "message-3" {
		t.Fatalf("unexpected context: %#v", context)
	}
	if context[2].Thumbnail != "thumb-data" {
		t.Fatalf("thumbnail was not persisted: %#v", context[2])
	}
	if !bytes.Equal(context[2].Waveform, []byte{3, 20, 80}) || context[2].MediaDuration != 12 || !context[2].IsPTT {
		t.Fatalf("voice-note metadata was not persisted: %#v", context[2])
	}
}

func TestChatStorePreviewKeepsNewestMessageOrder(t *testing.T) {
	cs := setupTestChatStore(t)
	chat := ChatRow{JID: "test@s.whatsapp.net", LastMessage: "new", LastMessageTime: 100, LastMessageID: "new", LastMessageOrder: 20}
	if err := cs.UpsertChat(chat); err != nil {
		t.Fatal(err)
	}
	chat.LastMessage, chat.LastMessageID, chat.LastMessageOrder = "old", "old", 10
	if err := cs.UpsertChat(chat); err != nil {
		t.Fatal(err)
	}
	rows, err := cs.GetAllChats()
	if err != nil || len(rows) != 1 {
		t.Fatalf("GetAllChats: rows=%d err=%v", len(rows), err)
	}
	if rows[0].LastMessage != "new" || rows[0].LastMessageOrder != 20 {
		t.Fatalf("late history overwrote preview: %#v", rows[0])
	}
}

func TestChatStoreReceiptDoesNotDowngradeRead(t *testing.T) {
	cs := setupTestChatStore(t)
	chatJID := "test@s.whatsapp.net"
	if err := cs.UpsertMessage(MessageRow{ID: "outgoing", ChatJID: chatJID, IsFromMe: true, DeliveryStatus: "sent"}); err != nil {
		t.Fatal(err)
	}
	if err := cs.UpdateMessageReceipt(chatJID, []string{"outgoing"}, "read"); err != nil {
		t.Fatal(err)
	}
	if err := cs.UpdateMessageReceipt(chatJID, []string{"outgoing"}, "delivered"); err != nil {
		t.Fatal(err)
	}
	rows, err := cs.GetMessages(chatJID, 1)
	if err != nil || len(rows) != 1 {
		t.Fatalf("GetMessages: rows=%d err=%v", len(rows), err)
	}
	if rows[0].DeliveryStatus != "read" || !rows[0].IsRead {
		t.Fatalf("receipt regressed: %#v", rows[0])
	}
}

func TestChatStorePersistsPreviewMetadataAndArchive(t *testing.T) {
	cs := setupTestChatStore(t)
	chat := ChatRow{
		JID:               "test@s.whatsapp.net",
		LastMessage:       "pesan terkirim",
		LastMessageTime:   100,
		LastMessageID:     "outgoing",
		LastMessageFromMe: true,
		LastMessageStatus: "read",
		IsArchived:        true,
	}
	if err := cs.UpsertChat(chat); err != nil {
		t.Fatal(err)
	}

	// A stale history replay must not replace the read tick for the same
	// preview, while a current archive state remains persisted.
	chat.LastMessageStatus = "sent"
	if err := cs.UpsertChat(chat); err != nil {
		t.Fatal(err)
	}

	chats, err := cs.GetAllChats()
	if err != nil || len(chats) != 1 {
		t.Fatalf("GetAllChats: rows=%d err=%v", len(chats), err)
	}
	got := chats[0]
	if !got.LastMessageFromMe || got.LastMessageStatus != "read" || !got.IsArchived {
		t.Fatalf("preview metadata not persisted: %#v", got)
	}
}

func TestChatStoreIncomingReadPersistsAcrossHistoryUpsert(t *testing.T) {
	cs := setupTestChatStore(t)
	chatJID := "test@s.whatsapp.net"
	if err := cs.UpsertChat(ChatRow{JID: chatJID, UnreadCount: 1, LastMessageTime: 100}); err != nil {
		t.Fatal(err)
	}
	if err := cs.UpsertMessage(MessageRow{ID: "incoming", ChatJID: chatJID, SenderJID: chatJID, Timestamp: 100, IsRead: false}); err != nil {
		t.Fatal(err)
	}
	if err := cs.MarkIncomingMessagesRead(chatJID, []string{"incoming"}, 150); err != nil {
		t.Fatal(err)
	}
	// A late history replay must not restore a stale unread badge or make the
	// message unread again.
	if err := cs.UpsertChat(ChatRow{JID: chatJID, UnreadCount: 1, LastMessageTime: 100}); err != nil {
		t.Fatal(err)
	}
	if err := cs.UpsertMessage(MessageRow{ID: "incoming", ChatJID: chatJID, SenderJID: chatJID, Timestamp: 100, IsRead: false}); err != nil {
		t.Fatal(err)
	}
	chats, err := cs.GetAllChats()
	if err != nil || len(chats) != 1 {
		t.Fatalf("GetAllChats: rows=%d err=%v", len(chats), err)
	}
	if chats[0].UnreadCount != 0 || chats[0].LastReadAt != 150 {
		t.Fatalf("read boundary regressed: %#v", chats[0])
	}
	messages, err := cs.GetMessages(chatJID, 1)
	if err != nil || len(messages) != 1 || !messages[0].IsRead {
		t.Fatalf("message read state regressed: %#v err=%v", messages, err)
	}
}
