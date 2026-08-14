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

func TestBuildArgs_LegacyProgress(t *testing.T) {
	modern, err := BuildArgs(Config{Hostname: "h", LocalPath: "/x", RemotePath: "/y"})
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}
	if !slices.Contains(modern, "--info=progress2") {
		t.Errorf("expected --info=progress2 by default, got %v", modern)
	}

	legacy, err := BuildArgs(Config{Hostname: "h", LocalPath: "/x", RemotePath: "/y", LegacyProgress: true})
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}
	if slices.Contains(legacy, "--info=progress2") || slices.Contains(legacy, "-h") {
		t.Errorf("legacy rsync must not be given 3.1+ flags, got %v", legacy)
	}
	if !slices.Contains(legacy, "--progress") {
		t.Errorf("expected --progress as the legacy fallback, got %v", legacy)
	}
}

func TestVersionSupportsInfoProgress(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   bool
	}{
		{"rsync 3.4", "rsync  version 3.4.1  protocol version 32\nCopyright…", true},
		{"rsync 3.1", "rsync  version 3.1.0  protocol version 31", true},
		{"rsync 3.0 (macOS-era)", "rsync  version 3.0.9  protocol version 30", false},
		{"rsync 2.6.9 (macOS)", "rsync  version 2.6.9  protocol version 29", false},
		{"openrsync (macOS 15+)", "openrsync: protocol version 27", false},
		{"unparseable", "some other tool", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := versionSupportsInfoProgress(tc.output); got != tc.want {
				t.Errorf("versionSupportsInfoProgress(%q) = %v, want %v", tc.output, got, tc.want)
			}
		})
	}
}
