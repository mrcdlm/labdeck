package redfish

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/integrations/redfish/redfishtest"
)

func setup(t *testing.T, p redfishtest.Profile) (*redfishtest.Fake, *httptest.Server) {
	t.Helper()
	fake := redfishtest.New("labdeck", "geheim-pw", p)
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return fake, srv
}

func cfg(t *testing.T, url, pw string, extra map[string]any, timeout time.Duration) integrations.Config {
	t.Helper()
	in := map[string]any{"base_url": url, "username": "labdeck"}
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

func collect(t *testing.T, c integrations.Config) (map[string]integrations.Metric, integrations.Resource, integrations.Snapshot) {
	t.Helper()
	snap, err := New().Collect(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]integrations.Metric{}
	for _, x := range snap.Metrics {
		m[x.Key] = x
	}
	if len(snap.Resources) != 1 {
		t.Fatalf("Ressourcen: %+v", snap.Resources)
	}
	return m, snap.Resources[0], snap
}

// problems: Kennzahlen, die im Gesamtstatus als Problem zählen.
func problems(m map[string]integrations.Metric) []string {
	var out []string
	for k, x := range m {
		if !x.Derived && (x.Status == integrations.StatusError || x.Status == integrations.StatusWarning) {
			out = append(out, k)
		}
	}
	return out
}

func TestCollectILO5(t *testing.T) {
	fake, srv := setup(t, redfishtest.ILO5())
	c := cfg(t, srv.URL, "geheim-pw", nil, 2*time.Second)
	m, res, snap := collect(t, c)

	if res.Kind != integrations.KindServer || res.Name != "pve-01" || res.State != "on" || res.Status != integrations.StatusOK ||
		res.Info["model"] != "ProLiant DL380 Gen10" || res.Info["bmc_firmware"] != "iLO 5 v3.19" || res.Info["cpus"] != "2" ||
		res.Info["memory_gib"] != "256" || res.Info["post"] != "FinishedPost" {
		t.Fatalf("Server: %+v", res)
	}
	if snap.Info["version"] != "ProLiant DL380 Gen10" || snap.Info["firmware"] != "iLO 5 v3.19" {
		t.Errorf("Info: %v", snap.Info)
	}
	if snap.Metrics[0].Key != "power" || m["power"].Value != 212 || m["power"].Unit != "W" {
		t.Errorf("Leistung zuerst: %+v", snap.Metrics[0])
	}
	if m["inlet"].Value != 23 || m["inlet"].Unit != "°C" || !m["health"].Derived || m["health"].Value != 1 {
		t.Errorf("Hauptwerte: %+v %+v", m["inlet"], m["health"])
	}
	if x := m["temp/02-CPU 1/temp"]; x.Value != 41 || x.Status != integrations.StatusOK || x.Resource != "server" {
		t.Errorf("CPU-Temperatur: %+v", x)
	}
	if _, ok := m["temp/20-PCI 1/temp"]; ok {
		t.Error("fehlender Sensor darf nicht erscheinen")
	}
	if m["fan/Fan 3/fan"].Value != 25 || m["psu/0/supply"].Group != "PSU 1" || m["psu/1/output"].Value != 102 {
		t.Errorf("Lüfter/Netzteile: %+v %+v %+v", m["fan/Fan 3/fan"], m["psu/0/supply"], m["psu/1/output"])
	}
	// SmartStorage per $expand: Controller, vier Laufwerke, zwei RAIDs
	if x := m["drive/1I:1:1/life"]; x.Value != 93 || x.Group != "1I:1:1 · MO000960JWFWR · 960 GB" {
		t.Errorf("SSD: %+v", x)
	}
	if m["drive/1I:1:3/drive"].Group != "1I:1:3 · MB004000GWKGV · 4 TB" || m["volume/0-1/volume"].Group != "LD 1 · boot · RAID 1" ||
		m["controller/0/controller"].Value != 1 {
		t.Errorf("Storage: %+v %+v", m["drive/1I:1:3/drive"], m["volume/0-1/volume"])
	}
	// HPE-Sammelzustände; mit Einzelwerten zählen nur diese
	if x := m["component/memory"]; x.Derived || x.Value != 1 {
		t.Errorf("Speicher: %+v", x)
	}
	if !m["component/fans"].Derived || !m["component/storage"].Derived {
		t.Error("Lüfter/Storage sind durch Einzelwerte abgedeckt")
	}
	if _, ok := m["component/agentless"]; ok {
		t.Error("AMS ist kein Hardware-Zustand")
	}
	if p := problems(m); len(p) != 0 {
		t.Errorf("Probleme: %v", p)
	}
	reqs := fake.Requests()

	// Netzteil fällt aus, Lüfter warnt, SSD verschlissen, CPU zu heiß
	fake.Update(func(p *redfishtest.Profile) {
		p.PSUs[1].Health = "Critical"
		p.Fans[2].Health = "Warning"
		p.Drives[1].WearUsed = 85
		p.Temps[1].Reading = 64 // 6 °C unter kritisch
		p.Health = "Critical"
	})
	m, res, _ = collect(t, c)
	if m["psu/1/supply"].Status != integrations.StatusError || m["fan/Fan 3/fan"].Status != integrations.StatusWarning ||
		m["drive/1I:1:2/life"].Status != integrations.StatusWarning || m["temp/02-CPU 1/temp"].Status != integrations.StatusWarning {
		t.Errorf("Ausfälle: %+v %+v %+v %+v", m["psu/1/supply"], m["fan/Fan 3/fan"], m["drive/1I:1:2/life"], m["temp/02-CPU 1/temp"])
	}
	if x := m["component/psu_redundancy"]; x.Status != integrations.StatusWarning || x.Derived {
		t.Errorf("Redundanz: %+v", x)
	}
	if res.Status != integrations.StatusError || m["health"].Status != integrations.StatusError {
		t.Errorf("Gesamt: %s %+v", res.Status, m["health"])
	}
	// jede Ursache genau einmal als Problem (nicht zusätzlich der Sammelzustand)
	if p := problems(m); len(p) != 5 {
		t.Errorf("Probleme: %v", p)
	}
	if fake.Requests()-reqs > 12 {
		t.Errorf("zu viele Anfragen: %d", fake.Requests()-reqs)
	}

	// Server aus: Warnung (abschaltbar)
	fake.Update(func(p *redfishtest.Profile) { *p = redfishtest.ILO5(); p.PowerOn = false })
	m, res, _ = collect(t, c)
	if res.State != "off" || res.Status != integrations.StatusWarning || m["power_state"].Status != integrations.StatusWarning {
		t.Errorf("aus: %+v %+v", res, m["power_state"])
	}
	m, res, _ = collect(t, cfg(t, srv.URL, "geheim-pw", map[string]any{"power_off_warn": false}, 2*time.Second))
	if res.Status != integrations.StatusOK || m["power_state"].Key != "" {
		t.Errorf("aus ohne Warnung: %+v", res)
	}
}

func TestCollectGeneric(t *testing.T) {
	fake, srv := setup(t, redfishtest.Generic())
	c := cfg(t, srv.URL+"/", "geheim-pw", nil, 2*time.Second)
	m, res, _ := collect(t, c)
	if res.Name != "backup-box" || res.Info["post"] != "" {
		t.Errorf("Server: %+v", res)
	}
	// ohne Oem.Hpe: Teilzustände aus Prozessoren, Speicher und Einzelwerten
	for _, k := range []string{"processors", "memory", "temperatures", "fans", "power_supplies"} {
		if _, ok := m["component/"+k]; !ok {
			t.Errorf("%s fehlt", k)
		}
	}
	// DMTF Storage ohne $expand: Laufwerke einzeln
	if x := m["drive/Disk.Bay.1/drive"]; x.Group != "Disk.Bay.1 · WDC WD80EFZZ · 8 TB" || x.Value != 1 {
		t.Errorf("Laufwerk: %+v", x)
	}
	if m["volume/0/volume"].Group != "md0 · RAID1" || m["controller/1/controller"].Value != 1 {
		t.Errorf("Volume: %+v", m["volume/0/volume"])
	}
	fake.Update(func(p *redfishtest.Profile) { p.Drives[0].Health = "Critical" })
	m, _, _ = collect(t, c)
	if m["drive/Disk.Bay.0/drive"].Status != integrations.StatusError {
		t.Errorf("defektes Laufwerk: %+v", m["drive/Disk.Bay.0/drive"])
	}

	// Laufwerke abgeschaltet: keine Storage-Anfragen
	m, _, _ = collect(t, cfg(t, srv.URL, "geheim-pw", map[string]any{"drives": false}, 2*time.Second))
	for k := range m {
		if strings.HasPrefix(k, "drive/") {
			t.Fatalf("Laufwerk trotz drives=false: %s", k)
		}
	}
}

func TestErrors(t *testing.T) {
	fake, srv := setup(t, redfishtest.ILO5())
	ctx := context.Background()
	code := func(c integrations.Config) string { return i18n.FromError(New().Test(ctx, c)).Code }

	if err := New().Test(ctx, cfg(t, srv.URL, "geheim-pw", nil, time.Second)); err != nil {
		t.Fatalf("Test: %v", err)
	}
	if c := code(cfg(t, srv.URL, "falsch", nil, time.Second)); c != "integration.unauthorized" {
		t.Errorf("401: %s", c)
	}
	fake.Broken(true)
	if c := code(cfg(t, srv.URL, "geheim-pw", nil, time.Second)); c != "integration.bad_json" {
		t.Errorf("kaputtes JSON: %s", c)
	}
	fake.Broken(false)
	fake.Delay(1500 * time.Millisecond)
	if c := code(cfg(t, srv.URL, "geheim-pw", nil, 300*time.Millisecond)); c != "integration.timeout" {
		t.Errorf("Timeout: %s", c)
	}
	fake.Delay(0)
	fake.Fail(http.StatusServiceUnavailable)
	_, err := New().Collect(ctx, cfg(t, srv.URL, "geheim-pw", nil, time.Second))
	if i18n.FromError(err).Code != "integration.http" {
		t.Errorf("503: %v", err)
	}
	// das Passwort steht in keiner Fehlermeldung
	fake.Fail(0)
	err = New().Test(ctx, cfg(t, srv.URL, "geheim-123", nil, time.Second))
	if err == nil || strings.Contains(err.Error(), "geheim-123") {
		t.Errorf("Fehler: %v", err)
	}
}

// Verweise der Antwort dürfen nicht auf fremde Hosts zeigen.
func TestForeignLink(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"Members":[{"@odata.id":"http://evil.example/redfish/v1/Systems/1"}]}`))
	}))
	t.Cleanup(srv.Close)
	err := New().Test(context.Background(), cfg(t, srv.URL, "x", nil, time.Second))
	if i18n.FromError(err).Code != "redfish.bad_link" {
		t.Errorf("fremder Host: %v", err)
	}
}

// Basis-Adresse mit Pfad (Reverse Proxy) bzw. mit /redfish/v1 eingegeben.
func TestBasePath(t *testing.T) {
	fake := redfishtest.New("labdeck", "geheim-pw", redfishtest.ILO5())
	mux := http.NewServeMux()
	mux.Handle("/ilo/", http.StripPrefix("/ilo", fake))
	mux.Handle("/redfish/", fake)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	for _, base := range []string{srv.URL + "/ilo", srv.URL + "/ilo/", srv.URL + "/redfish/v1/"} {
		if err := New().Test(context.Background(), cfg(t, base, "geheim-pw", nil, time.Second)); err != nil {
			t.Errorf("%s: %v", base, err)
		}
	}
}
