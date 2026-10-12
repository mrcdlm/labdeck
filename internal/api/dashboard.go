package api

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/mrcdlm/labdeck/internal/events"
	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/store"
)

// Dashboards: beliebig viele, je mit frei angeordneten Kacheln in einem
// Raster aus 12 Spalten (x, y, Breite, Höhe in Rastereinheiten).
//
// Kacheln: backups, checks, hosts, backup-timeline, events, je Integration
// (integration:<id>, außer Healthchecks – dafür gibt es die Checks-Kachel)
// und eigene Kennzahl-Kacheln (metric:<id> mit options).
//
// Das Haupt-Dashboard („main“, ohne es das erste) erhält neue Kacheln
// automatisch (neue Integration → neue Kachel); auf den anderen fügt man sie
// selbst hinzu. Gespeichert wird
// alles als JSON in den Einstellungen; die alte Anordnung (eine Liste mit
// Größe 1–3) wird beim Lesen übernommen.

const (
	keyDashboards = "dashboards"
	keyDashboard  = "dashboard_layout" // alte Anordnung (bis 0.4)

	gridCols      = 12
	maxTileHeight = 24
	maxTiles      = 60
	maxDashboards = 20
)

type prefsStore interface {
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
}

// tileOptions: Einstellungen einer Kennzahl-Kachel.
type tileOptions struct {
	IntegrationID int64    `json:"integration_id"`
	Keys          []string `json:"keys"`            // 1–3 Kennzahlen (mehrere: ein Diagramm mit Reihen)
	View          string   `json:"view"`            // number, sparkline, area, gauge
	Range         string   `json:"range"`           // 1h (live), 24h, 7d, 30d
	Min           *float64 `json:"min,omitempty"`   // Skala der Anzeige (gauge)
	Max           *float64 `json:"max,omitempty"`   //
	Label         string   `json:"label,omitempty"` // eigene Überschrift
}

type tileDTO struct {
	ID      string       `json:"id"`
	Visible bool         `json:"visible"`
	Size    int          `json:"size,omitempty"` // alt: Spalten 1–3 (nur Eingabe)
	X       int          `json:"x"`
	Y       int          `json:"y"`
	W       int          `json:"w"`
	H       int          `json:"h"`
	Options *tileOptions `json:"options,omitempty"`
}

type dashboardDTO struct {
	ID    string    `json:"id"`
	Name  string    `json:"name"` // leer = Standardname im Frontend („Übersicht“)
	Icon  string    `json:"icon,omitempty"`
	Tiles []tileDTO `json:"tiles"`
	// Kacheln mit Problemen nach vorne (gestapelte Ansicht auf dem Handy)
	ProblemsFirst bool `json:"problems_first"`
}

type dashboardsDTO struct {
	Dashboards []dashboardDTO `json:"dashboards"`
}

// Integrationstypen, die Hosts/Nodes liefern (Kachel „Hosts“).
var hostTypes = []string{"pve", "dockhand"}

var (
	metricTileID = regexp.MustCompile(`^metric:[a-z0-9-]{1,40}$`)
	dashboardID  = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)
	iconName     = regexp.MustCompile(`^[a-z0-9-]{0,40}$`)
	tileViews    = []string{"number", "sparkline", "area", "gauge"}
	tileRanges   = []string{"1h", "24h", "7d", "30d"}
)

// defaultSize: Breite und Höhe neuer Kacheln.
func defaultSize(id string) (w, h int) {
	switch {
	case id == "events":
		return 12, 7
	case id == "backup-timeline":
		return 12, 5
	case id == "hosts":
		return 8, 5
	case strings.HasPrefix(id, "metric:"):
		return 4, 3
	}
	return 4, 5
}

// availableTiles: eingebaute und Integrations-Kacheln, die es gerade geben
// kann, in Standardreihenfolge (Kennzahl-Kacheln legt man selbst an).
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

func bottom(tiles []tileDTO, skip string) int {
	b := 0
	for _, t := range tiles {
		if t.ID != skip {
			b = max(b, t.Y+t.H)
		}
	}
	return b
}

// place setzt t an die höchste freie Stelle (bei gleicher Höhe links), wie
// ein Raster, das von oben gefüllt wird. skip: nicht als belegt zählen.
func place(tiles []tileDTO, t tileDTO, skip string) tileDTO {
	var sky [gridCols]int
	for _, o := range tiles {
		if o.ID == skip {
			continue
		}
		for x := o.X; x < o.X+o.W && x < gridCols; x++ {
			sky[x] = max(sky[x], o.Y+o.H)
		}
	}
	best, bestY := 0, -1
	for x := 0; x+t.W <= gridCols; x++ {
		y := slices.Max(sky[x : x+t.W])
		if bestY < 0 || y < bestY {
			best, bestY = x, y
		}
	}
	t.X, t.Y = best, bestY
	return t
}

// legacyFlow rechnet die alte Anordnung (Größe 1–3 in 3 Spalten) in
// Rasterpositionen um: Zeile für Zeile, wie sie früher floss.
func legacyFlow(tiles []tileDTO) []tileDTO {
	x, y, rowH := 0, 0, 0
	for n, t := range tiles {
		_, h := defaultSize(t.ID)
		w := gridCols / 3 * min(max(t.Size, 1), 3)
		if x+w > gridCols {
			x, y, rowH = 0, y+rowH, 0
		}
		tiles[n].X, tiles[n].Y, tiles[n].W, tiles[n].H = x, y, w, h
		x += w
		rowH = max(rowH, h)
	}
	return tiles
}

// merge gleicht Kacheln mit den verfügbaren ab: verschwundene (gelöschte
// Integration) entfallen; auto: neue kommen dazu – vor die Ereignisse.
func merge(saved []tileDTO, available []string, integrationIDs map[int64]bool, auto bool) []tileDTO {
	out := []tileDTO{}
	legacy := false
	for _, t := range saved {
		ok := slices.Contains(available, t.ID) ||
			metricTileID.MatchString(t.ID) && t.Options != nil && integrationIDs[t.Options.IntegrationID]
		if !ok || slices.ContainsFunc(out, func(o tileDTO) bool { return o.ID == t.ID }) {
			continue
		}
		legacy = legacy || t.W == 0
		out = append(out, t)
	}
	if legacy {
		out = legacyFlow(out)
	}
	if !auto {
		return out
	}
	for _, id := range available {
		if slices.ContainsFunc(out, func(o tileDTO) bool { return o.ID == id }) {
			continue
		}
		t := tileDTO{ID: id, Visible: true}
		t.W, t.H = defaultSize(id)
		ev := slices.IndexFunc(out, func(o tileDTO) bool { return o.ID == "events" })
		// Ereignisse ganz unten: neue Kachel darüber, Ereignisse rücken nach
		evBottom := ev >= 0 && out[ev].Y >= bottom(out, "events")
		if evBottom {
			t = place(out, t, "events")
			out[ev].Y = max(out[ev].Y, t.Y+t.H)
		} else {
			t = place(out, t, "")
		}
		out = append(out, t)
	}
	return out
}

func newDashboardID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return "d-" + hex.EncodeToString(b)
}

// loadDashboards liest die gespeicherten Dashboards (oder übernimmt die alte
// Anordnung) und gleicht die Kacheln ab.
func (s *Server) loadDashboards(ctx context.Context) ([]dashboardDTO, error) {
	list, err := s.integrations.List(ctx)
	if err != nil {
		return nil, err
	}
	var saved []dashboardDTO
	raw, err := s.prefs.GetSetting(ctx, keyDashboards)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &saved) // kaputt = Standard
	}
	if len(saved) == 0 {
		main := dashboardDTO{ID: "main", ProblemsFirst: true} // fehlt das Feld, bleibt es an
		old, err := s.prefs.GetSetting(ctx, keyDashboard)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		if old != "" {
			_ = json.Unmarshal([]byte(old), &main)
			main.ID = "main"
		}
		saved = []dashboardDTO{main}
	}
	available := availableTiles(list)
	ids := map[int64]bool{}
	for _, v := range list {
		ids[v.ID] = true
	}
	// neue Kacheln aufs Haupt-Dashboard (auch wenn es nicht vorne steht)
	auto := mainIndex(saved)
	for n := range saved {
		saved[n].Tiles = merge(saved[n].Tiles, available, ids, n == auto)
	}
	return saved, nil
}

// mainIndex: Haupt-Dashboard („main“), sonst das erste.
func mainIndex(list []dashboardDTO) int {
	return max(slices.IndexFunc(list, func(d dashboardDTO) bool { return d.ID == "main" }), 0)
}

func (s *Server) saveDashboards(ctx context.Context, list []dashboardDTO) error {
	for n := range list {
		for k := range list[n].Tiles {
			list[n].Tiles[k].Size = 0 // nur Eingabe der alten API
		}
	}
	b, _ := json.Marshal(list)
	if err := s.prefs.SetSetting(ctx, keyDashboards, string(b)); err != nil {
		return err
	}
	events.Publish(s.events, events.TopicDashboard)
	return nil
}

func tileErr(id string) i18n.Msg {
	return i18n.M("dashboard.tile_invalid", "id", strings.TrimSpace(id))
}

// validate prüft Name, Symbol und Kacheln eines Dashboards; Kacheln ohne
// Position (alte API) werden eingereiht.
func validate(d *dashboardDTO, available []string, integrationIDs map[int64]bool) *i18n.Msg {
	d.Name = strings.TrimSpace(d.Name)
	if len([]rune(d.Name)) > 60 || !iconName.MatchString(d.Icon) {
		m := i18n.M("dashboard.invalid")
		return &m
	}
	if len(d.Tiles) > maxTiles {
		m := i18n.M("dashboard.too_many_tiles", "max", strconv.Itoa(maxTiles))
		return &m
	}
	seen := map[string]bool{}
	legacy := false
	for n := range d.Tiles {
		t := &d.Tiles[n]
		if seen[t.ID] {
			m := tileErr(t.ID)
			return &m
		}
		seen[t.ID] = true
		switch {
		case slices.Contains(available, t.ID):
			t.Options = nil
		case metricTileID.MatchString(t.ID):
			if msg := validateOptions(t.Options, integrationIDs); msg != nil {
				return msg
			}
		default:
			m := tileErr(t.ID)
			return &m
		}
		if t.W == 0 { // alte API: nur Größe
			if t.Size < 1 || t.Size > 3 {
				m := tileErr(t.ID)
				return &m
			}
			legacy = true
			continue
		}
		if t.W < 1 || t.H < 1 || t.H > maxTileHeight || t.X < 0 || t.X+t.W > gridCols || t.Y < 0 || t.Y > 1000 {
			m := tileErr(t.ID)
			return &m
		}
	}
	if legacy {
		d.Tiles = legacyFlow(d.Tiles)
	}
	return nil
}

func validateOptions(o *tileOptions, integrationIDs map[int64]bool) *i18n.Msg {
	bad := func() *i18n.Msg { m := i18n.M("dashboard.metric_invalid"); return &m }
	if o == nil || !integrationIDs[o.IntegrationID] || len(o.Keys) < 1 || len(o.Keys) > 3 ||
		!slices.Contains(tileViews, o.View) || !slices.Contains(tileRanges, o.Range) || len([]rune(o.Label)) > 80 {
		return bad()
	}
	for _, k := range o.Keys {
		if k == "" || len(k) > 300 {
			return bad()
		}
	}
	if o.Min != nil && o.Max != nil && *o.Min >= *o.Max {
		return bad()
	}
	return nil
}

// context der Validierung: verfügbare Kacheln und vorhandene Integrationen.
func (s *Server) tileContext(ctx context.Context) ([]string, map[int64]bool, error) {
	list, err := s.integrations.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	ids := map[int64]bool{}
	for _, v := range list {
		ids[v.ID] = true
	}
	return availableTiles(list), ids, nil
}

func (s *Server) handleListDashboards(w http.ResponseWriter, r *http.Request) {
	list, err := s.loadDashboards(r.Context())
	if err != nil {
		s.internalError(w, r, "Dashboards laden", err)
		return
	}
	writeJSON(w, http.StatusOK, dashboardsDTO{Dashboards: list})
}

// update liest, ändert und speichert die Dashboards unter Sperre.
func (s *Server) update(w http.ResponseWriter, r *http.Request, fn func(list []dashboardDTO) ([]dashboardDTO, *i18n.Msg, int)) {
	s.dashMu.Lock()
	defer s.dashMu.Unlock()
	list, err := s.loadDashboards(r.Context())
	if err != nil {
		s.internalError(w, r, "Dashboards laden", err)
		return
	}
	list, msg, status := fn(list)
	if msg != nil {
		writeMsg(w, r, status, *msg)
		return
	}
	if err := s.saveDashboards(r.Context(), list); err != nil {
		s.internalError(w, r, "Dashboards speichern", err)
		return
	}
	writeJSON(w, cmp.Or(status, http.StatusOK), dashboardsDTO{Dashboards: list})
}

func (s *Server) handleCreateDashboard(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
		Icon string `json:"icon"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	s.update(w, r, func(list []dashboardDTO) ([]dashboardDTO, *i18n.Msg, int) {
		if len(list) >= maxDashboards {
			m := i18n.M("dashboard.too_many", "max", strconv.Itoa(maxDashboards))
			return nil, &m, http.StatusBadRequest
		}
		d := dashboardDTO{ID: newDashboardID(), Name: in.Name, Icon: in.Icon, Tiles: []tileDTO{}, ProblemsFirst: true}
		if msg := validate(&d, nil, nil); msg != nil || d.Name == "" {
			m := i18n.M("dashboard.invalid")
			return nil, &m, http.StatusBadRequest
		}
		return append(list, d), nil, http.StatusCreated
	})
}

func (s *Server) putDashboard(w http.ResponseWriter, r *http.Request, id string) {
	in := dashboardDTO{ProblemsFirst: true}
	if !decodeJSONMax(w, r, &in, 256<<10) {
		return
	}
	available, ids, err := s.tileContext(r.Context())
	if err != nil {
		s.internalError(w, r, "Dashboard speichern", err)
		return
	}
	if msg := validate(&in, available, ids); msg != nil {
		writeMsg(w, r, http.StatusBadRequest, *msg)
		return
	}
	s.update(w, r, func(list []dashboardDTO) ([]dashboardDTO, *i18n.Msg, int) {
		n := mainIndex(list) // alte API: Haupt-Dashboard
		if id != "" {
			if n = slices.IndexFunc(list, func(d dashboardDTO) bool { return d.ID == id }); n < 0 {
				m := i18n.M("api.not_found")
				return nil, &m, http.StatusNotFound
			}
		}
		if in.Name == "" && list[n].ID != "main" {
			m := i18n.M("dashboard.invalid")
			return nil, &m, http.StatusBadRequest
		}
		in.ID = list[n].ID
		list[n] = in
		return list, nil, http.StatusOK
	})
}

func (s *Server) handlePutDashboardByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !dashboardID.MatchString(id) {
		writeMsg(w, r, http.StatusNotFound, i18n.M("api.not_found"))
		return
	}
	s.putDashboard(w, r, id)
}

func (s *Server) handleDeleteDashboard(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.update(w, r, func(list []dashboardDTO) ([]dashboardDTO, *i18n.Msg, int) {
		n := slices.IndexFunc(list, func(d dashboardDTO) bool { return d.ID == id })
		if n < 0 {
			m := i18n.M("api.not_found")
			return nil, &m, http.StatusNotFound
		}
		if len(list) == 1 {
			m := i18n.M("dashboard.last")
			return nil, &m, http.StatusBadRequest
		}
		return slices.Delete(list, n, n+1), nil, http.StatusOK
	})
}

func (s *Server) handleOrderDashboards(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []string `json:"ids"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	s.update(w, r, func(list []dashboardDTO) ([]dashboardDTO, *i18n.Msg, int) {
		if len(in.IDs) != len(list) {
			m := i18n.M("api.bad_request")
			return nil, &m, http.StatusBadRequest
		}
		out := make([]dashboardDTO, 0, len(list))
		for _, id := range in.IDs {
			n := slices.IndexFunc(list, func(d dashboardDTO) bool { return d.ID == id })
			if n < 0 || slices.ContainsFunc(out, func(d dashboardDTO) bool { return d.ID == id }) {
				m := i18n.M("api.bad_request")
				return nil, &m, http.StatusBadRequest
			}
			out = append(out, list[n])
		}
		return out, nil, http.StatusOK
	})
}

// Alte API (bis 0.4): das Haupt-Dashboard.

func (s *Server) handleGetDashboard(w http.ResponseWriter, r *http.Request) {
	list, err := s.loadDashboards(r.Context())
	if err != nil {
		s.internalError(w, r, "Dashboard laden", err)
		return
	}
	writeJSON(w, http.StatusOK, list[mainIndex(list)])
}

func (s *Server) handlePutDashboard(w http.ResponseWriter, r *http.Request) {
	s.putDashboard(w, r, "")
}
