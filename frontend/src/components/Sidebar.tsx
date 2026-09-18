import { useMemo, useState, useEffect, useRef } from "react";
import { BellOff, Pin, Search, Users } from "lucide-react";
import type { ChatItem } from "../types";
import { useChatStore } from "../stores/chatStore";

function getInitials(name: string): string {
  return name
    .split(" ")
    .slice(0, 2)
    .map((w) => w[0]?.toUpperCase() || "")
    .join("");
}

function formatTime(ts: number): string {
  if (!ts) return "";
  const d = new Date(ts * 1000);
  const now = new Date();
  const isToday =
    d.getDate() === now.getDate() &&
    d.getMonth() === now.getMonth() &&
    d.getFullYear() === now.getFullYear();

  const yesterday = new Date(now);
  yesterday.setDate(yesterday.getDate() - 1);
  const isYesterday =
    d.getDate() === yesterday.getDate() &&
    d.getMonth() === yesterday.getMonth() &&
    d.getFullYear() === yesterday.getFullYear();

  if (isToday) {
    return d.toLocaleTimeString("id-ID", {
      hour: "2-digit",
      minute: "2-digit",
    });
  }
  if (isYesterday) {
    return "Kemarin";
  }
  return d.toLocaleDateString("id-ID", {
    day: "2-digit",
    month: "2-digit",
    year: "2-digit",
  });
}

// Color palette for avatar backgrounds
const avatarColors = [
  "#00a884",
  "#53bdeb",
  "#8696a0",
  "#f7c948",
  "#ea4335",
  "#7c5cbf",
  "#e06c75",
  "#4ecdc4",
  "#ff6b6b",
  "#48dbfb",
];

function getAvatarColor(name: string): string {
  let hash = 0;
  for (let i = 0; i < name.length; i++) {
    hash = name.charCodeAt(i) + ((hash << 5) - hash);
  }
  return avatarColors[Math.abs(hash) % avatarColors.length];
}

interface ChatListItemProps {
  chat: ChatItem;
  isActive: boolean;
  onClick: () => void;
}

function PreviewReceipt({ status }: { status: ChatItem["lastMessageStatus"] }) {
  const receipt = status || "sent";
	if (receipt === "pending") {
		return <span className="chat-item__pending" aria-label="Mengirim" title="Mengirim" />;
	}
	if (receipt === "failed") {
		return <span className="chat-item__failed" aria-label="Gagal dikirim" title="Gagal dikirim">!</span>;
	}
  return (
    <svg
      className={`chat-item__receipt chat-item__receipt--${receipt}`}
      width="16"
      height="11"
      viewBox="0 0 16 11"
      aria-label={receipt}
    >
      <path
        d="M11.071.653a.457.457 0 0 0-.304-.102.493.493 0 0 0-.381.178l-6.19 7.636-2.011-2.095a.464.464 0 0 0-.352-.153.468.468 0 0 0-.34.131.477.477 0 0 0-.014.679l2.333 2.433a.515.515 0 0 0 .349.166.516.516 0 0 0 .387-.158l6.528-8.005a.484.484 0 0 0-.005-.71z"
        fill="currentColor"
      />
      {receipt !== "sent" && (
        <path
          d="M15.071.653a.457.457 0 0 0-.304-.102.493.493 0 0 0-.381.178l-6.19 7.636-2.011-2.095a.464.464 0 0 0-.352-.153.468.468 0 0 0-.34.131.477.477 0 0 0-.014.679l2.333 2.433a.515.515 0 0 0 .349.166.516.516 0 0 0 .387-.158l6.528-8.005a.484.484 0 0 0-.005-.71z"
          fill="currentColor"
        />
      )}
    </svg>
  );
}

function ChatListItem({ chat, isActive, onClick }: ChatListItemProps) {
  const color = useMemo(() => getAvatarColor(chat.name), [chat.name]);
  const initials = useMemo(() => getInitials(chat.name), [chat.name]);
  const itemRef = useRef<HTMLDivElement | null>(null);
  const [nearViewport, setNearViewport] = useState(false);
  const { setChatAvatar } = useChatStore();
  const typing = useChatStore(
    (state) => state.presence[chat.jid]?.typing ?? false,
  );

  // Do not request an avatar for every chat as soon as a fresh sync renders
  // the sidebar. Fetch only rows that are about to be visible.
  useEffect(() => {
    const element = itemRef.current;
    if (!element) return;
    if (!("IntersectionObserver" in window)) {
      setNearViewport(true);
      return;
    }
    const observer = new IntersectionObserver(
      (entries) => {
        if (!entries.some((entry) => entry.isIntersecting)) return;
        setNearViewport(true);
        observer.disconnect();
      },
      { rootMargin: "200px 0px" },
    );
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    if (chat.avatar || !nearViewport) return;

    import("../../wailsjs/go/whatsapp/WhatsAppService")
      .then((mod) => {
        mod
          .GetProfilePicture(chat.jid)
          .then((url: string) => {
            if (url) {
              setChatAvatar(chat.jid, url);
            }
          })
          .catch(() => {});
      })
      .catch(() => {});
  }, [chat.jid, chat.avatar, nearViewport, setChatAvatar]);
  return (
    <div
      ref={itemRef}
      className={`chat-item ${isActive ? "chat-item--active" : ""}`}
      onClick={onClick}
      role="button"
      tabIndex={0}
    >
      {/* Avatar */}
      <div className="chat-item__avatar" style={{ backgroundColor: color }}>
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
          <span>{initials}</span>
        )}
        {chat.isGroup && (
          <div className="chat-item__group-icon"><Users size={11} strokeWidth={2.5} /></div>
        )}
      </div>

      {/* Content */}
      <div className="chat-item__content">
        <div className="chat-item__header">
          <div className="chat-item__title">
            <span className="chat-item__name">{chat.name}</span>
            {chat.isArchived && (
              <span className="chat-item__archive-label">Diarsipkan</span>
            )}
            {chat.isPinned && <Pin className="chat-item__status-icon" size={14} aria-label="Disematkan" />}
            {chat.isMuted && <BellOff className="chat-item__status-icon" size={14} aria-label="Dibisukan" />}
          </div>
          <span
            className={`chat-item__time ${chat.unreadCount > 0 ? "chat-item__time--unread" : ""}`}
          >
            {formatTime(chat.lastMessageTime)}
          </span>
        </div>
        <div className="chat-item__preview">
          {typing ? (
            <span className="chat-item__typing">sedang mengetik...</span>
          ) : (
            <>
              {chat.lastMessageFromMe && chat.lastMessage && (
                <PreviewReceipt status={chat.lastMessageStatus} />
              )}
              <span className="chat-item__message">
                {chat.lastMessage || "\u00A0"}
              </span>
            </>
          )}
          {chat.unreadCount > 0 && (
            <span className="chat-item__badge">
              {chat.unreadCount > 99 ? "99+" : chat.unreadCount}
            </span>
          )}
        </div>
      </div>
    </div>
  );
}

export function Sidebar() {
  const {
    activeChatJID,
		chatLists,
		activeChatListID,
		setActiveChatList,
    setActiveChat,
    filteredChats,
    searchQuery,
    setSearchQuery,
    isSyncing,
    syncProgressCount,
    initialSyncState,
    initialSyncError,
  } = useChatStore();
  const chats = filteredChats();

  const handleChatClick = (jid: string) => {
    setActiveChat(jid);
  };

  return (
    <div className="sidebar">
      {/* Header */}
      <div className="sidebar__header">
        <span className="sidebar__app-name">Wamio</span>
      </div>

      {/* Search */}
      <div className="sidebar__search">
        <div className="sidebar__search-input">
          <Search size={18} />
          <input
            type="text"
            placeholder="Cari atau mulai chat baru"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
          />
        </div>
      </div>

		<div className="sidebar__filters" role="tablist" aria-label="Filter chat">
			{chatLists.map((list) => (
				<button key={list.id} type="button" role="tab" aria-selected={activeChatListID === list.id}
					className={activeChatListID === list.id ? 'sidebar__filter sidebar__filter--active' : 'sidebar__filter'}
					onClick={() => setActiveChatList(list.id)}>{list.name}{list.id === 'unread' && chats.filter((chat) => chat.unreadCount > 0).length ? ` ${chats.filter((chat) => chat.unreadCount > 0).length}` : ''}</button>
			))}
		</div>

      {/* Sync indicator */}
      {isSyncing && (
        <div className="sidebar__sync-banner">
          <div className="spinner spinner--sm" />
          <span>Menyinkronkan chat ({syncProgressCount} chat)...</span>
        </div>
      )}
      {initialSyncState === "degraded" && initialSyncError && (
        <div className="sidebar__sync-banner sidebar__sync-banner--warning">
          <span>{initialSyncError}</span>
        </div>
      )}

      {/* Chat List */}
      <div className="sidebar__chats">
        {chats.length === 0 && !isSyncing ? (
          <div className="sidebar__empty">
            {searchQuery ? (
              <p>Tidak ada chat ditemukan</p>
            ) : (
              <p>Belum ada chat</p>
            )}
          </div>
        ) : chats.length === 0 && isSyncing ? (
          <div className="sidebar__empty">
            <div className="spinner" />
            <p>Memuat chat...</p>
          </div>
        ) : (
          chats.map((chat) => (
            <ChatListItem
              key={chat.jid}
              chat={chat}
              isActive={chat.jid === activeChatJID}
              onClick={() => handleChatClick(chat.jid)}
            />
          ))
        )}
      </div>
    </div>
  );
}
