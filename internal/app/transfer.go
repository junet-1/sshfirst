package app

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"

	"ssh-first/internal/transfer"
)

// TransferRequest is the payload from the frontend to start an rsync transfer.
type TransferRequest struct {
	HostID     int64  `json:"hostId"`
	LocalPath  string `json:"localPath"`
	RemotePath string `json:"remotePath"`
	Upload     bool   `json:"upload"`
	Archive    bool   `json:"archive"`
	Compress   bool   `json:"compress"`
	Delete     bool   `json:"delete"`
	DryRun     bool   `json:"dryRun"`
}

// TransferOutputEvent streams one line of rsync output. Transient lines are
// progress updates (carriage-return terminated) that should replace the
// previous transient line rather than accumulate.
type TransferOutputEvent struct {
	TransferID string `json:"transferId"`
	Text       string `json:"text"`
	Transient  bool   `json:"transient"`
}

// TransferDoneEvent is emitted once a transfer finishes.
type TransferDoneEvent struct {
	TransferID string `json:"transferId"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
}

type transferHandle struct {
	cancel context.CancelFunc
}

type transferManager struct {
	mu     sync.Mutex
	active map[string]transferHandle
}

func newTransferManager() *transferManager {
	return &transferManager{active: make(map[string]transferHandle)}
}

// PreviewRsyncCommand returns the rsync command line that StartRsync would run,
// so the UI can show the user exactly what will execute.
func (a *App) PreviewRsyncCommand(req TransferRequest) (string, error) {
	cfg, err := a.rsyncConfig(req)
	if err != nil {
		return "", err
	}
	args, err := transfer.BuildArgs(cfg)
	if err != nil {
		return "", err
	}
	return "rsync " + shellQuoteArgs(args), nil
}

// StartRsync launches an rsync transfer and returns its ID. Output and
// completion are delivered via "transfer:output" / "transfer:done" events.
func (a *App) StartRsync(req TransferRequest) (string, error) {
	cfg, err := a.rsyncConfig(req)
	if err != nil {
		return "", err
	}
	args, err := transfer.BuildArgs(cfg)
	if err != nil {
		return "", err
	}

	id := uuid.NewString()
	ctx, cancel := context.WithCancel(context.Background())

	a.transfers.mu.Lock()
	a.transfers.active[id] = transferHandle{cancel: cancel}
	a.transfers.mu.Unlock()

	go func() {
		runErr := transfer.Run(ctx, args, func(text string, transient bool) {
			a.emit("transfer:output", TransferOutputEvent{TransferID: id, Text: text, Transient: transient})
		})

		a.transfers.mu.Lock()
		delete(a.transfers.active, id)
		a.transfers.mu.Unlock()

		evt := TransferDoneEvent{TransferID: id, Success: runErr == nil}
		if runErr != nil {
			evt.Error = runErr.Error()
		}
		a.emit("transfer:done", evt)
	}()

	return id, nil
}

// CancelRsync aborts a running transfer.
func (a *App) CancelRsync(transferID string) error {
	a.transfers.mu.Lock()
	handle, ok := a.transfers.active[transferID]
	a.transfers.mu.Unlock()
	if !ok {
		return nil
	}
	handle.cancel()
	return nil
}

func (a *App) rsyncConfig(req TransferRequest) (transfer.Config, error) {
	if err := a.requireStore(); err != nil {
		return transfer.Config{}, err
	}
	host, err := a.store.GetHost(req.HostID)
	if err != nil {
		return transfer.Config{}, fmt.Errorf("load host: %w", err)
	}
	return transfer.Config{
		User:           host.User,
		Hostname:       host.Hostname,
		Port:           host.Port,
		IdentityFiles:  host.IdentityFiles,
		ProxyJump:      host.ProxyJump,
		KnownHostsPath: a.knownHostsPath,
		LocalPath:      req.LocalPath,
		RemotePath:     req.RemotePath,
		Upload:         req.Upload,
		Archive:        req.Archive,
		Compress:       req.Compress,
		Delete:         req.Delete,
		DryRun:         req.DryRun,
		LegacyProgress: !transfer.SupportsInfoProgress(),
	}, nil
}

// shellQuoteArgs renders args for display, quoting any that contain spaces.
func shellQuoteArgs(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if a == "" || strings.ContainsRune(a, ' ') {
			out[i] = "'" + a + "'"
		} else {
			out[i] = a
		}
	}
	return strings.Join(out, " ")
}
