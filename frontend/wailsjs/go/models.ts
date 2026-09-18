export namespace keys {
	
	export class KeyPair {
	    Pub?: number[];
	    Priv?: number[];
	
	    static createFrom(source: any = {}) {
	        return new KeyPair(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Pub = source["Pub"];
	        this.Priv = source["Priv"];
	    }
	}
	export class PreKey {
	    Pub?: number[];
	    Priv?: number[];
	    KeyID: number;
	    Signature?: number[];
	
	    static createFrom(source: any = {}) {
	        return new PreKey(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Pub = source["Pub"];
	        this.Priv = source["Priv"];
	        this.KeyID = source["KeyID"];
	        this.Signature = source["Signature"];
	    }
	}

}

export namespace store {
	
	export class Device {
	    Log: any;
	    NoiseKey?: keys.KeyPair;
	    IdentityKey?: keys.KeyPair;
	    SignedPreKey?: keys.PreKey;
	    RegistrationID: number;
	    AdvSecretKey: number[];
	    ID?: types.JID;
	    LID: types.JID;
	    Account?: waAdv.ADVSignedDeviceIdentity;
	    Platform: string;
	    BusinessName: string;
	    PushName: string;
	    LIDMigrationTimestamp: number;
	    CompanionMetaNonce: string;
	    FacebookUUID: number[];
	    Initialized: boolean;
	    Deleted: boolean;
	    Identities: any;
	    Sessions: any;
	    PreKeys: any;
	    SenderKeys: any;
	    AppStateKeys: any;
	    AppState: any;
	    Contacts: any;
	    ChatSettings: any;
	    MsgSecrets: any;
	    PrivacyTokens: any;
	    NCTSalt: any;
	    EventBuffer: any;
	    LIDs: any;
	    Container: any;
	
	    static createFrom(source: any = {}) {
	        return new Device(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Log = source["Log"];
	        this.NoiseKey = this.convertValues(source["NoiseKey"], keys.KeyPair);
	        this.IdentityKey = this.convertValues(source["IdentityKey"], keys.KeyPair);
	        this.SignedPreKey = this.convertValues(source["SignedPreKey"], keys.PreKey);
	        this.RegistrationID = source["RegistrationID"];
	        this.AdvSecretKey = source["AdvSecretKey"];
	        this.ID = this.convertValues(source["ID"], types.JID);
	        this.LID = this.convertValues(source["LID"], types.JID);
	        this.Account = this.convertValues(source["Account"], waAdv.ADVSignedDeviceIdentity);
	        this.Platform = source["Platform"];
	        this.BusinessName = source["BusinessName"];
	        this.PushName = source["PushName"];
	        this.LIDMigrationTimestamp = source["LIDMigrationTimestamp"];
	        this.CompanionMetaNonce = source["CompanionMetaNonce"];
	        this.FacebookUUID = source["FacebookUUID"];
	        this.Initialized = source["Initialized"];
	        this.Deleted = source["Deleted"];
	        this.Identities = source["Identities"];
	        this.Sessions = source["Sessions"];
	        this.PreKeys = source["PreKeys"];
	        this.SenderKeys = source["SenderKeys"];
	        this.AppStateKeys = source["AppStateKeys"];
	        this.AppState = source["AppState"];
	        this.Contacts = source["Contacts"];
	        this.ChatSettings = source["ChatSettings"];
	        this.MsgSecrets = source["MsgSecrets"];
	        this.PrivacyTokens = source["PrivacyTokens"];
	        this.NCTSalt = source["NCTSalt"];
	        this.EventBuffer = source["EventBuffer"];
	        this.LIDs = source["LIDs"];
	        this.Container = source["Container"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace types {
	
	export class JID {
	    User: string;
	    RawAgent: number;
	    Device: number;
	    Integrator: number;
	    Server: string;
	
	    static createFrom(source: any = {}) {
	        return new JID(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.User = source["User"];
	        this.RawAgent = source["RawAgent"];
	        this.Device = source["Device"];
	        this.Integrator = source["Integrator"];
	        this.Server = source["Server"];
	    }
	}

}

export namespace waAdv {
	
	export class ADVSignedDeviceIdentity {
	    details?: number[];
	    accountSignatureKey?: number[];
	    accountSignature?: number[];
	    deviceSignature?: number[];
	
	    static createFrom(source: any = {}) {
	        return new ADVSignedDeviceIdentity(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.details = source["details"];
	        this.accountSignatureKey = source["accountSignatureKey"];
	        this.accountSignature = source["accountSignature"];
	        this.deviceSignature = source["deviceSignature"];
	    }
	}

}

export namespace whatsapp {
	
	export class UserInfo {
	    jid: string;
	    pushName: string;
	    phoneNumber: string;
	    platform: string;
	
	    static createFrom(source: any = {}) {
	        return new UserInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.jid = source["jid"];
	        this.pushName = source["pushName"];
	        this.phoneNumber = source["phoneNumber"];
	        this.platform = source["platform"];
	    }
	}
	export class AccountInfo {
	    id: string;
	    label: string;
	    userInfo: UserInfo;
	    isActive: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AccountInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.userInfo = this.convertValues(source["userInfo"], UserInfo);
	        this.isActive = source["isActive"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AttachmentDraft {
	    id: string;
	    kind: string;
	    fileName: string;
	    mimetype: string;
	    fileSize: number;
	    isPtt?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AttachmentDraft(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.fileName = source["fileName"];
	        this.mimetype = source["mimetype"];
	        this.fileSize = source["fileSize"];
	        this.isPtt = source["isPtt"];
	    }
	}
	export class CallInfo {
	    callId: string;
	    outcome: string;
	    duration?: number;
	    isVideo: boolean;
	    isIncoming: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CallInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.callId = source["callId"];
	        this.outcome = source["outcome"];
	        this.duration = source["duration"];
	        this.isVideo = source["isVideo"];
	        this.isIncoming = source["isIncoming"];
	    }
	}
	export class CallLogEntry {
	    chatJid: string;
	    chatName: string;
	    timestamp: number;
	    outcome: string;
	    duration?: number;
	    isVideo: boolean;
	    isIncoming: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CallLogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.chatJid = source["chatJid"];
	        this.chatName = source["chatName"];
	        this.timestamp = source["timestamp"];
	        this.outcome = source["outcome"];
	        this.duration = source["duration"];
	        this.isVideo = source["isVideo"];
	        this.isIncoming = source["isIncoming"];
	    }
	}
	export class ChatItem {
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
	
	    static createFrom(source: any = {}) {
	        return new ChatItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.jid = source["jid"];
	        this.name = source["name"];
	        this.lastMessage = source["lastMessage"];
	        this.lastMessageTime = source["lastMessageTime"];
	        this.unreadCount = source["unreadCount"];
	        this.isGroup = source["isGroup"];
	        this.lastMessageFromMe = source["lastMessageFromMe"];
	        this.lastMessageStatus = source["lastMessageStatus"];
	        this.isArchived = source["isArchived"];
	        this.isPinned = source["isPinned"];
	        this.isMuted = source["isMuted"];
	        this.mutedUntil = source["mutedUntil"];
	        this.listIds = source["listIds"];
	        this.avatar = source["avatar"];
	    }
	}
	export class ChatList {
	    id: string;
	    name: string;
	    type: string;
	    order: number;
	    isActive: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ChatList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.order = source["order"];
	        this.isActive = source["isActive"];
	    }
	}
	export class ChatProfile {
	    jid: string;
	    name: string;
	    avatar?: string;
	    isGroup: boolean;
	    phoneNumber?: string;
	    description?: string;
	    participantCount?: number;
	
	    static createFrom(source: any = {}) {
	        return new ChatProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.jid = source["jid"];
	        this.name = source["name"];
	        this.avatar = source["avatar"];
	        this.isGroup = source["isGroup"];
	        this.phoneNumber = source["phoneNumber"];
	        this.description = source["description"];
	        this.participantCount = source["participantCount"];
	    }
	}
	export class MessageReaction {
	    emoji: string;
	    count: number;
	    fromMe: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MessageReaction(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.emoji = source["emoji"];
	        this.count = source["count"];
	        this.fromMe = source["fromMe"];
	    }
	}
	export class MessageReference {
	    id: string;
	    chatJid: string;
	    senderJid: string;
	    senderName: string;
	    content: string;
	    mediaType?: string;
	    isFromMe: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MessageReference(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.chatJid = source["chatJid"];
	        this.senderJid = source["senderJid"];
	        this.senderName = source["senderName"];
	        this.content = source["content"];
	        this.mediaType = source["mediaType"];
	        this.isFromMe = source["isFromMe"];
	    }
	}
	export class MessageItem {
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
	    clientRequestId?: string;
	    replyTo?: MessageReference;
	    isForwarded?: boolean;
	    isDeleted?: boolean;
	    caption?: string;
	    fileSize?: number;
	    kind?: string;
	    call?: CallInfo;
	    reactions?: MessageReaction[];
	
	    static createFrom(source: any = {}) {
	        return new MessageItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.chatJid = source["chatJid"];
	        this.senderJid = source["senderJid"];
	        this.senderName = source["senderName"];
	        this.content = source["content"];
	        this.timestamp = source["timestamp"];
	        this.isFromMe = source["isFromMe"];
	        this.isRead = source["isRead"];
	        this.mediaType = source["mediaType"];
	        this.mediaUrl = source["mediaUrl"];
	        this.mediaDuration = source["mediaDuration"];
	        this.fileName = source["fileName"];
	        this.mimetype = source["mimetype"];
	        this.thumbnail = source["thumbnail"];
	        this.isPtt = source["isPtt"];
	        this.deliveryStatus = source["deliveryStatus"];
	        this.clientRequestId = source["clientRequestId"];
	        this.replyTo = this.convertValues(source["replyTo"], MessageReference);
	        this.isForwarded = source["isForwarded"];
	        this.isDeleted = source["isDeleted"];
	        this.caption = source["caption"];
	        this.fileSize = source["fileSize"];
	        this.kind = source["kind"];
	        this.call = this.convertValues(source["call"], CallInfo);
	        this.reactions = this.convertValues(source["reactions"], MessageReaction);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ChatWithMessages {
	    chat: ChatItem;
	    messages: MessageItem[];
	
	    static createFrom(source: any = {}) {
	        return new ChatWithMessages(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.chat = this.convertValues(source["chat"], ChatItem);
	        this.messages = this.convertValues(source["messages"], MessageItem);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DocumentState {
	    downloaded: boolean;
	    fileName: string;
	    fileSize: number;
	
	    static createFrom(source: any = {}) {
	        return new DocumentState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.downloaded = source["downloaded"];
	        this.fileName = source["fileName"];
	        this.fileSize = source["fileSize"];
	    }
	}
	export class ForwardResult {
	    chatJid: string;
	    success: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ForwardResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.chatJid = source["chatJid"];
	        this.success = source["success"];
	        this.error = source["error"];
	    }
	}
	
	export class MessagePage {
	    messages: MessageItem[];
	    hasMoreLocal: boolean;
	    canRequestOlder: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MessagePage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.messages = this.convertValues(source["messages"], MessageItem);
	        this.hasMoreLocal = source["hasMoreLocal"];
	        this.canRequestOlder = source["canRequestOlder"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class RestoreSessionResult {
	    attempted: boolean;
	    accountId?: string;
	
	    static createFrom(source: any = {}) {
	        return new RestoreSessionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.attempted = source["attempted"];
	        this.accountId = source["accountId"];
	    }
	}
	export class StickerItem {
	    id: string;
	    mimetype: string;
	    width: number;
	    height: number;
	    lastUsedAt: number;
	    available: boolean;
	
	    static createFrom(source: any = {}) {
	        return new StickerItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.mimetype = source["mimetype"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.lastUsedAt = source["lastUsedAt"];
	        this.available = source["available"];
	    }
	}

}

export namespace whatsmeow {
	
	export class MessengerConfig {
	    UserAgent: string;
	    BaseURL: string;
	    WebsocketURL: string;
	
	    static createFrom(source: any = {}) {
	        return new MessengerConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.UserAgent = source["UserAgent"];
	        this.BaseURL = source["BaseURL"];
	        this.WebsocketURL = source["WebsocketURL"];
	    }
	}
	export class Client {
	    Store?: store.Device;
	    Log: any;
	    EnableAutoReconnect: boolean;
	    InitialAutoReconnect: boolean;
	    // Go type: time
	    LastSuccessfulConnect: any;
	    AutoReconnectErrors: number;
	    SynchronousAck: boolean;
	    EnableDecryptedEventBuffer: boolean;
	    DisableLoginAutoReconnect: boolean;
	    EmitAppStateEventsOnFullSync: boolean;
	    AppStateDebugLogs: boolean;
	    AutomaticMessageRerequestFromPhone: boolean;
	    ManualHistorySyncDownload: boolean;
	    DisableManualHistorySyncReceipt: boolean;
	    UseRetryMessageStore: boolean;
	    QRClientType: string;
	    AutoTrustIdentity: boolean;
	    ErrorOnSubscribePresenceWithoutToken: boolean;
	    SendReportingTokens: boolean;
	    BackgroundEventCtx: any;
	    MessengerConfig?: MessengerConfig;
	    UserAgent: string;
	    WebSocketHeaders: Record<string, Array<string>>;
	
	    static createFrom(source: any = {}) {
	        return new Client(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Store = this.convertValues(source["Store"], store.Device);
	        this.Log = source["Log"];
	        this.EnableAutoReconnect = source["EnableAutoReconnect"];
	        this.InitialAutoReconnect = source["InitialAutoReconnect"];
	        this.LastSuccessfulConnect = this.convertValues(source["LastSuccessfulConnect"], null);
	        this.AutoReconnectErrors = source["AutoReconnectErrors"];
	        this.SynchronousAck = source["SynchronousAck"];
	        this.EnableDecryptedEventBuffer = source["EnableDecryptedEventBuffer"];
	        this.DisableLoginAutoReconnect = source["DisableLoginAutoReconnect"];
	        this.EmitAppStateEventsOnFullSync = source["EmitAppStateEventsOnFullSync"];
	        this.AppStateDebugLogs = source["AppStateDebugLogs"];
	        this.AutomaticMessageRerequestFromPhone = source["AutomaticMessageRerequestFromPhone"];
	        this.ManualHistorySyncDownload = source["ManualHistorySyncDownload"];
	        this.DisableManualHistorySyncReceipt = source["DisableManualHistorySyncReceipt"];
	        this.UseRetryMessageStore = source["UseRetryMessageStore"];
	        this.QRClientType = source["QRClientType"];
	        this.AutoTrustIdentity = source["AutoTrustIdentity"];
	        this.ErrorOnSubscribePresenceWithoutToken = source["ErrorOnSubscribePresenceWithoutToken"];
	        this.SendReportingTokens = source["SendReportingTokens"];
	        this.BackgroundEventCtx = source["BackgroundEventCtx"];
	        this.MessengerConfig = this.convertValues(source["MessengerConfig"], MessengerConfig);
	        this.UserAgent = source["UserAgent"];
	        this.WebSocketHeaders = source["WebSocketHeaders"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

