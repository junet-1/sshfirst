// Package transfer runs file transfers with the system rsync binary over SSH.
// rsync spawns its own ssh process, so it authenticates via the user's
// ssh-agent / identity files (not SSH First's Secret Service store); this is
// far faster than a re-implemented SFTP copy for large/again-synced trees.
package transfer

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Config describes a single rsync run.
type Config struct {
	User          string
	Hostname      string
	Port          int
	IdentityFiles []string
	ProxyJump     string
	KnownHostsPath string

	LocalPath  string
	RemotePath string
	Upload     bool // true: local → remote; false: remote → local

	Archive  bool
	Compress bool
	Delete   bool // --delete (mirror; removes extraneous files on the destination)
	DryRun   bool
}

// ErrRsyncMissing is returned when the rsync binary isn't on PATH.
var ErrRsyncMissing = fmt.Errorf("rsync is not installed or not on PATH")

// BuildArgs assembles the rsync argument list (excluding the "rsync" program
// name itself) for cfg.
func BuildArgs(cfg Config) ([]string, error) {
	if cfg.Hostname == "" {
		return nil, fmt.Errorf("hostname is required")
	}
	if cfg.LocalPath == "" || cfg.RemotePath == "" {
		return nil, fmt.Errorf("both a local and a remote path are required")
	}

	args := []string{}
	if cfg.Archive {
		args = append(args, "-a")
	} else {
		args = append(args, "-r")
	}
	if cfg.Compress {
		args = append(args, "-z")
	}
	if cfg.Delete {
		args = append(args, "--delete")
	}
	if cfg.DryRun {
		args = append(args, "-n")
	}
	// Human-readable sizes and a single overall progress line (rsync 3.1+).
	args = append(args, "-h", "--info=progress2")

	// The remote-shell command. rsync splits this string on spaces itself, so
	// paths with spaces in identity/known_hosts files are not supported here.
	ssh := []string{"ssh"}
	if cfg.Port != 0 && cfg.Port != 22 {
		ssh = append(ssh, "-p", strconv.Itoa(cfg.Port))
	}
	for _, id := range cfg.IdentityFiles {
		if id != "" {
			ssh = append(ssh, "-i", id)
		}
	}
	if len(cfg.IdentityFiles) > 0 {
		ssh = append(ssh, "-o", "IdentitiesOnly=yes")
	}
	if cfg.ProxyJump != "" {
		ssh = append(ssh, "-o", "ProxyJump="+cfg.ProxyJump)
	}
	if cfg.KnownHostsPath != "" {
		ssh = append(ssh, "-o", "UserKnownHostsFile="+cfg.KnownHostsPath, "-o", "StrictHostKeyChecking=accept-new")
	}
	// Never block on an interactive password/passphrase prompt (there's no tty).
	ssh = append(ssh, "-o", "BatchMode=yes")
	args = append(args, "-e", strings.Join(ssh, " "))

	remote := cfg.Hostname
	if cfg.User != "" {
		remote = cfg.User + "@" + cfg.Hostname
	}
	remoteSpec := remote + ":" + cfg.RemotePath

	if cfg.Upload {
		args = append(args, cfg.LocalPath, remoteSpec)
	} else {
		args = append(args, remoteSpec, cfg.LocalPath)
	}
	return args, nil
}

// Run executes rsync with args, streaming combined stdout/stderr to onLine.
// A segment terminated by '\r' (progress updates) is reported with
// transient=true; one terminated by '\n' with transient=false. Cancelling
// ctx kills the process. Returns the rsync exit error (nil on success).
func Run(ctx context.Context, args []string, onLine func(text string, transient bool)) error {
	if _, err := exec.LookPath("rsync"); err != nil {
		return ErrRsyncMissing
	}

	pr, pw, err := os.Pipe()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "rsync", args...)
	cmd.Stdout = pw
	cmd.Stderr = pw // combine both streams into one pipe

	if err := cmd.Start(); err != nil {
		pr.Close()
		pw.Close()
		return err
	}
	pw.Close() // parent's write end; the child keeps its own so pr sees EOF on exit

	done := make(chan struct{})
	go func() {
		streamSegments(bufio.NewReader(pr), onLine)
		close(done)
	}()
	<-done
	pr.Close()

	return cmd.Wait()
}

// streamSegments reads r and calls onLine for each '\n'- or '\r'-terminated
// segment, so rsync's carriage-return progress updates render as a single
// live line in the UI rather than flooding the log.
func streamSegments(r io.ByteReader, onLine func(text string, transient bool)) {
	var buf []byte
	for {
		c, err := r.ReadByte()
		if err != nil {
			if len(buf) > 0 {
				onLine(string(buf), false)
			}
			return
		}
		switch c {
		case '\n':
			onLine(string(buf), false)
			buf = buf[:0]
		case '\r':
			onLine(string(buf), true)
			buf = buf[:0]
		default:
			buf = append(buf, c)
		}
	}
}
