package store

import "context"

// BackupKey bezeichnet ein Backup: Integration und Backup-ID aus dem Snapshot.
type BackupKey struct {
	IntegrationID int64
	ItemID        string
}

// BackupMaxAges liefert die je Backup eingestellten Höchstalter in Stunden.
func (s *Store) BackupMaxAges(ctx context.Context) (map[BackupKey]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT integration_id, item_id, max_age_hours FROM backup_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[BackupKey]int{}
	for rows.Next() {
		var k BackupKey
		var h int
		if err := rows.Scan(&k.IntegrationID, &k.ItemID, &h); err != nil {
			return nil, err
		}
		out[k] = h
	}
	return out, rows.Err()
}

// SetBackupMaxAge speichert das Höchstalter eines Backups; hours 0 = zurück
// zum Standard. Gibt es die Integration nicht, liefert es ErrNotFound.
func (s *Store) SetBackupMaxAge(ctx context.Context, k BackupKey, hours int) error {
	if hours == 0 {
		_, err := s.db.ExecContext(ctx, `DELETE FROM backup_settings WHERE integration_id = ? AND item_id = ?`,
			k.IntegrationID, k.ItemID)
		return err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO backup_settings (integration_id, item_id, max_age_hours)
		SELECT id, ?, ? FROM integrations WHERE id = ?
		ON CONFLICT (integration_id, item_id) DO UPDATE SET max_age_hours = excluded.max_age_hours`,
		k.ItemID, hours, k.IntegrationID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
