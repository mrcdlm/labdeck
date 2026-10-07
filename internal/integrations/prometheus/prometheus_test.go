package prometheus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/integrations/prometheus/promtest"
)

var now = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

func setup(t *testing.T, user, pw, token string) (*promtest.Fake, *httptest.Server) {
	t.Helper()
	fake := promtest.New(user, pw, token).Demo(now)
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return fake, srv
}

func q(name, query string, extra map[string]any) map[string]any {
	m := map[string]any{"name": name, "query": query}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func cfg(t *testing.T, in map[string]any, secrets map[string]string, timeout time.Duration) integrations.Config {
	t.Helper()
	values, msgs := New().Schema().Normalize(in, i18n.EN)
	if len(msgs) != 0 {
		t.Fatal(msgs)
	}
	return integrations.Config{Values: values, Secrets: secrets, HTTP: integrations.NewHTTPClient(timeout, true), Timeout: timeout}
}

func integ() *Integration { return &Integration{now: func() time.Time { return now }} }

func TestCollect(t *testing.T) {
	_, srv := setup(t, "", "", "")
	c := cfg(t, map[string]any{"base_url": srv.URL + "/", "queries": []any{
		q("CPU", `100 - avg(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100`, map[string]any{"unit": "%", "warn": 80.0}),
		q("Frei", `node_filesystem_avail_bytes{mountpoint="/"} / node_filesystem_size_bytes{mountpoint="/"} * 100`,
			map[string]any{"unit": "%", "label": "instance", "direction": "below", "warn": 20.0, "error": 10.0}),
		q("RAM frei", `node_memory_MemAvailable_bytes`, map[string]any{"unit": "bytes", "label": "instance"}),
		q("Restic", `restic_backup_timestamp`, map[string]any{"kind": "timestamp", "label": "client_hostname",
			"backup": true, "warn": 26.0, "error": 48.0}),
		q("Ausgefallen", `up == 0`, map[string]any{"error": 1.0}),
		q("Ziele", `scalar(count(up))`, nil),
	}}, nil, time.Second)
	snap, err := integ().Collect(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]integrations.Metric{}
	for _, x := range snap.Metrics {
		m[x.Key] = x
	}
	check := func(key string, v float64, st integrations.Status, group string) {
		t.Helper()
		got, ok := m[key]
		if !ok || got.Value != v || got.Status != st || got.Group != group {
			t.Errorf("%s: %+v (vorhanden: %v)", key, got, ok)
		}
	}
	check("cpu", 12.4, integrations.StatusOK, "")
	check("frei/nas_9100/frei", 38.2, integrations.StatusOK, "nas:9100")
	check("frei/pve_9100/frei", 9.6, integrations.StatusError, "pve:9100")
	check("ram_frei/nas_9100/ram_frei", 6.2e9, integrations.StatusOK, "nas:9100")
	check("restic/nas/restic", 5*3600, integrations.StatusOK, "nas")
	check("restic/laptop/restic", 40*3600, integrations.StatusWarning, "laptop")
	check("ausgefallen", 0, integrations.StatusOK, "")
	check("ziele", 7, integrations.StatusOK, "")
	if m["cpu"].Unit != "%" || m["restic/nas/restic"].Unit != "s" {
		t.Error("Einheiten")
	}
	if len(snap.Backups) != 2 || snap.Backups[0].Group != "nas" || !snap.Backups[0].LastRun.Equal(now.Add(-5*time.Hour)) {
		t.Fatalf("Backups: %+v", snap.Backups)
	}
}

func TestAuth(t *testing.T) {
	_, srv := setup(t, "prom", "geheim", "")
	in := map[string]any{"base_url": srv.URL, "auth": "basic", "username": "prom", "queries": []any{q("Ziele", `scalar(count(up))`, nil)}}
	if err := integ().Test(context.Background(), cfg(t, in, map[string]string{"password": "geheim"}, time.Second)); err != nil {
		t.Fatal(err)
	}
	err := integ().Test(context.Background(), cfg(t, in, map[string]string{"password": "falsch"}, time.Second))
	if i18n.FromError(err).Code != "integration.unauthorized" {
		t.Fatalf("falsches Passwort: %v", err)
	}

	_, srv = setup(t, "", "", "tok")
	in = map[string]any{"base_url": srv.URL, "auth": "bearer", "queries": []any{q("Ziele", `scalar(count(up))`, nil)}}
	if err := integ().Test(context.Background(), cfg(t, in, map[string]string{"token": "tok"}, time.Second)); err != nil {
		t.Fatal(err)
	}
}

func TestErrors(t *testing.T) {
	fake, srv := setup(t, "", "", "")
	ctx := context.Background()
	code := func(query string, timeout time.Duration) (string, string) {
		t.Helper()
		err := integ().Test(ctx, cfg(t, map[string]any{"base_url": srv.URL, "queries": []any{q("Abfrage", query, nil)}}, nil, timeout))
		m := i18n.FromError(err)
		return m.Code, i18n.T(i18n.DE, m)
	}
	if c, text := code("kaputt(", time.Second); c != "prometheus.query_error" || text == "" {
		t.Errorf("ungültige Abfrage: %s %s", c, text)
	}
	fake.Set("range", promtest.Sample{Value: 1})
	fake.Broken(true)
	if c, _ := code(`scalar(count(up))`, time.Second); c != "integration.bad_json" {
		t.Errorf("kaputtes JSON: %s", c)
	}
	fake.Broken(false)
	fake.Fail(http.StatusBadGateway)
	if c, _ := code(`scalar(count(up))`, time.Second); c != "integration.http" {
		t.Errorf("502: %s", c)
	}
	fake.Fail(0)
	fake.Delay(300 * time.Millisecond)
	if c, _ := code(`scalar(count(up))`, 100*time.Millisecond); c != "integration.timeout" {
		t.Errorf("Timeout: %s", c)
	}
	fake.Delay(0)

	// Bereichsergebnis (matrix) und fremde Antwort
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("query") == "m" {
			w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
			return
		}
		w.Write([]byte(`{"hello":"world"}`))
	}))
	defer other.Close()
	test := func(query string) string {
		err := integ().Test(ctx, cfg(t, map[string]any{"base_url": other.URL, "queries": []any{q("A", query, nil)}}, nil, time.Second))
		return i18n.FromError(err).Code
	}
	if c := test("m"); c != "prometheus.matrix" {
		t.Errorf("matrix: %s", c)
	}
	if c := test("x"); c != "prometheus.unexpected" {
		t.Errorf("fremd: %s", c)
	}
}
