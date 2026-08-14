package app

import (
	"testing"

	"ssh-first/internal/storage"
)

// The stored password of a host must never be offered to a ProxyJump hop, and a
// host that is not configured for password auth must not have its leftover
// secret replayed to a server that refuses publickey.
func TestPasswordSecretUsable(t *testing.T) {
	password := resolvedAuth{user: "admin", authMethod: storage.AuthMethodPassword, hostID: 7}
	key := resolvedAuth{user: "admin", authMethod: storage.AuthMethodIdentity, hostID: 7}

	cases := []struct {
		name           string
		auth           resolvedAuth
		allowRemember  bool
		user, hostname string
		want           bool
	}{
		{"the host it was stored for", password, true, "admin", "db.internal", true},
		{"a proxy jump hop", password, true, "admin", "bastion.corp", false},
		{"a hop reached as another user", password, true, "jump", "bastion.corp", false},
		{"same host, different user", password, true, "root", "db.internal", false},
		{"key auth host falling back to password", key, true, "admin", "db.internal", false},
		{"ephemeral connection (quick connect)", password, false, "admin", "db.internal", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := passwordSecretUsable(tc.auth, tc.allowRemember, "admin", "db.internal", tc.user, tc.hostname)
			if got != tc.want {
				t.Errorf("passwordSecretUsable(%s@%s) = %v, want %v", tc.user, tc.hostname, got, tc.want)
			}
		})
	}
}
