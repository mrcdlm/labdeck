package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func insertChanges(ctx context.Context, db execer, integrationID int64, changes []StatusChange) error {
	for _, c := range changes {
		if _, err := db.ExecContext(ctx, `INSERT INTO status_changes
			(integration_id, kind, item_id, item_name, item_group, from_status, to_status, message, at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			integrationID, c.Kind, c.ItemID, c.ItemName, c.ItemGroup, c.From, c.To, c.Message, formatTime(c.At)); err != nil {
			return fmt.Errorf("status change: %w", err)
		}
	}
	return nil
}

// AddStatusChanges speichert Statuswechsel, die nicht aus einer Abfrage
// stammen (z. B. ein Backup wird überfällig). Gibt es die Integration nicht
// mehr, liefert es ErrNotFound.
func (s *Store) AddStatusChanges(ctx context.Context, integrationID int64, changes []StatusChange) error {
	if len(changes) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM integrations WHERE id = ?`, integrationID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	if err := insertChanges(ctx, tx, integrationID, changes); err != nil {
		return err
	}
	return tx.Commit()
}

// HistoryFilter schränkt den Verlauf ein (Nullwerte = alle).
type HistoryFilter struct {
	IntegrationID int64
	Kind          string
	// Problems: nur Wechsel zu einem schlechten Zustand (Fehler, Warnung,
	// überfällig …) und zurück zu ok
	Problems bool
	// BeforeID: nur ältere Einträge (Blättern)
	BeforeID int64
	Limit    int
}

// HistoryEntry ist ein Statuswechsel mit Namen und Typ der Integration.
type HistoryEntry struct {
	StatusChange
	IntegrationName string
	IntegrationType string
}

// ListHistory liefert Statuswechsel aller Integrationen, neueste zuerst.
func (s *Store) ListHistory(ctx context.Context, f HistoryFilter) ([]HistoryEntry, error) {
	where := []string{"1 = 1"}
	var args []any
	if f.IntegrationID != 0 {
		where = append(where, "c.integration_id = ?")
		args = append(args, f.IntegrationID)
	}
	if f.Kind != "" {
		where = append(where, "c.kind = ?")
		args = append(args, f.Kind)
	}
	if f.Problems {
		bad := `('error', 'warning', 'overdue', 'failed', 'due_soon')`
		where = append(where, "(c.to_status IN "+bad+" OR c.from_status IN "+bad+")")
	}
	if f.BeforeID != 0 {
		where = append(where, "c.id < ?")
		args = append(args, f.BeforeID)
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT c.id, c.integration_id, c.kind, c.item_id, c.item_name, c.item_group,
		c.from_status, c.to_status, c.message, c.at, i.name, i.type
		FROM status_changes c JOIN integrations i ON i.id = c.integration_id
		WHERE `+strings.Join(where, " AND ")+` ORDER BY c.id DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HistoryEntry{}
	for rows.Next() {
		var e HistoryEntry
		var at string
		if err := rows.Scan(&e.ID, &e.IntegrationID, &e.Kind, &e.ItemID, &e.ItemName, &e.ItemGroup, &e.From, &e.To,
			&e.Message, &at, &e.IntegrationName, &e.IntegrationType); err != nil {
			return nil, err
		}
		if e.At, err = parseTime(at); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// BackupStates liefert den zuletzt ausgewerteten Zustand je Backup.
func (s *Store) BackupStates(ctx context.Context) (map[BackupKey]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT integration_id, item_id, state FROM backup_states`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[BackupKey]string{}
	for rows.Next() {
		var k BackupKey
		var st string
		if err := rows.Scan(&k.IntegrationID, &k.ItemID, &st); err != nil {
			return nil, err
		}
		out[k] = st
	}
	return out, rows.Err()
}

// SetBackupState merkt sich den Zustand eines Backups (Integration muss existieren).
func (s *Store) SetBackupState(ctx context.Context, k BackupKey, state string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO backup_states (integration_id, item_id, state)
		SELECT id, ?, ? FROM integrations WHERE id = ?
		ON CONFLICT (integration_id, item_id) DO UPDATE SET state = excluded.state`, k.ItemID, state, k.IntegrationID)
	return err
}
