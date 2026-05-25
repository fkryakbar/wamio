// Connection states matching Go backend
export type ConnectionState =
  | 'disconnected'
  | 'connecting'
  | 'qr_ready'
  | 'connected'
  | 'logged_out';

// Events from the Go backend
export interface QRCodeEvent {
  code: string;
  event: 'code' | 'success' | 'timeout' | 'error';
}

export interface ConnectionStatusEvent {
  state: ConnectionState;
  message?: string;
}

export interface UserInfo {
  jid: string;
  pushName: string;
  phoneNumber: string;
  platform: string;
}

// Chat & Message types
export interface ChatItem {
  jid: string;
  name: string;
  lastMessage: string;
  lastMessageTime: number;
  unreadCount: number;
  isGroup: boolean;
  avatar?: string;
}

export interface MessageItem {
  id: string;
  chatJid: string;
  senderJid: string;
  senderName: string;
  content: string;
  timestamp: number;
  isFromMe: boolean;
  isRead: boolean;
  mediaType?: string;
  mediaUrl?: string;
  mediaDuration?: number;
  fileName?: string;
  mimetype?: string;
  isPtt?: boolean;
}

export interface MessageEvent {
  chatJid: string;
  message: MessageItem;
}

export interface ChatUpdateEvent {
  chat: ChatItem;
}

export interface NotificationEvent {
  chatJid: string;
  chatName: string;
  senderName: string;
  content: string;
  isGroup: boolean;
}

export interface InitialSyncEvent {
  state: 'running' | 'done';
}

export interface ChatWithMessages {
  chat: ChatItem;
  messages: MessageItem[];
}
