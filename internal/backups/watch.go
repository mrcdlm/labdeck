package backups

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/store"
)

// Sources liefert die Backups aktiver Integrationen mit Daten.
func Sources(list []integrations.View) []Source {
	var out []Source
	for _, v := range list {
		if !v.Enabled || v.Snapshot == nil || len(v.Snapshot.Backups) == 0 {
			continue
		}
		out = append(out, Source{IntegrationID: v.ID, IntegrationName: v.Name, IntegrationType: v.Type,
			Stale: v.Status == integrations.StatusError, Backups: v.Snapshot.Backups})
	}
	return out
}

type stateStore interface {
	BackupStates(ctx context.Context) (map[store.BackupKey]string, error)
	SetBackupState(ctx context.Context, k store.BackupKey, state string) error
	AddStatusChanges(ctx context.Context, integrationID int64, changes []store.StatusChange) error
}

// Change ist ein Zustandswechsel eines Backups.
type Change struct {
	Item Item
	From State // "" = erstmals gesehen
}

// Watcher wertet die Backups regelmäßig aus (auch ohne neue Abfrage wird ein
// Backup überfällig), speichert Zustandswechsel im Verlauf und meldet sie.
type Watcher struct {
	// Load liefert die aktuellen Backups (wie die API sie auswertet).
	Load  func(ctx context.Context, now time.Time) ([]Item, error)
	Store stateStore
	// OnChange erhält jeden Wechsel nach dem Speichern (nicht beim ersten Sehen).
	OnChange func(c Change)
	Now      func() time.Time

	mu sync.Mutex
}

// Run wertet einmal aus. Mehrere gleichzeitige Aufrufe laufen nacheinander.
func (w *Watcher) Run(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	if w.Now != nil {
		now = w.Now()
	}
	items, err := w.Load(ctx, now)
	if err != nil {
		return err
	}
	prev, err := w.Store.BackupStates(ctx)
	if err != nil {
		return err
	}
	for _, it := range items {
		k := store.BackupKey{IntegrationID: it.IntegrationID, ItemID: it.ID}
		old, seen := prev[k]
		if seen && State(old) == it.State {
			continue
		}
		// Erstmals gesehen: nur Probleme in den Verlauf, nichts melden.
		if seen || it.State.Problem() {
			from := StateUnknown
			if seen {
				from = State(old)
			}
			err := w.Store.AddStatusChanges(ctx, it.IntegrationID, []store.StatusChange{{Kind: "backup", ItemID: it.ID,
				ItemName: it.Name, ItemGroup: it.Group, From: string(from), To: string(it.State), At: now}})
			if errors.Is(err, store.ErrNotFound) {
				continue // Integration inzwischen gelöscht
			}
			if err != nil {
				return err
			}
		}
		if err := w.Store.SetBackupState(ctx, k, string(it.State)); err != nil {
			return err
		}
		if seen && w.OnChange != nil {
			w.OnChange(Change{Item: it, From: State(old)})
		}
	}
	return nil
}
