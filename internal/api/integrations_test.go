package api

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
)

type integrationResp struct {
	ID      int64                     `json:"id"`
	Name    string                    `json:"name"`
	Status  string                    `json:"status"`
	Config  map[string]any            `json:"config"`
	Secrets map[string]map[string]any `json:"secrets"`
	Error   string                    `json:"last_error"`
	Code    string                    `json:"last_error_code"`
	Snap    *struct{ Checks []any }   `json:"snapshot"`
}

func body(t *testing.T, r io.Reader) string {
	t.Helper()
	b, _ := io.ReadAll(r)
	return string(b)
}

func TestIntegrationTypes(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)
	resp := e.do(t, "GET", "/api/integration-types", "", c)
	var got typesDTO
	json.NewDecoder(resp.Body).Decode(&got)
	if len(got.Types) != 5 || got.Types[0].Type != "healthchecks" || got.Types[2].Type != "immich" || !got.SecretKeySet ||
		got.Limits["interval_min"] != 15 || got.Types[0].Fields[1].Type != "secret" ||
		!strings.HasPrefix(got.Types[0].Fields[1].Help, "Projekt") {
		t.Fatalf("Typen (de): %+v", got)
	}
	e.language = "en"
	resp = e.do(t, "GET", "/api/integration-types", "", c)
	json.NewDecoder(resp.Body).Decode(&got)
	if got.Types[0].Fields[0].Label != "Address" {
		t.Fatalf("Typen (en): %+v", got.Types[0].Fields[0])
	}
}

func TestIntegrationsAPI(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)
	create := fmt.Sprintf(`{"type":"healthchecks","name":"HC","enabled":true,"interval_seconds":60,
		"timeout_seconds":5,"tls_verify":true,"config":{"base_url":%q},"secrets":{"api_key":"ro-key"}}`, e.hcURL)

	// Verbindung testen, ohne zu speichern
	resp := e.do(t, "POST", "/api/integrations/test", create, c)
	var test map[string]any
	json.NewDecoder(resp.Body).Decode(&test)
	if resp.StatusCode != 200 || test["ok"] != true || test["checks"] != 7.0 {
		t.Fatalf("Test: %d %v", resp.StatusCode, test)
	}
	resp = e.do(t, "POST", "/api/integrations/test", strings.Replace(create, "ro-key", "falsch", 1), c)
	json.NewDecoder(resp.Body).Decode(&test)
	if test["ok"] != false || !strings.Contains(test["error"].(string), "Zugriff verweigert") {
		t.Fatalf("Test falscher Key: %v", test)
	}

	resp = e.do(t, "POST", "/api/integrations", create, c)
	raw := body(t, resp.Body)
	if resp.StatusCode != 201 || strings.Contains(raw, "ro-key") {
		t.Fatalf("anlegen: %d %s", resp.StatusCode, raw)
	}
	var it integrationResp
	json.Unmarshal([]byte(raw), &it)
	if it.Secrets["api_key"]["set"] != true || it.Status != "unknown" {
		t.Fatalf("angelegt: %+v", it)
	}

	// Jetzt abfragen
	resp = e.do(t, "POST", fmt.Sprintf("/api/integrations/%d/poll", it.ID), "", c)
	json.NewDecoder(resp.Body).Decode(&it)
	if resp.StatusCode != 200 || it.Status != "ok" || it.Snap == nil || len(it.Snap.Checks) != 7 {
		t.Fatalf("poll: %d %+v", resp.StatusCode, it)
	}

	// Liste und Einzelabruf enthalten nie das Secret
	for _, path := range []string{"/api/integrations", fmt.Sprintf("/api/integrations/%d", it.ID)} {
		if raw := body(t, e.do(t, "GET", path, "", c).Body); strings.Contains(raw, "ro-key") || !strings.Contains(raw, `"set":true`) {
			t.Fatalf("%s: %s", path, raw)
		}
	}

	// Fehler der Quelle in der Sprache der Anfrage
	e.hc.Fail(401)
	e.language = "en"
	resp = e.do(t, "POST", fmt.Sprintf("/api/integrations/%d/poll", it.ID), "", c)
	json.NewDecoder(resp.Body).Decode(&it)
	if it.Status != "error" || it.Code != "integration.unauthorized" || !strings.HasPrefix(it.Error, "Access denied") {
		t.Fatalf("Fehler: %+v", it)
	}
	e.hc.Fail(0)

	// ändern ohne Secret = Secret bleibt; Validierungsfehler
	upd := fmt.Sprintf(`{"name":"Neu","enabled":false,"interval_seconds":120,"timeout_seconds":5,"tls_verify":true,
		"config":{"base_url":%q}}`, e.hcURL)
	resp = e.do(t, "PUT", fmt.Sprintf("/api/integrations/%d", it.ID), upd, c)
	json.NewDecoder(resp.Body).Decode(&it)
	if resp.StatusCode != 200 || it.Name != "Neu" || it.Status != "paused" || it.Secrets["api_key"]["set"] != true {
		t.Fatalf("ändern: %d %+v", resp.StatusCode, it)
	}
	resp = e.do(t, "PUT", fmt.Sprintf("/api/integrations/%d", it.ID), strings.Replace(upd, `"interval_seconds":120`, `"interval_seconds":1`, 1), c)
	if raw := body(t, resp.Body); resp.StatusCode != 400 || !strings.Contains(raw, "polling interval") {
		t.Fatalf("Validierung: %d %s", resp.StatusCode, raw)
	}
	if resp := e.do(t, "PUT", fmt.Sprintf("/api/integrations/%d", it.ID), `{"unbekannt":1}`, c); resp.StatusCode != 400 {
		t.Fatalf("unbekanntes Feld: %d", resp.StatusCode)
	}

	// löschen
	if resp := e.do(t, "DELETE", fmt.Sprintf("/api/integrations/%d", it.ID), "", c); resp.StatusCode != 204 {
		t.Fatalf("löschen: %d", resp.StatusCode)
	}
	for _, p := range []struct{ m, path string }{
		{"GET", fmt.Sprintf("/api/integrations/%d", it.ID)},
		{"DELETE", fmt.Sprintf("/api/integrations/%d", it.ID)},
		{"POST", fmt.Sprintf("/api/integrations/%d/poll", it.ID)},
		{"GET", "/api/integrations/abc"},
	} {
		if resp := e.do(t, p.m, p.path, "", c); resp.StatusCode != 404 {
			t.Errorf("%s %s: %d", p.m, p.path, resp.StatusCode)
		}
	}
}

func TestIntegrationsRequireAuth(t *testing.T) {
	e := newTestEnv(t, nil)
	for _, p := range []struct{ m, path string }{
		{"GET", "/api/integration-types"}, {"GET", "/api/integrations"}, {"POST", "/api/integrations"},
		{"POST", "/api/integrations/test"}, {"GET", "/api/integrations/1"}, {"PUT", "/api/integrations/1"},
		{"DELETE", "/api/integrations/1"}, {"POST", "/api/integrations/1/poll"},
	} {
		if resp := e.do(t, p.m, p.path, "", nil); resp.StatusCode != 401 {
			t.Errorf("%s %s: %d", p.m, p.path, resp.StatusCode)
		}
	}
}
