-- The UNIQUE(label, source) constraint from 0001 was meant to make re-running
-- the ~/.ssh/config import idempotent, but UpsertImportedHost already does
-- that via an explicit SELECT-then-insert-or-update — the constraint wasn't
-- actually needed for that, and it wrongly blocked manually adding two hosts
-- that happen to share a label, with a cryptic SQL error.
--
-- SQLite has no ALTER TABLE ... DROP CONSTRAINT, so the table is rebuilt.
-- host_tags is dropped and recreated first (rather than toggling
-- PRAGMA foreign_keys, which is a no-op inside a transaction) so dropping
-- the old hosts table never triggers an ON DELETE CASCADE against it.

CREATE TABLE hosts_new (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    label          TEXT NOT NULL,
    hostname       TEXT NOT NULL,
    port           INTEGER NOT NULL DEFAULT 22,
    user           TEXT NOT NULL DEFAULT '',
    identity_files TEXT NOT NULL DEFAULT '[]',
    proxy_jump     TEXT NOT NULL DEFAULT '',
    forward_agent  INTEGER NOT NULL DEFAULT 0,
    auth_method    TEXT NOT NULL DEFAULT 'agent',
    folder_id      INTEGER REFERENCES folders(id) ON DELETE SET NULL,
    favorite       INTEGER NOT NULL DEFAULT 0,
    source         TEXT NOT NULL DEFAULT 'manual',
    notes          TEXT NOT NULL DEFAULT '',
    last_used_at   TEXT,
    created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

INSERT INTO hosts_new (id, label, hostname, port, user, identity_files, proxy_jump,
    forward_agent, auth_method, folder_id, favorite, source, notes, last_used_at,
    created_at, updated_at)
SELECT id, label, hostname, port, user, identity_files, proxy_jump,
    forward_agent, auth_method, folder_id, favorite, source, notes, last_used_at,
    created_at, updated_at
FROM hosts;

CREATE TABLE host_tags_new (
    host_id INTEGER NOT NULL REFERENCES hosts_new(id) ON DELETE CASCADE,
    tag_id  INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (host_id, tag_id)
);

INSERT INTO host_tags_new (host_id, tag_id) SELECT host_id, tag_id FROM host_tags;

DROP TABLE host_tags;
DROP TABLE hosts;

ALTER TABLE hosts_new RENAME TO hosts;
ALTER TABLE host_tags_new RENAME TO host_tags;

CREATE INDEX IF NOT EXISTS idx_hosts_folder_id ON hosts(folder_id);
CREATE INDEX IF NOT EXISTS idx_hosts_favorite ON hosts(favorite);
