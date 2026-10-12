package pihole

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/integrations/pihole/piholetest"
)

func setup(t *testing.T, password string) (*piholetest.Fake, *httptest.Server) {
	t.Helper()
	fake := piholetest.New(password)
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return fake, srv
}

func cfg(t *testing.T, url, pw string, extra map[string]any, timeout time.Duration) integrations.Config {
	t.Helper()
	in := map[string]any{"base_url": url}
	for k, v := range extra {
		in[k] = v
	}
	values, msgs := New().Schema().Normalize(in, i18n.EN)
	if len(msgs) != 0 {
		t.Fatal(msgs)
	}
	return integrations.Config{Values: values, Secrets: map[string]string{"password": pw},
		HTTP: integrations.NewHTTPClient(timeout, true), Timeout: timeout}
}

func metrics(t *testing.T, i *Integration, c integrations.Config) (map[string]integrations.Metric, integrations.Snapshot) {
	t.Helper()
	snap, err := i.Collect(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]integrations.Metric{}
	for _, x := range snap.Metrics {
		m[x.Key] = x
	}
	return m, snap
}

func TestCollect(t *testing.T) {
	fake, srv := setup(t, "app-pw")
	i := New()
	c := cfg(t, srv.URL, "app-pw", nil, time.Second)
	m, snap := metrics(t, i, c)
	if snap.Metrics[0].Key != "blocked_pct" || m["queries"].Value != 48213 || m["blocked"].Value != 7391 ||
		m["clients"].Value != 23 || m["gravity_domains"].Value != 182334 || m["blocking"].Value != 1 ||
		m["blocking"].Status != integrations.StatusOK {
		t.Fatalf("%+v", m)
	}
	if p := m["blocked_pct"].Value; p < 15.3 || p > 15.4 || m["blocked_pct"].Unit != "%" {
		t.Errorf("Prozent: %v", p)
	}
	if a := m["gravity_age"]; a.Unit != "s" || a.Value < 26*3600-5 || a.Status != integrations.StatusOK {
		t.Errorf("Gravity-Alter: %+v", a)
	}
	if snap.Info["version"] != "Core v6.1" || snap.Info["ftl"] != "v6.2.3" || m["updates"].Value != 0 || snap.Info["blocking"] != "enabled" {
		t.Errorf("Info: %v %+v", snap.Info, m["updates"])
	}

	// Session wird wiederverwendet: viele Abfragen, eine Anmeldung –
	// mehr als Pi-hole Sessions erlaubt (sonst „API seats exceeded“)
	for range piholetest.MaxSessions + 4 {
		metrics(t, i, c)
	}
	if fake.Logins() != 1 {
		t.Errorf("Anmeldungen: %d", fake.Logins())
	}

	// FTL-Neustart: Session weg → genau eine neue Anmeldung
	fake.Restart()
	metrics(t, i, c)
	if fake.Logins() != 2 {
		t.Errorf("nach Neustart: %d Anmeldungen", fake.Logins())
	}

	// Blocken aus (mit Timer), alte Blockliste, Update verfügbar
	timer := 300.0
	fake.Update(func(f *piholetest.Fake) {
		f.Blocking, f.Timer = "disabled", &timer
		f.GravityUpdated = time.Now().Add(-10 * 24 * time.Hour)
		f.CoreRemote = "v6.2"
	})
	m, snap = metrics(t, i, c)
	if m["blocking"].Status != integrations.StatusWarning || m["blocking"].Value != 0 || snap.Info["blocking_timer"] != "5m0s" {
		t.Errorf("Blocken aus: %+v %v", m["blocking"], snap.Info)
	}
	if m["gravity_age"].Status != integrations.StatusWarning || m["updates"].Value != 1 {
		t.Errorf("Gravity/Updates: %+v %+v", m["gravity_age"], m["updates"])
	}
	m, _ = metrics(t, i, cfg(t, srv.URL, "app-pw", map[string]any{"gravity_warn": 14.0}, time.Second))
	if m["gravity_age"].Status != integrations.StatusOK {
		t.Errorf("Schwelle 14 Tage: %+v", m["gravity_age"])
	}
	fake.Update(func(f *piholetest.Fake) { f.Blocking = "failed" })
	if m, _ = metrics(t, i, c); m["blocking"].Status != integrations.StatusError {
		t.Errorf("Blocken fehlgeschlagen: %+v", m["blocking"])
	}

	// Docker-Installation: Version des Images
	fake.Update(func(f *piholetest.Fake) { f.Docker, f.DockerRemote = "2026.09.0", "2026.10.0" })
	if m, snap = metrics(t, i, c); snap.Info["version"] != "Docker 2026.09.0" || snap.Info["update"] != "2026.10.0" || m["updates"].Value != 1 {
		t.Errorf("Docker: %v", snap.Info)
	}
}

func TestSessionExpiry(t *testing.T) {
	fake, srv := setup(t, "app-pw")
	fake.SetValidity(60)
	i := New()
	now := time.Now()
	i.now = func() time.Time { return now }
	c := cfg(t, srv.URL, "app-pw", nil, time.Second)
	metrics(t, i, c)
	now = now.Add(40 * time.Second) // noch gültig (wird bei Nutzung verlängert)
	metrics(t, i, c)
	now = now.Add(55 * time.Second) // seit der letzten Nutzung abgelaufen
	metrics(t, i, c)
	if fake.Logins() != 2 {
		t.Errorf("Anmeldungen: %d", fake.Logins())
	}
}

func TestWithoutPassword(t *testing.T) {
	fake, srv := setup(t, "")
	m, _ := metrics(t, New(), cfg(t, srv.URL, "", nil, time.Second))
	if m["queries"].Value != 48213 || fake.Logins() != 0 {
		t.Errorf("ohne Passwort: %+v %d", m["queries"], fake.Logins())
	}
}

func TestErrors(t *testing.T) {
	fake, srv := setup(t, "app-pw")
	ctx := context.Background()
	code := func(c integrations.Config) string { return i18n.FromError(New().Test(ctx, c)).Code }

	if err := New().Test(ctx, cfg(t, srv.URL, "app-pw", nil, time.Second)); err != nil {
		t.Fatalf("Test: %v", err)
	}
	if c := code(cfg(t, srv.URL, "falsch", nil, time.Second)); c != "pihole.login_failed" {
		t.Errorf("falsches Passwort: %s", c)
	}
	if c := code(cfg(t, srv.URL, "", nil, time.Second)); c != "pihole.password_required" {
		t.Errorf("ohne Passwort: %s", c)
	}
	fake.Broken(true)
	if c := code(cfg(t, srv.URL, "app-pw", nil, time.Second)); c != "integration.bad_json" {
		t.Errorf("kaputtes JSON: %s", c)
	}
	fake.Broken(false)
	fake.Delay(1500 * time.Millisecond)
	if c := code(cfg(t, srv.URL, "app-pw", nil, 300*time.Millisecond)); c != "integration.timeout" {
		t.Errorf("Timeout: %s", c)
	}
	fake.Delay(0)
	fake.Fail(http.StatusBadGateway)
	if c := code(cfg(t, srv.URL, "app-pw", nil, time.Second)); c != "integration.http" {
		t.Errorf("502: %s", c)
	}
	fake.Fail(0)
	err := New().Test(ctx, cfg(t, srv.URL, "geheim-123", nil, time.Second))
	if err == nil || strings.Contains(err.Error(), "geheim-123") {
		t.Errorf("Fehler: %v", err)
	}
}
