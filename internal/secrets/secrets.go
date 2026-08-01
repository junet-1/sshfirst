// Package secrets stores passwords and private-key passphrases in the
// platform-native secret store (Linux Secret Service — GNOME Keyring or
// KWallet's ksecretservice implementation — via D-Bus). Nothing handled by
// this package is ever written to the SQLite database or to logs.
package secrets

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

// service is the Secret Service "service" attribute all SSH First entries
// are stored under, so they show up grouped in KWallet/Seahorse.
const service = "ssh-first"

// ErrNotFound is returned when no secret exists for the given lookup.
var ErrNotFound = errors.New("secret not found")

// SetHostPassword stores (or replaces) the login password for a host.
func SetHostPassword(hostID int64, password string) error {
	return keyring.Set(service, passwordKey(hostID), password)
}

// GetHostPassword retrieves a previously stored host password.
func GetHostPassword(hostID int64) (string, error) {
	return get(passwordKey(hostID))
}

// DeleteHostPassword removes a stored host password, if any.
func DeleteHostPassword(hostID int64) error {
	return del(passwordKey(hostID))
}

// SetCredentialPassword stores (or replaces) the login password for a reusable
// credential, shared by every host that references it.
func SetCredentialPassword(credentialID int64, password string) error {
	return keyring.Set(service, credentialPasswordKey(credentialID), password)
}

// GetCredentialPassword retrieves a previously stored credential password.
func GetCredentialPassword(credentialID int64) (string, error) {
	return get(credentialPasswordKey(credentialID))
}

// DeleteCredentialPassword removes a stored credential password, if any.
func DeleteCredentialPassword(credentialID int64) error {
	return del(credentialPasswordKey(credentialID))
}

// SetIdentityPassphrase stores the passphrase protecting a private key file,
// keyed by the key's path (so the same key reused across hosts only prompts
// once).
func SetIdentityPassphrase(identityFilePath, passphrase string) error {
	return keyring.Set(service, passphraseKey(identityFilePath), passphrase)
}

// GetIdentityPassphrase retrieves a stored private key passphrase.
func GetIdentityPassphrase(identityFilePath string) (string, error) {
	return get(passphraseKey(identityFilePath))
}

// DeleteIdentityPassphrase removes a stored private key passphrase.
func DeleteIdentityPassphrase(identityFilePath string) error {
	return del(passphraseKey(identityFilePath))
}

func get(key string) (string, error) {
	v, err := keyring.Get(service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read secret: %w", err)
	}
	return v, nil
}

func del(key string) error {
	err := keyring.Delete(service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

func passwordKey(hostID int64) string {
	return fmt.Sprintf("host-password:%d", hostID)
}

func credentialPasswordKey(credentialID int64) string {
	return fmt.Sprintf("credential-password:%d", credentialID)
}

// passphraseKey hashes the file path so arbitrary filesystem paths (which
// may contain characters the underlying keyring backend dislikes) become a
// stable, opaque attribute key.
func passphraseKey(identityFilePath string) string {
	sum := sha256.Sum256([]byte(identityFilePath))
	return "identity-passphrase:" + hex.EncodeToString(sum[:])
}
