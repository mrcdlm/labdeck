package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Integration ist eine eingerichtete Datenquelle samt Ergebnis der letzten
// Abfrage. Secrets ist verschlüsselt und wird nur vom Manager geöffnet.
type Integration struct {
	ID        int64
	Type      string
	Name      string
	Enabled   bool
	Interval  time.Duration
	Timeout   time.Duration
	TLSVerify bool
	Config    string // JSON
	Secrets   string // verschlüsselt
	CreatedAt time.Time
	UpdatedAt time.Time

	Status        string
	LastPollAt    *time.Time
	LastSuccessAt *time.Time
	LastError     string // i18n.Msg als JSON
	LastDuration  time.Duration
	Snapshot      string // JSON
}

const integrationCols = `id, type, name, enabled, interval_seconds, timeout_seconds, tls_verify, config, secrets,
	created_at, updated_at, status, last_poll_at, last_success_at, last_error, last_duration_ms, snapshot`

func scanIntegration(row interface{ Scan(...any) error }) (Integration, error) {
	var (
		it                 Integration
		interval, timeout  int64
		created, updated   string
		lastPoll, lastSucc sql.NullString
		durMS              int64
	)
	err := row.Scan(&it.ID, &it.Type, &it.Name, &it.Enabled, &interval, &timeout, &it.TLSVerify, &it.Config,
		&it.Secrets, &created, &updated, &it.Status, &lastPoll, &lastSucc, &it.LastError, &durMS, &it.Snapshot)
	if errors.Is(err, sql.ErrNoRows) {
		return it, ErrNotFound
	}
	if err != nil {
		return it, err
	}
	it.Interval = time.Duration(interval) * time.Second
	it.Timeout = time.Duration(timeout) * time.Second
	it.LastDuration = time.Duration(durMS) * time.Millisecond
	if it.CreatedAt, err = parseTime(created); err != nil {
		return it, err
	}
	if it.UpdatedAt, err = parseTime(updated); err != nil {
		return it, err
	}
	if it.LastPollAt, err = parseNullTime(lastPoll); err != nil {
		return it, err
	}
	if it.LastSuccessAt, err = parseNullTime(lastSucc); err != nil {
		return it, err
	}
	return it, nil
}

func parseNullTime(s sql.NullString) (*time.Time, error) {
	if !s.Valid {
		return nil, nil
	}
	t, err := parseTime(s.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// CreateIntegration legt eine Integration an. seal liefert die verschlüsselten
// Secrets und erhält die neue ID (Verschlüsselung ist an die ID gebunden);
// schlägt seal fehl, wird nichts gespeichert.
func (s *Store) CreateIntegration(ctx context.Context, it Integration, seal func(id int64) (string, error)) (Integration, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return it, err
	}
	defer tx.Rollback()
	now := formatTime(time.Now())
	res, err := tx.ExecContext(ctx, `INSERT INTO integrations
		(type, name, enabled, interval_seconds, timeout_seconds, tls_verify, config, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		it.Type, it.Name, it.Enabled, int64(it.Interval/time.Second), int64(it.Timeout/time.Second), it.TLSVerify,
		it.Config, now, now)
	if err != nil {
		return it, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return it, err
	}
	if seal != nil {
		sealed, err := seal(id)
		if err != nil {
			return it, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE integrations SET secrets = ? WHERE id = ?`, sealed, id); err != nil {
			return it, err
		}
	}
	if err := tx.Commit(); err != nil {
		return it, err
	}
	return s.GetIntegration(ctx, id)
}

// UpdateIntegration speichert die Einstellungen (nicht das Abfrageergebnis).
func (s *Store) UpdateIntegration(ctx context.Context, it Integration) (Integration, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE integrations SET name = ?, enabled = ?, interval_seconds = ?,
		timeout_seconds = ?, tls_verify = ?, config = ?, secrets = ?, updated_at = ? WHERE id = ?`,
		it.Name, it.Enabled, int64(it.Interval/time.Second), int64(it.Timeout/time.Second), it.TLSVerify,
		it.Config, it.Secrets, formatTime(time.Now()), it.ID)
	if err != nil {
		return it, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return it, ErrNotFound
	}
	return s.GetIntegration(ctx, it.ID)
}

// DeleteIntegration löscht die Integration samt Verlauf und Kennzahlen.
func (s *Store) DeleteIntegration(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM integrations WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetIntegration(ctx context.Context, id int64) (Integration, error) {
	return scanIntegration(s.db.QueryRowContext(ctx, `SELECT `+integrationCols+` FROM integrations WHERE id = ?`, id))
}

// ListIntegrations liefert alle Integrationen, sortiert nach Name.
func (s *Store) ListIntegrations(ctx context.Context) ([]Integration, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+integrationCols+` FROM integrations ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Integration
	for rows.Next() {
		it, err := scanIntegration(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// PollResult ist das Ergebnis einer Abfrage.
type PollResult struct {
	At       time.Time
	Duration time.Duration
	Status   string
	Error    string // i18n.Msg als JSON; leer = Erfolg
	// Snapshot: neuer Snapshot (JSON); leer = bisherigen behalten (Fehlschlag)
	Snapshot string
	Changes  []StatusChange
	Samples  []MetricSample
}

type MetricSample struct {
	Key   string
	Value float64
}

type StatusChange struct {
	ID            int64
	IntegrationID int64
	Kind          string // integration, check, backup, metric
	ItemID        string
	ItemName      string
	From, To      string
	Message       string // i18n.Msg als JSON
	At            time.Time
}

// SavePollResult speichert Ergebnis, Statuswechsel und Kennzahlen einer
// Abfrage in einer Transaktion. Wurde die Integration inzwischen gelöscht,
// liefert es ErrNotFound.
func (s *Store) SavePollResult(ctx context.Context, id int64, r PollResult) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	at := formatTime(r.At)
	q := `UPDATE integrations SET status = ?, last_poll_at = ?, last_error = ?, last_duration_ms = ?`
	args := []any{r.Status, at, r.Error, r.Duration.Milliseconds()}
	if r.Snapshot != "" {
		q += `, snapshot = ?, last_success_at = ?`
		args = append(args, r.Snapshot, at)
	}
	res, err := tx.ExecContext(ctx, q+` WHERE id = ?`, append(args, id)...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}

	for _, c := range r.Changes {
		if _, err := tx.ExecContext(ctx, `INSERT INTO status_changes
			(integration_id, kind, item_id, item_name, from_status, to_status, message, at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, c.Kind, c.ItemID, c.ItemName, c.From, c.To, c.Message, formatTime(c.At)); err != nil {
			return fmt.Errorf("status change: %w", err)
		}
	}

	day := r.At.UTC().Format(time.DateOnly)
	for _, m := range r.Samples {
		if _, err := tx.ExecContext(ctx, `INSERT INTO metric_samples (integration_id, key, at, value) VALUES (?, ?, ?, ?)`,
			id, m.Key, at, m.Value); err != nil {
			return fmt.Errorf("metric sample: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO metric_daily (integration_id, key, day, last, min, max)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (integration_id, key, day) DO UPDATE SET
				last = excluded.last, min = MIN(min, excluded.min), max = MAX(max, excluded.max)`,
			id, m.Key, day, m.Value, m.Value, m.Value); err != nil {
			return fmt.Errorf("metric daily: %w", err)
		}
	}
	return tx.Commit()
}

// ListStatusChanges liefert die jüngsten Statuswechsel einer Integration
// (integrationID 0 = alle), neueste zuerst.
func (s *Store) ListStatusChanges(ctx context.Context, integrationID int64, limit int) ([]StatusChange, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, integration_id, kind, item_id, item_name, from_status, to_status,
		message, at FROM status_changes WHERE (? = 0 OR integration_id = ?) ORDER BY at DESC, id DESC LIMIT ?`,
		integrationID, integrationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StatusChange
	for rows.Next() {
		var c StatusChange
		var at string
		if err := rows.Scan(&c.ID, &c.IntegrationID, &c.Kind, &c.ItemID, &c.ItemName, &c.From, &c.To, &c.Message, &at); err != nil {
			return nil, err
		}
		if c.At, err = parseTime(at); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DailyValue ist der Tageswert einer Kennzahl.
type DailyValue struct {
	Day            string
	Last, Min, Max float64
}

// MetricDaily liefert die Tageswerte einer Kennzahl ab from (YYYY-MM-DD), aufsteigend.
func (s *Store) MetricDaily(ctx context.Context, integrationID int64, key, from string) ([]DailyValue, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT day, last, min, max FROM metric_daily
		WHERE integration_id = ? AND key = ? AND day >= ? ORDER BY day`, integrationID, key, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DailyValue
	for rows.Next() {
		var d DailyValue
		if err := rows.Scan(&d.Day, &d.Last, &d.Min, &d.Max); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// PurgeResult zählt gelöschte Einträge.
type PurgeResult struct {
	Samples, Daily, Changes int64
}

// PurgeOldData löscht Rohdaten vor rawBefore sowie Tageswerte und
// Statuswechsel vor dailyBefore.
func (s *Store) PurgeOldData(ctx context.Context, rawBefore, dailyBefore time.Time) (PurgeResult, error) {
	var r PurgeResult
	for _, q := range []struct {
		sql string
		arg string
		n   *int64
	}{
		{`DELETE FROM metric_samples WHERE at < ?`, formatTime(rawBefore), &r.Samples},
		{`DELETE FROM metric_daily WHERE day < ?`, dailyBefore.UTC().Format(time.DateOnly), &r.Daily},
		{`DELETE FROM status_changes WHERE at < ?`, formatTime(dailyBefore), &r.Changes},
	} {
		res, err := s.db.ExecContext(ctx, q.sql, q.arg)
		if err != nil {
			return r, err
		}
		*q.n, _ = res.RowsAffected()
	}
	return r, nil
}
