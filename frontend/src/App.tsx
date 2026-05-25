import { useEffect } from 'react';
import { LoginPage } from './pages/LoginPage';
import { Sidebar } from './components/Sidebar';
import { ChatView } from './components/ChatView';
import { useAuthStore } from './stores/authStore';
import { useChatStore } from './stores/chatStore';
import type { ConnectionStatusEvent, MessageEvent, ChatUpdateEvent, ChatItem, NotificationEvent, InitialSyncEvent, ChatWithMessages } from './types';
import { EventsOn } from '../wailsjs/runtime/runtime';

function FullScreenLoading({ text }: { text: string }) {
  return (
    <div className="app-loading">
      <div className="app-loading__spinner" />
      <p className="app-loading__text">{text}</p>
    </div>
  );
}

function App() {
  const { connectionState, setConnectionState, userInfo } = useAuthStore();
  const { setChats, updateChat, addMessage, setSyncing, initialSyncState, setInitialSyncState, setInitialMessages } = useChatStore();

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
      setInitialSyncState(data.state);
    });
    return () => { cancelInitSync(); };
  }, [setInitialSyncState]);

  // Preloaded messages for recent chats (7 days)
  useEffect(() => {
    const cancelRecent = EventsOn('wa:recent-chat-messages', (data: { entries: ChatWithMessages[] }) => {
      if (data?.entries) {
        for (const entry of data.entries) {
          if (entry.messages && entry.messages.length > 0) {
            setInitialMessages(entry.chat.jid, entry.messages);
          }
        }
      }
    });
    return () => { cancelRecent(); };
  }, [setInitialMessages]);

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
      setSyncing(false);
    });

    // Listen for individual chat updates
    const cancelChatUpdate = EventsOn('wa:chat-update', (data: ChatUpdateEvent) => {
      updateChat(data.chat);
    });

    // Listen for new messages
    const cancelMsg = EventsOn('wa:message', (data: MessageEvent) => {
      addMessage(data.chatJid, data.message);
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
      cancelNotif();
    };
  }, [connectionState, setChats, updateChat, addMessage, setSyncing]);

  // Show login page if not connected
  if (connectionState !== 'connected') {
    return <LoginPage />;
  }

  // Gate: block chat UI until initial sync completes
  if (initialSyncState !== 'done') {
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
