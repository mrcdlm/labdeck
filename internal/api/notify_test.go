package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mrcdlm/labdeck/internal/integrations"
)

type webhookResp struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	TokenSet     bool   `json:"token_set"`
	TokenMissing bool   `json:"token_missing"`
	LastError    string `json:"last_error"`
}

func TestWebhooksWithToken(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)
	var (
		mu   sync.Mutex
		auth []string
	)
	rec := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		mu.Lock()
		auth = append(auth, r.Header.Get("Authorization")+" "+r.URL.Path)
		mu.Unlock()
	}))
	defer rec.Close()

	// Klartext-Geheimnis im Header → abgelehnt
	resp := e.do(t, "POST", "/api/webhooks", fmt.Sprintf(`{"name":"ntfy","method":"POST","url":%q,
		"headers":[{"name":"Authorization","value":"Bearer abc"}],"events":["check_down"]}`, rec.URL), c)
	if resp.StatusCode != 400 || !strings.Contains(body(t, resp.Body), "${TOKEN}") {
		t.Fatalf("Klartext: %d", resp.StatusCode)
	}

	resp = e.do(t, "POST", "/api/webhooks", fmt.Sprintf(`{"name":"ntfy","method":"POST","url":%q,
		"headers":[{"name":"Authorization","value":"Bearer ${TOKEN}"}],"events":["check_down"],"token":"tk-geheim"}`, rec.URL+"/t"), c)
	raw := body(t, resp.Body)
	if resp.StatusCode != 201 || strings.Contains(raw, "tk-geheim") {
		t.Fatalf("anlegen: %d %s", resp.StatusCode, raw)
	}
	var wh webhookResp
	json.Unmarshal([]byte(raw), &wh)
	if !wh.TokenSet || wh.TokenMissing {
		t.Fatalf("%+v", wh)
	}

	// Testnachricht mit entschlüsseltem Token
	var res map[string]any
	json.NewDecoder(e.do(t, "POST", fmt.Sprintf("/api/webhooks/%d/test", wh.ID), "", c).Body).Decode(&res)
	if res["ok"] != true || len(auth) != 1 || auth[0] != "Bearer tk-geheim /t" {
		t.Fatalf("Test: %v %v", res, auth)
	}

	// Ändern ohne Token: bleibt; Liste liefert ihn nie
	update := func(extra string) webhookResp {
		t.Helper()
		resp := e.do(t, "PUT", fmt.Sprintf("/api/webhooks/%d", wh.ID), fmt.Sprintf(`{"name":"ntfy 2","method":"POST","url":%q,
			"headers":[{"name":"Authorization","value":"Bearer ${TOKEN}"}],"events":["check_down","backup_overdue"]%s}`, rec.URL+"/t", extra), c)
		var out webhookResp
		json.NewDecoder(resp.Body).Decode(&out)
		if resp.StatusCode != 200 {
			t.Fatalf("ändern: %d", resp.StatusCode)
		}
		return out
	}
	if got := update(""); !got.TokenSet || got.Name != "ntfy 2" {
		t.Fatalf("unverändert: %+v", got)
	}
	if list := body(t, e.do(t, "GET", "/api/webhooks", "", c).Body); strings.Contains(list, "tk-geheim") || strings.Contains(list, `"token":`) {
		t.Fatalf("Token in Liste: %s", list)
	}
	if got := update(`,"clear_token":true`); got.TokenSet || !got.TokenMissing {
		t.Fatalf("entfernt: %+v", got)
	}
	json.NewDecoder(e.do(t, "POST", fmt.Sprintf("/api/webhooks/%d/test", wh.ID), "", c).Body).Decode(&res)
	if res["ok"] != false || !strings.Contains(res["error"].(string), "Feld Token") {
		t.Fatalf("ohne Token: %v", res)
	}

	// Vorschau zeigt Platzhalter
	resp = e.do(t, "POST", "/api/webhooks/preview", fmt.Sprintf(`{"name":"x","method":"POST","url":%q,
		"headers":[{"name":"Authorization","value":"Bearer ${TOKEN}"}],"event_type":"backup_overdue"}`, rec.URL), c)
	raw = body(t, resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(raw, "${TOKEN}") || !strings.Contains(raw, "backup_overdue") {
		t.Fatalf("Vorschau: %d %s", resp.StatusCode, raw)
	}

	if resp := e.do(t, "DELETE", fmt.Sprintf("/api/webhooks/%d", wh.ID), "", c); resp.StatusCode != 204 {
		t.Fatalf("löschen: %d", resp.StatusCode)
	}
}

func TestHistoryAndDashboard(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)
	id := setupHC(t, e, c)

	var h struct {
		Entries []struct {
			Kind            string `json:"kind"`
			ItemName        string `json:"item_name"`
			From            string `json:"from"`
			To              string `json:"to"`
			IntegrationName string `json:"integration_name"`
		} `json:"entries"`
		More bool `json:"more"`
	}
	get := func(q string) {
		t.Helper()
		h.Entries = nil
		resp := e.do(t, "GET", "/api/history"+q, "", c)
		if resp.StatusCode != 200 {
			t.Fatalf("%s: %d", q, resp.StatusCode)
		}
		json.NewDecoder(resp.Body).Decode(&h)
	}
	get("")
	if len(h.Entries) == 0 || h.Entries[0].IntegrationName != "Homelab" {
		t.Fatalf("%+v", h)
	}
	get("?kind=check&problems=1")
	for _, x := range h.Entries {
		if x.Kind != "check" || (x.To != "error" && x.To != "warning") {
			t.Errorf("Filter: %+v", x)
		}
	}
	if len(h.Entries) != 2 { // vzdump down, Borg grace
		t.Fatalf("Probleme: %+v", h.Entries)
	}
	get("?limit=2")
	if len(h.Entries) != 2 || !h.More {
		t.Fatalf("Seite: %+v", h)
	}
	if resp := e.do(t, "GET", "/api/history?kind=foo", "", c); resp.StatusCode != 400 {
		t.Fatalf("ungültig: %d", resp.StatusCode)
	}

	// Dashboard: Standard-Anordnung, eigene Anordnung, neue Kacheln
	type tile struct {
		ID      string `json:"id"`
		Visible bool   `json:"visible"`
		Size    int    `json:"size"`
	}
	var l struct {
		Tiles         []tile
		ProblemsFirst bool `json:"problems_first"`
	}
	json.NewDecoder(e.do(t, "GET", "/api/dashboard", "", c).Body).Decode(&l)
	if len(l.Tiles) != 4 || l.Tiles[0].ID != "backups" || l.Tiles[2].ID != "backup-timeline" || l.Tiles[3].ID != "events" ||
		l.Tiles[3].Size != 3 || !l.ProblemsFirst {
		t.Fatalf("Standard: %+v", l)
	}
	resp := e.do(t, "PUT", "/api/dashboard", `{"problems_first":false,"tiles":[{"id":"checks","visible":true,"size":2},{"id":"events","visible":false,"size":3},{"id":"backups","visible":true,"size":1}]}`, c)
	if resp.StatusCode != 200 {
		t.Fatalf("speichern: %d %s", resp.StatusCode, body(t, resp.Body))
	}
	resp = e.do(t, "POST", "/api/integrations", fmt.Sprintf(`{"type":"immich","name":"Fotos","enabled":true,"interval_seconds":60,
		"timeout_seconds":5,"tls_verify":true,"config":{"base_url":%q},"secrets":{"api_key":"im-key"}}`, e.imURL), c)
	var im integrationResp
	json.NewDecoder(resp.Body).Decode(&im)
	json.NewDecoder(e.do(t, "GET", "/api/dashboard", "", c).Body).Decode(&l)
	ids := []string{}
	for _, x := range l.Tiles {
		ids = append(ids, x.ID)
	}
	want := fmt.Sprintf("checks integration:%d backup-timeline events backups", im.ID)
	if strings.Join(ids, " ") != want || l.Tiles[3].Visible || l.ProblemsFirst {
		t.Fatalf("neue Kachel vor den Ereignissen: %v (erwartet %s)", ids, want)
	}
	for _, bad := range []string{`{"tiles":[{"id":"gibts","visible":true,"size":1}]}`,
		`{"tiles":[{"id":"checks","visible":true,"size":4}]}`,
		`{"tiles":[{"id":"checks","visible":true,"size":1},{"id":"checks","visible":true,"size":1}]}`} {
		if resp := e.do(t, "PUT", "/api/dashboard", bad, c); resp.StatusCode != 400 {
			t.Errorf("%s: %d", bad, resp.StatusCode)
		}
	}
	// Integration gelöscht → Kachel weg
	e.do(t, "DELETE", fmt.Sprintf("/api/integrations/%d", im.ID), "", c)
	_ = id
	json.NewDecoder(e.do(t, "GET", "/api/dashboard", "", c).Body).Decode(&l)
	if len(l.Tiles) != 4 {
		t.Fatalf("nach Löschen: %+v", l)
	}
}

func TestAvailableTilesHosts(t *testing.T) {
	got := availableTiles([]integrations.View{{ID: 1, Type: "healthchecks"}, {ID: 2, Type: "immich"}})
	if strings.Join(got, " ") != "backups checks integration:2 backup-timeline events" {
		t.Errorf("ohne Hosts: %v", got)
	}
	got = availableTiles([]integrations.View{{ID: 3, Type: "dockhand"}, {ID: 4, Type: "pve"}})
	if strings.Join(got, " ") != "backups checks integration:3 integration:4 hosts backup-timeline events" || defaultSize("hosts") != 2 {
		t.Errorf("mit Hosts: %v", got)
	}
}
