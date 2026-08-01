package secrets

import (
	"testing"

	"github.com/zalando/go-keyring"
)

func TestMain(m *testing.M) {
	keyring.MockInit() // avoid touching a real Secret Service/D-Bus session in tests
	m.Run()
}

func TestHostPasswordRoundTrip(t *testing.T) {
	const hostID = int64(42)

	if _, err := GetHostPassword(hostID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound before any password is set, got %v", err)
	}

	if err := SetHostPassword(hostID, "s3cret"); err != nil {
		t.Fatalf("SetHostPassword: %v", err)
	}
	got, err := GetHostPassword(hostID)
	if err != nil {
		t.Fatalf("GetHostPassword: %v", err)
	}
	if got != "s3cret" {
		t.Fatalf("expected s3cret, got %q", got)
	}

	if err := DeleteHostPassword(hostID); err != nil {
		t.Fatalf("DeleteHostPassword: %v", err)
	}
	if _, err := GetHostPassword(hostID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestHostPasswordIsolatedPerHost(t *testing.T) {
	if err := SetHostPassword(1, "one"); err != nil {
		t.Fatalf("SetHostPassword(1): %v", err)
	}
	if err := SetHostPassword(2, "two"); err != nil {
		t.Fatalf("SetHostPassword(2): %v", err)
	}

	v1, err := GetHostPassword(1)
	if err != nil || v1 != "one" {
		t.Fatalf("GetHostPassword(1) = %q, %v", v1, err)
	}
	v2, err := GetHostPassword(2)
	if err != nil || v2 != "two" {
		t.Fatalf("GetHostPassword(2) = %q, %v", v2, err)
	}
}

func TestIdentityPassphraseRoundTrip(t *testing.T) {
	const path = "/home/user/.ssh/id_ed25519"

	if _, err := GetIdentityPassphrase(path); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound initially, got %v", err)
	}

	if err := SetIdentityPassphrase(path, "hunter2"); err != nil {
		t.Fatalf("SetIdentityPassphrase: %v", err)
	}
	got, err := GetIdentityPassphrase(path)
	if err != nil || got != "hunter2" {
		t.Fatalf("GetIdentityPassphrase = %q, %v", got, err)
	}

	if err := DeleteIdentityPassphrase(path); err != nil {
		t.Fatalf("DeleteIdentityPassphrase: %v", err)
	}
	if _, err := GetIdentityPassphrase(path); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDeleteHostPasswordWhenNoneExistsIsNotAnError(t *testing.T) {
	if err := DeleteHostPassword(999); err != nil {
		t.Fatalf("expected deleting a non-existent password to be a no-op, got %v", err)
	}
}
