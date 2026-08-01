package app

import (
	"os"
	"path/filepath"
)

// DataDir is the exported accessor for the per-user data directory, used by the
// composition root (main) to place side files such as the WebKit cookie store
// next to the database.
func DataDir() (string, error) {
	return dataDir()
}

// dataDir returns the XDG-compliant per-user data directory SSH First stores
// its database and managed known_hosts file in (not the user's ~/.ssh).
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
