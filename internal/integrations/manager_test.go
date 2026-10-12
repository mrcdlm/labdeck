package integrations_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/integrations/healthchecks"
	"github.com/mrcdlm/labdeck/internal/integrations/healthchecks/hctest"
	"github.com/mrcdlm/labdeck/internal/secrets"
	"github.com/mrcdlm/labdeck/internal/store"
)

type env struct {
	st   *store.Store
	m    *integrations.Manager
	fake *hctest.Fake
	url  string
	pubs int
}

func (e *env) Publish(string) { e.pubs++ }

func newEnv(t *testing.T, keyByte byte) *env {
	t.Helper()
	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	fake := hctest.New("ro-key", "", hctest.Demo()...)
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	e := &env{st: st, fake: fake, url: srv.URL}
	e.m = newManager(t, st, keyByte)
	e.m.Pub = e
	return e
}

func newManager(t *testing.T, st *store.Store, keyByte byte) *integrations.Manager {
	t.Helper()
	var box *secrets.Box
	if keyByte != 0 {
		box, _ = secrets.New(bytes.Repeat([]byte{keyByte}, 32))
	}
	envVars := func(name string) (string, bool) {
		v, ok := map[string]string{"SECRET_HC": "ro-key", "APP_PASSWORD": "pw"}[name]
		return v, ok
	}
	return integrations.NewManager(integrations.NewRegistry(healthchecks.New()), st, box, envVars,
		slog.New(slog.DiscardHandler))
}

func (e *env) input(key string) integrations.Input {
	in := integrations.Input{Type: "healthchecks", Name: "Homelab", Enabled: true, IntervalSeconds: 60,
		TimeoutSeconds: 5, TLSVerify: true, Config: map[string]any{"base_url": e.url}}
	if key != "" {
		in.Secrets = map[string]string{"api_key": key}
	}
	return in
}

func vcodes(t *testing.T, err error) string {
	t.Helper()
	var v *integrations.ValidationError
	if !errors.As(err, &v) {
		t.Fatalf("ValidationError erwartet, bekam %v", err)
	}
	var out []string
	for _, m := range v.Msgs {
		out = append(out, m.Code)
	}
	return strings.Join(out, ",")
}

func TestCreateStoresSecretEncrypted(t *testing.T) {
	e := newEnv(t, 1)
	ctx := context.Background()
	v, err := e.m.Create(ctx, e.input("ro-key"), i18n.DE)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Secrets["api_key"].Set || v.Secrets["api_key"].Ref != "" || v.Config["backup_tag"] != "backup" ||
		v.Status != integrations.StatusUnknown || e.pubs != 1 {
		t.Fatalf("View: %+v pubs=%d", v, e.pubs)
	}
	raw, _ := e.st.GetIntegration(ctx, v.ID)
	if strings.Contains(raw.Secrets, "ro-key") || strings.Contains(raw.Config, "ro-key") || raw.Secrets == "" {
		t.Fatalf("Klartext gespeichert: config=%s secrets=%s", raw.Config, raw.Secrets)
	}
	b, _ := json.Marshal(v)
	if strings.Contains(string(b), "ro-key") {
		t.Fatalf("Secret in der View: %s", b)
	}

	// leer lassen = unverändert, Abfrage funktioniert
	in := e.input("")
	in.Name = "Umbenannt"
	if v, err = e.m.Update(ctx, v.ID, in, i18n.DE); err != nil || v.Name != "Umbenannt" || !v.Secrets["api_key"].Set {
		t.Fatalf("Update: %+v %v", v, err)
	}
	if err := e.m.Poll(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	if v, _ = e.m.Get(ctx, v.ID); v.Status != integrations.StatusOK || v.Snapshot == nil || len(v.Snapshot.Checks) != 7 {
		t.Fatalf("nach Poll: %+v", v)
	}

	// Pflicht-Secret leeren ist nicht erlaubt
	in.ClearSecrets = []string{"api_key"}
	if _, err := e.m.Update(ctx, v.ID, in, i18n.DE); vcodes(t, err) != "integration.field_required" {
		t.Fatalf("leeren: %v", err)
	}
}

func TestSecretRefs(t *testing.T) {
	e := newEnv(t, 0) // ohne LABDECK_SECRET_KEY
	ctx := context.Background()

	if _, err := e.m.Create(ctx, e.input("ro-key"), i18n.DE); vcodes(t, err) != "secret.no_key" {
		t.Fatalf("ohne Schlüssel: %v", err)
	}
	if _, err := e.m.Create(ctx, e.input("${APP_PASSWORD}"), i18n.DE); vcodes(t, err) != "integration.field_secret_ref" {
		t.Fatalf("fremde Variable: %v", err)
	}

	v, err := e.m.Create(ctx, e.input("${SECRET_HC}"), i18n.DE)
	if err != nil {
		t.Fatal(err)
	}
	if st := v.Secrets["api_key"]; !st.Set || st.Ref != "${SECRET_HC}" {
		t.Fatalf("Verweis: %+v", st)
	}
	e.m.Poll(ctx, v.ID)
	if v, _ = e.m.Get(ctx, v.ID); v.Status != integrations.StatusOK {
		t.Fatalf("Poll mit Verweis: %+v %v", v.Status, i18n.T(i18n.EN, v.LastError))
	}

	// Verweis auf nicht gesetzte Variable: Fehler bei der Abfrage, ohne Wert
	in := e.input("${SECRET_FEHLT}")
	v, _ = e.m.Update(ctx, v.ID, in, i18n.DE)
	e.m.Poll(ctx, v.ID)
	v, _ = e.m.Get(ctx, v.ID)
	if v.Status != integrations.StatusError || v.LastError.Code != "secret.env_missing" {
		t.Fatalf("fehlende Variable: %+v", v.LastError)
	}
}

func TestValidation(t *testing.T) {
	e := newEnv(t, 1)
	ctx := context.Background()
	for name, tc := range map[string]struct {
		mod  func(*integrations.Input)
		code string
	}{
		"Typ":            {func(i *integrations.Input) { i.Type = "gibtsnicht" }, "integration.type_unknown"},
		"Name":           {func(i *integrations.Input) { i.Name = " " }, "integration.name_invalid"},
		"Intervall":      {func(i *integrations.Input) { i.IntervalSeconds = 4 }, "integration.interval_range"},
		"Timeout":        {func(i *integrations.Input) { i.TimeoutSeconds = 60 }, "integration.timeout_range"},
		"Secret fehlt":   {func(i *integrations.Input) { i.Secrets = nil }, "integration.field_required"},
		"Feld unbekannt": {func(i *integrations.Input) { i.Secrets["x"] = "y" }, "integration.field_unknown"},
		"URL":            {func(i *integrations.Input) { i.Config["base_url"] = "hc.example.com" }, "integration.field_url"},
	} {
		in := e.input("ro-key")
		tc.mod(&in)
		if _, err := e.m.Create(ctx, in, i18n.DE); vcodes(t, err) != tc.code {
			t.Errorf("%s: %v", name, err)
		}
	}

	v, _ := e.m.Create(ctx, e.input("ro-key"), i18n.DE)
	in := e.input("")
	in.Type = "prometheus"
	if _, err := e.m.Update(ctx, v.ID, in, i18n.DE); !strings.Contains(vcodes(t, err), "integration.type_change") {
		t.Fatalf("Typwechsel: %v", err)
	}
	if _, err := e.m.Update(ctx, 999, in, i18n.DE); !errors.Is(err, integrations.ErrNotFound) {
		t.Fatalf("unbekannt: %v", err)
	}
	if err := e.m.Delete(ctx, 999); !errors.Is(err, integrations.ErrNotFound) {
		t.Fatalf("löschen unbekannt: %v", err)
	}
}

func TestPollStatusChanges(t *testing.T) {
	e := newEnv(t, 1)
	ctx := context.Background()
	v, _ := e.m.Create(ctx, e.input("ro-key"), i18n.DE)

	var polled []store.StatusChange
	e.m.OnPolled = func(it integrations.PolledIntegration, ch []store.StatusChange) {
		if it.ID != v.ID || it.Type != "healthchecks" {
			t.Errorf("OnPolled: %+v", it)
		}
		polled = ch
	}
	e.m.Poll(ctx, v.ID)
	changes, _ := e.st.ListStatusChanges(ctx, v.ID, 100)
	if len(polled) != len(changes) {
		t.Fatalf("OnPolled: %d Wechsel, gespeichert %d", len(polled), len(changes))
	}
	kinds := map[string]int{}
	for _, c := range changes {
		kinds[c.Kind]++
	}
	// Integration unknown→ok; neue Checks/Kennzahlen nur mit Problem (vzdump down, Borg grace)
	if kinds["integration"] != 1 || kinds["check"] != 2 || kinds["backup"] != 0 || kinds["metric"] != 0 {
		t.Fatalf("erste Abfrage: %v", kinds)
	}

	// unverändert → keine neuen Wechsel
	e.m.Poll(ctx, v.ID)
	if again, _ := e.st.ListStatusChanges(ctx, v.ID, 100); len(again) != len(changes) {
		t.Fatalf("ohne Änderung: %d → %d", len(changes), len(again))
	}

	e.fake.SetStatus("Proxmox vzdump", "up")
	e.m.Poll(ctx, v.ID)
	latest, _ := e.st.ListStatusChanges(ctx, v.ID, 3)
	var gotCheck bool
	for _, c := range latest {
		if c.Kind == "check" && c.ItemName == "Proxmox vzdump" && c.From == "error" && c.To == "ok" {
			gotCheck = true
		}
	}
	if !gotCheck {
		t.Fatalf("Check-Wechsel fehlt: %+v", latest)
	}

	// Quelle nicht erreichbar → Integration error, Snapshot bleibt
	e.fake.Fail(http.StatusUnauthorized)
	e.m.Poll(ctx, v.ID)
	v, _ = e.m.Get(ctx, v.ID)
	if v.Status != integrations.StatusError || v.LastError.Code != "integration.unauthorized" ||
		v.Snapshot == nil || len(v.Snapshot.Checks) != 7 {
		t.Fatalf("Fehler: %+v", v)
	}
	last, _ := e.st.ListStatusChanges(ctx, v.ID, 1)
	if last[0].Kind != "integration" || last[0].To != "error" || !strings.Contains(last[0].Message, "integration.unauthorized") {
		t.Fatalf("Fehler-Wechsel: %+v", last[0])
	}

	e.fake.Fail(0)
	e.m.Poll(ctx, v.ID)
	if v, _ = e.m.Get(ctx, v.ID); v.Status != integrations.StatusOK || v.LastError.Code != "" {
		t.Fatalf("wieder ok: %+v", v)
	}

	// Kennzahlen landen als Tageswerte
	days, _ := e.st.MetricDaily(ctx, v.ID, "checks_total", "")
	if len(days) != 1 || days[0].Last != 7 {
		t.Fatalf("Tageswerte: %+v", days)
	}

	// deaktiviert → Status paused in der Ansicht, nicht in den Zeitplänen
	in := e.input("")
	in.Enabled = false
	v, _ = e.m.Update(ctx, v.ID, in, i18n.DE)
	if sched, _ := e.m.Schedules(ctx); v.Status != integrations.StatusPaused || len(sched) != 0 {
		t.Fatalf("deaktiviert: %s %v", v.Status, sched)
	}

	// gelöschte Integration: Poll ist harmlos
	e.m.Delete(ctx, v.ID)
	if err := e.m.Poll(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
}

func TestTest(t *testing.T) {
	e := newEnv(t, 1)
	ctx := context.Background()

	res, err := e.m.Test(ctx, 0, e.input("ro-key"), i18n.DE)
	if err != nil || !res.OK || res.Checks != 7 || res.Backups != 3 {
		t.Fatalf("Test: %+v %v", res, err)
	}
	res, _ = e.m.Test(ctx, 0, e.input("falsch"), i18n.DE)
	if res.OK || res.Error.Code != "integration.unauthorized" {
		t.Fatalf("falscher Key: %+v", res)
	}
	if _, err := e.m.Test(ctx, 0, e.input(""), i18n.DE); vcodes(t, err) != "integration.field_required" {
		t.Fatalf("ohne Key: %v", err)
	}

	// gespeicherte Integration: leeres Secret = gespeicherter Wert
	v, _ := e.m.Create(ctx, e.input("ro-key"), i18n.DE)
	if res, _ := e.m.Test(ctx, v.ID, e.input(""), i18n.DE); !res.OK {
		t.Fatalf("mit gespeichertem Key: %+v", res)
	}
	// Test speichert nichts
	if got, _ := e.m.Get(ctx, v.ID); got.LastPollAt != nil {
		t.Fatal("Test hat gespeichert")
	}
}

func TestKeyChanged(t *testing.T) {
	e := newEnv(t, 1)
	ctx := context.Background()
	v, _ := e.m.Create(ctx, e.input("ro-key"), i18n.DE)

	// Neustart mit anderem Schlüssel
	other := newManager(t, e.st, 2)
	other.Poll(ctx, v.ID)
	got, _ := other.Get(ctx, v.ID)
	if got.Status != integrations.StatusError || got.LastError.Code != "secret.decrypt_failed" || !got.Secrets["api_key"].Set {
		t.Fatalf("anderer Schlüssel: %+v", got)
	}
	// Speichern ohne neuen Key: verständlicher Hinweis
	if _, err := other.Update(ctx, v.ID, e.input(""), i18n.DE); vcodes(t, err) != "integration.field_secret_ref" {
		t.Fatalf("Update: %v", err)
	}
	// mit neuem Key wieder nutzbar
	if _, err := other.Update(ctx, v.ID, e.input("ro-key"), i18n.DE); err != nil {
		t.Fatal(err)
	}
	other.Poll(ctx, v.ID)
	if got, _ = other.Get(ctx, v.ID); got.Status != integrations.StatusOK {
		t.Fatalf("nach Neueingabe: %+v", got)
	}
}
