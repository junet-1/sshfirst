package app

// DataDir is the exported accessor for the per-user data directory, used by the
// composition root (main) to place side files such as the WebKit cookie store
// next to the database.
//
// The directory itself is platform-specific — see paths_darwin.go and
// paths_default.go. It holds the SQLite database, the log file and SSH First's
// own managed known_hosts file (never the user's ~/.ssh).
func DataDir() (string, error) {
	return dataDir()
}
