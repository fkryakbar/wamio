package store

import (
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
