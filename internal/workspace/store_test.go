package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	store := New(t.TempDir())
	contents := "{\n  \"version\": 1,\n  \"name\": \"Home Lab\",\n  \"resources\": {},\n  \"layout\": {\"type\": \"notes\"}\n}\n"
	if err := store.Save("Home Lab", contents); err != nil {
		t.Fatal(err)
	}
	names, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "Home Lab" {
		t.Fatalf("unexpected names: %#v", names)
	}
	loaded, err := store.Load("Home Lab")
	if err != nil {
		t.Fatal(err)
	}
	if loaded != contents {
		t.Fatalf("round trip changed contents:\n%s", loaded)
	}
	info, err := os.Stat(store.path("Home Lab"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("workspace mode = %o, want 600", info.Mode().Perm())
	}
}

func TestStoreRejectsInvalidInput(t *testing.T) {
	store := New(t.TempDir())
	for _, test := range []struct {
		name    string
		content string
		want    string
	}{
		{"Home", "{", "invalid JSON"},
		{"Home", `{"version":2,"name":"Home"}`, "unsupported workspace version"},
		{"Other", `{"version":1,"name":"Home"}`, "does not match"},
	} {
		if err := store.Save(test.name, test.content); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("Save(%q) error = %v, want %q", test.name, err, test.want)
		}
	}
}

func TestStorePathCannotEscapeDirectory(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	path := store.path("../../escape")
	if filepath.Dir(path) != filepath.Join(root, "workspaces") {
		t.Fatalf("workspace escaped store: %s", path)
	}
}

func TestStoreDeleteMissing(t *testing.T) {
	store := New(t.TempDir())
	if err := store.Delete("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete error = %v, want ErrNotFound", err)
	}
}
