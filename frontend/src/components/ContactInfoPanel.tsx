import { useEffect, useState } from 'react';
import type { ChatItem, ChatProfile } from '../types';

interface ContactInfoPanelProps {
  chat: ChatItem;
  onClose: () => void;
}

export function ContactInfoPanel({ chat, onClose }: ContactInfoPanelProps) {
  const [profile, setProfile] = useState<ChatProfile | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    let active = true;
    setProfile(null);
    setError('');
    import('../../wailsjs/go/whatsapp/WhatsAppService')
      .then((mod) => mod.GetChatProfile(chat.jid))
      .then((value: ChatProfile) => { if (active) setProfile(value); })
      .catch(() => { if (active) setError('Info profil belum tersedia.'); });
    return () => { active = false; };
  }, [chat.jid]);

  const avatar = profile?.avatar || chat.avatar;
  const name = profile?.name || chat.name;
  return (
    <aside className="contact-info" aria-label="Info kontak">
      <header className="contact-info__header">
        <button type="button" onClick={onClose} aria-label="Tutup info kontak">×</button>
        <span>{chat.isGroup ? 'Info grup' : 'Info kontak'}</span>
      </header>
      <div className="contact-info__body">
        <div className="contact-info__avatar">
          {avatar ? <img src={avatar.startsWith('http') ? avatar : `data:image/jpeg;base64,${avatar}`} alt={name} /> : name[0]?.toUpperCase()}
        </div>
        <h2>{name}</h2>
        {profile?.phoneNumber && <p className="contact-info__muted">{profile.phoneNumber}</p>}
        {profile?.isGroup && <p className="contact-info__muted">{profile.participantCount || 0} anggota</p>}
        {!profile && !error && <p className="contact-info__muted">Memuat info...</p>}
        {error && <p className="contact-info__muted">{error}</p>}
        {profile?.description && (
          <section className="contact-info__section">
            <h3>Deskripsi</h3>
            <p>{profile.description}</p>
          </section>
        )}
        <section className="contact-info__section">
          <h3>{chat.isGroup ? 'ID grup' : 'WhatsApp'}</h3>
          <p className="contact-info__jid">{profile?.jid || chat.jid}</p>
        </section>
      </div>
    </aside>
  );
}
