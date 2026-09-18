import { useEffect, useState } from 'react';
import type { StickerItem } from '../types';
import { EventsOn } from '../../wailsjs/runtime/runtime';

interface StickerPickerProps {
  onSelect: (sticker: StickerItem) => void;
}

function StickerTile({ sticker, onSelect }: { sticker: StickerItem; onSelect: () => void }) {
  const [image, setImage] = useState('');
  useEffect(() => {
    import('../../wailsjs/go/whatsapp/WhatsAppService')
      .then((mod) => mod.GetStickerPreview(sticker.id))
      .then((base64) => base64 && setImage(`data:${sticker.mimetype};base64,${base64}`))
      .catch(() => {});
  }, [sticker.id, sticker.mimetype]);
  return <button type="button" className="sticker-picker__tile" onClick={onSelect} disabled={!image} title={image ? 'Kirim stiker' : 'Memuat stiker'}>{image ? <img src={image} alt="Stiker" /> : <span className="sticker-picker__loading" />}</button>;
}

export function StickerPicker({ onSelect }: StickerPickerProps) {
  const [stickers, setStickers] = useState<StickerItem[]>([]);
  const [loading, setLoading] = useState(true);

  const reload = () => {
    setLoading(true);
    import('../../wailsjs/go/whatsapp/WhatsAppService')
      .then((mod) => mod.GetRecentStickers())
      .then((items) => setStickers(items as unknown as StickerItem[]))
      .catch(() => setStickers([]))
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    reload();
    return EventsOn('wa:stickers-update', (items: StickerItem[]) => {
      setStickers(items || []);
      setLoading(false);
    });
  }, []);
  return <div className="sticker-picker" role="dialog" aria-label="Stiker terbaru"><div className="sticker-picker__tabs"><strong>◷ Terbaru</strong><button type="button" onClick={reload} disabled={loading} title="Muat ulang stiker" aria-label="Muat ulang stiker">↻</button></div>{loading ? <div className="sticker-picker__empty">Memuat stiker…</div> : stickers.length ? <div className="sticker-picker__grid">{stickers.map((sticker) => <StickerTile key={sticker.id} sticker={sticker} onSelect={() => onSelect(sticker)} />)}</div> : <div className="sticker-picker__empty"><span>Belum ada stiker terbaru yang diterima dari sinkronisasi WhatsApp.</span><button type="button" onClick={reload}>Muat ulang</button></div>}</div>;
}
