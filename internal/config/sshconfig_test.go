package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestImport_BasicHost(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	writeFile(t, configPath, `
Host web1
    HostName web1.example.com
    User deploy
    Port 2222
    IdentityFile ~/.ssh/id_ed25519
    ForwardAgent yes
`)

	hosts, err := Import(configPath)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d: %+v", len(hosts), hosts)
	}
	h := hosts[0]
	if h.Alias != "web1" || h.Hostname != "web1.example.com" || h.User != "deploy" || h.Port != 2222 {
		t.Fatalf("unexpected host: %+v", h)
	}
	if !h.ForwardAgent {
		t.Fatalf("expected ForwardAgent true")
	}
	if len(h.IdentityFiles) != 1 {
		t.Fatalf("expected 1 identity file, got %+v", h.IdentityFiles)
	}
}

func TestImport_SkipsWildcardPatterns(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	writeFile(t, configPath, `
Host *
    ForwardAgent no

Host 10.0.0.*
    User root

Host db1
    HostName db1.internal
`)

	hosts, err := Import(configPath)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(hosts) != 1 || hosts[0].Alias != "db1" {
		t.Fatalf("expected only db1, got %+v", hosts)
	}
}

func TestImport_MultiplePatternsPerHostBlock(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	writeFile(t, configPath, `
Host web1 web2
    User deploy
`)

	hosts, err := Import(configPath)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %+v", hosts)
	}
	for _, h := range hosts {
		if h.User != "deploy" {
			t.Errorf("host %s: expected user deploy, got %q", h.Alias, h.User)
		}
	}
}

func TestImport_FollowsIncludeDirective(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	includedPath := filepath.Join(dir, "conf.d", "extra.conf")

	writeFile(t, includedPath, `
Host included-host
    HostName included.example.com
`)
	// Relative Include paths are resolved against ~/.ssh (matching real
	// OpenSSH semantics), so the test uses an absolute path to stay
	// independent of the machine running it.
	writeFile(t, configPath, `
Include `+includedPath+`

Host main-host
    HostName main.example.com
`)

	hosts, err := Import(configPath)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	aliases := map[string]bool{}
	for _, h := range hosts {
		aliases[h.Alias] = true
	}
	if !aliases["included-host"] || !aliases["main-host"] {
		t.Fatalf("expected both hosts present, got %+v", hosts)
	}
}

func TestImport_DefaultsPortTo22(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	writeFile(t, configPath, `
Host plain
    HostName plain.example.com
`)

	hosts, err := Import(configPath)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if hosts[0].Port != 22 {
		t.Fatalf("expected default port 22, got %d", hosts[0].Port)
	}
}
