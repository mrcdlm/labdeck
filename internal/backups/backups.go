// Package backups wertet die Backups aller Quellen einheitlich aus: Wann lief
// welches Backup zuletzt, war es erfolgreich, ist es (bald) überfällig?
package backups

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/store"
	"github.com/mrcdlm/labdeck/internal/uptime"
)

// State ist das Ergebnis der Auswertung.
type State string

const (
	StateOK      State = "ok"
	StateDueSoon State = "due_soon" // ab WarnRatio des Höchstalters oder Karenzzeit der Quelle
	StateOverdue State = "overdue"
	StateFailed  State = "failed"
	StateUnknown State = "unknown" // noch nie gelaufen
	StatePaused  State = "paused"
)

// WarnRatio: Warnung ab 80 % des Höchstalters.
const WarnRatio = 0.8

// order: Probleme zuerst.
var order = map[State]int{StateFailed: 0, StateOverdue: 1, StateDueSoon: 2, StateUnknown: 3, StateOK: 4, StatePaused: 5}

// Status liefert die Ampelfarbe des Zustands.
func (s State) Status() integrations.Status {
	switch s {
	case StateOK:
		return integrations.StatusOK
	case StateDueSoon:
		return integrations.StatusWarning
	case StateOverdue, StateFailed:
		return integrations.StatusError
	case StatePaused:
		return integrations.StatusPaused
	}
	return integrations.StatusUnknown
}

// Problem: überfällig, fehlgeschlagen oder bald fällig.
func (s State) Problem() bool { return s == StateFailed || s == StateOverdue || s == StateDueSoon }

// Source sind die Backups einer Integration (letzter erfolgreicher Snapshot).
type Source struct {
	IntegrationID   int64
	IntegrationName string
	IntegrationType string
	// Stale: die Integration ist gerade nicht erreichbar – die Daten sind
	// vom letzten erfolgreichen Abruf.
	Stale   bool
	Backups []integrations.Backup
}

// Item ist ein ausgewertetes Backup.
type Item struct {
	IntegrationID   int64                    `json:"integration_id"`
	IntegrationName string                   `json:"integration_name"`
	IntegrationType string                   `json:"integration_type"`
	ID              string                   `json:"id"`
	Name            string                   `json:"name"`
	Group           string                   `json:"group,omitempty"`
	Description     string                   `json:"description,omitempty"`
	LastRun         *time.Time               `json:"last_run,omitempty"`
	Result          integrations.Status      `json:"result"`
	SizeBytes       *int64                   `json:"size_bytes,omitempty"`
	DurationSeconds *float64                 `json:"duration_seconds,omitempty"`
	Verify          string                   `json:"verify,omitempty"`
	State           State                    `json:"state"`
	Status          integrations.Status      `json:"status"`
	MaxAgeHours     int                      `json:"max_age_hours"`
	MaxAgeCustom    bool                     `json:"max_age_custom"`
	DueAt           *time.Time               `json:"due_at,omitempty"` // letzter Lauf + Höchstalter
	Stale           bool                     `json:"stale,omitempty"`
	Uptime          map[string]uptime.Uptime `json:"uptime"`
}

// Evaluate wertet alle Backups aus, Probleme zuerst. maxAges: je Backup
// eingestellte Höchstalter (Stunden); sonst gilt defaultHours.
func Evaluate(sources []Source, maxAges map[store.BackupKey]int, defaultHours int, now time.Time) []Item {
	out := []Item{}
	for _, src := range sources {
		for _, b := range src.Backups {
			it := Item{IntegrationID: src.IntegrationID, IntegrationName: src.IntegrationName,
				IntegrationType: src.IntegrationType, ID: b.ID, Name: b.Name, Group: b.Group,
				Description: b.Description, LastRun: b.LastRun, Result: b.Result, SizeBytes: b.SizeBytes, Verify: b.Verify,
				MaxAgeHours: defaultHours, Stale: src.Stale,
			}
			if b.Duration != nil {
				d := b.Duration.Seconds()
				it.DurationSeconds = &d
			}
			if h, ok := maxAges[store.BackupKey{IntegrationID: src.IntegrationID, ItemID: b.ID}]; ok {
				it.MaxAgeHours, it.MaxAgeCustom = h, true
			}
			it.State = state(b, time.Duration(it.MaxAgeHours)*time.Hour, now)
			it.Status = it.State.Status()
			switch {
			case b.HistorySince != nil:
				it.Uptime = uptime.All(b.History, b.HistorySince, b.Result, now)
			case len(b.Runs) > 0:
				// Verlauf aus den Läufen: ok → bald fällig → überfällig bis zum nächsten Lauf
				tr, since := runHistory(b.Runs, time.Duration(it.MaxAgeHours)*time.Hour)
				it.Uptime = uptime.All(tr, &since, it.Status, now)
			}
			if b.LastRun != nil {
				due := b.LastRun.Add(time.Duration(it.MaxAgeHours) * time.Hour)
				it.DueAt = &due
			}
			out = append(out, it)
		}
	}
	slices.SortStableFunc(out, func(a, b Item) int {
		return cmp.Or(
			cmp.Compare(order[a.State], order[b.State]),
			strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
			strings.Compare(strings.ToLower(a.IntegrationName), strings.ToLower(b.IntegrationName)),
			cmp.Compare(a.IntegrationID, b.IntegrationID),
		)
	})
	return out
}

// runHistory macht aus Läufen Statuswechsel: nach einem erfolgreichen Lauf
// ok, ab WarnRatio des Höchstalters Warnung, danach Fehler (überfällig) bis
// zum nächsten Lauf; ein fehlgeschlagener Lauf ist Fehler bis zum nächsten.
func runHistory(runs []integrations.Run, maxAge time.Duration) ([]integrations.Transition, time.Time) {
	rs := slices.Clone(runs)
	slices.SortFunc(rs, func(a, b integrations.Run) int { return a.At.Compare(b.At) })
	var out []integrations.Transition
	cur := integrations.StatusUnknown
	set := func(at time.Time, st integrations.Status) {
		if st != cur {
			out = append(out, integrations.Transition{At: at, From: cur, To: st})
			cur = st
		}
	}
	for i, r := range rs {
		if r.Result == integrations.StatusError {
			set(r.At, integrations.StatusError)
			continue
		}
		set(r.At, integrations.StatusOK)
		next := r.At.Add(maxAge + 1) // nach dem letzten Lauf: bis „jetzt“
		if i+1 < len(rs) {
			next = rs[i+1].At
		}
		if w := r.At.Add(time.Duration(float64(maxAge) * WarnRatio)); w.Before(next) {
			set(w, integrations.StatusWarning)
		}
		if o := r.At.Add(maxAge); o.Before(next) {
			set(o, integrations.StatusError)
		}
	}
	return out, rs[0].At
}

func state(b integrations.Backup, maxAge time.Duration, now time.Time) State {
	if b.Result == integrations.StatusPaused {
		return StatePaused
	}
	overdue := b.LastRun != nil && now.Sub(*b.LastRun) > maxAge
	switch {
	case b.Result == integrations.StatusError && overdue:
		return StateOverdue // z. B. Healthchecks „down“, weil der Ping ausblieb
	case b.Result == integrations.StatusError:
		return StateFailed
	case b.LastRun == nil:
		return StateUnknown
	case overdue:
		return StateOverdue
	case b.Result == integrations.StatusWarning, now.Sub(*b.LastRun) >= time.Duration(float64(maxAge)*WarnRatio):
		return StateDueSoon
	}
	return StateOK
}

// Summary zählt die Backups je Zustand.
type Summary struct {
	Total  int                 `json:"total"`
	States map[State]int       `json:"states"`
	Status integrations.Status `json:"status"`
}

func Summarize(items []Item) Summary {
	s := Summary{Total: len(items), States: map[State]int{}}
	statuses := make([]integrations.Status, 0, len(items))
	for _, it := range items {
		s.States[it.State]++
		statuses = append(statuses, it.Status)
	}
	s.Status = integrations.Worst(statuses...)
	if len(items) == 0 {
		s.Status = integrations.StatusUnknown
	}
	return s
}
