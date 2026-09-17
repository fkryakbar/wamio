import type { ReactNode } from 'react';

const formatToken = /```([\s\S]+?)```|\*\*([^*\n]+)\*\*|\*([^*\n]+)\*|_([^_\n]+)_|~([^~\n]+)~/g;
const urlToken = /(?:https?:\/\/|www\.)[^\s<>()]+/gi;

function linkify(value: string, keyStart: number): ReactNode[] {
  const nodes: ReactNode[] = [];
	urlToken.lastIndex = 0;
  let cursor = 0; let match: RegExpExecArray | null; let key = keyStart;
  while ((match = urlToken.exec(value)) !== null) {
    if (match.index > cursor) nodes.push(value.slice(cursor, match.index));
    const url = match[0];
    nodes.push(<button key={key++} type="button" className="message__link" onClick={() => import('../../wailsjs/go/whatsapp/WhatsAppService').then((mod) => mod.OpenExternalURL(url)).catch(() => {})}>{url}</button>);
    cursor = match.index + url.length;
  }
  if (cursor < value.length) nodes.push(value.slice(cursor));
  return nodes;
}

// WhatsApp formatting is rendered as React nodes rather than injected HTML so
// message text remains safe and natively selectable.
export function FormattedMessage({ content }: { content: string }) {
  const nodes: ReactNode[] = [];
  let cursor = 0;
  let match: RegExpExecArray | null;
  let key = 0;
  while ((match = formatToken.exec(content)) !== null) {
    if (match.index > cursor) nodes.push(...linkify(content.slice(cursor, match.index), key++ * 100));
    if (match[1] !== undefined) nodes.push(<code key={key++}>{match[1]}</code>);
    else if (match[2] !== undefined || match[3] !== undefined) nodes.push(<strong key={key++}>{match[2] ?? match[3]}</strong>);
    else if (match[4] !== undefined) nodes.push(<em key={key++}>{match[4]}</em>);
    else nodes.push(<s key={key++}>{match[5]}</s>);
    cursor = match.index + match[0].length;
  }
  if (cursor < content.length) nodes.push(...linkify(content.slice(cursor), key++ * 100));
  return <>{nodes}</>;
}
