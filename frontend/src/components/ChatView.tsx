import { useEffect, useRef, useMemo, useState, useCallback } from 'react';
import { useChatStore } from '../stores/chatStore';
import { MessageInput } from './MessageInput';
import { MediaMessage } from './MediaMessage';
import { VoiceNote } from './VoiceNote';
import { Lightbox } from './Lightbox';
import type { MessageItem } from '../types';

function formatMessageTime(ts: number): string {
  if (!ts) return '';
  const d = new Date(ts * 1000);
  return d.toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit' });
}

function formatDateSeparator(ts: number): string {
  const d = new Date(ts * 1000);
  const now = new Date();
  const isToday =
    d.getDate() === now.getDate() &&
    d.getMonth() === now.getMonth() &&
    d.getFullYear() === now.getFullYear();

  if (isToday) return 'Hari ini';

  const yesterday = new Date(now);
  yesterday.setDate(yesterday.getDate() - 1);
  const isYesterday =
    d.getDate() === yesterday.getDate() &&
    d.getMonth() === yesterday.getMonth() &&
    d.getFullYear() === yesterday.getFullYear();

  if (isYesterday) return 'Kemarin';

  return d.toLocaleDateString('id-ID', { day: 'numeric', month: 'long', year: 'numeric' });
}

function isSameDay(ts1: number, ts2: number): boolean {
  const d1 = new Date(ts1 * 1000);
  const d2 = new Date(ts2 * 1000);
  return (
    d1.getDate() === d2.getDate() &&
    d1.getMonth() === d2.getMonth() &&
    d1.getFullYear() === d2.getFullYear()
  );
}

interface MessageBubbleProps {
  message: MessageItem;
  showSender: boolean;
  isGroup: boolean;
  onOpenLightbox: (src: string) => void;
}

function MessageBubble({ message, showSender, isGroup, onOpenLightbox }: MessageBubbleProps) {
  const hasMedia = !!message.mediaType && message.mediaType !== '';
  const isVoiceNote = message.mediaType === 'audio' && message.isPtt;
  const isAudio = message.mediaType === 'audio' && !message.isPtt;

  return (
    <div className={`message ${message.isFromMe ? 'message--sent' : 'message--received'}`}>
      <div className={`message__bubble ${message.isFromMe ? 'message__bubble--sent' : 'message__bubble--received'} ${hasMedia ? 'message__bubble--media' : ''}`}>
        {/* Sender name in groups */}
        {showSender && isGroup && !message.isFromMe && (
          <span className="message__sender">{message.senderName}</span>
        )}

        {/* Voice Note */}
        {(isVoiceNote || isAudio) && (
          <VoiceNote message={message} />
        )}

        {/* Media (image, video, document, sticker) */}
        {hasMedia && !isVoiceNote && !isAudio && (
          <MediaMessage message={message} onOpenLightbox={onOpenLightbox} />
        )}

        {/* Text content — only show for non-media or media with caption */}
        {!hasMedia && (
          <span className="message__text">{message.content}</span>
        )}

        {/* Timestamp */}
        <span className="message__meta">
          <span className="message__time">{formatMessageTime(message.timestamp)}</span>
          {message.isFromMe && (
            <svg className="message__check" width="16" height="11" viewBox="0 0 16 11">
              <path d="M11.071.653a.457.457 0 0 0-.304-.102.493.493 0 0 0-.381.178l-6.19 7.636-2.011-2.095a.464.464 0 0 0-.352-.153.468.468 0 0 0-.34.131.477.477 0 0 0-.014.679l2.333 2.433a.515.515 0 0 0 .349.166.516.516 0 0 0 .387-.158l6.528-8.005a.484.484 0 0 0-.005-.71z" fill="currentColor"/>
            </svg>
          )}
        </span>
      </div>
    </div>
  );
}

export function ChatView() {
  const {
    activeChatJID,
    activeChat,
    messages,
    pagination,
    setInitialMessages,
    prependMessagesPage,
    setLoadingMore,
  } = useChatStore();
  const chat = activeChat();
  const chatMessages = activeChatJID ? (messages[activeChatJID] || []) : [];
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [lightboxSrc, setLightboxSrc] = useState<string | null>(null);
  const prevMessageCountRef = useRef(0);
  const isAtBottomRef = useRef(true);

  // Auto-scroll to bottom when new messages arrive (only if user is at bottom)
  useEffect(() => {
    if (messagesEndRef.current && chatMessages.length > prevMessageCountRef.current) {
      if (isAtBottomRef.current) {
        messagesEndRef.current.scrollIntoView({ behavior: 'smooth' });
      }
    }
    prevMessageCountRef.current = chatMessages.length;
  }, [chatMessages.length]);

  // Scroll to bottom (instant) when switching chats
  useEffect(() => {
    if (!activeChatJID) return;
    // Wait a tick for the DOM to paint messages
    const timer = setTimeout(() => {
      if (messagesEndRef.current) {
        messagesEndRef.current.scrollIntoView({ behavior: 'instant' as ScrollBehavior });
      }
    }, 0);
    return () => clearTimeout(timer);
  }, [activeChatJID]);

  // Load initial 20 messages when active chat changes
  useEffect(() => {
    if (!activeChatJID) return;

    const page = pagination[activeChatJID];
    if (page) return; // already initialized

    import('../../wailsjs/go/whatsapp/WhatsAppService').then((mod) => {
      mod.GetMessagesPage(activeChatJID, 20, 0).then((msgs: MessageItem[]) => {
        if (msgs && msgs.length > 0) {
          setInitialMessages(activeChatJID, msgs);
        }
      });
    }).catch(() => {});
  }, [activeChatJID, pagination, setInitialMessages]);

  // Scroll-top handler for on-demand backfill
  const onScroll = useCallback(() => {
    const el = containerRef.current;
    if (!el || !activeChatJID) return;

    // Track whether user is at the bottom (for auto-scroll guard)
    const threshold = 80;
    isAtBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < threshold;

    if (el.scrollTop > 80) return;

    const page = pagination[activeChatJID];
    if (!page || page.loadingMore || !page.hasMore) return;

    setLoadingMore(activeChatJID, true);
    import('../../wailsjs/go/whatsapp/WhatsAppService').then((mod) => {
      mod.GetMessagesPage(activeChatJID, 50, page.oldestTimestampLoaded)
        .then((msgs: MessageItem[]) => {
          if (msgs && msgs.length > 0) {
            prependMessagesPage(activeChatJID, msgs);
          }
        })
        .finally(() => {
          setLoadingMore(activeChatJID, false);
        });
    }).catch(() => {
      setLoadingMore(activeChatJID, false);
    });
  }, [activeChatJID, pagination, setLoadingMore, prependMessagesPage]);

  // Attach scroll listener to container
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    el.addEventListener('scroll', onScroll, { passive: true });
    return () => el.removeEventListener('scroll', onScroll);
  }, [onScroll]);

  // Group messages with date separators
  const groupedMessages = useMemo(() => {
    const result: { type: 'date' | 'message'; date?: string; message?: MessageItem; showSender?: boolean }[] = [];
    let lastDate = 0;
    let lastSender = '';

    for (const msg of chatMessages) {
      if (!isSameDay(msg.timestamp, lastDate)) {
        result.push({ type: 'date', date: formatDateSeparator(msg.timestamp) });
        lastDate = msg.timestamp;
        lastSender = '';
      }

      const showSender = msg.senderJid !== lastSender;
      result.push({ type: 'message', message: msg, showSender });
      lastSender = msg.senderJid;
    }

    return result;
  }, [chatMessages]);

  const page = activeChatJID ? pagination[activeChatJID] : null;

  // No active chat — show welcome
  if (!activeChatJID || !chat) {
    return (
      <div className="chatview chatview--empty">
        <div className="chatview__welcome">
          <svg viewBox="0 0 303 172" width="260" xmlns="http://www.w3.org/2000/svg">
            <path fill="var(--wa-teal)" opacity="0.08" d="M229.565 160.229c32.647-12.996 50.467-43.156 50.467-76.348C280.032 37.543 240.583 0 191.484 0c-30.168 0-56.886 15.561-72.474 39.252C106.844 14.466 81.673 0 53.548 0 24.003 0 0 24.003 0 53.548c0 29.546 24.003 53.549 53.548 53.549 7.538 0 14.71-1.573 21.233-4.395C93.538 151.95 146.357 172 191.484 172c14.022 0 27.318-4.18 38.081-11.771z"/>
          </svg>
          <h2>Wamio</h2>
          <p>Pilih chat untuk mulai berkirim pesan</p>
        </div>
      </div>
    );
  }

  return (
    <div className="chatview">
      {/* Chat Header */}
      <div className="chatview__header">
        <div className="chatview__header-avatar" style={{ backgroundColor: '#00a884' }}>
          {chat.name?.[0]?.toUpperCase() || '?'}
        </div>
        <div className="chatview__header-info">
          <span className="chatview__header-name">{chat.name}</span>
          <span className="chatview__header-status">
            {chat.isGroup ? 'Grup' : 'Online'}
          </span>
        </div>
      </div>

      {/* Messages Area */}
      <div className="chatview__messages" ref={containerRef}>
        {/* Loading indicator for older messages */}
        {page?.loadingMore && (
          <div className="chatview__loading-more">
            <div className="chatview__loading-spinner" />
            <span>Memuat pesan lama...</span>
          </div>
        )}
        <div className="chatview__messages-inner">
          {groupedMessages.map((item, idx) =>
            item.type === 'date' ? (
              <div key={`date-${idx}`} className="chatview__date-separator">
                <span>{item.date}</span>
              </div>
            ) : item.message ? (
              <MessageBubble
                key={item.message.id || `msg-${idx}`}
                message={item.message}
                showSender={item.showSender ?? true}
                isGroup={chat.isGroup}
                onOpenLightbox={setLightboxSrc}
              />
            ) : null
          )}
          <div ref={messagesEndRef} />
        </div>
      </div>

      {/* Message Input */}
      <MessageInput chatJID={activeChatJID} />

      {/* Lightbox */}
      {lightboxSrc && (
        <Lightbox src={lightboxSrc} onClose={() => setLightboxSrc(null)} />
      )}
    </div>
  );
}
