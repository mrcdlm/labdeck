package api

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/mrcdlm/labdeck/internal/integrations"
)

func TestNormName(t *testing.T) {
	for in, want := range map[string]string{"PVE-01.lan": "pve1", "pve1": "pve1", "docker01": "docker1", "Docker_01": "docker1",
		"ap-living": "apliving", "10.0.0.1": "10", "node-100": "node100"} {
		if got := normName(in); got != want {
			t.Errorf("%q → %q, erwartet %q", in, got, want)
		}
	}
}

// topoViews: Switch (LLDP zu pve1, docker01, opnsense, unbekanntem Gerät),
// PVE mit pve1 und Gästen, iLO-Server pve-01, Arcane mit docker01, Immich.
func topoViews() []integrations.View {
	ok := integrations.StatusOK
	port := func(name, lldp, remote string, pvid, speed int, vlans string, st integrations.Status) integrations.Resource {
		info := map[string]string{"speed": fmt.Sprint(speed), "pvid": fmt.Sprint(pvid), "media": "rj45", "unit": "1"}
		if lldp != "" {
			info["lldp_name"], info["lldp_port"] = lldp, remote
		}
		if vlans != "" {
			info["vlans"] = vlans
		}
		return integrations.Resource{ID: "port/" + name, Name: name, Kind: integrations.KindPort, Parent: "core", State: "up", Status: st, Info: info}
	}
	view := func(id int64, typ, name string, res ...integrations.Resource) integrations.View {
		return integrations.View{ID: id, Type: typ, Name: name, Enabled: true, Status: ok, Snapshot: &integrations.Snapshot{Resources: res}}
	}
	mem := int64(64 << 30)
	return []integrations.View{
		view(1, "snmp", "Core-Switch",
			integrations.Resource{ID: "switch", Name: "core", Kind: integrations.KindSwitch, Status: ok,
				Info: map[string]string{"model": "C1300", "vlan_names": "1=default;10=LAN;20=DMZ"}},
			port("gi1", "pve1.lan", "enp3s0", 10, 1000, "20", ok),
			port("gi2", "docker01", "ens18", 10, 1000, "", ok),
			port("gi3", "mystery-box", "eth0", 10, 100, "", ok),
			port("gi4", "", "", 10, 0, "", integrations.StatusPaused),
			port("gi24", "OPNsense", "igc1", 1, 10000, "10,20", ok),
		),
		view(2, "pve", "Proxmox VE",
			integrations.Resource{ID: "node/pve1", Name: "pve1", Kind: integrations.KindNode, Status: ok, CPUs: 24, MemTotal: &mem},
			integrations.Resource{ID: "qemu/100", Name: "homeassistant", Kind: integrations.KindVM, Parent: "pve1", State: "running", Status: ok, Tags: []string{"iot"}},
			integrations.Resource{ID: "lxc/101", Name: "pihole", Kind: integrations.KindCT, Parent: "pve1", State: "running", Status: ok},
		),
		view(3, "redfish", "iLO pve-01",
			integrations.Resource{ID: "server", Name: "pve-01", Kind: integrations.KindServer, Status: integrations.StatusWarning,
				Info: map[string]string{"manufacturer": "HPE", "model": "ProLiant DL360 Gen10", "cpu_model": "Xeon Silver 4214R", "cpus": "2", "memory_gib": "128"}},
		),
		view(4, "arcane", "Arcane",
			integrations.Resource{ID: "env/0", Name: "docker01", Kind: integrations.KindNode, Status: ok, Info: map[string]string{"os": "Ubuntu 24.04"}},
			integrations.Resource{ID: "container/0/jellyfin", Name: "jellyfin", Kind: integrations.KindContainer, Parent: "docker01", State: "running",
				Status: ok, Info: map[string]string{"stack": "media"}},
		),
		view(5, "immich", "Immich"),
	}
}

func TestBuildTopology(t *testing.T) {
	cfg := topoConfig{
		Devices: []topoManualDevice{{ID: "opnsense", Name: "OPNsense", Type: "firewall", Model: "Lenovo M720q", Ports: 4},
			{ID: "internet", Name: "Internet", Type: "internet"}},
		Links:   []topoManualLink{{ID: "wan", A: topoEndpoint{"dev:internet", ""}, B: topoEndpoint{"dev:opnsense", "1"}, Speed: 1000}},
		VLANs:   []topoVLAN{{ID: 10, Color: "#58a6ff", Subnet: "10.0.10.0/24"}, {ID: 99, Name: "MGMT", Color: "#e3b341"}},
		Aliases: map[string]string{"int:5": "res:4:env/0"},
	}
	got := buildTopology(topoViews(), cfg)
	dev := map[string]topoDevice{}
	for _, d := range got.Devices {
		dev[d.ID] = d
	}

	// iLO-Server wurde mit pve1 zusammengeführt (pve-01 = pve1): Hardware + Gäste in einer Karte
	pve := dev["res:2:node/pve1"]
	if _, ok := dev["res:3:server"]; ok || len(pve.Sources) != 2 || len(pve.Guests) != 2 || pve.Status != integrations.StatusWarning ||
		pve.Type != "hypervisor" || pve.Tier != 3 {
		t.Fatalf("pve1: %+v", pve)
	}
	if f := factMap(pve.Facts); f["model"] != "HPE ProLiant DL360 Gen10" || f["cpu"] != "2× Xeon Silver 4214R" || f["memory"] != "128 GiB" {
		t.Errorf("Hardware: %v", f)
	}
	if g := pve.Guests[0]; g.Name != "homeassistant" || g.Group != "iot" || g.ID != "res:2:qemu/100" {
		t.Errorf("Gast: %+v", g)
	}
	// Immich läuft (Zuordnung) auf docker01; keine freien Dienste mehr
	if d := dev["res:4:env/0"]; d.Type != "docker" || len(d.Services) != 1 || d.Services[0].Name != "Immich" || len(got.Services) != 0 {
		t.Errorf("Dienste: %+v / %+v", d.Services, got.Services)
	}
	if fw := dev["dev:opnsense"]; !fw.Manual || fw.Tier != 1 || len(fw.Ports) != 4 || dev["dev:internet"].Tier != 0 || dev["res:1:switch"].Tier != 2 {
		t.Errorf("Ebenen: %+v", got.Devices)
	}

	// Kabel: eigenes (Internet → OPNsense) + LLDP zu pve1 (Hostname mit Domain), docker01, OPNsense (eigenes Gerät per Name)
	links := map[string]topoLink{}
	for _, l := range got.Links {
		links[l.B.Device+"/"+l.B.Port] = l
	}
	if l := links["res:2:node/pve1/enp3s0"]; l.Source != "lldp" || l.A.Port != "gi1" || l.VLAN != 10 || !slices.Equal(l.Tagged, []int{20}) || l.Speed != 1000 {
		t.Errorf("pve1-Kabel: %+v", l)
	}
	if l := links["dev:opnsense/igc1"]; l.Speed != 10000 || !slices.Equal(l.Tagged, []int{10, 20}) {
		t.Errorf("Uplink: %+v", l)
	}
	if _, ok := links["res:4:env/0/ens18"]; !ok || links["dev:opnsense/1"].Source != "manual" || len(got.Links) != 4 {
		t.Errorf("Kabel: %+v", got.Links)
	}
	// unbekannter LLDP-Nachbar → Vorschlag
	if len(got.Suggestions) != 1 || got.Suggestions[0].Name != "mystery-box" || got.Suggestions[0].Port != "gi3" {
		t.Errorf("Vorschläge: %+v", got.Suggestions)
	}
	// VLANs: Namen vom Switch, Farben/Subnetz gepflegt, 99 nur gepflegt
	if v := got.VLANs; len(v) != 4 || v[1].ID != 10 || v[1].Name != "LAN" || v[1].Color != "#58a6ff" || v[1].Subnet != "10.0.10.0/24" || v[3].Name != "MGMT" {
		t.Errorf("VLANs: %+v", v)
	}

	// Zuordnung von Hand: mystery-box ist der Drucker; eigenes Kabel ersetzt das LLDP-Kabel
	cfg.Devices = append(cfg.Devices, topoManualDevice{ID: "printer", Name: "Drucker", Type: "other"})
	cfg.Aliases["lldp:"+normName("mystery-box")] = "dev:printer"
	cfg.Links = append(cfg.Links, topoManualLink{ID: "nas", A: topoEndpoint{"res:1:switch", "gi2"}, B: topoEndpoint{"dev:printer", ""}})
	cfg.Hidden = []string{"dev:internet"}
	got = buildTopology(topoViews(), cfg)
	links = map[string]topoLink{}
	for _, l := range got.Links {
		links[l.A.Port] = l
	}
	if len(got.Suggestions) != 0 || links["gi3"].B.Device != "dev:printer" || links["gi2"].Source != "manual" || links["gi2"].Speed != 1000 {
		t.Errorf("Zuordnung: %+v / %+v", got.Suggestions, got.Links)
	}
	// ausgeblendetes Gerät samt Kabel weg
	for _, d := range got.Devices {
		if d.ID == "dev:internet" {
			t.Error("ausgeblendet sichtbar")
		}
	}
	for _, l := range got.Links {
		if l.A.Device == "dev:internet" || l.B.Device == "dev:internet" {
			t.Errorf("Kabel zu ausgeblendetem Gerät: %+v", l)
		}
	}
}

func factMap(f []topoFact) map[string]string {
	m := map[string]string{}
	for _, x := range f {
		m[x.Key] = x.Value
	}
	return m
}

func TestValidateTopo(t *testing.T) {
	ok := topoConfig{Devices: []topoManualDevice{{ID: "fw", Name: " OPNsense ", Type: "firewall"}},
		Links: []topoManualLink{{ID: "l1", A: topoEndpoint{"dev:fw", "1"}, B: topoEndpoint{"res:1:switch", "gi1"}, Tagged: []int{10}}},
		VLANs: []topoVLAN{{ID: 10, Color: "#112233"}}, Aliases: map[string]string{"lldp:opnsense": "dev:fw"}}
	if m := validateTopo(&ok); m != nil || ok.Devices[0].Name != "OPNsense" {
		t.Fatalf("gültig: %v %+v", m, ok.Devices[0])
	}
	bad := []func(c *topoConfig){
		func(c *topoConfig) { c.Devices[0].Type = "toaster" },
		func(c *topoConfig) { c.Devices[0].ID = "Nicht OK" },
		func(c *topoConfig) { c.Devices = append(c.Devices, c.Devices[0]) },
		func(c *topoConfig) { c.Links[0].B = c.Links[0].A },
		func(c *topoConfig) { c.Links[0].Tagged = []int{5000} },
		func(c *topoConfig) { c.VLANs[0].Color = "red" },
		func(c *topoConfig) { c.Aliases["foo"] = "dev:fw" },
		func(c *topoConfig) { c.Devices[0].Name = "" },
	}
	for n, f := range bad {
		var c topoConfig
		b, _ := json.Marshal(ok)
		_ = json.Unmarshal(b, &c)
		f(&c)
		if validateTopo(&c) == nil {
			t.Errorf("Fall %d wurde angenommen", n)
		}
	}
}

func TestTopologyAPI(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)
	resp := e.do(t, "GET", "/api/topology", "", c)
	if resp.StatusCode != 200 {
		t.Fatalf("GET: %d", resp.StatusCode)
	}
	cfg := `{"devices":[{"id":"fw","name":"OPNsense","type":"firewall","ports":2}],"links":[],"vlans":[{"id":10,"name":"LAN","color":"#58a6ff"}],"aliases":{},"hidden":[]}`
	resp = e.do(t, "PUT", "/api/topology", cfg, c)
	raw := body(t, resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(raw, `"id":"dev:fw"`) || !strings.Contains(raw, `"tier":1`) {
		t.Fatalf("PUT: %d %s", resp.StatusCode, raw)
	}
	if resp := e.do(t, "PUT", "/api/topology", `{"devices":[{"id":"x","name":"X","type":"toaster"}]}`, c); resp.StatusCode != 400 {
		t.Errorf("ungültig: %d", resp.StatusCode)
	}
	// gespeichert
	raw = body(t, e.do(t, "GET", "/api/topology", "", c).Body)
	if !strings.Contains(raw, `"name":"OPNsense"`) || !strings.Contains(raw, `"color":"#58a6ff"`) {
		t.Errorf("gespeichert: %s", raw)
	}
}
