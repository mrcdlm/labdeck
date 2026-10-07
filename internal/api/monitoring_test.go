package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// setupHC legt eine Healthchecks-Integration gegen den Fake an und fragt sie ab.
func setupHC(t *testing.T, e *testEnv, c *http.Cookie) int64 {
	t.Helper()
	resp := e.do(t, "POST", "/api/integrations", fmt.Sprintf(`{"type":"healthchecks","name":"Homelab","enabled":true,
		"interval_seconds":60,"timeout_seconds":5,"tls_verify":true,"config":{"base_url":%q},
		"secrets":{"api_key":"ro-key"}}`, e.hcURL), c)
	var it integrationResp
	json.NewDecoder(resp.Body).Decode(&it)
	if resp.StatusCode != 201 {
		t.Fatalf("anlegen: %d", resp.StatusCode)
	}
	if resp := e.do(t, "POST", fmt.Sprintf("/api/integrations/%d/poll", it.ID), "", c); resp.StatusCode != 200 {
		t.Fatalf("abfragen: %d", resp.StatusCode)
	}
	return it.ID
}

type checksResp struct {
	Checks []struct {
		IntegrationID   int64    `json:"integration_id"`
		IntegrationName string   `json:"integration_name"`
		ID              string   `json:"id"`
		Name            string   `json:"name"`
		Tags            []string `json:"tags"`
		Status          string   `json:"status"`
		Raw             string   `json:"raw"`
		Stale           bool     `json:"stale"`
		Uptime          map[string]struct {
			Percent *float64 `json:"percent"`
			Buckets []string `json:"buckets"`
		} `json:"uptime"`
	} `json:"checks"`
}

type backupsResp struct {
	Backups []struct {
		IntegrationID int64          `json:"integration_id"`
		ID            string         `json:"id"`
		Name          string         `json:"name"`
		State         string         `json:"state"`
		Status        string         `json:"status"`
		MaxAgeHours   int            `json:"max_age_hours"`
		MaxAgeCustom  bool           `json:"max_age_custom"`
		Uptime        map[string]any `json:"uptime"`
	} `json:"backups"`
	Summary struct {
		Total  int            `json:"total"`
		States map[string]int `json:"states"`
	} `json:"summary"`
	DefaultMaxAgeHours int `json:"default_max_age_hours"`
	WarnPercent        int `json:"warn_percent"`
}

type overviewResp struct {
	Status       string `json:"status"`
	Integrations struct {
		Total, Active, Error int
	} `json:"integrations"`
	Checks struct {
		Total    int            `json:"total"`
		Statuses map[string]int `json:"statuses"`
	} `json:"checks"`
	Backups struct {
		Total int `json:"total"`
	} `json:"backups"`
	Problems []struct {
		Kind   string `json:"kind"`
		Status string `json:"status"`
		Name   string `json:"name"`
		State  string `json:"state"`
		Detail string `json:"detail"`
	} `json:"problems"`
}

func TestChecksAPI(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)

	var got checksResp
	json.NewDecoder(e.do(t, "GET", "/api/checks", "", c).Body).Decode(&got)
	if got.Checks == nil || len(got.Checks) != 0 {
		t.Fatalf("ohne Integration: %+v", got)
	}

	id := setupHC(t, e, c)
	resp := e.do(t, "GET", "/api/checks", "", c)
	json.NewDecoder(resp.Body).Decode(&got)
	if resp.StatusCode != 200 || len(got.Checks) != 7 {
		t.Fatalf("%d %+v", resp.StatusCode, got)
	}
	// Probleme zuerst: down, grace, …
	if got.Checks[0].Name != "Proxmox vzdump" || got.Checks[1].Name != "Borg Laptop" ||
		got.Checks[len(got.Checks)-1].Status != "paused" {
		t.Errorf("Reihenfolge: %s, %s, …", got.Checks[0].Name, got.Checks[1].Name)
	}
	for _, ch := range got.Checks {
		if ch.IntegrationID != id || ch.IntegrationName != "Homelab" || ch.Tags == nil || ch.Stale {
			t.Errorf("%+v", ch)
		}
		if len(ch.Uptime["24h"].Buckets) != 48 || len(ch.Uptime["7d"].Buckets) != 56 || len(ch.Uptime["30d"].Buckets) != 60 {
			t.Errorf("%s: Uptime %+v", ch.Name, ch.Uptime)
		}
	}
	vz := got.Checks[0]
	if p := vz.Uptime["30d"].Percent; p == nil || *p <= 0 || *p >= 100 || vz.Uptime["24h"].Buckets[47] != "error" {
		t.Errorf("vzdump: %+v", vz.Uptime)
	}

	// /api/integrations liefert keine Verläufe mit
	raw := body(t, e.do(t, "GET", "/api/integrations", "", c).Body)
	if strings.Contains(raw, "history") || strings.Contains(raw, `"runs"`) {
		t.Error("Verlauf in /api/integrations")
	}

	// Integration nicht erreichbar → Daten bleiben, als veraltet markiert
	e.hc.Fail(http.StatusBadGateway)
	e.do(t, "POST", fmt.Sprintf("/api/integrations/%d/poll", id), "", c)
	json.NewDecoder(e.do(t, "GET", "/api/checks", "", c).Body).Decode(&got)
	if len(got.Checks) != 7 || !got.Checks[0].Stale {
		t.Errorf("veraltet: %+v", got.Checks[0])
	}

	// deaktiviert → nicht überwacht
	e.do(t, "PUT", fmt.Sprintf("/api/integrations/%d", id), fmt.Sprintf(`{"name":"Homelab","enabled":false,
		"interval_seconds":60,"timeout_seconds":5,"tls_verify":true,"config":{"base_url":%q}}`, e.hcURL), c)
	got = checksResp{}
	json.NewDecoder(e.do(t, "GET", "/api/checks", "", c).Body).Decode(&got)
	if len(got.Checks) != 0 {
		t.Errorf("deaktiviert: %d Checks", len(got.Checks))
	}
}

func TestBackupsAPI(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)
	id := setupHC(t, e, c)

	var got backupsResp
	json.NewDecoder(e.do(t, "GET", "/api/backups", "", c).Body).Decode(&got)
	if len(got.Backups) != 3 || got.DefaultMaxAgeHours != 26 || got.WarnPercent != 80 || got.Summary.Total != 3 {
		t.Fatalf("%+v", got)
	}
	// vzdump: down und 52 h alt → überfällig; Borg: grace → bald fällig; Restic: ok
	want := [][2]string{{"Proxmox vzdump", "overdue"}, {"Borg Laptop", "due_soon"}, {"Restic NAS → B2", "ok"}}
	for i, w := range want {
		if got.Backups[i].Name != w[0] || got.Backups[i].State != w[1] || len(got.Backups[i].Uptime) != 3 {
			t.Errorf("%d: %+v, erwartet %v", i, got.Backups[i], w)
		}
	}

	// Höchstalter je Backup
	vz := got.Backups[0]
	set := func(itemID string, hours int) *http.Response {
		return e.do(t, "PUT", "/api/backups/max-age",
			fmt.Sprintf(`{"integration_id":%d,"item_id":%q,"max_age_hours":%d}`, id, itemID, hours), c)
	}
	if resp := set(vz.ID, 72); resp.StatusCode != 204 {
		t.Fatalf("setzen: %d %s", resp.StatusCode, body(t, resp.Body))
	}
	json.NewDecoder(e.do(t, "GET", "/api/backups", "", c).Body).Decode(&got)
	for _, b := range got.Backups {
		if b.ID == vz.ID && (b.MaxAgeHours != 72 || !b.MaxAgeCustom || b.State != "failed") {
			t.Errorf("nach Setzen: %+v", b)
		}
	}
	if resp := set(vz.ID, 0); resp.StatusCode != 204 {
		t.Fatalf("zurücksetzen: %d", resp.StatusCode)
	}
	json.NewDecoder(e.do(t, "GET", "/api/backups", "", c).Body).Decode(&got)
	if got.Backups[0].MaxAgeCustom || got.Backups[0].MaxAgeHours != 26 {
		t.Errorf("nach Zurücksetzen: %+v", got.Backups[0])
	}

	// Fehler: Bereich, unbekanntes Backup, unbekannte Integration
	if resp := set(vz.ID, 99999); resp.StatusCode != 400 || !strings.Contains(body(t, resp.Body), "Stunden") {
		t.Errorf("Bereich: %d", resp.StatusCode)
	}
	if resp := set("gibt-es-nicht", 10); resp.StatusCode != 404 {
		t.Errorf("unbekanntes Backup: %d", resp.StatusCode)
	}
	resp := e.do(t, "PUT", "/api/backups/max-age", `{"integration_id":999,"item_id":"x","max_age_hours":10}`, c)
	if resp.StatusCode != 404 {
		t.Errorf("unbekannte Integration: %d", resp.StatusCode)
	}

	// Standard-Höchstalter aus den Einstellungen
	e.do(t, "PUT", "/api/settings", `{"timezone":"","raw_retention_days":30,"daily_retention_days":365,"backup_max_age_hours":100,"notify_language":"de"}`, c)
	json.NewDecoder(e.do(t, "GET", "/api/backups", "", c).Body).Decode(&got)
	if got.DefaultMaxAgeHours != 100 || got.Backups[0].Name != "Proxmox vzdump" || got.Backups[0].State != "failed" {
		t.Errorf("Standard 100 h: %+v", got)
	}
}

func TestOverviewAPI(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)

	var got overviewResp
	json.NewDecoder(e.do(t, "GET", "/api/overview", "", c).Body).Decode(&got)
	if got.Status != "unknown" || got.Problems == nil || len(got.Problems) != 0 {
		t.Fatalf("ohne Integration: %+v", got)
	}

	id := setupHC(t, e, c)
	json.NewDecoder(e.do(t, "GET", "/api/overview", "", c).Body).Decode(&got)
	if got.Status != "error" || got.Integrations.Active != 1 || got.Checks.Total != 7 || got.Backups.Total != 3 ||
		got.Checks.Statuses["error"] != 1 {
		t.Fatalf("%+v", got)
	}
	// vzdump (Backup, überfällig), Borg (Backup, bald fällig) – nicht zusätzlich als Check
	if len(got.Problems) != 2 || got.Problems[0].Kind != "backup" || got.Problems[0].State != "overdue" ||
		got.Problems[1].Status != "warning" || got.Problems[1].Kind != "backup" {
		t.Fatalf("Probleme: %+v", got.Problems)
	}

	// Ein Check ohne Backup-Tag fällt aus; die Integration ist nicht erreichbar
	e.hc.SetStatus("DynDNS", "down")
	e.do(t, "POST", fmt.Sprintf("/api/integrations/%d/poll", id), "", c)
	e.hc.Fail(http.StatusUnauthorized)
	e.do(t, "POST", fmt.Sprintf("/api/integrations/%d/poll", id), "", c)
	got = overviewResp{}
	json.NewDecoder(e.do(t, "GET", "/api/overview", "", c).Body).Decode(&got)
	kinds := []string{}
	for _, p := range got.Problems {
		kinds = append(kinds, p.Kind+":"+p.Name)
	}
	if len(got.Problems) != 4 || got.Problems[0].Kind != "integration" || !strings.Contains(got.Problems[0].Detail, "Zugriff verweigert") ||
		got.Problems[2].Kind != "check" || got.Problems[2].Name != "DynDNS" || got.Problems[2].State != "down" {
		t.Fatalf("Probleme: %v", kinds)
	}
}

func TestOverviewMetricThresholds(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"disk": 95, "temp": 41}`))
	}))
	defer src.Close()
	resp := e.do(t, "POST", "/api/integrations", fmt.Sprintf(`{"type":"httpjson","name":"NAS","enabled":true,
		"interval_seconds":60,"timeout_seconds":5,"tls_verify":true,"config":{"url":%q,"values":[
		{"name":"Disk","path":"disk","unit":"%%","warn":80,"error":90},{"name":"Temp","path":"temp","warn":60}]}}`, src.URL), c)
	if resp.StatusCode != 201 {
		t.Fatalf("anlegen: %d %s", resp.StatusCode, body(t, resp.Body))
	}
	var it integrationResp
	json.NewDecoder(resp.Body).Decode(&it)
	e.do(t, "POST", fmt.Sprintf("/api/integrations/%d/poll", it.ID), "", c)

	var got struct {
		Status   string `json:"status"`
		Problems []struct {
			Kind, Status, Name, Unit, IntegrationType string
			Value                                     *float64
		} `json:"problems"`
	}
	raw := body(t, e.do(t, "GET", "/api/overview", "", c).Body)
	json.Unmarshal([]byte(raw), &got)
	if got.Status != "error" || len(got.Problems) != 1 || got.Problems[0].Kind != "metric" || got.Problems[0].Name != "Disk" ||
		*got.Problems[0].Value != 95 || got.Problems[0].Unit != "%" || !strings.Contains(raw, `"integration_type":"httpjson"`) {
		t.Fatalf("%s", raw)
	}
}
