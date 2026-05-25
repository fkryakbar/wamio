# Progressive Sync Design (WhatsApp Desktop)

## Goal
Percepat startup dan buat sinkronisasi pesan lebih efisien: saat login hanya sync data awal terbatas, lalu histori lama dimuat **on-demand** per chat saat user scroll ke atas.

## User-approved decisions
1. Initial preload per chat: **20 message terbaru**.
2. Backfill per chat saat user membuka/scroll: **batch 100**.
3. Initial loading behavior: **strict gate** — UI chat list/chat view tidak ditampilkan sampai initial sync pertama selesai.

## Problem statement
Saat ini history sync datang bertahap dalam batch besar dan dapat berlangsung lama. UI bisa terlihat “terus bertambah” karena data terus masuk, dan startup terasa berat/noisy. Dibutuhkan strategi sync bertingkat agar:
- startup responsif dan terkontrol,
- data lama tetap tersedia saat dibutuhkan,
- traffic/CPU/memori tidak meledak di awal.

## Scope
In-scope:
- Flow initial sync ringan + loading gate.
- Progressive/on-demand backfill per chat.
- State management frontend untuk pagination/cursor per chat.
- Event contract backend ↔ frontend untuk status sync.

Out-of-scope:
- Migrasi besar API whatsmeow deprecated.
- Refactor total arsitektur event yang tidak terkait sync.

## High-level architecture

### 1) Initial sync gate (global)
Tambahkan status initial sync global yang mengontrol render app utama.

- Backend emit event baru: `wa:initial-sync` dengan state minimal:
  - `running`
  - `done`
- Frontend menahan render `Sidebar` + `ChatView` sampai menerima `done`.
- Selama `running`, tampilkan full-screen loading: “Menyinkronkan pesan pertama...”.

**Strict behavior (approved):** tidak ada fallback timeout ke chat list. UI tetap loading sampai initial sync berhasil.

### 2) Initial data policy
Saat session connected:
- Ambil daftar chat + metadata terbaru.
- Untuk tiap chat, preload maksimal **20 message terbaru** (dari persistence/cache lokal terlebih dahulu).
- Ketika baseline ini siap, backend emit `wa:initial-sync = done`.

### 3) On-demand backfill per chat
Saat user membuka chat dan/atau scroll ke atas:
- Frontend request page lama: `limit=100`, berbasis cursor (`beforeTimestamp` atau cursor setara).
- Backend ambil dari DB dulu; jika perlu, lanjutkan sinkronisasi tambahan sesuai kemampuan sumber data.
- Hasil di-prepend ke message list chat terkait, dengan dedup.

## Backend design

Target area:
- `internal/whatsapp/service.go`
- `internal/store/chatstore.go`
- (opsional) `internal/whatsapp/types.go` untuk event payload type

### New/updated backend behaviors
1. **Initial sync lifecycle**
   - Emit `wa:initial-sync(running)` ketika mulai baseline sync.
   - Emit `wa:initial-sync(done)` setelah baseline siap (chat list + preload 20/msg per chat).

2. **Paged message fetch API**
   - Tambah method bound ke frontend, contoh:
     - `GetMessagesPage(chatJID string, limit int, beforeTimestamp int64) []MessageItem`
   - Batas default/maks untuk `limit` tetap aman (default 100).

3. **DB-first retrieval**
   - Query histori dari `chatStore` dulu sebagai sumber utama pagination.
   - Jaga urutan pesan stabil (ascending untuk render, tapi selection window berdasarkan cursor oldest).

4. **Concurrency guards**
   - Hindari fetch ganda per chat (single in-flight/backfill lock per chat atau guard serupa).

## Frontend design

Target area:
- `frontend/src/App.tsx`
- `frontend/src/stores/chatStore.ts`
- `frontend/src/components/ChatView.tsx`
- `frontend/src/types/index.ts`

### App-level gating
- Tambah state initial sync di store (atau app state terpusat): `running | done`.
- `App.tsx`:
  - jika auth belum connected → `LoginPage`.
  - jika connected + initial sync belum done → full-screen loading.
  - jika connected + done → render normal (`Sidebar` + `ChatView`).

### Chat store additions
Per chat, simpan pagination metadata:
- `oldestTimestampLoaded`
- `hasMore`
- `loadingMore`

Actions baru:
- `setInitialMessages(chatJID, messages)` (seed 20 awal)
- `prependMessagesPage(chatJID, messages, nextCursor, hasMore)`
- `setLoadingMore(chatJID, boolean)`

### ChatView infinite scroll
- Trigger backfill ketika scroll mendekati top.
- Jika `loadingMore` true atau `hasMore` false, jangan request baru.
- Request: `GetMessagesPage(chatJID, 100, oldestTimestampLoaded)`.
- Prepend hasil + dedup (id utama, fallback timestamp+content+isFromMe).

## Data flow
1. User login → backend connected.
2. Backend emit `wa:initial-sync(running)`.
3. Backend siapkan baseline (chat list + preload 20 tiap chat).
4. Backend emit `wa:initial-sync(done)`.
5. Frontend buka UI utama.
6. User buka chat/scroll top → frontend request page 100 by demand.
7. Backend return page; frontend prepend + update cursor/hasMore.
8. Incoming real-time message tetap append via event `wa:message`.

## Error handling
- Initial sync stage:
  - karena strict gate, backend harus retry internal sampai sukses (atau expose status error terkontrol tapi tetap di loading, tanpa masuk chat list).
- Backfill stage:
  - jika request gagal, set `loadingMore=false`, tampilkan indikator ringan di chat (non-blocking), user bisa retry via scroll ulang.

## Verification plan
1. **Startup gate**
   - Login successful.
   - Pastikan UI tetap di loading sampai event `wa:initial-sync(done)`.
   - Setelah done, chat UI tampil sekaligus.

2. **Initial payload**
   - Buka beberapa chat setelah done.
   - Validasi masing-masing punya baseline terbaru (target 20).

3. **On-demand pagination**
   - Scroll top berulang pada 1 chat.
   - Setiap trigger menambah ~100 pesan (atau sisa) tanpa duplikasi/lompatan urutan.

4. **Realtime coexistence**
   - Saat backfill berjalan, kirim/terima pesan baru.
   - Pastikan pesan baru tetap append real-time.

5. **Regression checks**
   - `npm run build --prefix frontend`
   - `go test ./...`

## Trade-offs
- Pro:
  - Startup lebih terkontrol.
  - UX terasa cepat setelah gate selesai.
  - Histori lama hanya dimuat saat perlu.
- Kontra:
  - Kompleksitas state/cursor bertambah.
  - Strict gate berisiko loading lama jika sumber sync lambat.

## Rollout strategy
1. Implement event initial sync gate.
2. Implement paged API + DB-first pagination.
3. Implement frontend loading gate + infinite scroll backfill.
4. Verify E2E pada akun dengan histori besar.
