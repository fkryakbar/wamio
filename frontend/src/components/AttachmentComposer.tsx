import { useEffect, useRef, useState } from 'react';
import { FileText, Image, Music2, Send, X } from 'lucide-react';
import type { AttachmentDraft } from '../types';

interface AttachmentComposerProps {
  drafts: AttachmentDraft[];
  caption: string;
  onCaptionChange: (caption: string) => void;
  onRemove: (id: string) => void;
  onAddMore: () => void;
  onCancel: () => void;
  onSend: () => void;
}

function DraftTile({ draft, onRemove }: { draft: AttachmentDraft; onRemove: () => void }) {
  const [preview, setPreview] = useState('');

  useEffect(() => {
    if (draft.kind !== 'image' && draft.kind !== 'video') return;
    import('../../wailsjs/go/whatsapp/WhatsAppService')
      .then((mod) => mod.GetDraftPreview(draft.id))
      .then((data) => data && setPreview(`data:${draft.mimetype};base64,${data}`))
      .catch(() => {});
  }, [draft.id, draft.kind, draft.mimetype]);

  return (
    <div className={`attachment-composer__tile attachment-composer__tile--${draft.kind}`}>
      {preview && draft.kind === 'image' && <img src={preview} alt={draft.fileName} />}
      {preview && draft.kind === 'video' && <video src={preview} muted />}
      {!preview && <span className="attachment-composer__file-icon">{draft.kind === 'document' ? <FileText /> : draft.kind === 'audio' ? <Music2 /> : <Image />}</span>}
      <span className="attachment-composer__name" title={draft.fileName}>{draft.fileName}</span>
      <button type="button" className="attachment-composer__remove" onClick={onRemove} aria-label={`Hapus ${draft.fileName}`}><X size={16} /></button>
    </div>
  );
}

export function AttachmentComposer({ drafts, caption, onCaptionChange, onRemove, onAddMore, onCancel, onSend }: AttachmentComposerProps) {
  const composerRef = useRef<HTMLDivElement>(null);
  const captionRef = useRef<HTMLInputElement>(null);
  useEffect(() => {
    captionRef.current?.focus();
  }, []);
  const sendOnEnter = (event: React.KeyboardEvent) => {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      onSend();
    }
  };
  return (
    <div ref={composerRef} className="attachment-composer" role="dialog" aria-label="Pratinjau lampiran" tabIndex={-1} onClick={(event) => { if (!(event.target as HTMLElement).closest('button,input')) composerRef.current?.focus(); }} onKeyDown={sendOnEnter}>
      <div className="attachment-composer__header">
        <strong>{drafts.length > 1 ? `${drafts.length} lampiran` : 'Pratinjau lampiran'}</strong>
        <button type="button" onClick={onCancel} aria-label="Tutup pratinjau"><X size={20} /></button>
      </div>
      <div className="attachment-composer__grid">
        {drafts.map((draft) => <DraftTile key={draft.id} draft={draft} onRemove={() => onRemove(draft.id)} />)}
        <button type="button" className="attachment-composer__add" onClick={onAddMore} aria-label="Tambah lampiran">+</button>
      </div>
      <div className="attachment-composer__footer">
        <input ref={captionRef} value={caption} onChange={(event) => onCaptionChange(event.target.value)} onKeyDown={sendOnEnter} placeholder="Tambahkan caption" />
        <button type="button" className="attachment-composer__send" onClick={onSend} title="Kirim lampiran"><Send size={19} /></button>
      </div>
    </div>
  );
}
