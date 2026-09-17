import { useEffect, type ReactNode } from 'react';
import { LoginPage } from './pages/LoginPage';
import { Sidebar } from './components/Sidebar';
import { ChatView } from './components/ChatView';
import { useAuthStore } from './stores/authStore';
import { useChatStore } from './stores/chatStore';
import type { ConnectionStatusEvent, MessageEvent, ChatUpdateEvent, ChatItem, NotificationEvent, InitialSyncEvent, HistoryPageEvent, MessageReceiptEvent } from './types';
import { EventsOn } from '../wailsjs/runtime/runtime';

function FullScreenLoading({ text, action }: { text: string; action?: ReactNode }) {
  return (
    <div className="app-loading">
      <div className="app-loading__spinner" />
      <p className="app-loading__text">{text}</p>
      {action}
    </div>
  );
}

function App() {
  const { connectionState, setConnectionState, userInfo } = useAuthStore();
  const { setChats, updateChat, addMessage, updateMessageReceipts, setSyncing, initialSyncState, initialSyncError, setInitialSyncState, prependMessagesPage } = useChatStore();

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
    });
    return () => { cancelInitSync(); };
  }, [setInitialSyncState]);

  // History sync progress listener
  useEffect(() => {
    let timer: any;
    const cancelProgress = EventsOn('wa:history-sync-progress', (data: { count: number }) => {
      useChatStore.getState().addSyncProgress(data.count);
      useChatStore.getState().setSyncing(true); // force syncing banner to show

      clearTimeout(timer);
      timer = setTimeout(() => {
        useChatStore.getState().setSyncing(false); // hide sync banner after 5 seconds of inactivity
      }, 5000);
    });
    return () => {
      cancelProgress();
      clearTimeout(timer);
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
    }).catch(() => {});

    // Listen for history sync (full chat list refresh)
    const cancelSync = EventsOn('wa:chats-sync', (chats: ChatItem[]) => {
      if (chats && chats.length > 0) {
        setChats(chats);
      }
    });

    // Listen for individual chat updates
    const cancelChatUpdate = EventsOn('wa:chat-update', (data: ChatUpdateEvent) => {
      updateChat(data.chat);
    });

    // Listen for new messages
    const cancelMsg = EventsOn('wa:message', (data: MessageEvent) => {
      addMessage(data.chatJid, data.message);
    });

    const cancelReceipt = EventsOn('wa:message-receipt', (data: MessageReceiptEvent) => {
      updateMessageReceipts(data.chatJid, data.messageIds, data.deliveryStatus);
    });

    // Listen for notifications (incoming messages from others)
    const cancelNotif = EventsOn('wa:notification', (data: NotificationEvent) => {
      if ('Notification' in window && Notification.permission === 'granted') {
        const title = data.isGroup
          ? `${data.senderName} — ${data.chatName}`
          : data.chatName;
        new Notification(title, {
          body: data.content,
          icon: '/wails.png',
          silent: false,
        });
      }
    });

    // Request notification permission
    if ('Notification' in window && Notification.permission === 'default') {
      Notification.requestPermission();
    }

    return () => {
      cancelSync();
      cancelChatUpdate();
      cancelMsg();
      cancelReceipt();
      cancelNotif();
    };
  }, [connectionState, setChats, updateChat, addMessage, updateMessageReceipts, setSyncing]);

  // Show login page if not connected
  if (connectionState !== 'connected') {
    return <LoginPage />;
  }

  // Gate: block chat UI until initial sync completes
  if (initialSyncState !== 'done') {
    if (initialSyncState === 'failed') {
      return <FullScreenLoading
        text={initialSyncError || 'Sinkronisasi gagal. Tautkan ulang perangkat ini.'}
        action={<button className="login-button" onClick={() => {
          import('../wailsjs/go/whatsapp/WhatsAppService').then((mod) => mod.Logout());
        }}>Tautkan ulang</button>}
      />;
    }
    return <FullScreenLoading text="Menyinkronkan pesan..." />;
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
