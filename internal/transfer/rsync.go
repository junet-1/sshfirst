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
	"sync"
)

// Config describes a single rsync run.
type Config struct {
	User           string
	Hostname       string
	Port           int
	IdentityFiles  []string
	ProxyJump      string
	KnownHostsPath string

	LocalPath  string
	RemotePath string
	Upload     bool // true: local → remote; false: remote → local

	Archive  bool
	Compress bool
	Delete   bool // --delete (mirror; removes extraneous files on the destination)
	DryRun   bool

	// LegacyProgress selects flags an rsync without --info=progress2
	// understands. See SupportsInfoProgress, which is what callers should use
	// to fill this in.
	LegacyProgress bool
}

// ErrRsyncMissing is returned when the rsync binary isn't on PATH.
var ErrRsyncMissing = fmt.Errorf("rsync is not installed or not on PATH")

var (
	infoProgressOnce sync.Once
	infoProgress     bool
)

// SupportsInfoProgress reports whether the rsync on PATH understands
// --info=progress2, which arrived in rsync 3.1 (2013). Every current Linux
// distribution ships a newer one, but macOS still bundles rsync 2.6.9 — and
// since macOS 15 the openrsync rewrite — so the flag has to be probed instead
// of assumed. The answer is cached: rsync does not change under a running app.
func SupportsInfoProgress() bool {
	infoProgressOnce.Do(func() {
		out, err := exec.Command("rsync", "--version").Output()
		infoProgress = err == nil && versionSupportsInfoProgress(string(out))
	})
	return infoProgress
}

// versionSupportsInfoProgress parses the first line of `rsync --version`, which
// reads "rsync  version 3.4.1  protocol version 32" for rsync proper and starts
// with "openrsync" for the BSD/macOS rewrite.
func versionSupportsInfoProgress(output string) bool {
	line, _, _ := strings.Cut(output, "\n")
	if strings.Contains(strings.ToLower(line), "openrsync") {
		return false
	}
	_, rest, found := strings.Cut(line, "version ")
	if !found {
		return false
	}
	version, _, _ := strings.Cut(rest, " ")
	major, minor, _ := strings.Cut(version, ".")
	majorNum, err := strconv.Atoi(major)
	if err != nil {
		return false
	}
	if majorNum != 3 {
		return majorNum > 3
	}
	minorNum, err := strconv.Atoi(strings.SplitN(minor, ".", 2)[0])
	return err == nil && minorNum >= 1
}

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
	if cfg.LegacyProgress {
		// --progress is per-file rather than one overall line, but it is all
		// rsync 2.6.9 and openrsync (both shipped by macOS) understand.
		args = append(args, "--progress")
	} else {
		// Human-readable sizes and a single overall progress line (rsync 3.1+).
		args = append(args, "-h", "--info=progress2")
	}

	// The remote-shell command, assembled as separate tokens and quoted by
	// joinRemoteShell below.
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
		// StrictHostKeyChecking=yes, not accept-new: this is the same
		// known_hosts the interactive client verifies against, so accepting an
		// unknown key here would both skip the approval dialog and pin whatever
		// answered — poisoning every later terminal session to that host. A
		// transfer to a host that has never been connected fails instead; the
		// caller checks for that up front and says so (see internal/app).
		ssh = append(ssh, "-o", "UserKnownHostsFile="+cfg.KnownHostsPath, "-o", "StrictHostKeyChecking=yes")
	}
	// Never block on an interactive password/passphrase prompt (there's no tty).
	ssh = append(ssh, "-o", "BatchMode=yes")
	remoteShell, err := joinRemoteShell(ssh)
	if err != nil {
		return nil, err
	}
	args = append(args, "-e", remoteShell)

	remote := cfg.Hostname
	if cfg.User != "" {
		remote = cfg.User + "@" + cfg.Hostname
	}
	remoteSpec := remote + ":" + cfg.RemotePath

	// "--" so a path that happens to start with a dash is a path, not an option
	// like --rsync-path=<command>.
	args = append(args, "--")
	if cfg.Upload {
		args = append(args, cfg.LocalPath, remoteSpec)
	} else {
		args = append(args, remoteSpec, cfg.LocalPath)
	}
	return args, nil
}

// joinRemoteShell renders the ssh invocation for rsync's -e option.
//
// rsync splits that string on whitespace, but it does honour double quotes
// (verified against rsync 3.4.4), and quoting is what makes two things safe:
// a path containing a space survives as one argument — every macOS data dir is
// under "Application Support" — and a host field containing a space can no
// longer smuggle in extra ssh options such as -o ProxyCommand=<command>.
//
// A token containing a double quote has no representation here, so it is
// refused rather than mangled into something with a different meaning.
func joinRemoteShell(tokens []string) (string, error) {
	quoted := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if strings.Contains(token, `"`) {
			return "", fmt.Errorf("cannot pass %q to rsync: a double quote in an ssh option is not supported", token)
		}
		if strings.ContainsAny(token, " \t\n") {
			token = `"` + token + `"`
		}
		quoted = append(quoted, token)
	}
	return strings.Join(quoted, " "), nil
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
