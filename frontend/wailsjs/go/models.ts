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
	
	export class ChatItem {
	    jid: string;
	    name: string;
	    lastMessage: string;
	    lastMessageTime: number;
	    unreadCount: number;
	    isGroup: boolean;
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
	        this.avatar = source["avatar"];
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
	    isPtt?: boolean;
	    deliveryStatus?: string;
	
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
	        this.isPtt = source["isPtt"];
	        this.deliveryStatus = source["deliveryStatus"];
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

