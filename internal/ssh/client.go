// Package ssh wraps golang.org/x/crypto/ssh with SSH First's connection
// concerns: agent/identity/password/keyboard-interactive authentication,
// optional agent forwarding, mandatory known_hosts verification with explicit
// user approval for
// order, mandatory known_hosts verification with explicit user approval for
// unknown or changed keys, and ProxyJump hop chains. It never reads secrets
// itself — callers inject PasswordProvider/PassphraseProvider callbacks
// backed by internal/secrets and a frontend prompt.
package ssh

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	xssh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// ConnectConfig describes everything needed to establish one SSH connection.
type ConnectConfig struct {
	Hostname       string
	Port           int
	User           string
	IdentityFiles  []string
	WantPassword   bool // true when the host's AuthMethod is "password"
	ForwardAgent   bool
	ProxyJump      string // "[user@]host[:port]" hops, comma-separated
	KnownHostsPath string
	ConnectTimeout time.Duration

	ApproveHostKey     HostKeyApprover
	ProvidePassword    PasswordProvider
	ProvidePassphrase  PassphraseProvider
	ProvideInteractive KeyboardInteractiveProvider
}

// Client is an established SSH connection, possibly tunnelled through one or
// more ProxyJump hops. Closing it tears down the whole chain.
type Client struct {
	*xssh.Client
	hops            []*xssh.Client
	ServerVersion   string
	AuthMethod      string
	AgentForwarding bool
	// AgentForwardWarning is set when agent forwarding was requested but could
	// not be enabled. The connection is still usable; the caller decides how to
	// surface this (a non-fatal notice), rather than failing the whole connect.
	AgentForwardWarning string
}

type authMethodTracker struct {
	mu     sync.Mutex
	method string
}

func (t *authMethodTracker) mark(method string) {
	t.mu.Lock()
	t.method = method
	t.mu.Unlock()
}

func (t *authMethodTracker) value() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.method
}

// Close tears down the target connection and, in reverse order, every
// ProxyJump hop it was tunnelled through.
func (c *Client) Close() error {
	var firstErr error
	if c.Client != nil {
		firstErr = c.Client.Close()
	}
	for i := len(c.hops) - 1; i >= 0; i-- {
		if err := c.hops[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Connect dials cfg.Hostname (through any ProxyJump hops) and completes the
// SSH handshake, including host key verification.
func Connect(cfg ConnectConfig) (*Client, error) {
	timeout := cfg.ConnectTimeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	hops, err := parseProxyJump(cfg.ProxyJump)
	if err != nil {
		return nil, err
	}

	var hopClients []*xssh.Client
	var dialer func(network, addr string) (net.Conn, error) = func(network, addr string) (net.Conn, error) {
		return net.DialTimeout(network, addr, timeout)
	}

	for _, hop := range hops {
		hopUser := hop.user
		if hopUser == "" {
			hopUser = cfg.User // ProxyJump entries without an explicit user reuse the target host's user
		}
		hopClientConfig, err := clientConfigFor(hopUser, hop.host, cfg, timeout, nil)
		if err != nil {
			closeAll(hopClients)
			return nil, err
		}
		addr := net.JoinHostPort(hop.host, strconv.Itoa(hop.port))
		conn, err := dialer("tcp", addr)
		if err != nil {
			closeAll(hopClients)
			return nil, fmt.Errorf("dial proxy jump %s: %w", addr, err)
		}
		sshConn, chans, reqs, err := xssh.NewClientConn(conn, addr, hopClientConfig)
		if err != nil {
			closeAll(hopClients)
			return nil, fmt.Errorf("handshake with proxy jump %s: %w", addr, err)
		}
		hopClient := xssh.NewClient(sshConn, chans, reqs)
		hopClients = append(hopClients, hopClient)
		dialer = hopClient.Dial
	}

	authTracker := &authMethodTracker{}
	targetConfig, err := clientConfigFor(cfg.User, cfg.Hostname, cfg, timeout, authTracker.mark)
	if err != nil {
		closeAll(hopClients)
		return nil, err
	}
	targetAddr := net.JoinHostPort(cfg.Hostname, strconv.Itoa(portOrDefault(cfg.Port)))
	conn, err := dialer("tcp", targetAddr)
	if err != nil {
		closeAll(hopClients)
		return nil, fmt.Errorf("dial %s: %w", targetAddr, err)
	}
	sshConn, chans, reqs, err := xssh.NewClientConn(conn, targetAddr, targetConfig)
	if err != nil {
		closeAll(hopClients)
		return nil, fmt.Errorf("handshake with %s: %w", targetAddr, err)
	}

	client := xssh.NewClient(sshConn, chans, reqs)
	agentForwarding := false
	agentWarning := ""
	if cfg.ForwardAgent {
		// Agent forwarding is a convenience, not a requirement for the session.
		// If no agent is reachable (common when the app is launched from a
		// desktop shortcut that doesn't inherit SSH_AUTH_SOCK), connect anyway
		// and report the miss instead of tearing down a working connection.
		socket := os.Getenv("SSH_AUTH_SOCK")
		if socket == "" {
			agentWarning = "Agent forwarding was requested, but no SSH agent is available (SSH_AUTH_SOCK is not set). Connected without agent forwarding."
		} else if err := agent.ForwardToRemote(client, socket); err != nil {
			agentWarning = fmt.Sprintf("Agent forwarding could not be enabled (%v). Connected without it.", err)
		} else {
			agentForwarding = true
		}
	}
	return &Client{
		Client:              client,
		hops:                hopClients,
		ServerVersion:       string(sshConn.ServerVersion()),
		AuthMethod:          authTracker.value(),
		AgentForwarding:     agentForwarding,
		AgentForwardWarning: agentWarning,
	}, nil
}

func clientConfigFor(user, hostname string, cfg ConnectConfig, timeout time.Duration, onAuthAttempt func(string)) (*xssh.ClientConfig, error) {
	hostKeyCallback, err := knownHostsCallback(cfg.KnownHostsPath, cfg.ApproveHostKey)
	if err != nil {
		return nil, err
	}
	auth, err := buildAuthMethods(cfg.IdentityFiles, cfg.WantPassword, user, hostname, cfg.ProvidePassphrase, cfg.ProvidePassword, cfg.ProvideInteractive, onAuthAttempt)
	if err != nil {
		return nil, err
	}
	if len(auth) == 0 {
		return nil, fmt.Errorf("no authentication method available for %s@%s (no agent, identity file, password, or keyboard-interactive prompt configured)", user, hostname)
	}
	return &xssh.ClientConfig{
		User:            user,
		Auth:            auth,
		HostKeyCallback: hostKeyCallback,
		Timeout:         timeout,
	}, nil
}

func closeAll(clients []*xssh.Client) {
	for i := len(clients) - 1; i >= 0; i-- {
		clients[i].Close()
	}
}

func portOrDefault(port int) int {
	if port <= 0 {
		return 22
	}
	return port
}

type proxyHop struct {
	user string
	host string
	port int
}

// parseProxyJump splits an OpenSSH-style ProxyJump value ("user@host:port",
// comma-separated for multiple hops) into ordered hops.
func parseProxyJump(value string) ([]proxyHop, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	hops := make([]proxyHop, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		hop := proxyHop{port: 22}
		if at := strings.LastIndex(part, "@"); at >= 0 {
			hop.user = part[:at]
			part = part[at+1:]
		}
		if h, p, err := net.SplitHostPort(part); err == nil {
			hop.host = h
			if portNum, err := strconv.Atoi(p); err == nil {
				hop.port = portNum
			}
		} else {
			hop.host = part
		}
		if hop.host == "" {
			return nil, fmt.Errorf("invalid ProxyJump entry: %q", part)
		}
		hops = append(hops, hop)
	}
	return hops, nil
}
