import { create } from 'zustand';
import type { ChatItem, MessageItem, MessagePage, MessageCursor, MessageLoadProgress, HistoryState, HistoryPageEvent, PresenceEvent, ChatPresenceEvent, SyncProgressEvent, MessageReaction, ChatList, ReadAckResult } from '../types';

export type RailView = 'chats' | 'calls' | 'archived';

interface PaginationMeta {
  oldestCursor?: MessageCursor;
  anchorMessageId?: string;
  hasMoreLocal: boolean;
  canRequestOlder: boolean;
  loadingMore: boolean;
  historyState: HistoryState;
  requestId?: string;
  progress?: MessageLoadProgress;
  error?: string;
}

interface ChatState {
  // State
  chats: ChatItem[];
	activeRailView: RailView;
	chatLists: ChatList[];
	activeChatListID: string;
  activeChatJID: string | null;
  messages: Record<string, MessageItem[]>; // keyed by chat JID
  searchQuery: string;
  isSyncing: boolean;
  initialSyncState: 'running' | 'ready' | 'degraded';
  initialSyncError: string;
  pagination: Record<string, PaginationMeta>;
  syncProgressCount: number;
  syncProgress: SyncProgressEvent | null;
  syncStartedAt: number | null;
  presence: Record<string, PresenceEvent & { typing: boolean }>;

  // Actions
  setChats: (chats: ChatItem[]) => void;
	setActiveRailView: (view: RailView) => void;
	setChatLists: (lists: ChatList[]) => void;
	setActiveChatList: (id: string) => void;
  updateChat: (chat: ChatItem) => void;
  setActiveChat: (jid: string | null) => void;
  addMessage: (chatJid: string, message: MessageItem) => void;
  addOptimisticMessage: (message: MessageItem, preview: string) => void;
  markOutgoingFailed: (chatJid: string, clientRequestId: string) => void;
  retryOutgoing: (chatJid: string, previousRequestId: string, clientRequestId: string) => void;
  updateMessageReceipts: (chatJid: string, ids: string[], status: 'delivered' | 'read') => void;
  updateMessageReactions: (chatJid: string, messageId: string, reactions: MessageReaction[]) => void;
  removeMessage: (chatJid: string, messageId: string, forMe: boolean) => void;
  setMessages: (chatJid: string, messages: MessageItem[]) => void;
  mergeMessages: (chatJid: string, messages: MessageItem[]) => void;
  setInitialMessages: (chatJid: string, page: MessagePage) => void;
  beginChatOpen: (chatJid: string, requestId: string) => void;
  prependMessagesPage: (chatJid: string, page: MessagePage) => void;
  applyHistoryPage: (event: HistoryPageEvent) => void;
  setSearchQuery: (query: string) => void;
  setSyncing: (syncing: boolean) => void;
  setInitialSyncState: (state: 'running' | 'ready' | 'degraded', error?: string) => void;
  setLoadingMore: (chatJid: string, loading: boolean) => void;
  setChatAvatar: (jid: string, avatar: string) => void;
  addSyncProgress: (count: number) => void;
  setSyncProgress: (progress: SyncProgressEvent) => void;
  setPresence: (presence: PresenceEvent) => void;
  setChatPresence: (presence: ChatPresenceEvent) => void;
  applyReadResult: (chatJid: string, result: ReadAckResult) => void;
  reset: () => void;

  // Computed
  filteredChats: () => ChatItem[];
  activeChat: () => ChatItem | undefined;
}

function deliveryStatusRank(status: ChatItem['lastMessageStatus'] | undefined): number {
  switch (status) {
    case 'read': return 3;
    case 'delivered': return 2;
    case 'sent': return 1;
    default: return 0;
  }
}

function strongestDeliveryStatus(
  first: ChatItem['lastMessageStatus'] | undefined,
  second: ChatItem['lastMessageStatus'] | undefined,
): ChatItem['lastMessageStatus'] {
  return deliveryStatusRank(first) >= deliveryStatusRank(second) ? (first || '') : (second || '');
}

function keepNewestDeliveryStatus(existing: MessageItem | undefined, incoming: MessageItem): MessageItem {
  if (!existing || !existing.isFromMe || !incoming.isFromMe) return incoming;
  return {
    ...incoming,
    deliveryStatus: strongestDeliveryStatus(existing.deliveryStatus, incoming.deliveryStatus),
    isRead: existing.isRead || incoming.isRead,
  };
}

export function compareMessages(first: MessageItem, second: MessageItem): number {
  if (first.timestamp !== second.timestamp) return first.timestamp - second.timestamp;
  const orderDifference = (first.order || 0) - (second.order || 0);
  if (orderDifference !== 0) return orderDifference;
  return first.id.localeCompare(second.id);
}

export function mergeMessageLists(existing: MessageItem[], incoming: MessageItem[]): MessageItem[] {
  const byID = new Map<string, MessageItem>();
  const optimisticByRequest = new Map<string, string>();
  for (const message of existing) {
    if (message.id) byID.set(message.id, message);
    if (message.clientRequestId) optimisticByRequest.set(message.clientRequestId, message.id);
  }
  for (const message of incoming) {
    const optimisticID = message.clientRequestId ? optimisticByRequest.get(message.clientRequestId) : undefined;
    const key = optimisticID || message.id;
    if (!key) continue;
    const previous = byID.get(key);
    const merged = keepNewestDeliveryStatus(previous, { ...previous, ...message });
    if (optimisticID && optimisticID !== message.id) byID.delete(optimisticID);
    byID.set(message.id || key, merged);
  }
  return Array.from(byID.values()).sort(compareMessages);
}

function isSamePreview(first: ChatItem, second: ChatItem): boolean {
  return first.lastMessageTime === second.lastMessageTime &&
    first.lastMessage === second.lastMessage &&
    first.lastMessageFromMe === second.lastMessageFromMe;
}

function sortChats(chats: ChatItem[]) { return chats.sort((a, b) => Number(b.isPinned) - Number(a.isPinned) || b.lastMessageTime - a.lastMessageTime); }

export const useChatStore = create<ChatState>((set, get) => ({
  chats: [],
	activeRailView: 'chats',
	chatLists: [],
	activeChatListID: 'all',
  activeChatJID: null,
  messages: {},
  searchQuery: '',
  isSyncing: true,
  initialSyncState: 'running',
  initialSyncError: '',
  pagination: {},
  syncProgressCount: 0,
  syncProgress: null,
  syncStartedAt: Date.now(),
  presence: {},

  setChats: (incoming) =>
    set((state) => {
      const chatMap = new Map<string, ChatItem>();
      // GetChats and wa:chats-sync are complete backend snapshots. Their
      // unread state is authoritative, including reads made on other devices.
      for (const c of incoming) {
        const existing = state.chats.find((chat) => chat.jid === c.jid);
        chatMap.set(c.jid, {
          ...c,
          // Avatars are fetched client-side and are not part of GetChats.
          avatar: c.avatar || existing?.avatar,
          lastMessageStatus: c.lastMessageFromMe
            ? strongestDeliveryStatus(existing?.lastMessageStatus, c.lastMessageStatus)
            : c.lastMessageStatus,
        });
      }
	      const merged = sortChats(Array.from(chatMap.values()));
      return { chats: merged };
    }),

	setActiveRailView: (activeRailView) => set({ activeRailView }),

  updateChat: (chat) =>
    set((state) => {
      const idx = state.chats.findIndex((c) => c.jid === chat.jid);
      const newChats = [...state.chats];
      if (idx >= 0) {
        const existing = newChats[idx];
        newChats[idx] = isSamePreview(existing, chat) && chat.lastMessageFromMe
          ? { ...chat, lastMessageStatus: strongestDeliveryStatus(existing.lastMessageStatus, chat.lastMessageStatus) }
          : chat;
      } else {
        newChats.unshift(chat);
      }
	      sortChats(newChats);
      const activeRailView = state.activeChatJID === chat.jid && state.activeRailView !== 'calls'
        ? (chat.isArchived ? 'archived' : 'chats')
        : state.activeRailView;
      return { chats: newChats, activeRailView };
    }),

  setActiveChat: (jid) => set({ activeChatJID: jid }),

	setChatLists: (chatLists) => set({ chatLists: chatLists.filter((list) => list.isActive) }),
	setActiveChatList: (activeChatListID) => set({ activeChatListID }),

  addMessage: (chatJid, message) =>
    set((state) => {
      const existing = state.messages[chatJid] || [];
      return {
        messages: {
          ...state.messages,
          [chatJid]: mergeMessageLists(existing, [message]),
        },
      };
    }),

  addOptimisticMessage: (message, preview) =>
    set((state) => {
      const existing = state.messages[message.chatJid] || [];
      const chats = state.chats.map((chat) => chat.jid === message.chatJid
        ? {
            ...chat,
            lastMessage: preview,
            lastMessageTime: message.timestamp,
            lastMessageFromMe: true,
            lastMessageStatus: 'pending',
          }
        : chat);
      return {
        messages: { ...state.messages, [message.chatJid]: mergeMessageLists(existing, [message]) },
        chats: sortChats(chats),
      };
    }),

  markOutgoingFailed: (chatJid, clientRequestId) =>
    set((state) => ({
      messages: {
        ...state.messages,
        [chatJid]: (state.messages[chatJid] || []).map((message) =>
          message.clientRequestId === clientRequestId
            ? { ...message, localState: 'failed', deliveryStatus: 'failed' }
            : message
        ),
      },
      chats: state.chats.map((chat) => chat.jid === chatJid && chat.lastMessageStatus === 'pending'
        ? { ...chat, lastMessageStatus: 'failed' }
        : chat),
    })),

  retryOutgoing: (chatJid, previousRequestId, clientRequestId) =>
    set((state) => ({
      messages: {
        ...state.messages,
        [chatJid]: (state.messages[chatJid] || []).map((message) =>
          message.clientRequestId === previousRequestId
            ? { ...message, id: `local:${clientRequestId}`, clientRequestId, localState: 'pending', deliveryStatus: 'pending' }
            : message
        ),
      },
      chats: state.chats.map((chat) => chat.jid === chatJid
        ? { ...chat, lastMessageStatus: 'pending' }
        : chat),
    })),

  updateMessageReceipts: (chatJid, ids, status) =>
    set((state) => ({
      messages: {
        ...state.messages,
        [chatJid]: (state.messages[chatJid] || []).map((message) =>
          message.isFromMe && ids.includes(message.id)
            ? {
                ...message,
                // Receipt stanzas can arrive out of order. A delayed
                // "delivered" must never turn a blue/read receipt back into
                // a delivered one in the conversation.
                deliveryStatus: strongestDeliveryStatus(message.deliveryStatus, status),
                isRead: message.isRead || status === 'read',
              }
            : message
        ),
      },
    })),

  updateMessageReactions: (chatJid, messageId, reactions) =>
    set((state) => ({
      messages: {
        ...state.messages,
        [chatJid]: (state.messages[chatJid] || []).map((message) =>
          message.id === messageId ? { ...message, reactions } : message
        ),
      },
    })),

  removeMessage: (chatJid, messageId, forMe) =>
    set((state) => {
      const current = state.messages[chatJid] || [];
      const nextMessages = forMe
        ? current.filter((message) => message.id !== messageId)
        : current.map((message) => message.id === messageId
	          ? { ...message, isDeleted: true, content: '', mediaType: '', fileName: '', mimetype: '', thumbnail: '', caption: '', reactions: [] }
          : message.replyTo?.id === messageId && message.replyTo.chatJid === chatJid
            ? { ...message, replyTo: { ...message.replyTo, content: 'Pesan ini telah dihapus' } }
            : message);
      return { messages: { ...state.messages, [chatJid]: nextMessages } };
    }),

  setMessages: (chatJid, messages) =>
    set((state) => {
      return {
        messages: {
          ...state.messages,
          [chatJid]: mergeMessageLists([], messages).map((message) =>
            keepNewestDeliveryStatus((state.messages[chatJid] || []).find((item) => item.id === message.id), message)),
        },
      };
    }),

  mergeMessages: (chatJid, incoming) =>
    set((state) => {
      const merged = mergeMessageLists(state.messages[chatJid] || [], incoming);
      const oldest = merged[0];
      return {
        messages: { ...state.messages, [chatJid]: merged },
        pagination: {
          ...state.pagination,
          [chatJid]: {
            ...state.pagination[chatJid],
            oldestCursor: oldest ? { timestamp: oldest.timestamp, order: oldest.order || 0, messageId: oldest.id, isFromMe: oldest.isFromMe } : state.pagination[chatJid]?.oldestCursor,
            hasMoreLocal: state.pagination[chatJid]?.hasMoreLocal ?? false,
            canRequestOlder: state.pagination[chatJid]?.canRequestOlder ?? false,
            loadingMore: state.pagination[chatJid]?.loadingMore ?? false,
            historyState: state.pagination[chatJid]?.historyState ?? 'unknown',
          },
        },
      };
    }),

  setInitialMessages: (chatJid, page) =>
    set((state) => {
      const merged = mergeMessageLists([], page.messages);
      return {
        messages: {
          ...state.messages,
          [chatJid]: merged,
        },
        pagination: {
          ...state.pagination,
          [chatJid]: {
            oldestCursor: page.oldestCursor,
            anchorMessageId: page.anchorMessageId,
            hasMoreLocal: page.hasMoreLocal,
            canRequestOlder: page.canRequestOlder,
            loadingMore: page.progress?.phase === 'remote',
            historyState: page.historyState,
            requestId: page.requestId,
            progress: page.progress,
            error: page.error,
          },
        },
      };
    }),

  beginChatOpen: (chatJid, requestId) =>
    set((state) => ({
      pagination: {
        ...state.pagination,
        [chatJid]: {
          ...state.pagination[chatJid],
          anchorMessageId: undefined,
          hasMoreLocal: state.pagination[chatJid]?.hasMoreLocal ?? false,
          canRequestOlder: state.pagination[chatJid]?.canRequestOlder ?? false,
          loadingMore: true,
          historyState: state.pagination[chatJid]?.historyState ?? 'unknown',
          requestId,
          error: undefined,
        },
      },
    })),

  prependMessagesPage: (chatJid, pageResult) =>
    set((state) => {
      const existing = state.messages[chatJid] || [];
      const merged = mergeMessageLists(existing, pageResult.messages);

      return {
        messages: {
          ...state.messages,
          [chatJid]: merged,
        },
        pagination: {
          ...state.pagination,
          [chatJid]: {
            oldestCursor: pageResult.oldestCursor || state.pagination[chatJid]?.oldestCursor,
            anchorMessageId: state.pagination[chatJid]?.anchorMessageId,
            hasMoreLocal: pageResult.hasMoreLocal,
            canRequestOlder: pageResult.canRequestOlder,
            loadingMore: pageResult.progress?.phase === 'remote',
            historyState: pageResult.historyState,
            requestId: pageResult.requestId || state.pagination[chatJid]?.requestId,
            progress: pageResult.progress,
            error: pageResult.error,
          },
        },
      };
    }),

  applyHistoryPage: (event) =>
    set((state) => {
      const current = state.pagination[event.chatJid];
      if (event.requestId && current?.requestId && event.requestId !== current.requestId) return state;
      const merged = mergeMessageLists(state.messages[event.chatJid] || [], event.messages || []);
      return {
        messages: { ...state.messages, [event.chatJid]: merged },
        pagination: {
          ...state.pagination,
          [event.chatJid]: {
            oldestCursor: event.oldestCursor || current?.oldestCursor,
            anchorMessageId: current?.anchorMessageId,
            hasMoreLocal: false,
            canRequestOlder: event.canRequestOlder,
            loadingMore: event.progress?.phase === 'remote' || event.progress?.phase === 'merge',
            historyState: event.historyState,
            requestId: event.requestId || current?.requestId,
            progress: event.progress,
            error: event.error,
          },
        },
      };
    }),

  setSearchQuery: (query) => set({ searchQuery: query }),

  setSyncing: (syncing) => set((state) => ({
    isSyncing: syncing,
    syncStartedAt: syncing ? (state.isSyncing && state.syncStartedAt ? state.syncStartedAt : Date.now()) : null,
  })),

  setInitialSyncState: (state, error = '') => set({ initialSyncState: state, initialSyncError: error }),

  setLoadingMore: (chatJid, loading) =>
    set((state) => ({
      pagination: {
        ...state.pagination,
        [chatJid]: {
          ...state.pagination[chatJid],
          oldestCursor: state.pagination[chatJid]?.oldestCursor,
          anchorMessageId: state.pagination[chatJid]?.anchorMessageId,
          hasMoreLocal: state.pagination[chatJid]?.hasMoreLocal ?? false,
          canRequestOlder: state.pagination[chatJid]?.canRequestOlder ?? false,
          loadingMore: loading,
          historyState: state.pagination[chatJid]?.historyState ?? 'unknown',
        },
      },
    })),

  reset: () =>
    set({
      chats: [],
		activeRailView: 'chats',
		chatLists: [],
		activeChatListID: 'all',
      activeChatJID: null,
      messages: {},
      searchQuery: '',
      isSyncing: true,
      initialSyncState: 'running',
      initialSyncError: '',
      pagination: {},
      syncProgressCount: 0,
      syncProgress: null,
      syncStartedAt: Date.now(),
      presence: {},
    }),

  setChatAvatar: (jid, avatar) =>
    set((state) => ({
      chats: state.chats.map((c) =>
        c.jid === jid ? { ...c, avatar } : c
      ),
    })),

  addSyncProgress: (count) =>
    set((state) => ({
      syncProgressCount: state.syncProgressCount + count,
    })),

  setSyncProgress: (syncProgress) => set((state) => {
    const syncing = syncProgress.phase === 'initial' || syncProgress.phase === 'background';
    return {
      syncProgress,
      isSyncing: syncing,
      syncStartedAt: syncing
        ? (state.isSyncing && state.syncStartedAt ? state.syncStartedAt : Date.now())
        : null,
      syncProgressCount: syncProgress.processedChats,
    };
  }),

  setPresence: (event) => set((state) => ({
    presence: {
      ...state.presence,
      [event.chatJid]: { ...event, typing: state.presence[event.chatJid]?.typing ?? false },
    },
  })),

  setChatPresence: (event) => set((state) => ({
    presence: {
      ...state.presence,
      [event.chatJid]: {
        ...(state.presence[event.chatJid] || { chatJid: event.chatJid, online: false, unavailable: false, lastSeen: 0 }),
        typing: event.typing,
      },
    },
  })),

  applyReadResult: (chatJid, result) => set((state) => {
    const readIDs = new Set(result.messageIds || []);
    return {
      messages: {
        ...state.messages,
        [chatJid]: (state.messages[chatJid] || []).map((message) =>
          readIDs.has(message.id) ? { ...message, isRead: true } : message),
      },
      chats: state.chats.map((chat) =>
        chat.jid === chatJid ? { ...chat, unreadCount: result.unreadCount } : chat),
    };
  }),

  filteredChats: () => {
    const { chats, searchQuery, activeChatListID, activeRailView } = get();
		let visible = activeRailView === 'archived'
      ? chats.filter((chat) => chat.isArchived)
      : chats.filter((chat) => !chat.isArchived);
    if (activeChatListID === 'unread') visible = visible.filter((chat) => chat.unreadCount > 0);
    else if (activeChatListID === 'groups') visible = visible.filter((chat) => chat.isGroup);
    else if (activeChatListID !== 'all') visible = visible.filter((chat) => chat.listIds?.includes(activeChatListID));
    if (!searchQuery.trim()) return visible;
    const q = searchQuery.toLowerCase();
    return visible.filter(
      (c) =>
        c.name.toLowerCase().includes(q) ||
        c.lastMessage.toLowerCase().includes(q)
    );
  },

  activeChat: () => {
    const { chats, activeChatJID } = get();
    return chats.find((c) => c.jid === activeChatJID);
  },
}));
