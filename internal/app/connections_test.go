package app

import "testing"

func TestParseQuickTarget(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		user     string
		hostname string
		port     int
	}{
		{name: "standard", input: "alice@example.test", user: "alice", hostname: "example.test", port: 22},
		{name: "inline port", input: "alice@example.test:2222", user: "alice", hostname: "example.test", port: 2222},
		{name: "ssh command", input: "ssh -p 2200 alice@example.test", user: "alice", hostname: "example.test", port: 2200},
		{name: "ipv6", input: "alice@[2001:db8::1]:2222", user: "alice", hostname: "2001:db8::1", port: 2222},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			host, err := parseQuickTarget(test.input)
			if err != nil {
				t.Fatalf("parseQuickTarget() error = %v", err)
			}
			if host.User != test.user || host.Hostname != test.hostname || host.Port != test.port {
				t.Fatalf("parseQuickTarget() = %s@%s:%d, want %s@%s:%d", host.User, host.Hostname, host.Port, test.user, test.hostname, test.port)
			}
		})
	}
}

func TestParseQuickTargetRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{"", "alice@example.test:nope", "ssh -p alice@example.test", "ssh -J jump alice@example.test", "alice@example.test:70000"} {
		t.Run(input, func(t *testing.T) {
			if _, err := parseQuickTarget(input); err == nil {
				t.Fatalf("parseQuickTarget(%q) unexpectedly succeeded", input)
			}
		})
	}
}
