import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type MouseEvent as ReactMouseEvent,
} from "react";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { useChatStore } from "../stores/chatStore";
import { MessageInput } from "./MessageInput";
import { MediaMessage } from "./MediaMessage";
import { VoiceNote } from "./VoiceNote";
import { Lightbox } from "./Lightbox";
import { FormattedMessage } from "./FormattedMessage";
import { ForwardDialog } from "./ForwardDialog";
import { ContactInfoPanel } from "./ContactInfoPanel";
import type {
  HistoryPageEvent,
  MessageItem,
  MessagePage,
  MessageReference,
} from "../types";

const QUICK_REACTIONS = ["👍", "❤️", "😂", "😮", "😢", "🙏"];
const REPLY_HISTORY_BATCHES = 5;

function formatMessageTime(timestamp: number): string {
  return timestamp
    ? new Date(timestamp * 1000).toLocaleTimeString("id-ID", {
        hour: "2-digit",
        minute: "2-digit",
      })
    : "";
}

function formatDateSeparator(timestamp: number): string {
  const date = new Date(timestamp * 1000);
  const today = new Date();
  if (date.toDateString() === today.toDateString()) return "Hari ini";
  const yesterday = new Date(today);
  yesterday.setDate(today.getDate() - 1);
  if (date.toDateString() === yesterday.toDateString()) return "Kemarin";
  return date.toLocaleDateString("id-ID", {
    day: "numeric",
    month: "long",
    year: "numeric",
  });
}

function isSameDay(first: number, second: number): boolean {
  return (
    new Date(first * 1000).toDateString() ===
    new Date(second * 1000).toDateString()
  );
}

function toReference(message: MessageItem): MessageReference {
  return {
    id: message.id,
    chatJid: message.chatJid,
    senderJid: message.senderJid,
    senderName: message.senderName,
    content: message.isDeleted ? "Pesan ini telah dihapus" : message.content,
    mediaType: message.mediaType,
    isFromMe: message.isFromMe,
  };
}

function Receipt({ status }: { status: string }) {
  return (
    <svg
      className={`message__check message__check--${status}`}
      width="16"
      height="11"
      viewBox="0 0 16 11"
      aria-label={status}
    >
      <path
        d="M11.071.653a.457.457 0 0 0-.304-.102.493.493 0 0 0-.381.178l-6.19 7.636-2.011-2.095a.464.464 0 0 0-.352-.153.468.468 0 0 0-.34.131.477.477 0 0 0-.014.679l2.333 2.433a.515.515 0 0 0 .349.166.516.516 0 0 0 .387-.158l6.528-8.005a.484.484 0 0 0-.005-.71z"
        fill="currentColor"
      />
      {status !== "sent" && (
        <path
          d="M15.071.653a.457.457 0 0 0-.304-.102.493.493 0 0 0-.381.178l-6.19 7.636-2.011-2.095a.464.464 0 0 0-.352-.153.468.468 0 0 0-.34.131.477.477 0 0 0-.014.679l2.333 2.433a.515.515 0 0 0 .349.166.516.516 0 0 0 .387-.158l6.528-8.005a.484.484 0 0 0-.005-.71z"
          fill="currentColor"
        />
      )}
    </svg>
  );
}

interface MessageBubbleProps {
  message: MessageItem;
  showSender: boolean;
  isGroup: boolean;
  selecting: boolean;
  selected: boolean;
  highlighted: boolean;
  onToggleSelect: (messageID: string) => void;
  onOpenLightbox: (src: string) => void;
  onReply: (message: MessageItem) => void;
  onRevealReply: (reference: MessageReference) => void;
  onMenu: (event: ReactMouseEvent, message: MessageItem) => void;
  onReact: (message: MessageItem, emoji: string) => void;
  registerElement: (messageID: string, element: HTMLDivElement | null) => void;
}

function MessageBubble({
  message,
  showSender,
  isGroup,
  selecting,
  selected,
  highlighted,
  onToggleSelect,
  onOpenLightbox,
  onReply,
  onRevealReply,
  onMenu,
  onReact,
  registerElement,
}: MessageBubbleProps) {
  const hasMedia = !message.isDeleted && !!message.mediaType;
  const isVoice = message.mediaType === "audio";
  const quote = message.replyTo;
  return (
    <div
      ref={(element) => registerElement(message.id, element)}
      className={`message ${message.isFromMe ? "message--sent" : "message--received"} ${selecting ? "message--selecting" : ""} ${selected ? "message--selected" : ""} ${highlighted ? "message--highlighted" : ""}`}
      onClick={() => {
        if (selecting) onToggleSelect(message.id);
      }}
      onContextMenu={(event) => {
        if (!selecting) onMenu(event, message);
      }}
      onDoubleClick={() => {
        if (!selecting && !message.isDeleted) onReply(message);
      }}
    >
      {selecting && (
        <label
          className="message__select-control"
          aria-label="Pilih pesan"
          onClick={(event) => event.stopPropagation()}
        >
          <input
            type="checkbox"
            checked={selected}
            readOnly
            onClick={() => onToggleSelect(message.id)}
          />
          <span />
        </label>
      )}
      <div
        className={`message__bubble ${message.isFromMe ? "message__bubble--sent" : "message__bubble--received"} ${hasMedia ? "message__bubble--media" : ""} ${message.isDeleted ? "message__bubble--deleted" : ""}`}
      >
        {showSender && isGroup && !message.isFromMe && (
          <span className="message__sender">{message.senderName}</span>
        )}
        {message.isForwarded && (
          <span className="message__forwarded">Diteruskan</span>
        )}
        {quote && (
          <button
            type="button"
            className="message__reply"
            title="Buka pesan yang dibalas"
            onClick={(event) => {
              event.stopPropagation();
              onRevealReply(quote);
            }}
          >
            <strong>
              {quote.senderName || (quote.isFromMe ? "Anda" : "Pesan")}
            </strong>
            <span>
              {quote.content ||
                (quote.mediaType ? "Media" : "Pesan ini telah dihapus")}
            </span>
          </button>
        )}
        {message.isDeleted ? (
          <span className="message__deleted">Pesan ini telah dihapus</span>
        ) : (
          <>
							{message.kind === 'call' && <div className="message__call"><span>{message.call?.isVideo ? '📹' : '📞'}</span><span>{message.call?.isIncoming ? 'Panggilan masuk' : 'Panggilan keluar'}<small>{message.call?.outcome?.toLowerCase().replaceAll('_', ' ')}</small></span></div>}
            {isVoice && <VoiceNote message={message} />}
            {hasMedia && !isVoice && (
              <MediaMessage message={message} onOpenLightbox={onOpenLightbox} />
            )}
            {!hasMedia && message.kind !== 'call' && (
              <span className="message__text">
                <FormattedMessage content={message.content} />
              </span>
            )}
          </>
        )}
        <span className="message__meta">
          <span className="message__time">
            {formatMessageTime(message.timestamp)}
          </span>
          {message.isFromMe && !message.isDeleted && (
            <Receipt status={message.deliveryStatus || "sent"} />
          )}
        </span>
        {!!message.reactions?.length && (
          <div className="message__reactions" aria-label="Reaksi pesan">
            {message.reactions.map((reaction) => (
              <button
                key={reaction.emoji}
                type="button"
                className={
                  reaction.fromMe
                    ? "message__reaction message__reaction--mine"
                    : "message__reaction"
                }
                onClick={(event) => {
                  event.stopPropagation();
                  if (!selecting)
                    onReact(message, reaction.fromMe ? "" : reaction.emoji);
                }}
              >
                <span>{reaction.emoji}</span>
                {reaction.count > 1 && <small>{reaction.count}</small>}
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

function MessageMenu({
  message,
  position,
  onClose,
  onAction,
}: {
  message: MessageItem;
  position: { x: number; y: number };
  onClose: () => void;
  onAction: (action: string) => void;
}) {
  const style = {
    left: Math.max(8, Math.min(position.x, window.innerWidth - 250)),
    top: Math.max(8, Math.min(position.y, window.innerHeight - 310)),
  };
  return (
    <div
      className="message-menu-backdrop"
      onMouseDown={onClose}
      role="presentation"
    >
      <div
        className="message-menu"
        style={style}
        onMouseDown={(event) => event.stopPropagation()}
        role="menu"
      >
        {!message.isDeleted && (
          <div className="message-menu__reactions">
            {QUICK_REACTIONS.map((emoji) => (
              <button
                key={emoji}
                type="button"
                onClick={() => onAction(`react:${emoji}`)}
              >
                {emoji}
              </button>
            ))}
          </div>
        )}
        {!message.isDeleted && (
          <button type="button" onClick={() => onAction("reply")}>
            Reply
          </button>
        )}
        {!message.isDeleted && (
          <button type="button" onClick={() => onAction("copy")}>
            Copy
          </button>
        )}
        {!message.isDeleted && (
          <button type="button" onClick={() => onAction("forward")}>
            Forward
          </button>
        )}
        <button type="button" onClick={() => onAction("select")}>
          Select
        </button>
        <button
          type="button"
          className="message-menu__delete"
          onClick={() => onAction("delete")}
        >
          Delete
        </button>
      </div>
    </div>
  );
}

function DeleteDialog({
  message,
  onClose,
  onDelete,
}: {
  message: MessageItem;
  onClose: () => void;
  onDelete: (everyone: boolean) => void;
}) {
  return (
    <div
      className="confirm-dialog__backdrop"
      role="presentation"
      onMouseDown={onClose}
    >
      <section
        className="confirm-dialog"
        role="dialog"
        aria-modal="true"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <h2>Hapus pesan?</h2>
        <p>Pesan akan dihapus dari tampilan Anda.</p>
        <div className="confirm-dialog__actions">
          <button type="button" onClick={onClose}>
            Batal
          </button>
          <button type="button" onClick={() => onDelete(false)}>
            Hapus untuk saya
          </button>
          {message.isFromMe && (
            <button
              type="button"
              className="confirm-dialog__danger"
              onClick={() => onDelete(true)}
            >
              Hapus untuk semua
            </button>
          )}
        </div>
      </section>
    </div>
  );
}

export function ChatView() {
  const {
    activeChatJID,
    activeChat,
    messages,
    pagination,
    chats,
    setInitialMessages,
    prependMessagesPage,
    mergeMessages,
    setLoadingMore,
    presence,
  } = useChatStore();
  const chat = activeChat();
  const chatMessages = activeChatJID ? messages[activeChatJID] || [] : [];
  const activePage = activeChatJID ? pagination[activeChatJID] : undefined;
  const containerRef = useRef<HTMLDivElement>(null);
  const messageRefs = useRef(new Map<string, HTMLDivElement>());
  const renderedChatRef = useRef<string | null>(null);
  const previousCountRef = useRef(0);
  const initialBottomPendingRef = useRef(true);
  const isAtBottomRef = useRef(true);
  const prependAnchorRef = useRef<{ height: number; top: number } | null>(null);
  const pendingRevealRef = useRef<string | null>(null);
  const highlightTimerRef = useRef<number | null>(null);
  const resolvingReplyRef = useRef(false);
  const openedChatRef = useRef<string | null>(null);
  const [lightboxSrc, setLightboxSrc] = useState<string | null>(null);
  const [replyTo, setReplyTo] = useState<MessageReference | null>(null);
  const [menu, setMenu] = useState<{
    message: MessageItem;
    x: number;
    y: number;
  } | null>(null);
  const [selecting, setSelecting] = useState(false);
  const [selectedIDs, setSelectedIDs] = useState<string[]>([]);
  const [forwardIDs, setForwardIDs] = useState<string[] | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<MessageItem | null>(null);
  const [profileOpen, setProfileOpen] = useState(false);
  const [showScrollBottom, setShowScrollBottom] = useState(false);
  const [highlightedID, setHighlightedID] = useState<string | null>(null);
  const [notice, setNotice] = useState("");

  const registerElement = useCallback(
    (id: string, element: HTMLDivElement | null) => {
      if (element) messageRefs.current.set(id, element);
      else messageRefs.current.delete(id);
    },
    [],
  );

  const focusLoadedMessage = useCallback((messageID: string) => {
    const element = messageRefs.current.get(messageID);
    if (!element) {
      pendingRevealRef.current = messageID;
      return false;
    }
    pendingRevealRef.current = null;
    element.scrollIntoView({ block: "center", behavior: "smooth" });
    setHighlightedID(messageID);
    if (highlightTimerRef.current)
      window.clearTimeout(highlightTimerRef.current);
    highlightTimerRef.current = window.setTimeout(
      () => setHighlightedID(null),
      1800,
    );
    return true;
  }, []);

  useLayoutEffect(() => {
    const container = containerRef.current;
    if (!container || !activeChatJID) return;
    if (renderedChatRef.current !== activeChatJID) {
      renderedChatRef.current = activeChatJID;
      previousCountRef.current = 0;
      initialBottomPendingRef.current = true;
    }
    const anchor = prependAnchorRef.current;
    if (anchor) {
      container.scrollTop =
        anchor.top + (container.scrollHeight - anchor.height);
      prependAnchorRef.current = null;
    } else if (initialBottomPendingRef.current && chatMessages.length > 0) {
      container.scrollTop = container.scrollHeight;
      // Keep pinning while the first page is still replacing preloaded data.
      // Once pagination exists, the rendered page is final and normal scroll
      // behavior can resume.
      initialBottomPendingRef.current = !activePage;
      isAtBottomRef.current = true;
    } else if (
      chatMessages.length > previousCountRef.current &&
      isAtBottomRef.current
    ) {
      container.scrollTop = container.scrollHeight;
    }
    previousCountRef.current = chatMessages.length;
    setShowScrollBottom(
      container.scrollHeight - container.scrollTop - container.clientHeight >
        80,
    );
    if (pendingRevealRef.current) {
      const id = pendingRevealRef.current;
      window.requestAnimationFrame(() => focusLoadedMessage(id));
    }
  }, [activeChatJID, activePage, chatMessages.length, focusLoadedMessage]);

  useEffect(() => {
    setReplyTo(null);
    setMenu(null);
    setSelecting(false);
    setSelectedIDs([]);
    setProfileOpen(false);
    setNotice("");
    return () => {
      if (highlightTimerRef.current)
        window.clearTimeout(highlightTimerRef.current);
    };
  }, [activeChatJID]);

  useEffect(() => {
    if (!activeChatJID || pagination[activeChatJID]) return;
    import("../../wailsjs/go/whatsapp/WhatsAppService")
      .then((mod) =>
        mod
          .GetMessagesPage(activeChatJID, 20, 0)
          .then((page: MessagePage) => setInitialMessages(activeChatJID, page)),
      )
      .catch(() => {});
  }, [activeChatJID, pagination, setInitialMessages]);

  useEffect(() => {
    if (
      !activeChatJID ||
      !pagination[activeChatJID] ||
      openedChatRef.current === activeChatJID
    )
      return;
    openedChatRef.current = activeChatJID;
    import("../../wailsjs/go/whatsapp/WhatsAppService")
      .then((mod) => mod.OpenChat(activeChatJID))
      .catch(() => {});
  }, [activeChatJID, pagination]);

  const onScroll = useCallback(() => {
    const container = containerRef.current;
    if (!container || !activeChatJID) return;
    const distance =
      container.scrollHeight - container.scrollTop - container.clientHeight;
    isAtBottomRef.current = distance < 80;
    setShowScrollBottom(distance > 80);
    if (container.scrollTop > 80) return;
    const page = pagination[activeChatJID];
    const oldest = chatMessages[0];
    if (!page || page.loadingMore || !oldest) return;
    prependAnchorRef.current = {
      height: container.scrollHeight,
      top: container.scrollTop,
    };
    setLoadingMore(activeChatJID, true);
    import("../../wailsjs/go/whatsapp/WhatsAppService")
      .then((mod) => {
        if (page.hasMoreLocal)
          return mod
            .GetMessagesPage(activeChatJID, 50, page.oldestTimestampLoaded)
            .then((result: MessagePage) => {
              prependMessagesPage(activeChatJID, result);
              if (!result.messages.length && result.canRequestOlder)
                return mod.RequestOlderMessages(
                  activeChatJID,
                  oldest.id,
                  oldest.isFromMe,
                  oldest.timestamp,
                  50,
                );
            });
        if (page.canRequestOlder)
          return mod.RequestOlderMessages(
            activeChatJID,
            oldest.id,
            oldest.isFromMe,
            oldest.timestamp,
            50,
          );
      })
      .catch(() => {})
      .finally(() => setLoadingMore(activeChatJID, false));
  }, [
    activeChatJID,
    chatMessages,
    pagination,
    prependMessagesPage,
    setLoadingMore,
  ]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    container.addEventListener("scroll", onScroll, { passive: true });
    return () => container.removeEventListener("scroll", onScroll);
  }, [onScroll]);

  useEffect(() => {
    const onEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setMenu(null);
        setSelecting(false);
        setSelectedIDs([]);
        setReplyTo(null);
      }
    };
    window.addEventListener("keydown", onEscape);
    return () => window.removeEventListener("keydown", onEscape);
  }, []);

  const groupedMessages = useMemo(() => {
    const grouped: {
      type: "date" | "message";
      date?: string;
      message?: MessageItem;
      showSender?: boolean;
    }[] = [];
    let lastDate = 0;
    let lastSender = "";
    for (const message of chatMessages) {
      if (!lastDate || !isSameDay(message.timestamp, lastDate)) {
        grouped.push({
          type: "date",
          date: formatDateSeparator(message.timestamp),
        });
        lastSender = "";
      }
      grouped.push({
        type: "message",
        message,
        showSender: message.senderJid !== lastSender,
      });
      lastDate = message.timestamp;
      lastSender = message.senderJid;
    }
    return grouped;
  }, [chatMessages]);

  const toggleSelected = (id: string) =>
    setSelectedIDs((current) =>
      current.includes(id)
        ? current.filter((item) => item !== id)
        : [...current, id],
    );
  const scrollToBottom = () =>
    containerRef.current?.scrollTo({
      top: containerRef.current.scrollHeight,
      behavior: "smooth",
    });
  const react = async (message: MessageItem, emoji: string) => {
    try {
      const mod = await import("../../wailsjs/go/whatsapp/WhatsAppService");
      await mod.ReactToMessage(message.chatJid, message.id, emoji);
    } catch {
      setNotice("Reaksi gagal dikirim.");
    }
  };
  const copy = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      setNotice("Pesan tidak dapat disalin.");
    }
  };

  const revealReply = async (reference: MessageReference) => {
    if (
      !activeChatJID ||
      reference.chatJid !== activeChatJID ||
      resolvingReplyRef.current
    )
      return;
    if (chatMessages.some((message) => message.id === reference.id)) {
      focusLoadedMessage(reference.id);
      return;
    }
    resolvingReplyRef.current = true;
    setNotice("Mencari pesan asli...");
    try {
      const mod = await import("../../wailsjs/go/whatsapp/WhatsAppService");
      const local = (await mod.GetMessagesAround(
        activeChatJID,
        reference.id,
        25,
      )) as MessagePage;
      if (local.messages?.some((message) => message.id === reference.id)) {
        pendingRevealRef.current = reference.id;
        mergeMessages(activeChatJID, local.messages);
        setNotice("");
        return;
      }
      for (let batch = 0; batch < REPLY_HISTORY_BATCHES; batch++) {
        const state = useChatStore.getState();
        const page = state.pagination[activeChatJID];
        const oldest = state.messages[activeChatJID]?.[0];
        if (!page?.canRequestOlder || !oldest) break;
        const history = await new Promise<HistoryPageEvent>((resolve) => {
          const cancel = EventsOn(
            "wa:history-page",
            (event: HistoryPageEvent) => {
              if (event.chatJid !== activeChatJID) return;
              window.clearTimeout(timer);
              cancel();
              resolve(event);
            },
          );
          const timer = window.setTimeout(() => {
            cancel();
            resolve({
              chatJid: activeChatJID,
              messages: [],
              canRequestOlder: false,
              error: "Waktu permintaan habis.",
            });
          }, 31000);
          mod
            .RequestOlderMessages(
              activeChatJID,
              oldest.id,
              oldest.isFromMe,
              oldest.timestamp,
              50,
            )
            .catch(() => {
              window.clearTimeout(timer);
              cancel();
              resolve({
                chatJid: activeChatJID,
                messages: [],
                canRequestOlder: false,
                error: "Riwayat lama tidak tersedia.",
              });
            });
        });
        if (history.messages?.some((message) => message.id === reference.id)) {
          pendingRevealRef.current = reference.id;
          mergeMessages(activeChatJID, history.messages);
          setNotice("");
          return;
        }
        if (history.error || !history.canRequestOlder) break;
      }
      setNotice("Pesan asli belum tersedia.");
    } catch {
      setNotice("Pesan asli belum tersedia.");
    } finally {
      resolvingReplyRef.current = false;
    }
  };

  const openMenu = (event: ReactMouseEvent, message: MessageItem) => {
    event.preventDefault();
    setMenu({ message, x: event.clientX, y: event.clientY });
  };
  const handleMenuAction = (action: string) => {
    if (!menu) return;
    const message = menu.message;
    setMenu(null);
    if (action.startsWith("react:")) void react(message, action.slice(6));
    if (action === "reply" && !message.isDeleted)
      setReplyTo(toReference(message));
    if (action === "copy" && !message.isDeleted) void copy(message.content);
    if (action === "forward" && !message.isDeleted) setForwardIDs([message.id]);
    if (action === "select") {
      setSelecting(true);
      setSelectedIDs([message.id]);
    }
    if (action === "delete") setDeleteTarget(message);
  };
  const deleteMessage = async (everyone: boolean) => {
    if (!deleteTarget) return;
    try {
      const mod = await import("../../wailsjs/go/whatsapp/WhatsAppService");
      if (everyone)
        await mod.DeleteMessageForEveryone(
          deleteTarget.chatJid,
          deleteTarget.id,
        );
      else await mod.DeleteMessageForMe(deleteTarget.chatJid, deleteTarget.id);
    } catch {
      setNotice("Pesan gagal dihapus.");
    }
    setDeleteTarget(null);
  };

  const page = activePage;
  const chatPresence = activeChatJID ? presence[activeChatJID] : undefined;
  const headerStatus = chat?.isGroup
    ? "Grup"
    : chatPresence?.typing
      ? "sedang mengetik..."
      : chatPresence?.online
        ? "Online"
        : chatPresence?.lastSeen
          ? `terakhir dilihat ${formatMessageTime(chatPresence.lastSeen)}`
          : chatPresence?.unavailable
            ? "Status tidak tersedia"
            : "Memeriksa status...";
  if (!activeChatJID || !chat)
    return (
      <div className="chatview chatview--empty">
        <div className="chatview__welcome">
          <h2>Wamio</h2>
          <p>Pilih chat untuk mulai berkirim pesan</p>
        </div>
      </div>
    );

  return (
    <div className="chatview">
      <div className="chatview__header">
        <button
          type="button"
          className="chatview__header-profile"
          onClick={() => setProfileOpen(true)}
          aria-label="Buka info kontak"
        >
          <span className="chatview__header-avatar">
            {chat.avatar ? (
              <img
                src={
                  chat.avatar.startsWith("http")
                    ? chat.avatar
                    : `data:image/jpeg;base64,${chat.avatar}`
                }
                alt={chat.name}
              />
            ) : (
              chat.name?.[0]?.toUpperCase() || "?"
            )}
          </span>
          <span className="chatview__header-info">
            <span className="chatview__header-name">{chat.name}</span>
            <span className="chatview__header-status">{headerStatus}</span>
          </span>
        </button>
      </div>
      <div className="chatview__messages-wrap">
      <div className="chatview__messages" ref={containerRef}>
        {page?.loadingMore && (
          <div className="chatview__loading-more">
            <div className="chatview__loading-spinner" />
            <span>Memuat pesan lama...</span>
          </div>
        )}
        <div className="chatview__messages-inner">
          {groupedMessages.map((item, index) =>
            item.type === "date" ? (
              <div key={`date-${index}`} className="chatview__date-separator">
                <span>{item.date}</span>
              </div>
            ) : item.message ? (
              <MessageBubble
                key={item.message.id || `msg-${index}`}
                message={item.message}
                showSender={item.showSender ?? true}
                isGroup={chat.isGroup}
                selecting={selecting}
                selected={selectedIDs.includes(item.message.id)}
                highlighted={highlightedID === item.message.id}
                onToggleSelect={toggleSelected}
                onOpenLightbox={setLightboxSrc}
                onReply={(message) => setReplyTo(toReference(message))}
                onRevealReply={(reference) => void revealReply(reference)}
                onMenu={openMenu}
                onReact={(message, emoji) => void react(message, emoji)}
                registerElement={registerElement}
              />
            ) : null,
          )}
        </div>
      </div>
        {showScrollBottom && (
          <button
            type="button"
            className="chatview__scroll-bottom"
            onClick={scrollToBottom}
            aria-label="Scroll ke pesan terbaru"
          >
            ↓
          </button>
        )}
      </div>
      {selecting && (
        <div className="selection-bar">
          <button
            type="button"
            onClick={() => {
              setSelecting(false);
              setSelectedIDs([]);
            }}
          >
            Batal
          </button>
          <strong>{selectedIDs.length} dipilih</strong>
          <button
            type="button"
            disabled={!selectedIDs.length}
            onClick={() => setForwardIDs(selectedIDs)}
          >
            Forward
          </button>
        </div>
      )}
      <MessageInput
        chatJID={activeChatJID}
        replyTo={replyTo}
        onCancelReply={() => setReplyTo(null)}
      />
      {notice && (
        <div className="chatview__notice" role="status">
          {notice}
        </div>
      )}
      {menu && (
        <MessageMenu
          message={menu.message}
          position={menu}
          onClose={() => setMenu(null)}
          onAction={handleMenuAction}
        />
      )}
      {deleteTarget && (
        <DeleteDialog
          message={deleteTarget}
          onClose={() => setDeleteTarget(null)}
          onDelete={(everyone) => void deleteMessage(everyone)}
        />
      )}
      {forwardIDs && (
        <ForwardDialog
          sourceChatJID={activeChatJID}
          messageIDs={forwardIDs}
          chats={chats}
          onClose={() => setForwardIDs(null)}
        />
      )}
      {profileOpen && (
        <ContactInfoPanel chat={chat} onClose={() => setProfileOpen(false)} />
      )}
      {lightboxSrc && (
        <Lightbox src={lightboxSrc} onClose={() => setLightboxSrc(null)} />
      )}
    </div>
  );
}
