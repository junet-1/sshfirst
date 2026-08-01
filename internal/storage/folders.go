package storage

import (
	"database/sql"
	"fmt"
)

// ListFolders returns all folders, unordered by hierarchy (the frontend
// assembles the tree from ParentID).
func (s *Store) ListFolders() ([]Folder, error) {
	rows, err := s.db.Query(`SELECT id, name, icon, parent_id FROM folders ORDER BY name COLLATE NOCASE ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	folders := []Folder{} // non-nil: a nil slice marshals to JSON null, which the frontend's {#each} cannot iterate
	for rows.Next() {
		var f Folder
		var parentID sql.NullInt64
		if err := rows.Scan(&f.ID, &f.Name, &f.Icon, &parentID); err != nil {
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
	res, err := s.db.Exec(`INSERT INTO folders (name, icon, parent_id) VALUES (?, ?, ?)`, name, icon, parentID)
	if err != nil {
		return Folder{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Folder{}, err
	}
	return Folder{ID: id, Name: name, Icon: icon, ParentID: parentID}, nil
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
	if err := s.db.QueryRow(`SELECT id, name, icon, parent_id FROM folders WHERE id = ?`, id).
		Scan(&folder.ID, &folder.Name, &folder.Icon, &parentID); err != nil {
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

// DeleteFolder removes a folder; hosts inside it fall back to no folder
// (folder_id = NULL) via the ON DELETE SET NULL constraint.
func (s *Store) DeleteFolder(id int64) error {
	res, err := s.db.Exec(`DELETE FROM folders WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// MoveHostToFolder re-parents a host, or clears its folder when folderID is nil.
func (s *Store) MoveHostToFolder(hostID int64, folderID *int64) error {
	res, err := s.db.Exec(`UPDATE hosts SET folder_id = ? WHERE id = ?`, folderID, hostID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// MoveFolder re-parents a folder under parentID (nil = top level). It refuses
// to create a cycle (moving a folder into itself or one of its descendants).
func (s *Store) MoveFolder(id int64, parentID *int64) error {
	if parentID != nil {
		if *parentID == id {
			return fmt.Errorf("cannot move a folder into itself")
		}
		descendant, err := s.isDescendant(*parentID, id)
		if err != nil {
			return err
		}
		if descendant {
			return fmt.Errorf("cannot move a folder into one of its own subfolders")
		}
	}
	res, err := s.db.Exec(`UPDATE folders SET parent_id = ? WHERE id = ?`, parentID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// isDescendant reports whether candidate is equal to ancestor or nested
// somewhere beneath it, by walking parent links upward from candidate.
func (s *Store) isDescendant(candidate, ancestor int64) (bool, error) {
	current := candidate
	for range 128 { // hard cap guards against a pre-existing corrupt cycle
		if current == ancestor {
			return true, nil
		}
		var parent sql.NullInt64
		if err := s.db.QueryRow(`SELECT parent_id FROM folders WHERE id = ?`, current).Scan(&parent); err != nil {
			if err == sql.ErrNoRows {
				return false, nil
			}
			return false, err
		}
		if !parent.Valid {
			return false, nil
		}
		current = parent.Int64
	}
	return true, nil
}
