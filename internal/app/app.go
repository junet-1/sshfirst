// Package app wires storage, config import, secrets, SSH connections and
// terminal sessions together behind the methods Wails binds to the frontend.
// It holds Wails/runtime concerns (context, event emission); the packages it
// orchestrates know nothing about Wails.
package app

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"

	"ssh-first/internal/platform"
	sftppkg "ssh-first/internal/sftp"
	sshpkg "ssh-first/internal/ssh"
	"ssh-first/internal/storage"
	terminalpkg "ssh-first/internal/terminal"
	workspacepkg "ssh-first/internal/workspace"
)

// App is the Wails-bound application backend.
type App struct {
	ctx context.Context
	ui  *application.App

	windowMu    sync.Mutex
	mainWindow  *application.WebviewWindow
	toolWindows map[string]*application.WebviewWindow
	geom        *geometryStore

	migrations     fs.FS
	store          *storage.Store
	workspaces     *workspacepkg.Store
	knownHostsPath string
	notifier       platform.Notifier
	trayIcon       []byte
	tray           trayService
	trayActive     atomic.Bool
	quitting       atomic.Bool

	mu          sync.Mutex
	connections map[string]*connectionState
	tabs        map[string]*tabState
	output      *terminalOutputBroker

	pending   *pendingPrompts
	transfers *transferManager
}

type connectionState struct {
	id            string
	hostID        int64
	host          storage.Host
	hostLabel     string
	ephemeral     bool
	client        *sshpkg.Client
	sftp          *sftppkg.Client
	status        ConnectionStatus
	lastError     string
	latencyMs     int64
	stopKeepalive chan struct{}
	tabOrder      []string                 // tab IDs in creation order, for Reconnect
	forwards      map[int64]*activeForward // running port forwards, keyed by rule ID
}

type tabState struct {
	id                string
	connectionID      string
	title             string
	kind              TabKind
	session           *terminalpkg.Session
	sessionGeneration string
	cols, rows        int
}

// New constructs an App. migrations is the embedded migrations/ directory
// (see main.go — go:embed cannot reach outside its package directory, so the
// caller at the module root embeds it and hands it in here). Call Startup
// once a Wails context is available.
func New(migrations fs.FS, trayIcon ...[]byte) *App {
	a := &App{
		migrations:  migrations,
		connections: make(map[string]*connectionState),
		tabs:        make(map[string]*tabState),
		pending:     newPendingPrompts(),
		transfers:   newTransferManager(),
		toolWindows: make(map[string]*application.WebviewWindow),
		geom:        newGeometryStore(),
	}
	a.output = newTerminalOutputBroker(a.dispatchTerminalOutput)
	if len(trayIcon) > 0 {
		a.trayIcon = append([]byte(nil), trayIcon[0]...)
	}
	return a
}

// ConfigureApplication provides the UI shell without leaking it into the SSH,
// storage or terminal packages.
//
//wails:ignore
func (a *App) ConfigureApplication(ui *application.App) {
	a.ui = ui
}

//wails:ignore
func (a *App) SetMainWindow(window *application.WebviewWindow) {
	a.mainWindow = window
}

// ServiceStartup is called by Wails before any bound frontend calls are
// accepted.
//
//wails:ignore
func (a *App) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	a.ctx = ctx

	dir, err := dataDir()
	if err != nil {
		return fmt.Errorf("resolve data dir: %w", err)
	}

	// Log to both stderr (visible when run from a terminal) and a file
	// under the data dir (so double-clicked launches are still debuggable).
	// Never logs secret material — see internal/secrets and internal/ssh.
	//
	// The directory has to exist first. storage.Open creates it, but that comes
	// afterwards, so on a first run the log file was silently never created —
	// the one launch where having a log matters most.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	if logFile, err := os.OpenFile(filepath.Join(dir, "ssh-first.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		log.SetOutput(io.MultiWriter(os.Stderr, logFile))
	} else {
		log.Printf("ssh-first: open log file: %v", err)
	}

	store, err := storage.Open(dir, a.migrations)
	if err != nil {
		return fmt.Errorf("open storage: %w", err)
	}
	a.store = store
	a.workspaces = workspacepkg.New(dir)
	a.knownHostsPath = filepath.Join(dir, "known_hosts")
	a.notifier = platform.NewNotifier("SSH First")
	a.startTray()
	return nil
}

// ServiceShutdown closes every open SSH connection cleanly before the process
// exits.
//
//wails:ignore
func (a *App) ServiceShutdown() error {
	a.stopTray()
	a.output.close()

	a.mu.Lock()
	connections := make([]*connectionState, 0, len(a.connections))
	for _, c := range a.connections {
		connections = append(connections, c)
	}
	a.mu.Unlock()

	for _, c := range connections {
		a.closeConnection(c)
	}
	if a.store != nil {
		if err := a.store.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) emit(event string, data any) {
	if a.ui == nil {
		return
	}
	a.ui.Event.Emit(event, data)
}

// dispatchTerminalOutput deliberately targets only the main window. Terminal
// batches are high-frequency and irrelevant to tool windows; using the global
// event manager would create a goroutine and synchronous main-thread ExecJS
// call for every open native window.
func (a *App) dispatchTerminalOutput(event TerminalDataEvent) {
	if a.mainWindow == nil {
		return
	}
	a.mainWindow.DispatchWailsEvent(&application.CustomEvent{Name: "terminal:data", Data: event})
}

func (a *App) requireStore() error {
	if a.store == nil {
		return fmt.Errorf("storage is not available (startup failed, check logs)")
	}
	return nil
}
