import { useMemo, useState, useEffect } from "react";
import type { ChatItem } from "../types";
import { useChatStore } from "../stores/chatStore";
import { useAuthStore } from "../stores/authStore";

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
  const { setChatAvatar } = useChatStore();
  const typing = useChatStore(
    (state) => state.presence[chat.jid]?.typing ?? false,
  );

  // Asynchronously fetch profile pictures
  useEffect(() => {
    if (chat.avatar) return;

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
  }, [chat.jid, chat.avatar, setChatAvatar]);
  return (
    <div
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
          <div className="chat-item__group-icon">
            <svg width="10" height="10" viewBox="0 0 24 24" fill="white">
              <path d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z" />
            </svg>
          </div>
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
							{chat.isPinned && <span title="Disematkan" aria-label="Disematkan">📌</span>}
							{chat.isMuted && <span title="Dibisukan" aria-label="Dibisukan">🔕</span>}
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
  const { userInfo } = useAuthStore();
  const [isLoggingOut, setIsLoggingOut] = useState(false);
  const chats = filteredChats();

  const handleLogout = async () => {
    if (isLoggingOut) return;
    setIsLoggingOut(true);
    try {
      const mod = await import("../../wailsjs/go/whatsapp/WhatsAppService");
      await mod.Logout();
      useAuthStore.getState().reset();
      useChatStore.getState().reset();
    } catch (err) {
      console.error("Logout failed:", err);
    } finally {
      setIsLoggingOut(false);
    }
  };

  const handleChatClick = (jid: string) => {
    setActiveChat(jid);
  };

  return (
    <div className="sidebar">
      {/* Header */}
      <div className="sidebar__header">
        <div className="sidebar__user">
          <div
            className="sidebar__user-avatar"
            style={{ backgroundColor: "#00a884" }}
          >
            {userInfo?.pushName?.[0]?.toUpperCase() || "W"}
          </div>
          <span className="sidebar__user-name">
            {userInfo?.pushName || "Wamio"}
          </span>
        </div>
        <button
          type="button"
          onClick={handleLogout}
          disabled={isLoggingOut}
          className="sidebar__logout-btn"
        >
          {isLoggingOut ? "Logging out..." : "Logout"}
        </button>
      </div>

      {/* Search */}
      <div className="sidebar__search">
        <div className="sidebar__search-input">
          <svg
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="var(--text-tertiary)"
          >
            <path d="M15.5 14h-.79l-.28-.27C15.41 12.59 16 11.11 16 9.5 16 5.91 13.09 3 9.5 3S3 5.91 3 9.5 5.91 16 9.5 16c1.61 0 3.09-.59 4.23-1.57l.27.28v.79l5 4.99L20.49 19l-4.99-5zm-6 0C7.01 14 5 11.99 5 9.5S7.01 5 9.5 5 14 7.01 14 9.5 11.99 14 9.5 14z" />
          </svg>
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
