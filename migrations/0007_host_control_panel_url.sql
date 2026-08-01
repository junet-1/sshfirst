-- Optional per-host web control-panel URL (e.g. https://proxmox.example.com).
-- Opened as an embedded browser tab in the workspace, next to terminal/SFTP
-- tabs. Never a secret, so it lives here rather than in the Secret Service.
ALTER TABLE hosts ADD COLUMN control_panel_url TEXT NOT NULL DEFAULT '';
