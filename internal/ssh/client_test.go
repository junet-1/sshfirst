package ssh

import "testing"

func TestParseProxyJump_SingleHop(t *testing.T) {
	hops, err := parseProxyJump("bastion@jump.example.com:2200")
	if err != nil {
		t.Fatalf("parseProxyJump: %v", err)
	}
	if len(hops) != 1 {
		t.Fatalf("expected 1 hop, got %d", len(hops))
	}
	if hops[0].user != "bastion" || hops[0].host != "jump.example.com" || hops[0].port != 2200 {
		t.Fatalf("unexpected hop: %+v", hops[0])
	}
}

func TestParseProxyJump_MultipleHopsDefaultPort(t *testing.T) {
	hops, err := parseProxyJump("a@jump1.example.com, b@jump2.example.com")
	if err != nil {
		t.Fatalf("parseProxyJump: %v", err)
	}
	if len(hops) != 2 {
		t.Fatalf("expected 2 hops, got %+v", hops)
	}
	if hops[0].port != 22 || hops[1].port != 22 {
		t.Fatalf("expected default port 22 for both hops, got %+v", hops)
	}
	if hops[0].host != "jump1.example.com" || hops[1].host != "jump2.example.com" {
		t.Fatalf("unexpected hosts: %+v", hops)
	}
}

func TestParseProxyJump_NoUser(t *testing.T) {
	hops, err := parseProxyJump("jump.example.com")
	if err != nil {
		t.Fatalf("parseProxyJump: %v", err)
	}
	if len(hops) != 1 || hops[0].user != "" || hops[0].host != "jump.example.com" {
		t.Fatalf("unexpected hop: %+v", hops)
	}
}

func TestParseProxyJump_Empty(t *testing.T) {
	hops, err := parseProxyJump("")
	if err != nil {
		t.Fatalf("parseProxyJump: %v", err)
	}
	if hops != nil {
		t.Fatalf("expected nil hops for empty value, got %+v", hops)
	}
}

func TestPortOrDefault(t *testing.T) {
	cases := map[int]int{0: 22, -1: 22, 22: 22, 2222: 2222}
	for in, want := range cases {
		if got := portOrDefault(in); got != want {
			t.Errorf("portOrDefault(%d) = %d, want %d", in, got, want)
		}
	}
}
