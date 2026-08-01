-- Cached web-panel favicons, keyed by origin (scheme://host:port) so hosts on
-- the same domain share one entry. Stored as a data: URL. Lets the sidebar show
-- a panel's favicon persistently — offline and across restarts — without
-- re-fetching on every render. Purely a display cache; safe to clear.
CREATE TABLE IF NOT EXISTS favicon_cache (
    origin     TEXT PRIMARY KEY,
    data_url   TEXT NOT NULL,
    fetched_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
