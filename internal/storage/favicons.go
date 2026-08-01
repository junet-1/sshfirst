package storage

import (
	"database/sql"
	"errors"
)

// GetFavicon returns the cached favicon data URL for an origin, if present.
func (s *Store) GetFavicon(origin string) (string, bool, error) {
	var dataURL string
	err := s.db.QueryRow(`SELECT data_url FROM favicon_cache WHERE origin = ?`, origin).Scan(&dataURL)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return dataURL, true, nil
}

// SetFavicon inserts or refreshes the cached favicon for an origin.
func (s *Store) SetFavicon(origin, dataURL string) error {
	_, err := s.db.Exec(`
		INSERT INTO favicon_cache (origin, data_url, fetched_at)
		VALUES (?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		ON CONFLICT(origin) DO UPDATE SET data_url = excluded.data_url, fetched_at = excluded.fetched_at`,
		origin, dataURL)
	return err
}
