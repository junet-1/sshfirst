-- Snippets: reusable commands sent into a session on demand. A snippet with
-- host_id = NULL is global (available everywhere); a host-scoped snippet only
-- shows for that host.
CREATE TABLE IF NOT EXISTS snippets (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    command    TEXT NOT NULL,
    host_id    INTEGER REFERENCES hosts(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_snippets_host_id ON snippets(host_id);

-- Optional per-host login script: sent to the shell automatically right after
-- the session opens (e.g. "tmux attach || tmux new", "cd /var/www").
ALTER TABLE hosts ADD COLUMN login_script TEXT NOT NULL DEFAULT '';
