package forwarding

import (
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"testing"
	"time"
)

// fakeSSH stands in for *ssh.Client. Dialing and listening simply use the local
// network stack, which is enough to exercise the engine's accept/dial/splice/
// teardown machinery without a real SSH server.
type fakeSSH struct{}

func (fakeSSH) Dial(network, addr string) (net.Conn, error) { return net.Dial(network, addr) }
func (fakeSSH) Listen(network, addr string) (net.Listener, error) {
	return net.Listen(network, addr)
}

// echoServer accepts connections and echoes everything back until EOF.
func echoServer(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("echo listen: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()
	return ln
}

func hostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split %q: %v", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("port %q: %v", portStr, err)
	}
	return host, port
}

func roundTrip(t *testing.T, addr, msg string) string {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial tunnel %s: %v", addr, err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(buf)
}

func TestLocalForward(t *testing.T) {
	echo := echoServer(t)
	defer echo.Close()
	host, port := hostPort(t, echo.Addr().String())

	tun, err := Open(fakeSSH{}, Spec{Kind: Local, BindPort: 0, DestHost: host, DestPort: port}, nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer tun.Close()

	if got := roundTrip(t, tun.Addr(), "hello"); got != "hello" {
		t.Fatalf("echo mismatch: got %q", got)
	}
}

func TestRemoteForward(t *testing.T) {
	echo := echoServer(t)
	defer echo.Close()
	host, port := hostPort(t, echo.Addr().String())

	// For a remote forward the listener is opened via the SSH side (here the
	// local stack) and the destination is dialed locally.
	tun, err := Open(fakeSSH{}, Spec{Kind: Remote, BindPort: 0, DestHost: host, DestPort: port}, nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer tun.Close()

	if got := roundTrip(t, tun.Addr(), "remote"); got != "remote" {
		t.Fatalf("echo mismatch: got %q", got)
	}
}

func TestDynamicForwardSOCKS(t *testing.T) {
	echo := echoServer(t)
	defer echo.Close()
	echoHost, echoPort := hostPort(t, echo.Addr().String())

	tun, err := Open(fakeSSH{}, Spec{Kind: Dynamic, BindPort: 0}, nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer tun.Close()

	conn, err := net.DialTimeout("tcp", tun.Addr(), 2*time.Second)
	if err != nil {
		t.Fatalf("dial socks: %v", err)
	}
	defer conn.Close()

	// Greeting: one method, no-auth.
	if _, err := conn.Write([]byte{socksVersion, 1, socksNoAuth}); err != nil {
		t.Fatalf("greeting: %v", err)
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatalf("greeting reply: %v", err)
	}
	if reply[0] != socksVersion || reply[1] != socksNoAuth {
		t.Fatalf("bad greeting reply: %v", reply)
	}

	// CONNECT to the echo server by IPv4.
	req := []byte{socksVersion, socksCmdConnect, 0x00, socksAtypIPv4}
	req = append(req, net.ParseIP(echoHost).To4()...)
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(echoPort))
	req = append(req, portBytes...)
	if _, err := conn.Write(req); err != nil {
		t.Fatalf("connect req: %v", err)
	}
	connectReply := make([]byte, 10)
	if _, err := io.ReadFull(conn, connectReply); err != nil {
		t.Fatalf("connect reply: %v", err)
	}
	if connectReply[1] != socksRepSuccess {
		t.Fatalf("socks connect failed: rep=0x%02x", connectReply[1])
	}

	// The channel is now transparent to the echo server.
	if _, err := conn.Write([]byte("socks")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 5)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "socks" {
		t.Fatalf("echo mismatch: got %q", buf)
	}
}

// TestCloseUnblocksInFlight verifies that Close force-closes a connection whose
// copy goroutines are blocked on an idle peer, so Close returns promptly rather
// than hanging on wg.Wait.
func TestCloseUnblocksInFlight(t *testing.T) {
	// A blackhole destination accepts and then never reads, writes or closes.
	blackhole, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("blackhole listen: %v", err)
	}
	held := make(chan net.Conn, 1)
	go func() {
		conn, err := blackhole.Accept()
		if err != nil {
			return
		}
		held <- conn // keep it open, do nothing
	}()
	defer blackhole.Close()
	host, port := hostPort(t, blackhole.Addr().String())

	tun, err := Open(fakeSSH{}, Spec{Kind: Local, BindPort: 0, DestHost: host, DestPort: port}, nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	client, err := net.DialTimeout("tcp", tun.Addr(), 2*time.Second)
	if err != nil {
		t.Fatalf("dial tunnel: %v", err)
	}
	defer client.Close()
	// Trigger the dial to the blackhole and keep the client connection idle.
	if _, err := client.Write([]byte("x")); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case <-held:
	case <-time.After(2 * time.Second):
		t.Fatal("destination was never dialed through the tunnel")
	}

	done := make(chan struct{})
	go func() {
		_ = tun.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not return; in-flight connection was not force-closed")
	}
}

func TestCloseIdempotentAndStopsAccepting(t *testing.T) {
	echo := echoServer(t)
	defer echo.Close()
	host, port := hostPort(t, echo.Addr().String())

	tun, err := Open(fakeSSH{}, Spec{Kind: Local, BindPort: 0, DestHost: host, DestPort: port}, nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	addr := tun.Addr()

	if err := tun.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := tun.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	if conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond); err == nil {
		conn.Close()
		t.Fatal("tunnel still accepting after Close")
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		spec Spec
		ok   bool
	}{
		{"local ok", Spec{Kind: Local, BindPort: 8080, DestHost: "db", DestPort: 5432}, true},
		{"dynamic ok no dest", Spec{Kind: Dynamic, BindPort: 1080}, true},
		{"unknown kind", Spec{Kind: "sideways", BindPort: 1}, false},
		{"local missing dest host", Spec{Kind: Local, BindPort: 8080, DestPort: 5432}, false},
		{"local bad dest port", Spec{Kind: Local, BindPort: 8080, DestHost: "db", DestPort: 0}, false},
		{"bind port out of range", Spec{Kind: Dynamic, BindPort: 70000}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.spec.Validate()
			if tc.ok && err != nil {
				t.Fatalf("expected valid, got %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatal("expected invalid, got nil")
			}
		})
	}
}
