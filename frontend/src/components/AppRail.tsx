import { useEffect, useRef, useState } from "react";
import { CircleUserRound, LogOut, MessageCircle, PhoneCall, Plus } from "lucide-react";
import { useAuthStore } from "../stores/authStore";
import { useChatStore } from "../stores/chatStore";
import type { AccountInfo } from "../types";

function unreadLabel(total: number) {
  return total > 99 ? "99+" : String(total);
}

interface AppRailProps {
  accounts: AccountInfo[];
  onAddAccount: () => void;
  onSwitchAccount: (accountId: string) => void;
  onLogout: () => void;
}

export function AppRail({ accounts, onAddAccount, onSwitchAccount, onLogout }: AppRailProps) {
  const chats = useChatStore((state) => state.chats);
  const activeRailView = useChatStore((state) => state.activeRailView);
  const setActiveRailView = useChatStore((state) => state.setActiveRailView);
  const { userInfo, accountId } = useAuthStore();
  const [accountMenuOpen, setAccountMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);
  // WhatsApp's navigation badge counts conversations, not every unread
  // message contained in those conversations.
  const unreadTotal = chats.filter((chat) => chat.unreadCount > 0).length;
  const accountName = userInfo?.pushName || accountId || "Wamio";

  useEffect(() => {
    const close = (event: MouseEvent) => {
      if (!menuRef.current?.contains(event.target as Node)) setAccountMenuOpen(false);
    };
    if (accountMenuOpen) window.addEventListener("mousedown", close);
    return () => window.removeEventListener("mousedown", close);
  }, [accountMenuOpen]);

  const selectAccount = (id: string) => {
    setAccountMenuOpen(false);
    onSwitchAccount(id);
  };

  return (
    <aside className="app-rail" aria-label="Navigasi utama">
      <div className="app-rail__top">
        <button type="button" className={`app-rail__nav ${activeRailView === "chats" ? "app-rail__nav--active" : ""}`} onClick={() => setActiveRailView("chats")} aria-label="Pesan" title="Pesan">
          <MessageCircle size={25} strokeWidth={2.1} />
          {unreadTotal > 0 && <span className="app-rail__badge">{unreadLabel(unreadTotal)}</span>}
        </button>
        <button type="button" className={`app-rail__nav ${activeRailView === "calls" ? "app-rail__nav--active" : ""}`} onClick={() => setActiveRailView("calls")} aria-label="Log panggilan" title="Log panggilan">
          <PhoneCall size={24} strokeWidth={2.1} />
        </button>
      </div>
      <div className="app-rail__bottom" ref={menuRef}>
        <button type="button" className="app-rail__nav" onClick={onLogout} aria-label="Logout" title="Logout"><LogOut size={23} strokeWidth={2.1} /></button>
        <button type="button" className="app-rail__profile" onClick={() => setAccountMenuOpen((open) => !open)} aria-expanded={accountMenuOpen} aria-label="Pilih akun" title="Pilih akun">
          {accountName.slice(0, 1).toUpperCase() || <CircleUserRound size={22} />}
        </button>
        {accountMenuOpen && (
            <div className="account-menu" role="menu" aria-label="Pilihan akun">
            <div className="account-menu__heading">Akun</div>
            {accounts.map((account) => {
              const label = account.label || account.userInfo.pushName || account.id;
              const sublabel = account.isActive
                ? "Akun aktif"
                : account.userInfo.phoneNumber || account.userInfo.pushName || "Akun tersimpan";
              return (
                <button key={account.id} type="button" className="account-menu__account" role="menuitem" onClick={() => selectAccount(account.id)}>
                  <span className="account-menu__avatar">{label.slice(0, 1).toUpperCase()}</span>
                  <span><strong>{label}</strong><small>{sublabel}</small></span>
                </button>
              );
            })}
            {accounts.length === 0 && (
              <button type="button" className="account-menu__account" role="menuitem" onClick={() => setAccountMenuOpen(false)}>
                <span className="account-menu__avatar">{accountName.slice(0, 1).toUpperCase()}</span>
                <span><strong>{accountName}</strong><small>Akun aktif</small></span>
              </button>
            )}
            <button type="button" className="account-menu__add" role="menuitem" title="Tambahkan akun lain" onClick={() => { setAccountMenuOpen(false); onAddAccount(); }}><Plus size={18} /> Tambah akun</button>
          </div>
        )}
      </div>
    </aside>
  );
}
