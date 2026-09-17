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
  lastMessageFromMe: boolean;
  lastMessageStatus: string;
	isArchived: boolean;
	isPinned: boolean;
	isMuted: boolean;
	mutedUntil: number;
	listIds?: string[];
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
  thumbnail?: string;
  isPtt?: boolean;
  deliveryStatus?: string;
  replyTo?: MessageReference;
  isForwarded?: boolean;
	isDeleted?: boolean;
	caption?: string;
	fileSize?: number;
	kind?: string;
	call?: CallInfo;
  reactions?: MessageReaction[];
}

export interface CallInfo { callId: string; outcome: string; duration?: number; isVideo: boolean; isIncoming: boolean; }
export interface ChatList { id: string; name: string; type: string; order: number; isActive: boolean; }
export interface DocumentState { downloaded: boolean; fileName: string; fileSize: number; }
export interface RestoreSessionResult { attempted: boolean; accountId?: string; }

export interface MessageReference {
  id: string;
  chatJid: string;
  senderJid: string;
  senderName: string;
  content: string;
  mediaType?: string;
  isFromMe: boolean;
}

export interface MessageReaction {
  emoji: string;
  count: number;
  fromMe: boolean;
}

export interface MessageReactionEvent {
  chatJid: string;
  messageId: string;
  reactions: MessageReaction[];
}

export interface MessageDeleteEvent {
  chatJid: string;
  messageId: string;
  forMe: boolean;
}

export interface ChatProfile {
  jid: string;
  name: string;
  avatar?: string;
  isGroup: boolean;
  phoneNumber?: string;
  description?: string;
  participantCount?: number;
}

export interface ForwardResult {
  chatJid: string;
  success: boolean;
  error?: string;
}

export interface MessageEvent {
  chatJid: string;
  message: MessageItem;
}

export interface ChatUpdateEvent {
  chat: ChatItem;
}

export interface MessageReceiptEvent {
  chatJid: string;
  messageIds: string[];
  deliveryStatus: 'delivered' | 'read';
}

export interface NotificationEvent {
  chatJid: string;
  chatName: string;
  senderName: string;
  content: string;
  isGroup: boolean;
}

export interface InitialSyncEvent {
  state: 'running' | 'ready' | 'degraded';
  message?: string;
}

export interface SyncProgressEvent {
  phase: 'initial' | 'background' | 'complete' | 'degraded';
  progress: number;
  processedChats: number;
  preparedChats: number;
  message?: string;
}

export interface PresenceEvent {
  chatJid: string;
  online: boolean;
  lastSeen?: number;
  unavailable: boolean;
}

export interface ChatPresenceEvent {
  chatJid: string;
  typing: boolean;
}

export interface MessagePage {
  messages: MessageItem[];
  hasMoreLocal: boolean;
  canRequestOlder: boolean;
}

export interface HistoryPageEvent {
  chatJid: string;
  messages: MessageItem[];
  canRequestOlder: boolean;
  error?: string;
}

export interface ChatWithMessages {
  chat: ChatItem;
  messages: MessageItem[];
}
