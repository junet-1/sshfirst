package storage

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir(), os.DirFS("../../migrations"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// onlyMigration returns an fs.FS exposing just one migration file, so tests
// can simulate a database that has only had earlier migrations applied.
func onlyMigration(t *testing.T, name string) fs.FS {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("../../migrations", name))
	if err != nil {
		t.Fatalf("read migration %s: %v", name, err)
	}
	return fstest.MapFS{name: {Data: contents}}
}

func TestMigration_RelaxHostLabelUniqueness_AppliesOnTopOfExistingData(t *testing.T) {
	dir := t.TempDir()

	// Simulate a real installation that only ever saw 0001_init.sql: create
	// the DB, add a host under the old UNIQUE(label, source) constraint. The
	// row is inserted with raw SQL matching the 0001 schema (CreateHost now
	// writes columns that only exist in later migrations).
	s1, err := Open(dir, onlyMigration(t, "0001_init.sql"))
	if err != nil {
		t.Fatalf("Open (0001 only): %v", err)
	}
	if _, err := s1.db.Exec(`INSERT INTO hosts (label, hostname, source) VALUES (?, ?, ?)`,
		"Home Server", "home.example.com", HostSourceManual); err != nil {
		t.Fatalf("insert under old schema: %v", err)
	}
	if _, err := s1.db.Exec(`INSERT INTO hosts (label, hostname, source) VALUES (?, ?, ?)`,
		"Zulu", "zulu.example.com", HostSourceManual); err != nil {
		t.Fatalf("insert second host under old schema: %v", err)
	}
	if _, err := s1.db.Exec(`INSERT INTO hosts (label, hostname, source) VALUES (?, ?, ?)`,
		"Alpha", "alpha.example.com", HostSourceManual); err != nil {
		t.Fatalf("insert third host under old schema: %v", err)
	}
	if _, err := s1.db.Exec(`INSERT INTO folders (name) VALUES (?)`, "Legacy Folder"); err != nil {
		t.Fatalf("insert folder under old schema: %v", err)
	}
	if _, err := s1.db.Exec(`INSERT INTO folders (name) VALUES (?)`, "Alpha Folder"); err != nil {
		t.Fatalf("insert second folder under old schema: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Now open the same database file with the full, current migration set
	// and confirm 0002 applies cleanly and the old constraint is gone.
	s2, err := Open(dir, os.DirFS("../../migrations"))
	if err != nil {
		t.Fatalf("Open (full migrations, upgrading existing db): %v", err)
	}
	defer s2.Close()

	hosts, err := s2.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts after upgrade: %v", err)
	}
	if len(hosts) != 3 || hosts[0].Label != "Alpha" || hosts[1].Label != "Home Server" || hosts[2].Label != "Zulu" {
		t.Fatalf("expected existing hosts to start in alphabetical sidebar order, got %+v", hosts)
	}
	if hosts[1].Protocol != HostProtocolSSH || hosts[1].RemotePath != "." {
		t.Fatalf("expected upgraded host to default to SSH and its home folder, got %+v", hosts[1])
	}
	folders, err := s2.ListFolders()
	if err != nil {
		t.Fatalf("ListFolders after upgrade: %v", err)
	}
	if len(folders) != 2 || folders[0].Name != "Alpha Folder" || folders[1].Name != "Legacy Folder" || folders[0].Icon != "folder" {
		t.Fatalf("expected pre-existing folder to receive the default icon, got %+v", folders)
	}

	if _, err := s2.CreateHost(HostInput{Label: "Home Server", Hostname: "other.example.com"}, HostSourceManual); err != nil {
		t.Fatalf("expected duplicate label to be allowed after migration, got error: %v", err)
	}
}

func TestSFTPHostRoundTrip(t *testing.T) {
	s := newTestStore(t)

	created, err := s.CreateHost(HostInput{
		Label:      "files",
		Hostname:   "files.example.com",
		Port:       2222,
		User:       "deploy",
		Protocol:   HostProtocolSFTP,
		RemotePath: "/srv/releases",
	}, HostSourceManual)
	if err != nil {
		t.Fatalf("CreateHost: %v", err)
	}
	if created.Protocol != HostProtocolSFTP || created.RemotePath != "/srv/releases" {
		t.Fatalf("unexpected SFTP host after create: %+v", created)
	}

	updated, err := s.UpdateHost(created.ID, HostInput{
		Label:      created.Label,
		Hostname:   created.Hostname,
		Port:       created.Port,
		User:       created.User,
		Protocol:   HostProtocolSFTP,
		RemotePath: "uploads",
	})
	if err != nil {
		t.Fatalf("UpdateHost: %v", err)
	}
	if updated.Protocol != HostProtocolSFTP || updated.RemotePath != "uploads" {
		t.Fatalf("unexpected SFTP host after update: %+v", updated)
	}
}

func TestCreateGetUpdateDeleteHost(t *testing.T) {
	s := newTestStore(t)

	created, err := s.CreateHost(HostInput{
		Label:           "web1",
		Hostname:        "web1.example.com",
		Port:            22,
		User:            "deploy",
		ControlPanelURL: "https://panel.example.com:8006",
		Tags:            []string{"prod", "web"},
	}, HostSourceManual)
	if err != nil {
		t.Fatalf("CreateHost: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("expected non-zero ID")
	}
	if len(created.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %+v", created.Tags)
	}
	if created.ControlPanelURL != "https://panel.example.com:8006" {
		t.Fatalf("unexpected control panel URL: %q", created.ControlPanelURL)
	}

	fetched, err := s.GetHost(created.ID)
	if err != nil {
		t.Fatalf("GetHost: %v", err)
	}
	if fetched.Hostname != "web1.example.com" {
		t.Fatalf("unexpected hostname: %s", fetched.Hostname)
	}
	if fetched.ControlPanelURL != "https://panel.example.com:8006" {
		t.Fatalf("control panel URL did not round-trip: %q", fetched.ControlPanelURL)
	}

	updated, err := s.UpdateHost(created.ID, HostInput{
		Label:           "web1-renamed",
		Hostname:        "web1.example.com",
		Port:            2222,
		User:            "deploy",
		ControlPanelURL: "https://portainer.example.com",
		Tags:            []string{"prod"},
	})
	if err != nil {
		t.Fatalf("UpdateHost: %v", err)
	}
	if updated.Label != "web1-renamed" || updated.Port != 2222 || len(updated.Tags) != 1 {
		t.Fatalf("unexpected updated host: %+v", updated)
	}
	if updated.ControlPanelURL != "https://portainer.example.com" {
		t.Fatalf("unexpected updated control panel URL: %q", updated.ControlPanelURL)
	}

	if err := s.DeleteHost(created.ID); err != nil {
		t.Fatalf("DeleteHost: %v", err)
	}
	if _, err := s.GetHost(created.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestCreateHostAllowsDuplicateLabels(t *testing.T) {
	s := newTestStore(t)

	first, err := s.CreateHost(HostInput{Label: "web1", Hostname: "a.example.com"}, HostSourceManual)
	if err != nil {
		t.Fatalf("CreateHost (first): %v", err)
	}
	second, err := s.CreateHost(HostInput{Label: "web1", Hostname: "b.example.com"}, HostSourceManual)
	if err != nil {
		t.Fatalf("CreateHost (duplicate label): %v", err)
	}
	if first.ID == second.ID {
		t.Fatalf("expected two distinct hosts, got same ID twice")
	}

	hosts, err := s.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts with the same label, got %d", len(hosts))
	}
}

func TestHostTagsAndIdentityFilesAreNeverNil(t *testing.T) {
	// A nil slice marshals to JSON null, and the frontend iterates these
	// fields directly (for...of / .length), which throws on null. Every
	// read path must therefore return non-nil slices even when empty.
	s := newTestStore(t)

	created, err := s.CreateHost(HostInput{Label: "no-tags", Hostname: "x.example.com"}, HostSourceManual)
	if err != nil {
		t.Fatalf("CreateHost: %v", err)
	}
	if created.Tags == nil {
		t.Errorf("CreateHost returned nil Tags")
	}
	if created.IdentityFiles == nil {
		t.Errorf("CreateHost returned nil IdentityFiles")
	}

	got, err := s.GetHost(created.ID)
	if err != nil {
		t.Fatalf("GetHost: %v", err)
	}
	if got.Tags == nil {
		t.Errorf("GetHost returned nil Tags")
	}
	if got.IdentityFiles == nil {
		t.Errorf("GetHost returned nil IdentityFiles")
	}

	list, err := s.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	for _, h := range list {
		if h.Tags == nil {
			t.Errorf("ListHosts returned host %q with nil Tags", h.Label)
		}
		if h.IdentityFiles == nil {
			t.Errorf("ListHosts returned host %q with nil IdentityFiles", h.Label)
		}
	}
}

func TestListHostsKeepsSidebarInsertionOrder(t *testing.T) {
	s := newTestStore(t)

	for _, label := range []string{"zebra", "alpha", "mike"} {
		if _, err := s.CreateHost(HostInput{Label: label, Hostname: label + ".example.com"}, HostSourceManual); err != nil {
			t.Fatalf("CreateHost(%s): %v", label, err)
		}
	}

	hosts, err := s.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	if len(hosts) != 3 {
		t.Fatalf("expected 3 hosts, got %d", len(hosts))
	}
	if hosts[0].Label != "zebra" || hosts[1].Label != "alpha" || hosts[2].Label != "mike" {
		t.Fatalf("expected insertion order, got %v, %v, %v", hosts[0].Label, hosts[1].Label, hosts[2].Label)
	}
}

func TestReorderHostPersistsSiblingOrderAndFolder(t *testing.T) {
	s := newTestStore(t)
	folder, err := s.CreateFolder("Production", nil)
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}

	rootA, err := s.CreateHost(HostInput{Label: "root-a", Hostname: "a.example.com"}, HostSourceManual)
	if err != nil {
		t.Fatalf("CreateHost(root-a): %v", err)
	}
	rootB, err := s.CreateHost(HostInput{Label: "root-b", Hostname: "b.example.com"}, HostSourceManual)
	if err != nil {
		t.Fatalf("CreateHost(root-b): %v", err)
	}
	inside, err := s.CreateHost(HostInput{Label: "inside", Hostname: "inside.example.com", FolderID: &folder.ID}, HostSourceManual)
	if err != nil {
		t.Fatalf("CreateHost(inside): %v", err)
	}

	if err := s.ReorderHost(rootB.ID, nil, &rootA.ID, true); err != nil {
		t.Fatalf("ReorderHost before root target: %v", err)
	}
	if got := hostLabelsInFolder(t, s, nil); !equalStrings(got, []string{"root-b", "root-a"}) {
		t.Fatalf("unexpected root order: %v", got)
	}

	if err := s.ReorderHost(rootA.ID, &folder.ID, &inside.ID, true); err != nil {
		t.Fatalf("ReorderHost into folder: %v", err)
	}
	if got := hostLabelsInFolder(t, s, &folder.ID); !equalStrings(got, []string{"root-a", "inside"}) {
		t.Fatalf("unexpected folder order: %v", got)
	}
	if got := hostLabelsInFolder(t, s, nil); !equalStrings(got, []string{"root-b"}) {
		t.Fatalf("old root order was not compacted: %v", got)
	}
}

func TestReorderHostRejectsTargetFromAnotherFolder(t *testing.T) {
	s := newTestStore(t)
	folder := mustCreateFolder(t, s, "Production", nil)
	root := mustCreateHost(t, s, HostInput{Label: "root", Hostname: "root.example.com"})
	inside := mustCreateHost(t, s, HostInput{Label: "inside", Hostname: "inside.example.com", FolderID: &folder.ID})

	if err := s.ReorderHost(root.ID, nil, &inside.ID, true); err == nil {
		t.Fatal("expected mismatched destination target to fail")
	}
	got, err := s.GetHost(root.ID)
	if err != nil {
		t.Fatalf("GetHost: %v", err)
	}
	if got.FolderID != nil {
		t.Fatalf("failed reorder changed host folder: %+v", got.FolderID)
	}
}

func TestReorderFolderPersistsSiblingOrderAndRejectsCycles(t *testing.T) {
	s := newTestStore(t)
	a := mustCreateFolder(t, s, "A", nil)
	mustCreateFolder(t, s, "B", nil)
	c := mustCreateFolder(t, s, "C", nil)
	child := mustCreateFolder(t, s, "Child", &a.ID)

	if err := s.ReorderFolder(c.ID, nil, &a.ID, true); err != nil {
		t.Fatalf("ReorderFolder: %v", err)
	}
	if got := folderNamesUnder(t, s, nil); !equalStrings(got, []string{"C", "A", "B"}) {
		t.Fatalf("unexpected root folder order: %v", got)
	}

	if err := s.ReorderFolder(a.ID, &child.ID, nil, false); err == nil {
		t.Fatal("expected descendant move to be rejected")
	}
	if got := folderNamesUnder(t, s, nil); !equalStrings(got, []string{"C", "A", "B"}) {
		t.Fatalf("failed cycle move changed folder order: %v", got)
	}
}

func TestDeleteFolderAppendsNestedHostsToRootInTreeOrder(t *testing.T) {
	s := newTestStore(t)
	root := mustCreateHost(t, s, HostInput{Label: "root", Hostname: "root.example.com"})
	parent := mustCreateFolder(t, s, "Parent", nil)
	parentHost := mustCreateHost(t, s, HostInput{Label: "parent-host", Hostname: "parent.example.com", FolderID: &parent.ID})
	child := mustCreateFolder(t, s, "Child", &parent.ID)
	childHost := mustCreateHost(t, s, HostInput{Label: "child-host", Hostname: "child.example.com", FolderID: &child.ID})

	if err := s.DeleteFolder(parent.ID); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
	if got := hostLabelsInFolder(t, s, nil); !equalStrings(got, []string{root.Label, parentHost.Label, childHost.Label}) {
		t.Fatalf("nested hosts were not appended in tree order: %v", got)
	}
	if folders, err := s.ListFolders(); err != nil || len(folders) != 0 {
		t.Fatalf("nested folders survived parent deletion: folders=%v err=%v", folders, err)
	}
}

func mustCreateHost(t *testing.T, s *Store, input HostInput) Host {
	t.Helper()
	host, err := s.CreateHost(input, HostSourceManual)
	if err != nil {
		t.Fatalf("CreateHost(%q): %v", input.Label, err)
	}
	return host
}

func mustCreateFolder(t *testing.T, s *Store, name string, parentID *int64) Folder {
	t.Helper()
	folder, err := s.CreateFolder(name, parentID)
	if err != nil {
		t.Fatalf("CreateFolder(%q): %v", name, err)
	}
	return folder
}

func hostLabelsInFolder(t *testing.T, s *Store, folderID *int64) []string {
	t.Helper()
	hosts, err := s.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	var labels []string
	for _, host := range hosts {
		if (folderID == nil && host.FolderID == nil) ||
			(folderID != nil && host.FolderID != nil && *folderID == *host.FolderID) {
			labels = append(labels, host.Label)
		}
	}
	return labels
}

func folderNamesUnder(t *testing.T, s *Store, parentID *int64) []string {
	t.Helper()
	folders, err := s.ListFolders()
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	var names []string
	for _, folder := range folders {
		if (parentID == nil && folder.ParentID == nil) ||
			(parentID != nil && folder.ParentID != nil && *parentID == *folder.ParentID) {
			names = append(names, folder.Name)
		}
	}
	return names
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestUpsertImportedHostIsIdempotent(t *testing.T) {
	s := newTestStore(t)

	input := HostInput{Label: "imported1", Hostname: "old.example.com", Port: 22}
	first, err := s.UpsertImportedHost(input)
	if err != nil {
		t.Fatalf("first UpsertImportedHost: %v", err)
	}

	input.Hostname = "new.example.com"
	second, err := s.UpsertImportedHost(input)
	if err != nil {
		t.Fatalf("second UpsertImportedHost: %v", err)
	}

	if first.ID != second.ID {
		t.Fatalf("expected same host ID on re-import, got %d then %d", first.ID, second.ID)
	}
	if second.Hostname != "new.example.com" {
		t.Fatalf("expected hostname updated on re-import, got %s", second.Hostname)
	}

	hosts, err := s.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected re-import to update, not duplicate; got %d hosts", len(hosts))
	}
}

func TestFoldersAndMoveHost(t *testing.T) {
	s := newTestStore(t)

	folder, err := s.CreateFolder("Production", nil)
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	if folder.Icon != "folder" {
		t.Fatalf("expected default folder icon, got %q", folder.Icon)
	}

	host, err := s.CreateHost(HostInput{Label: "db1", Hostname: "db1.example.com"}, HostSourceManual)
	if err != nil {
		t.Fatalf("CreateHost: %v", err)
	}

	if err := s.MoveHostToFolder(host.ID, &folder.ID); err != nil {
		t.Fatalf("MoveHostToFolder: %v", err)
	}
	moved, err := s.GetHost(host.ID)
	if err != nil {
		t.Fatalf("GetHost: %v", err)
	}
	if moved.FolderID == nil || *moved.FolderID != folder.ID {
		t.Fatalf("expected host in folder %d, got %+v", folder.ID, moved.FolderID)
	}

	if err := s.DeleteFolder(folder.ID); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
	afterDelete, err := s.GetHost(host.ID)
	if err != nil {
		t.Fatalf("GetHost after folder delete: %v", err)
	}
	if afterDelete.FolderID != nil {
		t.Fatalf("expected host to become un-foldered, got %+v", afterDelete.FolderID)
	}
}

func TestFolderIconRoundTrip(t *testing.T) {
	s := newTestStore(t)

	folder, err := s.CreateFolderWithIcon("Databases", nil, "database")
	if err != nil {
		t.Fatalf("CreateFolderWithIcon: %v", err)
	}
	if folder.Icon != "database" {
		t.Fatalf("expected database icon, got %+v", folder)
	}

	updated, err := s.UpdateFolder(folder.ID, "Production Databases", "shield")
	if err != nil {
		t.Fatalf("UpdateFolder: %v", err)
	}
	if updated.Name != "Production Databases" || updated.Icon != "shield" {
		t.Fatalf("unexpected updated folder: %+v", updated)
	}

	listed, err := s.ListFolders()
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	if len(listed) != 1 || listed[0].Icon != "shield" {
		t.Fatalf("folder icon was not persisted: %+v", listed)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	s := newTestStore(t)

	if _, exists, err := s.GetSetting("theme"); err != nil || exists {
		t.Fatalf("expected no setting initially, exists=%v err=%v", exists, err)
	}

	if err := s.SetSetting("theme", "dark"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	value, exists, err := s.GetSetting("theme")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if !exists || value != "dark" {
		t.Fatalf("expected theme=dark, got %q (exists=%v)", value, exists)
	}

	if err := s.SetSetting("theme", "light"); err != nil {
		t.Fatalf("SetSetting overwrite: %v", err)
	}
	value, _, err = s.GetSetting("theme")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if value != "light" {
		t.Fatalf("expected theme=light after overwrite, got %q", value)
	}
}

func TestFavoriteToggle(t *testing.T) {
	s := newTestStore(t)

	host, err := s.CreateHost(HostInput{Label: "web1", Hostname: "web1.example.com"}, HostSourceManual)
	if err != nil {
		t.Fatalf("CreateHost: %v", err)
	}
	if host.Favorite {
		t.Fatalf("expected new host to not be favorite")
	}

	if err := s.SetFavorite(host.ID, true); err != nil {
		t.Fatalf("SetFavorite: %v", err)
	}
	fetched, err := s.GetHost(host.ID)
	if err != nil {
		t.Fatalf("GetHost: %v", err)
	}
	if !fetched.Favorite {
		t.Fatalf("expected host to be favorite after SetFavorite(true)")
	}
}
