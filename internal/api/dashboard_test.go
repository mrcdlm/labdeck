package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/mrcdlm/labdeck/internal/integrations"
)

func (e *testEnv) dashboards(t *testing.T, c *http.Cookie) []dashboardDTO {
	t.Helper()
	var out dashboardsDTO
	resp := e.do(t, "GET", "/api/dashboards", "", c)
	if resp.StatusCode != 200 {
		t.Fatalf("GET: %d", resp.StatusCode)
	}
	json.NewDecoder(resp.Body).Decode(&out)
	return out.Dashboards
}

func tileIDs(tiles []tileDTO) string {
	var ids []string
	for _, t := range tiles {
		ids = append(ids, t.ID)
	}
	return strings.Join(ids, " ")
}

func tileByID(tiles []tileDTO, id string) tileDTO {
	for _, t := range tiles {
		if t.ID == id {
			return t
		}
	}
	return tileDTO{}
}

func (e *testEnv) addImmich(t *testing.T, c *http.Cookie) int64 {
	t.Helper()
	resp := e.do(t, "POST", "/api/integrations", fmt.Sprintf(`{"type":"immich","name":"Fotos","enabled":true,"interval_seconds":60,
		"timeout_seconds":5,"tls_verify":true,"config":{"base_url":%q},"secrets":{"api_key":"im-key"}}`, e.imURL), c)
	var im integrationResp
	json.NewDecoder(resp.Body).Decode(&im)
	if im.ID == 0 {
		t.Fatalf("Immich anlegen: %d", resp.StatusCode)
	}
	return im.ID
}

func TestDashboardsDefault(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)
	list := e.dashboards(t, c)
	if len(list) != 1 || list[0].ID != "main" || list[0].Name != "" || !list[0].ProblemsFirst {
		t.Fatalf("Standard: %+v", list)
	}
	d := list[0]
	if tileIDs(d.Tiles) != "backups checks backup-timeline events" {
		t.Fatalf("Kacheln: %s", tileIDs(d.Tiles))
	}
	// von oben gefüllt: zwei Kacheln nebeneinander, darunter die breiten
	if b, ch, tl, ev := d.Tiles[0], d.Tiles[1], d.Tiles[2], d.Tiles[3]; b.X != 0 || b.Y != 0 || b.W != 4 || ch.X != 4 || ch.Y != 0 ||
		tl.Y != 5 || tl.W != 12 || ev.Y != 10 || ev.H != 7 {
		t.Fatalf("Positionen: %+v", d.Tiles)
	}

	// neue Integration: Kachel kommt dazu, die Ereignisse bleiben unten
	id := e.addImmich(t, c)
	d = e.dashboards(t, c)[0]
	im, ev := tileByID(d.Tiles, fmt.Sprintf("integration:%d", id)), tileByID(d.Tiles, "events")
	if im.W != 4 || im.X != 8 || im.Y != 0 || ev.Y != 10 {
		t.Fatalf("neue Kachel: %+v / %+v", im, ev)
	}
}

// Die alte Anordnung (Liste mit Größe 1–3) wird übernommen.
func TestDashboardsLegacy(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)
	old := `{"problems_first":false,"tiles":[{"id":"checks","visible":true,"size":2},{"id":"backups","visible":true,"size":1},
		{"id":"events","visible":false,"size":3},{"id":"backup-timeline","visible":true,"size":3}]}`
	if err := e.api.prefs.SetSetting(context.Background(), keyDashboard, old); err != nil {
		t.Fatal(err)
	}
	d := e.dashboards(t, c)[0]
	if d.ProblemsFirst || tileIDs(d.Tiles) != "checks backups events backup-timeline" {
		t.Fatalf("übernommen: %+v", d)
	}
	ch, b, ev, tl := d.Tiles[0], d.Tiles[1], d.Tiles[2], d.Tiles[3]
	if ch.W != 8 || b.X != 8 || b.W != 4 || ev.Y != 5 || ev.W != 12 || ev.Visible || tl.Y != 12 {
		t.Fatalf("umgerechnet: %+v", d.Tiles)
	}

	// alte API: GET/PUT /api/dashboard = erstes Dashboard
	var first dashboardDTO
	json.NewDecoder(e.do(t, "GET", "/api/dashboard", "", c).Body).Decode(&first)
	if first.ID != "main" || len(first.Tiles) != 4 {
		t.Fatalf("Alias: %+v", first)
	}
	resp := e.do(t, "PUT", "/api/dashboard", `{"tiles":[{"id":"backups","visible":true,"size":3}]}`, c)
	if resp.StatusCode != 200 {
		t.Fatalf("Alias speichern: %d %s", resp.StatusCode, body(t, resp.Body))
	}
	d = e.dashboards(t, c)[0]
	if b := tileByID(d.Tiles, "backups"); b.W != 12 || b.Size != 0 {
		t.Fatalf("Alias: %+v", d.Tiles)
	}
}

func TestDashboardsCRUD(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)
	imID := e.addImmich(t, c)

	resp := e.do(t, "POST", "/api/dashboards", `{"name":"Netzwerk","icon":"network"}`, c)
	if resp.StatusCode != 201 {
		t.Fatalf("anlegen: %d %s", resp.StatusCode, body(t, resp.Body))
	}
	var created dashboardsDTO
	json.NewDecoder(resp.Body).Decode(&created)
	if len(created.Dashboards) != 2 {
		t.Fatalf("anlegen: %+v", created)
	}
	net := created.Dashboards[1]
	if net.Name != "Netzwerk" || net.Icon != "network" || len(net.Tiles) != 0 || !strings.HasPrefix(net.ID, "d-") {
		t.Fatalf("neues Dashboard: %+v", net)
	}
	// neue Kacheln kommen nur aufs erste Dashboard
	if d := e.dashboards(t, c)[1]; len(d.Tiles) != 0 {
		t.Fatalf("zweites Dashboard: %+v", d.Tiles)
	}

	metric := fmt.Sprintf(`{"id":"metric:fotos","visible":true,"x":0,"y":0,"w":6,"h":3,
		"options":{"integration_id":%d,"keys":["photos","videos"],"view":"area","range":"7d","label":"Fotos"}}`, imID)
	put := func(tiles string) *http.Response {
		return e.do(t, "PUT", "/api/dashboards/"+net.ID, `{"name":"Netzwerk","icon":"network","tiles":[`+tiles+`]}`, c)
	}
	if resp := put(metric + `,{"id":"checks","visible":true,"x":6,"y":0,"w":6,"h":5}`); resp.StatusCode != 200 {
		t.Fatalf("speichern: %d %s", resp.StatusCode, body(t, resp.Body))
	}
	d := e.dashboards(t, c)[1]
	if m := tileByID(d.Tiles, "metric:fotos"); m.Options == nil || m.Options.View != "area" || len(m.Options.Keys) != 2 {
		t.Fatalf("Kennzahl-Kachel: %+v", d.Tiles)
	}

	for name, tiles := range map[string]string{
		"unbekannt":     `{"id":"gibts","visible":true,"x":0,"y":0,"w":4,"h":4}`,
		"doppelt":       `{"id":"checks","visible":true,"x":0,"y":0,"w":4,"h":4},{"id":"checks","visible":true,"x":4,"y":0,"w":4,"h":4}`,
		"zu breit":      `{"id":"checks","visible":true,"x":10,"y":0,"w":4,"h":4}`,
		"zu hoch":       `{"id":"checks","visible":true,"x":0,"y":0,"w":4,"h":40}`,
		"ohne Optionen": `{"id":"metric:x","visible":true,"x":0,"y":0,"w":4,"h":3}`,
		"falsche Integration": `{"id":"metric:x","visible":true,"x":0,"y":0,"w":4,"h":3,
			"options":{"integration_id":999,"keys":["photos"],"view":"number","range":"24h"}}`,
		"falsche Darstellung": fmt.Sprintf(`{"id":"metric:x","visible":true,"x":0,"y":0,"w":4,"h":3,
			"options":{"integration_id":%d,"keys":["photos"],"view":"torte","range":"24h"}}`, imID),
		"Skala verkehrt": fmt.Sprintf(`{"id":"metric:x","visible":true,"x":0,"y":0,"w":4,"h":3,
			"options":{"integration_id":%d,"keys":["photos"],"view":"gauge","range":"1h","min":10,"max":5}}`, imID),
	} {
		if resp := put(tiles); resp.StatusCode != 400 {
			t.Errorf("%s: %d", name, resp.StatusCode)
		}
	}
	if resp := e.do(t, "PUT", "/api/dashboards/"+net.ID, `{"name":"","tiles":[]}`, c); resp.StatusCode != 400 {
		t.Errorf("ohne Name: %d", resp.StatusCode)
	}
	if resp := e.do(t, "PUT", "/api/dashboards/gibts", `{"name":"x","tiles":[]}`, c); resp.StatusCode != 404 {
		t.Errorf("unbekannt: %d", resp.StatusCode)
	}

	// Reihenfolge
	if resp := e.do(t, "PUT", "/api/dashboards/order", fmt.Sprintf(`{"ids":[%q,"main"]}`, net.ID), c); resp.StatusCode != 200 {
		t.Fatalf("Reihenfolge: %d", resp.StatusCode)
	}
	if l := e.dashboards(t, c); l[0].ID != net.ID || l[1].ID != "main" {
		t.Fatalf("Reihenfolge: %s %s", l[0].ID, l[1].ID)
	}
	if resp := e.do(t, "PUT", "/api/dashboards/order", `{"ids":["main","main"]}`, c); resp.StatusCode != 400 {
		t.Errorf("Reihenfolge doppelt: %d", resp.StatusCode)
	}

	// Integration gelöscht → Kennzahl-Kachel weg
	e.do(t, "DELETE", fmt.Sprintf("/api/integrations/%d", imID), "", c)
	// „Netzwerk“ steht jetzt vorne, neue Kacheln gehen trotzdem nur aufs Haupt-Dashboard
	if d := e.dashboards(t, c)[0]; tileIDs(d.Tiles) != "checks" {
		t.Fatalf("nach Löschen: %s", tileIDs(d.Tiles))
	}

	// Löschen; das letzte bleibt
	if resp := e.do(t, "DELETE", "/api/dashboards/"+net.ID, "", c); resp.StatusCode != 200 {
		t.Fatalf("löschen: %d", resp.StatusCode)
	}
	if resp := e.do(t, "DELETE", "/api/dashboards/main", "", c); resp.StatusCode != 400 {
		t.Errorf("letztes löschen: %d", resp.StatusCode)
	}
}

func TestAvailableTilesHosts(t *testing.T) {
	got := availableTiles([]integrations.View{{ID: 1, Type: "healthchecks"}, {ID: 2, Type: "immich"}})
	if strings.Join(got, " ") != "backups checks integration:2 backup-timeline events" {
		t.Errorf("ohne Hosts: %v", got)
	}
	got = availableTiles([]integrations.View{{ID: 3, Type: "dockhand"}, {ID: 4, Type: "pve"}})
	if w, _ := defaultSize("hosts"); strings.Join(got, " ") != "backups checks integration:3 integration:4 hosts backup-timeline events" || w != 8 {
		t.Errorf("mit Hosts: %v", got)
	}
}

func TestPlace(t *testing.T) {
	tiles := []tileDTO{{ID: "a", X: 0, Y: 0, W: 4, H: 5}, {ID: "b", X: 4, Y: 0, W: 4, H: 2}}
	// höchste freie Stelle: rechts oben neben a, b
	if p := place(tiles, tileDTO{W: 4, H: 3}, ""); p.X != 8 || p.Y != 0 {
		t.Errorf("rechts: %+v", p)
	}
	tiles = append(tiles, tileDTO{ID: "c", X: 8, Y: 0, W: 4, H: 4})
	// unter b ist am frühesten Platz
	if p := place(tiles, tileDTO{W: 4, H: 3}, ""); p.X != 4 || p.Y != 2 {
		t.Errorf("unter b: %+v", p)
	}
}
