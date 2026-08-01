package app

import (
	"fmt"

	sftppkg "ssh-first/internal/sftp"
)

// ListSFTP lists one remote directory for a file-browser tab. The canonical
// path is returned so the address bar follows the server's working directory.
func (a *App) ListSFTP(tabID, remotePath string) (SFTPDirectory, error) {
	client, err := a.sftpClientForTab(tabID)
	if err != nil {
		return SFTPDirectory{}, err
	}
	resolved := client.Resolve(remotePath)
	entries, err := client.ReadDir(resolved)
	if err != nil {
		return SFTPDirectory{}, err
	}
	return SFTPDirectory{Path: resolved, Entries: entries}, nil
}

func (a *App) UploadSFTP(tabID, localPath, remoteDir string) (sftppkg.Entry, error) {
	client, err := a.sftpClientForTab(tabID)
	if err != nil {
		return sftppkg.Entry{}, err
	}
	if localPath == "" {
		return sftppkg.Entry{}, fmt.Errorf("no local file selected")
	}
	return client.Upload(localPath, remoteDir)
}

func (a *App) DownloadSFTP(tabID, remotePath, localDir string) (string, error) {
	client, err := a.sftpClientForTab(tabID)
	if err != nil {
		return "", err
	}
	if localDir == "" {
		return "", fmt.Errorf("no local destination selected")
	}
	return client.Download(remotePath, localDir)
}

func (a *App) sftpClientForTab(tabID string) (*sftppkg.Client, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	ts, ok := a.tabs[tabID]
	if !ok || ts.kind != TabKindSFTP {
		return nil, fmt.Errorf("no such SFTP tab %q", tabID)
	}
	cs, ok := a.connections[ts.connectionID]
	if !ok || cs.status != StatusConnected || cs.sftp == nil {
		return nil, fmt.Errorf("SFTP connection is not ready")
	}
	return cs.sftp, nil
}
