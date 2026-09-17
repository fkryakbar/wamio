import { useEffect, useRef } from 'react';
import { LoginPage } from './pages/LoginPage';
import { Sidebar } from './components/Sidebar';
import { ChatView } from './components/ChatView';
import { useAuthStore } from './stores/authStore';
import { useChatStore } from './stores/chatStore';
import type { ConnectionStatusEvent, MessageEvent, ChatUpdateEvent, ChatItem, ChatList, NotificationEvent, InitialSyncEvent, HistoryPageEvent, MessageReceiptEvent, SyncProgressEvent, PresenceEvent, ChatPresenceEvent, MessageReactionEvent, MessageDeleteEvent } from './types';
import { EventsOn } from '../wailsjs/runtime/runtime';

function FullScreenLoading({ text, progress }: { text: string; progress: SyncProgressEvent | null }) {
  return (
    <div className="app-loading">
      <div className="app-loading__spinner" />
      <p className="app-loading__text">{text}</p>
      {progress && (
        <div className="app-loading__progress" aria-live="polite">
          {progress.progress >= 0 && <div className="app-loading__progress-track"><div className="app-loading__progress-bar" style={{ width: `${Math.min(100, progress.progress)}%` }} /></div>}
          <span>{progress.preparedChats > 0 ? `${progress.preparedChats} chat terbaru siap dibuka` : `${progress.processedChats} chat diterima`}</span>
        </div>
      )}
    </div>
  );
}

function App() {
  const { connectionState, setConnectionState, userInfo } = useAuthStore();
  const { setChats, setChatLists, updateChat, addMessage, updateMessageReceipts, updateMessageReactions, removeMessage, initialSyncState, setInitialSyncState, prependMessagesPage, syncProgress } = useChatStore();
  const typingTimeoutsRef = useRef<Record<string, number>>({});
	const notificationRef = useRef<Notification | null>(null);
	const notificationTimerRef = useRef<number | null>(null);

  // Global connection event listener
  useEffect(() => {
    const cancelConn = EventsOn('wa:connection', (data: ConnectionStatusEvent) => {
      setConnectionState(data.state);
    });

    return () => { cancelConn(); };
  }, [setConnectionState]);

  // Initial sync lifecycle listener (always subscribed)
  useEffect(() => {
    const cancelInitSync = EventsOn('wa:initial-sync', (data: InitialSyncEvent) => {
      setInitialSyncState(data.state, data.message);
      if (data.state !== 'running') {
        import('../wailsjs/go/whatsapp/WhatsAppService').then((mod) => mod.GetChats()).then((chats: ChatItem[]) => {
          if (chats) setChats(chats);
        }).catch(() => {});
      }
    });
    return () => { cancelInitSync(); };
  }, [setInitialSyncState, setChats]);

  // Pairing progress is emitted by the backend from the actual history stream.
  useEffect(() => {
    const cancelProgress = EventsOn('wa:sync-progress', (data: SyncProgressEvent) => useChatStore.getState().setSyncProgress(data));
    const cancelPresence = EventsOn('wa:presence', (data: PresenceEvent) => useChatStore.getState().setPresence(data));
    const cancelTyping = EventsOn('wa:chat-presence', (data: ChatPresenceEvent) => {
      const previousTimeout = typingTimeoutsRef.current[data.chatJid];
      if (previousTimeout) {
        window.clearTimeout(previousTimeout);
        delete typingTimeoutsRef.current[data.chatJid];
      }
      useChatStore.getState().setChatPresence(data);
      if (data.typing) {
        typingTimeoutsRef.current[data.chatJid] = window.setTimeout(() => {
          useChatStore.getState().setChatPresence({ chatJid: data.chatJid, typing: false });
          delete typingTimeoutsRef.current[data.chatJid];
        }, 10000);
      }
    });
    return () => {
      cancelProgress();
      cancelPresence();
      cancelTyping();
      Object.values(typingTimeoutsRef.current).forEach((timeout) => window.clearTimeout(timeout));
      typingTimeoutsRef.current = {};
    };
  }, []);

  // Older history arrives asynchronously after a scroll-triggered request.
  useEffect(() => {
    const cancelHistoryPage = EventsOn('wa:history-page', (data: HistoryPageEvent) => {
      if (data?.chatJid) {
        prependMessagesPage(data.chatJid, {
          messages: data.messages || [],
          hasMoreLocal: false,
          canRequestOlder: data.canRequestOlder,
        });
      }
    });
    return () => { cancelHistoryPage(); };
  }, [prependMessagesPage]);

  // Chat & message event listeners (only when connected)
  useEffect(() => {
    if (connectionState !== 'connected') return;

    // Load initial chat list
    import('../wailsjs/go/whatsapp/WhatsAppService').then((mod) => {
      mod.GetChats().then((chats: ChatItem[]) => {
        if (chats && chats.length > 0) {
          setChats(chats);
        }
      });
		mod.GetChatLists?.().then((lists: ChatList[]) => setChatLists(lists || [])).catch(() => {});
    }).catch(() => {});

    // Listen for history sync (full chat list refresh)
    const cancelSync = EventsOn('wa:chats-sync', (chats: ChatItem[]) => {
      if (chats && chats.length > 0) {
        setChats(chats);
      }
    });
		const cancelLists = EventsOn('wa:chat-lists', (lists: ChatList[]) => setChatLists(lists || []));

    // Listen for individual chat updates
    const cancelChatUpdate = EventsOn('wa:chat-update', (data: ChatUpdateEvent) => {
      updateChat(data.chat);
    });

    // Listen for new messages
    const cancelMsg = EventsOn('wa:message', (data: MessageEvent) => {
      addMessage(data.chatJid, data.message);
      if (!data.message.isFromMe && useChatStore.getState().activeChatJID === data.chatJid) {
        import('../wailsjs/go/whatsapp/WhatsAppService')
          .then((mod) => mod.OpenChat(data.chatJid))
          .catch(() => {});
      }
    });

    const cancelReceipt = EventsOn('wa:message-receipt', (data: MessageReceiptEvent) => {
      updateMessageReceipts(data.chatJid, data.messageIds, data.deliveryStatus);
    });
    const cancelReaction = EventsOn('wa:message-reaction', (data: MessageReactionEvent) => {
      updateMessageReactions(data.chatJid, data.messageId, data.reactions || []);
    });
    const cancelDelete = EventsOn('wa:message-delete', (data: MessageDeleteEvent) => {
      removeMessage(data.chatJid, data.messageId, data.forMe);
    });

    // Listen for notifications (incoming messages from others)
    const cancelNotif = EventsOn('wa:notification', (data: NotificationEvent) => {
		const active = useChatStore.getState().activeChatJID;
		const chat = useChatStore.getState().chats.find((item) => item.jid === data.chatJid);
		if (active === data.chatJid || chat?.isArchived || chat?.isMuted) return;
      if ('Notification' in window && Notification.permission === 'granted') {
        const title = data.isGroup
          ? `${data.senderName} — ${data.chatName}`
          : data.chatName;
		if (notificationTimerRef.current) window.clearTimeout(notificationTimerRef.current);
		notificationRef.current?.close();
		const notification = new Notification(title, {
          body: data.content,
          icon: '/wails.png',
          silent: false,
        });
		notificationRef.current = notification;
		notificationTimerRef.current = window.setTimeout(() => { notification.close(); if (notificationRef.current === notification) notificationRef.current = null; }, 5000);
      }
    });

    // Request notification permission
    if ('Notification' in window && Notification.permission === 'default') {
      Notification.requestPermission();
    }

    return () => {
      cancelSync();
		cancelLists();
      cancelChatUpdate();
      cancelMsg();
      cancelReceipt();
      cancelReaction();
      cancelDelete();
      cancelNotif();
		if (notificationTimerRef.current) window.clearTimeout(notificationTimerRef.current);
		notificationRef.current?.close();
    };
  }, [connectionState, setChats, setChatLists, updateChat, addMessage, updateMessageReceipts, updateMessageReactions, removeMessage]);

  // Show login page if not connected
  if (connectionState !== 'connected') {
    return <LoginPage />;
  }

  // Gate: block chat UI until initial sync completes
  if (initialSyncState === 'running') {
    return <FullScreenLoading text={syncProgress?.message || 'Menyinkronkan chat terbaru...'} progress={syncProgress} />;
  }

  // Connected + synced — show chat interface
  return (
    <div className="app-container">
      <Sidebar />
      <ChatView />
    </div>
  );
}

export default App;
