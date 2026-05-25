package whatsapp

// ConnectionState represents the current connection status
type ConnectionState string

const (
	StateDisconnected ConnectionState = "disconnected"
	StateConnecting   ConnectionState = "connecting"
	StateQRReady      ConnectionState = "qr_ready"
	StateConnected    ConnectionState = "connected"
	StateLoggedOut    ConnectionState = "logged_out"
)

// QRCodeEvent is emitted to the frontend when a QR code is available
type QRCodeEvent struct {
	Code  string `json:"code"`
	Event string `json:"event"` // "code", "success", "timeout", "error"
}

// ConnectionStatusEvent is emitted to the frontend on connection changes
type ConnectionStatusEvent struct {
	State   ConnectionState `json:"state"`
	Message string          `json:"message,omitempty"`
}

// UserInfo represents the logged-in user's profile
type UserInfo struct {
	JID         string `json:"jid"`
	PushName    string `json:"pushName"`
	PhoneNumber string `json:"phoneNumber"`
	Platform    string `json:"platform"`
}

// AccountInfo holds multi-account session data
type AccountInfo struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	UserInfo UserInfo `json:"userInfo"`
	DBPath   string   `json:"dbPath"`
	IsActive bool     `json:"isActive"`
}
