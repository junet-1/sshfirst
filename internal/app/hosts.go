package app

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"ssh-first/internal/config"
	"ssh-first/internal/secrets"
	"ssh-first/internal/storage"
)

// secretsCleanupForHost removes the stored login password (if any) when a
// host is deleted. Identity-file passphrases are keyed by file path, not
// host, since the same key may be reused by other hosts — they are left
// alone.
func secretsCleanupForHost(hostID int64) error {
	return errors.Join(
		secrets.DeleteHostPassword(hostID),
		secrets.DeleteWebPassword(hostID),
	)
}

// ListHosts returns every known host (manual + imported), for the sidebar list.
func (a *App) ListHosts() ([]storage.Host, error) {
	if err := a.requireStore(); err != nil {
		return nil, err
	}
	return a.store.ListHosts()
}

// GetHost returns one host by ID, for the inspector panel.
func (a *App) GetHost(id int64) (storage.Host, error) {
	if err := a.requireStore(); err != nil {
		return storage.Host{}, err
	}
	return a.store.GetHost(id)
}

// CreateHost adds a manually-entered host.
func (a *App) CreateHost(input storage.HostInput) (storage.Host, error) {
	if err := a.requireStore(); err != nil {
		return storage.Host{}, err
	}
	if err := validateHostInput(input); err != nil {
		return storage.Host{}, err
	}
	input = normalizeHostInput(input)
	host, err := a.store.CreateHost(input, storage.HostSourceManual)
	if err == nil {
		a.refreshTray()
		a.emit("hosts:changed", nil)
	}
	return host, err
}

// UpdateHost overwrites an existing host's fields.
func (a *App) UpdateHost(id int64, input storage.HostInput) (storage.Host, error) {
	if err := a.requireStore(); err != nil {
		return storage.Host{}, err
	}
	if err := validateHostInput(input); err != nil {
		return storage.Host{}, err
	}
	input = normalizeHostInput(input)
	host, err := a.store.UpdateHost(id, input)
	if err == nil {
		a.refreshTray()
		a.emit("hosts:changed", nil)
	}
	return host, err
}

// validateHostInput rejects incomplete host definitions. A web host is
// addressed by its ControlPanelURL rather than an SSH hostname, so the required
// fields differ by protocol.
func validateHostInput(input storage.HostInput) error {
	if input.Label == "" {
		return fmt.Errorf("label is required")
	}
	switch input.Protocol {
	case "", storage.HostProtocolSSH, storage.HostProtocolSFTP:
		if input.Hostname == "" {
			return fmt.Errorf("hostname is required")
		}
		// Both fields end up inside rsync's remote-shell string, which rsync
		// splits on whitespace. The transfer layer quotes them, so this is the
		// second line of defence rather than the only one — but a hostname or
		// jump spec with whitespace in it is malformed anyway, and refusing it
		// here keeps the value from ever reaching the database.
		if strings.ContainsAny(input.Hostname, " \t\n") {
			return fmt.Errorf("hostname must not contain spaces")
		}
		if strings.ContainsAny(input.ProxyJump, " \t\n") {
			return fmt.Errorf("proxy jump must not contain spaces")
		}
	case storage.HostProtocolWeb:
		if strings.TrimSpace(input.ControlPanelURL) == "" {
			return fmt.Errorf("a control panel URL is required for a web host")
		}
		if err := validatePanelURL(input.ControlPanelURL); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported host protocol %q", input.Protocol)
	}
	return nil
}

// validatePanelURL keeps anything but a plain http(s) address out of a stored
// web host. The URL is rendered as an <iframe src> in the app shell, and a
// javascript: URL there executes in the shell's own origin — the one holding
// the Wails bindings that read stored passwords and write to live SSH sessions.
// Embedded credentials are refused as well: they make the origin the password
// is posted to differ from the host the address appears to name.
//
// The frontend applies the same rule before saving (see lib/panelUrl.ts); this
// is the authoritative copy, so no other write path can bypass it.
func validatePanelURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("control panel URL is not a valid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("control panel URL must use http or https")
	}
	if parsed.Host == "" {
		return fmt.Errorf("control panel URL needs a hostname")
	}
	if parsed.User != nil {
		return fmt.Errorf("control panel URL must not contain a username or password")
	}
	return nil
}

func normalizeHostInput(input storage.HostInput) storage.HostInput {
	input.ControlPanelURL = strings.TrimSpace(input.ControlPanelURL)
	if input.Protocol == "" {
		input.Protocol = storage.HostProtocolSSH
	}
	if input.RemotePath == "" {
		input.RemotePath = "."
	}
	switch input.Protocol {
	case storage.HostProtocolSFTP:
		input.ForwardAgent = false
		input.LoginScript = ""
	case storage.HostProtocolWeb:
		// A web host has no SSH transport. User is intentionally retained as the
		// email/login name used by browser autofill; the remaining SSH/SFTP-only
		// fields are stripped so a protocol switch cannot leave stale behavior.
		input.User = strings.TrimSpace(input.User)
		input.ForwardAgent = false
		input.LoginScript = ""
		input.RemotePath = "."
		input.IdentityFiles = []string{}
		input.ProxyJump = ""
		input.AuthMethod = storage.AuthMethodPassword
		input.CredentialID = nil
	}
	return input
}

// DeleteHost removes a host. The frontend is responsible for confirming
// destructive intent (Delete key / context menu) before calling this.
func (a *App) DeleteHost(id int64) error {
	if err := a.requireStore(); err != nil {
		return err
	}
	if err := secretsCleanupForHost(id); err != nil {
		return err
	}
	if err := a.store.DeleteHost(id); err != nil {
		return err
	}
	a.refreshTray()
	a.emit("hosts:changed", nil)
	return nil
}

// SetFavorite toggles a host's favorite flag.
func (a *App) SetFavorite(id int64, favorite bool) error {
	if err := a.requireStore(); err != nil {
		return err
	}
	if err := a.store.SetFavorite(id, favorite); err != nil {
		return err
	}
	a.refreshTray()
	a.emit("hosts:changed", nil)
	return nil
}

// ListFolders returns the folder tree (flat; the frontend nests by ParentID).
func (a *App) ListFolders() ([]storage.Folder, error) {
	if err := a.requireStore(); err != nil {
		return nil, err
	}
	return a.store.ListFolders()
}

// CreateFolder adds a new sidebar folder.
func (a *App) CreateFolder(name string, parentID *int64, icon string) (storage.Folder, error) {
	if err := a.requireStore(); err != nil {
		return storage.Folder{}, err
	}
	if name == "" {
		return storage.Folder{}, fmt.Errorf("folder name is required")
	}
	folder, err := a.store.CreateFolderWithIcon(name, parentID, normalizeFolderIcon(icon))
	if err == nil {
		a.emit("folders:changed", nil)
	}
	return folder, err
}

// UpdateFolder changes a folder's display name and symbolic sidebar icon.
func (a *App) UpdateFolder(id int64, name, icon string) (storage.Folder, error) {
	if err := a.requireStore(); err != nil {
		return storage.Folder{}, err
	}
	if name == "" {
		return storage.Folder{}, fmt.Errorf("folder name is required")
	}
	folder, err := a.store.UpdateFolder(id, name, normalizeFolderIcon(icon))
	if err == nil {
		a.emit("folders:changed", nil)
	}
	return folder, err
}

func normalizeFolderIcon(icon string) string {
	switch icon {
	case "folder", "server", "terminal", "cloud", "database", "globe", "code", "home", "shield", "archive":
		return icon
	default:
		return "folder"
	}
}

// RenameFolder renames a sidebar folder.
func (a *App) RenameFolder(id int64, name string) error {
	if err := a.requireStore(); err != nil {
		return err
	}
	if err := a.store.RenameFolder(id, name); err != nil {
		return err
	}
	a.emit("folders:changed", nil)
	return nil
}

// DeleteFolder removes a folder; its hosts become un-foldered, not deleted.
func (a *App) DeleteFolder(id int64) error {
	if err := a.requireStore(); err != nil {
		return err
	}
	if err := a.store.DeleteFolder(id); err != nil {
		return err
	}
	a.emit("folders:changed", nil)
	a.emit("hosts:changed", nil)
	return nil
}

// MoveFolder re-parents a folder via drag-and-drop in the sidebar (nil parent
// = top level). Rejects cycles.
func (a *App) MoveFolder(id int64, parentID *int64) error {
	if err := a.requireStore(); err != nil {
		return err
	}
	if err := a.store.MoveFolder(id, parentID); err != nil {
		return err
	}
	a.emit("folders:changed", nil)
	return nil
}

// MoveHostToFolder re-parents a host via drag-and-drop in the sidebar.
func (a *App) MoveHostToFolder(hostID int64, folderID *int64) error {
	if err := a.requireStore(); err != nil {
		return err
	}
	if err := a.store.MoveHostToFolder(hostID, folderID); err != nil {
		return err
	}
	a.emit("hosts:changed", nil)
	return nil
}

// ReorderHost persists a host's exact position among the destination folder's
// children. A nil targetHostID appends; otherwise before selects which side of
// the target row receives the moved host.
func (a *App) ReorderHost(hostID int64, folderID, targetHostID *int64, before bool) error {
	if err := a.requireStore(); err != nil {
		return err
	}
	if err := a.store.ReorderHost(hostID, folderID, targetHostID, before); err != nil {
		return err
	}
	a.refreshTray()
	a.emit("hosts:changed", nil)
	return nil
}

// ReorderFolder persists a folder's exact position among siblings while also
// supporting re-parenting. A nil targetFolderID appends under parentID.
func (a *App) ReorderFolder(folderID int64, parentID, targetFolderID *int64, before bool) error {
	if err := a.requireStore(); err != nil {
		return err
	}
	if err := a.store.ReorderFolder(folderID, parentID, targetFolderID, before); err != nil {
		return err
	}
	a.emit("folders:changed", nil)
	return nil
}

// ImportSSHConfig reads ~/.ssh/config (following Include directives) and
// upserts each concrete Host alias into the local host list, matched by
// label so re-running the import updates rather than duplicates entries.
func (a *App) ImportSSHConfig() (ImportResult, error) {
	if err := a.requireStore(); err != nil {
		return ImportResult{}, err
	}
	path, err := config.DefaultPath()
	if err != nil {
		return ImportResult{}, fmt.Errorf("locate ~/.ssh/config: %w", err)
	}
	imported, err := config.Import(path)
	if errors.Is(err, os.ErrNotExist) {
		return ImportResult{}, fmt.Errorf("no SSH config found at %s — nothing to import", path)
	}
	if err != nil {
		return ImportResult{}, fmt.Errorf("read %s: %w", path, err)
	}

	hosts := make([]storage.Host, 0, len(imported))
	for _, ih := range imported {
		authMethod := storage.AuthMethodAgent
		if len(ih.IdentityFiles) > 0 {
			authMethod = storage.AuthMethodIdentity
		}
		host, err := a.store.UpsertImportedHost(storage.HostInput{
			Label:         ih.Alias,
			Hostname:      ih.Hostname,
			Port:          ih.Port,
			User:          ih.User,
			IdentityFiles: ih.IdentityFiles,
			ProxyJump:     ih.ProxyJump,
			ForwardAgent:  ih.ForwardAgent,
			AuthMethod:    authMethod,
			Protocol:      storage.HostProtocolSSH,
			RemotePath:    ".",
		})
		if err != nil {
			continue
		}
		hosts = append(hosts, host)
	}

	a.refreshTray()
	a.emit("hosts:changed", nil)
	return ImportResult{ImportedCount: len(hosts), Hosts: hosts}, nil
}

// GetSetting reads a UI setting (theme, locale, ...). Exists is false if the
// key was never set, letting the frontend fall back to a default.
func (a *App) GetSetting(key string) (SettingResult, error) {
	if err := a.requireStore(); err != nil {
		return SettingResult{}, err
	}
	value, exists, err := a.store.GetSetting(key)
	if err != nil {
		return SettingResult{}, err
	}
	return SettingResult{Value: value, Exists: exists}, nil
}

// SetSetting persists a UI setting.
func (a *App) SetSetting(key, value string) error {
	if err := a.requireStore(); err != nil {
		return err
	}
	if err := a.store.SetSetting(key, value); err != nil {
		return err
	}
	a.emit("settings:changed", map[string]string{"key": key, "value": value})
	return nil
}
