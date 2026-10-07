package httpjson

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
)

var now = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

// server liefert JSON wie eine Statistik-API (Paperless) und ein Backup-Status.
func server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token geheim" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/statistics/":
			if r.Header.Get("Accept") != "application/json" {
				w.WriteHeader(http.StatusNotAcceptable)
				return
			}
			w.Write([]byte(`{"documents_total": 1523, "inbox": "12", "healthy": true,
				"tags": [{"n":"a"},{"n":"b"}], "storage": {"used_pct": 91.5},
				"backup": {"last_run": "2026-10-07T02:00:00Z", "unix": 1791338400}}`))
		case "/query":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			b, _ := io.ReadAll(r.Body)
			w.Write([]byte(`{"echo": ` + string(b) + `}`))
		case "/slow":
			time.Sleep(300 * time.Millisecond)
			w.Write([]byte(`{}`))
		case "/broken":
			w.Write([]byte(`{"a": `))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func cfg(t *testing.T, in map[string]any, auth string, timeout time.Duration) integrations.Config {
	t.Helper()
	values, msgs := New().Schema().Normalize(in, i18n.EN)
	if len(msgs) != 0 {
		t.Fatal(msgs)
	}
	return integrations.Config{Values: values, Secrets: map[string]string{"auth_header": auth},
		HTTP: integrations.NewHTTPClient(timeout, true), Timeout: timeout}
}

func val(name, path string, extra map[string]any) map[string]any {
	m := map[string]any{"name": name, "path": path}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func integ() *Integration { return &Integration{now: func() time.Time { return now }} }

func TestCollect(t *testing.T) {
	srv := server(t)
	c := cfg(t, map[string]any{"url": srv.URL + "/api/statistics/", "headers": "Accept: application/json\n\n",
		"values": []any{
			val("Dokumente", "documents_total", map[string]any{"warn": 1000.0, "error": 2000.0}),
			val("Posteingang", "inbox", nil),
			val("Gesund", "healthy", map[string]any{"direction": "below", "error": 0.0}),
			val("Tags", "tags.#", nil),
			val("Speicher", "storage.used_pct", map[string]any{"unit": "%", "warn": 80.0, "error": 90.0}),
			val("Backup", "backup.last_run", map[string]any{"kind": "timestamp", "backup": true, "warn": 26.0}),
			val("Backup", "backup.unix", map[string]any{"kind": "timestamp"}),
		}}, "Authorization: Token geheim", time.Second)
	snap, err := integ().Collect(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]integrations.Metric{}
	for _, x := range snap.Metrics {
		m[x.Key] = x
	}
	check := func(key string, v float64, st integrations.Status, unit string) {
		t.Helper()
		if got := m[key]; got.Value != v || got.Status != st || got.Unit != unit {
			t.Errorf("%s: %+v", key, got)
		}
	}
	check("dokumente", 1523, integrations.StatusWarning, "")
	check("posteingang", 12, integrations.StatusOK, "")
	check("gesund", 1, integrations.StatusOK, "")
	check("tags", 2, integrations.StatusOK, "")
	check("speicher", 91.5, integrations.StatusError, "%")
	check("backup", 10*3600, integrations.StatusOK, "s")
	check("backup_2", 10*3600, integrations.StatusOK, "s")
	if m["backup"].Name != "Backup" {
		t.Errorf("Name: %+v", m["backup"])
	}
	if len(snap.Backups) != 1 || snap.Backups[0].ID != "backup" || !snap.Backups[0].LastRun.Equal(now.Add(-10*time.Hour)) {
		t.Fatalf("Backups: %+v", snap.Backups)
	}
}

func TestPost(t *testing.T) {
	srv := server(t)
	c := cfg(t, map[string]any{"url": srv.URL + "/query", "method": "POST", "body": `{"x": 42}`,
		"values": []any{val("X", "echo.x", nil)}}, "Authorization: Token geheim", time.Second)
	snap, err := integ().Collect(context.Background(), c)
	if err != nil || snap.Metrics[0].Value != 42 {
		t.Fatalf("%v %+v", err, snap)
	}
}

func TestErrors(t *testing.T) {
	srv := server(t)
	ctx := context.Background()
	code := func(in map[string]any, auth string, timeout time.Duration) string {
		t.Helper()
		return i18n.FromError(integ().Test(ctx, cfg(t, in, auth, timeout))).Code
	}
	base := func(path string, values ...any) map[string]any {
		if len(values) == 0 {
			values = []any{val("Dokumente", "documents_total", nil)}
		}
		return map[string]any{"url": srv.URL + path, "headers": "Accept: application/json", "values": values}
	}
	const auth = "Authorization: Token geheim"
	if c := code(base("/api/statistics/"), auth, time.Second); c != "" {
		t.Fatalf("ok erwartet: %s", c)
	}
	if c := code(base("/api/statistics/"), "Authorization: falsch", time.Second); c != "integration.unauthorized" {
		t.Errorf("401: %s", c)
	}
	if c := code(base("/api/statistics/", val("X", "gibt.es.nicht", nil)), auth, time.Second); c != "httpjson.path_missing" {
		t.Errorf("Pfad: %s", c)
	}
	if c := code(base("/api/statistics/", val("X", "tags", nil)), auth, time.Second); c != "httpjson.not_number" {
		t.Errorf("keine Zahl: %s", c)
	}
	if c := code(base("/api/statistics/", val("X", "documents_total", map[string]any{"kind": "timestamp"})), auth,
		time.Second); c != "integration.field_url" && c != "" {
		// 1523 als Unix-Zeit ist gültig (1970) – kein Fehler
		t.Errorf("Zahl als Zeitstempel: %s", c)
	}
	if c := code(base("/api/statistics/", val("X", "healthy", map[string]any{"kind": "timestamp"})), auth,
		time.Second); c != "httpjson.not_timestamp" {
		t.Errorf("kein Zeitstempel: %s", c)
	}
	in := base("/api/statistics/")
	in["headers"] = "kaputt"
	if c := code(in, auth, time.Second); c != "httpjson.header_invalid" {
		t.Errorf("Header: %s", c)
	}
	if c := code(base("/api/statistics/"), "ohne Doppelpunkt", time.Second); c != "httpjson.auth_header_invalid" {
		t.Errorf("Auth-Header: %s", c)
	}
	if c := code(base("/broken"), auth, time.Second); c != "integration.bad_json" {
		t.Errorf("kaputtes JSON: %s", c)
	}
	if c := code(base("/slow"), auth, 100*time.Millisecond); c != "integration.timeout" {
		t.Errorf("Timeout: %s", c)
	}
}
