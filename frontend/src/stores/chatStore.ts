import { create } from 'zustand';
import type { ChatItem, MessageItem } from '../types';

interface PaginationMeta {
  oldestTimestampLoaded: number;
  hasMore: boolean;
  loadingMore: boolean;
}

interface ChatState {
  // State
  chats: ChatItem[];
  activeChatJID: string | null;
  messages: Record<string, MessageItem[]>; // keyed by chat JID
  searchQuery: string;
  isSyncing: boolean;
  initialSyncState: 'running' | 'done';
  pagination: Record<string, PaginationMeta>;

  // Actions
  setChats: (chats: ChatItem[]) => void;
  updateChat: (chat: ChatItem) => void;
  setActiveChat: (jid: string | null) => void;
  addMessage: (chatJid: string, message: MessageItem) => void;
  setMessages: (chatJid: string, messages: MessageItem[]) => void;
  setInitialMessages: (chatJid: string, msgs: MessageItem[]) => void;
  prependMessagesPage: (chatJid: string, msgs: MessageItem[]) => void;
  setSearchQuery: (query: string) => void;
  markChatRead: (jid: string) => void;
  setSyncing: (syncing: boolean) => void;
  setInitialSyncState: (state: 'running' | 'done') => void;
  setLoadingMore: (chatJid: string, loading: boolean) => void;
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
  pagination: {},

  setChats: (incoming) =>
    set((state) => {
      const chatMap = new Map<string, ChatItem>();
      for (const c of state.chats) {
        chatMap.set(c.jid, c);
      }
      for (const c of incoming) {
        const existing = chatMap.get(c.jid);
        if (existing) {
          if (c.lastMessageTime >= existing.lastMessageTime) {
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

  setInitialMessages: (chatJid, msgs) =>
    set((state) => {
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
            hasMore: deduped.length >= 20,
            loadingMore: false,
          },
        },
      };
    }),

  prependMessagesPage: (chatJid, msgs) =>
    set((state) => {
      const existing = state.messages[chatJid] || [];
      const existingIds = new Set(existing.map((m) => m.id).filter(Boolean));
      const newMsgs = msgs.filter((m) => !existingIds.has(m.id));
      if (newMsgs.length === 0) return state;

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
            hasMore: newMsgs.length >= 50,
            loadingMore: false,
          },
        },
      };
    }),

  setSearchQuery: (query) => set({ searchQuery: query }),

  setSyncing: (syncing) => set({ isSyncing: syncing }),

  setInitialSyncState: (state) => set({ initialSyncState: state }),

  setLoadingMore: (chatJid, loading) =>
    set((state) => ({
      pagination: {
        ...state.pagination,
        [chatJid]: {
          ...state.pagination[chatJid],
          oldestTimestampLoaded: state.pagination[chatJid]?.oldestTimestampLoaded ?? 0,
          hasMore: state.pagination[chatJid]?.hasMore ?? false,
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
      pagination: {},
    }),

  markChatRead: (jid) =>
    set((state) => ({
      chats: state.chats.map((c) =>
        c.jid === jid ? { ...c, unreadCount: 0 } : c
      ),
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
