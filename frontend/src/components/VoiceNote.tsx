import { useState, useRef, useCallback, useEffect } from 'react';
import type { MessageItem } from '../types';

interface VoiceNoteProps {
  message: MessageItem;
}

export function VoiceNote({ message }: VoiceNoteProps) {
  const [audioData, setAudioData] = useState<string | null>(message.mediaUrl || null);
  const [loading, setLoading] = useState(false);
  const [playing, setPlaying] = useState(false);
  const [progress, setProgress] = useState(0);
  const [currentTime, setCurrentTime] = useState(0);
  const [actualDuration, setActualDuration] = useState(message.mediaDuration || 0);
  const [error, setError] = useState('');
  const audioRef = useRef<HTMLAudioElement>(null);
  const playedReceiptRef = useRef(false);
  const autoplayAfterLoadRef = useRef(false);
  const instanceIDRef = useRef(globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random()}`);
  const duration = actualDuration || message.mediaDuration || 0;

  useEffect(() => {
    setAudioData(message.mediaUrl || null);
    setPlaying(false);
    setProgress(0);
    setCurrentTime(0);
    setActualDuration(message.mediaDuration || 0);
    setError('');
    playedReceiptRef.current = false;
    autoplayAfterLoadRef.current = false;
  }, [message.id, message.mediaDuration, message.mediaUrl]);

  const loadAudio = useCallback(async () => {
    if (audioData || loading) return;
    setLoading(true);
    setError('');
    autoplayAfterLoadRef.current = true;
    try {
      const mod = await import('../../wailsjs/go/whatsapp/WhatsAppService');
      const base64 = await mod.DownloadMedia(message.chatJid, message.id);
      if (base64) {
        const mime = message.mimetype || 'audio/ogg';
        setAudioData(`data:${mime};base64,${base64}`);
      } else {
        autoplayAfterLoadRef.current = false;
        setError('Voice note tidak dapat dimuat.');
      }
    } catch (err) {
      console.error('Failed to load audio:', err);
      autoplayAfterLoadRef.current = false;
      setError('Voice note tidak dapat dimuat.');
    } finally {
      setLoading(false);
    }
  }, [message.chatJid, message.id, audioData, loading, message.mimetype]);

  const togglePlay = useCallback(async () => {
    if (!audioData) {
      await loadAudio();
      return;
    }

    const audio = audioRef.current;
    if (!audio) return;

    if (playing) {
      audio.pause();
      setPlaying(false);
    } else {
      try {
        await audio.play();
      } catch {
        setError('Voice note tidak dapat diputar.');
      }
    }
  }, [audioData, playing, loadAudio]);

  // Start playing once audio data loads
  useEffect(() => {
    if (audioData && audioRef.current && autoplayAfterLoadRef.current) {
      autoplayAfterLoadRef.current = false;
      void audioRef.current.play().catch(() => setError('Voice note tidak dapat diputar.'));
    }
  }, [audioData]);

  useEffect(() => {
    const stopOther = (event: Event) => {
      if ((event as CustomEvent<string>).detail === instanceIDRef.current) return;
      audioRef.current?.pause();
    };
    window.addEventListener('wamio:voice-play', stopOther);
    return () => window.removeEventListener('wamio:voice-play', stopOther);
  }, []);

  const handleTimeUpdate = () => {
    const audio = audioRef.current;
    if (audio && audio.duration) {
      setProgress((audio.currentTime / audio.duration) * 100);
      setCurrentTime(audio.currentTime);
    }
  };

  const handleEnded = () => {
    setPlaying(false);
    setProgress(0);
    setCurrentTime(0);
  };

  const handlePlay = () => {
    setPlaying(true);
    setError('');
    window.dispatchEvent(new CustomEvent('wamio:voice-play', { detail: instanceIDRef.current }));
    if (!message.isFromMe && message.isPtt && !playedReceiptRef.current) {
      playedReceiptRef.current = true;
      import('../../wailsjs/go/whatsapp/WhatsAppService')
        .then((mod) => mod.MarkVoiceMessagePlayed(message.chatJid, message.id))
        .catch(() => { playedReceiptRef.current = false; });
    }
  };

  const handleSeek = (e: React.MouseEvent<HTMLDivElement>) => {
    const audio = audioRef.current;
    if (!audio || !audio.duration) return;
    const rect = e.currentTarget.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const pct = x / rect.width;
    audio.currentTime = pct * audio.duration;
  };

  const formatDuration = (seconds: number) => {
    const m = Math.floor(seconds / 60);
    const s = Math.floor(seconds % 60);
    return `${m}:${String(s).padStart(2, '0')}`;
  };

  // Generate waveform bars
  const bars = Array.from({ length: 28 }, (_, i) => {
    const waveform = message.waveform || [];
    if (waveform.length) {
      const sample = waveform[Math.min(waveform.length - 1, Math.floor(i * waveform.length / 28))];
      return Math.max(4, Math.min(24, (sample / 100) * 24));
    }
    const height = 8 + Math.sin(i * 0.7 + 2) * 10 + Math.cos(i * 1.3) * 6;
    return Math.max(4, Math.min(24, height));
  });

  return (
    <div className="voice-note">
      {audioData && (
        <audio
          ref={audioRef}
          src={audioData}
          onTimeUpdate={handleTimeUpdate}
          onEnded={handleEnded}
          onPlay={handlePlay}
          onPause={() => setPlaying(false)}
          onLoadedMetadata={(event) => {
            if (Number.isFinite(event.currentTarget.duration)) setActualDuration(event.currentTarget.duration);
          }}
          onError={() => setError('Voice note tidak dapat diputar.')}
          preload="metadata"
        />
      )}

      {/* Play/Pause button */}
      <button className="voice-note__play" onClick={togglePlay} disabled={loading}>
        {loading ? (
          <div className="spinner spinner--sm" />
        ) : playing ? (
          <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
            <path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z"/>
          </svg>
        ) : (
          <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
            <path d="M8 5v14l11-7z"/>
          </svg>
        )}
      </button>

      {/* Waveform */}
      <div className="voice-note__waveform" onClick={handleSeek}>
        <div className="voice-note__bars">
          {bars.map((h, i) => (
            <div
              key={i}
              className={`voice-note__bar ${
                progress > (i / bars.length) * 100 ? 'voice-note__bar--played' : ''
              }`}
              style={{ height: `${h}px` }}
            />
          ))}
        </div>
      </div>

      {/* Duration */}
      <span className="voice-note__duration">
        {playing || currentTime > 0
          ? formatDuration(currentTime)
          : formatDuration(duration)}
      </span>
      {error && <span className="voice-note__error" title={error}>!</span>}
    </div>
  );
}
