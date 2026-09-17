import { useMemo, useState } from 'react';
import type { ChatItem, ForwardResult } from '../types';

interface ForwardDialogProps {
  sourceChatJID: string;
  messageIDs: string[];
  chats: ChatItem[];
  onClose: () => void;
}

export function ForwardDialog({ sourceChatJID, messageIDs, chats, onClose }: ForwardDialogProps) {
  const [query, setQuery] = useState('');
  const [selected, setSelected] = useState<string[]>([]);
  const [isSending, setIsSending] = useState(false);
  const [error, setError] = useState('');
  const filtered = useMemo(() => chats.filter((chat) => chat.name.toLowerCase().includes(query.toLowerCase())), [chats, query]);

  const toggle = (jid: string) => setSelected((current) => current.includes(jid) ? current.filter((id) => id !== jid) : [...current, jid]);
  const forward = async () => {
    if (!selected.length || isSending) return;
    setIsSending(true);
    setError('');
    try {
      const mod = await import('../../wailsjs/go/whatsapp/WhatsAppService');
      const results = await mod.ForwardMessages(sourceChatJID, messageIDs, selected) as ForwardResult[];
      const failures = results.filter((result) => !result.success);
      if (failures.length) {
        setError(failures.map((result) => result.error || result.chatJid).join(' '));
      } else {
        onClose();
      }
    } catch (err: any) {
      setError(err?.message || 'Pesan gagal diteruskan.');
    } finally {
      setIsSending(false);
    }
  };

  return (
    <div className="forward-dialog__backdrop" role="presentation" onMouseDown={onClose}>
      <section className="forward-dialog" role="dialog" aria-modal="true" aria-label="Teruskan pesan" onMouseDown={(event) => event.stopPropagation()}>
        <header><h2>Teruskan pesan ke</h2><button type="button" onClick={onClose} aria-label="Tutup">×</button></header>
        <input autoFocus value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Cari nama chat" />
        <div className="forward-dialog__list">
          {filtered.map((chat) => (
            <label key={chat.jid} className={selected.includes(chat.jid) ? 'forward-dialog__chat forward-dialog__chat--selected' : 'forward-dialog__chat'}>
              <input type="checkbox" checked={selected.includes(chat.jid)} onChange={() => toggle(chat.jid)} />
              <span className="forward-dialog__avatar">{chat.avatar ? <img src={chat.avatar.startsWith('http') ? chat.avatar : `data:image/jpeg;base64,${chat.avatar}`} alt="" /> : chat.name[0]?.toUpperCase()}</span>
              <span><strong>{chat.name}</strong><small>{chat.lastMessage}</small></span>
            </label>
          ))}
        </div>
        {error && <p className="forward-dialog__error">{error}</p>}
        <footer><span>{selected.length ? `${selected.length} chat dipilih` : 'Pilih chat tujuan'}</span><button type="button" onClick={forward} disabled={!selected.length || isSending}>{isSending ? 'Mengirim...' : 'Teruskan'}</button></footer>
      </section>
    </div>
  );
}
