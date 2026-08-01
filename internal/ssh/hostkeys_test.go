package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	xssh "golang.org/x/crypto/ssh"
)

func newTestPublicKey(t *testing.T) xssh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	sshPub, err := xssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}
	return sshPub
}

func TestKnownHostsCallbackPromptsAndPersistsUnknownKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	key := newTestPublicKey(t)
	called := 0
	callback, err := knownHostsCallback(path, func(req HostKeyDecisionRequest) (bool, error) {
		called++
		if req.Status != HostKeyUnknown {
			t.Fatalf("status = %v, want unknown", req.Status)
		}
		if req.Fingerprint != xssh.FingerprintSHA256(key) {
			t.Fatalf("fingerprint = %q", req.Fingerprint)
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("knownHostsCallback: %v", err)
	}
	remote := &net.TCPAddr{IP: net.ParseIP("192.0.2.10"), Port: 22}
	if err := callback("example.com:22", remote, key); err != nil {
		t.Fatalf("first callback: %v", err)
	}
	if called != 1 {
		t.Fatalf("approver called %d times", called)
	}

	knownCallback, err := knownHostsCallback(path, func(HostKeyDecisionRequest) (bool, error) {
		t.Fatal("known key must not prompt")
		return false, nil
	})
	if err != nil {
		t.Fatalf("knownHostsCallback after persist: %v", err)
	}
	if err := knownCallback("example.com:22", remote, key); err != nil {
		t.Fatalf("known callback: %v", err)
	}
}

func TestKnownHostsCallbackReportsChangedKeyFingerprints(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	oldKey := newTestPublicKey(t)
	newKey := newTestPublicKey(t)
	if err := persistHostKey(path, "example.com", oldKey); err != nil {
		t.Fatalf("persist old key: %v", err)
	}

	callback, err := knownHostsCallback(path, func(req HostKeyDecisionRequest) (bool, error) {
		if req.Status != HostKeyChanged {
			t.Fatalf("status = %v, want changed", req.Status)
		}
		want := xssh.FingerprintSHA256(oldKey)
		if len(req.PreviousFingerprints) != 1 || req.PreviousFingerprints[0] != want {
			t.Fatalf("previous fingerprints = %#v, want %q", req.PreviousFingerprints, want)
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("knownHostsCallback: %v", err)
	}
	remote := &net.TCPAddr{IP: net.ParseIP("192.0.2.10"), Port: 22}
	if err := callback("example.com:22", remote, newKey); !errors.Is(err, ErrHostKeyRejected) {
		t.Fatalf("error = %v, want ErrHostKeyRejected", err)
	}
}

func TestHostMatchesField(t *testing.T) {
	cases := []struct {
		field string
		host  string
		want  bool
	}{
		{"example.com", "example.com", true},
		{"example.com,other.com", "other.com", true},
		{"example.com,other.com", "nope.com", false},
		{"[example.com]:2222", "[example.com]:2222", true},
	}
	for _, c := range cases {
		if got := hostMatchesField(c.field, c.host); got != c.want {
			t.Errorf("hostMatchesField(%q, %q) = %v, want %v", c.field, c.host, got, c.want)
		}
	}
}

func TestPersistHostKey_AppendsNewEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "known_hosts")
	if err := ensureFile(path); err != nil {
		t.Fatalf("ensureFile: %v", err)
	}

	key := newTestPublicKey(t)
	if err := persistHostKey(path, "example.com", key); err != nil {
		t.Fatalf("persistHostKey: %v", err)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(contents), "example.com") {
		t.Fatalf("expected known_hosts to contain the host, got: %q", contents)
	}
}

func TestPersistHostKey_ReplacesStaleEntryForSameAlgorithm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "known_hosts")
	if err := ensureFile(path); err != nil {
		t.Fatalf("ensureFile: %v", err)
	}

	oldKey := newTestPublicKey(t)
	if err := persistHostKey(path, "example.com", oldKey); err != nil {
		t.Fatalf("persistHostKey (old): %v", err)
	}

	newKey := newTestPublicKey(t)
	if err := persistHostKey(path, "example.com", newKey); err != nil {
		t.Fatalf("persistHostKey (new): %v", err)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(contents)), "\n")
	count := 0
	for _, line := range lines {
		if strings.Contains(line, "example.com") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 entry for example.com after key change, got %d:\n%s", count, contents)
	}
	if !strings.Contains(string(contents), string(xssh.MarshalAuthorizedKey(newKey)[:20])) {
		t.Fatalf("expected the new key to be present in known_hosts")
	}
}
