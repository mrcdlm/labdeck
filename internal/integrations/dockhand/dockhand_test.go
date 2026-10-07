package dockhand

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/integrations/dockhand/dockhandtest"
)

const token = "dh_testtoken"

func setup(t *testing.T, tok string) (*dockhandtest.Fake, *httptest.Server) {
	t.Helper()
	fake := dockhandtest.New(tok).Demo()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return fake, srv
}

func cfg(t *testing.T, url string, extra map[string]any, tok string, timeout time.Duration) integrations.Config {
	t.Helper()
	in := map[string]any{"base_url": url}
	for k, v := range extra {
		in[k] = v
	}
	values, msgs := New().Schema().Normalize(in, i18n.EN)
	if len(msgs) != 0 {
		t.Fatal(msgs)
	}
	return integrations.Config{Values: values, Secrets: map[string]string{"api_token": tok},
		HTTP: integrations.NewHTTPClient(timeout, true), Timeout: timeout}
}

func TestCollect(t *testing.T) {
	_, srv := setup(t, token)
	snap, err := New().Collect(context.Background(), cfg(t, srv.URL+"/", nil, token, time.Second))
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
	if m["containers_running"].Value != 4 || m["containers"].Value != 5 || m["updates"].Value != 2 || m["hosts"].Value != 2 {
		t.Errorf("Übersicht: %+v %+v %+v", m["containers_running"], m["containers"], m["updates"])
	}
	if snap.Info["version"] != "Docker 28.4.0" {
		t.Errorf("Version: %v", snap.Info)
	}
	h := r["env/1"]
	if h.Kind != integrations.KindNode || h.Name != "docker01" || *h.CPU != 23.5 || h.CPUs != 8 || *h.MemTotal != 32e9 ||
		h.Uptime != 1_047_845 || h.Status != integrations.StatusOK || h.Info["hostname"] != "docker01.lan" {
		t.Errorf("Host: %+v", h)
	}
	if mm := m["res/env/1/mem_pct"]; mm.Resource != "env/1" || mm.Unit != "%" {
		t.Errorf("Host-RAM: %+v", mm)
	}
	j := r["container/1/jellyfin"]
	if j.Parent != "docker01" || j.State != "healthy" || *j.CPU != 41.2 || *j.MemUsed != 1.4e9 || *j.NetIn != 12e9 ||
		j.NetRate || j.Info["image"] != "jellyfin/jellyfin:10.10" || j.Status != integrations.StatusOK {
		t.Errorf("Container: %+v", j)
	}
	if v := r["container/1/vaultwarden"]; v.State != "unhealthy" || v.Status != integrations.StatusError {
		t.Errorf("unhealthy: %+v", v)
	}
	if hm := m["res/container/1/vaultwarden/container_health"]; hm.Status != integrations.StatusError || hm.Group != "vaultwarden · docker01" {
		t.Errorf("unhealthy-Kennzahl: %+v", hm)
	}
	if o := r["container/1/old-wiki"]; o.Status != integrations.StatusPaused || o.CPU != nil {
		t.Errorf("gestoppt: %+v", o)
	}
	if z := r["container/2/zigbee2mqtt"]; z.Parent != "pi-edge" {
		t.Errorf("zweiter Host: %+v", z)
	}
}

func TestOptions(t *testing.T) {
	fake, srv := setup(t, "")
	ctx := context.Background()
	// ohne Anmeldung in Dockhand: kein Token nötig; Filter und ohne Container-Statistik
	snap, err := New().Collect(ctx, cfg(t, srv.URL, map[string]any{"environments": "pi-edge", "container_stats": false}, "", time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Resources) != 2 || snap.Resources[1].CPU != nil {
		t.Errorf("Filter/ohne Statistik: %+v", snap.Resources)
	}
	// Host offline
	fake.SetEnvs(dockhandtest.Env{ID: 3, Name: "nas", Online: false})
	snap, err = New().Collect(ctx, cfg(t, srv.URL, nil, "", time.Second))
	if err != nil || len(snap.Resources) != 1 || snap.Resources[0].Status != integrations.StatusError {
		t.Errorf("offline: %v %+v", err, snap.Resources)
	}
	// Live: Container wird unhealthy
	fake.Demo()
	fake.SetHealth("jellyfin", "unhealthy")
	snap, _ = New().Collect(ctx, cfg(t, srv.URL, nil, "", time.Second))
	for _, r := range snap.Resources {
		if r.Name == "jellyfin" && r.Status != integrations.StatusError {
			t.Errorf("jellyfin: %+v", r)
		}
	}
}

func TestHealthFromStatus(t *testing.T) {
	for status, want := range map[string]string{"Up 2 hours (healthy)": "healthy", "Up 5 minutes (unhealthy)": "unhealthy",
		"Up 3 seconds (health: starting)": "", "Up 1 day": "", "Exited (0)": ""} {
		if got := health(container{Status: status}); got != want {
			t.Errorf("%q: %q, erwartet %q", status, got, want)
		}
	}
	if health(container{Health: "healthy", Status: "Up"}) != "healthy" {
		t.Error("Feld health hat Vorrang")
	}
}

func TestErrors(t *testing.T) {
	fake, srv := setup(t, token)
	ctx := context.Background()
	code := func(c integrations.Config) string { return i18n.FromError(New().Test(ctx, c)).Code }

	if c := code(cfg(t, srv.URL, nil, token, time.Second)); c != "" {
		t.Fatalf("ok erwartet: %s", c)
	}
	if c := code(cfg(t, srv.URL, nil, "", time.Second)); c != "integration.unauthorized" {
		t.Errorf("ohne Token: %s", c)
	}
	if c := code(cfg(t, srv.URL, nil, "dh_falsch", time.Second)); c != "integration.unauthorized" {
		t.Errorf("falsches Token: %s", c)
	}
	if c := code(cfg(t, srv.URL, map[string]any{"environments": "gibtsnicht"}, token, time.Second)); c != "dockhand.env_missing" {
		t.Errorf("Umgebung: %s", c)
	}
	fake.Broken(true)
	if c := code(cfg(t, srv.URL, nil, token, time.Second)); c != "dockhand.unexpected" {
		t.Errorf("kaputtes JSON: %s", c)
	}
	fake.Broken(false)
	fake.Fail(http.StatusBadGateway)
	if c := code(cfg(t, srv.URL, nil, token, time.Second)); c != "integration.http" {
		t.Errorf("502: %s", c)
	}
	fake.Fail(0)
	fake.Delay(300 * time.Millisecond)
	if c := code(cfg(t, srv.URL, nil, token, 100*time.Millisecond)); c != "integration.timeout" {
		t.Errorf("Timeout: %s", c)
	}
	fake.Delay(0)
	fake.SetEnvs()
	if c := code(cfg(t, srv.URL, nil, token, time.Second)); c != "dockhand.no_envs" {
		t.Errorf("keine Umgebungen: %s", c)
	}
	// andere Web-App unter der Adresse
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html>")) }))
	defer other.Close()
	if c := code(cfg(t, other.URL, nil, token, time.Second)); c != "dockhand.unexpected" {
		t.Errorf("fremd: %s", c)
	}
}
