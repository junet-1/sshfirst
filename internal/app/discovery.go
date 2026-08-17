package app

import (
	"fmt"
	"net"
	"strconv"
	"time"

	xssh "golang.org/x/crypto/ssh"

	"ssh-first/internal/discovery"
	"ssh-first/internal/forwarding"
	"ssh-first/internal/storage"
)

// discoveryTimeout bounds the remote `ss`/`netstat` call. A host that is slow
// enough to exceed this is not one the user wants to keep waiting on, and the
// scan is always repeatable.
const discoveryTimeout = 15 * time.Second

// DiscoveredForward is an ad-hoc tunnel opened straight from a scan result.
type DiscoveredForward struct {
	// RuleID is negative: an ad-hoc forward has no persisted rule behind it,
	// but StopForward and ListActiveForwards address it the same way.
	RuleID int64 `json:"ruleId"`
	// LocalAddr is what the tunnel actually bound, e.g. "127.0.0.1:43117".
	LocalAddr string `json:"localAddr"`
	// Port is the remote port being forwarded, so the frontend can match the
	// result back to the scan row that triggered it.
	Port int `json:"port"`
}

// sshRunner executes one-shot commands on an established connection, on their
// own channel, so a scan never touches the user's interactive shells.
type sshRunner struct {
	client *xssh.Client
}

// Run returns the command's stdout. A non-zero exit is an error, which is what
// lets discovery.Scan tell "this tool is missing" from "this host listens on
// nothing". The session is closed on timeout, which unblocks the pending read.
func (r sshRunner) Run(command string) ([]byte, error) {
	session, err := r.client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("open session: %w", err)
	}

	type result struct {
		out []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := session.Output(command)
		done <- result{out: out, err: err}
	}()

	select {
	case res := <-done:
		_ = session.Close()
		return res.out, res.err
	case <-time.After(discoveryTimeout):
		_ = session.Close()
		<-done // the goroutine is unblocked by the close; let it finish
		return nil, fmt.Errorf("%q timed out after %s", command, discoveryTimeout)
	}
}

// DiscoverPorts reports what the connected host is listening on, so the user
// can reach a service without working out the ssh -L invocation themselves.
func (a *App) DiscoverPorts(connectionID string) ([]discovery.Listener, error) {
	client, err := a.connectedClient(connectionID)
	if err != nil {
		return nil, err
	}
	listeners, err := discovery.Scan(sshRunner{client: client})
	if err != nil {
		return nil, err
	}
	return listeners, nil
}

// ForwardDiscoveredPort opens a local forward to a port found by DiscoverPorts,
// binding a free local port chosen by the OS. Asking for the same remote port
// twice returns the existing tunnel rather than stacking a second one.
func (a *App) ForwardDiscoveredPort(connectionID string, port int, address string) (DiscoveredForward, error) {
	if port < 1 || port > 65535 {
		return DiscoveredForward{}, fmt.Errorf("port %d out of range", port)
	}
	destination := discovery.DialAddress(address)
	if net.ParseIP(destination) == nil {
		return DiscoveredForward{}, fmt.Errorf("%q is not a valid listening address", address)
	}

	client, err := a.connectedClient(connectionID)
	if err != nil {
		return DiscoveredForward{}, err
	}
	if existing, ok := a.existingAdhocForward(connectionID, destination, port); ok {
		return existing, nil
	}

	rule := storage.ForwardRule{
		ID:       a.nextAdhocForwardID(),
		Kind:     storage.ForwardLocal,
		Label:    fmt.Sprintf("Discovered :%d", port),
		BindAddr: "127.0.0.1",
		BindPort: 0, // let the OS pick, so a scan can never collide with a saved rule
		DestHost: destination,
		DestPort: port,
	}

	tunnel, err := forwarding.Open(client, specFromRule(rule), a.forwardErrorReporter(connectionID, rule))
	if err != nil {
		return DiscoveredForward{}, err
	}
	// Record the port the OS actually handed out, so everything downstream —
	// the Inspector's forward list, the status event — describes the real
	// tunnel rather than the placeholder 0.
	if _, boundPort, splitErr := net.SplitHostPort(tunnel.Addr()); splitErr == nil {
		rule.BindPort, _ = strconv.Atoi(boundPort)
	}

	a.mu.Lock()
	// Re-check under the lock: the connection may have gone away, or a second
	// click may have won the race to forward this same port, while Open ran.
	cs, ok := a.connections[connectionID]
	if !ok || cs.status != StatusConnected {
		a.mu.Unlock()
		_ = tunnel.Close()
		return DiscoveredForward{}, fmt.Errorf("connection %q is no longer ready", connectionID)
	}
	if existing, found := findAdhocForward(cs, destination, port); found {
		a.mu.Unlock()
		_ = tunnel.Close()
		return existing, nil
	}
	cs.forwards[rule.ID] = &activeForward{tunnel: tunnel, rule: rule}
	a.mu.Unlock()

	a.emit("forward:status", ForwardStatusEvent{
		ConnectionID: connectionID,
		RuleID:       rule.ID,
		Active:       true,
		Kind:         string(rule.Kind),
		Label:        rule.Label,
		BoundAddr:    tunnel.Addr(),
	})
	return DiscoveredForward{RuleID: rule.ID, LocalAddr: tunnel.Addr(), Port: port}, nil
}

// nextAdhocForwardID hands out negative IDs. Persisted rules are positive
// SQLite row IDs, so the two can share the connection's forward map without a
// saved rule and a scan result ever addressing each other.
func (a *App) nextAdhocForwardID() int64 {
	return -a.adhocForwards.Add(1)
}

// existingAdhocForward finds a running ad-hoc tunnel to the same destination,
// so repeatedly clicking Open reuses one local port instead of leaking tunnels.
func (a *App) existingAdhocForward(connectionID, destination string, port int) (DiscoveredForward, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cs, ok := a.connections[connectionID]
	if !ok {
		return DiscoveredForward{}, false
	}
	return findAdhocForward(cs, destination, port)
}

// findAdhocForward must be called with the App lock held. Persisted rules are
// deliberately skipped: reusing one here would later let a scan row stop a
// forward the user configured themselves.
func findAdhocForward(cs *connectionState, destination string, port int) (DiscoveredForward, bool) {
	for id, af := range cs.forwards {
		if id >= 0 || af.rule.DestPort != port || af.rule.DestHost != destination {
			continue
		}
		return DiscoveredForward{RuleID: id, LocalAddr: af.tunnel.Addr(), Port: port}, true
	}
	return DiscoveredForward{}, false
}

// connectedClient resolves a connection ID to a client that is ready to carry
// new channels.
func (a *App) connectedClient(connectionID string) (*xssh.Client, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cs, ok := a.connections[connectionID]
	if !ok {
		return nil, fmt.Errorf("no such connection %q", connectionID)
	}
	if cs.status != StatusConnected || cs.client == nil {
		return nil, fmt.Errorf("connection %q is not ready", connectionID)
	}
	return cs.client.Client, nil
}
