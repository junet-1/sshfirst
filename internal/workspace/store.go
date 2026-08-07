// Package workspace persists declarative workspace JSON files. It deliberately
// does not interpret layout nodes; format evolution belongs to WorkspaceManager
// in the frontend, while this package provides safe, atomic file storage.
package workspace

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

var ErrNotFound = errors.New("workspace not found")

type header struct {
	Version int    `json:"version"`
	Name    string `json:"name"`
}

type Store struct {
	dir string
}

func New(dataDir string) *Store {
	return &Store{dir: filepath.Join(dataDir, "workspaces")}
}

func (s *Store) List() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		contents, readErr := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if readErr != nil {
			return nil, fmt.Errorf("read workspace %s: %w", entry.Name(), readErr)
		}
		parsed, parseErr := parseHeader(contents)
		if parseErr != nil {
			return nil, fmt.Errorf("workspace %s is invalid: %w", entry.Name(), parseErr)
		}
		names = append(names, parsed.Name)
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	return names, nil
}

func (s *Store) Load(name string) (string, error) {
	contents, err := os.ReadFile(s.path(name))
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if err != nil {
		return "", fmt.Errorf("load workspace %s: %w", name, err)
	}
	return string(contents), nil
}

func (s *Store) Save(name, contents string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("workspace name is required")
	}
	parsed, err := parseHeader([]byte(contents))
	if err != nil {
		return err
	}
	if parsed.Version != 1 {
		return fmt.Errorf("unsupported workspace version %d", parsed.Version)
	}
	if strings.TrimSpace(parsed.Name) != name {
		return fmt.Errorf("workspace JSON name %q does not match %q", parsed.Name, name)
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create workspace directory: %w", err)
	}
	temp, err := os.CreateTemp(s.dir, ".workspace-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary workspace: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.WriteString(contents); err != nil {
		temp.Close()
		return fmt.Errorf("write workspace: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync workspace: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close workspace: %w", err)
	}
	if err := os.Rename(tempPath, s.path(name)); err != nil {
		return fmt.Errorf("replace workspace: %w", err)
	}
	return nil
}

func (s *Store) Delete(name string) error {
	err := os.Remove(s.path(name))
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if err != nil {
		return fmt.Errorf("delete workspace %s: %w", name, err)
	}
	return nil
}

func parseHeader(contents []byte) (header, error) {
	if !json.Valid(contents) {
		return header{}, errors.New("invalid JSON")
	}
	var parsed header
	if err := json.Unmarshal(contents, &parsed); err != nil {
		return header{}, fmt.Errorf("decode JSON: %w", err)
	}
	if strings.TrimSpace(parsed.Name) == "" {
		return header{}, errors.New("workspace name is required")
	}
	return parsed, nil
}

func (s *Store) path(name string) string {
	name = strings.TrimSpace(name)
	var slug strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			slug.WriteRune(unicode.ToLower(r))
			lastDash = false
		case r == '-' || r == '_' || unicode.IsSpace(r):
			if !lastDash && slug.Len() > 0 {
				slug.WriteByte('-')
				lastDash = true
			}
		}
		if slug.Len() >= 48 {
			break
		}
	}
	base := strings.Trim(slug.String(), "-")
	if base == "" {
		base = "workspace"
	}
	sum := sha256.Sum256([]byte(name))
	return filepath.Join(s.dir, fmt.Sprintf("%s-%x.json", base, sum[:4]))
}
