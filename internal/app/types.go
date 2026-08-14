package app

import (
	sftppkg "ssh-first/internal/sftp"
	"ssh-first/internal/storage"
)

// ConnectionStatus mirrors the lifecycle shown in the status bar and tab header.
type ConnectionStatus string

const (
	StatusConnecting   ConnectionStatus = "connecting"
	StatusConnected    ConnectionStatus = "connected"
	StatusDisconnected ConnectionStatus = "disconnected"
	StatusError        ConnectionStatus = "error"
)

// TabKind tells the frontend which native workspace belongs in a tab.
type TabKind string

const (
	TabKindTerminal TabKind = "terminal"
	TabKindSFTP     TabKind = "sftp"
)

// ConnectionInfo is returned to the frontend after Connect/Reconnect and
// describes the connection plus its initial terminal tab.
type ConnectionInfo struct {
	ConnectionID   string               `json:"connectionId"`
	HostID         int64                `json:"hostId"`
	HostLabel      string               `json:"hostLabel"`
	Status         ConnectionStatus     `json:"status"`
	ServerVersion  string               `json:"serverVersion,omitempty"`
	AuthMethod     string               `json:"authMethod,omitempty"`
	Protocol       storage.HostProtocol `json:"protocol"`
	InitialTabID   string               `json:"initialTabId"`
	InitialTabKind TabKind              `json:"initialTabKind"`
	Error          string               `json:"error,omitempty"`
	// Warning carries a non-fatal notice about an otherwise successful
	// connection (e.g. agent forwarding was requested but unavailable).
	Warning string `json:"warning,omitempty"`
}

// TabInfo describes one terminal tab.
type TabInfo struct {
	TabID        string  `json:"tabId"`
	ConnectionID string  `json:"connectionId"`
	Title        string  `json:"title"`
	Kind         TabKind `json:"kind"`
}

// SFTPDirectory is one directory snapshot plus its canonical remote path.
type SFTPDirectory struct {
	Path    string          `json:"path"`
	Entries []sftppkg.Entry `json:"entries"`
}

// StatusEvent is emitted on the "connection:status" event as a connection's
// state, latency or error changes.
type StatusEvent struct {
	ConnectionID string           `json:"connectionId"`
	HostID       int64            `json:"hostId"`
	Status       ConnectionStatus `json:"status"`
	LatencyMs    int64            `json:"latencyMs,omitempty"`
	Error        string           `json:"error,omitempty"`
}

// TerminalDataEvent is one bounded output batch. The frontend acknowledges
// Sequence only after xterm has parsed Data, which keeps backend buffering and
// Wails event dispatch bounded when a native webview is throttled or hidden.
type TerminalDataEvent struct {
	TabID             string `json:"tabId"`
	SessionGeneration string `json:"sessionGeneration"`
	Sequence          uint64 `json:"sequence"`
	Data              string `json:"data"`
}

// TerminalClosedEvent is emitted on "terminal:closed" once a shell exits.
type TerminalClosedEvent struct {
	TabID string `json:"tabId"`
	Error string `json:"error,omitempty"`
}

// HostKeyPromptEvent is emitted on "hostkey:request"; the frontend must
// eventually call RespondHostKey with the same RequestID.
type HostKeyPromptEvent struct {
	RequestID            string   `json:"requestId"`
	Hostname             string   `json:"hostname"`
	Algorithm            string   `json:"algorithm"`
	Fingerprint          string   `json:"fingerprint"`
	Status               string   `json:"status"` // "unknown" | "changed"
	PreviousFingerprints []string `json:"previousFingerprints,omitempty"`
}

// PasswordPromptEvent is emitted on "password:request"; the frontend must
// eventually call RespondPassword with the same RequestID.
type PasswordPromptEvent struct {
	RequestID     string `json:"requestId"`
	User          string `json:"user"`
	Hostname      string `json:"hostname"`
	AllowRemember bool   `json:"allowRemember"`
}

// PassphrasePromptEvent is emitted on "passphrase:request"; the frontend
// must eventually call RespondPassphrase with the same RequestID.
type PassphrasePromptEvent struct {
	RequestID    string `json:"requestId"`
	IdentityFile string `json:"identityFile"`
}

// KeyboardInteractiveQuestion is one server-defined authentication prompt.
type KeyboardInteractiveQuestion struct {
	Prompt string `json:"prompt"`
	Echo   bool   `json:"echo"`
}

// KeyboardInteractivePromptEvent supports multi-step challenges such as a
// password followed by a TOTP code. Answers are never persisted.
type KeyboardInteractivePromptEvent struct {
	RequestID   string                        `json:"requestId"`
	User        string                        `json:"user"`
	Hostname    string                        `json:"hostname"`
	Instruction string                        `json:"instruction,omitempty"`
	Questions   []KeyboardInteractiveQuestion `json:"questions"`
}

// SettingResult is a single key's value from the settings table.
type SettingResult struct {
	Value  string `json:"value"`
	Exists bool   `json:"exists"`
}

// ImportResult summarizes an ~/.ssh/config import run.
type ImportResult struct {
	ImportedCount int            `json:"importedCount"`
	Hosts         []storage.Host `json:"hosts"`
}
