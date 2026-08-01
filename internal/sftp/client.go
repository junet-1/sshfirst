// Package sftp exposes filesystem operations over an already authenticated
// SSH connection. Authentication and host-key verification deliberately stay
// in internal/ssh; this package only owns the SFTP subsystem and file I/O.
package sftp

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	pkgSFTP "github.com/pkg/sftp"
	xssh "golang.org/x/crypto/ssh"
)

// Entry is the JSON-safe representation of a remote directory entry.
type Entry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	IsDir      bool   `json:"isDir"`
	IsSymlink  bool   `json:"isSymlink"`
	Size       int64  `json:"size"`
	Mode       string `json:"mode"`
	ModifiedAt string `json:"modifiedAt"`
}

// Client wraps the SFTP subsystem. Operations are serialized because a user
// can trigger refresh and transfer actions very quickly from the file pane.
type Client struct {
	client *pkgSFTP.Client
	mu     sync.Mutex
	home   string
}

// New starts the SFTP subsystem over client without creating a second SSH
// transport. The server-reported working directory becomes the base for
// relative paths such as the default start path ".".
func New(client *xssh.Client) (*Client, error) {
	inner, err := pkgSFTP.NewClient(client)
	if err != nil {
		return nil, fmt.Errorf("start SFTP subsystem: %w", err)
	}
	home, err := inner.Getwd()
	if err != nil {
		_ = inner.Close()
		return nil, fmt.Errorf("read SFTP working directory: %w", err)
	}
	return &Client{client: inner, home: path.Clean(home)}, nil
}

// Resolve turns a configured or user-entered path into a canonical absolute
// remote path. POSIX path handling is intentional even on Windows clients.
func (c *Client) Resolve(remotePath string) string {
	remotePath = strings.TrimSpace(remotePath)
	if remotePath == "" || remotePath == "." {
		return c.home
	}
	if strings.HasPrefix(remotePath, "/") {
		return path.Clean(remotePath)
	}
	return path.Clean(path.Join(c.home, remotePath))
}

// ReadDir returns directories first and then files, both case-insensitively.
func (c *Client) ReadDir(remotePath string) ([]Entry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	resolved := c.Resolve(remotePath)
	infos, err := c.client.ReadDir(resolved)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", resolved, err)
	}
	entries := make([]Entry, 0, len(infos))
	for _, info := range infos {
		entries = append(entries, entryFromInfo(resolved, info))
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	return entries, nil
}

// Upload copies one local file into remoteDir and returns its refreshed
// metadata. Directories are intentionally excluded from this first native
// SFTP surface so partial recursive transfers cannot be mistaken for success.
func (c *Client) Upload(localPath, remoteDir string) (Entry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	source, err := os.Open(localPath)
	if err != nil {
		return Entry{}, fmt.Errorf("open local file: %w", err)
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return Entry{}, fmt.Errorf("inspect local file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Entry{}, fmt.Errorf("only regular files can be uploaded")
	}

	resolvedDir := c.Resolve(remoteDir)
	destinationPath := path.Join(resolvedDir, filepath.Base(localPath))
	destination, err := c.client.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return Entry{}, fmt.Errorf("create %s: %w", destinationPath, err)
	}
	_, copyErr := io.Copy(destination, source)
	closeErr := destination.Close()
	if copyErr != nil {
		_ = c.client.Remove(destinationPath)
		return Entry{}, fmt.Errorf("upload %s: %w", destinationPath, copyErr)
	}
	if closeErr != nil {
		_ = c.client.Remove(destinationPath)
		return Entry{}, fmt.Errorf("finish upload %s: %w", destinationPath, closeErr)
	}
	remoteInfo, err := c.client.Stat(destinationPath)
	if err != nil {
		return Entry{}, fmt.Errorf("inspect uploaded file: %w", err)
	}
	return entryFromInfo(resolvedDir, remoteInfo), nil
}

// Download copies one remote regular file into localDir and returns the full
// local destination path.
func (c *Client) Download(remotePath, localDir string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	resolved := c.Resolve(remotePath)
	source, err := c.client.Open(resolved)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", resolved, err)
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return "", fmt.Errorf("inspect %s: %w", resolved, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("only regular files can be downloaded")
	}

	destinationPath := filepath.Join(localDir, path.Base(resolved))
	destination, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("create local file: %w", err)
	}
	_, copyErr := io.Copy(destination, source)
	closeErr := destination.Close()
	if copyErr != nil {
		_ = os.Remove(destinationPath)
		return "", fmt.Errorf("download %s: %w", resolved, copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(destinationPath)
		return "", fmt.Errorf("finish local file: %w", closeErr)
	}
	return destinationPath, nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client.Close()
}

func entryFromInfo(parent string, info os.FileInfo) Entry {
	return Entry{
		Name:       info.Name(),
		Path:       path.Join(parent, info.Name()),
		IsDir:      info.IsDir(),
		IsSymlink:  info.Mode()&os.ModeSymlink != 0,
		Size:       info.Size(),
		Mode:       info.Mode().String(),
		ModifiedAt: info.ModTime().UTC().Format(time.RFC3339),
	}
}
