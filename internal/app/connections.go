package app

import (
	"fmt"
	"net"
	"os"
	osuser "os/user"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	sftppkg "ssh-first/internal/sftp"
	sshpkg "ssh-first/internal/ssh"
	"ssh-first/internal/storage"
	terminalpkg "ssh-first/internal/terminal"
)

const keepaliveInterval = 10 * time.Second

// ConnectedConnectionID returns one live connection for a persisted host.
// Tool windows use this to offer connection-bound actions without having to
// duplicate the main window's frontend connection store.
func (a *App) ConnectedConnectionID(hostID int64) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	for id, connection := range a.connections {
		if connection.hostID == hostID && connection.status == StatusConnected && connection.client != nil {
			return id
		}
	}
	return ""
}

// Connect establishes an SSH connection to host and opens its first terminal
// tab. It blocks until the handshake (including any host key or credential
// prompts routed to the frontend) completes.
func (a *App) Connect(hostID int64, cols, rows int) (ConnectionInfo, error) {
	if err := a.requireStore(); err != nil {
		return ConnectionInfo{}, err
	}
	host, err := a.store.GetHost(hostID)
	if err != nil {
		return ConnectionInfo{}, err
	}
	return a.connectHost(host, false, cols, rows)
}

// QuickConnect opens an ephemeral connection from an OpenSSH-style target.
// It deliberately does not create a sidebar host or persist credentials.
func (a *App) QuickConnect(target string, cols, rows int) (ConnectionInfo, error) {
	host, err := parseQuickTarget(target)
	if err != nil {
		return ConnectionInfo{}, err
	}
	return a.connectHost(host, true, cols, rows)
}

func (a *App) connectHost(host storage.Host, ephemeral bool, cols, rows int) (ConnectionInfo, error) {
	if err := a.requireStore(); err != nil {
		return ConnectionInfo{}, err
	}

	connID := uuid.NewString()
	cs := &connectionState{
		id:            connID,
		hostID:        host.ID,
		host:          host,
		hostLabel:     host.Label,
		ephemeral:     ephemeral,
		status:        StatusConnecting,
		stopKeepalive: make(chan struct{}),
		forwards:      make(map[int64]*activeForward),
	}
	a.mu.Lock()
	a.connections[connID] = cs
	a.mu.Unlock()
	a.emit("connection:status", StatusEvent{ConnectionID: connID, HostID: host.ID, Status: StatusConnecting})

	client, err := a.dial(host, !ephemeral)
	if err != nil {
		a.mu.Lock()
		cs.status = StatusError
		cs.lastError = safeErrorMessage(err)
		delete(a.connections, connID)
		a.mu.Unlock()
		a.emit("connection:status", StatusEvent{ConnectionID: connID, HostID: host.ID, Status: StatusError, Error: cs.lastError})
		return ConnectionInfo{ConnectionID: connID, HostID: host.ID, HostLabel: host.Label, Status: StatusError, Error: cs.lastError}, err
	}

	var fileClient *sftppkg.Client
	if host.Protocol == storage.HostProtocolSFTP {
		fileClient, err = sftppkg.New(client.Client)
		if err != nil {
			_ = client.Close()
			a.mu.Lock()
			cs.status = StatusError
			cs.lastError = safeErrorMessage(err)
			delete(a.connections, connID)
			a.mu.Unlock()
			a.emit("connection:status", StatusEvent{ConnectionID: connID, HostID: host.ID, Status: StatusError, Error: cs.lastError})
			return ConnectionInfo{}, err
		}
	}

	a.mu.Lock()
	cs.client = client
	cs.sftp = fileClient
	cs.status = StatusConnected
	a.mu.Unlock()

	if !ephemeral {
		_ = a.store.TouchLastUsed(host.ID)
		a.refreshTray()
	}
	go a.runKeepalive(cs)

	var tab TabInfo
	if host.Protocol == storage.HostProtocolSFTP {
		tab = a.openSFTPTab(cs, host.Label)
	} else {
		tab, err = a.openTab(cs, cols, rows, fmt.Sprintf("%s — 1", host.Label))
	}
	if err != nil {
		a.mu.Lock()
		delete(a.connections, connID)
		a.mu.Unlock()
		a.closeConnection(cs)
		a.emit("connection:status", StatusEvent{ConnectionID: connID, HostID: host.ID, Status: StatusError, Error: safeErrorMessage(err)})
		return ConnectionInfo{}, err
	}

	a.emit("connection:status", StatusEvent{ConnectionID: connID, HostID: host.ID, Status: StatusConnected})
	a.autoStartForwards(connID, host)
	return ConnectionInfo{
		ConnectionID:   connID,
		HostID:         host.ID,
		HostLabel:      host.Label,
		Status:         StatusConnected,
		ServerVersion:  client.ServerVersion,
		AuthMethod:     client.AuthMethod,
		Protocol:       protocolOrSSH(host.Protocol),
		InitialTabID:   tab.TabID,
		InitialTabKind: tab.Kind,
		Warning:        client.AgentForwardWarning,
	}, nil
}

func (a *App) dial(host storage.Host, allowRemember bool) (*sshpkg.Client, error) {
	// User, auth method and identity files come from a referenced credential
	// when the host has one, otherwise from the host's own inline fields.
	auth := a.resolveAuth(host)
	return sshpkg.Connect(sshpkg.ConnectConfig{
		Hostname:           host.Hostname,
		Port:               host.Port,
		User:               auth.user,
		IdentityFiles:      auth.identityFiles,
		WantPassword:       auth.authMethod == storage.AuthMethodPassword,
		ForwardAgent:       host.ForwardAgent,
		ProxyJump:          host.ProxyJump,
		KnownHostsPath:     a.knownHostsPath,
		ApproveHostKey:     a.approveHostKey,
		ProvidePassword:    a.providePassword(auth, allowRemember),
		ProvidePassphrase:  a.providePassphrase(),
		ProvideInteractive: a.provideKeyboardInteractive(),
	})
}

// runKeepalive periodically measures round-trip latency with an SSH global
// request (servers reply "request not known" but the reply itself is enough
// to time the round trip) and pushes updates to the status bar.
func (a *App) runKeepalive(cs *connectionState) {
	a.mu.Lock()
	client := cs.client
	stop := cs.stopKeepalive
	a.mu.Unlock()
	if client == nil || stop == nil {
		return
	}

	ticker := time.NewTicker(keepaliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			start := time.Now()
			_, _, err := client.SendRequest("keepalive@ssh-first", true, nil)
			if err != nil {
				a.mu.Lock()
				cs.status = StatusError
				cs.lastError = "connection lost"
				a.mu.Unlock()
				a.emit("connection:status", StatusEvent{ConnectionID: cs.id, HostID: cs.hostID, Status: StatusError, Error: "connection lost"})
				if a.notifier != nil {
					a.notifier.Notify("SSH First", fmt.Sprintf("Connection to %s lost", cs.hostLabel))
				}
				return
			}
			latency := time.Since(start).Milliseconds()

			a.mu.Lock()
			cs.latencyMs = latency
			status := cs.status
			a.mu.Unlock()

			a.emit("connection:status", StatusEvent{
				ConnectionID: cs.id,
				HostID:       cs.hostID,
				Status:       status,
				LatencyMs:    latency,
			})
		}
	}
}

// NewTerminalTab opens an additional terminal tab on an already-established connection.
func (a *App) NewTerminalTab(connectionID string, cols, rows int) (TabInfo, error) {
	a.mu.Lock()
	cs, ok := a.connections[connectionID]
	ready := ok && cs.status == StatusConnected && cs.client != nil
	a.mu.Unlock()
	if !ok {
		return TabInfo{}, fmt.Errorf("no such connection %q", connectionID)
	}
	if !ready {
		return TabInfo{}, fmt.Errorf("connection %q is not ready", connectionID)
	}
	if cs.host.Protocol == storage.HostProtocolSFTP {
		return TabInfo{}, fmt.Errorf("SFTP connections use a single file-browser tab")
	}
	return a.openTab(cs, cols, rows, fmt.Sprintf("Session %d", len(cs.tabOrder)+1))
}

func (a *App) openTab(cs *connectionState, cols, rows int, title string) (TabInfo, error) {
	tabID := uuid.NewString()
	generation := uuid.NewString()
	session, err := a.startTerminalSession(cs.client, tabID, generation, cols, rows)
	if err != nil {
		return TabInfo{}, fmt.Errorf("start terminal session: %w", err)
	}

	ts := &tabState{
		id:                tabID,
		connectionID:      cs.id,
		title:             title,
		kind:              TabKindTerminal,
		session:           session,
		sessionGeneration: generation,
		cols:              cols,
		rows:              rows,
	}
	a.mu.Lock()
	a.tabs[tabID] = ts
	cs.tabOrder = append(cs.tabOrder, tabID)
	a.mu.Unlock()

	a.runLoginScript(cs.host, session)

	return TabInfo{TabID: tabID, ConnectionID: cs.id, Title: title, Kind: TabKindTerminal}, nil
}

func (a *App) openSFTPTab(cs *connectionState, title string) TabInfo {
	tabID := uuid.NewString()
	ts := &tabState{id: tabID, connectionID: cs.id, title: title, kind: TabKindSFTP}
	a.mu.Lock()
	a.tabs[tabID] = ts
	cs.tabOrder = append(cs.tabOrder, tabID)
	a.mu.Unlock()
	return TabInfo{TabID: tabID, ConnectionID: cs.id, Title: title, Kind: TabKindSFTP}
}

// startTerminalSession creates a PTY session whose close callback only acts on
// the exact session generation that is still installed for tabID. Reconnects
// deliberately replace sessions under the same tab ID; a late close callback
// from the old generation must not delete or visually close the new one.
func (a *App) startTerminalSession(client *sshpkg.Client, tabID, generation string, cols, rows int) (*terminalpkg.Session, error) {
	started, err := terminalpkg.Start(client.Client, terminalpkg.Options{
		Cols:         cols,
		Rows:         rows,
		ForwardAgent: client.AgentForwarding,
		OnOutput: func(data []byte) {
			a.emit("terminal:data", TerminalDataEvent{TabID: tabID, Data: string(data)})
		},
		OnClosed: func(closeErr error) {
			a.mu.Lock()
			current, ok := a.tabs[tabID]
			isCurrentGeneration := ok && current.sessionGeneration == generation
			if isCurrentGeneration {
				// Keep the tab metadata so a later reconnect can restore this
				// terminal under the same ID and preserve the frontend scrollback.
				current.session = nil
				current.sessionGeneration = ""
			}
			a.mu.Unlock()
			if !isCurrentGeneration {
				return
			}

			msg := ""
			if closeErr != nil {
				msg = safeErrorMessage(closeErr)
			}
			a.emit("terminal:closed", TerminalClosedEvent{TabID: tabID, Error: msg})
		},
	})
	if err != nil {
		return nil, err
	}
	return started, nil
}

// runLoginScript sends the host's optional login script into a freshly opened
// session. It runs after a short delay so the remote shell has printed its
// prompt first, and each non-empty line is sent as its own command.
func (a *App) runLoginScript(host storage.Host, session *terminalpkg.Session) {
	if strings.TrimSpace(host.LoginScript) == "" {
		return
	}
	script := host.LoginScript
	go func() {
		time.Sleep(400 * time.Millisecond)
		for _, line := range strings.Split(script, "\n") {
			line = strings.TrimRight(line, "\r")
			if strings.TrimSpace(line) == "" {
				continue
			}
			if err := session.Write([]byte(line + "\n")); err != nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()
}

// CloseTab ends one terminal tab without affecting the rest of the connection.
func (a *App) CloseTab(tabID string) error {
	a.mu.Lock()
	ts, ok := a.tabs[tabID]
	if ok {
		delete(a.tabs, tabID)
		if cs, ok := a.connections[ts.connectionID]; ok {
			cs.tabOrder = removeString(cs.tabOrder, tabID)
		}
	}
	a.mu.Unlock()
	if !ok {
		return nil
	}
	if ts.session == nil {
		return nil
	}
	return ts.session.Close()
}

// WriteTerminalInput forwards xterm.js keystrokes/paste data to the remote shell.
func (a *App) WriteTerminalInput(tabID, data string) error {
	a.mu.Lock()
	ts, ok := a.tabs[tabID]
	a.mu.Unlock()
	if !ok {
		return fmt.Errorf("no such terminal tab %q", tabID)
	}
	if ts.kind != TabKindTerminal {
		return fmt.Errorf("tab %q is not a terminal", tabID)
	}
	if ts.session == nil {
		return fmt.Errorf("terminal tab %q is reconnecting", tabID)
	}
	return ts.session.Write([]byte(data))
}

// ResizeTerminal propagates an xterm.js viewport resize to the remote PTY.
func (a *App) ResizeTerminal(tabID string, cols, rows int) error {
	a.mu.Lock()
	ts, ok := a.tabs[tabID]
	if ok {
		ts.cols, ts.rows = cols, rows
	}
	a.mu.Unlock()
	if !ok {
		return fmt.Errorf("no such terminal tab %q", tabID)
	}
	if ts.kind != TabKindTerminal {
		return fmt.Errorf("tab %q is not a terminal", tabID)
	}
	if ts.session == nil {
		// Retain the latest viewport size for the replacement PTY. A resize
		// during reconnect is expected and does not need to fail in the UI.
		return nil
	}
	return ts.session.Resize(cols, rows)
}

// Disconnect closes an SSH connection and every terminal tab on it.
func (a *App) Disconnect(connectionID string) error {
	a.mu.Lock()
	cs, ok := a.connections[connectionID]
	delete(a.connections, connectionID)
	a.mu.Unlock()
	if !ok {
		return fmt.Errorf("no such connection %q", connectionID)
	}
	a.closeConnection(cs)
	a.emit("connection:status", StatusEvent{ConnectionID: connectionID, HostID: cs.hostID, Status: StatusDisconnected})
	return nil
}

func (a *App) closeConnection(cs *connectionState) {
	a.mu.Lock()
	sessions := make([]*terminalpkg.Session, 0, len(cs.tabOrder))
	for _, id := range cs.tabOrder {
		if ts, ok := a.tabs[id]; ok && ts.session != nil {
			sessions = append(sessions, ts.session)
		}
		delete(a.tabs, id)
	}
	cs.tabOrder = nil
	client := cs.client
	cs.client = nil
	fileClient := cs.sftp
	cs.sftp = nil
	stopKeepalive := cs.stopKeepalive
	cs.stopKeepalive = nil
	forwards := cs.forwards
	cs.forwards = make(map[int64]*activeForward)
	a.mu.Unlock()

	if stopKeepalive != nil {
		close(stopKeepalive)
	}
	// Close port forwards before the underlying client: each Tunnel.Close is
	// synchronous and force-closes its in-flight connections, so no forwarding
	// goroutine outlives the connection.
	for _, af := range forwards {
		_ = af.tunnel.Close()
	}
	for _, session := range sessions {
		_ = session.Close()
	}
	if fileClient != nil {
		_ = fileClient.Close()
	}
	if client != nil {
		_ = client.Close()
	}
}

// Reconnect tears down and re-establishes an SSH connection, restarting a
// fresh shell in every tab that was open beforehand (same tab IDs and titles,
// so the frontend's tab strip doesn't need to change).
func (a *App) Reconnect(connectionID string) (ConnectionInfo, error) {
	a.mu.Lock()
	cs, ok := a.connections[connectionID]
	a.mu.Unlock()
	if !ok {
		return ConnectionInfo{}, fmt.Errorf("no such connection %q", connectionID)
	}

	host := cs.host

	type reconnectTab struct {
		id         string
		title      string
		cols, rows int
		oldSession *terminalpkg.Session
		kind       TabKind
	}
	previousTabs := make([]reconnectTab, 0, len(cs.tabOrder))
	a.mu.Lock()
	for _, id := range cs.tabOrder {
		if ts, ok := a.tabs[id]; ok {
			previousTabs = append(previousTabs, reconnectTab{
				id:         ts.id,
				title:      ts.title,
				cols:       ts.cols,
				rows:       ts.rows,
				oldSession: ts.session,
				kind:       ts.kind,
			})
			// Detach before Close so the old generation's OnClosed callback is
			// ignored rather than written into the terminal during reconnect.
			ts.session = nil
			ts.sessionGeneration = ""
		}
	}
	oldClient := cs.client
	oldFileClient := cs.sftp
	oldStopKeepalive := cs.stopKeepalive
	oldForwards := cs.forwards
	reopenForwards := make([]int64, 0, len(oldForwards))
	for id := range oldForwards {
		reopenForwards = append(reopenForwards, id)
	}
	cs.client = nil
	cs.sftp = nil
	cs.stopKeepalive = nil
	cs.forwards = make(map[int64]*activeForward)
	cs.status = StatusConnecting
	cs.lastError = ""
	a.mu.Unlock()

	if oldStopKeepalive != nil {
		close(oldStopKeepalive)
	}
	// Port forwards are bound to the old client; tear them down now and reopen
	// the ones that were active once the new connection is in place.
	for _, af := range oldForwards {
		_ = af.tunnel.Close()
	}
	for _, previous := range previousTabs {
		if previous.oldSession != nil {
			_ = previous.oldSession.Close()
		}
	}
	if oldFileClient != nil {
		_ = oldFileClient.Close()
	}
	if oldClient != nil {
		_ = oldClient.Close()
	}
	a.emit("connection:status", StatusEvent{ConnectionID: connectionID, HostID: cs.hostID, Status: StatusConnecting})

	client, err := a.dial(host, !cs.ephemeral)
	if err != nil {
		a.mu.Lock()
		cs.status = StatusError
		cs.lastError = safeErrorMessage(err)
		a.mu.Unlock()
		a.emit("connection:status", StatusEvent{ConnectionID: connectionID, HostID: cs.hostID, Status: StatusError, Error: cs.lastError})
		return ConnectionInfo{}, err
	}

	var fileClient *sftppkg.Client
	if host.Protocol == storage.HostProtocolSFTP {
		fileClient, err = sftppkg.New(client.Client)
		if err != nil {
			_ = client.Close()
			a.mu.Lock()
			cs.status = StatusError
			cs.lastError = safeErrorMessage(err)
			a.mu.Unlock()
			a.emit("connection:status", StatusEvent{ConnectionID: connectionID, HostID: cs.hostID, Status: StatusError, Error: cs.lastError})
			return ConnectionInfo{}, err
		}
	}

	type replacementSession struct {
		session    *terminalpkg.Session
		generation string
	}
	replacements := make(map[string]replacementSession, len(previousTabs))
	for _, prev := range previousTabs {
		if prev.kind != TabKindTerminal {
			continue
		}
		generation := uuid.NewString()
		session, err := a.startTerminalSession(client, prev.id, generation, prev.cols, prev.rows)
		if err != nil {
			for _, replacement := range replacements {
				_ = replacement.session.Close()
			}
			if fileClient != nil {
				_ = fileClient.Close()
			}
			_ = client.Close()
			reconnectErr := fmt.Errorf("restart terminal session: %w", err)
			a.mu.Lock()
			cs.status = StatusError
			cs.lastError = safeErrorMessage(reconnectErr)
			a.mu.Unlock()
			a.emit("connection:status", StatusEvent{ConnectionID: connectionID, HostID: cs.hostID, Status: StatusError, Error: cs.lastError})
			return ConnectionInfo{}, reconnectErr
		}
		replacements[prev.id] = replacementSession{session: session, generation: generation}
	}

	a.mu.Lock()
	current, stillConnected := a.connections[connectionID]
	if stillConnected && current == cs {
		for _, prev := range previousTabs {
			if prev.kind != TabKindTerminal {
				continue
			}
			if ts, ok := a.tabs[prev.id]; ok {
				replacement := replacements[prev.id]
				ts.session = replacement.session
				ts.sessionGeneration = replacement.generation
			}
		}
		cs.client = client
		cs.sftp = fileClient
		cs.status = StatusConnected
		cs.stopKeepalive = make(chan struct{})
	}
	a.mu.Unlock()
	if !stillConnected || current != cs {
		for _, replacement := range replacements {
			_ = replacement.session.Close()
		}
		if fileClient != nil {
			_ = fileClient.Close()
		}
		_ = client.Close()
		return ConnectionInfo{}, fmt.Errorf("connection %q was closed while reconnecting", connectionID)
	}

	go a.runKeepalive(cs)
	a.restartForwards(connectionID, reopenForwards)
	initialTabID := ""
	if len(previousTabs) > 0 {
		initialTabID = previousTabs[0].id
	}
	a.emit("connection:status", StatusEvent{ConnectionID: connectionID, HostID: cs.hostID, Status: StatusConnected})
	return ConnectionInfo{
		ConnectionID:  connectionID,
		HostID:        cs.hostID,
		HostLabel:     cs.hostLabel,
		Status:        StatusConnected,
		ServerVersion: client.ServerVersion,
		AuthMethod:    client.AuthMethod,
		Protocol:      protocolOrSSH(host.Protocol),
		Warning:       client.AgentForwardWarning,
		InitialTabID:  initialTabID,
		InitialTabKind: func() TabKind {
			if host.Protocol == storage.HostProtocolSFTP {
				return TabKindSFTP
			}
			return TabKindTerminal
		}(),
	}, nil
}

// parseQuickTarget accepts user@host, user@host:port, host:port and a compact
// `ssh [-p port] target` form. It intentionally does not interpret arbitrary
// shell syntax or options.
func parseQuickTarget(value string) (storage.Host, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return storage.Host{}, fmt.Errorf("enter a hostname")
	}

	port := 22
	target := value
	fields := strings.Fields(value)
	if len(fields) > 0 && fields[0] == "ssh" {
		target = ""
		for i := 1; i < len(fields); i++ {
			switch fields[i] {
			case "-p":
				if i+1 >= len(fields) {
					return storage.Host{}, fmt.Errorf("ssh -p requires a port")
				}
				i++
				parsed, err := strconv.Atoi(fields[i])
				if err != nil {
					return storage.Host{}, fmt.Errorf("invalid port %q", fields[i])
				}
				port = parsed
			default:
				if strings.HasPrefix(fields[i], "-") {
					return storage.Host{}, fmt.Errorf("unsupported quick-connect option %q", fields[i])
				}
				if target != "" {
					return storage.Host{}, fmt.Errorf("enter a single SSH target")
				}
				target = fields[i]
			}
		}
	} else if len(fields) != 1 {
		return storage.Host{}, fmt.Errorf("enter a target like user@host")
	}
	if target == "" {
		return storage.Host{}, fmt.Errorf("enter a hostname")
	}

	login := ""
	if at := strings.LastIndex(target, "@"); at >= 0 {
		login = strings.TrimSpace(target[:at])
		target = target[at+1:]
	}

	hostname := target
	if host, portText, err := net.SplitHostPort(target); err == nil {
		hostname = host
		parsed, parseErr := strconv.Atoi(portText)
		if parseErr != nil {
			return storage.Host{}, fmt.Errorf("invalid port %q", portText)
		}
		port = parsed
	} else if strings.Count(target, ":") == 1 {
		parts := strings.SplitN(target, ":", 2)
		parsed, parseErr := strconv.Atoi(parts[1])
		if parseErr != nil {
			return storage.Host{}, fmt.Errorf("invalid port %q", parts[1])
		}
		hostname = parts[0]
		port = parsed
	} else if strings.HasPrefix(target, "[") && strings.HasSuffix(target, "]") {
		hostname = strings.TrimSuffix(strings.TrimPrefix(target, "["), "]")
	}

	if hostname == "" {
		return storage.Host{}, fmt.Errorf("enter a hostname")
	}
	if port < 1 || port > 65535 {
		return storage.Host{}, fmt.Errorf("port must be between 1 and 65535")
	}
	if login == "" {
		if current, err := osuser.Current(); err == nil {
			login = current.Username
		}
		if login == "" {
			login = os.Getenv("USER")
		}
	}
	if login == "" {
		return storage.Host{}, fmt.Errorf("could not determine the SSH user; enter user@host")
	}

	label := fmt.Sprintf("%s@%s", login, hostname)
	if port != 22 {
		label = fmt.Sprintf("%s:%d", label, port)
	}
	return storage.Host{
		ID:         0,
		Label:      label,
		Hostname:   hostname,
		Port:       port,
		User:       login,
		AuthMethod: storage.AuthMethodAgent,
		Protocol:   storage.HostProtocolSSH,
		RemotePath: ".",
		Source:     storage.HostSourceManual,
	}, nil
}

func protocolOrSSH(protocol storage.HostProtocol) storage.HostProtocol {
	if protocol == storage.HostProtocolSFTP {
		return protocol
	}
	return storage.HostProtocolSSH
}

func removeString(list []string, target string) []string {
	out := list[:0]
	for _, s := range list {
		if s != target {
			out = append(out, s)
		}
	}
	return out
}

// safeErrorMessage returns an error's text for display in the UI. All errors
// surfaced here originate from internal/ssh, which never embeds secret
// material (passwords/passphrases) in error strings.
func safeErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
