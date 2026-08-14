package app

import (
	"errors"
	"fmt"
	"log"

	"ssh-first/internal/secrets"
	"ssh-first/internal/storage"
)

// ListCredentials returns every reusable credential, for the manager window and
// the host editor's credential picker.
func (a *App) ListCredentials() ([]storage.Credential, error) {
	if err := a.requireStore(); err != nil {
		return nil, err
	}
	return a.store.ListCredentials()
}

// CreateCredential adds a new reusable credential.
func (a *App) CreateCredential(input storage.CredentialInput) (storage.Credential, error) {
	if err := a.requireStore(); err != nil {
		return storage.Credential{}, err
	}
	if input.Name == "" {
		return storage.Credential{}, fmt.Errorf("credential name is required")
	}
	cred, err := a.store.CreateCredential(input)
	if err == nil {
		a.emit("credentials:changed", nil)
	}
	return cred, err
}

// UpdateCredential overwrites an existing credential's fields.
func (a *App) UpdateCredential(id int64, input storage.CredentialInput) (storage.Credential, error) {
	if err := a.requireStore(); err != nil {
		return storage.Credential{}, err
	}
	if input.Name == "" {
		return storage.Credential{}, fmt.Errorf("credential name is required")
	}
	cred, err := a.store.UpdateCredential(id, input)
	if err == nil {
		a.emit("credentials:changed", nil)
	}
	return cred, err
}

// DeleteCredential removes a credential and its stored password. Hosts that
// referenced it revert to their own inline auth fields (ON DELETE SET NULL), so
// the host list also changes.
func (a *App) DeleteCredential(id int64) error {
	if err := a.requireStore(); err != nil {
		return err
	}
	if err := a.store.DeleteCredential(id); err != nil {
		return err
	}
	if err := secrets.DeleteCredentialPassword(id); err != nil {
		// A missing password secret is fine; only log anything unexpected.
		if err != secrets.ErrNotFound {
			log.Printf("ssh-first: delete credential password: %v", err)
		}
	}
	a.emit("credentials:changed", nil)
	a.emit("hosts:changed", nil)
	return nil
}

// HostUsername returns a host's effective login user (from its credential when
// referenced, otherwise its own field) for the "Copy Username" action.
func (a *App) HostUsername(hostID int64) (string, error) {
	if err := a.requireStore(); err != nil {
		return "", err
	}
	host, err := a.store.GetHost(hostID)
	if err != nil {
		return "", err
	}
	return a.resolveAuth(host).user, nil
}

// HostPassword returns a host's stored login password (resolving a referenced
// credential), or "" when none is stored, for the "Copy Password" action. The
// password lives only in the Secret Service; this is the sole path that hands it
// to the frontend, and only on explicit user request.
func (a *App) HostPassword(hostID int64) (string, error) {
	if err := a.requireStore(); err != nil {
		return "", err
	}
	host, err := a.store.GetHost(hostID)
	if err != nil {
		return "", err
	}
	pw, err := a.resolveAuth(host).getPassword()
	if errors.Is(err, secrets.ErrNotFound) {
		return "", nil
	}
	return pw, err
}

// WebPassword returns a web host's autofill password, or "" when none is
// stored. Web secrets use their own keyring namespace and can never be reused
// accidentally as an SSH password after a protocol change.
func (a *App) WebPassword(hostID int64) (string, error) {
	if err := a.requireStore(); err != nil {
		return "", err
	}
	host, err := a.store.GetHost(hostID)
	if err != nil {
		return "", err
	}
	if host.Protocol != storage.HostProtocolWeb {
		return "", fmt.Errorf("host %d is not a web host", hostID)
	}
	password, err := secrets.GetWebPassword(hostID)
	if errors.Is(err, secrets.ErrNotFound) {
		return "", nil
	}
	return password, err
}

// SetWebPassword stores or replaces a web host's autofill password in the
// system secret store. An empty password removes the saved secret.
func (a *App) SetWebPassword(hostID int64, password string) error {
	if err := a.requireStore(); err != nil {
		return err
	}
	host, err := a.store.GetHost(hostID)
	if err != nil {
		return err
	}
	if host.Protocol != storage.HostProtocolWeb {
		return fmt.Errorf("host %d is not a web host", hostID)
	}
	if password == "" {
		return secrets.DeleteWebPassword(hostID)
	}
	return secrets.SetWebPassword(hostID, password)
}

// resolvedAuth is the effective login identity for a connection: taken from a
// referenced credential when the host has one, otherwise from the host's own
// inline fields. passwordOwner picks the Secret Service key namespace.
type resolvedAuth struct {
	user          string
	authMethod    storage.AuthMethod
	identityFiles []string
	credentialID  *int64
	hostID        int64
}

// resolveAuth applies the hybrid credential model: a host with a credential_id
// inherits its user/auth/identity files; otherwise the host's own fields win.
// A dangling reference (credential deleted mid-flight) falls back to inline.
func (a *App) resolveAuth(host storage.Host) resolvedAuth {
	if host.CredentialID != nil {
		if cred, err := a.store.GetCredential(*host.CredentialID); err == nil {
			id := cred.ID
			return resolvedAuth{
				user:          cred.User,
				authMethod:    cred.AuthMethod,
				identityFiles: cred.IdentityFiles,
				credentialID:  &id,
				hostID:        host.ID,
			}
		}
	}
	return resolvedAuth{
		user:          host.User,
		authMethod:    host.AuthMethod,
		identityFiles: host.IdentityFiles,
		hostID:        host.ID,
	}
}

// getPassword / setPassword route to the credential's shared secret when the
// auth came from a credential, otherwise to the host's own secret.
func (r resolvedAuth) getPassword() (string, error) {
	if r.credentialID != nil {
		return secrets.GetCredentialPassword(*r.credentialID)
	}
	return secrets.GetHostPassword(r.hostID)
}

func (r resolvedAuth) setPassword(password string) error {
	if r.credentialID != nil {
		return secrets.SetCredentialPassword(*r.credentialID, password)
	}
	return secrets.SetHostPassword(r.hostID, password)
}
