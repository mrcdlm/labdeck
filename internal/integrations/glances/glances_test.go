package glances

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/integrations/glances/glancestest"
)

func setup(t *testing.T, user, pw string) (*glancestest.Fake, *httptest.Server) {
	t.Helper()
	fake := glancestest.New(user, pw)
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return fake, srv
}

func cfg(t *testing.T, in map[string]any, pw string, timeout time.Duration) integrations.Config {
	t.Helper()
	values, msgs := New().Schema().Normalize(in, i18n.EN)
	if len(msgs) != 0 {
		t.Fatal(msgs)
	}
	return integrations.Config{Values: values, Secrets: map[string]string{"password": pw},
		HTTP: integrations.NewHTTPClient(timeout, true), Timeout: timeout}
}

func TestCollect(t *testing.T) {
	_, srv := setup(t, "", "")
	snap, err := New().Collect(context.Background(), cfg(t, map[string]any{"base_url": srv.URL + "/"}, "", time.Second))
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]integrations.Metric{}
	for _, x := range snap.Metrics {
		m[x.Key] = x
	}
	r := map[string]integrations.Resource{}
	for _, x := range snap.Resources {
		r[x.ID] = x
	}
	h := r["host/docker01"]
	if h.Kind != integrations.KindNode || *h.CPU != 23.5 || h.CPUs != 8 || *h.MemTotal != 32e9 || *h.DiskTotal != 250e9 ||
		h.Uptime != 12*86400+3*3600+4*60+5 || h.Status != integrations.StatusOK || h.Info["os"] == "" {
		t.Errorf("Host: %+v", h)
	}
	if m["cpu"].Value != 23.5 || m["mem_pct"].Value != 62 || m["containers_running"].Value != 3 || m["load5"].Value != 0.9 {
		t.Errorf("Übersicht: %+v", m)
	}
	if f := m["fs//srv/data/used_pct"]; f.Value != 93 || f.Status != integrations.StatusError || f.Group != "/srv/data" {
		t.Errorf("Dateisystem: %+v", f)
	}
	for k := range m {
		if len(k) > 20 && k[:20] == "fs//var/lib/docker/o" {
			t.Errorf("overlay als Dateisystem: %s", k)
		}
	}
	if j := r["container/jellyfin"]; j.Kind != integrations.KindContainer || j.Parent != "docker01" || *j.CPU != 41.2 ||
		!j.NetRate || j.Status != integrations.StatusOK || j.Info["image"] != "jellyfin/jellyfin:10.10" {
		t.Errorf("Container: %+v", j)
	}
	if v := r["container/vaultwarden"]; v.Status != integrations.StatusError || m["res/container/vaultwarden/container_health"].Status != integrations.StatusError {
		t.Errorf("unhealthy: %+v", v)
	}
	if o := r["container/old-wiki"]; o.Status != integrations.StatusPaused || o.CPU != nil {
		t.Errorf("gestoppt: %+v", o)
	}
	if mm := m["res/container/paperless/mem"]; mm.Unit != "bytes" || mm.Resource != "container/paperless" {
		t.Errorf("Container-RAM: %+v", mm)
	}
}

func TestAuthAndOptions(t *testing.T) {
	fake, srv := setup(t, "glances", "geheim")
	ctx := context.Background()
	in := map[string]any{"base_url": srv.URL, "username": "glances", "containers": false}
	if err := New().Test(ctx, cfg(t, in, "falsch", time.Second)); i18n.FromError(err).Code != "integration.unauthorized" {
		t.Fatalf("401: %v", err)
	}
	snap, err := New().Collect(ctx, cfg(t, in, "geheim", time.Second))
	if err != nil || len(snap.Resources) != 1 {
		t.Fatalf("ohne Container: %v %+v", err, snap.Resources)
	}
	// Container-Plugin aus → kein Fehler
	fake.NoDocker(true)
	in["containers"] = true
	if snap, err = New().Collect(ctx, cfg(t, in, "geheim", time.Second)); err != nil || len(snap.Resources) != 1 {
		t.Fatalf("ohne Docker: %v", err)
	}
}

func TestErrors(t *testing.T) {
	fake, srv := setup(t, "", "")
	ctx := context.Background()
	code := func(url string, timeout time.Duration) string {
		return i18n.FromError(New().Test(ctx, cfg(t, map[string]any{"base_url": url}, "", timeout))).Code
	}
	fake.Broken(true)
	if c := code(srv.URL, time.Second); c != "integration.bad_json" {
		t.Errorf("kaputtes JSON: %s", c)
	}
	fake.Broken(false)
	fake.Fail(http.StatusInternalServerError)
	if c := code(srv.URL, time.Second); c != "integration.http" {
		t.Errorf("500: %s", c)
	}
	fake.Fail(0)
	fake.Delay(300 * time.Millisecond)
	if c := code(srv.URL, 100*time.Millisecond); c != "integration.timeout" {
		t.Errorf("Timeout: %s", c)
	}
	fake.Delay(0)
	v3 := httptest.NewServer(http.NotFoundHandler())
	defer v3.Close()
	if c := code(v3.URL, time.Second); c != "glances.no_v4" {
		t.Errorf("v3/404: %s", c)
	}
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"x":1}`)) }))
	defer other.Close()
	if c := code(other.URL, time.Second); c != "glances.unexpected" {
		t.Errorf("fremd: %s", c)
	}
}

func TestParseUptime(t *testing.T) {
	for in, want := range map[string]int64{"1:46:07": 6367, "1 day, 0:00:01": 86401, "12 days, 3:04:05": 1047845, "": 0, "x": 0} {
		if got := parseUptime(in); got != want {
			t.Errorf("%q: %d, erwartet %d", in, got, want)
		}
	}
}
