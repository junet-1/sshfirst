package storage

import "testing"

func TestSnippetsCRUDAndScoping(t *testing.T) {
	s := newTestStore(t)

	host, err := s.CreateHost(HostInput{Label: "web1", Hostname: "web1.example.com"}, HostSourceManual)
	if err != nil {
		t.Fatalf("CreateHost: %v", err)
	}

	global, err := s.CreateSnippet(SnippetInput{Name: "uptime", Command: "uptime"})
	if err != nil {
		t.Fatalf("CreateSnippet global: %v", err)
	}
	if global.HostID != nil {
		t.Fatalf("expected global snippet to have nil HostID")
	}
	scoped, err := s.CreateSnippet(SnippetInput{Name: "restart", Command: "systemctl restart nginx", HostID: &host.ID})
	if err != nil {
		t.Fatalf("CreateSnippet scoped: %v", err)
	}

	// Listing for the host returns both global and its own scoped snippet.
	forHost, err := s.ListSnippets(&host.ID)
	if err != nil {
		t.Fatalf("ListSnippets(host): %v", err)
	}
	if len(forHost) != 2 {
		t.Fatalf("expected 2 snippets for host, got %d", len(forHost))
	}

	// Listing for a different host returns only the global one.
	otherHost, err := s.CreateHost(HostInput{Label: "db1", Hostname: "db1.example.com"}, HostSourceManual)
	if err != nil {
		t.Fatalf("CreateHost other: %v", err)
	}
	forOther, err := s.ListSnippets(&otherHost.ID)
	if err != nil {
		t.Fatalf("ListSnippets(other): %v", err)
	}
	if len(forOther) != 1 || forOther[0].Name != "uptime" {
		t.Fatalf("expected only the global snippet for another host, got %+v", forOther)
	}

	if _, err := s.UpdateSnippet(scoped.ID, SnippetInput{Name: "restart-web", Command: "systemctl restart apache2", HostID: &host.ID}); err != nil {
		t.Fatalf("UpdateSnippet: %v", err)
	}
	if err := s.DeleteSnippet(global.ID); err != nil {
		t.Fatalf("DeleteSnippet: %v", err)
	}

	all, err := s.ListSnippets(nil)
	if err != nil {
		t.Fatalf("ListSnippets(nil): %v", err)
	}
	if len(all) != 1 || all[0].Name != "restart-web" {
		t.Fatalf("expected only the renamed scoped snippet to remain, got %+v", all)
	}
}

func TestDeletingHostCascadesSnippets(t *testing.T) {
	s := newTestStore(t)
	host, err := s.CreateHost(HostInput{Label: "web1", Hostname: "web1.example.com"}, HostSourceManual)
	if err != nil {
		t.Fatalf("CreateHost: %v", err)
	}
	if _, err := s.CreateSnippet(SnippetInput{Name: "x", Command: "x", HostID: &host.ID}); err != nil {
		t.Fatalf("CreateSnippet: %v", err)
	}
	if err := s.DeleteHost(host.ID); err != nil {
		t.Fatalf("DeleteHost: %v", err)
	}
	all, err := s.ListSnippets(nil)
	if err != nil {
		t.Fatalf("ListSnippets: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("expected host-scoped snippets to be removed with the host, got %+v", all)
	}
}

func TestHostLoginScriptRoundTrip(t *testing.T) {
	s := newTestStore(t)
	created, err := s.CreateHost(HostInput{Label: "web1", Hostname: "web1.example.com", LoginScript: "tmux attach || tmux new"}, HostSourceManual)
	if err != nil {
		t.Fatalf("CreateHost: %v", err)
	}
	if created.LoginScript != "tmux attach || tmux new" {
		t.Fatalf("login script not persisted on create: %q", created.LoginScript)
	}
	got, err := s.GetHost(created.ID)
	if err != nil {
		t.Fatalf("GetHost: %v", err)
	}
	if got.LoginScript != "tmux attach || tmux new" {
		t.Fatalf("login script not returned on get: %q", got.LoginScript)
	}
}
