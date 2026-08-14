// Package terminal manages interactive PTY sessions multiplexed over an
// established SSH connection (one ssh.Client can host several terminal tabs,
// each its own Session/channel).
package terminal

import (
	"fmt"
	"io"
	"sync"

	xssh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// OutputHandler receives raw terminal output bytes as they arrive, meant to
// be forwarded to xterm.js verbatim (it handles ANSI escape sequences itself).
// Returning false stops the pump when the owning tab/session was closed or
// replaced while a backpressured write was waiting.
type OutputHandler func(data []byte) bool

// ClosedHandler is invoked once when the remote shell exits, with the reason
// (nil for a clean exit).
type ClosedHandler func(err error)

// Options configures a new terminal Session.
type Options struct {
	Cols, Rows   int
	Term         string // TERM value advertised to the remote shell; defaults to "xterm-256color"
	ForwardAgent bool   // expose the local SSH agent inside this remote PTY

	OnOutput OutputHandler
	OnClosed ClosedHandler
}

// Session is one interactive PTY channel within an SSH connection.
type Session struct {
	session *xssh.Session

	mu     sync.Mutex
	stdin  io.WriteCloser
	closed bool
}

// Start opens a new PTY-backed shell session on client.
func Start(client *xssh.Client, opts Options) (*Session, error) {
	sess, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("open session: %w", err)
	}

	term := opts.Term
	if term == "" {
		term = "xterm-256color"
	}
	cols, rows := opts.Cols, opts.Rows
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	modes := xssh.TerminalModes{
		xssh.ECHO:          1,
		xssh.TTY_OP_ISPEED: 14400,
		xssh.TTY_OP_OSPEED: 14400,
	}
	if err := sess.RequestPty(term, rows, cols, modes); err != nil {
		sess.Close()
		return nil, fmt.Errorf("request pty: %w", err)
	}
	if opts.ForwardAgent {
		if err := agent.RequestAgentForwarding(sess); err != nil {
			sess.Close()
			return nil, fmt.Errorf("request agent forwarding: %w", err)
		}
	}

	stdin, err := sess.StdinPipe()
	if err != nil {
		sess.Close()
		return nil, err
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		sess.Close()
		return nil, err
	}
	stderr, err := sess.StderrPipe()
	if err != nil {
		sess.Close()
		return nil, err
	}

	if err := sess.Shell(); err != nil {
		sess.Close()
		return nil, fmt.Errorf("start shell: %w", err)
	}

	s := &Session{session: sess, stdin: stdin}

	var pumpsDone sync.WaitGroup
	pumpsDone.Add(2)
	go func() { defer pumpsDone.Done(); pump(stdout, opts.OnOutput) }()
	go func() { defer pumpsDone.Done(); pump(stderr, opts.OnOutput) }()

	go func() {
		waitErr := sess.Wait()
		pumpsDone.Wait() // make sure all buffered output reached the frontend before signalling closed
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		if opts.OnClosed != nil {
			opts.OnClosed(waitErr)
		}
	}()

	return s, nil
}

func pump(r io.Reader, onOutput OutputHandler) {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 && onOutput != nil {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			if !onOutput(chunk) {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// Write sends keystrokes/pasted input to the remote shell.
func (s *Session) Write(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("session already closed")
	}
	_, err := s.stdin.Write(data)
	return err
}

// Resize propagates an xterm.js viewport resize to the remote PTY.
func (s *Session) Resize(cols, rows int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("session already closed")
	}
	return s.session.WindowChange(rows, cols)
}

// Close terminates the remote shell and releases the underlying channel.
func (s *Session) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	return s.session.Close()
}
