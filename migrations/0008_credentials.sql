-- Reusable credentials: the "who + how to log in" (user, auth method, identity
-- files) defined once and referenced by many hosts, so shared logins aren't
-- re-entered per host. Passwords live in the Secret Service under
-- credential-password:<id>, never here. Hybrid model: a host may reference a
-- credential (credential_id) OR keep its own inline user/auth fields (NULL).
CREATE TABLE IF NOT EXISTS credentials (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    name           TEXT NOT NULL,
    user           TEXT NOT NULL DEFAULT '',
    auth_method    TEXT NOT NULL DEFAULT 'agent',
    identity_files TEXT NOT NULL DEFAULT '[]',
    created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- ON DELETE SET NULL: deleting a credential reverts referencing hosts to their
-- own inline auth fields rather than deleting the hosts (FKs are enforced, see
-- storage.Open's foreign_keys pragma).
ALTER TABLE hosts ADD COLUMN credential_id INTEGER REFERENCES credentials(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_hosts_credential_id ON hosts(credential_id);
