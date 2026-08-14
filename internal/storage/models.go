// Package storage persists host metadata, folders, tags and settings in a
// local SQLite database. It never stores secrets (passwords, passphrases,
// private key material) — see internal/secrets for that.
package storage

// AuthMethod is the preferred authentication strategy for a Host.
type AuthMethod string

const (
	AuthMethodAgent    AuthMethod = "agent"
	AuthMethodIdentity AuthMethod = "identity"
	AuthMethodPassword AuthMethod = "password"
)

// HostProtocol controls which native workspace is opened after the SSH
// transport has authenticated.
type HostProtocol string

const (
	HostProtocolSSH  HostProtocol = "ssh"
	HostProtocolSFTP HostProtocol = "sftp"
	// HostProtocolWeb is a host whose "connection" is a web control panel opened
	// as an embedded browser tab (ControlPanelURL), not an SSH transport.
	HostProtocolWeb HostProtocol = "web"
)

// HostSource records where a Host definition came from.
type HostSource string

const (
	HostSourceManual    HostSource = "manual"
	HostSourceSSHConfig HostSource = "ssh_config"
)

// Host is a connection target as shown in the sidebar host list.
type Host struct {
	ID              int64        `json:"id"`
	Label           string       `json:"label"`
	Hostname        string       `json:"hostname"`
	Port            int          `json:"port"`
	User            string       `json:"user"`
	IdentityFiles   []string     `json:"identityFiles"`
	ProxyJump       string       `json:"proxyJump"`
	ForwardAgent    bool         `json:"forwardAgent"`
	AuthMethod      AuthMethod   `json:"authMethod"`
	Protocol        HostProtocol `json:"protocol"`
	RemotePath      string       `json:"remotePath"`
	FolderID        *int64       `json:"folderId,omitempty"`
	CredentialID    *int64       `json:"credentialId,omitempty"`
	Favorite        bool         `json:"favorite"`
	Source          HostSource   `json:"source"`
	Notes           string       `json:"notes"`
	LoginScript     string       `json:"loginScript"`
	ControlPanelURL string       `json:"controlPanelUrl"`
	Tags            []string     `json:"tags"`
	SortOrder       int          `json:"sortOrder"`
	LastUsedAt      *string      `json:"lastUsedAt,omitempty"`
	CreatedAt       string       `json:"createdAt"`
	UpdatedAt       string       `json:"updatedAt"`
}

// Folder groups hosts in the sidebar tree.
type Folder struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Icon      string `json:"icon"`
	ParentID  *int64 `json:"parentId,omitempty"`
	SortOrder int    `json:"sortOrder"`
}

// HostInput is the payload for creating/updating a Host from the UI.
type HostInput struct {
	Label         string       `json:"label"`
	Hostname      string       `json:"hostname"`
	Port          int          `json:"port"`
	User          string       `json:"user"`
	IdentityFiles []string     `json:"identityFiles"`
	ProxyJump     string       `json:"proxyJump"`
	ForwardAgent  bool         `json:"forwardAgent"`
	AuthMethod    AuthMethod   `json:"authMethod"`
	Protocol      HostProtocol `json:"protocol"`
	RemotePath    string       `json:"remotePath"`
	FolderID      *int64       `json:"folderId,omitempty"`
	// CredentialID, when set, makes the host inherit user/auth/identity files
	// from a shared credential; the inline fields above are then ignored.
	CredentialID    *int64   `json:"credentialId,omitempty"`
	Notes           string   `json:"notes"`
	LoginScript     string   `json:"loginScript"`
	ControlPanelURL string   `json:"controlPanelUrl"`
	Tags            []string `json:"tags"`
}

// Credential is a reusable login identity (user + auth method + identity files)
// referenced by hosts. Its password, if any, lives in the Secret Service keyed
// by credential ID — never in this database.
type Credential struct {
	ID            int64      `json:"id"`
	Name          string     `json:"name"`
	User          string     `json:"user"`
	AuthMethod    AuthMethod `json:"authMethod"`
	IdentityFiles []string   `json:"identityFiles"`
	CreatedAt     string     `json:"createdAt"`
	UpdatedAt     string     `json:"updatedAt"`
}

// CredentialInput is the payload for creating/updating a Credential from the UI.
type CredentialInput struct {
	Name          string     `json:"name"`
	User          string     `json:"user"`
	AuthMethod    AuthMethod `json:"authMethod"`
	IdentityFiles []string   `json:"identityFiles"`
}
