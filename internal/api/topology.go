package api

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/mrcdlm/labdeck/internal/events"
	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/store"
)

// Netzplan: Geräte, Kabel und VLANs des HomeLabs. Live aus den Integrationen
// (Switches mit Ports, LLDP-Nachbarn und VLANs per SNMP, Hosts aus Proxmox VE
// und Arcane mit ihren Gästen, Server-Hardware per Redfish, NetBird), dazu im
// UI gepflegt: eigene Geräte (Router, Access Point, PC …), Kabel, VLAN-Farben
// und Zuordnungen. Gespeichert wird nur das Gepflegte (JSON in den
// Einstellungen); alles andere entsteht bei jedem Abruf neu.

const keyTopology = "topology"

// Grenzen der Konfiguration.
const (
	maxTopoDevices = 200
	maxTopoLinks   = 500
	maxTopoVLANs   = 64
	maxTopoText    = 80
)

// Gerätetypen; die Ebene im Plan folgt aus dem Typ.
var topoTypes = []string{"internet", "router", "firewall", "switch", "ap", "server", "hypervisor", "docker", "nas", "pc", "iot", "other"}

func tierOf(typ string) int {
	switch typ {
	case "internet":
		return 0
	case "router", "firewall":
		return 1
	case "switch":
		return 2
	}
	return 3
}

var (
	topoIDRe  = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)
	colorRe   = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	aliasKeys = regexp.MustCompile(`^(lldp:[^\s]{1,80}|int:\d{1,12}|res:\d{1,12}:[^\s]{1,120})$`)
)

// --- gepflegte Konfiguration -----------------------------------------------

type topoEndpoint struct {
	Device string `json:"device"`
	Port   string `json:"port,omitempty"`
}

type topoManualDevice struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	Model string `json:"model,omitempty"`
	IP    string `json:"ip,omitempty"`
	Note  string `json:"note,omitempty"`
	// Ports: Anzahl Ports für die Front (Router, kleine Switches); 0 = keine
	Ports int `json:"ports,omitempty"`
}

type topoManualLink struct {
	ID     string       `json:"id"`
	A      topoEndpoint `json:"a"`
	B      topoEndpoint `json:"b"`
	Speed  int          `json:"speed,omitempty"` // Mbit/s
	VLAN   int          `json:"vlan,omitempty"`  // ohne Tag
	Tagged []int        `json:"tagged,omitempty"`
	Note   string       `json:"note,omitempty"`
}

type topoVLAN struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Subnet string `json:"subnet,omitempty"`
	Color  string `json:"color,omitempty"`
	Note   string `json:"note,omitempty"`
}

type topoConfig struct {
	Devices []topoManualDevice `json:"devices"`
	Links   []topoManualLink   `json:"links"`
	VLANs   []topoVLAN         `json:"vlans"`
	// Aliases ordnet zu: „res:<integration>:<ressource>“ → Gerät (zusammenführen),
	// „lldp:<name>“ → Gerät (Nachbar), „int:<integration>“ → Gerät (Dienst läuft dort)
	Aliases map[string]string `json:"aliases"`
	// Hidden: Geräte, die im Plan nicht erscheinen
	Hidden []string `json:"hidden"`
}

// --- berechneter Plan ------------------------------------------------------

type topoSource struct {
	IntegrationID   int64  `json:"integration_id"`
	IntegrationName string `json:"integration_name"`
	IntegrationType string `json:"integration_type"`
	ResourceID      string `json:"resource_id,omitempty"`
}

type topoFact struct {
	Key   string `json:"key"` // model, os, cpu, memory, ip, firmware
	Value string `json:"value"`
}

type topoPort struct {
	Name   string              `json:"name"`
	Status integrations.Status `json:"status"`
	State  string              `json:"state"`
	Speed  int                 `json:"speed,omitempty"`
	Media  string              `json:"media,omitempty"`
	Unit   int                 `json:"unit,omitempty"`
	Pos    int                 `json:"pos,omitempty"`
	Alias  string              `json:"alias,omitempty"`
	PVID   int                 `json:"pvid,omitempty"`
	Tagged []int               `json:"tagged,omitempty"`
	Util   string              `json:"util,omitempty"`
	Res    string              `json:"resource_id,omitempty"`
	// LLDP-Nachbar (lldp_name, lldp_port, lldp_chassis) – nur zum Zuordnen
	Extra map[string]string `json:"-"`
}

type topoGuest struct {
	ID     string              `json:"id"`
	Name   string              `json:"name"`
	Kind   string              `json:"kind"`
	State  string              `json:"state"`
	Status integrations.Status `json:"status"`
	Group  string              `json:"group,omitempty"` // Stack/Tag zum Gruppieren
}

type topoService struct {
	IntegrationID int64               `json:"integration_id"`
	Name          string              `json:"name"`
	Type          string              `json:"type"`
	Status        integrations.Status `json:"status"`
}

type topoDevice struct {
	ID       string              `json:"id"`
	Name     string              `json:"name"`
	Type     string              `json:"type"`
	Tier     int                 `json:"tier"`
	Status   integrations.Status `json:"status"`
	Manual   bool                `json:"manual"`
	Facts    []topoFact          `json:"facts"`
	Sources  []topoSource        `json:"sources"`
	Ports    []topoPort          `json:"ports"`
	Guests   []topoGuest         `json:"guests"`
	Services []topoService       `json:"services"`
	Note     string              `json:"note,omitempty"`
	// Names: weitere Namen zum Zuordnen (LLDP, Hostname)
	names []string
}

type topoLink struct {
	ID     string              `json:"id"`
	A      topoEndpoint        `json:"a"`
	B      topoEndpoint        `json:"b"`
	Speed  int                 `json:"speed,omitempty"`
	VLAN   int                 `json:"vlan,omitempty"`
	Tagged []int               `json:"tagged,omitempty"`
	Status integrations.Status `json:"status"`
	Source string              `json:"source"` // lldp, manual
	Note   string              `json:"note,omitempty"`
}

// topoSuggestion: LLDP-Nachbar ohne passendes Gerät.
type topoSuggestion struct {
	Switch  string `json:"switch"`
	Port    string `json:"port"`
	Name    string `json:"name"`
	Remote  string `json:"remote_port,omitempty"`
	Chassis string `json:"chassis,omitempty"`
}

type topoExposed struct {
	Name    string              `json:"name"`
	Targets string              `json:"targets,omitempty"`
	Status  integrations.Status `json:"status"`
	Source  string              `json:"source"`
}

type topoVPN struct {
	IntegrationID int64  `json:"integration_id"`
	Name          string `json:"name"`
	Online        int    `json:"online"`
	Total         int    `json:"total"`
	Problems      int    `json:"problems"`
}

type topologyDTO struct {
	Devices     []topoDevice     `json:"devices"`
	Links       []topoLink       `json:"links"`
	VLANs       []topoVLAN       `json:"vlans"`
	Services    []topoService    `json:"services"` // keinem Gerät zugeordnet
	Suggestions []topoSuggestion `json:"suggestions"`
	Exposed     []topoExposed    `json:"exposed"`
	VPN         []topoVPN        `json:"vpn"`
	Config      topoConfig       `json:"config"`
}

// normName vergleicht Gerätenamen tolerant: Kleinbuchstaben, ohne Domain,
// ohne Trennzeichen, Zahlen ohne führende Nullen („PVE-01.lan“ = „pve1“).
func normName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.IndexByte(s, '.'); i > 0 {
		s = s[:i]
	}
	var b strings.Builder
	zeros := true // am Anfang einer Zahl
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			if r == '0' && zeros {
				continue
			}
			zeros = false
			b.WriteRune(r)
		case r >= 'a' && r <= 'z':
			zeros = true
			b.WriteRune(r)
		default:
			zeros = true
		}
	}
	if b.Len() == 0 && s != "" {
		return "0"
	}
	return b.String()
}

func resKey(integrationID int64, resID string) string {
	return "res:" + strconv.FormatInt(integrationID, 10) + ":" + resID
}

var guestKinds = []string{integrations.KindVM, integrations.KindCT, integrations.KindContainer}

// buildTopology führt Integrationen und gepflegte Angaben zum Plan zusammen.
func buildTopology(list []integrations.View, cfg topoConfig) topologyDTO {
	out := topologyDTO{Config: cfg, Devices: []topoDevice{}, Links: []topoLink{}, Services: []topoService{},
		Suggestions: []topoSuggestion{}, Exposed: []topoExposed{}, VPN: []topoVPN{}}
	byID := map[string]*topoDevice{}
	var order []string
	add := func(d topoDevice) *topoDevice {
		order = append(order, d.ID)
		byID[d.ID] = &d
		return byID[d.ID]
	}
	alias := func(key string) string { return cfg.Aliases[key] }
	vlanNames := map[int]string{}

	// 1. Geräte aus Integrationen (Switch, Host, Server)
	type pending struct {
		v integrations.View
		r integrations.Resource
	}
	var servers []pending
	for _, v := range list {
		if !v.Enabled || v.Snapshot == nil {
			continue
		}
		src := topoSource{IntegrationID: v.ID, IntegrationName: v.Name, IntegrationType: v.Type}
		res := v.Snapshot.Resources
		hasDevice := false
		for _, r := range res {
			s := src
			s.ResourceID = r.ID
			switch r.Kind {
			case integrations.KindSwitch:
				hasDevice = true
				d := add(topoDevice{ID: resKey(v.ID, r.ID), Name: r.Name, Type: "switch", Status: r.Status, Sources: []topoSource{s}})
				d.Facts = facts(map[string]string{"model": r.Info["model"], "firmware": r.Info["firmware"]})
				for _, p := range res {
					if p.Kind == integrations.KindPort && p.Parent == r.Name {
						d.Ports = append(d.Ports, portOf(p))
					}
				}
				for _, kv := range strings.Split(r.Info["vlan_names"], ";") {
					if id, name, ok := strings.Cut(kv, "="); ok {
						if n, err := strconv.Atoi(id); err == nil {
							vlanNames[n] = name
						}
					}
				}
			case integrations.KindNode:
				hasDevice = true
				typ := "hypervisor"
				if v.Type == "arcane" {
					typ = "docker"
				}
				d := add(topoDevice{ID: resKey(v.ID, r.ID), Name: r.Name, Type: typ, Status: r.Status, Sources: []topoSource{s},
					names: []string{r.Info["hostname"]}})
				d.Facts = facts(map[string]string{"os": r.Info["os"], "cpu": cpuText(r), "memory": memText(r.MemTotal)})
				for _, g := range res {
					if slices.Contains(guestKinds, g.Kind) && g.Parent == r.Name {
						d.Guests = append(d.Guests, topoGuest{ID: resKey(v.ID, g.ID), Name: g.Name, Kind: g.Kind, State: g.State,
							Status: g.Status, Group: cmp.Or(g.Info["stack"], firstTag(g.Tags))})
					}
				}
			case integrations.KindServer:
				hasDevice = true
				servers = append(servers, pending{v, r})
			case integrations.KindProxy:
				out.Exposed = append(out.Exposed, topoExposed{Name: r.Name, Targets: r.Info["targets"], Status: r.Status, Source: v.Name})
			}
		}
		if v.Type == "netbird" {
			vpn := topoVPN{IntegrationID: v.ID, Name: v.Name}
			for _, r := range res {
				if r.Kind == integrations.KindPeer {
					vpn.Total++
					if r.State == "online" {
						vpn.Online++
					}
					if r.Info["watched"] == "1" && r.Status != integrations.StatusOK {
						vpn.Problems++
					}
				}
			}
			out.VPN = append(out.VPN, vpn)
			hasDevice = true
		}
		if !hasDevice {
			out.Services = append(out.Services, topoService{IntegrationID: v.ID, Name: v.Name, Type: v.Type, Status: v.Status})
		}
	}
	// Server (Redfish) gehören meist zu einem Host gleichen Namens: Hardware dort
	for _, p := range servers {
		s := topoSource{IntegrationID: p.v.ID, IntegrationName: p.v.Name, IntegrationType: p.v.Type, ResourceID: p.r.ID}
		hw := facts(map[string]string{"model": strings.TrimSpace(p.r.Info["manufacturer"] + " " + p.r.Info["model"]),
			"cpu": cpuModel(p.r.Info), "memory": gib(p.r.Info["memory_gib"]), "firmware": p.r.Info["bmc"]})
		id := resKey(p.v.ID, p.r.ID)
		target := alias(id)
		if target == "" {
			for _, oid := range order {
				if d := byID[oid]; d.Type == "hypervisor" && normName(d.Name) == normName(p.r.Name) {
					target = oid
					break
				}
			}
		}
		if d := byID[target]; d != nil {
			d.Sources = append(d.Sources, s)
			d.Facts = mergeFacts(hw, d.Facts)
			d.Status = integrations.Worst(d.Status, p.r.Status)
			d.names = append(d.names, p.r.Name)
			continue
		}
		add(topoDevice{ID: id, Name: p.r.Name, Type: "server", Status: p.r.Status, Sources: []topoSource{s}, Facts: hw})
	}

	// 2. eigene Geräte
	for _, m := range cfg.Devices {
		d := add(topoDevice{ID: "dev:" + m.ID, Name: m.Name, Type: m.Type, Status: integrations.StatusUnknown, Manual: true, Note: m.Note})
		d.Facts = facts(map[string]string{"model": m.Model, "ip": m.IP})
		for n := 1; n <= m.Ports; n++ {
			d.Ports = append(d.Ports, topoPort{Name: strconv.Itoa(n), Status: integrations.StatusUnknown, Pos: n})
		}
	}

	// 3. zusammenführen per Zuordnung („res:…“ → Gerät): Quellen, Gäste, Ports wandern mit
	for _, oid := range slices.Clone(order) {
		target := alias(oid)
		d, t := byID[oid], byID[target]
		if d == nil || t == nil || t == d {
			continue
		}
		t.Sources = append(t.Sources, d.Sources...)
		t.Guests = append(t.Guests, d.Guests...)
		t.Ports = append(t.Ports, d.Ports...)
		t.Facts = mergeFacts(t.Facts, d.Facts)
		t.names = append(t.names, append(d.names, d.Name)...)
		if t.Manual {
			t.Status = d.Status
			t.Manual = len(t.Sources) == 0
		} else {
			t.Status = integrations.Worst(t.Status, d.Status)
		}
		delete(byID, oid)
	}

	// 4. Dienste ohne eigenes Gerät einem Host zuordnen („int:<id>“ → Gerät)
	var unassigned []topoService
	for _, s := range out.Services {
		if d := byID[alias("int:"+strconv.FormatInt(s.IntegrationID, 10))]; d != nil {
			d.Services = append(d.Services, s)
			continue
		}
		unassigned = append(unassigned, s)
	}
	if unassigned == nil {
		unassigned = []topoService{}
	}
	out.Services = unassigned

	// Gerät zu einem Namen (LLDP-Systemname, Hostname)
	findByName := func(name string) *topoDevice {
		if d := byID[alias("lldp:"+normName(name))]; d != nil {
			return d
		}
		n := normName(name)
		for _, oid := range order {
			d := byID[oid]
			if d == nil {
				continue
			}
			if normName(d.Name) == n {
				return d
			}
			for _, x := range d.names {
				if x != "" && normName(x) == n {
					return d
				}
			}
		}
		return nil
	}

	// 5. Kabel: eigene zuerst, dann LLDP (nicht für Ports, die schon ein eigenes Kabel haben)
	used := map[topoEndpoint]bool{}
	resolve := func(e topoEndpoint) topoEndpoint {
		if t := alias(e.Device); t != "" && byID[t] != nil {
			e.Device = t
		}
		return e
	}
	for _, l := range cfg.Links {
		a, b := resolve(l.A), resolve(l.B)
		if byID[a.Device] == nil || byID[b.Device] == nil {
			continue
		}
		link := topoLink{ID: "link:" + l.ID, A: a, B: b, Speed: l.Speed, VLAN: l.VLAN, Tagged: l.Tagged, Source: "manual",
			Note: l.Note, Status: integrations.StatusUnknown}
		for _, e := range []topoEndpoint{a, b} {
			if p := portByName(byID[e.Device], e.Port); p != nil && p.Res != "" {
				link.Status = p.Status
				link.Speed = cmp.Or(link.Speed, p.Speed)
				link.VLAN = cmp.Or(link.VLAN, p.PVID)
				if len(link.Tagged) == 0 {
					link.Tagged = p.Tagged
				}
			}
			used[e] = true
		}
		out.Links = append(out.Links, link)
	}
	seen := map[string]bool{}
	for _, oid := range order {
		sw := byID[oid]
		if sw == nil || sw.Type != "switch" {
			continue
		}
		for _, p := range sw.Ports {
			if p.Extra["lldp_name"] == "" || used[topoEndpoint{sw.ID, p.Name}] {
				continue
			}
			peer := findByName(p.Extra["lldp_name"])
			if peer == nil {
				out.Suggestions = append(out.Suggestions, topoSuggestion{Switch: sw.ID, Port: p.Name, Name: p.Extra["lldp_name"],
					Remote: p.Extra["lldp_port"], Chassis: p.Extra["lldp_chassis"]})
				continue
			}
			if peer == sw {
				continue
			}
			// zwei Switches sehen sich gegenseitig: nur ein Kabel
			pair := []string{sw.ID + "|" + p.Name, peer.ID + "|" + p.Extra["lldp_port"]}
			slices.Sort(pair)
			if seen[pair[0]+"~"+pair[1]] {
				continue
			}
			seen[pair[0]+"~"+pair[1]] = true
			b := topoEndpoint{Device: peer.ID, Port: p.Extra["lldp_port"]}
			if used[b] {
				continue
			}
			out.Links = append(out.Links, topoLink{ID: "lldp:" + sw.ID + ":" + p.Name, A: topoEndpoint{sw.ID, p.Name}, B: b,
				Speed: p.Speed, VLAN: p.PVID, Tagged: p.Tagged, Status: p.Status, Source: "lldp"})
		}
	}

	// 6. VLANs: Namen vom Switch, Farben und Subnetze aus der Pflege
	vl := map[int]topoVLAN{}
	for id, name := range vlanNames {
		vl[id] = topoVLAN{ID: id, Name: name}
	}
	for _, v := range cfg.VLANs {
		x := vl[v.ID]
		x.ID, x.Name = v.ID, cmp.Or(v.Name, x.Name)
		x.Subnet, x.Color, x.Note = v.Subnet, v.Color, v.Note
		vl[v.ID] = x
	}
	for _, v := range vl {
		out.VLANs = append(out.VLANs, v)
	}
	slices.SortFunc(out.VLANs, func(a, b topoVLAN) int { return a.ID - b.ID })

	for _, oid := range order {
		d := byID[oid]
		if d == nil || slices.Contains(cfg.Hidden, d.ID) {
			continue
		}
		d.Tier = tierOf(d.Type)
		if d.Ports == nil {
			d.Ports = []topoPort{}
		}
		if d.Guests == nil {
			d.Guests = []topoGuest{}
		}
		if d.Services == nil {
			d.Services = []topoService{}
		}
		out.Devices = append(out.Devices, *d)
	}
	// Kabel zu ausgeblendeten Geräten entfallen
	visible := map[string]bool{}
	for _, d := range out.Devices {
		visible[d.ID] = true
	}
	out.Links = slices.DeleteFunc(out.Links, func(l topoLink) bool { return !visible[l.A.Device] || !visible[l.B.Device] })
	return out
}

func portOf(p integrations.Resource) topoPort {
	atoi := func(k string) int { n, _ := strconv.Atoi(p.Info[k]); return n }
	out := topoPort{Name: p.Name, Status: p.Status, State: p.State, Speed: atoi("speed"), Media: p.Info["media"],
		Unit: atoi("unit"), Pos: atoi("pos"), Alias: p.Info["alias"], PVID: atoi("pvid"), Util: p.Info["util"], Res: p.ID,
		Extra: map[string]string{"lldp_name": p.Info["lldp_name"], "lldp_port": p.Info["lldp_port"], "lldp_chassis": p.Info["lldp_chassis"]}}
	for _, s := range strings.Split(p.Info["vlans"], ",") {
		if n, err := strconv.Atoi(s); err == nil {
			out.Tagged = append(out.Tagged, n)
		}
	}
	return out
}

func portByName(d *topoDevice, name string) *topoPort {
	if d == nil || name == "" {
		return nil
	}
	for i := range d.Ports {
		if d.Ports[i].Name == name {
			return &d.Ports[i]
		}
	}
	return nil
}

// facts: nur gesetzte Angaben, feste Reihenfolge.
func facts(m map[string]string) []topoFact {
	out := []topoFact{}
	for _, k := range []string{"model", "os", "firmware", "cpu", "memory", "ip"} {
		if v := strings.TrimSpace(m[k]); v != "" {
			out = append(out, topoFact{Key: k, Value: v})
		}
	}
	return out
}

// mergeFacts: a hat Vorrang, b ergänzt fehlende Schlüssel.
func mergeFacts(a, b []topoFact) []topoFact {
	out := slices.Clone(a)
	for _, f := range b {
		if !slices.ContainsFunc(out, func(x topoFact) bool { return x.Key == f.Key }) {
			out = append(out, f)
		}
	}
	order := []string{"model", "os", "firmware", "cpu", "memory", "ip"}
	slices.SortStableFunc(out, func(x, y topoFact) int { return slices.Index(order, x.Key) - slices.Index(order, y.Key) })
	return out
}

func cpuText(r integrations.Resource) string {
	if r.CPUs > 0 {
		return strconv.FormatFloat(r.CPUs, 'f', -1, 64) + " vCPU"
	}
	return ""
}

func cpuModel(info map[string]string) string {
	m := strings.TrimSpace(info["cpu_model"])
	if n := info["cpus"]; n != "" && n != "1" && m != "" {
		return n + "× " + m
	}
	return m
}

func memText(b *int64) string {
	if b == nil || *b <= 0 {
		return ""
	}
	return strconv.FormatFloat(float64(*b)/(1<<30), 'f', 0, 64) + " GiB"
}

func gib(s string) string {
	if s == "" {
		return ""
	}
	return s + " GiB"
}

func firstTag(tags []string) string {
	if len(tags) > 0 {
		return tags[0]
	}
	return ""
}

// --- Speichern, Prüfen, HTTP -----------------------------------------------

var topoMu sync.Mutex

func (s *Server) loadTopoConfig(ctx context.Context) (topoConfig, error) {
	cfg := topoConfig{Aliases: map[string]string{}}
	raw, err := s.prefs.GetSetting(ctx, keyTopology)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return cfg, err
	}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg) // kaputt = leer
	}
	if cfg.Aliases == nil {
		cfg.Aliases = map[string]string{}
	}
	return cfg, nil
}

// validateTopo prüft die gepflegten Angaben; Texte werden gekürzt.
func validateTopo(c *topoConfig) *i18n.Msg {
	bad := func(what string) *i18n.Msg { m := i18n.M("topology.invalid", "what", what); return &m }
	if len(c.Devices) > maxTopoDevices || len(c.Links) > maxTopoLinks || len(c.VLANs) > maxTopoVLANs || len(c.Aliases) > maxTopoLinks {
		m := i18n.M("topology.too_many")
		return &m
	}
	clip := func(s string) string {
		s = strings.TrimSpace(s)
		if r := []rune(s); len(r) > maxTopoText {
			return string(r[:maxTopoText])
		}
		return s
	}
	ids := map[string]bool{}
	for n := range c.Devices {
		d := &c.Devices[n]
		d.Name, d.Model, d.IP, d.Note = clip(d.Name), clip(d.Model), clip(d.IP), clip(d.Note)
		if !topoIDRe.MatchString(d.ID) || ids[d.ID] || d.Name == "" || !slices.Contains(topoTypes, d.Type) || d.Ports < 0 || d.Ports > 64 {
			return bad(cmp.Or(d.Name, d.ID))
		}
		ids[d.ID] = true
	}
	linkIDs := map[string]bool{}
	for n := range c.Links {
		l := &c.Links[n]
		l.Note = clip(l.Note)
		if !topoIDRe.MatchString(l.ID) || linkIDs[l.ID] || l.A.Device == "" || l.B.Device == "" || l.A == l.B ||
			l.Speed < 0 || l.Speed > 1_000_000 || l.VLAN < 0 || l.VLAN > 4094 || len(l.Tagged) > 4094 ||
			len(l.A.Device) > 160 || len(l.B.Device) > 160 || len(l.A.Port) > maxTopoText || len(l.B.Port) > maxTopoText {
			return bad(l.ID)
		}
		for _, t := range l.Tagged {
			if t < 1 || t > 4094 {
				return bad(l.ID)
			}
		}
		linkIDs[l.ID] = true
	}
	vids := map[int]bool{}
	for n := range c.VLANs {
		v := &c.VLANs[n]
		v.Name, v.Subnet, v.Note = clip(v.Name), clip(v.Subnet), clip(v.Note)
		if v.ID < 1 || v.ID > 4094 || vids[v.ID] || (v.Color != "" && !colorRe.MatchString(v.Color)) {
			return bad("VLAN " + strconv.Itoa(v.ID))
		}
		vids[v.ID] = true
	}
	for k, v := range c.Aliases {
		if !aliasKeys.MatchString(k) || v == "" || len(v) > 160 {
			return bad(k)
		}
	}
	if len(c.Hidden) > maxTopoDevices {
		m := i18n.M("topology.too_many")
		return &m
	}
	if c.Aliases == nil {
		c.Aliases = map[string]string{}
	}
	return nil
}

func (s *Server) topology(ctx context.Context) (topologyDTO, error) {
	cfg, err := s.loadTopoConfig(ctx)
	if err != nil {
		return topologyDTO{}, err
	}
	list, err := s.integrations.List(ctx)
	if err != nil {
		return topologyDTO{}, err
	}
	return buildTopology(list, cfg), nil
}

func (s *Server) handleGetTopology(w http.ResponseWriter, r *http.Request) {
	t, err := s.topology(r.Context())
	if err != nil {
		s.internalError(w, r, "Netzplan laden", err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handlePutTopology(w http.ResponseWriter, r *http.Request) {
	var in topoConfig
	if !decodeJSONMax(w, r, &in, 512<<10) {
		return
	}
	if msg := validateTopo(&in); msg != nil {
		writeMsg(w, r, http.StatusBadRequest, *msg)
		return
	}
	b, _ := json.Marshal(in)
	topoMu.Lock()
	err := s.prefs.SetSetting(r.Context(), keyTopology, string(b))
	topoMu.Unlock()
	if err != nil {
		s.internalError(w, r, "Netzplan speichern", err)
		return
	}
	events.Publish(s.events, events.TopicTopology)
	s.handleGetTopology(w, r)
}
