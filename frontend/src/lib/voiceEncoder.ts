import { FFmpeg } from '@ffmpeg/ffmpeg';
import { fetchFile } from '@ffmpeg/util';
// Vite 3 cannot apply ?url to this package's export map, so resolve the
// installed, version-locked core assets directly. They are emitted into dist.
import coreURL from '../../node_modules/@ffmpeg/core/dist/esm/ffmpeg-core.js?url';
import wasmURL from '../../node_modules/@ffmpeg/core/dist/esm/ffmpeg-core.wasm?url';

let encoder: FFmpeg | null = null;
let loading: Promise<void> | null = null;

function withTimeout<T>(promise: Promise<T>, timeoutMs: number, label: string): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const timer = window.setTimeout(() => reject(new Error(`${label} melebihi batas waktu`)), timeoutMs);
    promise.then((value) => { window.clearTimeout(timer); resolve(value); }, (error) => { window.clearTimeout(timer); reject(error); });
  });
}

async function getEncoder(): Promise<FFmpeg> {
  if (!encoder) encoder = new FFmpeg();
  if (!encoder.loaded) {
    loading ||= withTimeout(encoder.load({ coreURL, wasmURL }).then(() => undefined), 45000, 'Memuat encoder voice note')
      .catch((error) => {
        encoder?.terminate();
        encoder = null;
        loading = null;
        throw error;
      });
    await loading;
  }
  return encoder;
}

// Chromium records Opus in WebM. WhatsApp voice notes require OGG/Opus, so
// conversion stays entirely in the webview before any bytes cross the bridge.
export interface VoiceSegment {
  blob: Blob;
  mimeType: string;
}

function extensionForMime(mimeType: string): string {
  return mimeType.includes('ogg') ? 'ogg' : mimeType.includes('mp4') ? 'm4a' : 'webm';
}

export async function warmVoiceEncoder(): Promise<void> {
  await getEncoder();
}

export async function encodeVoiceNote(source: Blob | VoiceSegment[]): Promise<Blob> {
  const ffmpeg = await getEncoder();
  const token = `${Date.now()}-${Math.random().toString(36).slice(2)}`;
  const segments: VoiceSegment[] = Array.isArray(source)
    ? source
    : [{ blob: source, mimeType: source.type || 'audio/webm' }];
  if (!segments.length) throw new Error('Rekaman kosong');
  const inputs = segments.map((segment, index) => `recording-${token}-${index}.${extensionForMime(segment.mimeType || segment.blob.type)}`);
  const output = `voice-${token}.ogg`;
  try {
    await Promise.all(segments.map(async (segment, index) => ffmpeg.writeFile(inputs[index], await fetchFile(segment.blob))));
    const args = inputs.flatMap((input) => ['-i', input]);
    if (inputs.length > 1) {
      const streams = inputs.map((_, index) => `[${index}:a]`).join('');
      args.push('-filter_complex', `${streams}concat=n=${inputs.length}:v=0:a=1[a]`, '-map', '[a]');
    }
    args.push('-vn', '-c:a', 'libopus', '-b:a', '64k', '-application', 'voip', output);
    const result = await ffmpeg.exec(args, 60000);
    if (result !== 0) throw new Error('Konversi OGG/Opus gagal');
    const encoded = await ffmpeg.readFile(output);
    if (typeof encoded === 'string') throw new Error('Data audio tidak valid');
    return new Blob([encoded], { type: 'audio/ogg; codecs=opus' });
  } finally {
    await Promise.all([...inputs, output].map((path) => ffmpeg.deleteFile(path).catch(() => {})));
  }
}
