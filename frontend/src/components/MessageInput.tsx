import { useState, useCallback, useRef, useEffect } from 'react';
import { Camera, FileText, Image, Mic, Paperclip, Send, Smile, X } from 'lucide-react';
import type { AttachmentDraft, MessageItem, MessageReference, StickerItem } from '../types';
import { useChatStore } from '../stores/chatStore';
import { AttachmentComposer } from './AttachmentComposer';
import { StickerPicker } from './StickerPicker';
import { VoiceRecorder } from './VoiceRecorder';

interface MessageInputProps {
  chatJID: string;
  replyTo?: MessageReference | null;
  onCancelReply?: () => void;
}

export function MessageInput({ chatJID, replyTo, onCancelReply }: MessageInputProps) {
  const [text, setText] = useState('');
  const [attachOpen, setAttachOpen] = useState(false);
	const [stickerOpen, setStickerOpen] = useState(false);
  const [drafts, setDrafts] = useState<AttachmentDraft[]>([]);
  const [caption, setCaption] = useState('');
  const [cameraOpen, setCameraOpen] = useState(false);
  const [cameraError, setCameraError] = useState('');
  const [voiceActive, setVoiceActive] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const cameraVideoRef = useRef<HTMLVideoElement>(null);
  const cameraStreamRef = useRef<MediaStream | null>(null);
  const { addOptimisticMessage, markOutgoingFailed } = useChatStore();

  // Auto-resize textarea
  useEffect(() => {
    const el = textareaRef.current;
    if (el) {
      el.style.height = 'auto';
      el.style.height = Math.min(el.scrollHeight, 150) + 'px';
    }
  }, [text]);

  // Focus on chat change
  useEffect(() => {
    setVoiceActive(false);
    textareaRef.current?.focus();
  }, [chatJID]);

	// Reply is selected by double click or the message menu. Move focus after
	// that state is committed so typing can begin immediately.
	useEffect(() => {
		if (replyTo) requestAnimationFrame(() => textareaRef.current?.focus());
	}, [replyTo?.id]);

  useEffect(() => {
    const receiveDrafts = (event: Event) => {
      const incoming = (event as CustomEvent<AttachmentDraft[]>).detail || [];
      if (incoming.length) setDrafts((current) => [...current, ...incoming]);
    };
    window.addEventListener('wamio:attachments', receiveDrafts);
    return () => window.removeEventListener('wamio:attachments', receiveDrafts);
  }, []);

  useEffect(() => {
    if (!cameraOpen) return;
    let cancelled = false;
    setCameraError('');
    navigator.mediaDevices.getUserMedia({ video: true })
      .then((stream) => {
        if (cancelled) { stream.getTracks().forEach((track) => track.stop()); return; }
        cameraStreamRef.current = stream;
        if (cameraVideoRef.current) cameraVideoRef.current.srcObject = stream;
      })
      .catch(() => setCameraError('Kamera tidak tersedia atau izin ditolak.'));
    return () => {
      cancelled = true;
      cameraStreamRef.current?.getTracks().forEach((track) => track.stop());
      cameraStreamRef.current = null;
    };
  }, [cameraOpen]);

  const appendDrafts = useCallback((incoming: AttachmentDraft[]) => {
    if (incoming.length) setDrafts((current) => [...current, ...incoming]);
    setAttachOpen(false);
  }, []);

  const pick = useCallback(async (kind: 'document' | 'media' | 'audio') => {
    try {
      const app = await import('../../wailsjs/go/main/App');
      appendDrafts(await app.PickAttachments(kind) as unknown as AttachmentDraft[]);
    } catch (error) {
      console.error('Failed to select attachments:', error);
    }
  }, [appendDrafts]);

  const capturePhoto = useCallback(async () => {
    const video = cameraVideoRef.current;
    if (!video || !video.videoWidth) return;
    const canvas = document.createElement('canvas');
    canvas.width = video.videoWidth;
    canvas.height = video.videoHeight;
    canvas.getContext('2d')?.drawImage(video, 0, 0);
    try {
      const app = await import('../../wailsjs/go/main/App');
      const draft = await app.StageCapturedMedia(canvas.toDataURL('image/jpeg', 0.9), `photo-${Date.now()}.jpg`, 'media', false) as unknown as AttachmentDraft;
      appendDrafts([draft]);
      setCameraOpen(false);
    } catch (error) {
      setCameraError('Foto tidak dapat disiapkan.');
    }
  }, [appendDrafts]);

  const closeVoice = useCallback(() => setVoiceActive(false), []);
  const showVoiceError = useCallback((message: string) => setCameraError(message), []);
  const startRecording = useCallback(() => {
    setAttachOpen(false);
    setStickerOpen(false);
    setCameraError('');
    setVoiceActive(true);
  }, []);

  const handleSend = useCallback(async () => {
    const trimmed = text.trim();
    if (!trimmed) return;

    const clientRequestId = globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random()}`;
    const optimistic: MessageItem = {
      id: `local:${clientRequestId}`,
      clientRequestId,
      localState: 'pending',
      chatJid: chatJID,
      senderJid: '',
      senderName: 'Anda',
      content: trimmed,
      timestamp: Math.floor(Date.now() / 1000),
      isFromMe: true,
      isRead: true,
      deliveryStatus: 'pending',
      replyTo: replyTo || undefined,
    };
    addOptimisticMessage(optimistic, trimmed);
    setText('');
		onCancelReply?.();
		requestAnimationFrame(() => textareaRef.current?.focus());
    try {
      const mod = await import('../../wailsjs/go/whatsapp/WhatsAppService');
      if (replyTo) {
        await mod.SendReply(chatJID, trimmed, replyTo, clientRequestId);
      } else {
        await mod.SendMessage(chatJID, trimmed, clientRequestId);
      }
    } catch (err: any) {
      console.error('Failed to send message:', err);
      markOutgoingFailed(chatJID, clientRequestId);
    }
  }, [text, chatJID, replyTo, onCancelReply, addOptimisticMessage, markOutgoingFailed]);

  const sendDrafts = useCallback(() => {
    const outgoing = [...drafts];
    if (!outgoing.length) return;
    const sentCaption = caption.trim();
    setDrafts([]);
    setCaption('');
    outgoing.forEach((draft, index) => {
      const clientRequestId = globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random()}`;
      const itemCaption = index === 0 ? sentCaption : '';
      const label = itemCaption || (draft.kind === 'image' ? 'Foto' : draft.kind === 'video' ? 'Video' : draft.kind === 'audio' ? (draft.isPtt ? 'Pesan suara' : 'Audio') : `Dokumen: ${draft.fileName}`);
      addOptimisticMessage({
        id: `local:${clientRequestId}`, clientRequestId, localState: 'pending', draftId: draft.id,
        chatJid: chatJID, senderJid: '', senderName: 'Anda', content: itemCaption,
        caption: itemCaption, timestamp: Math.floor(Date.now() / 1000), isFromMe: true, isRead: true,
        mediaType: draft.kind, fileName: draft.fileName, mimetype: draft.mimetype, fileSize: draft.fileSize,
        isPtt: draft.isPtt, deliveryStatus: 'pending',
      }, label);
      import('../../wailsjs/go/whatsapp/WhatsAppService')
        .then((mod) => mod.SendAttachment(chatJID, draft.id, itemCaption, clientRequestId))
        .catch((error) => { console.error('Failed to send attachment:', error); markOutgoingFailed(chatJID, clientRequestId); });
    });
  }, [drafts, caption, chatJID, addOptimisticMessage, markOutgoingFailed]);

	const sendSticker = useCallback((sticker: StickerItem) => {
		const clientRequestId = globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random()}`;
		addOptimisticMessage({ id: `local:${clientRequestId}`, clientRequestId, localState: 'pending', chatJid: chatJID, senderJid: '', senderName: 'Anda', content: '', timestamp: Math.floor(Date.now() / 1000), isFromMe: true, isRead: true, mediaType: 'sticker', mimetype: sticker.mimetype, deliveryStatus: 'pending' }, 'Stiker');
		setStickerOpen(false);
		import('../../wailsjs/go/whatsapp/WhatsAppService')
			.then((mod) => mod.SendRecentSticker(chatJID, sticker.id, clientRequestId))
			.catch((error) => { console.error('Failed to send sticker:', error); markOutgoingFailed(chatJID, clientRequestId); });
	}, [chatJID, addOptimisticMessage, markOutgoingFailed]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        handleSend();
      }
    },
    [handleSend]
  );

  return (
    <div className="message-input">
			{cameraError && <div className="message-input__capture-error" role="status">{cameraError}<button type="button" onClick={() => setCameraError('')}><X size={16} /></button></div>}
      {replyTo && (
        <div className="message-input__reply">
          <div className="message-input__reply-content">
            <strong>{replyTo.senderName || (replyTo.isFromMe ? 'Anda' : 'Pesan')}</strong>
            <span>{replyTo.content}</span>
          </div>
          <button type="button" className="message-input__reply-close" onClick={onCancelReply} aria-label="Batalkan balasan"><X size={18} /></button>
        </div>
      )}
		{voiceActive ? (
			<VoiceRecorder chatJID={chatJID} onClose={closeVoice} onError={showVoiceError} />
		) : <div className="message-input__container">
			<div className="message-input__attach-wrap">
				<button type="button" className="message-input__icon" onClick={() => setAttachOpen((open) => !open)} aria-label="Tambah lampiran" title="Tambah lampiran"><Paperclip size={21} /></button>
				{attachOpen && <div className="message-input__attach-menu">
					<button type="button" onClick={() => pick('document')}><FileText size={18} /> <span>Document</span></button>
					<button type="button" onClick={() => pick('media')}><Image size={18} /> <span>Photos &amp; videos</span></button>
					<button type="button" onClick={() => { setAttachOpen(false); setCameraOpen(true); }}><Camera size={18} /> <span>Camera</span></button>
					<button type="button" onClick={() => pick('audio')}><Mic size={18} /> <span>Audio</span></button>
				</div>}
			</div>
        <textarea
          ref={textareaRef}
          className="message-input__textarea"
          placeholder="Ketik pesan..."
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={handleKeyDown}
          rows={1}
        />
			<div className="message-input__sticker-wrap"><button type="button" className="message-input__icon" onClick={() => setStickerOpen((open) => !open)} aria-label="Emoji dan stiker" title="Emoji dan stiker"><Smile size={21} /></button>{stickerOpen && <StickerPicker onSelect={sendSticker} />}</div>
        <button
          className="message-input__send"
          onClick={handleSend}
          disabled={!text.trim()}
          title="Kirim pesan"
        >
          <Send size={20} />
        </button>
			<button type="button" className="message-input__voice" onClick={startRecording} disabled={drafts.length > 0 || cameraOpen} aria-label="Rekam voice note" title="Rekam voice note"><Mic size={23} /></button>
      </div>}
			{drafts.length > 0 && <AttachmentComposer drafts={drafts} caption={caption} onCaptionChange={setCaption} onRemove={(id) => setDrafts((current) => current.filter((draft) => draft.id !== id))} onAddMore={() => pick('media')} onCancel={() => { setDrafts([]); setCaption(''); }} onSend={sendDrafts} />}
			{cameraOpen && <div className="capture-dialog" role="dialog" aria-label="Ambil foto"><div className="capture-dialog__card"><div className="capture-dialog__header"><strong>Camera</strong><button type="button" onClick={() => setCameraOpen(false)} aria-label="Tutup kamera"><X size={20} /></button></div>{cameraError ? <p>{cameraError}</p> : <video ref={cameraVideoRef} autoPlay playsInline muted />}<div className="capture-dialog__actions"><button type="button" onClick={() => setCameraOpen(false)}>Batal</button><button type="button" className="capture-dialog__take" onClick={capturePhoto} disabled={!!cameraError}>Ambil foto</button></div></div></div>}
    </div>
  );
}
