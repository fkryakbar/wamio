# Progressive Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement strict initial sync gating plus progressive per-chat message backfill (20 initial messages per chat, 100-message on-demand pages on scroll-top).

**Architecture:** Backend emits an explicit initial-sync lifecycle event and serves DB-first paginated message APIs. Frontend blocks main chat UI until initial sync completes, stores per-chat pagination metadata, and requests older pages only when users scroll up in an opened chat. Real-time message events continue to append normally.

**Tech Stack:** Go (Wails backend, SQLite chat store), React + TypeScript, Zustand, Wails generated JS bindings.

---

## File map and responsibilities

- `internal/whatsapp/types.go` (modify)
  - Add payload type for initial sync event and optional paginated response type.
- `internal/store/chatstore.go` (modify)
  - Add DB query for cursor-based pagination (`beforeTimestamp`, `limit`) in chronological output.
- `internal/whatsapp/service.go` (modify)
  - Emit `wa:initial-sync` lifecycle events.
  - Add/adjust message fetch methods for initial 20 and paged 100 backfill.
  - Keep DB-first retrieval and safe bounds for limits.
- `frontend/src/types/index.ts` (modify)
  - Add TS types for initial sync event and message page metadata.
- `frontend/src/stores/chatStore.ts` (modify)
  - Add initial sync gate state + per-chat pagination state and actions.
- `frontend/src/App.tsx` (modify)
  - Gate `Sidebar`/`ChatView` render until initial sync is done.
- `frontend/src/components/ChatView.tsx` (modify)
  - Trigger backfill on scroll-top with concurrency guards.
- `frontend/wailsjs/go/whatsapp/WhatsAppService.d.ts` and `.js` (generated)
  - Regenerated via `wails dev`/`wails generate module` after backend API changes.

---

### Task 1: Add backend pagination query and tests

**Files:**
- Modify: `internal/store/chatstore.go`
- Test: `internal/store/database_test.go`

- [ ] **Step 1: Write failing tests for paged query**

Add tests in `internal/store/database_test.go` covering:
- returns oldest-first order within page
- respects `beforeTimestamp` cursor
- respects `limit`
- returns empty when no older messages

```go
func TestChatStore_GetMessagesBefore(t *testing.T) {
	// setup temp db + chatstore
	// insert 5 messages with timestamps 100, 200, 300, 400, 500
	// query before=500 limit=2 => expect [300,400] (oldest-first in returned slice)
}
```

- [ ] **Step 2: Run targeted store test and verify it fails**

Run: `go test ./internal/store -run GetMessagesBefore -v`
Expected: FAIL because method does not exist yet.

- [ ] **Step 3: Implement cursor-based DB method in ChatStore**

Add method in `internal/store/chatstore.go`:

```go
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
	// scan rows (same pattern as GetMessages)
	// reverse before return to keep chronological order
}
```

- [ ] **Step 4: Run targeted store test and verify it passes**

Run: `go test ./internal/store -run GetMessagesBefore -v`
Expected: PASS.

- [ ] **Step 5: Commit Task 1**

```bash
git add internal/store/chatstore.go internal/store/database_test.go
git commit -m "feat: add cursor-based chat message pagination in store"
```

---

### Task 2: Add backend initial sync lifecycle + paged API

**Files:**
- Modify: `internal/whatsapp/types.go`
- Modify: `internal/whatsapp/service.go`
- Test: `internal/whatsapp/service_test.go`

- [ ] **Step 1: Write failing backend tests for new behavior**

Add tests in `internal/whatsapp/service_test.go` for:
- limit normalization (default/max bounds)
- paged fetch returns empty safely when store not ready
- initial sync state transitions call helper without panic

```go
func TestGetMessagesPage_DefaultLimit(t *testing.T) {
	svc := NewWhatsAppService()
	msgs := svc.GetMessagesPage("chat@jid", 0, 12345)
	if msgs == nil {
		t.Fatal("expected non-nil slice")
	}
}
```

- [ ] **Step 2: Run targeted whatsapp tests and verify fail**

Run: `go test ./internal/whatsapp -run GetMessagesPage -v`
Expected: FAIL because method/types not implemented.

- [ ] **Step 3: Define event payload and add API methods**

In `internal/whatsapp/types.go`, add:

```go
type InitialSyncEvent struct {
	State string `json:"state"` // running | done
}
```

In `internal/whatsapp/service.go`, add helpers:

```go
func (s *WhatsAppService) emitInitialSync(state string) {
	s.emitEvent("wa:initial-sync", InitialSyncEvent{State: state})
}
```

Call lifecycle points:
- emit `running` right before baseline load starts after connect
- emit `done` after baseline chats/messages are ready for frontend consumption

Add paged API:

```go
func (s *WhatsAppService) GetMessagesPage(chatJID string, limit int, beforeTimestamp int64) []MessageItem {
	if limit <= 0 { limit = 100 }
	if limit > 100 { limit = 100 }
	if s.chatStore == nil { return []MessageItem{} }
	if beforeTimestamp <= 0 {
		return s.GetMessages(chatJID, limit)
	}
	rows, err := s.chatStore.GetMessagesBefore(chatJID, beforeTimestamp, limit)
	if err != nil { return []MessageItem{} }
	// map rows -> []MessageItem
}
```

- [ ] **Step 4: Run targeted whatsapp tests and verify pass**

Run: `go test ./internal/whatsapp -run GetMessagesPage -v`
Expected: PASS.

- [ ] **Step 5: Commit Task 2**

```bash
git add internal/whatsapp/types.go internal/whatsapp/service.go internal/whatsapp/service_test.go
git commit -m "feat: emit initial sync lifecycle and add paged message API"
```

---

### Task 3: Add frontend types and store states for sync gate/pagination

**Files:**
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/stores/chatStore.ts`

- [ ] **Step 1: Write failing frontend type/store checks**

Add minimal TS usage in store (or unit test if test runner exists) that references:
- `InitialSyncEvent`
- per-chat pagination state map
- new actions (`setInitialSyncState`, `prependMessagesPage`, `setLoadingMore`)

If no test runner configured, use typecheck as failure detector.

- [ ] **Step 2: Run typecheck and verify fail**

Run: `npm run build --prefix frontend`
Expected: FAIL due to missing types/actions.

- [ ] **Step 3: Implement TS types + store state/actions**

In `frontend/src/types/index.ts`, add:

```ts
export interface InitialSyncEvent {
  state: 'running' | 'done';
}
```

In `frontend/src/stores/chatStore.ts`, add state:

```ts
initialSyncState: 'running' | 'done';
pagination: Record<string, {
  oldestTimestampLoaded: number;
  hasMore: boolean;
  loadingMore: boolean;
}>;
```

Add actions:

```ts
setInitialSyncState: (state) => set({ initialSyncState: state }),
setInitialMessages: (chatJid, msgs) => { /* seed + cursor init */ },
prependMessagesPage: (chatJid, msgs) => { /* prepend + dedup + cursor update */ },
setLoadingMore: (chatJid, loading) => { /* guard state */ },
```

- [ ] **Step 4: Run typecheck and verify pass**

Run: `npm run build --prefix frontend`
Expected: PASS.

- [ ] **Step 5: Commit Task 3**

```bash
git add frontend/src/types/index.ts frontend/src/stores/chatStore.ts
git commit -m "feat: add initial sync and pagination state in chat store"
```

---

### Task 4: Gate main UI with strict initial sync loading screen

**Files:**
- Modify: `frontend/src/App.tsx`

- [ ] **Step 1: Write failing behavior check via build + manual expectation**

Define acceptance check in code comments/notes for this task execution:
- connected + initialSyncState running => loading screen only
- connected + done => normal chat UI

- [ ] **Step 2: Implement initial sync event subscription + gate render**

Update `App.tsx`:

```tsx
const { initialSyncState, setInitialSyncState } = useChatStore();

useEffect(() => {
  const cancelInitialSync = EventsOn('wa:initial-sync', (data: InitialSyncEvent) => {
    setInitialSyncState(data.state);
  });
  return () => cancelInitialSync();
}, [setInitialSyncState]);

if (connectionState === 'connected' && initialSyncState !== 'done') {
  return <FullScreenLoading text="Menyinkronkan pesan pertama..." />;
}
```

- [ ] **Step 3: Run frontend build verification**

Run: `npm run build --prefix frontend`
Expected: PASS.

- [ ] **Step 4: Commit Task 4**

```bash
git add frontend/src/App.tsx
git commit -m "feat: gate chat UI until initial sync completes"
```

---

### Task 5: Implement ChatView on-demand backfill (scroll-top)

**Files:**
- Modify: `frontend/src/components/ChatView.tsx`
- Modify: `frontend/src/stores/chatStore.ts` (if minor selector/action adjustments needed)

- [ ] **Step 1: Write failing behavior check**

Define measurable manual checks:
- opening chat loads initial window
- scrolling to top triggers exactly one fetch when not loading
- fetched page prepends without duplicates

- [ ] **Step 2: Implement scroll-top trigger with in-flight guard**

In `ChatView.tsx` add scroll handler:

```tsx
const onScroll = async () => {
  const el = containerRef.current;
  if (!el || !activeChatJID) return;
  if (el.scrollTop > 80) return;

  const page = pagination[activeChatJID];
  if (!page || page.loadingMore || !page.hasMore) return;

  setLoadingMore(activeChatJID, true);
  try {
    const mod = await import('../../wailsjs/go/whatsapp/WhatsAppService');
    const rows = await mod.GetMessagesPage(activeChatJID, 100, page.oldestTimestampLoaded);
    prependMessagesPage(activeChatJID, rows);
  } finally {
    setLoadingMore(activeChatJID, false);
  }
};
```

Attach listener to `.chatview__messages` container.

- [ ] **Step 3: Add loading indicator for older messages**

Render top inline loader when `loadingMore` true for active chat.

- [ ] **Step 4: Run frontend build verification**

Run: `npm run build --prefix frontend`
Expected: PASS.

- [ ] **Step 5: Commit Task 5**

```bash
git add frontend/src/components/ChatView.tsx frontend/src/stores/chatStore.ts
git commit -m "feat: add on-demand message backfill on scroll-top"
```

---

### Task 6: Regenerate bindings and run full verification

**Files:**
- Generated: `frontend/wailsjs/go/whatsapp/WhatsAppService.d.ts`
- Generated: `frontend/wailsjs/go/whatsapp/WhatsAppService.js`
- Verify app runtime behavior

- [ ] **Step 1: Regenerate Wails bindings**

Run: `wails dev`
Expected: bindings regenerate successfully without compile errors.

- [ ] **Step 2: Run backend and frontend regression checks**

Run:
- `go test ./...`
- `npm run build --prefix frontend`

Expected: PASS.

- [ ] **Step 3: Run manual E2E verification**

Manual checks:
1. Login => app shows strict full-screen sync loading.
2. Before initial sync done, chat list is not visible.
3. After `done`, main UI appears.
4. Open chat, scroll top repeatedly => loads older messages in ~100 batches.
5. No duplicate messages and ordering stable.
6. Incoming real-time message still appears while backfill active.

- [ ] **Step 4: Commit Task 6**

```bash
git add frontend/wailsjs/go/whatsapp/WhatsAppService.d.ts frontend/wailsjs/go/whatsapp/WhatsAppService.js
git commit -m "chore: regenerate wails bindings for progressive sync APIs"
```

---

## Spec self-review checklist (completed)

1. **Spec coverage:**
   - Initial sync strict gate: Tasks 2, 3, 4, 6
   - Initial preload policy and lifecycle event: Task 2
   - On-demand pagination 100: Tasks 1, 2, 5
   - Frontend pagination/cursor state: Task 3
   - Verification requirements: Task 6

2. **Placeholder scan:**
   - No TODO/TBD markers in executable steps.
   - Each task includes concrete files, commands, and expected outcomes.

3. **Type consistency:**
   - Event name `wa:initial-sync` and states `running|done` are consistent across backend/frontend tasks.
   - Pagination API signature `GetMessagesPage(chatJID, limit, beforeTimestamp)` is consistent across tasks.
