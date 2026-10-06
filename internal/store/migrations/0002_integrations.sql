-- Integrationen (Datenquellen). config: nicht geheime Werte als JSON (inkl.
-- Verweisen ${SECRET_…}); secrets: verschlüsselte Zugangsdaten (secrets.Box).
CREATE TABLE integrations (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    type             TEXT    NOT NULL,
    name             TEXT    NOT NULL,
    enabled          INTEGER NOT NULL DEFAULT 1,
    interval_seconds INTEGER NOT NULL,
    timeout_seconds  INTEGER NOT NULL,
    tls_verify       INTEGER NOT NULL DEFAULT 1,
    config           TEXT    NOT NULL DEFAULT '{}',
    secrets          TEXT    NOT NULL DEFAULT '',
    created_at       TEXT    NOT NULL,
    updated_at       TEXT    NOT NULL,
    -- Ergebnis der letzten Abfrage
    status           TEXT    NOT NULL DEFAULT 'unknown',
    last_poll_at     TEXT,
    last_success_at  TEXT,
    last_error       TEXT    NOT NULL DEFAULT '', -- i18n.Msg als JSON
    last_duration_ms INTEGER NOT NULL DEFAULT 0,
    snapshot         TEXT    NOT NULL DEFAULT ''  -- letzter erfolgreicher Snapshot (JSON)
);

-- Kennzahlen: jede Abfrage (Rohdaten) …
CREATE TABLE metric_samples (
    integration_id INTEGER NOT NULL REFERENCES integrations (id) ON DELETE CASCADE,
    key            TEXT    NOT NULL,
    at             TEXT    NOT NULL,
    value          REAL    NOT NULL
);
CREATE INDEX idx_metric_samples_key ON metric_samples (integration_id, key, at);
CREATE INDEX idx_metric_samples_at ON metric_samples (at);

-- … und ein Wert pro Tag (UTC) für lange Zeiträume.
CREATE TABLE metric_daily (
    integration_id INTEGER NOT NULL REFERENCES integrations (id) ON DELETE CASCADE,
    key            TEXT    NOT NULL,
    day            TEXT    NOT NULL, -- YYYY-MM-DD
    last           REAL    NOT NULL,
    min            REAL    NOT NULL,
    max            REAL    NOT NULL,
    PRIMARY KEY (integration_id, key, day)
);
CREATE INDEX idx_metric_daily_day ON metric_daily (day);

-- Statuswechsel der Integration selbst und ihrer Checks, Backups und
-- Kennzahlen → Verlauf, Uptime, Benachrichtigungen.
CREATE TABLE status_changes (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    integration_id INTEGER NOT NULL REFERENCES integrations (id) ON DELETE CASCADE,
    kind           TEXT    NOT NULL CHECK (kind IN ('integration', 'check', 'backup', 'metric')),
    item_id        TEXT    NOT NULL DEFAULT '',
    item_name      TEXT    NOT NULL DEFAULT '',
    from_status    TEXT    NOT NULL,
    to_status      TEXT    NOT NULL,
    message        TEXT    NOT NULL DEFAULT '', -- i18n.Msg als JSON
    at             TEXT    NOT NULL
);
CREATE INDEX idx_status_changes_item ON status_changes (integration_id, kind, item_id, at);
CREATE INDEX idx_status_changes_at ON status_changes (at);
