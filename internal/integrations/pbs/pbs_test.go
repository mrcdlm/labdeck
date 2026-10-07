package pbs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/integrations/pbs/pbstest"
)

const token = "labdeck@pbs!dash"

func setup(t *testing.T, stores ...pbstest.Store) (*pbstest.Fake, *httptest.Server) {
	t.Helper()
	if stores == nil {
		stores = pbstest.Demo()
	}
	fake := pbstest.New(token+":geheim", stores...)
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return fake, srv
}

func cfg(t *testing.T, url string, extra map[string]any, secret string, timeout time.Duration) integrations.Config {
	t.Helper()
	in := map[string]any{"base_url": url, "token_id": token}
	for k, v := range extra {
		in[k] = v
	}
	values, msgs := New().Schema().Normalize(in, i18n.EN)
	if len(msgs) != 0 {
		t.Fatal(msgs)
	}
	return integrations.Config{Values: values, Secrets: map[string]string{"token_secret": secret},
		HTTP: integrations.NewHTTPClient(timeout, true), Timeout: timeout}
}

func TestCollect(t *testing.T) {
	_, srv := setup(t)
	snap, err := New().Collect(context.Background(), cfg(t, srv.URL+"/", nil, "geheim", time.Second))
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]integrations.Metric{}
	for _, x := range snap.Metrics {
		m[x.Key] = x
	}
	if m["groups"].Value != 6 || m["verify_failed"].Value != 1 || m["verify_failed"].Status != integrations.StatusError ||
		!m["verify_failed"].Derived || m["datastores"].Value != 2 {
		t.Errorf("Übersicht: %+v %+v %+v", m["groups"], m["verify_failed"], m["datastores"])
	}
	if l := m["local/used_pct"]; l.Value != 73 || l.Status != integrations.StatusOK || l.Group != "local" {
		t.Errorf("local: %+v", l)
	}
	if o := m["offsite/used_pct"]; o.Value != 93.5 || o.Status != integrations.StatusError {
		t.Errorf("offsite: %+v", o)
	}
	if m["local/snapshots"].Value != 17 || m["local/groups"].Value != 5 || m["local/used"].Unit != "bytes" {
		t.Errorf("Snapshots: %+v %+v", m["local/snapshots"], m["local/groups"])
	}

	b := map[string]integrations.Backup{}
	var names []string
	for _, x := range snap.Backups {
		b[x.ID] = x
		names = append(names, x.Name)
	}
	if len(snap.Backups) != 6 {
		t.Fatalf("Backups: %v", names)
	}
	// Reihenfolge: Gruppe, dann Typ und ID numerisch
	if names[0] != "ct/101 (pihole)" || names[3] != "vm/102 (nextcloud)" {
		t.Errorf("Reihenfolge: %v", names)
	}
	ha := b["local//vm/100"]
	if ha.Name != "vm/100 (homeassistant)" || ha.Group != "local" || ha.Result != integrations.StatusOK || ha.Verify != "ok" ||
		*ha.SizeBytes != 34e9 || time.Since(*ha.LastRun) > 6*time.Hour {
		t.Errorf("vm/100: %+v", ha)
	}
	if nc := b["local//vm/102"]; nc.Result != integrations.StatusError || nc.Verify != "failed" {
		t.Errorf("vm/102: %+v", nc)
	}
	if lab := b["local/lab/ct/200"]; lab.Group != "local · lab" || lab.Verify != "none" {
		t.Errorf("Namespace: %+v", lab)
	}
	if len(ha.Runs) != 5 || b["local//vm/102"].Runs[4].Result != integrations.StatusError {
		t.Errorf("Läufe: %+v", ha.Runs)
	}
	if host := b["local//host/nas"]; time.Since(*host.LastRun) < 3*24*time.Hour {
		t.Errorf("host/nas: %+v", host)
	}
}

func TestDatastoreFilterAndErrors(t *testing.T) {
	fake, srv := setup(t)
	ctx := context.Background()
	snap, err := New().Collect(ctx, cfg(t, srv.URL, map[string]any{"datastores": " offsite ", "warn_pct": 95.0, "error_pct": 99.0}, "geheim", time.Second))
	if err != nil || len(snap.Backups) != 1 {
		t.Fatalf("Filter: %v %+v", err, snap.Backups)
	}
	for _, x := range snap.Metrics {
		if x.Key == "offsite/used_pct" && x.Status != integrations.StatusOK {
			t.Errorf("eigene Schwellwerte: %+v", x)
		}
	}

	code := func(c integrations.Config) string { return i18n.FromError(New().Test(ctx, c)).Code }
	if c := code(cfg(t, srv.URL, map[string]any{"datastores": "gibtsnicht"}, "geheim", time.Second)); c != "pbs.datastore_missing" {
		t.Errorf("unbekannter Datastore: %s", c)
	}
	if c := code(cfg(t, srv.URL, nil, "falsch", time.Second)); c != "integration.unauthorized" {
		t.Errorf("401: %s", c)
	}
	fake.Broken(true)
	if c := code(cfg(t, srv.URL, nil, "geheim", time.Second)); c != "integration.bad_json" {
		t.Errorf("kaputtes JSON: %s", c)
	}
	fake.Broken(false)
	fake.Delay(300 * time.Millisecond)
	if c := code(cfg(t, srv.URL, nil, "geheim", 100*time.Millisecond)); c != "integration.timeout" {
		t.Errorf("Timeout: %s", c)
	}
	fake.Delay(0)

	// Token ohne Rechte sieht keine Datastores
	_, empty := setup(t, []pbstest.Store{}...)
	if c := code(cfg(t, empty.URL, nil, "geheim", time.Second)); c != "pbs.no_datastores" {
		t.Errorf("keine Datastores: %s", c)
	}
	// fremde Antwort
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"x":1}`)) }))
	defer other.Close()
	if c := code(cfg(t, other.URL, nil, "geheim", time.Second)); c != "pbs.unexpected" {
		t.Errorf("fremd: %s", c)
	}
}

func TestUnreadableDatastore(t *testing.T) {
	_, srv := setup(t, pbstest.Store{Name: "usb", Error: "datastore is not mounted"})
	snap, err := New().Collect(context.Background(), cfg(t, srv.URL, nil, "geheim", time.Second))
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range snap.Metrics {
		if x.Key == "usb/used_pct" && x.Status != integrations.StatusError {
			t.Errorf("%+v", x)
		}
	}
}
