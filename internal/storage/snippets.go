package storage

import "database/sql"

// Snippet is a reusable command. HostID is nil for a global snippet.
type Snippet struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Command string `json:"command"`
	HostID  *int64 `json:"hostId,omitempty"`
}

// SnippetInput is the payload for creating/updating a snippet.
type SnippetInput struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	HostID  *int64 `json:"hostId,omitempty"`
}

// ListSnippets returns global snippets plus, if hostID is non-nil, those
// scoped to that host. Ordered by name.
func (s *Store) ListSnippets(hostID *int64) ([]Snippet, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if hostID != nil {
		rows, err = s.db.Query(`
			SELECT id, name, command, host_id FROM snippets
			WHERE host_id IS NULL OR host_id = ?
			ORDER BY name COLLATE NOCASE ASC`, *hostID)
	} else {
		rows, err = s.db.Query(`
			SELECT id, name, command, host_id FROM snippets
			ORDER BY name COLLATE NOCASE ASC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	snippets := []Snippet{}
	for rows.Next() {
		var sn Snippet
		var hid sql.NullInt64
		if err := rows.Scan(&sn.ID, &sn.Name, &sn.Command, &hid); err != nil {
			return nil, err
		}
		if hid.Valid {
			sn.HostID = &hid.Int64
		}
		snippets = append(snippets, sn)
	}
	return snippets, rows.Err()
}

// CreateSnippet inserts a snippet and returns it with its assigned ID.
func (s *Store) CreateSnippet(input SnippetInput) (Snippet, error) {
	res, err := s.db.Exec(`INSERT INTO snippets (name, command, host_id) VALUES (?, ?, ?)`,
		input.Name, input.Command, input.HostID)
	if err != nil {
		return Snippet{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Snippet{}, err
	}
	return Snippet{ID: id, Name: input.Name, Command: input.Command, HostID: input.HostID}, nil
}

// UpdateSnippet overwrites an existing snippet's fields.
func (s *Store) UpdateSnippet(id int64, input SnippetInput) (Snippet, error) {
	res, err := s.db.Exec(`UPDATE snippets SET name = ?, command = ?, host_id = ? WHERE id = ?`,
		input.Name, input.Command, input.HostID, id)
	if err != nil {
		return Snippet{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Snippet{}, ErrNotFound
	}
	return Snippet{ID: id, Name: input.Name, Command: input.Command, HostID: input.HostID}, nil
}

// DeleteSnippet removes a snippet.
func (s *Store) DeleteSnippet(id int64) error {
	res, err := s.db.Exec(`DELETE FROM snippets WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
