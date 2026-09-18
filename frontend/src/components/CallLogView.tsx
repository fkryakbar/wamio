import { useEffect, useState } from "react";
import { PhoneIncoming, PhoneMissed, PhoneOutgoing, Video } from "lucide-react";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { useChatStore } from "../stores/chatStore";
import type { CallLogEntry } from "../types";

function formatTime(timestamp: number) {
  if (!timestamp) return "";
  const date = new Date(timestamp * 1000);
  const today = new Date();
  if (date.toDateString() === today.toDateString()) {
    return date.toLocaleTimeString("id-ID", { hour: "2-digit", minute: "2-digit" });
  }
  return date.toLocaleDateString("id-ID", { day: "2-digit", month: "short", year: "numeric" });
}

function formatDuration(seconds?: number) {
  if (!seconds) return "";
  const minutes = Math.floor(seconds / 60);
  const remainder = seconds % 60;
  return minutes ? `${minutes} m ${remainder} dtk` : `${remainder} dtk`;
}

function isMissed(outcome: string) {
  const normalized = outcome.toLowerCase();
  return normalized.includes("miss") || normalized.includes("reject") || normalized.includes("not_answer");
}

function CallTypeIcon({ call }: { call: CallLogEntry }) {
  if (call.isVideo) return <Video size={21} />;
  if (isMissed(call.outcome)) return <PhoneMissed size={21} />;
  return call.isIncoming ? <PhoneIncoming size={21} /> : <PhoneOutgoing size={21} />;
}

export function CallLogView() {
  const [calls, setCalls] = useState<CallLogEntry[]>([]);
  const setActiveChat = useChatStore((state) => state.setActiveChat);
  const setActiveRailView = useChatStore((state) => state.setActiveRailView);

  useEffect(() => {
    let disposed = false;
    import("../../wailsjs/go/whatsapp/WhatsAppService")
      .then((mod) => mod.GetCallLog().then((entries: CallLogEntry[]) => {
        if (!disposed) setCalls(entries || []);
      }))
      .catch(() => {});
    const cancel = EventsOn("wa:call-log", (entries: CallLogEntry[]) => setCalls(entries || []));
    return () => {
      disposed = true;
      cancel();
    };
  }, []);

  const openChat = (chatJid: string) => {
    setActiveChat(chatJid);
    setActiveRailView("chats");
  };

  return (
    <main className="call-log" aria-label="Log panggilan">
      <header className="call-log__header">
        <h1>Log panggilan</h1>
        <span>{calls.length ? `${calls.length} panggilan` : ""}</span>
      </header>
      {calls.length === 0 ? (
        <div className="call-log__empty">
          <PhoneOutgoing size={36} strokeWidth={1.5} />
          <h2>Belum ada riwayat panggilan</h2>
          <p>Riwayat panggilan dari WhatsApp akan tampil di sini setelah tersinkron.</p>
        </div>
      ) : (
        <div className="call-log__list">
          {calls.map((call) => (
            <button key={`${call.chatJid}-${call.timestamp}-${call.outcome}`} type="button" className="call-log__item" onClick={() => openChat(call.chatJid)}>
              <span className={`call-log__icon ${isMissed(call.outcome) ? "call-log__icon--missed" : ""}`}><CallTypeIcon call={call} /></span>
              <span className="call-log__details">
                <strong>{call.chatName || call.chatJid}</strong>
                <small>{call.isIncoming ? "Panggilan masuk" : "Panggilan keluar"}{call.isVideo ? " · Video" : " · Suara"}{formatDuration(call.duration) ? ` · ${formatDuration(call.duration)}` : ""}</small>
              </span>
              <time>{formatTime(call.timestamp)}</time>
            </button>
          ))}
        </div>
      )}
    </main>
  );
}
