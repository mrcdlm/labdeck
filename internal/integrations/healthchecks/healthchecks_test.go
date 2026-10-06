package healthchecks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/integrations/healthchecks/hctest"
)

func setup(t *testing.T) (*hctest.Fake, *httptest.Server) {
	t.Helper()
	fake := hctest.New("ro-key", "rw-key", hctest.Demo()...)
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return fake, srv
}

func cfg(url, key string, timeout time.Duration) integrations.Config {
	values, msgs := New().Schema().Normalize(map[string]any{"base_url": url}, i18n.EN)
	if len(msgs) != 0 {
		panic(msgs)
	}
	return integrations.Config{Values: values, Secrets: map[string]string{"api_key": key},
		HTTP: integrations.NewHTTPClient(timeout, true), Timeout: timeout}
}

func code(err error) string { return i18n.FromError(err).Code }

func TestCollectReadOnlyKey(t *testing.T) {
	_, srv := setup(t)
	snap, err := New().Collect(context.Background(), cfg(srv.URL+"/", "ro-key", time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Checks) != 7 {
		t.Fatalf("checks: %d", len(snap.Checks))
	}
	byName := map[string]integrations.Check{}
	ids := map[string]bool{}
	for _, c := range snap.Checks {
		byName[c.Name] = c
		if ids[c.ID] {
			t.Errorf("ID %q doppelt", c.ID)
		}
		ids[c.ID] = true
		if c.ID == "" || c.ID[:2] != "uk" {
			t.Errorf("%s: ID %q (unique_key erwartet)", c.Name, c.ID)
		}
	}
	for name, want := range map[string]integrations.Status{
		"Restic NAS → B2": integrations.StatusOK, "Borg Laptop": integrations.StatusWarning,
		"Proxmox vzdump": integrations.StatusError, "Alter Sync-Job": integrations.StatusPaused,
		"Neuer Check": integrations.StatusUnknown,
	} {
		if byName[name].Status != want {
			t.Errorf("%s: %s, erwartet %s", name, byName[name].Status, want)
		}
	}
	r := byName["Restic NAS → B2"]
	if r.LastPing == nil || time.Since(*r.LastPing) < 2*time.Hour || r.NextPing == nil ||
		len(r.Tags) != 2 || r.Raw != "up" {
		t.Errorf("Restic: %+v", r)
	}
	if byName["Neuer Check"].LastPing != nil {
		t.Error("Neuer Check hat last_ping")
	}

	// Backups: Checks mit Tag "backup"
	if len(snap.Backups) != 3 {
		t.Fatalf("backups: %+v", snap.Backups)
	}
	for _, b := range snap.Backups {
		if b.ID != byName[b.Name].ID || b.Result != byName[b.Name].Status {
			t.Errorf("backup %+v", b)
		}
	}

	metrics := map[string]integrations.Metric{}
	for _, m := range snap.Metrics {
		metrics[m.Key] = m
	}
	if metrics["checks_total"].Value != 7 || metrics["checks_down"].Value != 1 ||
		metrics["checks_down"].Status != integrations.StatusError || metrics["checks_grace"].Status != integrations.StatusWarning {
		t.Errorf("metrics: %+v", metrics)
	}
}

func TestCollectReadWriteKeyAndBackupTag(t *testing.T) {
	_, srv := setup(t)
	c := cfg(srv.URL, "rw-key", time.Second)
	c.Values["backup_tag"] = "proxmox"
	snap, err := New().Collect(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Checks[0].ID != "0b6f0c1e-1d5a-4d0e-9a51-6f1c1f6a0001" {
		t.Errorf("UUID erwartet: %q", snap.Checks[0].ID)
	}
	if len(snap.Backups) != 1 || snap.Backups[0].Name != "Proxmox vzdump" {
		t.Errorf("backups: %+v", snap.Backups)
	}

	c.Values["backup_tag"] = ""
	snap, _ = New().Collect(context.Background(), c)
	if len(snap.Backups) != 0 {
		t.Errorf("ohne Tag: %+v", snap.Backups)
	}
}

func TestErrors(t *testing.T) {
	fake, srv := setup(t)
	ctx := context.Background()
	hc := New()

	if err := hc.Test(ctx, cfg(srv.URL, "ro-key", time.Second)); err != nil {
		t.Fatalf("Test: %v", err)
	}
	if err := hc.Test(ctx, cfg(srv.URL, "falsch", time.Second)); code(err) != "integration.unauthorized" {
		t.Errorf("401: %v", err)
	}

	fake.Broken(true)
	if _, err := hc.Collect(ctx, cfg(srv.URL, "ro-key", time.Second)); code(err) != "integration.bad_json" {
		t.Errorf("kaputtes JSON: %v", err)
	}
	fake.Broken(false)

	fake.Fail(http.StatusInternalServerError)
	if _, err := hc.Collect(ctx, cfg(srv.URL, "ro-key", time.Second)); code(err) != "integration.http" {
		t.Errorf("500: %v", err)
	}
	fake.Fail(0)

	fake.Delay(300 * time.Millisecond)
	if err := hc.Test(ctx, cfg(srv.URL, "ro-key", 100*time.Millisecond)); code(err) != "integration.timeout" {
		t.Errorf("Timeout: %v", err)
	}
	fake.Delay(0)

	// Adresse ist kein Healthchecks (anderes JSON bzw. falscher Pfad)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/checks/" {
			w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer other.Close()
	if err := hc.Test(ctx, cfg(other.URL, "ro-key", time.Second)); code(err) != "healthchecks.unexpected" {
		t.Errorf("fremdes JSON: %v", err)
	}
	if err := hc.Test(ctx, cfg(other.URL+"/unterpfad", "ro-key", time.Second)); code(err) != "integration.not_found" {
		t.Errorf("404: %v", err)
	}
}

func TestSchemaDefaults(t *testing.T) {
	values, msgs := New().Schema().Normalize(map[string]any{}, i18n.DE)
	if len(msgs) != 0 || values["base_url"] != "https://healthchecks.io" || values["backup_tag"] != "backup" {
		t.Fatalf("%v %v", values, msgs)
	}
}
