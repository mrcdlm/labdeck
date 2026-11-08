// Package nut überwacht USVs über Network UPS Tools (upsd): Netz- oder
// Batteriebetrieb, Ladung, Restlaufzeit, Last und Eingangsspannung.
//
// Protokoll (geprüft gegen die NUT-Entwicklerdoku „Network protocol“):
// zeilenbasiert über TCP 3493 – LIST UPS, LIST VAR <ups>, optional
// USERNAME/PASSWORD, Fehler als „ERR <CODE>“.
package nut

import (
	"context"
	"net"
	"slices"
	"strconv"
	"strings"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
)

const Type = "nut"

const (
	defaultPort        = 3493
	defaultRuntimeWarn = 10 // Minuten Restlaufzeit im Batteriebetrieb
)

type Integration struct{}

func New() *Integration { return &Integration{} }

func (*Integration) Type() string { return Type }

func (*Integration) Schema() integrations.ConfigSchema {
	type T = integrations.Text
	return integrations.ConfigSchema{
		Title: T{DE: "USV (NUT)", EN: "UPS (NUT)"},
		Description: T{
			DE: "Netz- oder Batteriebetrieb, Ladung, Restlaufzeit und Last deiner USV über Network UPS Tools.",
			EN: "Mains or battery operation, charge, runtime and load of your UPS via Network UPS Tools.",
		},
		Icon:  "battery-charging",
		Docs:  "https://networkupstools.org/docs/user-manual.chunked/",
		NoTLS: true,
		Fields: []integrations.Field{
			{Key: "host", Type: integrations.FieldString, Required: true, Target: true,
				Label: T{DE: "Host", EN: "Host"},
				Help: T{
					DE: "Rechner mit upsd, z. B. das NAS oder ein Raspberry Pi. upsd muss im Netz lauschen (LISTEN in upsd.conf).",
					EN: "Machine running upsd, e.g. the NAS or a Raspberry Pi. upsd has to listen on the network (LISTEN in upsd.conf).",
				},
				Placeholder: "nas.lan"},
			{Key: "port", Type: integrations.FieldNumber, Default: float64(defaultPort), Half: true, Target: true,
				Min: integrations.Ptr(1), Max: integrations.Ptr(65535),
				Label: T{DE: "Port", EN: "Port"}},
			{Key: "ups", Type: integrations.FieldString, Half: true,
				Label: T{DE: "USV-Name", EN: "UPS name"},
				Help:  T{DE: "Leer = alle USVs an diesem upsd.", EN: "Empty = all UPS on this upsd."}},
			{Key: "username", Type: integrations.FieldString, Half: true,
				Label: T{DE: "Benutzer (optional)", EN: "User (optional)"},
				Help:  T{DE: "Aus upsd.users, falls upsd eine Anmeldung verlangt.", EN: "From upsd.users if upsd requires a login."}},
			{Key: "password", Type: integrations.FieldSecret,
				Label: T{DE: "Passwort (optional)", EN: "Password (optional)"}},
			{Key: "runtime_warn", Type: integrations.FieldNumber, Default: float64(defaultRuntimeWarn),
				Min: integrations.Ptr(1), Max: integrations.Ptr(600),
				Label: T{DE: "Fehler bei Restlaufzeit unter (Minuten)", EN: "Error below runtime (minutes)"},
				Help:  T{DE: "Gilt im Batteriebetrieb.", EN: "Applies on battery."}},
		},
	}
}

func addr(cfg integrations.Config) string {
	port := defaultPort
	if cfg.Has("port") {
		port = cfg.Int("port")
	}
	return net.JoinHostPort(strings.TrimSpace(cfg.String("host")), strconv.Itoa(port))
}

type ups struct {
	name, desc string
	vars       map[string]string
}

func (*Integration) fetch(ctx context.Context, cfg integrations.Config) ([]ups, error) {
	if strings.TrimSpace(cfg.String("host")) == "" {
		return nil, i18n.E("integration.field_required", "field", "Host")
	}
	c, err := dial(ctx, addr(cfg), cfg.Timeout)
	if err != nil {
		return nil, err
	}
	defer c.close()
	if u := strings.TrimSpace(cfg.String("username")); u != "" {
		if err := c.login(u, cfg.Secret("password")); err != nil {
			return nil, err
		}
	}
	rows, err := c.list("UPS")
	if err != nil {
		return nil, err
	}
	want := strings.TrimSpace(cfg.String("ups"))
	var out []ups
	for _, r := range rows {
		if len(r) < 2 || r[0] != "UPS" || want != "" && r[1] != want {
			continue
		}
		u := ups{name: r[1], vars: map[string]string{}}
		if len(r) > 2 {
			u.desc = r[2]
		}
		vars, err := c.list("VAR " + u.name)
		if err != nil {
			return nil, err
		}
		for _, v := range vars {
			if len(v) >= 4 && v[0] == "VAR" {
				u.vars[v[2]] = v[3]
			}
		}
		out = append(out, u)
	}
	if len(out) == 0 {
		if want != "" {
			return nil, i18n.E("nut.unknown_ups")
		}
		return nil, i18n.E("nut.no_ups")
	}
	return out, nil
}

func (i *Integration) Test(ctx context.Context, cfg integrations.Config) error {
	_, err := i.fetch(ctx, cfg)
	return err
}

// powerStatus deutet ups.status (z. B. „OL CHRG“, „OB DISCHRG“, „OB LB“).
func powerStatus(raw string) (onMains bool, st integrations.Status) {
	flags := strings.Fields(raw)
	has := func(f string) bool { return slices.Contains(flags, f) }
	onMains = has("OL")
	switch {
	case has("LB"), has("FSD"), has("OVER"):
		return onMains, integrations.StatusError
	case has("OB"), has("RB"), has("BYPASS"), has("OFF"):
		return onMains, integrations.StatusWarning
	case onMains:
		return true, integrations.StatusOK
	}
	return onMains, integrations.StatusUnknown
}

func (i *Integration) Collect(ctx context.Context, cfg integrations.Config) (integrations.Snapshot, error) {
	list, err := i.fetch(ctx, cfg)
	if err != nil {
		return integrations.Snapshot{}, err
	}
	runtimeWarn := float64(defaultRuntimeWarn)
	if cfg.Has("runtime_warn") {
		runtimeWarn = cfg.Float("runtime_warn")
	}
	ok := integrations.StatusOK
	var snap integrations.Snapshot
	snap.Info = map[string]string{}
	single := len(list) == 1
	for _, u := range list {
		// Eine USV: Kennzahlen ohne Gruppe (Kachel), mehrere: je USV gruppiert
		key, group := func(k string) string { return k }, ""
		if !single {
			key = func(k string) string { return "ups/" + u.name + "/" + k }
			group = u.name
			if u.desc != "" {
				group = u.desc
			}
		}
		num := func(name string) (float64, bool) {
			v, err := strconv.ParseFloat(strings.TrimSpace(u.vars[name]), 64)
			return v, err == nil
		}
		onMains, st := powerStatus(u.vars["ups.status"])
		v := 0.0
		if onMains {
			v = 1
		}
		snap.Metrics = append(snap.Metrics, integrations.Metric{Key: key("power"), Name: "power", Group: group, Value: v, Unit: "mains", Status: st})
		if c, ok2 := num("battery.charge"); ok2 {
			snap.Metrics = append(snap.Metrics, integrations.Metric{Key: key("charge"), Name: "charge", Group: group,
				Value: c, Unit: "%", Status: ok})
		}
		if r, ok2 := num("battery.runtime"); ok2 {
			rst := ok
			if !onMains && r < runtimeWarn*60 {
				rst = integrations.StatusError
			}
			snap.Metrics = append(snap.Metrics, integrations.Metric{Key: key("runtime"), Name: "runtime", Group: group,
				Value: r, Unit: "s", Status: rst})
		}
		if l, ok2 := num("ups.load"); ok2 {
			warn, fail := 80.0, 95.0
			spec := integrations.ValueSpec{Warn: &warn, Fail: &fail}
			snap.Metrics = append(snap.Metrics, integrations.Metric{Key: key("load"), Name: "load", Group: group,
				Value: l, Unit: "%", Status: spec.Status(l)})
		}
		if in, ok2 := num("input.voltage"); ok2 {
			snap.Metrics = append(snap.Metrics, integrations.Metric{Key: key("input_voltage"), Name: "input_voltage",
				Group: group, Value: in, Unit: "V", Status: ok})
		}
		model := strings.TrimSpace(u.vars["ups.mfr"] + " " + u.vars["ups.model"])
		if single {
			snap.Info["version"] = model
			snap.Info["status"] = u.vars["ups.status"]
		} else {
			snap.Info["status:"+u.name] = u.vars["ups.status"]
		}
	}
	return snap, nil
}
