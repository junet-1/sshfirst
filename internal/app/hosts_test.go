package app

import (
	"testing"

	"ssh-first/internal/storage"
)

func TestNormalizeFolderIcon(t *testing.T) {
	for _, icon := range []string{"folder", "server", "terminal", "cloud", "database", "globe", "code", "home", "shield", "archive"} {
		if got := normalizeFolderIcon(icon); got != icon {
			t.Errorf("normalizeFolderIcon(%q) = %q", icon, got)
		}
	}
	if got := normalizeFolderIcon("not-an-icon"); got != "folder" {
		t.Fatalf("unknown icon should fall back to folder, got %q", got)
	}
}

func TestValidateHostInput(t *testing.T) {
	// SSH/SFTP hosts are addressed by hostname.
	if err := validateHostInput(storage.HostInput{Label: "web1", Protocol: storage.HostProtocolSSH}); err == nil {
		t.Error("ssh host without hostname should be rejected")
	}
	if err := validateHostInput(storage.HostInput{Label: "web1", Protocol: storage.HostProtocolSSH, Hostname: "a.example.com"}); err != nil {
		t.Errorf("valid ssh host rejected: %v", err)
	}

	// A web host needs a URL, not a hostname.
	if err := validateHostInput(storage.HostInput{Label: "panel", Protocol: storage.HostProtocolWeb}); err == nil {
		t.Error("web host without URL should be rejected")
	}
	if err := validateHostInput(storage.HostInput{Label: "panel", Protocol: storage.HostProtocolWeb, ControlPanelURL: "https://panel.example.com"}); err != nil {
		t.Errorf("valid web host (no hostname) rejected: %v", err)
	}

	// An empty label is always invalid, and an unknown protocol is rejected.
	if err := validateHostInput(storage.HostInput{Protocol: storage.HostProtocolWeb, ControlPanelURL: "https://x"}); err == nil {
		t.Error("host without label should be rejected")
	}
	if err := validateHostInput(storage.HostInput{Label: "x", Protocol: "gopher"}); err == nil {
		t.Error("unknown protocol should be rejected")
	}
}

func TestNormalizeHostInputWebStripsSSHFields(t *testing.T) {
	in := storage.HostInput{
		Label:           "panel",
		Protocol:        storage.HostProtocolWeb,
		User:            "  admin@example.com  ",
		AuthMethod:      storage.AuthMethodAgent,
		ControlPanelURL: "  https://panel.example.com  ",
		ProxyJump:       "bastion",
		LoginScript:     "tmux",
		ForwardAgent:    true,
		IdentityFiles:   []string{"~/.ssh/id_ed25519"},
	}
	out := normalizeHostInput(in)
	if out.ControlPanelURL != "https://panel.example.com" {
		t.Errorf("URL not trimmed: %q", out.ControlPanelURL)
	}
	if out.ProxyJump != "" || out.LoginScript != "" || out.ForwardAgent || len(out.IdentityFiles) != 0 {
		t.Errorf("SSH-only fields not stripped for web host: %+v", out)
	}
	if out.User != "admin@example.com" || out.AuthMethod != storage.AuthMethodPassword {
		t.Errorf("web autofill identity not retained: %+v", out)
	}
}

func TestValidatePanelURL(t *testing.T) {
	accepted := []string{
		"https://proxmox.example.com:8006/",
		"http://192.168.1.1/",
		"https://panel.example.com/path?a=1",
	}
	for _, raw := range accepted {
		if err := validatePanelURL(raw); err != nil {
			t.Errorf("validatePanelURL(%q) = %v, want nil", raw, err)
		}
	}

	// A javascript: URL reaching an <iframe src> would run in the app shell's
	// origin, where every binding — including the stored passwords — is exposed.
	rejected := []string{
		"javascript://x%0aalert(1)",
		"data:text/html,<script>alert(1)</script>",
		"file:///etc/passwd",
		"wails://wails.localhost/wails/runtime",
		"https://panel.corp.example@evil.tld/",
		"https://user:pw@evil.tld/",
		"not a url at all",
		"https://",
	}
	for _, raw := range rejected {
		if err := validatePanelURL(raw); err == nil {
			t.Errorf("validatePanelURL(%q) = nil, want an error", raw)
		}
	}
}

func TestValidateHostInputRejectsHostilePanelURL(t *testing.T) {
	input := storage.HostInput{
		Label:           "panel",
		Protocol:        storage.HostProtocolWeb,
		ControlPanelURL: "javascript://x%0afetch('https://evil.tld')",
	}
	if err := validateHostInput(input); err == nil {
		t.Fatal("validateHostInput accepted a javascript: control panel URL")
	}
}
