// Package forwarding implements SSH port forwarding (local -L, remote -R and
// dynamic -D/SOCKS5) layered on top of an already-established connection from
// internal/ssh. Each rule becomes a Tunnel that owns a single listener and the
// set of connection pairs it is currently splicing.
//
// The engine is written for a long-lived desktop app, so teardown is the part
// that matters most: Tunnel.Close is synchronous and leak-free — it stops the
// listener, force-closes every in-flight connection pair so their copy
// goroutines unblock, and only returns once they have all exited. Callers can
// therefore Close a tunnel (or close the whole SSH connection) without leaving
// dangling goroutines or half-open sockets behind.
package forwarding

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
)

// Kind is the direction of a forward, mirroring ssh -L / -R / -D.
type Kind string

const (
	// Local forwards a local listener through the SSH connection to a
	// destination the SSH server can reach (ssh -L).
	Local Kind = "local"
	// Remote forwards a listener opened on the SSH server back to a
	// destination this machine can reach (ssh -R).
	Remote Kind = "remote"
	// Dynamic exposes a local SOCKS5 proxy whose per-connection destination is
	// dialed through the SSH server (ssh -D).
	Dynamic Kind = "dynamic"
)

// SSHConn is the minimal subset of *ssh.Client the engine needs. Taking an
// interface keeps the engine testable without a real SSH server.
type SSHConn interface {
	// Dial opens a connection from the SSH server's side of the link.
	Dial(network, addr string) (net.Conn, error)
	// Listen opens a listener on the SSH server's side of the link.
	Listen(network, addr string) (net.Listener, error)
}

// Spec describes a single forwarding rule.
type Spec struct {
	Kind     Kind
	BindAddr string // interface to bind; empty means loopback only
	BindPort int    // 0 asks the OS to pick a free port (useful in tests)
	DestHost string // ignored for Dynamic
	DestPort int    // ignored for Dynamic
}

// defaultBindAddr keeps forwards loopback-only unless the user opts into a
// wider bind, matching OpenSSH's safe default.
const defaultBindAddr = "127.0.0.1"

func (s Spec) bindAddr() string {
	if strings.TrimSpace(s.BindAddr) == "" {
		return defaultBindAddr
	}
	return s.BindAddr
}

// Validate reports whether the spec is well-formed, without opening anything.
func (s Spec) Validate() error {
	switch s.Kind {
	case Local, Remote, Dynamic:
	default:
		return fmt.Errorf("unknown forward kind %q", s.Kind)
	}
	if s.BindPort < 0 || s.BindPort > 65535 {
		return fmt.Errorf("bind port %d out of range", s.BindPort)
	}
	if s.Kind != Dynamic {
		if strings.TrimSpace(s.DestHost) == "" {
			return errors.New("destination host is required")
		}
		if s.DestPort < 1 || s.DestPort > 65535 {
			return fmt.Errorf("destination port %d out of range", s.DestPort)
		}
	}
	return nil
}

// Tunnel is a running forward. It is safe to Close exactly once; further calls
// are no-ops.
type Tunnel struct {
	spec     Spec
	conn     SSHConn
	listener net.Listener
	onError  func(error)

	wg sync.WaitGroup // accept loop + every in-flight handler

	mu     sync.Mutex
	closed bool
	active map[net.Conn]struct{} // every socket a handler currently owns
}

// Open validates spec, opens its listener and starts accepting connections.
// onError, if non-nil, receives non-fatal per-connection errors (a failed
// dial, a rejected SOCKS request); it must be safe to call from many
// goroutines. The tunnel keeps running when a single connection fails.
func Open(conn SSHConn, spec Spec, onError func(error)) (*Tunnel, error) {
	if conn == nil {
		return nil, errors.New("forwarding: nil SSH connection")
	}
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	if onError == nil {
		onError = func(error) {}
	}

	bind := net.JoinHostPort(spec.bindAddr(), strconv.Itoa(spec.BindPort))

	var listener net.Listener
	var err error
	switch spec.Kind {
	case Remote:
		// The listener lives on the SSH server; connections arrive from there.
		listener, err = conn.Listen("tcp", bind)
		if err != nil {
			return nil, fmt.Errorf("open remote listener on %s: %w", bind, err)
		}
	default: // Local, Dynamic
		listener, err = net.Listen("tcp", bind)
		if err != nil {
			return nil, fmt.Errorf("open local listener on %s: %w", bind, err)
		}
	}

	t := &Tunnel{
		spec:     spec,
		conn:     conn,
		listener: listener,
		onError:  onError,
		active:   make(map[net.Conn]struct{}),
	}
	t.wg.Add(1)
	go t.serve()
	return t, nil
}

// Addr returns the address the listener actually bound to. For Local and
// Dynamic this is the local address; for Remote it is the server-side address.
func (t *Tunnel) Addr() string {
	return t.listener.Addr().String()
}

// Close stops the tunnel and blocks until every goroutine it started has
// exited. It is idempotent.
func (t *Tunnel) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	conns := make([]net.Conn, 0, len(t.active))
	for c := range t.active {
		conns = append(conns, c)
	}
	t.mu.Unlock()

	// Closing the listener unblocks the accept loop; force-closing every socket
	// a handler owns unblocks its io.Copy / handshake read so wg.Wait can
	// return. net.Conn.Close is safe to call concurrently with the handler's
	// own deferred close.
	err := t.listener.Close()
	for _, c := range conns {
		_ = c.Close()
	}
	t.wg.Wait()
	return err
}

func (t *Tunnel) serve() {
	defer t.wg.Done()
	for {
		incoming, err := t.listener.Accept()
		if err != nil {
			// A closed listener is the normal stop path; anything else is
			// reported once and also ends the accept loop.
			if !t.isClosed() {
				t.onError(fmt.Errorf("accept on %s: %w", t.Addr(), err))
			}
			return
		}
		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			t.handle(incoming)
		}()
	}
}

// handle services one accepted connection: it works out the destination, dials
// it through the appropriate side of the link and splices the two together.
// The incoming socket is tracked from the outset so a Close during the
// (possibly stalled) SOCKS handshake or dial force-closes it too.
func (t *Tunnel) handle(incoming net.Conn) {
	if !t.register(incoming) {
		_ = incoming.Close()
		return
	}
	defer t.unregister(incoming)

	dest, err := t.destinationFor(incoming)
	if err != nil {
		t.onError(err)
		_ = incoming.Close()
		return
	}

	var outgoing net.Conn
	switch t.spec.Kind {
	case Remote:
		// Server-side connection; dial the destination from this machine.
		outgoing, err = net.Dial("tcp", dest)
	default: // Local, Dynamic
		// Local connection; dial the destination from the server's side.
		outgoing, err = t.conn.Dial("tcp", dest)
	}
	if err != nil {
		t.onError(fmt.Errorf("dial %s: %w", dest, err))
		_ = incoming.Close()
		return
	}
	if !t.register(outgoing) {
		// Tunnel closed while dialing; drop both sockets cleanly.
		_ = incoming.Close()
		_ = outgoing.Close()
		return
	}
	defer t.unregister(outgoing)

	splice(incoming, outgoing)
}

// destinationFor resolves the "host:port" this connection should reach. For
// Local/Remote it is the fixed spec destination; for Dynamic it is negotiated
// with the SOCKS5 client on the incoming connection.
func (t *Tunnel) destinationFor(incoming net.Conn) (string, error) {
	if t.spec.Kind == Dynamic {
		return socksHandshake(incoming)
	}
	return net.JoinHostPort(t.spec.DestHost, strconv.Itoa(t.spec.DestPort)), nil
}

func (t *Tunnel) register(c net.Conn) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return false
	}
	t.active[c] = struct{}{}
	return true
}

func (t *Tunnel) unregister(c net.Conn) {
	t.mu.Lock()
	delete(t.active, c)
	t.mu.Unlock()
}

func (t *Tunnel) isClosed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closed
}

// splice copies bytes in both directions until each side reaches EOF, using a
// TCP half-close (CloseWrite) so an EOF from one peer is propagated to the
// other without tearing down the still-active direction. Both connections are
// closed once both directions have finished, which also unblocks either copy
// if the tunnel force-closed the connections during shutdown.
func splice(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go copyHalf(&wg, a, b)
	go copyHalf(&wg, b, a)
	wg.Wait()
	_ = a.Close()
	_ = b.Close()
}

type closeWriter interface{ CloseWrite() error }

func copyHalf(wg *sync.WaitGroup, dst, src net.Conn) {
	defer wg.Done()
	_, _ = io.Copy(dst, src)
	// Signal EOF to the peer's reader without closing the whole socket, so the
	// opposite direction can keep draining. Both *net.TCPConn and ssh channels
	// implement CloseWrite; anything else falls back to the deferred full close
	// in splice.
	if cw, ok := dst.(closeWriter); ok {
		_ = cw.CloseWrite()
	}
}
