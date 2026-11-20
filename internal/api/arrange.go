package api

import (
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
)

// Standardanordnung des Dashboards in Bereichen, von oben nach unten:
//
//	0 Status-Kacheln (Backups, Checks, je Integration): kompakt, Reihen voll
//	1 breite Inhalte (Switch-Front, Server-Hardware, Hosts, Backup-Zeitleiste):
//	  je zwei nebeneinander
//	2 eigene Kennzahl-Kacheln: Größe bleibt, fließen in Reihen
//	3 Ereignisse: ganz unten, volle Breite
//
// So entstehen keine Löcher, und neue Kacheln landen in ihrem Bereich statt an
// der ersten freien Stelle.

// statusPerRow: höchstens so viele Status-Kacheln nebeneinander.
const statusPerRow = 4

// wideTypes: Integrationskacheln mit breitem Zusatzinhalt (Switch-Front,
// Hardware-Komponenten) – sie gehören zu den breiten Inhalten.
var wideTypes = []string{"snmp", "redfish"}

// wideRow: so viele breite Kacheln nebeneinander.
const wideRow = 2

// zone: Bereich einer Kachel; typ ist der Integrationstyp (sonst leer).
func zone(id, typ string) int {
	switch {
	case id == "hosts", id == "backup-timeline", slices.Contains(wideTypes, typ):
		return 1
	case id == "backups", id == "checks", strings.HasPrefix(id, "integration:"):
		return 0
	case id == "events":
		return 3
	}
	return 2
}

// defaultSize: Breite und Höhe neuer Kacheln; typ ist der Integrationstyp
// einer integration:-Kachel (sonst leer).
func defaultSize(id, typ string) (w, h int) {
	switch {
	case id == "events":
		return 12, 6
	case id == "hosts", id == "backup-timeline":
		return 6, 5
	case strings.HasPrefix(id, "metric:"):
		return 4, 3
	case slices.Contains(wideTypes, typ):
		return 6, 4
	}
	return 3, 3
}

// tileTypes ordnet integration:<id> dem Integrationstyp zu.
func tileTypes(list []integrations.View) map[string]string {
	out := make(map[string]string, len(list))
	for _, v := range list {
		out["integration:"+strconv.FormatInt(v.ID, 10)] = v.Type
	}
	return out
}

// arrange ordnet die sichtbaren Kacheln neu an (Bereiche siehe oben).
// Status-Kacheln verteilen sich gleichmäßig auf Reihen (10 → 4, 3, 3) und
// füllen jede Reihe ganz; Kennzahl-Kacheln behalten ihre Größe. Ausgeblendete
// Kacheln bleiben unverändert.
func arrange(tiles []tileDTO, types map[string]string) []tileDTO {
	out := slices.Clone(tiles)
	var zones [4][]int
	for n, t := range out {
		if t.Visible {
			z := zone(t.ID, types[t.ID])
			zones[z] = append(zones[z], n)
		}
	}
	size := func(n int) (int, int) { return defaultSize(out[n].ID, types[out[n].ID]) }
	y := 0

	// 0: Status-Kacheln, 1: breite Inhalte – ausgeglichene, volle Reihen
	// gleicher Höhe (z. B. 10 Status-Kacheln → 4, 3, 3)
	rowsOf := func(list []int, perRow int) {
		rows := (len(list) + perRow - 1) / perRow
		for r, i := 0, 0; r < rows; r++ {
			cnt := len(list) / rows
			if r < len(list)%rows {
				cnt++
			}
			w, h := gridCols/cnt, 0
			for _, n := range list[i : i+cnt] {
				_, th := size(n)
				h = max(h, th)
			}
			for k, n := range list[i : i+cnt] {
				out[n].X, out[n].Y, out[n].W, out[n].H = k*w, y, w, h
			}
			i += cnt
			y += h
		}
	}
	rowsOf(zones[0], statusPerRow)
	rowsOf(zones[1], wideRow)

	// 2: Kennzahl-Kacheln in Reihen, Größe bleibt
	x, rowH := 0, 0
	for _, n := range zones[2] {
		t := &out[n]
		if t.W < 2 || t.H < 2 {
			t.W, t.H = size(n)
		}
		t.W = min(t.W, gridCols)
		if x+t.W > gridCols {
			x, y, rowH = 0, y+rowH, 0
		}
		t.X, t.Y = x, y
		x += t.W
		rowH = max(rowH, t.H)
	}
	y += rowH

	// 3: Ereignisse
	for _, n := range zones[3] {
		out[n].W, out[n].H = size(n)
		out[n].X, out[n].Y = 0, y
		y += out[n].H
	}
	return out
}

// insert setzt eine neue Kachel in ihren Bereich: in eine Lücke der Reihen
// bis zu ihrem Bereich, sonst darunter – alles weiter unten rückt nach. Ohne
// Lücke verteilen Status- und breite Kacheln ihren Bereich neu (volle Reihen
// bzw. nebeneinander statt einer neuen Reihe mit nur dieser Kachel).
func insert(tiles []tileDTO, t tileDTO, types map[string]string) []tileDTO {
	z := zone(t.ID, types[t.ID])
	zb := 0 // Unterkante der Bereiche bis einschließlich z
	for _, o := range tiles {
		if o.Visible && zone(o.ID, types[o.ID]) <= z {
			zb = max(zb, o.Y+o.H)
		}
	}
	var above []tileDTO
	for _, o := range tiles {
		if o.Visible && o.Y < zb {
			above = append(above, o)
		}
	}
	if p := place(above, t, ""); p.Y+p.H <= zb {
		return append(tiles, p)
	}
	if z <= 1 {
		return rebalance(tiles, t, types, z, zb)
	}
	for n := range tiles {
		if tiles[n].Y >= zb {
			tiles[n].Y += t.H
		}
	}
	t.X, t.Y = 0, zb
	return append(tiles, t)
}

// rebalance nimmt t in den Bereich z auf, ordnet nur diesen Bereich neu (wie
// arrange) und verschiebt alles ab seiner alten Unterkante oldBottom um die
// Differenz.
func rebalance(tiles []tileDTO, t tileDTO, types map[string]string, z, oldBottom int) []tileDTO {
	var status []tileDTO
	top := -1
	for _, o := range tiles {
		if o.Visible && zone(o.ID, types[o.ID]) == z {
			status = append(status, o)
			if top < 0 || o.Y < top {
				top = o.Y
			}
		}
	}
	if top < 0 { // Bereich noch leer: direkt unter die Bereiche darüber
		top = oldBottom
	}
	tiles = append(tiles, t)
	status = arrange(append(status, t), types)
	newBottom := 0
	byID := map[string]tileDTO{}
	for _, s := range status {
		s.Y += top
		byID[s.ID] = s
		newBottom = max(newBottom, s.Y+s.H)
	}
	for n, o := range tiles {
		if s, ok := byID[o.ID]; ok && o.Visible {
			tiles[n] = s
		} else if o.Y >= oldBottom {
			tiles[n].Y += newBottom - oldBottom
		}
	}
	return tiles
}

// handleArrange liefert die Standardanordnung für die übergebenen Kacheln
// (Knopf „Anordnung zurücksetzen“ im Bearbeiten-Modus) – ohne zu speichern.
func (s *Server) handleArrange(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Tiles []tileDTO `json:"tiles"`
	}
	if !decodeJSONMax(w, r, &in, 256<<10) {
		return
	}
	if len(in.Tiles) > maxTiles {
		writeMsg(w, r, http.StatusBadRequest, i18n.M("dashboard.too_many_tiles", "max", strconv.Itoa(maxTiles)))
		return
	}
	list, err := s.integrations.List(r.Context())
	if err != nil {
		s.internalError(w, r, "Dashboard anordnen", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tiles": arrange(in.Tiles, tileTypes(list))})
}
