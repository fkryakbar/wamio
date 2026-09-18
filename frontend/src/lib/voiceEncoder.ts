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
export async function encodeVoiceNote(source: Blob): Promise<Blob> {
  const ffmpeg = await getEncoder();
  const token = `${Date.now()}-${Math.random().toString(36).slice(2)}`;
  const input = `recording-${token}.webm`;
  const output = `voice-${token}.ogg`;
  await ffmpeg.writeFile(input, await fetchFile(source));
  const result = await ffmpeg.exec(['-i', input, '-vn', '-c:a', 'libopus', '-b:a', '64k', '-application', 'voip', output], 60000);
  if (result !== 0) throw new Error('Konversi OGG/Opus gagal');
  const encoded = await ffmpeg.readFile(output);
  await Promise.all([ffmpeg.deleteFile(input), ffmpeg.deleteFile(output)]);
  if (typeof encoded === 'string') throw new Error('Data audio tidak valid');
  return new Blob([encoded], { type: 'audio/ogg; codecs=opus' });
}
