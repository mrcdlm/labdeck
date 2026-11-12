package api

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mrcdlm/labdeck/internal/backups"
	"github.com/mrcdlm/labdeck/internal/config"
	"github.com/mrcdlm/labdeck/internal/events"
	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/store"
	"github.com/mrcdlm/labdeck/internal/uptime"
)

// backupStore speichert Einstellungen je Backup.
type backupStore interface {
	BackupMaxAges(ctx context.Context) (map[store.BackupKey]int, error)
	SetBackupMaxAge(ctx context.Context, k store.BackupKey, hours int) error
}

// active liefert die aktiven Integrationen mit Daten (letzter erfolgreicher
// Snapshot). Deaktivierte Integrationen werden nicht überwacht.
func (s *Server) active(ctx context.Context) ([]integrations.View, error) {
	list, err := s.integrations.List(ctx)
	if err != nil {
		return nil, err
	}
	out := list[:0]
	for _, v := range list {
		if v.Enabled {
			out = append(out, v)
		}
	}
	return out, nil
}

func stale(v integrations.View) bool { return v.Status == integrations.StatusError }

// --- Checks ------------------------------------------------------------------

type checkDTO struct {
	IntegrationID   int64                    `json:"integration_id"`
	IntegrationName string                   `json:"integration_name"`
	ID              string                   `json:"id"`
	Name            string                   `json:"name"`
	Group           string                   `json:"group,omitempty"`
	Tags            []string                 `json:"tags"`
	Status          integrations.Status      `json:"status"`
	Raw             string                   `json:"raw,omitempty"`
	Desc            string                   `json:"desc,omitempty"`
	LastPing        *time.Time               `json:"last_ping,omitempty"`
	NextPing        *time.Time               `json:"next_ping,omitempty"`
	Stale           bool                     `json:"stale,omitempty"`
	Uptime          map[string]uptime.Uptime `json:"uptime"`
}

// checkOrder: Probleme zuerst.
var checkOrder = map[integrations.Status]int{
	integrations.StatusError: 0, integrations.StatusWarning: 1, integrations.StatusUnknown: 2,
	integrations.StatusOK: 3, integrations.StatusPaused: 4,
}

func collectChecks(list []integrations.View, now time.Time) []checkDTO {
	out := []checkDTO{}
	for _, v := range list {
		if v.Snapshot == nil {
			continue
		}
		for _, c := range v.Snapshot.Checks {
			tags := c.Tags
			if tags == nil {
				tags = []string{}
			}
			out = append(out, checkDTO{IntegrationID: v.ID, IntegrationName: v.Name, ID: c.ID, Name: c.Name,
				Group: c.Group, Tags: tags, Status: c.Status, Raw: c.Raw, Desc: c.Desc, LastPing: c.LastPing,
				NextPing: c.NextPing, Stale: stale(v), Uptime: uptime.All(c.History, c.HistorySince, c.Status, now)})
		}
	}
	slices.SortStableFunc(out, func(a, b checkDTO) int {
		return cmp.Or(
			cmp.Compare(checkOrder[a.Status], checkOrder[b.Status]),
			strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
			cmp.Compare(a.IntegrationID, b.IntegrationID),
		)
	})
	return out
}

func (s *Server) handleChecks(w http.ResponseWriter, r *http.Request) {
	list, err := s.active(r.Context())
	if err != nil {
		s.internalError(w, r, "Checks laden", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"checks": collectChecks(list, time.Now())})
}

// --- Backups -----------------------------------------------------------------

func (s *Server) defaultMaxAge() int {
	if s.settings == nil {
		return config.DefaultBackupMaxAgeHours
	}
	return s.settings.Get().BackupMaxAgeHours
}

func (s *Server) evaluateBackups(ctx context.Context, list []integrations.View, now time.Time) ([]backups.Item, error) {
	maxAges, err := s.backups.BackupMaxAges(ctx)
	if err != nil {
		return nil, err
	}
	return backups.Evaluate(backups.Sources(list), maxAges, s.defaultMaxAge(), now), nil
}

type backupsDTO struct {
	Backups            []backups.Item  `json:"backups"`
	Summary            backups.Summary `json:"summary"`
	DefaultMaxAgeHours int             `json:"default_max_age_hours"`
	WarnPercent        int             `json:"warn_percent"`
	Limits             map[string]int  `json:"limits"`
}

func (s *Server) handleBackups(w http.ResponseWriter, r *http.Request) {
	list, err := s.active(r.Context())
	if err != nil {
		s.internalError(w, r, "Backups laden", err)
		return
	}
	items, err := s.evaluateBackups(r.Context(), list, time.Now())
	if err != nil {
		s.internalError(w, r, "Backups laden", err)
		return
	}
	writeJSON(w, http.StatusOK, backupsDTO{Backups: items, Summary: backups.Summarize(items),
		DefaultMaxAgeHours: s.defaultMaxAge(), WarnPercent: int(backups.WarnRatio * 100),
		Limits: map[string]int{"max_age_hours_min": config.MinBackupMaxAgeHours, "max_age_hours_max": config.MaxBackupMaxAgeHours}})
}

// handleSetBackupMaxAge stellt das Höchstalter eines Backups ein (0 = Standard).
func (s *Server) handleSetBackupMaxAge(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IntegrationID int64  `json:"integration_id"`
		ItemID        string `json:"item_id"`
		MaxAgeHours   int    `json:"max_age_hours"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.MaxAgeHours != 0 && (in.MaxAgeHours < config.MinBackupMaxAgeHours || in.MaxAgeHours > config.MaxBackupMaxAgeHours) {
		writeMsg(w, r, http.StatusBadRequest, i18n.M("settings.backup_max_age_range",
			"min", strconv.Itoa(config.MinBackupMaxAgeHours), "max", strconv.Itoa(config.MaxBackupMaxAgeHours)))
		return
	}
	// Nur Backups, die es gerade gibt (kein Ablegen beliebiger Schlüssel).
	v, err := s.integrations.Get(r.Context(), in.IntegrationID)
	if err != nil {
		s.integrationError(w, r, "Integration laden", err)
		return
	}
	found := false
	if v.Snapshot != nil {
		found = slices.ContainsFunc(v.Snapshot.Backups, func(b integrations.Backup) bool { return b.ID == in.ItemID })
	}
	if !found {
		writeMsg(w, r, http.StatusNotFound, i18n.M("backup.not_found"))
		return
	}
	err = s.backups.SetBackupMaxAge(r.Context(), store.BackupKey{IntegrationID: in.IntegrationID, ItemID: in.ItemID}, in.MaxAgeHours)
	if errors.Is(err, store.ErrNotFound) {
		writeMsg(w, r, http.StatusNotFound, i18n.M("integration.not_found_id"))
		return
	}
	if err != nil {
		s.internalError(w, r, "Höchstalter speichern", err)
		return
	}
	s.log.Info("Höchstalter eines Backups geändert", "integration", in.IntegrationID, "max_age_hours", in.MaxAgeHours)
	events.Publish(s.events, events.TopicBackups)
	w.WriteHeader(http.StatusNoContent)
}

// --- Gesamtstatus --------------------------------------------------------------

type problemDTO struct {
	// ID: stabil je Problem (<kind>:<integration>:<item>)
	ID string `json:"id"`
	// Kind: integration, check, backup, metric
	Kind            string              `json:"kind"`
	Status          integrations.Status `json:"status"` // error, warning
	IntegrationID   int64               `json:"integration_id"`
	IntegrationName string              `json:"integration_name"`
	IntegrationType string              `json:"integration_type"`
	ItemID          string              `json:"item_id,omitempty"`
	Name            string              `json:"name"`
	// Kennzahl: Gruppe (z. B. Benutzer), Wert und Einheit
	Group string   `json:"group,omitempty"`
	Value *float64 `json:"value,omitempty"`
	Unit  string   `json:"unit,omitempty"`
	// State: Zustand des Backups (overdue, failed, due_soon) bzw. Status
	// der Quelle beim Check (down, grace)
	State string `json:"state,omitempty"`
	// Detail: Fehlermeldung der Integration
	Detail string `json:"detail,omitempty"`
	// Resource: Host, VM, Container, Port …, zu dem die Kennzahl gehört
	ResourceID   string `json:"resource_id,omitempty"`
	ResourceName string `json:"resource_name,omitempty"`
	// At: letzter Erfolg (Integration), letzter Ping (Check), letzter Lauf (Backup)
	At *time.Time `json:"at,omitempty"`
	// Since: Beginn des Problems laut Verlauf (fehlt, wenn nicht bekannt)
	Since *time.Time `json:"since,omitempty"`
}

type checksSummary struct {
	Total    int                         `json:"total"`
	Statuses map[integrations.Status]int `json:"statuses"`
	Status   integrations.Status         `json:"status"`
}

type overviewDTO struct {
	Status       integrations.Status `json:"status"`
	Integrations struct {
		Total  int `json:"total"`
		Active int `json:"active"`
		Error  int `json:"error"`
	} `json:"integrations"`
	Checks   checksSummary   `json:"checks"`
	Backups  backups.Summary `json:"backups"`
	Problems []problemDTO    `json:"problems"`
}

var kindOrder = map[string]int{"integration": 0, "backup": 1, "check": 2, "metric": 3}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	all, err := s.integrations.List(r.Context())
	if err != nil {
		s.internalError(w, r, "Gesamtstatus laden", err)
		return
	}
	lang := i18n.FromRequest(r)
	now := time.Now()
	out := overviewDTO{Problems: []problemDTO{}}
	out.Integrations.Total = len(all)

	var active []integrations.View
	for _, v := range all {
		if !v.Enabled {
			continue
		}
		active = append(active, v)
		if v.Status == integrations.StatusError {
			out.Integrations.Error++
			out.Problems = append(out.Problems, problemDTO{Kind: "integration", Status: integrations.StatusError,
				IntegrationID: v.ID, IntegrationName: v.Name, IntegrationType: v.Type, Name: v.Name,
				Detail: i18n.T(lang, v.LastError), At: v.LastSuccessAt})
		}
		// Schwellwerte von Kennzahlen (abgeleitete zählen über Checks/Backups)
		if v.Snapshot != nil {
			for _, m := range v.Snapshot.Metrics {
				if m.Derived || (m.Status != integrations.StatusError && m.Status != integrations.StatusWarning) {
					continue
				}
				val := m.Value
				p := problemDTO{Kind: "metric", Status: m.Status, IntegrationID: v.ID,
					IntegrationName: v.Name, IntegrationType: v.Type, ItemID: m.Key, Name: m.Name, Group: m.Group,
					Value: &val, Unit: m.Unit}
				if m.Resource != "" {
					p.ResourceID = m.Resource
					for _, res := range v.Snapshot.Resources {
						if res.ID == m.Resource {
							p.ResourceName = res.Name
							break
						}
					}
				}
				out.Problems = append(out.Problems, p)
			}
		}
	}
	out.Integrations.Active = len(active)

	items, err := s.evaluateBackups(r.Context(), active, now)
	if err != nil {
		s.internalError(w, r, "Gesamtstatus laden", err)
		return
	}
	out.Backups = backups.Summarize(items)
	isBackup := map[store.BackupKey]bool{}
	for _, b := range items {
		isBackup[store.BackupKey{IntegrationID: b.IntegrationID, ItemID: b.ID}] = true
		if b.State.Problem() {
			out.Problems = append(out.Problems, problemDTO{Kind: "backup", Status: b.Status, IntegrationID: b.IntegrationID,
				IntegrationName: b.IntegrationName, IntegrationType: b.IntegrationType, ItemID: b.ID, Name: b.Name,
				Group: b.Group, State: string(b.State), At: b.LastRun})
		}
	}

	checks := collectChecks(active, now)
	out.Checks = checksSummary{Total: len(checks), Statuses: map[integrations.Status]int{}, Status: integrations.StatusUnknown}
	// Ausgefallene Checks einzeln; Karenzzeit ist (noch) kein Problem.
	// Checks, die zugleich Backups sind, erscheinen nur als Backup.
	for _, c := range checks {
		out.Checks.Statuses[c.Status]++
		if c.Status != integrations.StatusError || isBackup[store.BackupKey{IntegrationID: c.IntegrationID, ItemID: c.ID}] {
			continue
		}
		out.Problems = append(out.Problems, problemDTO{Kind: "check", Status: integrations.StatusError,
			IntegrationID: c.IntegrationID, IntegrationName: c.IntegrationName, IntegrationType: "healthchecks",
			ItemID: c.ID, Name: c.Name, Group: c.Group, State: c.Raw, At: c.LastPing})
	}

	// Beginn je Problem aus dem Verlauf; ohne Verlauf fehlt die Angabe nur
	since := map[store.ProblemKey]time.Time{}
	if s.history != nil {
		if m, err := s.history.ProblemSince(r.Context()); err != nil {
			s.log.Warn("Beginn der Probleme laden", "err", err)
		} else {
			since = m
		}
	}
	for i := range out.Problems {
		p := &out.Problems[i]
		p.ID = p.Kind + ":" + strconv.FormatInt(p.IntegrationID, 10) + ":" + p.ItemID
		if t, ok := since[store.ProblemKey{IntegrationID: p.IntegrationID, Kind: p.Kind, ItemID: p.ItemID}]; ok {
			p.Since = &t
		}
	}
	switch {
	case len(checks) == 0:
	case out.Checks.Statuses[integrations.StatusError] > 0:
		out.Checks.Status = integrations.StatusError
	default:
		out.Checks.Status = integrations.StatusOK
	}

	// Fehler vor Warnungen, dann Integration, Backup, Check
	slices.SortStableFunc(out.Problems, func(a, b problemDTO) int {
		return cmp.Or(
			cmp.Compare(checkOrder[a.Status], checkOrder[b.Status]),
			cmp.Compare(kindOrder[a.Kind], kindOrder[b.Kind]),
		)
	})
	switch {
	case len(active) == 0:
		out.Status = integrations.StatusUnknown
	case len(out.Problems) == 0:
		out.Status = integrations.StatusOK
	default:
		out.Status = out.Problems[0].Status
	}
	writeJSON(w, http.StatusOK, out)
}
