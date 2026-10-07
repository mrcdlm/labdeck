-- Webhooks (wie dnsdeck). Geheimnisse: als Platzhalter ${WEBHOOK_…}/${SECRET_…}
-- aus Env-Variablen oder im Feld token (verschlüsselt, secrets.Box), das als
-- ${TOKEN} bzw. {{token}} eingesetzt wird.
CREATE TABLE webhooks (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT NOT NULL,
    enabled         INTEGER NOT NULL DEFAULT 1,
    method          TEXT NOT NULL DEFAULT 'POST',
    url             TEXT NOT NULL,
    headers         TEXT NOT NULL DEFAULT '[]', -- JSON [{"name":…,"value":…}]
    content_type    TEXT NOT NULL DEFAULT 'application/json',
    body_template   TEXT NOT NULL DEFAULT '',   -- leer = Standard-JSON
    events          TEXT NOT NULL DEFAULT '[]', -- JSON ["check_down", …]
    token           TEXT NOT NULL DEFAULT '',   -- verschlüsselt
    last_sent_at    TEXT,
    last_error_i18n TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

-- Zuletzt ausgewerteter Zustand je Backup (ok, due_soon, overdue …) – damit
-- Wechsel wie „überfällig“ auch ohne neue Abfrage erkannt und nur einmal
-- gemeldet werden.
CREATE TABLE backup_states (
    integration_id INTEGER NOT NULL REFERENCES integrations (id) ON DELETE CASCADE,
    item_id        TEXT    NOT NULL,
    state          TEXT    NOT NULL,
    PRIMARY KEY (integration_id, item_id)
);

-- Gruppe des Eintrags (z. B. Benutzer, Datastore) für den Verlauf.
ALTER TABLE status_changes ADD COLUMN item_group TEXT NOT NULL DEFAULT '';
