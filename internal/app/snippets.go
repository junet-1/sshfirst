package app

import (
	"fmt"

	"ssh-first/internal/storage"
)

// ListSnippets returns global snippets plus (when hostID != 0) those scoped to
// that host. Pass hostID 0 to list all.
func (a *App) ListSnippets(hostID int64) ([]storage.Snippet, error) {
	if err := a.requireStore(); err != nil {
		return nil, err
	}
	var scope *int64
	if hostID != 0 {
		scope = &hostID
	}
	return a.store.ListSnippets(scope)
}

// CreateSnippet adds a snippet.
func (a *App) CreateSnippet(input storage.SnippetInput) (storage.Snippet, error) {
	if err := a.requireStore(); err != nil {
		return storage.Snippet{}, err
	}
	if input.Name == "" || input.Command == "" {
		return storage.Snippet{}, fmt.Errorf("name and command are required")
	}
	return a.store.CreateSnippet(input)
}

// UpdateSnippet overwrites an existing snippet.
func (a *App) UpdateSnippet(id int64, input storage.SnippetInput) (storage.Snippet, error) {
	if err := a.requireStore(); err != nil {
		return storage.Snippet{}, err
	}
	if input.Name == "" || input.Command == "" {
		return storage.Snippet{}, fmt.Errorf("name and command are required")
	}
	return a.store.UpdateSnippet(id, input)
}

// DeleteSnippet removes a snippet.
func (a *App) DeleteSnippet(id int64) error {
	if err := a.requireStore(); err != nil {
		return err
	}
	return a.store.DeleteSnippet(id)
}

// SendToTab writes arbitrary text to a terminal tab's stdin — used to run a
// snippet in the active session. A trailing newline (to actually execute the
// command) is the caller's responsibility.
func (a *App) SendToTab(tabID, text string) error {
	return a.WriteTerminalInput(tabID, text)
}
