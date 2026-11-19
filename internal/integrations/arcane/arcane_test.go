package arcane

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/integrations/arcane/arcanetest"
)

const key = "arc_testkey"

func setup(t *testing.T) (*arcanetest.Fake, *httptest.Server) {
	t.Helper()
	fake := arcanetest.New(key).Demo()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return fake, srv
}

func cfg(t *testing.T, url string, extra map[string]any, k string, timeout time.Duration) integrations.Config {
	t.Helper()
	in := map[string]any{"base_url": url}
	for n, v := range extra {
		in[n] = v
	}
	values, msgs := New().Schema().Normalize(in, i18n.EN)
	if len(msgs) != 0 {
		t.Fatal(msgs)
	}
	return integrations.Config{Values: values, Secrets: map[string]string{"api_key": k},
		HTTP: integrations.NewHTTPClient(timeout, true), Timeout: timeout}
}

func index(snap integrations.Snapshot) (map[string]integrations.Metric, map[string]integrations.Resource) {
	m := map[string]integrations.Metric{}
	for _, x := range snap.Metrics {
		m[x.Key] = x
	}
	r := map[string]integrations.Resource{}
	for _, x := range snap.Resources {
		r[x.ID] = x
	}
	return m, r
}

func TestCollect(t *testing.T) {
	_, srv := setup(t)
	snap, err := New().Collect(context.Background(), cfg(t, srv.URL+"/", nil, key, time.Second))
	if err != nil {
		t.Fatal(err)
	}
	m, r := index(snap)
	if m["containers_running"].Value != 4 || m["containers"].Value != 5 || m["hosts"].Value != 2 ||
		m["stacks"].Value != 6 || m["stacks_running"].Value != 4 {
		t.Errorf("Übersicht: %+v", snap.Metrics[:7])
	}
	// Updates: ein Container + ein Stack
	if m["updates"].Value != 2 {
		t.Errorf("Updates: %+v", m["updates"])
	}
	if u := m["unhealthy"]; u.Value != 1 || u.Status != integrations.StatusError || !u.Derived {
		t.Errorf("unhealthy: %+v", u)
	}
	if snap.Info["version"] != "Arcane 3.1.0" {
		t.Errorf("Version: %v", snap.Info)
	}
	h := r["env/0"]
	if h.Kind != integrations.KindNode || h.Name != "docker01" || h.State != "online" || h.CPUs != 8 ||
		*h.MemTotal != 32e9 || *h.MemUsed != 1_400_000_000+820_000_000+64_000_000 || h.Info["docker"] != "28.4.0" ||
		h.Info["containers"] != "4" || h.Status != integrations.StatusOK {
		t.Errorf("Host: %+v", h)
	}
	if *h.CPU != (41.2+1.8+0.1)/8 {
		t.Errorf("Host-CPU: %v", *h.CPU)
	}
	j := r["container/0/jellyfin"]
	if j.Parent != "docker01" || j.State != "healthy" || *j.CPU != 41.2 || *j.MemUsed != 1.4e9 || *j.NetIn != 12e9 ||
		j.Info["image"] != "jellyfin/jellyfin:10.10" || j.Info["stack"] != "media" || j.Status != integrations.StatusOK {
		t.Errorf("Container: %+v", j)
	}
	if p := r["container/0/paperless"]; p.Info["update"] != "1" {
		t.Errorf("Update-Hinweis: %+v", p)
	}
	if v := r["container/0/vaultwarden"]; v.State != "unhealthy" || v.Status != integrations.StatusError {
		t.Errorf("unhealthy: %+v", v)
	}
	if hm := m["res/container/0/vaultwarden/container_health"]; hm.Status != integrations.StatusError || hm.Group != "vaultwarden · docker01" {
		t.Errorf("unhealthy-Kennzahl: %+v", hm)
	}
	if s := m["stack/0/monitoring"]; s.Status != integrations.StatusWarning || s.Name != "stack_partial" {
		t.Errorf("Stack teilweise: %+v", s)
	}
	if o := r["container/0/old-wiki"]; o.Status != integrations.StatusPaused || o.CPU != nil {
		t.Errorf("gestoppt: %+v", o)
	}
	if z := r["container/6f1c2a9e-0b7d-4f2e-9a55-3c1d2e4f5a6b/zigbee2mqtt"]; z.Parent != "pi-edge" {
		t.Errorf("zweiter Host: %+v", z)
	}
}

func TestOptions(t *testing.T) {
	fake, srv := setup(t)
	ctx := context.Background()
	// Filter und ohne Container-Statistik
	snap, err := New().Collect(ctx, cfg(t, srv.URL, map[string]any{"environments": "pi-edge", "container_stats": false}, key, time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Resources) != 2 || snap.Resources[0].CPU != nil || snap.Resources[1].CPU != nil {
		t.Errorf("Filter/ohne Statistik: %+v", snap.Resources)
	}
	// Host offline, deaktiviert
	fake.SetEnvs(arcanetest.Env{ID: "0", Name: "nas", Status: "offline"}, arcanetest.Env{ID: "1", Name: "alt", Status: "online", Disabled: true})
	snap, err = New().Collect(ctx, cfg(t, srv.URL, nil, key, time.Second))
	m, r := index(snap)
	if err != nil || r["env/0"].Status != integrations.StatusError || r["env/1"].Status != integrations.StatusPaused ||
		m["res/env/0/online"].Status != integrations.StatusError {
		t.Errorf("offline: %v %+v", err, snap)
	}
	// Live: Container wird unhealthy
	fake.Demo()
	fake.SetHealth("jellyfin", "unhealthy")
	snap, _ = New().Collect(ctx, cfg(t, srv.URL, nil, key, time.Second))
	if _, r := index(snap); r["container/0/jellyfin"].Status != integrations.StatusError {
		t.Errorf("jellyfin: %+v", r["container/0/jellyfin"])
	}
}

// Arcane 2.x: dieselben Pfade unter /api/environments.
func TestV2Fallback(t *testing.T) {
	fake, srv := setup(t)
	fake.V2(true)
	snap, err := New().Collect(context.Background(), cfg(t, srv.URL, nil, key, time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if m, _ := index(snap); m["containers"].Value != 5 {
		t.Errorf("v2: %+v", snap.Metrics)
	}
}

// Mehr Container als eine Seite fasst.
func TestPagination(t *testing.T) {
	fake, srv := setup(t)
	var cs []arcanetest.Container
	for i := range 250 {
		cs = append(cs, arcanetest.Container{ID: fmt.Sprintf("c%013d", i), Name: fmt.Sprintf("app-%03d", i), State: "running"})
	}
	fake.SetEnvs(arcanetest.Env{ID: "0", Name: "big", Status: "online", Containers: cs})
	snap, err := New().Collect(context.Background(), cfg(t, srv.URL, nil, key, time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if m, r := index(snap); m["containers"].Value != 250 || r["container/0/app-249"].Name != "app-249" {
		t.Errorf("Seiten: %v", m["containers"])
	}
}

func TestHealthFromStatus(t *testing.T) {
	for status, want := range map[string]string{"Up 2 hours (healthy)": "healthy", "Up 5 minutes (unhealthy)": "unhealthy",
		"Up 3 seconds (health: starting)": "starting", "Up 1 day": "", "Exited (0)": ""} {
		if got := (container{Status: status}).health(); got != want {
			t.Errorf("%q: %q, erwartet %q", status, got, want)
		}
	}
	if n := (container{Names: []string{"/web"}}).name(); n != "web" {
		t.Errorf("Name: %q", n)
	}
}

func TestErrors(t *testing.T) {
	fake, srv := setup(t)
	ctx := context.Background()
	code := func(c integrations.Config) string { return i18n.FromError(New().Test(ctx, c)).Code }

	if c := code(cfg(t, srv.URL, nil, key, time.Second)); c != "" {
		t.Fatalf("ok erwartet: %s", c)
	}
	if c := code(cfg(t, srv.URL, nil, "arc_falsch", time.Second)); c != "integration.unauthorized" {
		t.Errorf("falscher Key: %s", c)
	}
	if c := code(cfg(t, srv.URL, map[string]any{"environments": "gibtsnicht"}, key, time.Second)); c != "arcane.env_missing" {
		t.Errorf("Umgebung: %s", c)
	}
	fake.Broken(true)
	if c := code(cfg(t, srv.URL, nil, key, time.Second)); c != "arcane.unexpected" {
		t.Errorf("kaputtes JSON: %s", c)
	}
	fake.Broken(false)
	fake.Fail(http.StatusBadGateway)
	if c := code(cfg(t, srv.URL, nil, key, time.Second)); c != "integration.http" {
		t.Errorf("502: %s", c)
	}
	fake.Fail(0)
	fake.Delay(300 * time.Millisecond)
	if c := code(cfg(t, srv.URL, nil, key, 100*time.Millisecond)); c != "integration.timeout" {
		t.Errorf("Timeout: %s", c)
	}
	fake.Delay(0)
	fake.SetEnvs()
	if c := code(cfg(t, srv.URL, nil, key, time.Second)); c != "arcane.no_envs" {
		t.Errorf("keine Umgebungen: %s", c)
	}
	// andere Web-App unter der Adresse
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html>")) }))
	defer other.Close()
	if c := code(cfg(t, other.URL, nil, key, time.Second)); c != "arcane.unexpected" {
		t.Errorf("fremd: %s", c)
	}
	// JSON ohne Arcane-Hülle
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`[]`)) }))
	defer plain.Close()
	if c := code(cfg(t, plain.URL, nil, key, time.Second)); c != "arcane.unexpected" {
		t.Errorf("ohne Hülle: %s", c)
	}
}
