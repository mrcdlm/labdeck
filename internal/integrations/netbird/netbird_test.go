package netbird

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/integrations/netbird/netbirdtest"
)

const token = "nbp_testtoken"

func setup(t *testing.T) (*netbirdtest.Fake, *httptest.Server) {
	t.Helper()
	fake := netbirdtest.New(token).Demo()
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
	snap, err := New().Collect(context.Background(), cfg(t, srv.URL+"/", map[string]any{"watch_group": "Server"}, token, time.Second))
	if err != nil {
		t.Fatal(err)
	}
	m, r := index(snap)
	if m["peers"].Value != 5 || m["peers_online"].Value != 3 || m["proxies"].Value != 3 || m["proxies_active"].Value != 2 {
		t.Errorf("Übersicht: %+v", snap.Metrics)
	}
	if w := m["watched_offline"]; w.Value != 1 || w.Status != integrations.StatusError || !w.Derived {
		t.Errorf("überwacht offline: %+v", w)
	}
	if snap.Info["version"] != "NetBird 0.66.2" {
		t.Errorf("Version: %v", snap.Info)
	}
	d := r["peer/p1"]
	if d.Kind != integrations.KindPeer || d.Name != "docker01" || d.State != "online" || d.Status != integrations.StatusOK ||
		d.Info["ip"] != "100.92.10.1" || d.Info["dns"] != "docker01.netbird.cloud" || d.Info["watched"] != "1" ||
		d.Info["version"] != "0.66.2" || len(d.Tags) != 1 || d.Tags[0] != "server" || d.Info["last_seen"] == "" {
		t.Errorf("Server-Peer: %+v", d)
	}
	// überwachter Peer offline → Problem
	if p := r["peer/p3"]; p.State != "offline" || p.Status != integrations.StatusError {
		t.Errorf("pi-edge: %+v", p)
	}
	if pm := m["res/peer/p3/peer"]; pm.Status != integrations.StatusError || pm.Group != "pi-edge" {
		t.Errorf("Problem-Kennzahl: %+v", pm)
	}
	// Laptop offline: nur Anzeige
	if l := r["peer/p4"]; l.State != "offline" || l.Status != integrations.StatusPaused || l.Info["watched"] != "" {
		t.Errorf("Laptop: %+v", l)
	}
	if _, ok := m["res/peer/p4/peer"]; ok {
		t.Error("Laptop als Problem")
	}
	ph := r["proxy/s1"]
	if ph.Kind != integrations.KindProxy || ph.Name != "photos.example.org" || ph.State != "active" ||
		ph.Status != integrations.StatusOK || ph.Info["targets"] != "docker01:2283" || ph.Info["cluster"] != "eu.proxy.netbird.io" ||
		ph.Info["cert_issued"] != "2026-11-01T10:00:00Z" {
		t.Errorf("Proxy-Dienst: %+v", ph)
	}
	if pm := m["res/proxy/s3/proxy"]; pm.Status != integrations.StatusError || pm.Group != "ha.example.org" {
		t.Errorf("Zertifikat fehlgeschlagen: %+v", pm)
	}
}

func TestWatchAndProxyStates(t *testing.T) {
	fake, srv := setup(t)
	ctx := context.Background()
	// ohne Auswahl: nichts überwacht, keine Kennzahl dafür
	snap, err := New().Collect(ctx, cfg(t, srv.URL, nil, token, time.Second))
	if err != nil {
		t.Fatal(err)
	}
	m, r := index(snap)
	if _, ok := m["watched_offline"]; ok || r["peer/p3"].Status != integrations.StatusPaused {
		t.Errorf("ohne Auswahl: %+v %+v", m["watched_offline"], r["peer/p3"])
	}
	// einzelne Peers per Name bzw. DNS-Kurzname, Login abgelaufen = Warnung
	fake.Update(func(f *netbirdtest.Fake) { f.Peers[1].LoginExpired = true })
	snap, _ = New().Collect(ctx, cfg(t, srv.URL, map[string]any{"watch_peers": "NAS, marco-laptop"}, token, time.Second))
	m, r = index(snap)
	if r["peer/p2"].Status != integrations.StatusWarning || r["peer/p2"].State != "login_expired" ||
		r["peer/p4"].Status != integrations.StatusError || m["watched_offline"].Value != 1 {
		t.Errorf("Namen: %+v %+v %+v", r["peer/p2"], r["peer/p4"], m["watched_offline"])
	}
	// Dienst deaktiviert / Zertifikat ausstehend / Cluster offline
	fake.Update(func(f *netbirdtest.Fake) {
		f.Services[0].Enabled = false
		f.Services[1].Status = "certificate_pending"
		f.Offline = []string{"eu.proxy.netbird.io"}
	})
	snap, _ = New().Collect(ctx, cfg(t, srv.URL, nil, token, time.Second))
	m, r = index(snap)
	if r["proxy/s1"].Status != integrations.StatusPaused || r["proxy/s1"].State != "disabled" ||
		m["res/proxy/s2/proxy"].Status != integrations.StatusWarning ||
		m["cluster/eu.proxy.netbird.io/online"].Status != integrations.StatusError {
		t.Errorf("Proxy-Zustände: %+v", snap.Metrics)
	}
	// ältere Version ohne Reverse Proxy: Peers trotzdem
	fake.Update(func(f *netbirdtest.Fake) { f.NoProxy = true })
	snap, err = New().Collect(ctx, cfg(t, srv.URL, nil, token, time.Second))
	m, _ = index(snap)
	if _, ok := m["proxies"]; err != nil || ok || m["peers"].Value != 5 {
		t.Errorf("ohne Reverse Proxy: %v %+v", err, snap.Metrics)
	}
	// Reverse Proxy abgeschaltet
	fake.Update(func(f *netbirdtest.Fake) { f.NoProxy = false })
	before := fake.Requests()
	New().Collect(ctx, cfg(t, srv.URL, map[string]any{"reverse_proxy": false}, token, time.Second))
	if n := fake.Requests() - before; n != 2 { // peers + version
		t.Errorf("Anfragen ohne Reverse Proxy: %d", n)
	}
}

func TestErrors(t *testing.T) {
	fake, srv := setup(t)
	ctx := context.Background()
	code := func(c integrations.Config) string { return i18n.FromError(New().Test(ctx, c)).Code }

	if c := code(cfg(t, srv.URL, nil, token, time.Second)); c != "" {
		t.Fatalf("ok erwartet: %s", c)
	}
	if c := code(cfg(t, srv.URL, nil, "nbp_falsch", time.Second)); c != "integration.unauthorized" {
		t.Errorf("falsches Token: %s", c)
	}
	fake.Broken(true)
	if c := code(cfg(t, srv.URL, nil, token, time.Second)); c != "netbird.unexpected" {
		t.Errorf("kaputtes JSON: %s", c)
	}
	fake.Broken(false)
	fake.Fail(http.StatusInternalServerError)
	if c := code(cfg(t, srv.URL, nil, token, time.Second)); c != "integration.http" {
		t.Errorf("500: %s", c)
	}
	fake.Fail(0)
	fake.Delay(300 * time.Millisecond)
	if c := code(cfg(t, srv.URL, nil, token, 100*time.Millisecond)); c != "integration.timeout" {
		t.Errorf("Timeout: %s", c)
	}
	fake.Delay(0)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html>")) }))
	defer other.Close()
	if c := code(cfg(t, other.URL, nil, token, time.Second)); c != "netbird.unexpected" {
		t.Errorf("fremd: %s", c)
	}
}
