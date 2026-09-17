import { useState, useCallback, useEffect } from 'react';
import type { DocumentState, MessageItem } from '../types';
import { FormattedMessage } from './FormattedMessage';

const mediaMemoryCache = new Map<string, string>();

function mediaCacheKey(message: MessageItem): string {
  return `${message.chatJid}:${message.id}`;
}

interface MediaMessageProps {
  message: MessageItem;
  onOpenLightbox: (src: string) => void;
}

export function MediaMessage({ message, onOpenLightbox }: MediaMessageProps) {
	const [mediaData, setMediaData] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [documentState, setDocumentState] = useState<DocumentState | null>(null);
  const [documentBusy, setDocumentBusy] = useState(false);

	const key = mediaCacheKey(message);

	useEffect(() => {
		let active = true;
		setMediaData(mediaMemoryCache.get(key) || null);
		setError(false);
		if (mediaMemoryCache.has(key)) return () => { active = false; };
		import('../../wailsjs/go/whatsapp/WhatsAppService')
			.then((mod) => mod.GetCachedMedia(message.chatJid, message.id))
			.then((base64) => {
				if (!active || !base64) return;
				mediaMemoryCache.set(key, base64);
				setMediaData(base64);
			})
			.catch(() => {});
		return () => { active = false; };
	}, [key, message.chatJid, message.id]);

  useEffect(() => {
    if (message.mediaType !== 'document') return;
    let active = true;
    import('../../wailsjs/go/whatsapp/WhatsAppService').then((mod) => mod.GetDocumentState(message.chatJid, message.id))
      .then((state) => { if (active) setDocumentState(state); }).catch(() => {});
    return () => { active = false; };
  }, [message.chatJid, message.id, message.mediaType]);

	const loadMedia = useCallback(async () => {
		if (mediaData || loading) return;
		setLoading(true);
    setError(false);
    try {
      const mod = await import('../../wailsjs/go/whatsapp/WhatsAppService');
		const base64 = await mod.DownloadMedia(message.chatJid, message.id);
		if (base64) {
			mediaMemoryCache.set(key, base64);
			setMediaData(base64);
      } else {
        setError(true);
      }
    } catch {
      setError(true);
    } finally {
      setLoading(false);
    }
	}, [key, message.chatJid, message.id, mediaData, loading]);

  const getMimePrefix = () => {
    const mime = message.mimetype || '';
    if (mime.startsWith('image/')) return `data:${mime};base64,`;
    if (mime.startsWith('video/')) return `data:${mime};base64,`;
    return 'data:image/jpeg;base64,';
  };

  // Automatically load stickers on render
  useEffect(() => {
    if (message.mediaType === 'sticker') {
      loadMedia();
    }
  }, [message.mediaType, loadMedia]);

	if (message.mediaType === 'image' || message.mediaType === 'sticker') {
		const isSticker = message.mediaType === 'sticker';
		const thumbnailSrc = message.thumbnail ? `data:image/jpeg;base64,${message.thumbnail}` : '';
    return (
      <div className={`media-message media-message--image ${isSticker ? 'media-message--sticker' : ''}`}>
        {mediaData ? (
          <img
            src={`${getMimePrefix()}${mediaData}`}
            alt={message.content}
            className="media-message__img"
            onClick={() => !isSticker && onOpenLightbox(`${getMimePrefix()}${mediaData}`)}
          />
		) : (
			<div className="media-message__placeholder" onClick={loadMedia}>
				{thumbnailSrc && <img src={thumbnailSrc} alt="" className="media-message__thumbnail" />}
				<div className="media-message__download-overlay">
				{loading ? (
					<div className="spinner spinner--sm" />
            ) : error ? (
              <span className="media-message__error">⚠️ Gagal memuat</span>
            ) : (
              <>
                <svg width="32" height="32" viewBox="0 0 24 24" fill="var(--text-tertiary)">
                  <path d="M21 19V5c0-1.1-.9-2-2-2H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2zM8.5 13.5l2.5 3.01L14.5 12l4.5 6H5l3.5-4.5z"/>
                </svg>
                <span>Ketuk untuk muat</span>
					</>
				)}
				</div>
			</div>
        )}
        {message.caption && !isSticker && (
          <span className="media-message__caption"><FormattedMessage content={message.caption} /></span>
        )}
      </div>
    );
  }

  if (message.mediaType === 'video') {
    return (
      <div className="media-message media-message--video">
        {mediaData ? (
          <video
            src={`${getMimePrefix()}${mediaData}`}
            controls
            className="media-message__video"
            preload="metadata"
          />
        ) : (
          <div className="media-message__placeholder" onClick={loadMedia}>
            {loading ? (
              <div className="spinner spinner--sm" />
            ) : (
              <>
                <svg width="40" height="40" viewBox="0 0 24 24" fill="var(--wa-teal)">
                  <path d="M8 5v14l11-7z"/>
                </svg>
                <span>{message.mediaDuration ? `${Math.floor(message.mediaDuration / 60)}:${String(message.mediaDuration % 60).padStart(2, '0')}` : 'Video'}</span>
              </>
            )}
          </div>
        )}
        {message.caption && <span className="media-message__caption"><FormattedMessage content={message.caption} /></span>}
      </div>
    );
  }

  if (message.mediaType === 'document') {
		const download = async () => {
			if (documentBusy) return;
			setDocumentBusy(true); setError(false);
			try { const mod = await import('../../wailsjs/go/whatsapp/WhatsAppService'); setDocumentState(await mod.DownloadDocument(message.chatJid, message.id)); }
			catch { setError(true); } finally { setDocumentBusy(false); }
		};
		const open = () => import('../../wailsjs/go/whatsapp/WhatsAppService').then((mod) => mod.OpenDocument(message.chatJid, message.id)).catch(() => setError(true));
		const saveAs = () => import('../../wailsjs/go/whatsapp/WhatsAppService').then((mod) => mod.SaveDocumentAs(message.chatJid, message.id)).catch(() => setError(true));
		const bytes = documentState?.fileSize ?? message.fileSize ?? 0;
    return (
      <div className="media-message media-message--document">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="var(--text-link)">
          <path d="M14 2H6c-1.1 0-1.99.9-1.99 2L4 20c0 1.1.89 2 1.99 2H18c1.1 0 2-.9 2-2V8l-6-6zm2 16H8v-2h8v2zm0-4H8v-2h8v2zm-3-5V3.5L18.5 9H13z"/>
        </svg>
        <div className="media-message__doc-info">
          <span className="media-message__doc-name">{message.fileName || 'Dokumen'}</span>
          <span className="media-message__doc-type">{message.mimetype || 'Dokumen'}{bytes ? ` · ${(bytes / 1024).toFixed(bytes > 1024 * 1024 ? 1 : 0)} ${bytes > 1024 * 1024 ? 'MB' : 'KB'}` : ''}</span>
				<div className="media-message__doc-actions">
					{documentState?.downloaded ? <><button type="button" onClick={open}>Buka</button><button type="button" onClick={saveAs}>Save as…</button></> : <button type="button" onClick={download} disabled={documentBusy}>{documentBusy ? 'Mengunduh…' : 'Download'}</button>}
				</div>
          {message.caption && <span className="media-message__caption"><FormattedMessage content={message.caption} /></span>}
				{error && <span className="media-message__error">Gagal memproses dokumen</span>}
        </div>
      </div>
    );
  }

  return null;
}
