//go:build darwin

package app

import (
	"os"
	"path/filepath"
)

// dataDir returns ~/Library/Application Support/ssh-first, where macOS expects
// application state to live. XDG_DATA_HOME is still honoured when set, so a
// user who deliberately exports it (or migrates a directory over from Linux)
// keeps a single location.
func dataDir() (string, error) {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "ssh-first"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Application Support", "ssh-first"), nil
}
