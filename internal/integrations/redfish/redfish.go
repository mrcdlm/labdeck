// Package redfish überwacht Server-Hardware über den BMC per DMTF Redfish:
// Gesamtzustand, Netzteile, Lüfter, Temperaturen, Speicher, Prozessoren und
// Laufwerke. Referenz ist HPE iLO 5 (zusätzlich Oem.Hpe.AggregateHealthStatus
// und SmartStorage); Dell iDRAC, Lenovo XCC, Supermicro und OpenBMC liefern
// dieselben DMTF-Felder.
//
// Endpunkte (geprüft gegen die iLO-5-Resource-Map v3.19 und das DMTF-Schema):
// /redfish/v1/Systems/{id}, Chassis/{id}/Thermal, Chassis/{id}/Power,
// Managers/{id}, Systems/{id}/Storage bzw. Systems/{id}/SmartStorage/
// ArrayControllers/{id}/DiskDrives|LogicalDrives.
package redfish

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
)

const Type = "redfish"

const (
	defaultTempMargin  = 10.0 // Warnung so viele °C unter der kritischen Schwelle
	defaultWearoutWarn = 20.0 // Warnung bei Restlebensdauer unter %
)

type Integration struct{}

func New() *Integration { return &Integration{} }

func (*Integration) Type() string { return Type }

func (*Integration) Schema() integrations.ConfigSchema {
	type T = integrations.Text
	return integrations.ConfigSchema{
		Title: T{DE: "Server-Management (Redfish / iLO)", EN: "Server management (Redfish / iLO)"},
		Description: T{
			DE: "Hardware-Zustand über den BMC: Netzteile, Lüfter, Temperaturen, Speicher, Laufwerke – HPE iLO, Dell iDRAC, Lenovo XCC, Supermicro.",
			EN: "Hardware health via the BMC: power supplies, fans, temperatures, memory, drives – HPE iLO, Dell iDRAC, Lenovo XCC, Supermicro.",
		},
		Icon: "server-cog",
		Docs: "https://servermanagementportal.ext.hpe.com/docs/redfishservices/ilos/ilo5/",
		Fields: []integrations.Field{
			{Key: "base_url", Type: integrations.FieldURL, Required: true,
				Label: T{DE: "Adresse des BMC", EN: "BMC address"},
				Help: T{
					DE: "Adresse der iLO- bzw. iDRAC-Oberfläche. Das Zertifikat ist meist selbst signiert – dann unter „Erweitert“ die TLS-Prüfung ausschalten.",
					EN: "Address of the iLO or iDRAC web interface. The certificate is usually self-signed – then turn off TLS verification under \"Advanced\".",
				},
				Placeholder: "https://ilo.lan"},
			{Key: "username", Type: integrations.FieldString, Required: true, Half: true,
				Label: T{DE: "Benutzer", EN: "User"},
				Help: T{
					DE: "Eigener Benutzer nur mit dem Recht „Login“ (iLO) bzw. „ReadOnly“ (iDRAC) genügt.",
					EN: "A dedicated user with only the \"Login\" (iLO) or \"ReadOnly\" (iDRAC) privilege is enough.",
				}},
			{Key: "password", Type: integrations.FieldSecret, Required: true, Half: true,
				Label: T{DE: "Passwort", EN: "Password"}},
			{Key: "drives", Type: integrations.FieldBool, Default: true,
				Label: T{DE: "Laufwerke abfragen", EN: "Query drives"},
				Help: T{
					DE: "Zustand und Restlebensdauer je Laufwerk und logischem Laufwerk (RAID). Kostet einige Anfragen mehr.",
					EN: "Health and remaining life per drive and logical drive (RAID). Costs a few more requests.",
				}},
			{Key: "temp_margin", Type: integrations.FieldNumber, Default: defaultTempMargin, Half: true,
				Min: integrations.Ptr(0), Max: integrations.Ptr(50),
				Label: T{DE: "Temperatur: Warnung … °C vor kritisch", EN: "Temperature: warn … °C before critical"}},
			{Key: "wearout_warn", Type: integrations.FieldNumber, Default: defaultWearoutWarn, Half: true,
				Min: integrations.Ptr(0), Max: integrations.Ptr(100),
				Label: T{DE: "SSD: Warnung bei Restlebensdauer unter %", EN: "SSD: warn below remaining life %"}},
			{Key: "power_off_warn", Type: integrations.FieldBool, Default: true,
				Label: T{DE: "Ausgeschalteter Server ist eine Warnung", EN: "Powered-off server is a warning"}},
		},
	}
}

// --- Redfish-Ressourcen (nur benötigte Felder) ------------------------------

type status struct {
	State        string `json:"State"`
	Health       string `json:"Health"`
	HealthRollup string `json:"HealthRollup"`
}

type system struct {
	ID           string `json:"Id"`
	Name         string `json:"Name"`
	HostName     string `json:"HostName"`
	Manufacturer string `json:"Manufacturer"`
	Model        string `json:"Model"`
	SerialNumber string `json:"SerialNumber"`
	BiosVersion  string `json:"BiosVersion"`
	PowerState   string `json:"PowerState"`
	Status       status `json:"Status"`

	MemorySummary struct {
		TotalSystemMemoryGiB float64 `json:"TotalSystemMemoryGiB"`
		Status               status  `json:"Status"`
	} `json:"MemorySummary"`
	ProcessorSummary struct {
		Count  int    `json:"Count"`
		Model  string `json:"Model"`
		Status status `json:"Status"`
	} `json:"ProcessorSummary"`
	Links struct {
		Chassis   []ref `json:"Chassis"`
		ManagedBy []ref `json:"ManagedBy"`
	} `json:"Links"`
	Storage *ref `json:"Storage"`
	Oem     struct {
		Hpe *struct {
			PostState             string                     `json:"PostState"`
			AggregateHealthStatus map[string]json.RawMessage `json:"AggregateHealthStatus"`
		} `json:"Hpe"`
	} `json:"Oem"`
}

type chassis struct {
	Thermal *ref `json:"Thermal"`
	Power   *ref `json:"Power"`
}

type thermal struct {
	Temperatures []struct {
		MemberID               string   `json:"MemberId"`
		Name                   string   `json:"Name"`
		ReadingCelsius         *float64 `json:"ReadingCelsius"`
		UpperThresholdCritical *float64 `json:"UpperThresholdCritical"`
		UpperThresholdFatal    *float64 `json:"UpperThresholdFatal"`
		PhysicalContext        string   `json:"PhysicalContext"`
		Status                 status   `json:"Status"`
	} `json:"Temperatures"`
	Fans []struct {
		MemberID     string   `json:"MemberId"`
		Name         string   `json:"Name"`
		FanName      string   `json:"FanName"` // ältere Schemata
		Reading      *float64 `json:"Reading"`
		ReadingUnits string   `json:"ReadingUnits"`
		Status       status   `json:"Status"`
	} `json:"Fans"`
}

type power struct {
	PowerControl []struct {
		PowerConsumedWatts *float64 `json:"PowerConsumedWatts"`
	} `json:"PowerControl"`
	PowerSupplies []struct {
		MemberID             string   `json:"MemberId"`
		Name                 string   `json:"Name"`
		Model                string   `json:"Model"`
		PowerCapacityWatts   *float64 `json:"PowerCapacityWatts"`
		LastPowerOutputWatts *float64 `json:"LastPowerOutputWatts"`
		Status               status   `json:"Status"`
	} `json:"PowerSupplies"`
}

type manager struct {
	Model           string `json:"Model"`
	FirmwareVersion string `json:"FirmwareVersion"`
}

// --- Abfrage ----------------------------------------------------------------

type data struct {
	sys     system
	thermal *thermal
	power   *power
	manager *manager
	storage []item // Controller, Laufwerke, logische Laufwerke
	// storageOK: Laufwerke vollständig gelesen (ersetzt den Sammelzustand Storage)
	storageOK bool
}

func (i *Integration) Test(ctx context.Context, cfg integrations.Config) error {
	c, err := newClient(cfg)
	if err != nil {
		return err
	}
	path, err := c.first(ctx, "/redfish/v1/Systems")
	if err != nil {
		return err
	}
	var s system
	return c.get(ctx, path, &s)
}

// optional ignoriert Fehler einzelner Teile (best effort) – außer
// Anmeldefehlern, die gelten für alles.
func optional(err error) error {
	if err != nil && i18n.FromError(err).Code == "integration.unauthorized" {
		return err
	}
	return nil
}

func (i *Integration) fetch(ctx context.Context, cfg integrations.Config) (*data, error) {
	c, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	path, err := c.first(ctx, "/redfish/v1/Systems")
	if err != nil {
		return nil, err
	}
	d := &data{}
	if err := c.get(ctx, path, &d.sys); err != nil {
		return nil, err
	}

	// Gehäuse, BMC und Laufwerke parallel – BMCs antworten oft langsam
	var wg sync.WaitGroup
	errs := make([]error, 3)
	wg.Go(func() {
		chPath := ""
		if len(d.sys.Links.Chassis) > 0 {
			chPath = d.sys.Links.Chassis[0].ID
		} else if chPath, errs[0] = c.first(ctx, "/redfish/v1/Chassis"); errs[0] != nil {
			return
		}
		var ch chassis
		if errs[0] = c.get(ctx, chPath, &ch); errs[0] != nil {
			return
		}
		thPath, pwPath := strings.TrimRight(chPath, "/")+"/Thermal", strings.TrimRight(chPath, "/")+"/Power"
		if ch.Thermal != nil {
			thPath = ch.Thermal.ID
		}
		if ch.Power != nil {
			pwPath = ch.Power.ID
		}
		var th thermal
		if err := c.get(ctx, thPath, &th); err == nil {
			d.thermal = &th
		} else if errs[0] = optional(err); errs[0] != nil {
			return
		}
		var pw power
		if err := c.get(ctx, pwPath, &pw); err == nil {
			d.power = &pw
		} else {
			errs[0] = optional(err)
		}
	})
	wg.Go(func() {
		if len(d.sys.Links.ManagedBy) == 0 {
			return
		}
		var m manager
		if err := c.get(ctx, d.sys.Links.ManagedBy[0].ID, &m); err == nil {
			d.manager = &m
		} else {
			errs[1] = optional(err)
		}
	})
	if cfg.Bool("drives") || !cfg.Has("drives") {
		wg.Go(func() { errs[2] = optional(fetchStorage(ctx, c, path, d)) })
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return d, nil
}

func (i *Integration) Collect(ctx context.Context, cfg integrations.Config) (integrations.Snapshot, error) {
	d, err := i.fetch(ctx, cfg)
	if err != nil {
		return integrations.Snapshot{}, err
	}
	margin, wearWarn := defaultTempMargin, defaultWearoutWarn
	if cfg.Has("temp_margin") {
		margin = cfg.Float("temp_margin")
	}
	if cfg.Has("wearout_warn") {
		wearWarn = cfg.Float("wearout_warn")
	}
	powerOffWarn := !cfg.Has("power_off_warn") || cfg.Bool("power_off_warn")
	return snapshot(d, margin, wearWarn, powerOffWarn), nil
}

// --- Auswertung ---------------------------------------------------------------

// health übersetzt Redfish-Health; ok=false: Komponente fehlt bzw. ist aus.
func health(s status) (integrations.Status, bool) {
	switch s.State {
	case "Absent", "Disabled", "UnavailableOffline":
		return "", false
	}
	return healthOf(cmp.Or(s.HealthRollup, s.Health))
}

func healthOf(h string) (integrations.Status, bool) {
	switch strings.ToLower(h) {
	case "ok":
		return integrations.StatusOK, true
	case "warning":
		return integrations.StatusWarning, true
	case "critical":
		return integrations.StatusError, true
	}
	return integrations.StatusUnknown, h != ""
}

func okfail(st integrations.Status) float64 {
	if st == integrations.StatusOK {
		return 1
	}
	return 0
}

// Sammelzustände von HPE (Oem.Hpe.AggregateHealthStatus) → eigener Schlüssel.
var hpeComponents = []struct{ hpe, key string }{
	{"PowerSupplies", "power_supplies"},
	{"PowerSupplyRedundancy", "psu_redundancy"},
	{"Fans", "fans"},
	{"FanRedundancy", "fan_redundancy"},
	{"Temperatures", "temperatures"},
	{"Processors", "processors"},
	{"Memory", "memory"},
	{"Storage", "storage"},
	{"SmartStorageBattery", "smart_storage_battery"},
	{"Network", "network"},
	{"BiosOrHardwareHealth", "bios_hardware"},
	{"LiquidCooling", "liquid_cooling"},
	{"LiquidCoolingRedundancy", "liquid_cooling_redundancy"},
}

// hpeHealth deutet einen Eintrag: {"Status":{"Health":"OK"}} oder ein Text
// wie "Redundant" / "FailedRedundant".
func hpeHealth(raw json.RawMessage) (integrations.Status, bool) {
	var obj struct {
		Status status `json:"Status"`
	}
	if json.Unmarshal(raw, &obj) == nil && obj.Status.Health != "" {
		return healthOf(obj.Status.Health)
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return "", false
	}
	switch {
	case s == "Redundant", s == "NonRedundant": // nicht redundant gebaut ist kein Fehler
		return integrations.StatusOK, true
	case strings.HasPrefix(s, "Failed"), strings.HasPrefix(s, "Degraded"), s == "Lost":
		return integrations.StatusWarning, true
	}
	return "", false
}

func snapshot(d *data, margin, wearWarn float64, powerOffWarn bool) integrations.Snapshot {
	ok := integrations.StatusOK
	s := d.sys
	res := integrations.Resource{ID: "server", Kind: integrations.KindServer,
		Name:  cmp.Or(s.HostName, strings.TrimSpace(s.Model), s.Name, "Server"),
		State: strings.ToLower(cmp.Or(s.PowerState, "unknown")), Info: map[string]string{}}
	for k, v := range map[string]string{"model": strings.TrimSpace(s.Model), "manufacturer": s.Manufacturer,
		"serial": strings.TrimSpace(s.SerialNumber), "bios": s.BiosVersion} {
		if v != "" {
			res.Info[k] = v
		}
	}
	if s.ProcessorSummary.Count > 0 {
		res.Info["cpus"] = strconv.Itoa(s.ProcessorSummary.Count)
		if m := strings.Join(strings.Fields(s.ProcessorSummary.Model), " "); m != "" {
			res.Info["cpu_model"] = m
		}
	}
	if s.MemorySummary.TotalSystemMemoryGiB > 0 {
		res.Info["memory_gib"] = strconv.FormatFloat(s.MemorySummary.TotalSystemMemoryGiB, 'f', -1, 64)
	}
	if s.Oem.Hpe != nil && s.Oem.Hpe.PostState != "" {
		res.Info["post"] = s.Oem.Hpe.PostState
	}
	if d.manager != nil {
		if d.manager.FirmwareVersion != "" {
			res.Info["bmc_firmware"] = d.manager.FirmwareVersion
		}
		if d.manager.Model != "" {
			res.Info["bmc"] = d.manager.Model
		}
	}

	var main, detail []integrations.Metric
	add := func(m integrations.Metric) {
		m.Resource = res.ID
		detail = append(detail, m)
	}
	var statuses []integrations.Status

	// Temperaturen: Status der Quelle, zusätzlich Abstand zur kritischen Schwelle
	if d.thermal != nil {
		for _, t := range d.thermal.Temperatures {
			st, present := health(t.Status)
			if !present || t.ReadingCelsius == nil {
				continue
			}
			v := *t.ReadingCelsius
			if crit := t.UpperThresholdCritical; crit != nil && *crit > 0 {
				res.Info["temp_crit:"+t.Name] = strconv.FormatFloat(*crit, 'f', -1, 64)
				switch {
				case v >= *crit:
					st = integrations.Worst(st, integrations.StatusError)
				case v >= *crit-margin:
					st = integrations.Worst(st, integrations.StatusWarning)
				}
			}
			name := cmp.Or(t.Name, t.MemberID)
			add(integrations.Metric{Key: "temp/" + name + "/temp", Name: "temp", Group: name, Value: v, Unit: "°C", Status: st})
			statuses = append(statuses, st)
			if _, done := res.Info["inlet"]; !done && (t.PhysicalContext == "Intake" || strings.Contains(strings.ToLower(name), "inlet")) {
				res.Info["inlet"] = name
				main = append(main, integrations.Metric{Key: "inlet", Name: "inlet", Value: v, Unit: "°C", Status: st})
			}
		}
		for _, f := range d.thermal.Fans {
			st, present := health(f.Status)
			if !present {
				continue
			}
			name := cmp.Or(f.Name, f.FanName, f.MemberID)
			m := integrations.Metric{Key: "fan/" + name + "/fan", Name: "fan", Group: name, Status: st}
			if f.Reading != nil && strings.EqualFold(cmp.Or(f.ReadingUnits, "Percent"), "Percent") {
				m.Value, m.Unit = *f.Reading, "%"
			} else if f.Reading != nil {
				m.Value, m.Unit = *f.Reading, "RPM"
			} else {
				m.Unit, m.Value = "okfail", okfail(st)
			}
			add(m)
			statuses = append(statuses, st)
		}
	}
	if d.power != nil {
		for _, pc := range d.power.PowerControl {
			if pc.PowerConsumedWatts != nil {
				main = append([]integrations.Metric{{Key: "power", Name: "power", Value: *pc.PowerConsumedWatts, Unit: "W", Status: ok}}, main...)
				break
			}
		}
		for n, p := range d.power.PowerSupplies {
			st, present := health(p.Status)
			if !present {
				continue
			}
			id := cmp.Or(p.MemberID, strconv.Itoa(n))
			name := fmt.Sprintf("PSU %d", n+1)
			if p.Name != "" && !strings.HasPrefix(p.Name, "Hpe") { // iLO nennt alle „HpeServerPowerSupply“
				name = p.Name
			}
			add(integrations.Metric{Key: "psu/" + id + "/supply", Name: "supply", Group: name, Value: okfail(st), Unit: "okfail", Status: st})
			if p.LastPowerOutputWatts != nil {
				add(integrations.Metric{Key: "psu/" + id + "/output", Name: "output", Group: name, Value: *p.LastPowerOutputWatts, Unit: "W", Status: ok})
			}
			statuses = append(statuses, st)
		}
	}
	for _, it := range d.storage {
		kind, _, _ := strings.Cut(it.key, "/") // drive, volume, controller
		add(integrations.Metric{Key: it.key + "/" + kind, Name: kind, Group: it.label, Value: okfail(it.state), Unit: "okfail", Status: it.state})
		statuses = append(statuses, it.state)
		if it.life != nil {
			st := ok
			if *it.life < wearWarn {
				st = integrations.StatusWarning
			}
			add(integrations.Metric{Key: it.key + "/life", Name: "life", Group: it.label, Value: *it.life, Unit: "%", Status: st})
			statuses = append(statuses, st)
		}
	}

	// Sammelzustände je Komponente. Wo es Einzelwerte gibt (Lüfter, Netzteile,
	// Temperaturen, Laufwerke), zählen nur diese als Problem (Derived).
	covered := map[string]bool{
		"temperatures":   d.thermal != nil && len(d.thermal.Temperatures) > 0,
		"fans":           d.thermal != nil && len(d.thermal.Fans) > 0,
		"power_supplies": d.power != nil && len(d.power.PowerSupplies) > 0,
		"storage":        d.storageOK,
	}
	component := func(key string, st integrations.Status) {
		add(integrations.Metric{Key: "component/" + key, Name: key, Value: okfail(st),
			Unit: "okfail", Status: st, Derived: covered[key]})
		if !covered[key] {
			statuses = append(statuses, st)
		}
	}
	if s.Oem.Hpe != nil && len(s.Oem.Hpe.AggregateHealthStatus) > 0 {
		for _, c := range hpeComponents {
			if raw, found := s.Oem.Hpe.AggregateHealthStatus[c.hpe]; found {
				if st, present := hpeHealth(raw); present {
					component(c.key, st)
				}
			}
		}
	} else {
		if st, present := health(s.ProcessorSummary.Status); present {
			component("processors", st)
		}
		if st, present := health(s.MemorySummary.Status); present {
			component("memory", st)
		}
		// ohne Sammelzustände: aus den Einzelwerten
		for _, c := range []struct{ key, prefix string }{{"temperatures", "temp/"}, {"fans", "fan/"}, {"power_supplies", "psu/"}} {
			if covered[c.key] {
				component(c.key, worstOf(detail, c.prefix))
			}
		}
	}

	// Gesamtzustand: Rollup des Systems, aber nie besser als die Teile
	rollup, present := health(s.Status)
	if !present {
		rollup = ok
	}
	rollup = integrations.Worst(append(statuses, rollup)...)
	main = append(main, integrations.Metric{Key: "health", Name: "health", Value: okfail(rollup), Unit: "okfail",
		Status: rollup, Derived: true})

	res.Status = integrations.Worst(append(statuses, rollup)...)
	if res.State != "on" && powerOffWarn {
		res.Status = integrations.Worst(res.Status, integrations.StatusWarning)
		main = append(main, integrations.Metric{Key: "power_state", Name: "power_state", Value: 0, Unit: "okfail",
			Status: integrations.StatusWarning})
	}

	snap := integrations.Snapshot{Metrics: append(main, detail...), Resources: []integrations.Resource{res},
		Info: map[string]string{}}
	if v := res.Info["model"]; v != "" {
		snap.Info["version"] = v
	}
	if v := res.Info["bmc_firmware"]; v != "" {
		// iLO meldet „iLO 5 v3.19“, andere BMCs nur die Nummer
		if bmc := cmp.Or(res.Info["bmc"], "BMC"); !strings.HasPrefix(v, bmc) {
			v = bmc + " " + v
		}
		snap.Info["firmware"] = v
	}
	return snap
}

// worstOf: schlechtester Status der Kennzahlen mit Präfix.
func worstOf(list []integrations.Metric, prefix string) integrations.Status {
	var out []integrations.Status
	for _, m := range list {
		if strings.HasPrefix(m.Key, prefix) {
			out = append(out, m.Status)
		}
	}
	return integrations.Worst(out...)
}
