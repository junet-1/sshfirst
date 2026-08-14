package storage

import (
	"database/sql"
)

// ListFolders returns all folders in persisted sibling order. The frontend
// assembles the hierarchy from ParentID while preserving that order.
func (s *Store) ListFolders() ([]Folder, error) {
	rows, err := s.db.Query(`SELECT id, name, icon, parent_id, sort_order FROM folders ORDER BY sort_order ASC, name COLLATE NOCASE ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	folders := []Folder{} // non-nil: a nil slice marshals to JSON null, which the frontend's {#each} cannot iterate
	for rows.Next() {
		var f Folder
		var parentID sql.NullInt64
		if err := rows.Scan(&f.ID, &f.Name, &f.Icon, &parentID, &f.SortOrder); err != nil {
			return nil, err
		}
		if parentID.Valid {
			f.ParentID = &parentID.Int64
		}
		folders = append(folders, f)
	}
	return folders, rows.Err()
}

// CreateFolder inserts a new folder, optionally nested under parentID.
func (s *Store) CreateFolder(name string, parentID *int64) (Folder, error) {
	return s.CreateFolderWithIcon(name, parentID, "folder")
}

// CreateFolderWithIcon inserts a folder with one of the UI's symbolic icons.
func (s *Store) CreateFolderWithIcon(name string, parentID *int64, icon string) (Folder, error) {
	res, err := s.db.Exec(`
		INSERT INTO folders (name, icon, parent_id, sort_order)
		VALUES (?, ?, ?, (SELECT COALESCE(MAX(sort_order) + 1, 0) FROM folders WHERE parent_id IS ?))`,
		name, icon, parentID, parentID)
	if err != nil {
		return Folder{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Folder{}, err
	}
	var folder Folder
	var storedParentID sql.NullInt64
	if err := s.db.QueryRow(`SELECT id, name, icon, parent_id, sort_order FROM folders WHERE id = ?`, id).
		Scan(&folder.ID, &folder.Name, &folder.Icon, &storedParentID, &folder.SortOrder); err != nil {
		return Folder{}, err
	}
	if storedParentID.Valid {
		folder.ParentID = &storedParentID.Int64
	}
	return folder, nil
}

// RenameFolder updates a folder's display name.
func (s *Store) RenameFolder(id int64, name string) error {
	res, err := s.db.Exec(`UPDATE folders SET name = ? WHERE id = ?`, name, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateFolder changes the user-visible name and symbolic icon together.
func (s *Store) UpdateFolder(id int64, name, icon string) (Folder, error) {
	res, err := s.db.Exec(`UPDATE folders SET name = ?, icon = ? WHERE id = ?`, name, icon, id)
	if err != nil {
		return Folder{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Folder{}, ErrNotFound
	}
	var folder Folder
	var parentID sql.NullInt64
	if err := s.db.QueryRow(`SELECT id, name, icon, parent_id, sort_order FROM folders WHERE id = ?`, id).
		Scan(&folder.ID, &folder.Name, &folder.Icon, &parentID, &folder.SortOrder); err != nil {
		if err == sql.ErrNoRows {
			return Folder{}, ErrNotFound
		}
		return Folder{}, err
	}
	if parentID.Valid {
		folder.ParentID = &parentID.Int64
	}
	return folder, nil
}

// DeleteFolder removes a folder and its nested folders. Their hosts are kept
// and appended to the root sidebar list in deterministic order.
func (s *Store) DeleteFolder(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// A parent deletion cascades to nested folders. Move every affected host to
	// the end of the root list explicitly before that cascade so their relative
	// order remains deterministic instead of colliding on old sort positions.
	rows, err := tx.Query(`
		WITH RECURSIVE subtree(id, path) AS (
			SELECT id, printf('%020d', sort_order) FROM folders WHERE id = ?
			UNION ALL
			SELECT child.id, subtree.path || '/' || printf('%020d', child.sort_order)
			FROM folders AS child JOIN subtree ON child.parent_id = subtree.id
		)
		SELECT host.id FROM hosts AS host
		JOIN subtree ON host.folder_id = subtree.id
		ORDER BY subtree.path ASC, host.sort_order ASC, host.label COLLATE NOCASE ASC, host.id ASC`, id)
	if err != nil {
		return err
	}
	movedHostIDs, err := scanIDs(rows)
	rows.Close()
	if err != nil {
		return err
	}
	rootRows, err := tx.Query(`
		SELECT id FROM hosts WHERE folder_id IS NULL
		ORDER BY sort_order ASC, label COLLATE NOCASE ASC, id ASC`)
	if err != nil {
		return err
	}
	rootHostIDs, err := scanIDs(rootRows)
	rootRows.Close()
	if err != nil {
		return err
	}
	for _, hostID := range movedHostIDs {
		if _, err := tx.Exec(`UPDATE hosts SET folder_id = NULL WHERE id = ?`, hostID); err != nil {
			return err
		}
	}
	if err := writeHostOrder(tx, append(rootHostIDs, movedHostIDs...)); err != nil {
		return err
	}

	res, err := tx.Exec(`DELETE FROM folders WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// MoveHostToFolder re-parents a host, or clears its folder when folderID is nil.
func (s *Store) MoveHostToFolder(hostID int64, folderID *int64) error {
	return s.ReorderHost(hostID, folderID, nil, false)
}

// MoveFolder re-parents a folder under parentID (nil = top level). It refuses
// to create a cycle (moving a folder into itself or one of its descendants).
func (s *Store) MoveFolder(id int64, parentID *int64) error {
	return s.ReorderFolder(id, parentID, nil, false)
}
