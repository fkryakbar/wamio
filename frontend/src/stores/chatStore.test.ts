import { beforeEach, describe, expect, it } from 'vitest';
import type { ChatItem, HistoryPageEvent, MessageItem, MessagePage } from '../types';
import { compareMessages, mergeMessageLists, useChatStore } from './chatStore';

function message(id: string, timestamp: number, order: number, overrides: Partial<MessageItem> = {}): MessageItem {
  return {
    id,
    chatJid: 'chat@s.whatsapp.net',
    senderJid: 'sender@s.whatsapp.net',
    senderName: 'Sender',
    content: 'same content',
    timestamp,
    order,
    isFromMe: false,
    isRead: false,
    ...overrides,
  };
}

function chat(jid: string, isArchived: boolean, unreadCount = 0): ChatItem {
  return {
    jid,
    name: jid,
    lastMessage: '',
    lastMessageTime: 1,
    unreadCount,
    isGroup: false,
    isArchived,
    lastMessageFromMe: false,
    lastMessageStatus: '',
    isPinned: false,
    isMuted: false,
    mutedUntil: 0,
  };
}

describe('canonical message merge', () => {
  beforeEach(() => useChatStore.getState().reset());

  it('orders equal-second messages by server order and then ID', () => {
    const messages = [message('c', 100, 20), message('a', 100, 10), message('b', 100, 20)];
    expect(messages.sort(compareMessages).map((item) => item.id)).toEqual(['a', 'b', 'c']);
  });

  it('keeps distinct IDs even when timestamp and content are identical', () => {
    const merged = mergeMessageLists([message('a', 100, 10)], [message('b', 100, 20)]);
    expect(merged.map((item) => item.id)).toEqual(['a', 'b']);
  });

  it('reconciles an optimistic message by client request ID', () => {
    const optimistic = message('local:req-1', 100, 0, {
      clientRequestId: 'req-1',
      isFromMe: true,
      deliveryStatus: 'pending',
      localState: 'pending',
    });
    const confirmed = message('server-id', 100, 30, {
      clientRequestId: 'req-1',
      isFromMe: true,
      deliveryStatus: 'sent',
    });
    const merged = mergeMessageLists([optimistic], [confirmed]);
    expect(merged).toHaveLength(1);
    expect(merged[0]).toMatchObject({ id: 'server-id', clientRequestId: 'req-1', deliveryStatus: 'sent' });
  });

  it('ignores a late event from a superseded history request', () => {
    const currentPage: MessagePage = {
      messages: [message('current', 200, 1)],
      hasMoreLocal: false,
      historyState: 'in_progress',
      canRequestOlder: false,
      requestId: 'request-current',
    };
    useChatStore.getState().setInitialMessages('chat@s.whatsapp.net', currentPage);

    const staleEvent: HistoryPageEvent = {
      chatJid: 'chat@s.whatsapp.net',
      requestId: 'request-old',
      messages: [message('stale', 100, 1)],
      historyState: 'more_on_primary',
      canRequestOlder: true,
    };
    useChatStore.getState().applyHistoryPage(staleEvent);

    expect(useChatStore.getState().messages['chat@s.whatsapp.net'].map((item) => item.id)).toEqual(['current']);
    expect(useChatStore.getState().pagination['chat@s.whatsapp.net'].requestId).toBe('request-current');
  });

  it('starts every reopen with a fresh request while retaining cached bubbles', () => {
    useChatStore.getState().setMessages('chat@s.whatsapp.net', [message('cached', 100, 1)]);
    useChatStore.getState().beginChatOpen('chat@s.whatsapp.net', 'reopen-1');

    expect(useChatStore.getState().messages['chat@s.whatsapp.net'][0].id).toBe('cached');
    expect(useChatStore.getState().pagination['chat@s.whatsapp.net']).toMatchObject({
      requestId: 'reopen-1',
      loadingMore: true,
      anchorMessageId: undefined,
    });
  });

  it('applies the durable unread count returned by the backend', () => {
    const jid = 'chat@s.whatsapp.net';
    useChatStore.getState().setChats([chat(jid, false, 1)]);
    useChatStore.getState().setMessages(jid, [message('only-unread', 100, 1)]);
    useChatStore.getState().applyReadResult(jid, { messageIds: ['only-unread'], unreadCount: 0 });

    expect(useChatStore.getState().messages[jid][0].isRead).toBe(true);
    expect(useChatStore.getState().chats[0].unreadCount).toBe(0);
  });

  it('keeps archived chats in a separate rail and follows remote archive changes', () => {
    const active = chat('active@s.whatsapp.net', false);
    const archived = chat('archived@s.whatsapp.net', true);
    useChatStore.getState().setChats([active, archived]);
    useChatStore.getState().setActiveChat(active.jid);

    expect(useChatStore.getState().filteredChats().map((item) => item.jid)).toEqual([active.jid]);
    useChatStore.getState().updateChat({ ...active, isArchived: true });
    expect(useChatStore.getState().activeRailView).toBe('archived');
    expect(useChatStore.getState().filteredChats().map((item) => item.jid)).toEqual([
      active.jid,
      archived.jid,
    ]);
  });
});
