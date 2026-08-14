//go:build !darwin

package app

import (
	"os"
	"path/filepath"
)

// dataDir returns the XDG-compliant per-user data directory.
func dataDir() (string, error) {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "ssh-first"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "ssh-first"), nil
}
