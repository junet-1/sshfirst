package app

import (
	"errors"
	"net"
	"testing"

	"ssh-first/internal/forwarding"
	"ssh-first/internal/storage"
)

// stubSSHConn stands in for the SSH side of a tunnel. Only the local listener
// matters here — nothing in these tests dials through the link.
type stubSSHConn struct{}

func (stubSSHConn) Dial(string, string) (net.Conn, error) {
	return nil, errors.New("stub connection is never dialled")
}

func (stubSSHConn) Listen(string, string) (net.Listener, error) {
	return nil, errors.New("stub connection never listens")
}

// adhocForward builds a running local tunnel to destination, registered the way
// ForwardDiscoveredPort registers one.
func adhocForward(t *testing.T, id int64, destHost string, destPort int) *activeForward {
	t.Helper()
	rule := storage.ForwardRule{
		ID:       id,
		Kind:     storage.ForwardLocal,
		BindAddr: "127.0.0.1",
		DestHost: destHost,
		DestPort: destPort,
	}
	tunnel, err := forwarding.Open(stubSSHConn{}, specFromRule(rule), nil)
	if err != nil {
		t.Fatalf("open tunnel: %v", err)
	}
	t.Cleanup(func() { _ = tunnel.Close() })
	return &activeForward{tunnel: tunnel, rule: rule}
}

// Ad-hoc forwards share the connection's forward map with persisted rules, so
// their IDs have to stay out of SQLite's positive row-ID range for good.
func TestAdhocForwardIDsAreNegativeAndUnique(t *testing.T) {
	a := New(nil)
	seen := make(map[int64]bool)
	for i := 0; i < 100; i++ {
		id := a.nextAdhocForwardID()
		if id >= 0 {
			t.Fatalf("ad-hoc forward ID %d would collide with a persisted rule", id)
		}
		if seen[id] {
			t.Fatalf("ad-hoc forward ID %d handed out twice", id)
		}
		seen[id] = true
	}
}

// Clicking Open on the same scan row twice must reuse the tunnel rather than
// binding a second local port to the same service.
func TestExistingAdhocForwardMatchesDestination(t *testing.T) {
	a := New(nil)
	a.connections["conn"] = &connectionState{
		id:     "conn",
		status: StatusConnected,
		forwards: map[int64]*activeForward{
			-1: adhocForward(t, -1, "127.0.0.1", 3000),
			// A persisted rule must not be mistaken for an ad-hoc one: reusing
			// it here would later let a scan row stop a forward the user set up
			// deliberately.
			7: adhocForward(t, 7, "127.0.0.1", 9090),
		},
	}

	if _, ok := a.existingAdhocForward("conn", "127.0.0.1", 9090); ok {
		t.Error("a persisted rule was returned as a reusable ad-hoc forward")
	}
	if _, ok := a.existingAdhocForward("conn", "127.0.0.1", 5432); ok {
		t.Error("an unrelated port was reported as already forwarded")
	}
	if _, ok := a.existingAdhocForward("conn", "10.0.0.5", 3000); ok {
		t.Error("the same port on a different bind address is a different service")
	}
	got, ok := a.existingAdhocForward("conn", "127.0.0.1", 3000)
	if !ok {
		t.Fatal("the matching ad-hoc forward was not found")
	}
	if got.RuleID != -1 || got.Port != 3000 {
		t.Errorf("got rule %d port %d, want -1/3000", got.RuleID, got.Port)
	}
}

func TestForwardDiscoveredPortRejectsBadInput(t *testing.T) {
	a := New(nil)
	if _, err := a.ForwardDiscoveredPort("conn", 0, "127.0.0.1"); err == nil {
		t.Error("port 0 should be rejected")
	}
	if _, err := a.ForwardDiscoveredPort("conn", 70000, "127.0.0.1"); err == nil {
		t.Error("an out-of-range port should be rejected")
	}
	// A scan reports literal addresses; anything else means the caller is not
	// passing back a scan result, so it must not become a dial destination.
	if _, err := a.ForwardDiscoveredPort("conn", 3000, "evil.example.test"); err == nil {
		t.Error("a non-IP listening address should be rejected")
	}
}
