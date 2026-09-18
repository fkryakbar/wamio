import { useCallback, useEffect, useRef, useState } from 'react';
import { LoginPage } from './pages/LoginPage';
import { AppRail } from './components/AppRail';
import { Sidebar } from './components/Sidebar';
import { ChatView } from './components/ChatView';
import { CallLogView } from './components/CallLogView';
import { useAuthStore } from './stores/authStore';
import { useChatStore } from './stores/chatStore';
import type { AccountInfo, ConnectionStatusEvent, MessageEvent, ChatUpdateEvent, ChatItem, ChatList, NotificationEvent, InitialSyncEvent, HistoryPageEvent, MessageReceiptEvent, QRCodeEvent, SyncProgressEvent, PresenceEvent, ChatPresenceEvent, MessageReactionEvent, MessageDeleteEvent } from './types';
import { EventsOn, WindowSetTitle } from '../wailsjs/runtime/runtime';

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
  const { connectionState, setConnectionState, setQRCode, setUserInfo, setAccountId, setAccounts, setError, clearQRCode, accounts } = useAuthStore();
  const { setChats, setChatLists, updateChat, addMessage, updateMessageReceipts, updateMessageReactions, removeMessage, initialSyncState, setInitialSyncState, prependMessagesPage, syncProgress, activeRailView } = useChatStore();
  const [appMode, setAppMode] = useState<'booting' | 'login' | 'linking' | 'switching' | 'workspace'>('booting');
  const [loginMode, setLoginMode] = useState<'initial' | 'add'>('initial');
  const appModeRef = useRef(appMode);
  const typingTimeoutsRef = useRef<Record<string, number>>({});
	const notificationRef = useRef<Notification | null>(null);
	const notificationTimerRef = useRef<number | null>(null);
  const unreadTotal = useChatStore((state) => state.chats.filter((chat) => chat.unreadCount > 0).length);

  useEffect(() => { appModeRef.current = appMode; }, [appMode]);

  useEffect(() => {
    const badge = unreadTotal > 99 ? '99+' : String(unreadTotal);
    const title = unreadTotal > 0 ? `(${badge}) Wamio` : 'Wamio';
    document.title = title;
    WindowSetTitle(title);
  }, [unreadTotal]);

  // Global connection event listener
  useEffect(() => {
    const cancelConn = EventsOn('wa:connection', (data: ConnectionStatusEvent) => {
      setConnectionState(data.state);
      if (data.state === 'connected') {
        setAppMode('workspace');
        import('../wailsjs/go/whatsapp/WhatsAppService').then(async (mod) => {
          const [info, knownAccounts] = await Promise.all([
            mod.GetUserInfo().catch(() => null),
            mod.GetAccounts().catch(() => [] as AccountInfo[]),
          ]);
          if (info) setUserInfo(info);
          setAccounts(knownAccounts || []);
          const active = knownAccounts?.find((account: AccountInfo) => account.isActive);
          if (active) setAccountId(active.id);
        }).catch(() => {});
      } else if (data.state === 'disconnected' || data.state === 'logged_out') {
        const mode = appModeRef.current;
        if (mode === 'booting' || mode === 'switching') {
          setLoginMode('initial');
          setAppMode('login');
        } else if (mode === 'linking') {
          setLoginMode('add');
          setAppMode('login');
        }
        if (data.message) setError(data.message);
      }
    });
    const cancelQR = EventsOn('wa:qr-code', (data: QRCodeEvent) => setQRCode(data));
    const cancelAccounts = EventsOn('wa:accounts-changed', (data: AccountInfo[]) => setAccounts(data || []));

    return () => { cancelConn(); cancelQR(); cancelAccounts(); };
  }, [setAccountId, setAccounts, setConnectionState, setError, setQRCode, setUserInfo]);

  // Restore is initiated before any login surface is eligible to render. This
  // avoids the Hubungkan page flashing while the last account reconnects.
  useEffect(() => {
    let active = true;
    import('../wailsjs/go/whatsapp/WhatsAppService').then(async (mod) => {
      try {
        const restored = await mod.RestoreLastSession();
        if (!active) return;
        if (!restored.attempted) {
          setLoginMode('initial');
          setAppMode('login');
          return;
        }
        if (restored.accountId) setAccountId(restored.accountId);
      } catch (error: any) {
        if (!active) return;
        setConnectionState('disconnected');
        setError(error?.message || 'Akun terakhir tidak dapat dipulihkan');
        setLoginMode('initial');
        setAppMode('login');
      }
    }).catch(() => {
      if (!active) return;
      setConnectionState('disconnected');
      setError('Aplikasi belum siap');
      setLoginMode('initial');
      setAppMode('login');
    });
    return () => { active = false; };
  }, [setAccountId, setConnectionState, setError]);

  const beginPairing = useCallback(async (label: string) => {
    setAppMode('linking');
    setError(null);
    clearQRCode();
    setUserInfo(null);
    useChatStore.getState().reset();
    try {
      const mod = await import('../wailsjs/go/whatsapp/WhatsAppService');
      await mod.BeginAddAccount(label);
    } catch (error) {
      setConnectionState('disconnected');
      setAppMode('login');
      throw error;
    }
  }, [clearQRCode, setConnectionState, setError, setUserInfo]);

  const addAccount = useCallback(() => {
    setLoginMode('add');
    setAppMode('login');
    setAccountId('');
    setError(null);
    clearQRCode();
    setUserInfo(null);
  }, [clearQRCode, setAccountId, setError, setUserInfo]);

  const switchAccount = useCallback(async (accountID: string) => {
    setAppMode('switching');
    setError(null);
    clearQRCode();
    setUserInfo(null);
    useChatStore.getState().reset();
    try {
      const mod = await import('../wailsjs/go/whatsapp/WhatsAppService');
      await mod.SwitchAccount(accountID);
    } catch (error: any) {
      setConnectionState('disconnected');
      setError(error?.message || 'Gagal membuka akun');
      setLoginMode('initial');
      setAppMode('login');
    }
  }, [clearQRCode, setConnectionState, setError, setUserInfo]);

  const cancelAddAccount = useCallback(async () => {
    // Before pairing starts, the Add account page is only a form; returning
    // from it must not ask the backend to cancel a session that does not exist.
    if (appModeRef.current !== 'linking') {
      setAppMode('workspace');
      setError(null);
      return;
    }
    setAppMode('switching');
    setError(null);
    clearQRCode();
    setUserInfo(null);
    try {
      const mod = await import('../wailsjs/go/whatsapp/WhatsAppService');
      await mod.CancelAddAccount();
    } catch (error: any) {
      setConnectionState('disconnected');
      setError(error?.message || 'Gagal kembali ke akun sebelumnya');
      setLoginMode('add');
      setAppMode('login');
    }
  }, [clearQRCode, setConnectionState, setError, setUserInfo]);

  const logout = useCallback(async () => {
    setAppMode('switching');
    setError(null);
    clearQRCode();
    setUserInfo(null);
    useChatStore.getState().reset();
    try {
      const mod = await import('../wailsjs/go/whatsapp/WhatsAppService');
      await mod.Logout();
    } catch (error: any) {
      setConnectionState('disconnected');
      setError(error?.message || 'Gagal logout');
      setLoginMode('initial');
      setAppMode('login');
    }
  }, [clearQRCode, setConnectionState, setError, setUserInfo]);

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

  if (appMode === 'booting') {
    return <FullScreenLoading text="Memuat akun terakhir..." progress={null} />;
  }

  if (appMode === 'switching') {
    return <FullScreenLoading text="Membuka akun..." progress={null} />;
  }

  if (appMode === 'login' || appMode === 'linking' || connectionState !== 'connected') {
    return <LoginPage mode={loginMode} onBeginPairing={beginPairing} onCancel={loginMode === 'add' ? () => { void cancelAddAccount(); } : undefined} />;
  }

  // Gate: block chat UI until initial sync completes
  if (initialSyncState === 'running') {
    return <FullScreenLoading text={syncProgress?.message || 'Menyinkronkan chat terbaru...'} progress={syncProgress} />;
  }

  // Connected + synced — show chat interface
  return (
    <div className="app-container">
      <AppRail accounts={accounts} onAddAccount={addAccount} onSwitchAccount={(accountID) => { void switchAccount(accountID); }} onLogout={() => { void logout(); }} />
      <Sidebar />
      {activeRailView === 'calls' ? <CallLogView /> : <ChatView />}
    </div>
  );
}

export default App;
