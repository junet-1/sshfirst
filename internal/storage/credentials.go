package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// ListCredentials returns all reusable credentials ordered by name.
func (s *Store) ListCredentials() ([]Credential, error) {
	rows, err := s.db.Query(`
		SELECT id, name, user, auth_method, identity_files, created_at, updated_at
		FROM credentials ORDER BY name COLLATE NOCASE ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	creds := []Credential{} // non-nil so it marshals to [] not null for the frontend
	for rows.Next() {
		c, err := scanCredential(rows)
		if err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	return creds, rows.Err()
}

// GetCredential returns a single credential by ID.
func (s *Store) GetCredential(id int64) (Credential, error) {
	row := s.db.QueryRow(`
		SELECT id, name, user, auth_method, identity_files, created_at, updated_at
		FROM credentials WHERE id = ?`, id)
	c, err := scanCredential(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Credential{}, ErrNotFound
	}
	return c, err
}

func scanCredential(row rowScanner) (Credential, error) {
	var c Credential
	var identityFilesJSON string
	if err := row.Scan(&c.ID, &c.Name, &c.User, &c.AuthMethod, &identityFilesJSON, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return Credential{}, err
	}
	if identityFilesJSON == "" {
		identityFilesJSON = "[]"
	}
	if err := json.Unmarshal([]byte(identityFilesJSON), &c.IdentityFiles); err != nil {
		return Credential{}, fmt.Errorf("decode identity_files: %w", err)
	}
	if c.IdentityFiles == nil {
		c.IdentityFiles = []string{}
	}
	return c, nil
}

// CreateCredential inserts a new reusable credential and returns it with its ID.
func (s *Store) CreateCredential(input CredentialInput) (Credential, error) {
	identityFilesJSON, err := json.Marshal(nonNilStrings(input.IdentityFiles))
	if err != nil {
		return Credential{}, err
	}
	res, err := s.db.Exec(`
		INSERT INTO credentials (name, user, auth_method, identity_files, updated_at)
		VALUES (?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`,
		input.Name, input.User, authMethodOrDefault(input.AuthMethod), string(identityFilesJSON))
	if err != nil {
		return Credential{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Credential{}, err
	}
	return s.GetCredential(id)
}

// UpdateCredential overwrites the mutable fields of an existing credential.
func (s *Store) UpdateCredential(id int64, input CredentialInput) (Credential, error) {
	identityFilesJSON, err := json.Marshal(nonNilStrings(input.IdentityFiles))
	if err != nil {
		return Credential{}, err
	}
	res, err := s.db.Exec(`
		UPDATE credentials SET name = ?, user = ?, auth_method = ?, identity_files = ?,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id = ?`,
		input.Name, input.User, authMethodOrDefault(input.AuthMethod), string(identityFilesJSON), id)
	if err != nil {
		return Credential{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Credential{}, ErrNotFound
	}
	return s.GetCredential(id)
}

// DeleteCredential removes a credential. Hosts referencing it revert to their
// own inline auth fields via the credential_id ON DELETE SET NULL foreign key.
func (s *Store) DeleteCredential(id int64) error {
	res, err := s.db.Exec(`DELETE FROM credentials WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
