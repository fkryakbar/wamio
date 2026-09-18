import { create } from 'zustand';
import type { ChatItem, MessageItem, MessagePage, PresenceEvent, ChatPresenceEvent, SyncProgressEvent, MessageReaction, ChatList } from '../types';

interface PaginationMeta {
  oldestTimestampLoaded: number;
  hasMoreLocal: boolean;
  canRequestOlder: boolean;
  loadingMore: boolean;
}

interface ChatState {
  // State
  chats: ChatItem[];
	activeRailView: 'chats' | 'calls';
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
  presence: Record<string, PresenceEvent & { typing: boolean }>;

  // Actions
  setChats: (chats: ChatItem[]) => void;
	setActiveRailView: (view: 'chats' | 'calls') => void;
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
  prependMessagesPage: (chatJid: string, page: MessagePage) => void;
  setSearchQuery: (query: string) => void;
  setSyncing: (syncing: boolean) => void;
  setInitialSyncState: (state: 'running' | 'ready' | 'degraded', error?: string) => void;
  setLoadingMore: (chatJid: string, loading: boolean) => void;
  setChatAvatar: (jid: string, avatar: string) => void;
  addSyncProgress: (count: number) => void;
  setSyncProgress: (progress: SyncProgressEvent) => void;
  setPresence: (presence: PresenceEvent) => void;
  setChatPresence: (presence: ChatPresenceEvent) => void;
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
      return { chats: newChats };
    }),

  setActiveChat: (jid) => set({ activeChatJID: jid }),

	setChatLists: (chatLists) => set({ chatLists: chatLists.filter((list) => list.isActive) }),
	setActiveChatList: (activeChatListID) => set({ activeChatListID }),

  addMessage: (chatJid, message) =>
    set((state) => {
      const existing = state.messages[chatJid] || [];
			const optimisticIndex = message.clientRequestId
				? existing.findIndex((item) => item.clientRequestId === message.clientRequestId)
				: -1;
			if (optimisticIndex >= 0) {
				const next = [...existing];
				next[optimisticIndex] = message;
				return { messages: { ...state.messages, [chatJid]: next } };
			}
      if (
        existing.some(
          (m) =>
            (m.id && m.id === message.id) ||
            (m.timestamp === message.timestamp &&
              m.content === message.content &&
              m.isFromMe === message.isFromMe)
        )
      ) {
        return state;
      }
      return {
        messages: {
          ...state.messages,
          [chatJid]: [...existing, message],
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
        messages: { ...state.messages, [message.chatJid]: [...existing, message] },
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
      const existing = new Map((state.messages[chatJid] || []).map((message) => [message.id, message]));
      const seen = new Set<string>();
      const deduped = messages.filter((m) => {
        const key = m.id || `${m.timestamp}-${m.content}-${m.isFromMe}`;
        if (seen.has(key)) return false;
        seen.add(key);
        return true;
      });
      return {
        messages: {
          ...state.messages,
          [chatJid]: deduped.map((message) => keepNewestDeliveryStatus(existing.get(message.id), message)),
        },
      };
    }),

  mergeMessages: (chatJid, incoming) =>
    set((state) => {
      const byID = new Map<string, MessageItem>();
      for (const message of state.messages[chatJid] || []) byID.set(message.id, message);
      for (const message of incoming) {
        const previous = byID.get(message.id);
        byID.set(message.id, keepNewestDeliveryStatus(previous, { ...previous, ...message }));
      }
      const merged = Array.from(byID.values()).sort((first, second) =>
        first.timestamp === second.timestamp ? first.id.localeCompare(second.id) : first.timestamp - second.timestamp
      );
      const oldest = merged[0]?.timestamp ?? state.pagination[chatJid]?.oldestTimestampLoaded ?? 0;
      return {
        messages: { ...state.messages, [chatJid]: merged },
        pagination: {
          ...state.pagination,
          [chatJid]: {
            ...state.pagination[chatJid],
            oldestTimestampLoaded: oldest,
            hasMoreLocal: state.pagination[chatJid]?.hasMoreLocal ?? false,
            canRequestOlder: state.pagination[chatJid]?.canRequestOlder ?? false,
            loadingMore: state.pagination[chatJid]?.loadingMore ?? false,
          },
        },
      };
    }),

  setInitialMessages: (chatJid, page) =>
    set((state) => {
      const msgs = page.messages;
      const existing = new Map((state.messages[chatJid] || []).map((message) => [message.id, message]));
      const seen = new Set<string>();
      const deduped = msgs.filter((m) => {
        const key = m.id || `${m.timestamp}-${m.content}-${m.isFromMe}`;
        if (seen.has(key)) return false;
        seen.add(key);
        return true;
      });
      const oldest = deduped[0]?.timestamp ?? 0;
      return {
        messages: {
          ...state.messages,
          [chatJid]: deduped.map((message) => keepNewestDeliveryStatus(existing.get(message.id), message)),
        },
        pagination: {
          ...state.pagination,
          [chatJid]: {
            oldestTimestampLoaded: oldest,
            hasMoreLocal: page.hasMoreLocal,
            canRequestOlder: page.canRequestOlder,
            loadingMore: false,
          },
        },
      };
    }),

  prependMessagesPage: (chatJid, pageResult) =>
    set((state) => {
      const msgs = pageResult.messages;
      const existing = state.messages[chatJid] || [];
      const existingIds = new Set(existing.map((m) => m.id).filter(Boolean));
      const newMsgs = msgs.filter((m) => !existingIds.has(m.id));
      if (newMsgs.length === 0) {
        const page = state.pagination[chatJid];
        return {
          pagination: {
            ...state.pagination,
            [chatJid]: {
              oldestTimestampLoaded: page?.oldestTimestampLoaded ?? 0,
              hasMoreLocal: pageResult.hasMoreLocal,
              canRequestOlder: pageResult.canRequestOlder,
              loadingMore: false,
            },
          },
        };
      }

      const merged = [...newMsgs, ...existing];
      const page = state.pagination[chatJid];
      const oldest = newMsgs[0]?.timestamp ?? page?.oldestTimestampLoaded ?? 0;

      return {
        messages: {
          ...state.messages,
          [chatJid]: merged,
        },
        pagination: {
          ...state.pagination,
          [chatJid]: {
            oldestTimestampLoaded: oldest,
            hasMoreLocal: pageResult.hasMoreLocal,
            canRequestOlder: pageResult.canRequestOlder,
            loadingMore: false,
          },
        },
      };
    }),

  setSearchQuery: (query) => set({ searchQuery: query }),

  setSyncing: (syncing) => set({ isSyncing: syncing }),

  setInitialSyncState: (state, error = '') => set({ initialSyncState: state, initialSyncError: error }),

  setLoadingMore: (chatJid, loading) =>
    set((state) => ({
      pagination: {
        ...state.pagination,
        [chatJid]: {
          ...state.pagination[chatJid],
          oldestTimestampLoaded: state.pagination[chatJid]?.oldestTimestampLoaded ?? 0,
          hasMoreLocal: state.pagination[chatJid]?.hasMoreLocal ?? false,
          canRequestOlder: state.pagination[chatJid]?.canRequestOlder ?? false,
          loadingMore: loading,
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

  setSyncProgress: (syncProgress) => set({
    syncProgress,
    isSyncing: syncProgress.phase === 'initial' || syncProgress.phase === 'background',
    syncProgressCount: syncProgress.processedChats,
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

  filteredChats: () => {
    const { chats, searchQuery, activeChatListID } = get();
    let visible = chats;
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
