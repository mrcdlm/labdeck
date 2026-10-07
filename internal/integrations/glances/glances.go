// Package glances liest Auslastung eines Hosts und seiner Docker-Container
// über die REST-API von Glances (v4): CPU, RAM, Load, Dateisysteme und je
// Container Status, CPU, RAM und Netzwerk.
//
// API (geprüft gegen die Glances-Doku und den Quellcode des Docker-Plugins):
// GET /api/4/{cpu,mem,load,system,uptime,fs,containers}
package glances

import (
	"context"
	"encoding/base64"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
)

const Type = "glances"

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
		Title: T{DE: "Glances", EN: "Glances"},
		Description: T{
			DE: "CPU, RAM, Load und Platten eines Hosts sowie je Docker-Container Status, CPU, RAM und Netzwerk.",
			EN: "CPU, RAM, load and disks of a host plus status, CPU, RAM and network per Docker container.",
		},
		Icon: "gauge",
		Docs: "https://glances.readthedocs.io/en/latest/api/restful.html",
		Fields: []integrations.Field{
			{Key: "base_url", Type: integrations.FieldURL, Required: true, Default: "http://docker.lan:61208",
				Label: T{DE: "Adresse", EN: "Address"},
				Help: T{
					DE: "Glances im Webserver-Modus (glances -w, Port 61208), z. B. als Container nicolargo/glances.",
					EN: "Glances in web server mode (glances -w, port 61208), e.g. as container nicolargo/glances.",
				},
				Placeholder: "http://docker.lan:61208"},
			{Key: "username", Type: integrations.FieldString, Half: true,
				Label: T{DE: "Benutzer (optional)", EN: "User (optional)"},
				Help:  T{DE: "Nur bei glances --password.", EN: "Only with glances --password."}},
			{Key: "password", Type: integrations.FieldSecret,
				Label: T{DE: "Passwort (optional)", EN: "Password (optional)"}},
			{Key: "warn_pct", Type: integrations.FieldNumber, Default: float64(defaultWarnPct), Half: true,
				Min: integrations.Ptr(1), Max: integrations.Ptr(100),
				Label: T{DE: "Warnung ab %", EN: "Warning at %"},
				Help:  T{DE: "CPU, RAM und Dateisysteme des Hosts.", EN: "CPU, RAM and filesystems of the host."}},
			{Key: "error_pct", Type: integrations.FieldNumber, Default: float64(defaultErrorPct), Half: true,
				Min: integrations.Ptr(1), Max: integrations.Ptr(100),
				Label: T{DE: "Fehler ab %", EN: "Error at %"}},
			{Key: "containers", Type: integrations.FieldBool, Default: true,
				Label: T{DE: "Docker-Container anzeigen", EN: "Show Docker containers"},
				Help: T{
					DE: "„unhealthy“ gilt als Fehler, gestoppte Container werden grau angezeigt.",
					EN: "\"unhealthy\" counts as error, stopped containers are shown in grey.",
				}},
		},
	}
}

func header(cfg integrations.Config) http.Header {
	h := http.Header{}
	if u := strings.TrimSpace(cfg.String("username")); u != "" {
		h.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(u+":"+cfg.Secret("password"))))
	}
	return h
}

func get(ctx context.Context, cfg integrations.Config, path string, out any) error {
	return integrations.GetJSON(ctx, cfg.HTTP, cfg.BaseURL("base_url")+"/api/4"+path, header(cfg), out)
}

type cpu struct {
	Total   *float64 `json:"total"`
	CPUCore float64  `json:"cpucore"`
}

func (*Integration) Test(ctx context.Context, cfg integrations.Config) error {
	var c cpu
	if err := get(ctx, cfg, "/cpu", &c); err != nil {
		if i18n.FromError(err).Code == "integration.not_found" {
			return i18n.E("glances.no_v4")
		}
		return err
	}
	if c.Total == nil {
		return i18n.E("glances.unexpected")
	}
	return nil
}

var uptimeRe = regexp.MustCompile(`^(?:(\d+) days?, )?(\d+):(\d+):(\d+)`)

// parseUptime: "1:46:07" bzw. "12 days, 3:04:05" → Sekunden.
func parseUptime(s string) int64 {
	m := uptimeRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0
	}
	n := func(i int) int64 { v, _ := strconv.ParseInt(m[i], 10, 64); return v }
	return n(1)*86400 + n(2)*3600 + n(3)*60 + n(4)
}

func ptr[T any](v T) *T { return &v }

// Dateisysteme, die nichts über Plattenplatz aussagen.
var pseudoFS = []string{"overlay", "tmpfs", "squashfs", "devtmpfs", "nsfs", "proc", "sysfs"}

func (i *Integration) Collect(ctx context.Context, cfg integrations.Config) (integrations.Snapshot, error) {
	if err := i.Test(ctx, cfg); err != nil {
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
	ok := integrations.StatusOK

	var (
		c    cpu
		mem  struct{ Total, Used int64 }
		load struct {
			Min1, Min5, Min15 float64
		}
		sys struct {
			Hostname string `json:"hostname"`
			HRName   string `json:"hr_name"`
		}
		up string
		fs []struct {
			Mnt     string  `json:"mnt_point"`
			Type    string  `json:"fs_type"`
			Size    int64   `json:"size"`
			Used    int64   `json:"used"`
			Percent float64 `json:"percent"`
		}
	)
	for path, out := range map[string]any{"/cpu": &c, "/mem": &mem, "/load": &load, "/system": &sys, "/fs": &fs} {
		if err := get(ctx, cfg, path, out); err != nil {
			return integrations.Snapshot{}, err
		}
	}
	_ = get(ctx, cfg, "/uptime", &up) // optional
	host := sys.Hostname
	if host == "" {
		host = "host"
	}

	var snap integrations.Snapshot
	h := integrations.Resource{ID: "host/" + host, Name: host, Kind: integrations.KindNode, State: "online",
		Status: ok, CPU: c.Total, CPUs: c.CPUCore, Uptime: parseUptime(up), Info: map[string]string{"os": sys.HRName}}
	if mem.Total > 0 {
		h.MemUsed, h.MemTotal = ptr(mem.Used), ptr(mem.Total)
	}
	add := func(r *integrations.Resource, name string, v float64, thresholds bool) {
		st := ok
		if thresholds {
			st = spec.Status(v)
		}
		r.Status = integrations.Worst(r.Status, st)
		snap.Metrics = append(snap.Metrics, integrations.Metric{Key: "res/" + r.ID + "/" + name, Name: name, Group: r.Name,
			Value: v, Unit: "%", Status: st, Resource: r.ID})
	}
	add(&h, "cpu", *c.Total, true)
	if mem.Total > 0 {
		add(&h, "mem_pct", float64(mem.Used)/float64(mem.Total)*100, true)
	}

	// Dateisysteme: Root als Platte des Hosts, alle als Kennzahlen
	for _, f := range fs {
		if slices.Contains(pseudoFS, f.Type) || f.Size <= 0 || strings.HasPrefix(f.Mnt, "/var/lib/docker/") {
			continue
		}
		if f.Mnt == "/" {
			h.DiskUsed, h.DiskTotal = ptr(f.Used), ptr(f.Size)
			add(&h, "disk_pct", f.Percent, true)
		}
		snap.Metrics = append(snap.Metrics,
			integrations.Metric{Key: "fs/" + f.Mnt + "/used_pct", Name: "used_pct", Group: f.Mnt, Value: f.Percent, Unit: "%",
				Status: spec.Status(f.Percent)},
			integrations.Metric{Key: "fs/" + f.Mnt + "/used", Name: "used", Group: f.Mnt, Value: float64(f.Used), Unit: "bytes", Status: ok},
			integrations.Metric{Key: "fs/" + f.Mnt + "/total", Name: "total", Group: f.Mnt, Value: float64(f.Size), Unit: "bytes", Status: ok},
		)
	}
	snap.Resources = append(snap.Resources, h)

	running, containers := 0, 0
	if !cfg.Has("containers") || cfg.Bool("containers") {
		var list []struct {
			Name   string   `json:"name"`
			Image  string   `json:"image"`
			Status string   `json:"status"`
			CPU    *float64 `json:"cpu_percent"`
			Mem    *int64   `json:"memory_usage"`
			Limit  *int64   `json:"memory_limit"`
			RX     *int64   `json:"network_rx"`
			TX     *int64   `json:"network_tx"`
			Uptime string   `json:"uptime"`
		}
		// Container-Plugin aus (404) oder kein Docker: dann eben ohne
		if err := get(ctx, cfg, "/containers", &list); err == nil {
			for _, ct := range list {
				containers++
				r := integrations.Resource{ID: "container/" + ct.Name, Name: ct.Name, Kind: integrations.KindContainer,
					Parent: host, State: ct.Status, CPU: ct.CPU, MemUsed: ct.Mem, MemTotal: ct.Limit, NetIn: ct.RX, NetOut: ct.TX,
					NetRate: true, Info: map[string]string{"image": ct.Image, "uptime": ct.Uptime}}
				switch ct.Status {
				case "running", "healthy":
					r.Status = ok
					running++
				case "unhealthy":
					r.Status = integrations.StatusError
					running++
				case "starting", "restarting", "created":
					r.Status = integrations.StatusWarning
				default: // exited, paused, dead
					r.Status = integrations.StatusPaused
				}
				if ct.CPU != nil {
					snap.Metrics = append(snap.Metrics, integrations.Metric{Key: "res/" + r.ID + "/cpu", Name: "cpu", Group: r.Name,
						Value: *ct.CPU, Unit: "%", Status: ok, Resource: r.ID})
				}
				if ct.Mem != nil {
					snap.Metrics = append(snap.Metrics, integrations.Metric{Key: "res/" + r.ID + "/mem", Name: "mem", Group: r.Name,
						Value: float64(*ct.Mem), Unit: "bytes", Status: ok, Resource: r.ID})
				}
				// „unhealthy“ als Kennzahl, damit es im Gesamtstatus und in Benachrichtigungen erscheint
				if r.Status == integrations.StatusError {
					snap.Metrics = append(snap.Metrics, integrations.Metric{Key: "res/" + r.ID + "/container_health", Name: "container_health",
						Group: r.Name, Unit: "okfail", Status: integrations.StatusError, Resource: r.ID})
				}
				snap.Resources = append(snap.Resources, r)
			}
		}
	}

	main := []integrations.Metric{
		{Key: "cpu", Name: "cpu", Value: *c.Total, Unit: "%", Status: ok},
	}
	if mem.Total > 0 {
		main = append(main, integrations.Metric{Key: "mem_pct", Name: "mem_pct", Value: float64(mem.Used) / float64(mem.Total) * 100, Unit: "%", Status: ok})
	}
	if containers > 0 {
		main = append(main, integrations.Metric{Key: "containers_running", Name: "containers_running", Value: float64(running), Status: ok})
	}
	main = append(main, integrations.Metric{Key: "load5", Name: "load5", Value: load.Min5, Status: ok})
	snap.Metrics = append(main, snap.Metrics...)
	if sys.HRName != "" {
		snap.Info = map[string]string{"version": sys.HRName}
	}
	return snap, nil
}
