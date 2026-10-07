package nut

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/integrations/nut/nuttest"
)

func start(t *testing.T, ups ...nuttest.UPS) *nuttest.Server {
	t.Helper()
	if ups == nil {
		ups = []nuttest.UPS{nuttest.Demo()}
	}
	s, err := nuttest.Start("127.0.0.1:0", ups...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func cfg(t *testing.T, addr string, extra map[string]any, pw string, timeout time.Duration) integrations.Config {
	t.Helper()
	host, port, _ := net.SplitHostPort(addr)
	p, _ := net.LookupPort("tcp", port)
	in := map[string]any{"host": host, "port": float64(p)}
	for k, v := range extra {
		in[k] = v
	}
	values, msgs := New().Schema().Normalize(in, i18n.EN)
	if len(msgs) != 0 {
		t.Fatal(msgs)
	}
	return integrations.Config{Values: values, Secrets: map[string]string{"password": pw}, Timeout: timeout}
}

func metrics(s integrations.Snapshot) map[string]integrations.Metric {
	m := map[string]integrations.Metric{}
	for _, x := range s.Metrics {
		m[x.Key] = x
	}
	return m
}

func TestCollect(t *testing.T) {
	srv := start(t)
	ctx := context.Background()
	snap, err := New().Collect(ctx, cfg(t, srv.Addr(), nil, "", time.Second))
	if err != nil {
		t.Fatal(err)
	}
	m := metrics(snap)
	if m["power"].Value != 1 || m["power"].Status != integrations.StatusOK || m["charge"].Value != 97 ||
		m["runtime"].Value != 1830 || m["runtime"].Unit != "s" || m["load"].Value != 31 || m["input_voltage"].Unit != "V" {
		t.Fatalf("%+v", m)
	}
	if snap.Info["version"] != "EATON 5SC 1000" || snap.Info["status"] != "OL CHRG" {
		t.Errorf("Info: %v", snap.Info)
	}

	// Stromausfall: Batteriebetrieb = Warnung, wenig Restlaufzeit = Fehler
	srv.Set("ups", "ups.status", "OB DISCHRG")
	srv.Set("ups", "battery.runtime", "420")
	m = metrics(collect(t, cfg(t, srv.Addr(), nil, "", time.Second)))
	if m["power"].Value != 0 || m["power"].Status != integrations.StatusWarning || m["runtime"].Status != integrations.StatusError {
		t.Errorf("Batterie: %+v %+v", m["power"], m["runtime"])
	}
	srv.Set("ups", "ups.status", "OB LB")
	m = metrics(collect(t, cfg(t, srv.Addr(), nil, "", time.Second)))
	if m["power"].Status != integrations.StatusError {
		t.Errorf("Batterie leer: %+v", m["power"])
	}
}

func collect(t *testing.T, c integrations.Config) integrations.Snapshot {
	t.Helper()
	s, err := New().Collect(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSeveralUPS(t *testing.T) {
	second := nuttest.Demo()
	second.Name, second.Desc = "rack", "APC Smart-UPS"
	second.Vars = map[string]string{"ups.status": "OL", "battery.charge": "100", "ups.load": "88"}
	srv := start(t, nuttest.Demo(), second)
	m := metrics(collect(t, cfg(t, srv.Addr(), nil, "", time.Second)))
	if l := m["ups/rack/load"]; l.Group != "APC Smart-UPS" || l.Status != integrations.StatusWarning {
		t.Errorf("gruppiert: %+v", l)
	}
	// nur eine USV ausgewählt
	m = metrics(collect(t, cfg(t, srv.Addr(), map[string]any{"ups": "rack"}, "", time.Second)))
	if _, ok := m["load"]; !ok || m["load"].Value != 88 {
		t.Errorf("Auswahl: %+v", m)
	}
}

func TestErrors(t *testing.T) {
	srv := start(t)
	ctx := context.Background()
	code := func(c integrations.Config) string { return i18n.FromError(New().Test(ctx, c)).Code }

	srv.RequireLogin("monuser", "geheim")
	if c := code(cfg(t, srv.Addr(), nil, "", time.Second)); c != "nut.access_denied" {
		t.Errorf("ohne Anmeldung: %s", c)
	}
	if c := code(cfg(t, srv.Addr(), map[string]any{"username": "monuser"}, "falsch", time.Second)); c != "nut.access_denied" {
		t.Errorf("falsches Passwort: %s", c)
	}
	if c := code(cfg(t, srv.Addr(), map[string]any{"username": "monuser"}, "geheim", time.Second)); c != "" {
		t.Errorf("Anmeldung: %s", c)
	}
	srv.RequireLogin("", "")

	if c := code(cfg(t, srv.Addr(), map[string]any{"ups": "gibtsnicht"}, "", time.Second)); c != "nut.unknown_ups" {
		t.Errorf("unbekannt: %s", c)
	}
	srv.Stale(true)
	if c := code(cfg(t, srv.Addr(), nil, "", time.Second)); c != "nut.driver" {
		t.Errorf("Treiber: %s", c)
	}
	srv.Stale(false)
	srv.Broken(true)
	if c := code(cfg(t, srv.Addr(), nil, "", time.Second)); c != "nut.protocol" {
		t.Errorf("kaputt: %s", c)
	}
	srv.Broken(false)
	srv.Delay(300 * time.Millisecond)
	if c := code(cfg(t, srv.Addr(), nil, "", 100*time.Millisecond)); c != "integration.timeout" {
		t.Errorf("Timeout: %s", c)
	}
	srv.Delay(0)

	// Port zu → Verbindung abgelehnt
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	closed := ln.Addr().String()
	ln.Close()
	if c := code(cfg(t, closed, nil, "", time.Second)); c != "integration.refused" {
		t.Errorf("abgelehnt: %s", c)
	}
	// kein upsd (HTTP-Server o. Ä.) → Protokollfehler
	other, _ := net.Listen("tcp", "127.0.0.1:0")
	defer other.Close()
	go func() {
		c, err := other.Accept()
		if err == nil {
			c.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
			c.Close()
		}
	}()
	if c := code(cfg(t, other.Addr().String(), nil, "", time.Second)); c != "nut.protocol" {
		t.Errorf("fremder Dienst: %s", c)
	}
}

func TestFields(t *testing.T) {
	got, err := fields(`VAR ups ups.model "Smart \"UPS\" 1500 \\ X"`)
	if err != nil || len(got) != 4 || got[3] != `Smart "UPS" 1500 \ X` {
		t.Fatalf("%q %v", got, err)
	}
	if got, _ := fields(`UPS ups ""`); len(got) != 3 || got[2] != "" {
		t.Fatalf("leer: %q", got)
	}
	if _, err := fields(`VAR ups x "offen`); err == nil {
		t.Fatal("offenes Anführungszeichen")
	}
}
