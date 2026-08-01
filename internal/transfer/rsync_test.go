package transfer

import (
	"slices"
	"strings"
	"testing"
)

func TestBuildArgs_UploadWithOptions(t *testing.T) {
	args, err := BuildArgs(Config{
		User:           "deploy",
		Hostname:       "web1.example.com",
		Port:           2222,
		IdentityFiles:  []string{"/home/u/.ssh/id_ed25519"},
		ProxyJump:      "bastion",
		KnownHostsPath: "/home/u/.local/share/ssh-first/known_hosts",
		LocalPath:      "/home/u/site/",
		RemotePath:     "/var/www/site/",
		Upload:         true,
		Archive:        true,
		Compress:       true,
		Delete:         true,
		DryRun:         true,
	})
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}

	for _, want := range []string{"-a", "-z", "--delete", "-n"} {
		if !slices.Contains(args, want) {
			t.Errorf("expected arg %q in %v", want, args)
		}
	}

	// Last two args: source then destination (upload => local first).
	if args[len(args)-2] != "/home/u/site/" {
		t.Errorf("expected local path as source for upload, got %q", args[len(args)-2])
	}
	if args[len(args)-1] != "deploy@web1.example.com:/var/www/site/" {
		t.Errorf("unexpected remote spec: %q", args[len(args)-1])
	}

	// The -e ssh command must carry port, identity, proxy jump, known_hosts
	// and BatchMode.
	e := sshCommand(t, args)
	for _, want := range []string{"-p 2222", "-i /home/u/.ssh/id_ed25519", "IdentitiesOnly=yes", "ProxyJump=bastion", "UserKnownHostsFile=/home/u/.local/share/ssh-first/known_hosts", "BatchMode=yes"} {
		if !strings.Contains(e, want) {
			t.Errorf("expected ssh command to contain %q, got %q", want, e)
		}
	}
}

func TestBuildArgs_DownloadOrderAndDefaults(t *testing.T) {
	args, err := BuildArgs(Config{
		Hostname:   "host",
		LocalPath:  "/tmp/dl/",
		RemotePath: "/data/",
		Upload:     false,
	})
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}
	// Download => remote first, local second.
	if args[len(args)-2] != "host:/data/" {
		t.Errorf("expected remote spec as source for download, got %q", args[len(args)-2])
	}
	if args[len(args)-1] != "/tmp/dl/" {
		t.Errorf("expected local path as destination for download, got %q", args[len(args)-1])
	}
	// No archive requested => plain -r, and no port flag for default port.
	if !slices.Contains(args, "-r") {
		t.Errorf("expected -r when Archive is false, got %v", args)
	}
	if strings.Contains(sshCommand(t, args), "-p ") {
		t.Errorf("did not expect a -p flag for the default port")
	}
}

func TestBuildArgs_RequiresPaths(t *testing.T) {
	if _, err := BuildArgs(Config{Hostname: "h", LocalPath: "/x"}); err == nil {
		t.Fatalf("expected error when remote path is missing")
	}
	if _, err := BuildArgs(Config{LocalPath: "/x", RemotePath: "/y"}); err == nil {
		t.Fatalf("expected error when hostname is missing")
	}
}

// sshCommand returns the value passed to rsync's -e flag.
func sshCommand(t *testing.T, args []string) string {
	t.Helper()
	for i, a := range args {
		if a == "-e" && i+1 < len(args) {
			return args[i+1]
		}
	}
	t.Fatalf("no -e flag in args: %v", args)
	return ""
}
