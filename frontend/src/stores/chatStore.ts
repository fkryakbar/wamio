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
	setChatLists: (lists: ChatList[]) => void;
	setActiveChatList: (id: string) => void;
  updateChat: (chat: ChatItem) => void;
  setActiveChat: (jid: string | null) => void;
  addMessage: (chatJid: string, message: MessageItem) => void;
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

function isSamePreview(first: ChatItem, second: ChatItem): boolean {
  return first.lastMessageTime === second.lastMessageTime &&
    first.lastMessage === second.lastMessage &&
    first.lastMessageFromMe === second.lastMessageFromMe;
}

function sortChats(chats: ChatItem[]) { return chats.sort((a, b) => Number(b.isPinned) - Number(a.isPinned) || b.lastMessageTime - a.lastMessageTime); }

export const useChatStore = create<ChatState>((set, get) => ({
  chats: [],
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
      for (const c of state.chats) {
        chatMap.set(c.jid, c);
      }
      for (const c of incoming) {
        const existing = chatMap.get(c.jid);
        if (existing) {
          const samePreview = isSamePreview(existing, c);
          if (c.lastMessageTime > existing.lastMessageTime ||
              samePreview) {
            chatMap.set(c.jid, {
              ...c,
              lastMessageStatus: samePreview && c.lastMessageFromMe
                ? strongestDeliveryStatus(existing.lastMessageStatus, c.lastMessageStatus)
                : c.lastMessageStatus,
            });
          } else {
            chatMap.set(c.jid, {
              ...existing,
              unreadCount: Math.max(existing.unreadCount, c.unreadCount),
              isArchived: c.isArchived,
            });
          }
        } else {
          chatMap.set(c.jid, c);
        }
      }
	      const merged = sortChats(Array.from(chatMap.values()));
      return { chats: merged };
    }),

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

  updateMessageReceipts: (chatJid, ids, status) =>
    set((state) => ({
      messages: {
        ...state.messages,
        [chatJid]: (state.messages[chatJid] || []).map((message) =>
          message.isFromMe && ids.includes(message.id)
            ? { ...message, deliveryStatus: status, isRead: status === 'read' }
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
          [chatJid]: deduped,
        },
      };
    }),

  mergeMessages: (chatJid, incoming) =>
    set((state) => {
      const byID = new Map<string, MessageItem>();
      for (const message of state.messages[chatJid] || []) byID.set(message.id, message);
      for (const message of incoming) byID.set(message.id, { ...byID.get(message.id), ...message });
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
          [chatJid]: deduped,
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
