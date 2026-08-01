package ssh

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// KnownHostStatus classifies why a host key needed explicit approval.
type KnownHostStatus int

const (
	// HostKeyKnown means the presented key matched known_hosts; no prompt needed.
	HostKeyKnown KnownHostStatus = iota
	// HostKeyUnknown means this host has never been seen before.
	HostKeyUnknown
	// HostKeyChanged means a DIFFERENT key was previously recorded for this
	// host — a strong signal of a possible MITM attack or a re-imaged server.
	HostKeyChanged
)

// HostKeyDecisionRequest is the information a user needs to decide whether
// to trust a server's host key.
type HostKeyDecisionRequest struct {
	Hostname             string
	Algorithm            string
	Fingerprint          string
	PreviousFingerprints []string
	Status               KnownHostStatus
}

// HostKeyApprover blocks until the user has approved or rejected an unknown
// or changed host key (typically backed by a frontend confirmation dialog).
type HostKeyApprover func(req HostKeyDecisionRequest) (accept bool, err error)

// ErrHostKeyRejected is returned when the user declines to trust a host key.
var ErrHostKeyRejected = errors.New("host key rejected by user")

// Serialize writes so two simultaneous first connections cannot overwrite
// each other's newly accepted host keys.
var knownHostsMu sync.Mutex

// knownHostsCallback builds an ssh.HostKeyCallback backed by the SSH First
// managed known_hosts file at path (created if missing). Unknown or changed
// keys are routed through approve; accepted keys are persisted back to path.
func knownHostsCallback(path string, approve HostKeyApprover) (ssh.HostKeyCallback, error) {
	if err := ensureFile(path); err != nil {
		return nil, err
	}
	base, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("read known_hosts: %w", err)
	}

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := base(hostname, remote, key)
		if err == nil {
			return nil
		}

		var keyErr *knownhosts.KeyError
		if !errors.As(err, &keyErr) {
			return err
		}

		status := HostKeyUnknown
		var previousFingerprints []string
		if len(keyErr.Want) > 0 {
			status = HostKeyChanged
			seen := make(map[string]struct{}, len(keyErr.Want))
			for _, known := range keyErr.Want {
				fingerprint := ssh.FingerprintSHA256(known.Key)
				if _, exists := seen[fingerprint]; exists {
					continue
				}
				seen[fingerprint] = struct{}{}
				previousFingerprints = append(previousFingerprints, fingerprint)
			}
		}

		req := HostKeyDecisionRequest{
			Hostname:             knownhosts.Normalize(hostname),
			Algorithm:            key.Type(),
			Fingerprint:          ssh.FingerprintSHA256(key),
			PreviousFingerprints: previousFingerprints,
			Status:               status,
		}
		accept, approveErr := approve(req)
		if approveErr != nil {
			return approveErr
		}
		if !accept {
			return ErrHostKeyRejected
		}

		return persistHostKey(path, knownhosts.Normalize(hostname), key)
	}, nil
}

func ensureFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0o600)
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

// persistHostKey appends key for normalizedHost to the known_hosts file at
// path, first removing any stale entry for the same host+algorithm pair (the
// case where the user just approved a CHANGED key).
func persistHostKey(path, normalizedHost string, key ssh.PublicKey) error {
	knownHostsMu.Lock()
	defer knownHostsMu.Unlock()

	if err := ensureFile(path); err != nil {
		return err
	}
	existing, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var kept []string
	scanner := bufio.NewScanner(strings.NewReader(string(existing)))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			hostsField, algo := fields[0], fields[1]
			if algo == key.Type() && hostMatchesField(hostsField, normalizedHost) {
				continue // drop the stale entry for this host+algorithm
			}
		}
		kept = append(kept, line)
	}

	kept = append(kept, knownhosts.Line([]string{normalizedHost}, key))

	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".known_hosts-*")
	if err != nil {
		return err
	}
	tempPath := f.Name()
	defer os.Remove(tempPath)
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.WriteString(strings.Join(kept, "\n") + "\n"); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func hostMatchesField(hostsField, normalizedHost string) bool {
	for _, h := range strings.Split(hostsField, ",") {
		if h == normalizedHost {
			return true
		}
	}
	return false
}
