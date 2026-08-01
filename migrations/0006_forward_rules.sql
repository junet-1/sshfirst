-- Per-host SSH port-forwarding rules (ssh -L / -R / -D).
-- No secrets here; these are just connection routing definitions.

CREATE TABLE IF NOT EXISTS forward_rules (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    host_id    INTEGER NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL,                 -- local | remote | dynamic
    label      TEXT NOT NULL DEFAULT '',
    bind_addr  TEXT NOT NULL DEFAULT '',      -- empty means loopback (127.0.0.1)
    bind_port  INTEGER NOT NULL,
    dest_host  TEXT NOT NULL DEFAULT '',      -- unused for dynamic
    dest_port  INTEGER NOT NULL DEFAULT 0,    -- unused for dynamic
    auto_start INTEGER NOT NULL DEFAULT 0,    -- start automatically on connect
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_forward_rules_host_id ON forward_rules(host_id);
