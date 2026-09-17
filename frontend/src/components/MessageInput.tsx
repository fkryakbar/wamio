import { useState, useCallback, useRef, useEffect } from 'react';
import type { MessageReference } from '../types';

interface MessageInputProps {
  chatJID: string;
  replyTo?: MessageReference | null;
  onCancelReply?: () => void;
}

export function MessageInput({ chatJID, replyTo, onCancelReply }: MessageInputProps) {
  const [text, setText] = useState('');
  const [isSending, setIsSending] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const restoreFocusRef = useRef(false);

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
    textareaRef.current?.focus();
  }, [chatJID]);

	// Reply is selected by double click or the message menu. Move focus after
	// that state is committed so typing can begin immediately.
	useEffect(() => {
		if (replyTo) requestAnimationFrame(() => textareaRef.current?.focus());
	}, [replyTo?.id]);

  // The textarea is disabled while SendMessage awaits. Focus only after the
  // state transition has rendered it enabled again.
  useEffect(() => {
    if (!isSending && restoreFocusRef.current) {
      restoreFocusRef.current = false;
      textareaRef.current?.focus();
    }
  }, [isSending]);

  const handleSend = useCallback(async () => {
    const trimmed = text.trim();
    if (!trimmed || isSending) return;

    setIsSending(true);
    try {
      const mod = await import('../../wailsjs/go/whatsapp/WhatsAppService');
      if (replyTo) {
        await mod.SendReply(chatJID, trimmed, replyTo);
      } else {
        await mod.SendMessage(chatJID, trimmed);
      }
      setText('');
	  onCancelReply?.();
    } catch (err: any) {
      console.error('Failed to send message:', err);
    } finally {
      restoreFocusRef.current = true;
      setIsSending(false);
    }
  }, [text, chatJID, isSending, replyTo, onCancelReply]);

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
      {replyTo && (
        <div className="message-input__reply">
          <div className="message-input__reply-content">
            <strong>{replyTo.senderName || (replyTo.isFromMe ? 'Anda' : 'Pesan')}</strong>
            <span>{replyTo.content}</span>
          </div>
          <button type="button" className="message-input__reply-close" onClick={onCancelReply} aria-label="Batalkan balasan">×</button>
        </div>
      )}
      <div className="message-input__container">
        <textarea
          ref={textareaRef}
          className="message-input__textarea"
          placeholder="Ketik pesan..."
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={handleKeyDown}
          rows={1}
          disabled={isSending}
        />
        <button
          className="message-input__send"
          onClick={handleSend}
          disabled={!text.trim() || isSending}
          title="Kirim pesan"
        >
          {isSending ? (
            <div className="spinner spinner--sm" />
          ) : (
            <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
              <path d="M2.01 21L23 12 2.01 3 2 10l15 2-15 2z"/>
            </svg>
          )}
        </button>
      </div>
    </div>
  );
}
