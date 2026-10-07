-- Einstellungen je Backup (Integration + Backup-ID aus dem Snapshot).
-- max_age_hours: ab diesem Alter des letzten Laufs überfällig (statt Standard).
CREATE TABLE backup_settings (
    integration_id INTEGER NOT NULL REFERENCES integrations (id) ON DELETE CASCADE,
    item_id        TEXT    NOT NULL,
    max_age_hours  INTEGER NOT NULL,
    PRIMARY KEY (integration_id, item_id)
);
