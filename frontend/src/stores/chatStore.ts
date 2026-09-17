import { create } from 'zustand';
import type { ChatItem, MessageItem, MessagePage } from '../types';

interface PaginationMeta {
  oldestTimestampLoaded: number;
  hasMoreLocal: boolean;
  canRequestOlder: boolean;
  loadingMore: boolean;
}

interface ChatState {
  // State
  chats: ChatItem[];
  activeChatJID: string | null;
  messages: Record<string, MessageItem[]>; // keyed by chat JID
  searchQuery: string;
  isSyncing: boolean;
  initialSyncState: 'running' | 'done' | 'failed';
  initialSyncError: string;
  pagination: Record<string, PaginationMeta>;
  syncProgressCount: number;

  // Actions
  setChats: (chats: ChatItem[]) => void;
  updateChat: (chat: ChatItem) => void;
  setActiveChat: (jid: string | null) => void;
  addMessage: (chatJid: string, message: MessageItem) => void;
  updateMessageReceipts: (chatJid: string, ids: string[], status: 'delivered' | 'read') => void;
  setMessages: (chatJid: string, messages: MessageItem[]) => void;
  setInitialMessages: (chatJid: string, page: MessagePage) => void;
  prependMessagesPage: (chatJid: string, page: MessagePage) => void;
  setSearchQuery: (query: string) => void;
  markChatRead: (jid: string) => void;
  setSyncing: (syncing: boolean) => void;
  setInitialSyncState: (state: 'running' | 'done' | 'failed', error?: string) => void;
  setLoadingMore: (chatJid: string, loading: boolean) => void;
  setChatAvatar: (jid: string, avatar: string) => void;
  addSyncProgress: (count: number) => void;
  reset: () => void;

  // Computed
  filteredChats: () => ChatItem[];
  activeChat: () => ChatItem | undefined;
}

export const useChatStore = create<ChatState>((set, get) => ({
  chats: [],
  activeChatJID: null,
  messages: {},
  searchQuery: '',
  isSyncing: true,
  initialSyncState: 'running',
  initialSyncError: '',
  pagination: {},
  syncProgressCount: 0,

  setChats: (incoming) =>
    set((state) => {
      const chatMap = new Map<string, ChatItem>();
      for (const c of state.chats) {
        chatMap.set(c.jid, c);
      }
      for (const c of incoming) {
        const existing = chatMap.get(c.jid);
        if (existing) {
          if (c.lastMessageTime > existing.lastMessageTime ||
              (c.lastMessageTime === existing.lastMessageTime && c.lastMessage === existing.lastMessage)) {
            chatMap.set(c.jid, c);
          } else {
            chatMap.set(c.jid, {
              ...existing,
              unreadCount: Math.max(existing.unreadCount, c.unreadCount),
            });
          }
        } else {
          chatMap.set(c.jid, c);
        }
      }
      const merged = Array.from(chatMap.values()).sort(
        (a, b) => b.lastMessageTime - a.lastMessageTime
      );
      return { chats: merged };
    }),

  updateChat: (chat) =>
    set((state) => {
      const idx = state.chats.findIndex((c) => c.jid === chat.jid);
      const newChats = [...state.chats];
      if (idx >= 0) {
        newChats[idx] = chat;
      } else {
        newChats.unshift(chat);
      }
      newChats.sort((a, b) => b.lastMessageTime - a.lastMessageTime);
      return { chats: newChats };
    }),

  setActiveChat: (jid) => set({ activeChatJID: jid }),

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
      activeChatJID: null,
      messages: {},
      searchQuery: '',
      isSyncing: true,
      initialSyncState: 'running',
      initialSyncError: '',
      pagination: {},
      syncProgressCount: 0,
    }),

  markChatRead: (jid) =>
    set((state) => ({
      chats: state.chats.map((c) =>
        c.jid === jid ? { ...c, unreadCount: 0 } : c
      ),
    })),

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

  filteredChats: () => {
    const { chats, searchQuery } = get();
    if (!searchQuery.trim()) return chats;
    const q = searchQuery.toLowerCase();
    return chats.filter(
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
