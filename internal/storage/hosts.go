package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound is returned when a lookup by ID matches no row.
var ErrNotFound = errors.New("not found")

// ListHosts returns all hosts ordered by label, including their tags.
func (s *Store) ListHosts() ([]Host, error) {
	rows, err := s.db.Query(`
		SELECT id, label, hostname, port, user, identity_files, proxy_jump,
		       forward_agent, auth_method, protocol, remote_path, folder_id, credential_id, favorite, source, notes,
		       login_script, control_panel_url, last_used_at, created_at, updated_at
		FROM hosts ORDER BY label COLLATE NOCASE ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hosts := []Host{} // non-nil: a nil slice marshals to JSON null, which the frontend's {#each} cannot iterate
	for rows.Next() {
		h, err := scanHost(rows)
		if err != nil {
			return nil, err
		}
		hosts = append(hosts, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	tagsByHost, err := s.tagsByHost()
	if err != nil {
		return nil, err
	}
	for i := range hosts {
		if tags := tagsByHost[hosts[i].ID]; tags != nil {
			hosts[i].Tags = tags
		}
	}
	return hosts, nil
}

// GetHost returns a single host by ID.
func (s *Store) GetHost(id int64) (Host, error) {
	row := s.db.QueryRow(`
		SELECT id, label, hostname, port, user, identity_files, proxy_jump,
		       forward_agent, auth_method, protocol, remote_path, folder_id, credential_id, favorite, source, notes,
		       login_script, control_panel_url, last_used_at, created_at, updated_at
		FROM hosts WHERE id = ?`, id)
	h, err := scanHost(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Host{}, ErrNotFound
	}
	if err != nil {
		return Host{}, err
	}
	tags, err := s.tagsForHost(h.ID)
	if err != nil {
		return Host{}, err
	}
	if tags != nil {
		h.Tags = tags
	}
	return h, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanHost(row rowScanner) (Host, error) {
	var h Host
	var identityFilesJSON string
	var folderID sql.NullInt64
	var credentialID sql.NullInt64
	var lastUsedAt sql.NullString

	err := row.Scan(&h.ID, &h.Label, &h.Hostname, &h.Port, &h.User, &identityFilesJSON,
		&h.ProxyJump, &h.ForwardAgent, &h.AuthMethod, &h.Protocol, &h.RemotePath, &folderID, &credentialID, &h.Favorite, &h.Source,
		&h.Notes, &h.LoginScript, &h.ControlPanelURL, &lastUsedAt, &h.CreatedAt, &h.UpdatedAt)
	if err != nil {
		return Host{}, err
	}

	if folderID.Valid {
		h.FolderID = &folderID.Int64
	}
	if credentialID.Valid {
		h.CredentialID = &credentialID.Int64
	}
	if lastUsedAt.Valid {
		h.LastUsedAt = &lastUsedAt.String
	}
	if identityFilesJSON == "" {
		identityFilesJSON = "[]"
	}
	if err := json.Unmarshal([]byte(identityFilesJSON), &h.IdentityFiles); err != nil {
		return Host{}, fmt.Errorf("decode identity_files: %w", err)
	}
	if h.IdentityFiles == nil {
		h.IdentityFiles = []string{}
	}
	// Always a non-nil slice: a nil slice marshals to JSON null, and the
	// frontend iterates host.tags directly (for...of), which throws on null.
	h.Tags = []string{}
	return h, nil
}

// CreateHost inserts a manually-defined or imported host and returns it with
// its assigned ID.
func (s *Store) CreateHost(input HostInput, source HostSource) (Host, error) {
	identityFilesJSON, err := json.Marshal(nonNilStrings(input.IdentityFiles))
	if err != nil {
		return Host{}, err
	}
	res, err := s.db.Exec(`
		INSERT INTO hosts (label, hostname, port, user, identity_files, proxy_jump,
			forward_agent, auth_method, protocol, remote_path, folder_id, credential_id, source, notes, login_script, control_panel_url, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`,
		input.Label, input.Hostname, portOrDefault(input.Port), input.User, string(identityFilesJSON),
		input.ProxyJump, input.ForwardAgent, authMethodOrDefault(input.AuthMethod), protocolOrDefault(input.Protocol),
		remotePathOrDefault(input.RemotePath), input.FolderID, input.CredentialID, source, input.Notes, input.LoginScript, input.ControlPanelURL)
	if err != nil {
		return Host{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Host{}, err
	}
	if err := s.setHostTags(id, input.Tags); err != nil {
		return Host{}, err
	}
	return s.GetHost(id)
}

// UpdateHost overwrites the mutable fields of an existing host.
func (s *Store) UpdateHost(id int64, input HostInput) (Host, error) {
	identityFilesJSON, err := json.Marshal(nonNilStrings(input.IdentityFiles))
	if err != nil {
		return Host{}, err
	}
	res, err := s.db.Exec(`
		UPDATE hosts SET label = ?, hostname = ?, port = ?, user = ?, identity_files = ?,
			proxy_jump = ?, forward_agent = ?, auth_method = ?, protocol = ?, remote_path = ?, folder_id = ?, credential_id = ?, notes = ?,
			login_script = ?, control_panel_url = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id = ?`,
		input.Label, input.Hostname, portOrDefault(input.Port), input.User, string(identityFilesJSON),
		input.ProxyJump, input.ForwardAgent, authMethodOrDefault(input.AuthMethod), protocolOrDefault(input.Protocol),
		remotePathOrDefault(input.RemotePath), input.FolderID, input.CredentialID, input.Notes, input.LoginScript, input.ControlPanelURL, id)
	if err != nil {
		return Host{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Host{}, ErrNotFound
	}
	if err := s.setHostTags(id, input.Tags); err != nil {
		return Host{}, err
	}
	return s.GetHost(id)
}

// DeleteHost removes a host and its tag associations.
func (s *Store) DeleteHost(id int64) error {
	res, err := s.db.Exec(`DELETE FROM hosts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetFavorite toggles the favorite flag for a host.
func (s *Store) SetFavorite(id int64, favorite bool) error {
	res, err := s.db.Exec(`UPDATE hosts SET favorite = ? WHERE id = ?`, favorite, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// TouchLastUsed stamps a host as just-connected, for the "recently used"
// sidebar section.
func (s *Store) TouchLastUsed(id int64) error {
	_, err := s.db.Exec(`UPDATE hosts SET last_used_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// UpsertImportedHost inserts or updates a host that originated from
// ~/.ssh/config, matched by label. Manually-edited fields on a previously
// imported host are overwritten to reflect the config file on re-import.
func (s *Store) UpsertImportedHost(input HostInput) (Host, error) {
	var existingID int64
	err := s.db.QueryRow(`SELECT id FROM hosts WHERE label = ? AND source = ?`, input.Label, HostSourceSSHConfig).Scan(&existingID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return s.CreateHost(input, HostSourceSSHConfig)
	case err != nil:
		return Host{}, err
	default:
		return s.UpdateHost(existingID, input)
	}
}

func (s *Store) setHostTags(hostID int64, names []string) error {
	if _, err := s.db.Exec(`DELETE FROM host_tags WHERE host_id = ?`, hostID); err != nil {
		return err
	}
	for _, name := range names {
		if name == "" {
			continue
		}
		if _, err := s.db.Exec(`INSERT OR IGNORE INTO tags (name) VALUES (?)`, name); err != nil {
			return err
		}
		var tagID int64
		if err := s.db.QueryRow(`SELECT id FROM tags WHERE name = ?`, name).Scan(&tagID); err != nil {
			return err
		}
		if _, err := s.db.Exec(`INSERT OR IGNORE INTO host_tags (host_id, tag_id) VALUES (?, ?)`, hostID, tagID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) tagsForHost(hostID int64) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT t.name FROM tags t
		JOIN host_tags ht ON ht.tag_id = t.id
		WHERE ht.host_id = ? ORDER BY t.name`, hostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

func (s *Store) tagsByHost() (map[int64][]string, error) {
	rows, err := s.db.Query(`
		SELECT ht.host_id, t.name FROM host_tags ht
		JOIN tags t ON t.id = ht.tag_id
		ORDER BY t.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[int64][]string{}
	for rows.Next() {
		var hostID int64
		var name string
		if err := rows.Scan(&hostID, &name); err != nil {
			return nil, err
		}
		result[hostID] = append(result[hostID], name)
	}
	return result, rows.Err()
}

func portOrDefault(p int) int {
	if p <= 0 {
		return 22
	}
	return p
}

func authMethodOrDefault(m AuthMethod) AuthMethod {
	if m == "" {
		return AuthMethodAgent
	}
	return m
}

func protocolOrDefault(protocol HostProtocol) HostProtocol {
	switch protocol {
	case HostProtocolSFTP:
		return HostProtocolSFTP
	case HostProtocolWeb:
		return HostProtocolWeb
	default:
		return HostProtocolSSH
	}
}

func remotePathOrDefault(remotePath string) string {
	if remotePath == "" {
		return "."
	}
	return remotePath
}

func nonNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
