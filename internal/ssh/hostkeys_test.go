package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	xssh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
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

// Accepting a changed key must leave the host with exactly one trusted key,
// even when the replacement uses a different algorithm. Keeping the old entry
// would mean an attacker whose key the user once clicked through stays pinned
// and can return later without triggering the warning again.
func TestPersistHostKey_ReplacesEntriesOfEveryAlgorithm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "known_hosts")
	if err := ensureFile(path); err != nil {
		t.Fatalf("ensureFile: %v", err)
	}

	ed25519Key := newTestPublicKey(t)
	if err := persistHostKey(path, "example.com", ed25519Key); err != nil {
		t.Fatalf("persistHostKey (ed25519): %v", err)
	}

	rsaRaw, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	rsaKey, err := xssh.NewPublicKey(&rsaRaw.PublicKey)
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}
	if rsaKey.Type() == ed25519Key.Type() {
		t.Fatal("test needs two different key algorithms")
	}
	if err := persistHostKey(path, "example.com", rsaKey); err != nil {
		t.Fatalf("persistHostKey (rsa): %v", err)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(contents)), "\n") {
		if strings.Contains(line, "example.com") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 entry after an algorithm change, got %d:\n%s", count, contents)
	}
	if !strings.Contains(string(contents), rsaKey.Type()) {
		t.Fatalf("expected the newly accepted key to be the one left, got:\n%s", contents)
	}
}

// Transfers are blocked unless the host key was verified interactively first,
// so a false negative here would break rsync for hosts the user has long since
// approved. The port handling is the fiddly part: known_hosts writes a bare
// hostname for port 22 and a bracketed [host]:port for anything else, and the
// caller passes 0 for "default".
func TestIsHostKnown(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "known_hosts")
	key := newTestPublicKey(t)

	// Exactly what knownHostsCallback persists after the user approves.
	if err := persistHostKey(path, knownhosts.Normalize("example.com:22"), key); err != nil {
		t.Fatalf("persistHostKey: %v", err)
	}
	if err := persistHostKey(path, knownhosts.Normalize("odd.example:2222"), key); err != nil {
		t.Fatalf("persistHostKey: %v", err)
	}

	cases := []struct {
		host string
		port int
		want bool
	}{
		{"example.com", 22, true},
		{"example.com", 0, true}, // 0 means "default", i.e. 22
		{"example.com", 2222, false},
		{"odd.example", 2222, true},
		{"odd.example", 22, false},
		{"unseen.example", 22, false},
	}
	for _, tc := range cases {
		got, err := IsHostKnown(path, tc.host, tc.port)
		if err != nil {
			t.Fatalf("IsHostKnown(%s:%d): %v", tc.host, tc.port, err)
		}
		if got != tc.want {
			t.Errorf("IsHostKnown(%s:%d) = %v, want %v", tc.host, tc.port, got, tc.want)
		}
	}

	missing, err := IsHostKnown(filepath.Join(dir, "does-not-exist"), "example.com", 22)
	if err != nil || missing {
		t.Errorf("IsHostKnown on a missing file = (%v, %v), want (false, nil)", missing, err)
	}
}
