package storage

import (
	"database/sql"
	"fmt"
)

// ReorderHost moves hostID into folderID and places it before or after
// targetHostID. A nil target appends it to the destination folder/root.
// Every affected sibling gets a compact, deterministic position in one
// transaction so a restart observes exactly the order shown after the drop.
func (s *Store) ReorderHost(hostID int64, folderID, targetHostID *int64, before bool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var oldFolder sql.NullInt64
	if err := tx.QueryRow(`SELECT folder_id FROM hosts WHERE id = ?`, hostID).Scan(&oldFolder); err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return err
	}
	if targetHostID != nil {
		if *targetHostID == hostID {
			return nil
		}
		var targetFolder sql.NullInt64
		if err := tx.QueryRow(`SELECT folder_id FROM hosts WHERE id = ?`, *targetHostID).Scan(&targetFolder); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("target host: %w", ErrNotFound)
			}
			return err
		}
		if !sameNullableID(targetFolder, folderID) {
			return fmt.Errorf("target host is not in the destination folder")
		}
	}

	ids, err := orderedHostIDs(tx, folderID, hostID)
	if err != nil {
		return err
	}
	ids, err = insertRelative(ids, hostID, targetHostID, before)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE hosts SET folder_id = ? WHERE id = ?`, folderID, hostID); err != nil {
		return err
	}
	if err := writeHostOrder(tx, ids); err != nil {
		return err
	}

	if !sameNullableID(oldFolder, folderID) {
		oldFolderID := nullableIDPointer(oldFolder)
		oldIDs, err := orderedHostIDs(tx, oldFolderID, hostID)
		if err != nil {
			return err
		}
		if err := writeHostOrder(tx, oldIDs); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ReorderFolder moves folderID under parentID and places it relative to a
// sibling. A nil target appends it. Dropping into a descendant is rejected.
func (s *Store) ReorderFolder(folderID int64, parentID, targetFolderID *int64, before bool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var oldParent sql.NullInt64
	if err := tx.QueryRow(`SELECT parent_id FROM folders WHERE id = ?`, folderID).Scan(&oldParent); err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return err
	}
	if parentID != nil {
		if *parentID == folderID {
			return fmt.Errorf("cannot move a folder into itself")
		}
		descendant, err := isDescendantTx(tx, *parentID, folderID)
		if err != nil {
			return err
		}
		if descendant {
			return fmt.Errorf("cannot move a folder into one of its own subfolders")
		}
	}
	if targetFolderID != nil {
		if *targetFolderID == folderID {
			return nil
		}
		var targetParent sql.NullInt64
		if err := tx.QueryRow(`SELECT parent_id FROM folders WHERE id = ?`, *targetFolderID).Scan(&targetParent); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("target folder: %w", ErrNotFound)
			}
			return err
		}
		if !sameNullableID(targetParent, parentID) {
			return fmt.Errorf("target folder is not under the destination parent")
		}
	}

	ids, err := orderedFolderIDs(tx, parentID, folderID)
	if err != nil {
		return err
	}
	ids, err = insertRelative(ids, folderID, targetFolderID, before)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE folders SET parent_id = ? WHERE id = ?`, parentID, folderID); err != nil {
		return err
	}
	if err := writeFolderOrder(tx, ids); err != nil {
		return err
	}

	if !sameNullableID(oldParent, parentID) {
		oldParentID := nullableIDPointer(oldParent)
		oldIDs, err := orderedFolderIDs(tx, oldParentID, folderID)
		if err != nil {
			return err
		}
		if err := writeFolderOrder(tx, oldIDs); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func orderedHostIDs(tx *sql.Tx, folderID *int64, excludedID int64) ([]int64, error) {
	rows, err := tx.Query(`
		SELECT id FROM hosts
		WHERE folder_id IS ? AND id <> ?
		ORDER BY sort_order ASC, label COLLATE NOCASE ASC, id ASC`, folderID, excludedID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIDs(rows)
}

func orderedFolderIDs(tx *sql.Tx, parentID *int64, excludedID int64) ([]int64, error) {
	rows, err := tx.Query(`
		SELECT id FROM folders
		WHERE parent_id IS ? AND id <> ?
		ORDER BY sort_order ASC, name COLLATE NOCASE ASC, id ASC`, parentID, excludedID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIDs(rows)
}

func scanIDs(rows *sql.Rows) ([]int64, error) {
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func insertRelative(ids []int64, movingID int64, targetID *int64, before bool) ([]int64, error) {
	index := len(ids)
	if targetID != nil {
		index = -1
		for i, id := range ids {
			if id == *targetID {
				index = i
				break
			}
		}
		if index < 0 {
			return nil, fmt.Errorf("reorder target: %w", ErrNotFound)
		}
		if !before {
			index++
		}
	}
	ids = append(ids, 0)
	copy(ids[index+1:], ids[index:])
	ids[index] = movingID
	return ids, nil
}

func writeHostOrder(tx *sql.Tx, ids []int64) error {
	for position, id := range ids {
		if _, err := tx.Exec(`UPDATE hosts SET sort_order = ? WHERE id = ?`, position, id); err != nil {
			return err
		}
	}
	return nil
}

func writeFolderOrder(tx *sql.Tx, ids []int64) error {
	for position, id := range ids {
		if _, err := tx.Exec(`UPDATE folders SET sort_order = ? WHERE id = ?`, position, id); err != nil {
			return err
		}
	}
	return nil
}

func sameNullableID(value sql.NullInt64, pointer *int64) bool {
	if pointer == nil {
		return !value.Valid
	}
	return value.Valid && value.Int64 == *pointer
}

func nullableIDPointer(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	id := value.Int64
	return &id
}

func isDescendantTx(tx *sql.Tx, candidate, ancestor int64) (bool, error) {
	current := candidate
	for range 128 {
		if current == ancestor {
			return true, nil
		}
		var parent sql.NullInt64
		if err := tx.QueryRow(`SELECT parent_id FROM folders WHERE id = ?`, current).Scan(&parent); err != nil {
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
