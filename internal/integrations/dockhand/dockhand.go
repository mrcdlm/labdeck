// Package dockhand liest Docker-Hosts und Container aus Dockhand: je Umgebung
// (Docker-Host) CPU, RAM, Uptime und ausstehende Image-Updates, je Container
// Status, Health, CPU, RAM und Netzwerk.
//
// API (geprüft gegen den Quellcode von Dockhand, @openapi-Kommentare der
// Routen): GET /api/environments, /api/dashboard/stats?env=, /api/host?env=,
// /api/containers?env=, /api/containers/stats?env=. Anmeldung per API-Token
// („Authorization: Bearer dh_…“); Leserechte genügen.
package dockhand

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
)

const Type = "dockhand"

const (
	defaultWarnPct  = 80
	defaultErrorPct = 90
)

type Integration struct{}

func New() *Integration { return &Integration{} }

func (*Integration) Type() string { return Type }

func (*Integration) Schema() integrations.ConfigSchema {
	type T = integrations.Text
	return integrations.ConfigSchema{
		Title: T{DE: "Dockhand", EN: "Dockhand"},
		Description: T{
			DE: "Docker-Hosts und Container aus Dockhand: CPU, RAM, Health und ausstehende Image-Updates.",
			EN: "Docker hosts and containers from Dockhand: CPU, RAM, health and pending image updates.",
		},
		Icon:     "container",
		Category: integrations.CategoryCompute,
		Docs:     "https://github.com/Finsys/dockhand",
		Fields: []integrations.Field{
			{Key: "base_url", Type: integrations.FieldURL, Required: true, Default: "http://dockhand.lan:3000",
				Label:       T{DE: "Adresse", EN: "Address"},
				Help:        T{DE: "Adresse der Dockhand-Oberfläche.", EN: "Address of the Dockhand web interface."},
				Placeholder: "http://dockhand.lan:3000"},
			{Key: "api_token", Type: integrations.FieldSecret,
				Label: T{DE: "API-Token", EN: "API token"},
				Help: T{
					DE: "Nur nötig, wenn in Dockhand die Anmeldung aktiv ist: in Dockhand ein API-Token anlegen (beginnt mit dh_). Leserechte genügen.",
					EN: "Only needed if authentication is enabled in Dockhand: create an API token there (starts with dh_). Read permissions are enough.",
				}},
			{Key: "environments", Type: integrations.FieldString,
				Label:       T{DE: "Umgebungen", EN: "Environments"},
				Help:        T{DE: "Namen der Docker-Hosts in Dockhand, komma-getrennt; leer = alle.", EN: "Names of the Docker hosts in Dockhand, comma-separated; empty = all."},
				Placeholder: "docker01, pi-edge"},
			{Key: "warn_pct", Type: integrations.FieldNumber, Default: float64(defaultWarnPct), Half: true,
				Min: integrations.Ptr(1), Max: integrations.Ptr(100),
				Label: T{DE: "Warnung ab %", EN: "Warning at %"},
				Help:  T{DE: "CPU und RAM der Hosts.", EN: "CPU and RAM of the hosts."}},
			{Key: "error_pct", Type: integrations.FieldNumber, Default: float64(defaultErrorPct), Half: true,
				Min: integrations.Ptr(1), Max: integrations.Ptr(100),
				Label: T{DE: "Fehler ab %", EN: "Error at %"}},
			{Key: "container_stats", Type: integrations.FieldBool, Default: true,
				Label: T{DE: "CPU und RAM je Container", EN: "CPU and RAM per container"},
				Help: T{
					DE: "Dockhand misst dafür alle laufenden Container (dauert etwa 2 Sekunden je Abfrage).",
					EN: "Dockhand samples all running containers for this (takes about 2 seconds per poll).",
				}},
		},
	}
}

func header(cfg integrations.Config) http.Header {
	h := http.Header{}
	if t := strings.TrimSpace(cfg.Secret("api_token")); t != "" {
		h.Set("Authorization", "Bearer "+t)
	}
	return h
}

func get(ctx context.Context, cfg integrations.Config, path string, out any) error {
	return integrations.GetJSON(ctx, cfg.HTTP, cfg.BaseURL("base_url")+"/api"+path, header(cfg), out)
}

type env struct {
	ID   *int   `json:"id"`
	Name string `json:"name"`
}

// environments liefert die (gewählten) Umgebungen.
func environments(ctx context.Context, cfg integrations.Config) ([]env, error) {
	var all []env
	if err := get(ctx, cfg, "/environments", &all); err != nil {
		if m := i18n.FromError(err); m.Code == "integration.bad_json" || m.Code == "integration.not_found" {
			return nil, i18n.E("dockhand.unexpected")
		}
		return nil, err
	}
	var want []string
	for _, n := range strings.Split(cfg.String("environments"), ",") {
		if n = strings.TrimSpace(n); n != "" {
			want = append(want, n)
		}
	}
	var out []env
	for _, e := range all {
		if e.ID == nil {
			return nil, i18n.E("dockhand.unexpected")
		}
		if len(want) == 0 || slices.Contains(want, e.Name) {
			out = append(out, e)
		}
	}
	if len(out) == 0 {
		if len(want) > 0 {
			return nil, i18n.E("dockhand.env_missing", "names", strings.Join(want, ", "))
		}
		return nil, i18n.E("dockhand.no_envs")
	}
	return out, nil
}

func (*Integration) Test(ctx context.Context, cfg integrations.Config) error {
	_, err := environments(ctx, cfg)
	return err
}

type dashboard struct {
	Online     *bool `json:"online"`
	Containers struct {
		Total, Running, Stopped, Unhealthy, PendingUpdates int
	} `json:"containers"`
	Metrics *struct {
		CPUPercent  float64 `json:"cpuPercent"`
		MemoryUsed  int64   `json:"memoryUsed"`
		MemoryTotal int64   `json:"memoryTotal"`
	} `json:"metrics"`
}

type host struct {
	Hostname      string `json:"hostname"`
	CPUs          float64
	TotalMemory   int64  `json:"totalMemory"`
	FreeMemory    int64  `json:"freeMemory"`
	Uptime        int64  `json:"uptime"`
	DockerVersion string `json:"dockerVersion"`
}

type container struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Image  string `json:"image"`
	State  string `json:"state"`
	Status string `json:"status"`
	Health string `json:"health"`
}

type stats struct {
	ID          string  `json:"id"`
	CPUPercent  float64 `json:"cpuPercent"`
	MemoryUsage int64   `json:"memoryUsage"`
	MemoryLimit int64   `json:"memoryLimit"`
	NetworkRx   int64   `json:"networkRx"`
	NetworkTx   int64   `json:"networkTx"`
}

func ptr[T any](v T) *T { return &v }

// health: Feld health, sonst aus dem Status („Up 2 hours (healthy)“).
func health(c container) string {
	if c.Health != "" {
		return c.Health
	}
	for _, h := range []string{"unhealthy", "healthy", "starting"} {
		if strings.Contains(c.Status, "("+h+")") {
			return h
		}
	}
	return ""
}

func (*Integration) Collect(ctx context.Context, cfg integrations.Config) (integrations.Snapshot, error) {
	envs, err := environments(ctx, cfg)
	if err != nil {
		return integrations.Snapshot{}, err
	}
	warn, fail := float64(defaultWarnPct), float64(defaultErrorPct)
	if cfg.Has("warn_pct") {
		warn = cfg.Float("warn_pct")
	}
	if cfg.Has("error_pct") {
		fail = cfg.Float("error_pct")
	}
	spec := integrations.ValueSpec{Warn: &warn, Fail: &fail}
	withStats := !cfg.Has("container_stats") || cfg.Bool("container_stats")
	ok := integrations.StatusOK

	var snap integrations.Snapshot
	var running, total, updates int
	var cpuSum float64
	var cpuN int
	var memUsed, memTotal int64
	versions := map[string]bool{}

	for _, e := range envs {
		id := strconv.Itoa(*e.ID)
		var d dashboard
		if err := get(ctx, cfg, "/dashboard/stats?env="+id, &d); err != nil {
			return integrations.Snapshot{}, err
		}
		node := integrations.Resource{ID: "env/" + id, Name: e.Name, Kind: integrations.KindNode, State: "online", Status: ok}
		if d.Online != nil && !*d.Online {
			node.State, node.Status = "offline", integrations.StatusError
			snap.Resources = append(snap.Resources, node)
			snap.Metrics = append(snap.Metrics, integrations.Metric{Key: "res/" + node.ID + "/online", Name: "online",
				Group: e.Name, Unit: "okfail", Resource: node.ID, Status: integrations.StatusError})
			continue
		}
		updates += d.Containers.PendingUpdates

		// Host: Kerne, Uptime, Docker-Version (optional – ohne Recht nur Dashboard-Werte)
		var h host
		if get(ctx, cfg, "/host?env="+id, &h) == nil {
			node.CPUs, node.Uptime = h.CPUs, h.Uptime
			node.Info = map[string]string{"hostname": h.Hostname, "docker": h.DockerVersion}
			if h.DockerVersion != "" {
				versions["Docker "+h.DockerVersion] = true
			}
		}
		add := func(name string, v float64) {
			st := spec.Status(v)
			node.Status = integrations.Worst(node.Status, st)
			snap.Metrics = append(snap.Metrics, integrations.Metric{Key: "res/" + node.ID + "/" + name, Name: name,
				Group: e.Name, Value: v, Unit: "%", Status: st, Resource: node.ID})
		}
		switch {
		case d.Metrics != nil:
			node.CPU = ptr(d.Metrics.CPUPercent)
			add("cpu", d.Metrics.CPUPercent)
			cpuSum += d.Metrics.CPUPercent
			cpuN++
			if d.Metrics.MemoryTotal > 0 {
				node.MemUsed, node.MemTotal = ptr(d.Metrics.MemoryUsed), ptr(d.Metrics.MemoryTotal)
			}
		case h.TotalMemory > 0: // Metriken in Dockhand aus: RAM aus den Host-Daten
			node.MemUsed, node.MemTotal = ptr(h.TotalMemory-h.FreeMemory), ptr(h.TotalMemory)
		}
		if node.MemTotal != nil {
			memUsed += *node.MemUsed
			memTotal += *node.MemTotal
			add("mem_pct", float64(*node.MemUsed)/float64(*node.MemTotal)*100)
		}
		snap.Resources = append(snap.Resources, node)

		// Container
		var list []container
		if err := get(ctx, cfg, "/containers?env="+id, &list); err != nil {
			return integrations.Snapshot{}, err
		}
		byID := map[string]stats{}
		if withStats {
			var st []stats
			if get(ctx, cfg, "/containers/stats?env="+id, &st) == nil {
				for _, s := range st {
					byID[s.ID] = s
				}
			}
		}
		for _, c := range list {
			total++
			r := integrations.Resource{ID: "container/" + id + "/" + c.Name, Name: c.Name, Kind: integrations.KindContainer,
				Parent: e.Name, State: c.State, Info: map[string]string{"image": c.Image, "uptime": c.Status}}
			hl := health(c)
			switch {
			case c.State == "running" && hl == "unhealthy":
				r.State, r.Status = "unhealthy", integrations.StatusError
				running++
			case c.State == "running":
				r.Status = ok
				if hl == "healthy" {
					r.State = "healthy"
				}
				running++
			case c.State == "restarting":
				r.Status = integrations.StatusWarning
			default: // exited, paused, created, dead
				r.Status = integrations.StatusPaused
			}
			if s, found := byID[c.ID]; found {
				r.CPU = ptr(s.CPUPercent)
				r.MemUsed, r.NetIn, r.NetOut = ptr(s.MemoryUsage), ptr(s.NetworkRx), ptr(s.NetworkTx)
				if s.MemoryLimit > 0 {
					r.MemTotal = ptr(s.MemoryLimit)
				}
				snap.Metrics = append(snap.Metrics,
					integrations.Metric{Key: "res/" + r.ID + "/cpu", Name: "cpu", Group: c.Name, Value: s.CPUPercent,
						Unit: "%", Status: ok, Resource: r.ID},
					integrations.Metric{Key: "res/" + r.ID + "/mem", Name: "mem", Group: c.Name, Value: float64(s.MemoryUsage),
						Unit: "bytes", Status: ok, Resource: r.ID})
			}
			// „unhealthy“ als Kennzahl: erscheint im Gesamtstatus und in Benachrichtigungen
			if r.Status == integrations.StatusError {
				snap.Metrics = append(snap.Metrics, integrations.Metric{Key: "res/" + r.ID + "/container_health",
					Name: "container_health", Group: c.Name + " · " + e.Name, Unit: "okfail",
					Status: integrations.StatusError, Resource: r.ID})
			}
			snap.Resources = append(snap.Resources, r)
		}
	}

	main := []integrations.Metric{
		{Key: "containers_running", Name: "containers_running", Value: float64(running), Status: ok},
	}
	if cpuN > 0 {
		main = append(main, integrations.Metric{Key: "cpu", Name: "cpu", Value: cpuSum / float64(cpuN), Unit: "%", Status: ok})
	}
	if memTotal > 0 {
		main = append(main, integrations.Metric{Key: "mem_pct", Name: "mem_pct", Value: float64(memUsed) / float64(memTotal) * 100,
			Unit: "%", Status: ok})
	}
	main = append(main,
		integrations.Metric{Key: "updates", Name: "updates", Value: float64(updates), Status: ok},
		integrations.Metric{Key: "containers", Name: "containers", Value: float64(total), Status: ok},
		integrations.Metric{Key: "hosts", Name: "hosts", Value: float64(len(envs)), Status: ok},
	)
	snap.Metrics = append(main, snap.Metrics...)
	if len(versions) > 0 {
		vs := make([]string, 0, len(versions))
		for v := range versions {
			vs = append(vs, v)
		}
		slices.Sort(vs)
		snap.Info = map[string]string{"version": strings.Join(vs, ", ")}
	}
	return snap, nil
}
