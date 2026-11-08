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
	// Neue URL (anderer Host) ohne neues Token: abgelehnt, Token geht nicht mit
	resp = e.do(t, "PUT", fmt.Sprintf("/api/webhooks/%d", wh.ID), `{"name":"ntfy 2","method":"POST","url":"https://evil.example/t",
		"headers":[{"name":"Authorization","value":"Bearer ${TOKEN}"}],"events":["check_down"]}`, c)
	if raw := body(t, resp.Body); resp.StatusCode != 400 || !strings.Contains(raw, "webhook.token_reenter") {
		t.Fatalf("neue URL: %d %s", resp.StatusCode, raw)
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

func TestHistory(t *testing.T) {
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

	_ = id
}
