-- Stable, user-controlled ordering for the sidebar tree. Existing installs
-- keep their previous alphabetical order as the initial position.
ALTER TABLE hosts ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;
ALTER TABLE folders ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;

WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY folder_id
               ORDER BY label COLLATE NOCASE ASC, id ASC
           ) - 1 AS position
    FROM hosts
)
UPDATE hosts
SET sort_order = (SELECT position FROM ranked WHERE ranked.id = hosts.id);

WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY parent_id
               ORDER BY name COLLATE NOCASE ASC, id ASC
           ) - 1 AS position
    FROM folders
)
UPDATE folders
SET sort_order = (SELECT position FROM ranked WHERE ranked.id = folders.id);

CREATE INDEX IF NOT EXISTS idx_hosts_sidebar_order
    ON hosts(folder_id, sort_order);
CREATE INDEX IF NOT EXISTS idx_folders_sidebar_order
    ON folders(parent_id, sort_order);
