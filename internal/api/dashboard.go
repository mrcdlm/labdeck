package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/mrcdlm/labdeck/internal/events"
	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/store"
)

// Kacheln des Dashboards: Backups, Checks, je Integration (außer
// Healthchecks – dafür gibt es die Checks-Kachel), Hosts (sobald es eine
// Quelle mit Hosts gibt), Backup-Zeitleiste und die letzten Ereignisse.
// Der Gesamtstatus steht immer oben. Gespeichert wird die Anordnung als JSON
// in den Einstellungen.

const keyDashboard = "dashboard_layout"

type prefsStore interface {
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
}

type tileDTO struct {
	ID      string `json:"id"` // backups, checks, hosts, backup-timeline, events, integration:<id>
	Visible bool   `json:"visible"`
	Size    int    `json:"size"` // Spalten: 1–3
}

type layoutDTO struct {
	Tiles []tileDTO `json:"tiles"`
	// Kacheln mit Problemen nach vorne (Standard: ja)
	ProblemsFirst bool `json:"problems_first"`
}

// Integrationstypen, die Hosts/Nodes liefern (Kachel „Hosts“).
var hostTypes = []string{"pve", "dockhand"}

func defaultSize(id string) int {
	switch id {
	case "events", "backup-timeline":
		return 3
	case "hosts":
		return 2
	}
	return 1
}

// availableTiles: Kacheln, die es gerade geben kann, in Standardreihenfolge.
func availableTiles(list []integrations.View) []string {
	out := []string{"backups", "checks"}
	hosts := false
	for _, v := range list {
		if v.Type != "healthchecks" {
			out = append(out, "integration:"+strconv.FormatInt(v.ID, 10))
		}
		hosts = hosts || slices.Contains(hostTypes, v.Type)
	}
	if hosts {
		out = append(out, "hosts")
	}
	return append(out, "backup-timeline", "events")
}

// mergeLayout gleicht die gespeicherte Anordnung mit den verfügbaren Kacheln
// ab: verschwundene entfallen, neue kommen vor die Ereignisse.
func mergeLayout(saved []tileDTO, available []string) []tileDTO {
	out := []tileDTO{}
	for _, t := range saved {
		if slices.Contains(available, t.ID) && !slices.ContainsFunc(out, func(o tileDTO) bool { return o.ID == t.ID }) {
			out = append(out, t)
		}
	}
	for _, id := range available {
		if slices.ContainsFunc(out, func(o tileDTO) bool { return o.ID == id }) {
			continue
		}
		t := tileDTO{ID: id, Visible: true, Size: defaultSize(id)}
		if i := slices.IndexFunc(out, func(o tileDTO) bool { return o.ID == "events" }); i >= 0 && id != "events" {
			out = slices.Insert(out, i, t)
		} else {
			out = append(out, t)
		}
	}
	return out
}

func (s *Server) layout(ctx context.Context) (layoutDTO, error) {
	list, err := s.integrations.List(ctx)
	if err != nil {
		return layoutDTO{}, err
	}
	saved := layoutDTO{ProblemsFirst: true} // fehlt das Feld, bleibt es an
	raw, err := s.prefs.GetSetting(ctx, keyDashboard)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return layoutDTO{}, err
	}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &saved) // kaputt = Standard
	}
	return layoutDTO{Tiles: mergeLayout(saved.Tiles, availableTiles(list)), ProblemsFirst: saved.ProblemsFirst}, nil
}

func (s *Server) handleGetDashboard(w http.ResponseWriter, r *http.Request) {
	l, err := s.layout(r.Context())
	if err != nil {
		s.internalError(w, r, "Dashboard laden", err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (s *Server) handlePutDashboard(w http.ResponseWriter, r *http.Request) {
	in := layoutDTO{ProblemsFirst: true}
	if !decodeJSON(w, r, &in) {
		return
	}
	list, err := s.integrations.List(r.Context())
	if err != nil {
		s.internalError(w, r, "Dashboard speichern", err)
		return
	}
	available := availableTiles(list)
	seen := map[string]bool{}
	for _, t := range in.Tiles {
		if !slices.Contains(available, t.ID) || seen[t.ID] || t.Size < 1 || t.Size > 3 {
			writeMsg(w, r, http.StatusBadRequest, i18n.M("dashboard.tile_invalid", "id", strings.TrimSpace(t.ID)))
			return
		}
		seen[t.ID] = true
	}
	b, _ := json.Marshal(in)
	if err := s.prefs.SetSetting(r.Context(), keyDashboard, string(b)); err != nil {
		s.internalError(w, r, "Dashboard speichern", err)
		return
	}
	events.Publish(s.events, events.TopicDashboard)
	writeJSON(w, http.StatusOK, layoutDTO{Tiles: mergeLayout(in.Tiles, available), ProblemsFirst: in.ProblemsFirst})
}
