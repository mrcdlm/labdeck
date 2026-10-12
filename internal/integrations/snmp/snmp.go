// Package snmp überwacht Switches über SNMP (v2c oder v3) mit den Standard-
// MIBs IF-MIB, ENTITY-MIB und SNMPv2-MIB: Link, Geschwindigkeit, Traffic und
// Fehler je Port sowie das Frontbild (RJ45/SFP, Einheit, Position).
//
// Herstellerunabhängig: Der Cisco C1300 ist das Referenzgerät, jeder Switch
// mit IF-MIB funktioniert ohne Anpassung. Wo die Erkennung des Frontbilds
// nicht passt, lässt sie sich im Formular überschreiben (Frontlayout).
package snmp

import (
	"cmp"
	"context"
	"math"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mrcdlm/labdeck/internal/integrations"
)

const Type = "snmp"

const (
	defaultPort     = 161
	defaultUtilWarn = 80.0
	// Frontbild (ENTITY-MIB) ändert sich selten – nicht bei jedem Poll lesen
	entityTTL = 30 * time.Minute
)

// Integration merkt sich je Switch die letzten Zählerstände (für Raten) und
// das Frontbild aus der ENTITY-MIB.
type Integration struct {
	mu      sync.Mutex
	devices map[string]*devState
	now     func() time.Time
}

type devState struct {
	counters map[string]counters // ifIndex →
	at       time.Time
	ent      table // nil = keine ENTITY-MIB
	alias    map[string]string
	entAt    time.Time
}

type counters struct{ in, out, errs uint64 }

func New() *Integration { return &Integration{devices: map[string]*devState{}, now: time.Now} }

func (*Integration) Type() string { return Type }

func (*Integration) Schema() integrations.ConfigSchema {
	type T = integrations.Text
	type F = integrations.Field
	v3 := &integrations.Condition{Key: "version", Values: []string{"v3"}}
	opt := func(v, de, en string) integrations.Option {
		return integrations.Option{Value: v, Label: T{DE: de, EN: en}}
	}
	same := func(v, label string) integrations.Option { return opt(v, label, label) }
	return integrations.ConfigSchema{
		Title: T{DE: "Switch (SNMP)", EN: "Switch (SNMP)"},
		Description: T{
			DE: "Ports, Link, Traffic und Fehler eines Switches über SNMP – z. B. Cisco C1300, UniFi, MikroTik, TP-Link, Netgear.",
			EN: "Ports, link, traffic and errors of a switch via SNMP – e.g. Cisco C1300, UniFi, MikroTik, TP-Link, Netgear.",
		},
		Icon:  "network",
		Docs:  "https://www.rfc-editor.org/rfc/rfc2863",
		NoTLS: true,
		Fields: []F{
			{Key: "host", Type: integrations.FieldString, Required: true,
				Label: T{DE: "Host", EN: "Host"},
				Help: T{
					DE: "IP-Adresse oder Name des Switches. Auf dem Cisco C1300: Administration → SNMP aktivieren (Security → SNMP), Community bzw. Benutzer anlegen und die IP von labdeck als Management-Station zulassen.",
					EN: "IP address or name of the switch. On the Cisco C1300: enable SNMP (Security → SNMP), create a community or user and allow labdeck's IP as management station.",
				},
				Placeholder: "192.168.1.2"},
			{Key: "port", Type: integrations.FieldNumber, Default: float64(defaultPort), Half: true,
				Min: integrations.Ptr(1), Max: integrations.Ptr(65535),
				Label: T{DE: "Port (UDP)", EN: "Port (UDP)"}},
			{Key: "version", Type: integrations.FieldSelect, Default: "v2c", Half: true,
				Label:   T{DE: "SNMP-Version", EN: "SNMP version"},
				Options: []integrations.Option{same("v2c", "v2c"), opt("v3", "v3 (verschlüsselt)", "v3 (encrypted)")}},
			{Key: "community", Type: integrations.FieldSecret,
				ShowIf: &integrations.Condition{Key: "version", Values: []string{"v2c"}},
				Label:  T{DE: "Community", EN: "Community"},
				Help: T{
					DE: "Nur Lesezugriff (read-only) genügt. Nicht „public“ verwenden.",
					EN: "Read-only access is enough. Don't use \"public\".",
				}},
			{Key: "user", Type: integrations.FieldString, ShowIf: v3, Half: true,
				Label: T{DE: "Benutzer", EN: "User"},
				Help: T{
					DE: "v3-Benutzer in einer Gruppe mit Lese-View.",
					EN: "v3 user in a group with a read view.",
				}},
			{Key: "level", Type: integrations.FieldSelect, Default: "authPriv", ShowIf: v3, Half: true,
				Label: T{DE: "Sicherheitsstufe", EN: "Security level"},
				Options: []integrations.Option{
					opt("authPriv", "Anmeldung + Verschlüsselung", "Authentication + encryption"),
					opt("authNoPriv", "Nur Anmeldung", "Authentication only"),
					opt("noAuthNoPriv", "Ohne (nicht empfohlen)", "None (not recommended)"),
				}},
			{Key: "auth_protocol", Type: integrations.FieldSelect, Default: "sha", ShowIf: v3, Half: true,
				Label: T{DE: "Auth-Protokoll", EN: "Auth protocol"},
				Options: []integrations.Option{same("sha", "SHA-1"), same("sha224", "SHA-224"), same("sha256", "SHA-256"),
					same("sha384", "SHA-384"), same("sha512", "SHA-512"), same("md5", "MD5")}},
			{Key: "auth_password", Type: integrations.FieldSecret, ShowIf: v3, Half: true,
				Label: T{DE: "Auth-Passwort", EN: "Auth password"},
				Help:  T{DE: "Mindestens 8 Zeichen.", EN: "At least 8 characters."}},
			{Key: "priv_protocol", Type: integrations.FieldSelect, Default: "aes", ShowIf: v3, Half: true,
				Label: T{DE: "Priv-Protokoll", EN: "Priv protocol"},
				Options: []integrations.Option{same("aes", "AES-128"), opt("aes256c", "AES-256 (Cisco)", "AES-256 (Cisco)"),
					opt("aes256", "AES-256 (Blumenthal)", "AES-256 (Blumenthal)"), opt("aes192c", "AES-192 (Cisco)", "AES-192 (Cisco)"),
					opt("aes192", "AES-192 (Blumenthal)", "AES-192 (Blumenthal)"), same("des", "DES")}},
			{Key: "priv_password", Type: integrations.FieldSecret, ShowIf: v3, Half: true,
				Label: T{DE: "Priv-Passwort", EN: "Priv password"},
				Help:  T{DE: "Mindestens 8 Zeichen.", EN: "At least 8 characters."}},
			{Key: "monitor", Type: integrations.FieldSelect, Default: "described",
				Label: T{DE: "Ports überwachen", EN: "Monitor ports"},
				Help: T{
					DE: "Ein überwachter Port, der down ist, gilt als Problem. Andere Ports dürfen frei kommen und gehen.",
					EN: "A monitored port that is down counts as a problem. Other ports may come and go freely.",
				},
				Options: []integrations.Option{
					opt("described", "Ports mit Beschreibung", "Ports with a description"),
					opt("list", "Ausgewählte Ports", "Selected ports"),
					opt("none", "Keine", "None"),
				}},
			{Key: "monitor_ports", Type: integrations.FieldString,
				ShowIf: &integrations.Condition{Key: "monitor", Values: []string{"list"}},
				Label:  T{DE: "Überwachte Ports", EN: "Monitored ports"},
				Help: T{
					DE: "Portnamen oder Nummern, Bereiche mit Bindestrich, z. B. gi1/0/1-8, gi1/0/24 oder 1-4, 24.",
					EN: "Port names or numbers, ranges with a hyphen, e.g. gi1/0/1-8, gi1/0/24 or 1-4, 24.",
				},
				Placeholder: "gi1/0/1-8, gi1/0/24"},
			{Key: "util_warn", Type: integrations.FieldNumber, Default: defaultUtilWarn, Half: true,
				Min: integrations.Ptr(1), Max: integrations.Ptr(100),
				Label: T{DE: "Warnung ab Auslastung (%)", EN: "Warning from utilisation (%)"}},
			{Key: "lags", Type: integrations.FieldBool, Default: false, Half: true,
				Label: T{DE: "LAGs anzeigen", EN: "Show LAGs"},
				Help:  T{DE: "Port-Channels bzw. Link-Aggregationen.", EN: "Port channels or link aggregations."}},
			{Key: "layout", Type: integrations.FieldString,
				Label: T{DE: "Frontlayout (optional)", EN: "Front layout (optional)"},
				Help: T{
					DE: "Nur nötig, wenn die Ansicht nicht zum Gerät passt: Anzahl und Art der Ports von links, z. B. „24 rj45, 4 sfp“.",
					EN: "Only needed if the view doesn't match the device: number and kind of ports from the left, e.g. \"24 rj45, 4 sfp\".",
				},
				Placeholder: "24 rj45, 4 sfp"},
		},
	}
}

func key(cfg integrations.Config) string {
	port := defaultPort
	if cfg.Has("port") {
		port = cfg.Int("port")
	}
	return net.JoinHostPort(strings.TrimSpace(cfg.String("host")), strconv.Itoa(port))
}

func (i *Integration) Test(ctx context.Context, cfg integrations.Config) error {
	g, err := connect(ctx, cfg)
	if err != nil {
		return err
	}
	defer g.Conn.Close()
	return fetchSystem(ctx, g, &device{})
}

// fetch liest System, Interfaces und (zwischengespeichert) die ENTITY-MIB.
func (i *Integration) fetch(ctx context.Context, cfg integrations.Config) (*device, *devState, error) {
	g, err := connect(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	defer g.Conn.Close()
	d := &device{}
	if err := fetchSystem(ctx, g, d); err != nil {
		return nil, nil, err
	}
	if d.ifs, err = walk(ctx, g, ifColumns); err != nil {
		return nil, nil, err
	}
	d.at = i.now()

	i.mu.Lock()
	st := i.devices[key(cfg)]
	if st == nil {
		st = &devState{}
		i.devices[key(cfg)] = st
	}
	fresh := !st.entAt.IsZero() && d.at.Sub(st.entAt) < entityTTL
	i.mu.Unlock()
	if !fresh {
		ent, err := walk(ctx, g, entColumns)
		if err != nil {
			return nil, nil, err
		}
		var alias map[string]string
		if len(ent[colEntClass]) == 0 {
			ent = nil // Switch ohne ENTITY-MIB: Frontbild aus den Namen
		} else if alias, err = entAliases(ctx, g); err != nil {
			return nil, nil, err
		}
		i.mu.Lock()
		st.ent, st.alias, st.entAt = ent, alias, d.at
		i.mu.Unlock()
	}
	i.mu.Lock()
	d.ent, d.alias = st.ent, st.alias
	i.mu.Unlock()
	return d, st, nil
}

// Werte von ifAdminStatus/ifOperStatus und ifType (IF-MIB, IANAifType-MIB).
const (
	ifUp         = 1
	ifNotPresent = 6
	ifTypeEther  = 6
	ifTypeGigE   = 117
	ifTypeLAG    = 161
)

func (i *Integration) Collect(ctx context.Context, cfg integrations.Config) (integrations.Snapshot, error) {
	d, st, err := i.fetch(ctx, cfg)
	if err != nil {
		return integrations.Snapshot{}, err
	}
	ports := buildPorts(d, cfg.Bool("lags"))
	front(ports, d, cfg.String("layout"))
	monitored := monitorSet(ports, cfg)

	// Raten aus der Differenz zur letzten Abfrage
	i.mu.Lock()
	prev, prevAt := st.counters, st.at
	st.counters, st.at = map[string]counters{}, d.at
	for _, p := range ports {
		if p.hasCounters {
			st.counters[p.ifIndex] = counters{p.inOctets, p.outOctets, p.errs}
		}
	}
	i.mu.Unlock()
	dt := d.at.Sub(prevAt).Seconds()
	for _, p := range ports {
		c, ok := prev[p.ifIndex]
		if !ok || !p.hasCounters || dt <= 0 || p.inOctets < c.in || p.outOctets < c.out {
			continue // erster Poll oder Zähler zurückgesetzt (Neustart)
		}
		in, out := float64(p.inOctets-c.in)/dt, float64(p.outOctets-c.out)/dt
		p.inRate, p.outRate = &in, &out
		if p.errs >= c.errs {
			e := p.errs - c.errs
			p.newErrs = &e
		}
	}

	utilWarn := defaultUtilWarn
	if cfg.Has("util_warn") {
		utilWarn = cfg.Float("util_warn")
	}
	return snapshot(d, ports, monitored, utilWarn, cfg.String("host")), nil
}

// snapshot baut Ressourcen (Switch, Ports) und Kennzahlen.
func snapshot(d *device, ports []*port, monitored map[*port]bool, utilWarn float64, host string) integrations.Snapshot {
	ok := integrations.StatusOK
	name := cmp.Or(d.name, host)
	var snap integrations.Snapshot
	var statuses []integrations.Status
	up, total := 0, 0
	var sumIn, sumOut float64
	haveRates := false
	for _, p := range ports {
		res := integrations.Resource{ID: "port/" + p.name, Name: p.name, Kind: integrations.KindPort, Parent: name,
			Info: map[string]string{"media": p.media, "unit": strconv.Itoa(p.unit), "pos": strconv.Itoa(p.pos),
				"if_index": p.ifIndex}}
		if p.alias != "" {
			res.Info["alias"] = p.alias
		}
		if p.speed > 0 {
			res.Info["speed"] = strconv.FormatUint(p.speed, 10)
		}
		if monitored[p] {
			res.Info["monitored"] = "1"
		}
		lag := p.media == mediaLAG
		if !lag {
			total++
		}
		switch {
		case p.admin != ifUp:
			res.State, res.Status = "disabled", integrations.StatusPaused
		case p.oper == ifUp:
			res.State, res.Status = "up", ok
			if !lag {
				up++
			}
		case monitored[p]:
			res.State, res.Status = "down", integrations.StatusError
		default:
			res.State, res.Status = "down", integrations.StatusPaused
		}
		group := p.name
		if p.alias != "" {
			group += " · " + p.alias
		}
		if monitored[p] {
			v := 0.0
			if p.oper == ifUp {
				v = 1
			}
			snap.Metrics = append(snap.Metrics, integrations.Metric{Key: "port/" + p.name + "/link", Name: "link",
				Group: group, Value: v, Unit: "okfail", Status: res.Status, Resource: res.ID})
		}
		if p.oper == ifUp && p.inRate != nil {
			in, out := int64(math.Round(*p.inRate)), int64(math.Round(*p.outRate))
			res.NetIn, res.NetOut, res.NetRate = &in, &out, true
			st := ok
			if p.speed > 0 {
				util := math.Max(*p.inRate, *p.outRate) * 8 / (float64(p.speed) * 1e6) * 100
				res.Info["util"] = strconv.FormatFloat(math.Round(util*10)/10, 'f', -1, 64)
				if util >= utilWarn {
					st = integrations.StatusWarning
				}
			}
			snap.Metrics = append(snap.Metrics,
				integrations.Metric{Key: "port/" + p.name + "/in", Name: "in", Group: group, Value: *p.inRate * 8,
					Unit: "bit/s", Status: st, Resource: res.ID},
				integrations.Metric{Key: "port/" + p.name + "/out", Name: "out", Group: group, Value: *p.outRate * 8,
					Unit: "bit/s", Status: st, Resource: res.ID})
			if p.newErrs != nil {
				est := ok
				if *p.newErrs > 0 {
					est = integrations.StatusWarning
				}
				res.Info["errors"] = strconv.FormatUint(*p.newErrs, 10)
				snap.Metrics = append(snap.Metrics, integrations.Metric{Key: "port/" + p.name + "/errors", Name: "errors",
					Group: group, Value: float64(*p.newErrs), Status: est, Resource: res.ID})
				st = integrations.Worst(st, est)
			}
			if res.Status == ok {
				res.Status = st
			}
			if !lag {
				sumIn += *p.inRate * 8
				sumOut += *p.outRate * 8
				haveRates = true
			}
		}
		statuses = append(statuses, res.Status)
		snap.Resources = append(snap.Resources, res)
	}

	sw := integrations.Resource{ID: "switch", Name: name, Kind: integrations.KindSwitch, State: "up",
		Status: integrations.Worst(append(statuses, ok)...), Uptime: int64(d.uptime.Seconds()),
		Info: map[string]string{"ports_total": strconv.Itoa(total), "ports_up": strconv.Itoa(up)}}
	model, firmware, serial := chassisInfo(d)
	for k, v := range map[string]string{"model": model, "firmware": firmware, "serial": serial, "descr": d.descr} {
		if v != "" {
			sw.Info[k] = v
		}
	}
	snap.Resources = append([]integrations.Resource{sw}, snap.Resources...)

	main := []integrations.Metric{{Key: "ports_up", Name: "ports_up", Value: float64(up), Status: ok}}
	if haveRates {
		main = append(main,
			integrations.Metric{Key: "traffic_in", Name: "traffic_in", Value: sumIn, Unit: "bit/s", Status: ok},
			integrations.Metric{Key: "traffic_out", Name: "traffic_out", Value: sumOut, Unit: "bit/s", Status: ok})
	}
	snap.Metrics = append(main, snap.Metrics...)
	snap.Info = map[string]string{"ports_total": strconv.Itoa(total)}
	if v := cmp.Or(model, shorten(d.descr, 60)); v != "" {
		snap.Info["version"] = v
	}
	if firmware != "" {
		snap.Info["firmware"] = firmware
	}
	return snap
}

// chassisInfo: Modell, Firmware und Seriennummer aus der ENTITY-MIB
// (erstes Gehäuse, entPhysicalClass chassis(3)).
func chassisInfo(d *device) (model, firmware, serial string) {
	if d.ent == nil {
		return "", "", ""
	}
	idx := slices.SortedFunc(func(yield func(string) bool) {
		for k := range d.ent[colEntClass] {
			if !yield(k) {
				return
			}
		}
	}, compareIndex)
	for _, k := range idx {
		if c, _ := d.ent.num(colEntClass, k); c == entChassis {
			return strings.TrimSpace(d.ent.str(colEntModel, k)), strings.TrimSpace(d.ent.str(colEntSoftware, k)),
				strings.TrimSpace(d.ent.str(colEntSerial, k))
		}
	}
	return "", "", ""
}

// compareIndex sortiert OID-Indizes numerisch („2“ vor „10“, „1.2“ vor „1.10“).
func compareIndex(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for n := 0; n < len(as) && n < len(bs); n++ {
		x, _ := strconv.Atoi(as[n])
		y, _ := strconv.Atoi(bs[n])
		if c := cmp.Compare(x, y); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(as), len(bs))
}

// shorten kürzt s auf höchstens n Zeichen (an einer Wortgrenze).
func shorten(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) <= n {
		return s
	}
	r := []rune(s)[:n]
	if i := strings.LastIndex(string(r), " "); i > 0 {
		return string(r)[:i] + " …"
	}
	return string(r) + "…"
}
