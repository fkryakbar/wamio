import { useCallback, useEffect, useRef, useState } from 'react';
import { Mic, Pause, Play, Send, Trash2 } from 'lucide-react';
import { useChatStore } from '../stores/chatStore';
import type { AttachmentDraft, MessageItem } from '../types';
import { encodeVoiceNote, type VoiceSegment, warmVoiceEncoder } from '../lib/voiceEncoder';

type VoicePhase = 'starting' | 'recording' | 'preparing' | 'paused' | 'sending';

interface VoiceRecorderProps {
  chatJID: string;
  onClose: () => void;
  onError: (message: string) => void;
}

function formatDuration(milliseconds: number): string {
  const seconds = Math.floor(milliseconds / 1000);
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`;
}

function normalizeWaveform(samples: number[], size = 64): number[] {
  if (!samples.length) return Array.from({ length: size }, () => 8);
  return Array.from({ length: size }, (_, index) => {
    const start = Math.floor(index * samples.length / size);
    const end = Math.max(start + 1, Math.floor((index + 1) * samples.length / size));
    const peak = Math.max(...samples.slice(start, end));
    return Math.max(1, Math.min(100, Math.round(peak * 100)));
  });
}

function blobToDataURL(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = () => reject(new Error('Voice note tidak dapat dibaca.'));
    reader.readAsDataURL(blob);
  });
}

export function VoiceRecorder({ chatJID, onClose, onError }: VoiceRecorderProps) {
  const addOptimisticMessage = useChatStore((state) => state.addOptimisticMessage);
  const markOutgoingFailed = useChatStore((state) => state.markOutgoingFailed);
  const [phase, setPhase] = useState<VoicePhase>('starting');
  const [elapsed, setElapsed] = useState(0);
  const [levels, setLevels] = useState<number[]>([]);
  const [previewURL, setPreviewURL] = useState('');
  const [previewPlaying, setPreviewPlaying] = useState(false);
  const [error, setError] = useState('');
  const streamRef = useRef<MediaStream | null>(null);
  const recorderRef = useRef<MediaRecorder | null>(null);
  const chunksRef = useRef<Blob[]>([]);
  const segmentsRef = useRef<VoiceSegment[]>([]);
  const mimeRef = useRef('');
  const activeStartedRef = useRef(0);
  const accumulatedRef = useRef(0);
  const waveformRef = useRef<number[]>([]);
  const encodedRef = useRef<Blob | null>(null);
  const previewAudioRef = useRef<HTMLAudioElement>(null);
  const audioContextRef = useRef<AudioContext | null>(null);
  const animationRef = useRef<number | null>(null);
  const sampleAtRef = useRef(0);
  const sessionRef = useRef(1);
  const recordingRef = useRef(false);

  const stopPreview = useCallback(() => {
    previewAudioRef.current?.pause();
    setPreviewPlaying(false);
  }, []);

  const clearPreview = useCallback(() => {
    stopPreview();
    setPreviewURL((current) => {
      if (current) URL.revokeObjectURL(current);
      return '';
    });
    encodedRef.current = null;
  }, [stopPreview]);

  const cleanup = useCallback(() => {
    recordingRef.current = false;
    if (animationRef.current != null) cancelAnimationFrame(animationRef.current);
    animationRef.current = null;
    const recorder = recorderRef.current;
    recorderRef.current = null;
    if (recorder && recorder.state !== 'inactive') {
      recorder.ondataavailable = null;
      recorder.stop();
    }
    streamRef.current?.getTracks().forEach((track) => track.stop());
    streamRef.current = null;
    void audioContextRef.current?.close();
    audioContextRef.current = null;
    setPreviewURL((current) => {
      if (current) URL.revokeObjectURL(current);
      return '';
    });
  }, []);

  const beginSegment = useCallback((stream: MediaStream, mimeType: string) => {
    chunksRef.current = [];
    const recorder = new MediaRecorder(stream, { mimeType });
    recorder.ondataavailable = (event) => {
      if (event.data.size) chunksRef.current.push(event.data);
    };
    recorder.onerror = () => setError('Perekaman voice note gagal.');
    recorderRef.current = recorder;
    activeStartedRef.current = performance.now();
    recordingRef.current = true;
    recorder.start(250);
    setPhase('recording');
  }, []);

  const finishSegment = useCallback(async () => {
    const recorder = recorderRef.current;
    if (!recorder || recorder.state === 'inactive') return;
    recordingRef.current = false;
    const startedAt = activeStartedRef.current;
    await new Promise<void>((resolve) => {
      recorder.addEventListener('stop', () => resolve(), { once: true });
      recorder.stop();
    });
    recorderRef.current = null;
    accumulatedRef.current += Math.max(0, performance.now() - startedAt);
    setElapsed(accumulatedRef.current);
    const blob = new Blob(chunksRef.current, { type: mimeRef.current });
    if (blob.size) segmentsRef.current.push({ blob, mimeType: mimeRef.current });
    chunksRef.current = [];
  }, []);

  const encodeCurrent = useCallback(async () => {
    if (encodedRef.current) return encodedRef.current;
    if (!segmentsRef.current.length) throw new Error('Rekaman voice note kosong.');
    const encoded = await encodeVoiceNote(segmentsRef.current);
    encodedRef.current = encoded;
    return encoded;
  }, []);

  useEffect(() => {
    const session = sessionRef.current;
    void navigator.mediaDevices.getUserMedia({ audio: true }).then((stream) => {
      if (sessionRef.current !== session) {
        stream.getTracks().forEach((track) => track.stop());
        return;
      }
      const mimeType = MediaRecorder.isTypeSupported('audio/webm;codecs=opus')
        ? 'audio/webm;codecs=opus'
        : MediaRecorder.isTypeSupported('audio/ogg;codecs=opus')
          ? 'audio/ogg;codecs=opus'
          : '';
      if (!mimeType) throw new Error('Browser tidak mendukung perekaman audio Opus.');
      streamRef.current = stream;
      mimeRef.current = mimeType;
      const context = new AudioContext();
      const analyser = context.createAnalyser();
      analyser.fftSize = 256;
      context.createMediaStreamSource(stream).connect(analyser);
      audioContextRef.current = context;
      const buffer = new Uint8Array(analyser.fftSize);
      const sample = (now: number) => {
        if (sessionRef.current !== session) return;
        if (recordingRef.current && now - sampleAtRef.current >= 90) {
          analyser.getByteTimeDomainData(buffer);
          let peak = 0;
          for (const value of buffer) peak = Math.max(peak, Math.abs(value - 128) / 128);
          waveformRef.current.push(peak);
          setLevels((current) => [...current.slice(-47), peak]);
          sampleAtRef.current = now;
        }
        animationRef.current = requestAnimationFrame(sample);
      };
      animationRef.current = requestAnimationFrame(sample);
      void warmVoiceEncoder().catch(() => {});
      beginSegment(stream, mimeType);
    }).catch((reason) => {
      const message = reason instanceof Error ? reason.message : 'Mikrofon tidak tersedia atau izin ditolak.';
      onError(message);
      onClose();
    });
    return () => {
      sessionRef.current += 1;
      cleanup();
    };
  }, [beginSegment, cleanup, onClose, onError]);

  useEffect(() => {
    if (phase !== 'recording') return;
    const timer = window.setInterval(() => {
      setElapsed(accumulatedRef.current + Math.max(0, performance.now() - activeStartedRef.current));
    }, 100);
    return () => window.clearInterval(timer);
  }, [phase]);

  const pause = async () => {
    setError('');
    setPhase('preparing');
    try {
      await finishSegment();
      const encoded = await encodeCurrent();
      setPreviewURL((current) => {
        if (current) URL.revokeObjectURL(current);
        return URL.createObjectURL(encoded);
      });
      setPhase('paused');
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Preview voice note gagal disiapkan.');
      setPhase('paused');
    }
  };

  const resume = () => {
    const stream = streamRef.current;
    if (!stream) return;
    clearPreview();
    beginSegment(stream, mimeRef.current);
  };

  const discard = () => {
    sessionRef.current += 1;
    cleanup();
    onClose();
  };

  const togglePreview = async () => {
    const audio = previewAudioRef.current;
    if (!audio) return;
    try {
      if (audio.paused) await audio.play();
      else audio.pause();
    } catch {
      setError('Preview voice note tidak dapat diputar.');
    }
  };

  const send = async () => {
    const session = sessionRef.current;
    setError('');
    setPhase('preparing');
    try {
      if (recorderRef.current?.state !== 'inactive') await finishSegment();
      const voice = await encodeCurrent();
      if (sessionRef.current !== session || useChatStore.getState().activeChatJID !== chatJID) return;
      setPhase('sending');
      const dataURL = await blobToDataURL(voice);
      const waveform = normalizeWaveform(waveformRef.current);
      const duration = Math.max(1, Math.ceil(accumulatedRef.current / 1000));
      const app = await import('../../wailsjs/go/main/App');
      const draft = await app.StageVoiceNote(dataURL, `voice-${Date.now()}.ogg`, duration, waveform) as unknown as AttachmentDraft;
      if (sessionRef.current !== session || useChatStore.getState().activeChatJID !== chatJID) return;
      const clientRequestId = globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random()}`;
      const optimistic: MessageItem = {
        id: `local:${clientRequestId}`,
        clientRequestId,
        localState: 'pending',
        draftId: draft.id,
        chatJid: chatJID,
        senderJid: '',
        senderName: 'Anda',
        content: '',
        timestamp: Math.floor(Date.now() / 1000),
        isFromMe: true,
        isRead: true,
        mediaType: 'audio',
        mediaUrl: dataURL,
        mediaDuration: duration,
        waveform,
        fileName: draft.fileName,
        mimetype: draft.mimetype,
        fileSize: draft.fileSize,
        isPtt: true,
        deliveryStatus: 'pending',
      };
      addOptimisticMessage(optimistic, 'Pesan suara');
      cleanup();
      onClose();
      import('../../wailsjs/go/whatsapp/WhatsAppService')
        .then((mod) => mod.SendAttachment(chatJID, draft.id, '', clientRequestId))
        .catch((reason) => {
          console.error('Failed to send voice note:', reason);
          markOutgoingFailed(chatJID, clientRequestId);
        });
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Voice note tidak dapat disiapkan.');
      setPhase('paused');
    }
  };

  const shownLevels = levels.length ? levels : Array.from({ length: 24 }, () => 0.08);

  return (
    <div className="voice-composer" role="group" aria-label="Perekam voice note">
      <button type="button" className="voice-composer__discard" onClick={discard} aria-label="Buang voice note" title="Buang voice note"><Trash2 size={20} /></button>
      <div className="voice-composer__status">
        <span className={`voice-composer__dot${phase === 'recording' ? ' voice-composer__dot--active' : ''}`} />
        <span className="voice-composer__time">{formatDuration(elapsed)}</span>
      </div>
      <div className="voice-composer__waveform" aria-hidden="true">
        {shownLevels.map((level, index) => <span key={index} style={{ height: `${Math.max(4, level * 28)}px` }} />)}
      </div>
      {previewURL && <audio ref={previewAudioRef} src={previewURL} onPlay={() => setPreviewPlaying(true)} onPause={() => setPreviewPlaying(false)} onEnded={() => setPreviewPlaying(false)} />}
      {phase === 'recording' && <button type="button" className="voice-composer__control" onClick={() => { void pause(); }} aria-label="Pause rekaman" title="Pause rekaman"><Pause size={20} /></button>}
      {phase === 'paused' && <button type="button" className="voice-composer__control" onClick={() => { void togglePreview(); }} disabled={!previewURL} aria-label={previewPlaying ? 'Pause preview' : 'Putar preview'} title={previewPlaying ? 'Pause preview' : 'Putar preview'}>{previewPlaying ? <Pause size={20} /> : <Play size={20} />}</button>}
      {phase === 'paused' && <button type="button" className="voice-composer__control" onClick={resume} aria-label="Lanjutkan rekaman" title="Lanjutkan rekaman"><Mic size={20} /></button>}
      {(phase === 'preparing' || phase === 'sending' || phase === 'starting') && <div className="spinner spinner--sm" aria-label={phase === 'sending' ? 'Mengirim voice note' : 'Menyiapkan voice note'} />}
      <button type="button" className="voice-composer__send" onClick={() => { void send(); }} disabled={phase === 'starting' || phase === 'preparing' || phase === 'sending'} aria-label="Kirim voice note" title="Kirim voice note"><Send size={20} /></button>
      {error && <span className="voice-composer__error" role="status">{error}</span>}
    </div>
  );
}
