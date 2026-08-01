-- SSH First initial schema.
-- Secrets (passwords, key passphrases) are NEVER stored here — see internal/secrets.

CREATE TABLE IF NOT EXISTS folders (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    name      TEXT NOT NULL,
    parent_id INTEGER REFERENCES folders(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS tags (
    id   INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS hosts (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    label          TEXT NOT NULL,
    hostname       TEXT NOT NULL,
    port           INTEGER NOT NULL DEFAULT 22,
    user           TEXT NOT NULL DEFAULT '',
    identity_files TEXT NOT NULL DEFAULT '[]', -- JSON array of paths
    proxy_jump     TEXT NOT NULL DEFAULT '',
    forward_agent  INTEGER NOT NULL DEFAULT 0,
    auth_method    TEXT NOT NULL DEFAULT 'agent', -- agent | identity | password
    folder_id      INTEGER REFERENCES folders(id) ON DELETE SET NULL,
    favorite       INTEGER NOT NULL DEFAULT 0,
    source         TEXT NOT NULL DEFAULT 'manual', -- manual | ssh_config
    notes          TEXT NOT NULL DEFAULT '',
    last_used_at   TEXT,
    created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (label, source)
);

CREATE TABLE IF NOT EXISTS host_tags (
    host_id INTEGER NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    tag_id  INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (host_id, tag_id)
);

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_hosts_folder_id ON hosts(folder_id);
CREATE INDEX IF NOT EXISTS idx_hosts_favorite ON hosts(favorite);
