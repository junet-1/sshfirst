package sftp

import "testing"

func TestResolveRemotePath(t *testing.T) {
	client := &Client{home: "/home/deploy"}
	tests := map[string]string{
		"":                  "/home/deploy",
		".":                 "/home/deploy",
		"uploads":           "/home/deploy/uploads",
		"./uploads/../logs": "/home/deploy/logs",
		"/srv/releases/..":  "/srv",
	}
	for input, want := range tests {
		if got := client.Resolve(input); got != want {
			t.Errorf("Resolve(%q) = %q, want %q", input, got, want)
		}
	}
}
