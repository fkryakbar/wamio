import { useState, useCallback, useRef, useEffect } from 'react';

interface MessageInputProps {
  chatJID: string;
}

export function MessageInput({ chatJID }: MessageInputProps) {
  const [text, setText] = useState('');
  const [isSending, setIsSending] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

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

  const handleSend = useCallback(async () => {
    const trimmed = text.trim();
    if (!trimmed || isSending) return;

    setIsSending(true);
    try {
      const mod = await import('../../wailsjs/go/whatsapp/WhatsAppService');
      await mod.SendMessage(chatJID, trimmed);
      setText('');
    } catch (err: any) {
      console.error('Failed to send message:', err);
    } finally {
      setIsSending(false);
      textareaRef.current?.focus();
    }
  }, [text, chatJID, isSending]);

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
