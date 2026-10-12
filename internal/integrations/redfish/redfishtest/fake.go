// Package redfishtest bildet die benötigten Teile einer Redfish-API nach –
// HPE iLO 5 (mit Oem.Hpe und SmartStorage) oder ein generischer BMC (nur
// DMTF-Felder) – für Tests und den Entwicklungs-Mock.
package redfishtest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Temp ist ein Temperatursensor.
type Temp struct {
	Name, Context     string
	Reading, Critical float64
	Health            string // OK, Warning, Critical
	Absent            bool
}

// Fan ist ein Lüfter (Drehzahl in %).
type Fan struct {
	Name   string
	Speed  float64
	Health string
}

// PSU ist ein Netzteil.
type PSU struct {
	Health string
	Output float64
	Absent bool
}

// Drive ist ein physisches Laufwerk.
type Drive struct {
	Location, Model string
	CapacityGB      float64
	SSD             bool
	WearUsed        float64 // verbrauchte Lebensdauer in % (nur SSD)
	Health          string
}

// Volume ist ein logisches Laufwerk (RAID).
type Volume struct {
	Name, Raid string
	Health     string
}

// Profile beschreibt einen Server.
type Profile struct {
	HPE                                    bool // iLO: Oem.Hpe, SmartStorage, $expand
	HostName, Model, Serial, Bios, BMC, FW string
	PowerOn                                bool
	Health                                 string // Rollup des Systems
	Temps                                  []Temp
	Fans                                   []Fan
	PSUs                                   []PSU
	Drives                                 []Drive
	Volumes                                []Volume
	Watts                                  float64
	CPUs                                   int
	CPUModel                               string
	MemGiB                                 float64
}

// ILO5 ist ein HPE ProLiant DL380 Gen10 mit iLO 5, zwei Netzteilen, sechs
// Lüftern, Smart-Array mit vier Laufwerken (eine SSD) und zwei RAIDs.
func ILO5() Profile {
	return Profile{
		HPE: true, HostName: "pve-01", Model: "ProLiant DL380 Gen10", Serial: "CZJ12345AB", Bios: "U30 v2.80 (01/19/2026)",
		BMC: "iLO 5", FW: "iLO 5 v3.19", PowerOn: true, Health: "OK", Watts: 212,
		CPUs: 2, CPUModel: "Intel(R) Xeon(R) Silver 4214R CPU @ 2.40GHz", MemGiB: 256,
		Temps: []Temp{
			{Name: "01-Inlet Ambient", Context: "Intake", Reading: 23, Critical: 42, Health: "OK"},
			{Name: "02-CPU 1", Context: "CPU", Reading: 41, Critical: 70, Health: "OK"},
			{Name: "03-CPU 2", Context: "CPU", Reading: 39, Critical: 70, Health: "OK"},
			{Name: "05-P1 DIMM 1-6", Context: "SystemBoard", Reading: 33, Critical: 90, Health: "OK"},
			{Name: "12-HD Max", Context: "StorageDevice", Reading: 35, Critical: 60, Health: "OK"},
			{Name: "14-Chipset", Context: "SystemBoard", Reading: 48, Critical: 105, Health: "OK"},
			{Name: "20-PCI 1", Context: "SystemBoard", Absent: true},
		},
		Fans: []Fan{{"Fan 1", 23, "OK"}, {"Fan 2", 23, "OK"}, {"Fan 3", 25, "OK"}, {"Fan 4", 25, "OK"}, {"Fan 5", 23, "OK"}, {"Fan 6", 23, "OK"}},
		PSUs: []PSU{{Health: "OK", Output: 110}, {Health: "OK", Output: 102}},
		Drives: []Drive{
			{Location: "1I:1:1", Model: "MO000960JWFWR", CapacityGB: 960, SSD: true, WearUsed: 7, Health: "OK"},
			{Location: "1I:1:2", Model: "MO000960JWFWR", CapacityGB: 960, SSD: true, WearUsed: 8, Health: "OK"},
			{Location: "1I:1:3", Model: "MB004000GWKGV", CapacityGB: 4000, Health: "OK"},
			{Location: "1I:1:4", Model: "MB004000GWKGV", CapacityGB: 4000, Health: "OK"},
		},
		Volumes: []Volume{{Name: "boot", Raid: "1", Health: "OK"}, {Name: "data", Raid: "1", Health: "OK"}},
	}
}

// Generic ist ein BMC ohne Herstellererweiterungen (z. B. Supermicro):
// Laufwerke über DMTF Storage, kein $expand.
func Generic() Profile {
	return Profile{
		HostName: "backup-box", Model: "X11SCL-F", Serial: "S1234567", Bios: "2.1", BMC: "BMC", FW: "1.74.11",
		PowerOn: true, Health: "OK", Watts: 64, CPUs: 1, CPUModel: "Intel(R) Xeon(R) E-2236 CPU @ 3.40GHz", MemGiB: 64,
		Temps: []Temp{{Name: "CPU Temp", Context: "CPU", Reading: 38, Critical: 98, Health: "OK"},
			{Name: "Inlet Temp", Context: "Intake", Reading: 25, Critical: 45, Health: "OK"}},
		Fans:    []Fan{{"FAN1", 30, "OK"}, {"FAN2", 31, "OK"}},
		PSUs:    []PSU{{Health: "OK", Output: 64}},
		Drives:  []Drive{{Location: "Disk.Bay.0", Model: "WDC WD80EFZZ", CapacityGB: 8000, Health: "OK"}, {Location: "Disk.Bay.1", Model: "WDC WD80EFZZ", CapacityGB: 8000, Health: "OK"}},
		Volumes: []Volume{{Name: "md0", Raid: "RAID1", Health: "OK"}},
	}
}

type Fake struct {
	mu       sync.Mutex
	user     string
	pass     string
	p        Profile
	fail     int
	broken   bool
	delay    time.Duration
	requests int
}

func New(user, pass string, p Profile) *Fake { return &Fake{user: user, pass: pass, p: p} }

func (f *Fake) Fail(status int)       { f.mu.Lock(); f.fail = status; f.mu.Unlock() }
func (f *Fake) Broken(b bool)         { f.mu.Lock(); f.broken = b; f.mu.Unlock() }
func (f *Fake) Delay(d time.Duration) { f.mu.Lock(); f.delay = d; f.mu.Unlock() }
func (f *Fake) Requests() int         { f.mu.Lock(); defer f.mu.Unlock(); return f.requests }

// Update ändert das Profil (z. B. Netzteil ausfallen lassen).
func (f *Fake) Update(fn func(p *Profile)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(&f.p)
}

type obj = map[string]any

func link(path string) obj { return obj{"@odata.id": path} }

func st(health string) obj {
	if health == "" {
		return obj{"State": "Absent"}
	}
	return obj{"State": "Enabled", "Health": health}
}

func worst(list ...string) string {
	rank := map[string]int{"OK": 1, "Warning": 2, "Critical": 3}
	out := "OK"
	for _, h := range list {
		if rank[h] > rank[out] {
			out = h
		}
	}
	return out
}

const sys = "/redfish/v1/Systems/1/"

func (f *Fake) system() obj {
	p := f.p
	power := "Off"
	if p.PowerOn {
		power = "On"
	}
	s := obj{
		"@odata.id": sys, "Id": "1", "Name": "Computer System", "HostName": p.HostName, "Manufacturer": "HPE",
		"Model": p.Model, "SerialNumber": p.Serial, "BiosVersion": p.Bios, "PowerState": power,
		"Status":           obj{"State": "Enabled", "Health": p.Health, "HealthRollup": p.Health},
		"MemorySummary":    obj{"TotalSystemMemoryGiB": p.MemGiB, "Status": obj{"HealthRollup": "OK"}},
		"ProcessorSummary": obj{"Count": p.CPUs, "Model": p.CPUModel, "Status": obj{"HealthRollup": "OK"}},
		"Links":            obj{"Chassis": []obj{link("/redfish/v1/Chassis/1/")}, "ManagedBy": []obj{link("/redfish/v1/Managers/1/")}},
		"Storage":          link(sys + "Storage/"),
	}
	if !p.HPE {
		s["Manufacturer"] = "Supermicro"
		return s
	}
	var psus, fans, temps, drives []string
	for _, x := range p.PSUs {
		psus = append(psus, x.Health)
	}
	for _, x := range p.Fans {
		fans = append(fans, x.Health)
	}
	for _, x := range p.Temps {
		temps = append(temps, x.Health)
	}
	for _, x := range p.Drives {
		drives = append(drives, x.Health)
	}
	redundancy := "Redundant"
	if worst(psus...) != "OK" {
		redundancy = "FailedRedundant"
	}
	s["Oem"] = obj{"Hpe": obj{
		"PostState": "FinishedPost",
		"AggregateHealthStatus": obj{
			"AgentlessManagementService": "Unavailable",
			"BiosOrHardwareHealth":       obj{"Status": obj{"Health": "OK"}},
			"FanRedundancy":              "Redundant",
			"Fans":                       obj{"Status": obj{"Health": worst(fans...)}},
			"Memory":                     obj{"Status": obj{"Health": "OK"}},
			"Network":                    obj{"Status": obj{"Health": "OK"}},
			"PowerSupplies":              obj{"PowerSuppliesMismatch": false, "Status": obj{"Health": worst(psus...)}},
			"PowerSupplyRedundancy":      redundancy,
			"Processors":                 obj{"Status": obj{"Health": "OK"}},
			"SmartStorageBattery":        obj{"Status": obj{"Health": "OK"}},
			"Storage":                    obj{"Status": obj{"Health": worst(drives...)}},
			"Temperatures":               obj{"Status": obj{"Health": worst(temps...)}},
		},
	}}
	return s
}

func (f *Fake) thermal() obj {
	var temps, fans []obj
	for n, t := range f.p.Temps {
		x := obj{"MemberId": fmt.Sprint(n), "Name": t.Name, "PhysicalContext": t.Context, "Status": st(t.Health)}
		if !t.Absent {
			x["ReadingCelsius"], x["UpperThresholdCritical"] = t.Reading, t.Critical
		} else {
			x["Status"] = obj{"State": "Absent"}
		}
		temps = append(temps, x)
	}
	for n, fan := range f.p.Fans {
		fans = append(fans, obj{"MemberId": fmt.Sprint(n), "Name": fan.Name, "Reading": fan.Speed, "ReadingUnits": "Percent", "Status": st(fan.Health)})
	}
	return obj{"Temperatures": temps, "Fans": fans}
}

func (f *Fake) power() obj {
	var psus []obj
	for n, p := range f.p.PSUs {
		x := obj{"MemberId": fmt.Sprint(n), "Name": "HpeServerPowerSupply", "PowerCapacityWatts": 800, "Status": st(p.Health)}
		if p.Absent {
			x["Status"] = obj{"State": "Absent"}
		} else {
			x["LastPowerOutputWatts"] = p.Output
		}
		psus = append(psus, x)
	}
	return obj{"PowerControl": []obj{{"PowerConsumedWatts": f.p.Watts}}, "PowerSupplies": psus}
}

// collection liefert Verweise oder (mit $expand, nur iLO) die Einträge selbst.
func (f *Fake) collection(r *http.Request, paths []string, get func(path string) obj) obj {
	members := []obj{}
	for _, p := range paths {
		if f.p.HPE && r.URL.Query().Get("$expand") == "." {
			members = append(members, get(p))
		} else {
			members = append(members, link(p))
		}
	}
	return obj{"Members": members, "Members@odata.count": len(members)}
}

const smart = sys + "SmartStorage/ArrayControllers/"

func (f *Fake) resource(r *http.Request, path string) (obj, bool) {
	p := f.p
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	n := func(prefix string) (int, bool) {
		rest, ok := strings.CutPrefix(path, prefix)
		if !ok {
			return 0, false
		}
		var i int
		_, err := fmt.Sscanf(strings.TrimSuffix(rest, "/"), "%d", &i)
		return i, err == nil && strings.Count(rest, "/") == 1
	}
	drivePaths, volPaths := []string{}, []string{}
	for i := range p.Drives {
		drivePaths = append(drivePaths, fmt.Sprintf("%s0/DiskDrives/%d/", smart, i))
	}
	for i := range p.Volumes {
		volPaths = append(volPaths, fmt.Sprintf("%s0/LogicalDrives/%d/", smart, i+1))
	}
	switch path {
	case "/redfish/v1/Systems/":
		return f.collection(r, []string{sys}, func(string) obj { return f.system() }), true
	case sys:
		return f.system(), true
	case "/redfish/v1/Chassis/1/":
		return obj{"Thermal": link("/redfish/v1/Chassis/1/Thermal/"), "Power": link("/redfish/v1/Chassis/1/Power/")}, true
	case "/redfish/v1/Chassis/1/Thermal/":
		return f.thermal(), true
	case "/redfish/v1/Chassis/1/Power/":
		return f.power(), true
	case "/redfish/v1/Managers/1/":
		return obj{"Model": p.BMC, "FirmwareVersion": p.FW}, true
	case sys + "Storage/":
		if p.HPE { // iLO 5 mit Smart Array: DMTF Storage bleibt leer
			return obj{"Members": []obj{}}, true
		}
		return f.collection(r, []string{sys + "Storage/1/"}, nil), true
	case sys + "Storage/1/":
		var drives []obj
		for i := range p.Drives {
			drives = append(drives, link(fmt.Sprintf("%sStorage/1/Drives/%d/", sys, i)))
		}
		return obj{"Id": "1", "Name": "SATA Controller", "Status": st("OK"), "Drives": drives, "Volumes": link(sys + "Storage/1/Volumes/")}, true
	case sys + "Storage/1/Volumes/":
		var paths []string
		for i := range p.Volumes {
			paths = append(paths, fmt.Sprintf("%sStorage/1/Volumes/%d/", sys, i))
		}
		return f.collection(r, paths, nil), true
	case smart:
		if !p.HPE {
			return nil, false
		}
		return f.collection(r, []string{smart + "0/"}, func(string) obj { return f.controller() }), true
	case smart + "0/":
		return f.controller(), true
	case smart + "0/DiskDrives/":
		return f.collection(r, drivePaths, func(s string) obj { return f.hpeDrive(indexOf(drivePaths, s, 0)) }), true
	case smart + "0/LogicalDrives/":
		return f.collection(r, volPaths, func(s string) obj { return f.hpeVolume(indexOf(volPaths, s, 0)) }), true
	}
	if i, ok := n(smart + "0/DiskDrives/"); ok && i < len(p.Drives) && p.HPE {
		return f.hpeDrive(i), true
	}
	if i, ok := n(smart + "0/LogicalDrives/"); ok && i >= 1 && i <= len(p.Volumes) && p.HPE {
		return f.hpeVolume(i - 1), true
	}
	if i, ok := n(sys + "Storage/1/Drives/"); ok && i < len(p.Drives) && !p.HPE {
		d := p.Drives[i]
		x := obj{"Id": d.Location, "Name": d.Location, "Model": d.Model, "CapacityBytes": d.CapacityGB * 1e9, "MediaType": "HDD", "Status": st(d.Health),
			"PhysicalLocation": obj{"PartLocation": obj{"ServiceLabel": d.Location}}}
		if d.SSD {
			x["MediaType"], x["PredictedMediaLifeLeftPercent"] = "SSD", 100-d.WearUsed
		}
		return x, true
	}
	if i, ok := n(sys + "Storage/1/Volumes/"); ok && i < len(p.Volumes) && !p.HPE {
		v := p.Volumes[i]
		return obj{"Id": fmt.Sprint(i), "Name": v.Name, "RAIDType": v.Raid, "Status": st(v.Health)}, true
	}
	return nil, false
}

func indexOf(list []string, s string, def int) int {
	for i, x := range list {
		if x == s {
			return i
		}
	}
	return def
}

func (f *Fake) controller() obj {
	return obj{"@odata.id": smart + "0/", "Id": "0", "Model": "HPE Smart Array P408i-a SR Gen10", "Location": "Slot 0",
		"Status": st("OK"), "Links": obj{"PhysicalDrives": link(smart + "0/DiskDrives/"), "LogicalDrives": link(smart + "0/LogicalDrives/")}}
}

func (f *Fake) hpeDrive(i int) obj {
	d := f.p.Drives[i]
	x := obj{"Id": fmt.Sprint(i), "Location": d.Location, "Model": d.Model, "CapacityMiB": d.CapacityGB * 1e9 / 1024 / 1024,
		"MediaType": "HDD", "Status": st(d.Health)}
	if d.SSD {
		x["MediaType"], x["SSDEnduranceUtilizationPercentage"] = "SSD", d.WearUsed
	}
	return x
}

func (f *Fake) hpeVolume(i int) obj {
	v := f.p.Volumes[i]
	return obj{"Id": fmt.Sprint(i + 1), "LogicalDriveNumber": i + 1, "LogicalDriveName": v.Name, "Raid": v.Raid, "Status": st(v.Health)}
}

func (f *Fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests++
	fail, broken, delay := f.fail, f.broken, f.delay
	f.mu.Unlock()
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
	}
	if fail != 0 {
		http.Error(w, http.StatusText(fail), fail)
		return
	}
	if u, p, ok := r.BasicAuth(); !ok || u != f.user || p != f.pass {
		w.Header().Set("WWW-Authenticate", `Basic realm="BMC"`)
		http.Error(w, `{"error":{"code":"Base.1.4.NoValidSession"}}`, http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if broken {
		w.Write([]byte(`{"Members": [`))
		return
	}
	f.mu.Lock()
	res, ok := f.resource(r, r.URL.Path)
	f.mu.Unlock()
	if !ok {
		http.Error(w, `{"error":{"code":"Base.1.4.ResourceMissingAtURI"}}`, http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(res)
}
