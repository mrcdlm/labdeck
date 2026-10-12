package snmp

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/integrations/snmp/snmptest"
)

const community = "labdeck-ro"

func start(t *testing.T, p snmptest.Profile) *snmptest.Agent {
	t.Helper()
	a, err := snmptest.Start("127.0.0.1:0", community, p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	return a
}

func cfg(t *testing.T, addr string, extra map[string]any, secrets map[string]string) integrations.Config {
	t.Helper()
	host, port, _ := net.SplitHostPort(addr)
	p, _ := net.LookupPort("udp", port)
	in := map[string]any{"host": host, "port": float64(p)}
	for k, v := range extra {
		in[k] = v
	}
	values, msgs := New().Schema().Normalize(in, i18n.EN)
	if len(msgs) != 0 {
		t.Fatal(msgs)
	}
	if secrets == nil {
		secrets = map[string]string{"community": community}
	}
	return integrations.Config{Values: values, Secrets: secrets, Timeout: 1500 * time.Millisecond}
}

// clock lässt die Zeit zwischen zwei Abfragen gezielt vergehen.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newIntegration() (*Integration, *clock) {
	c := &clock{t: time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)}
	i := New()
	i.now = c.now
	return i, c
}

func collect(t *testing.T, i *Integration, c integrations.Config) integrations.Snapshot {
	t.Helper()
	snap, err := i.Collect(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func metrics(s integrations.Snapshot) map[string]integrations.Metric {
	m := map[string]integrations.Metric{}
	for _, x := range s.Metrics {
		m[x.Key] = x
	}
	return m
}

func resources(s integrations.Snapshot) map[string]integrations.Resource {
	m := map[string]integrations.Resource{}
	for _, r := range s.Resources {
		m[r.ID] = r
	}
	return m
}

func TestCollectC1300(t *testing.T) {
	a := start(t, snmptest.C1300())
	i, clk := newIntegration()
	c := cfg(t, a.Addr(), nil, nil)

	snap := collect(t, i, c)
	res := resources(snap)
	sw := res["switch"]
	if sw.Kind != integrations.KindSwitch || sw.Name != "core-switch" || sw.Info["model"] != "C1300-24P-4G" ||
		sw.Info["firmware"] != "4.1.6.53" || sw.Info["ports_total"] != "28" || sw.Uptime != 1234567 {
		t.Fatalf("Switch: %+v", sw)
	}
	if snap.Info["version"] != "C1300-24P-4G" {
		t.Errorf("Info: %v", snap.Info)
	}
	ports := 0
	for _, r := range snap.Resources {
		if r.Kind == integrations.KindPort {
			ports++
			if r.Parent != "core-switch" {
				t.Errorf("Parent: %+v", r)
			}
		}
	}
	if ports != 28 { // ohne VLAN, Loopback und LAG
		t.Fatalf("Ports: %d", ports)
	}
	// Frontbild aus der ENTITY-MIB: 1–24 RJ45, 25–28 SFP
	if p := res["port/gi1/0/24"]; p.Info["media"] != "rj45" || p.Info["unit"] != "1" || p.Info["pos"] != "24" {
		t.Errorf("gi1/0/24: %v", p.Info)
	}
	if p := res["port/gi1/0/25"]; p.Info["media"] != "sfp" || p.Info["pos"] != "25" {
		t.Errorf("gi1/0/25: %v", p.Info)
	}
	// Ports mit Beschreibung werden überwacht: der Drucker ist down
	m := metrics(snap)
	if l := m["port/gi1/0/12/link"]; l.Status != integrations.StatusError || l.Value != 0 || l.Resource != "port/gi1/0/12" {
		t.Errorf("Drucker: %+v", l)
	}
	if l := m["port/gi1/0/1/link"]; l.Status != integrations.StatusOK || l.Value != 1 || l.Group != "gi1/0/1 · pve-01" {
		t.Errorf("pve-01: %+v", l)
	}
	if _, ok := m["port/gi1/0/15/link"]; ok {
		t.Error("Port ohne Beschreibung darf nicht überwacht sein")
	}
	if r := res["port/gi1/0/15"]; r.State != "down" || r.Status != integrations.StatusPaused {
		t.Errorf("freier Port: %+v", r)
	}
	if r := res["port/gi1/0/23"]; r.State != "disabled" {
		t.Errorf("abgeschalteter Port: %+v", r)
	}
	if sw.Status != integrations.StatusError || m["ports_up"].Value != 14 {
		t.Errorf("Status %s, up %v", sw.Status, m["ports_up"].Value)
	}
	// erster Poll: noch keine Raten
	if _, ok := m["port/gi1/0/1/in"]; ok {
		t.Error("Rate ohne Vorwert")
	}

	// 10 s später: 12,5 MB rein = 10 Mbit/s; 1,2 GB raus = 960 Mbit/s (96 % → Warnung)
	a.AddTraffic("gi1/0/1", 12_500_000, 0)
	a.AddTraffic("gi1/0/2", 0, 1_200_000_000)
	a.AddErrors("gi1/0/3", 4)
	clk.t = clk.t.Add(10 * time.Second)
	snap = collect(t, i, c)
	m, res = metrics(snap), resources(snap)
	if in := m["port/gi1/0/1/in"]; in.Value != 10e6 || in.Unit != "bit/s" || in.Status != integrations.StatusOK {
		t.Errorf("in: %+v", in)
	}
	if r := res["port/gi1/0/1"]; *r.NetIn != 1_250_000 || !r.NetRate || r.Info["util"] != "1" {
		t.Errorf("Port-Ressource: %+v", r)
	}
	if out := m["port/gi1/0/2/out"]; out.Value != 960e6 || out.Status != integrations.StatusWarning {
		t.Errorf("out: %+v", out)
	}
	if e := m["port/gi1/0/3/errors"]; e.Value != 4 || e.Status != integrations.StatusWarning {
		t.Errorf("Fehler: %+v", e)
	}
	if m["traffic_in"].Value != 10e6 || m["traffic_out"].Value != 960e6 {
		t.Errorf("Summen: %v %v", m["traffic_in"].Value, m["traffic_out"].Value)
	}

	// Zähler zurückgesetzt (Neustart des Switches): keine negative Rate
	a.SetCounters("gi1/0/1", 0, 0)
	clk.t = clk.t.Add(10 * time.Second)
	m = metrics(collect(t, i, c))
	if _, ok := m["port/gi1/0/1/in"]; ok {
		t.Error("Rate nach Zähler-Reset")
	}
	if e := m["port/gi1/0/3/errors"]; e.Value != 0 || e.Status != integrations.StatusOK {
		t.Errorf("Fehler ohne neue: %+v", e)
	}
}

func TestLAGs(t *testing.T) {
	a := start(t, snmptest.C1300())
	i, _ := newIntegration()
	res := resources(collect(t, i, cfg(t, a.Addr(), map[string]any{"lags": true}, nil)))
	if p := res["port/Po1"]; p.Info["media"] != "lag" || p.State != "up" {
		t.Errorf("LAG: %+v", p)
	}
	if res["switch"].Info["ports_total"] != "28" {
		t.Errorf("LAGs zählen nicht als Port: %v", res["switch"].Info)
	}
}

func TestGenericWithoutEntity(t *testing.T) {
	a := start(t, snmptest.Generic8())
	i, _ := newIntegration()
	snap := collect(t, i, cfg(t, a.Addr(), map[string]any{"monitor": "list", "monitor_ports": "1-3, 7"}, nil))
	res, m := resources(snap), metrics(snap)
	if p := res["port/Port 8"]; p.Info["media"] != "rj45" || p.Info["pos"] != "8" || p.Info["unit"] != "1" {
		t.Errorf("Port 8: %v", p.Info)
	}
	if _, ok := m["port/Port 2/link"]; !ok {
		t.Error("Port 2 sollte überwacht sein")
	}
	if l := m["port/Port 7/link"]; l.Status != integrations.StatusError {
		t.Errorf("Port 7: %+v", l)
	}
	if _, ok := m["port/Port 5/link"]; ok {
		t.Error("Port 5 ist nicht ausgewählt")
	}
	if snap.Info["version"] != "Smart Switch GS308E" {
		t.Errorf("Info: %v", snap.Info)
	}

	// Frontlayout überschreibt die Erkennung
	res = resources(collect(t, i, cfg(t, a.Addr(), map[string]any{"layout": "6 rj45, 2 sfp", "monitor": "none"}, nil)))
	if res["port/Port 6"].Info["media"] != "rj45" || res["port/Port 7"].Info["media"] != "sfp" {
		t.Errorf("Layout: %v %v", res["port/Port 6"].Info, res["port/Port 7"].Info)
	}
}

func TestErrors(t *testing.T) {
	a := start(t, snmptest.C1300())
	ctx := context.Background()
	code := func(c integrations.Config) string { return i18n.FromError(New().Test(ctx, c)).Code }

	if err := New().Test(ctx, cfg(t, a.Addr(), nil, nil)); err != nil {
		t.Fatalf("Test: %v", err)
	}
	// falsche Community: das Gerät schweigt
	if c := code(cfg(t, a.Addr(), nil, map[string]string{"community": "public"})); c != "snmp.timeout" {
		t.Errorf("Community: %s", c)
	}
	if c := code(cfg(t, a.Addr(), nil, map[string]string{})); c != "integration.field_required" {
		t.Errorf("ohne Community: %s", c)
	}
	a.Silent(true)
	if c := code(cfg(t, a.Addr(), nil, nil)); c != "snmp.timeout" {
		t.Errorf("Timeout: %s", c)
	}
	// Fehlermeldungen nennen keine Zugangsdaten
	err := New().Test(ctx, cfg(t, a.Addr(), nil, map[string]string{"community": "geheim-123"}))
	if err == nil || strings.Contains(err.Error(), "geheim-123") {
		t.Errorf("Fehler: %v", err)
	}
}

func TestUSM(t *testing.T) {
	c := func(extra map[string]any, secrets map[string]string) integrations.Config {
		in := map[string]any{"host": "sw", "version": "v3", "user": "labdeck"}
		for k, v := range extra {
			in[k] = v
		}
		values, msgs := New().Schema().Normalize(in, i18n.EN)
		if len(msgs) != 0 {
			t.Fatal(msgs)
		}
		return integrations.Config{Values: values, Secrets: secrets}
	}
	sp, flags, err := usm(c(map[string]any{"auth_protocol": "sha256", "priv_protocol": "aes256c"},
		map[string]string{"auth_password": "authpass1", "priv_password": "privpass1"}))
	if err != nil || flags != 3 || sp.AuthenticationProtocol != authProtocols["sha256"] ||
		sp.PrivacyProtocol != privProtocols["aes256c"] || sp.UserName != "labdeck" {
		t.Fatalf("authPriv: %+v %v %v", sp, flags, err)
	}
	_, flags, err = usm(c(map[string]any{"level": "authNoPriv"}, map[string]string{"auth_password": "authpass1"}))
	if err != nil || flags != 1 {
		t.Errorf("authNoPriv: %v %v", flags, err)
	}
	_, _, err = usm(c(nil, map[string]string{"auth_password": "kurz", "priv_password": "privpass1"}))
	if i18n.FromError(err).Code != "snmp.passphrase_short" {
		t.Errorf("kurzes Passwort: %v", err)
	}
}

func TestPortList(t *testing.T) {
	l := parsePortList("gi1/0/1-3, GigabitEthernet1/0/24; 5 10-11 Po1")
	for _, name := range []string{"gi1/0/1", "gi1/0/3", "gi1/0/24", "po1", "port-channel1"} {
		if !l.names[shortName(name)] {
			t.Errorf("%s fehlt", name)
		}
	}
	if l.names[shortName("gi1/0/4")] || !l.nums[5] || !l.nums[11] || l.nums[12] {
		t.Errorf("%+v", l)
	}
	if shortName("Port 12") == shortName("Po12") {
		t.Error("Port 12 und Po12 verwechselt")
	}
}

func TestParseLayout(t *testing.T) {
	g := parseLayout("24 RJ45, 4x SFP+")
	if len(g) != 2 || g[0] != (layoutGroup{24, "rj45"}) || g[1] != (layoutGroup{4, "sfp"}) {
		t.Errorf("%+v", g)
	}
	if parseLayout("24 Ports") != nil || parseLayout("") != nil {
		t.Error("ungültig muss nil sein")
	}
}

func TestFrontFromNames(t *testing.T) {
	d := &device{descr: "CBS350-8T-E-2G"}
	var ports []*port
	for _, n := range []string{"gi1/0/1", "gi1/0/2", "te1/0/1", "GigabitEthernet2/0/1", "Po1"} {
		ports = append(ports, &port{name: n})
	}
	front(ports, d, "")
	want := []struct {
		media     string
		unit, pos int
	}{{"rj45", 1, 1}, {"rj45", 1, 2}, {"sfp", 1, 1}, {"rj45", 2, 1}, {"lag", 1, 1}}
	for n, w := range want {
		if p := ports[n]; p.media != w.media || p.unit != w.unit || p.pos != w.pos {
			t.Errorf("%s: %s %d/%d", p.name, p.media, p.unit, p.pos)
		}
	}

	// Modellname „…-24P-4G“: ohne ENTITY-MIB sind die letzten vier Ports SFP
	d = &device{descr: "C1300-24P-4G 24-Port Gigabit PoE+ Managed Switch"}
	ports = nil
	for n := 1; n <= 28; n++ {
		ports = append(ports, &port{name: "gi1/0/" + string(rune('0'+n/10)) + string(rune('0'+n%10))})
	}
	front(ports, d, "")
	if ports[23].media != "rj45" || ports[24].media != "sfp" || ports[27].media != "sfp" {
		t.Errorf("Uplinks: %s %s %s", ports[23].media, ports[24].media, ports[27].media)
	}
}
