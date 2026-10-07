package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mrcdlm/labdeck/internal/store"
)

func TestImmichAndHistory(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)
	input := func(key string) string {
		return fmt.Sprintf(`{"type":"immich","name":"Fotos","enabled":true,"interval_seconds":60,"timeout_seconds":5,
			"tls_verify":true,"config":{"base_url":%q},"secrets":{"api_key":%q}}`, e.imURL, key)
	}

	// Key ohne Berechtigung → verständlicher Hinweis
	var test map[string]any
	json.NewDecoder(e.do(t, "POST", "/api/integrations/test", input("im-user"), c).Body).Decode(&test)
	if test["ok"] != false || !strings.Contains(test["error"].(string), "server.statistics") {
		t.Fatalf("403: %v", test)
	}

	resp := e.do(t, "POST", "/api/integrations", input("im-key"), c)
	var it integrationResp
	json.NewDecoder(resp.Body).Decode(&it)
	e.do(t, "POST", fmt.Sprintf("/api/integrations/%d/poll", it.ID), "", c)
	e.im.AddPhotos(100)
	resp = e.do(t, "POST", fmt.Sprintf("/api/integrations/%d/poll", it.ID), "", c)
	raw := body(t, resp.Body)
	if !strings.Contains(raw, `"version":"v1.140.1"`) || !strings.Contains(raw, `"group":"Anna"`) {
		t.Fatalf("Snapshot: %s", raw)
	}

	var h struct {
		Key, Name, Unit, Range string
		Points                 []struct {
			T time.Time
			V float64
		}
	}
	get := func(q string) int {
		h.Points = nil
		r := e.do(t, "GET", fmt.Sprintf("/api/integrations/%d/history?%s", it.ID, q), "", c)
		json.NewDecoder(r.Body).Decode(&h)
		return r.StatusCode
	}
	// Rohdaten: beide Abfragen fallen in denselben 15-Minuten-Abschnitt → letzter Wert
	if st := get("key=photos&range=24h"); st != 200 || len(h.Points) != 1 || h.Points[0].V != 48_313 || h.Range != "24h" {
		t.Fatalf("24h: %d %+v", st, h)
	}
	if st := get("key=usage&range=1y"); st != 200 || len(h.Points) != 1 || h.Unit != "bytes" {
		t.Fatalf("1y: %d %+v", st, h)
	}
	if st := get("key=gibt-es-nicht"); st != 200 || len(h.Points) != 0 {
		t.Fatalf("unbekannte Kennzahl: %d %+v", st, h)
	}
	if st := get("key=photos&range=5y"); st != 400 {
		t.Errorf("ungültiger Zeitraum: %d", st)
	}
	if r := e.do(t, "GET", "/api/integrations/999/history?key=photos", "", c); r.StatusCode != 404 {
		t.Errorf("unbekannte Integration: %d", r.StatusCode)
	}
}

func TestBucketLast(t *testing.T) {
	t0 := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	got := bucketLast([]store.Sample{
		{At: t0.Add(time.Minute), Value: 1}, {At: t0.Add(10 * time.Minute), Value: 2},
		{At: t0.Add(16 * time.Minute), Value: 3},
	}, 15*time.Minute)
	if len(got) != 2 || got[0].V != 2 || !got[0].T.Equal(t0) || got[1].V != 3 {
		t.Fatalf("%+v", got)
	}
}
