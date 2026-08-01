//go:build linux

package app

import (
	"bytes"
	"image/png"
	"testing"

	"ssh-first/internal/storage"
)

func TestTrayProtocolIconsAreDistinctPNGs(t *testing.T) {
	sshIcon := trayProtocolIcon(storage.HostProtocolSSH)
	sftpIcon := trayProtocolIcon(storage.HostProtocolSFTP)

	if bytes.Equal(sshIcon, sftpIcon) {
		t.Fatal("SSH and SFTP tray icons must be visually distinct")
	}
	for name, icon := range map[string][]byte{"ssh": sshIcon, "sftp": sftpIcon} {
		decoded, err := png.Decode(bytes.NewReader(icon))
		if err != nil {
			t.Fatalf("decode %s tray icon: %v", name, err)
		}
		if decoded.Bounds().Dx() != 24 || decoded.Bounds().Dy() != 24 {
			t.Fatalf("%s tray icon is %v, want 24x24", name, decoded.Bounds())
		}
	}
}
