// Package arcane liest Docker-Hosts, Container und Compose-Stacks aus Arcane
// (https://getarcane.app): je Umgebung (Docker-Host) Erreichbarkeit,
// Container mit Zustand, Health und – optional – CPU/RAM, Stacks und
// ausstehende Image-Updates.
//
// API (geprüft gegen den Quellcode von Arcane v3.1.0, types/*.go und
// frontend/src/lib/services/*.ts): Anmeldung per API-Key im Header
// „X-Api-Key“ (arc_…). Ab v3 heißen die Umgebungen „agents“:
//
//	GET /api/agents                          Umgebungen (paginiert)
//	GET /api/agents/{id}/containers          Container (paginiert; sort=cpuUsage liefert resourceSample)
//	GET /api/agents/{id}/containers/counts   laufend, gestoppt, unhealthy, Updates
//	GET /api/agents/{id}/projects            Compose-Stacks (paginiert)
//	GET /api/agents/{id}/projects/counts     Stacks laufend/gestoppt, Updates
//	GET /api/app/version                     Version
//
// Arcane v2 bietet dieselben Pfade unter /api/environments – antwortet
// /api/agents mit 404, wird darauf ausgewichen. Antworten stecken in einer
// Hülle {success, data, pagination}.
package arcane

import (
	"cmp"
	"context"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
)

const Type = "arcane"

// pageSize: Einträge je Seite beim Blättern durch Listen.
const pageSize = 100

// maxPages begrenzt das Blättern (100 × 100 Container sind mehr als genug).
const maxPages = 100

type Integration struct{}

func New() *Integration { return &Integration{} }

func (*Integration) Type() string { return Type }

func (*Integration) Schema() integrations.ConfigSchema {
	type T = integrations.Text
	return integrations.ConfigSchema{
		Title: T{DE: "Arcane", EN: "Arcane"},
		Description: T{
			DE: "Docker-Hosts, Container und Compose-Stacks aus Arcane: Zustand, Health, CPU/RAM und ausstehende Image-Updates.",
			EN: "Docker hosts, containers and compose stacks from Arcane: state, health, CPU/RAM and pending image updates.",
		},
		Icon:     "container",
		Category: integrations.CategoryCompute,
		Docs:     "https://getarcane.app/docs",
		Fields: []integrations.Field{
			{Key: "base_url", Type: integrations.FieldURL, Required: true, Default: "http://arcane.lan:3552",
				Label:       T{DE: "Adresse", EN: "Address"},
				Help:        T{DE: "Adresse der Arcane-Oberfläche (Standard-Port 3552).", EN: "Address of the Arcane web interface (default port 3552)."},
				Placeholder: "http://arcane.lan:3552"},
			{Key: "api_key", Type: integrations.FieldSecret, Required: true,
				Label: T{DE: "API-Key", EN: "API key"},
				Help: T{
					DE: "In Arcane unter Einstellungen → API-Keys anlegen (beginnt mit arc_). Leserechte genügen: Environments, Containers und Projects jeweils auflisten/ansehen.",
					EN: "Create one in Arcane under Settings → API Keys (starts with arc_). Read access is enough: list/view for Environments, Containers and Projects.",
				}},
			{Key: "environments", Type: integrations.FieldString,
				Label:       T{DE: "Umgebungen", EN: "Environments"},
				Help:        T{DE: "Namen der Docker-Hosts in Arcane, komma-getrennt; leer = alle.", EN: "Names of the Docker hosts in Arcane, comma-separated; empty = all."},
				Placeholder: "Local Docker, pi-edge"},
			{Key: "container_stats", Type: integrations.FieldBool, Default: true,
				Label: T{DE: "CPU und RAM je Container", EN: "CPU and RAM per container"},
				Help: T{
					DE: "Arcane misst dafür alle laufenden Container (dauert je nach Anzahl 1–2 Sekunden je Abfrage).",
					EN: "Arcane samples all running containers for this (takes 1–2 seconds per poll, depending on their number).",
				}},
		},
	}
}

// client fragt eine Arcane-Instanz ab; prefix ist /api/agents (v3) bzw.
// /api/environments (v2).
type client struct {
	cfg    integrations.Config
	base   string
	prefix string
}

func (c *client) get(ctx context.Context, path string, out any) error {
	h := http.Header{"X-Api-Key": {strings.TrimSpace(c.cfg.Secret("api_key"))}}
	return integrations.GetJSON(ctx, c.cfg.HTTP, c.base+path, h, out)
}

// envelope ist die Hülle der Arcane-Antworten.
type envelope[T any] struct {
	Success    *bool `json:"success"`
	Data       T     `json:"data"`
	Pagination *struct {
		HasNextPage bool `json:"hasNextPage"`
	} `json:"pagination"`
}

// getData holt path und liefert data aus der Hülle.
func getData[T any](ctx context.Context, c *client, path string) (T, error) {
	var e envelope[T]
	if err := c.get(ctx, path, &e); err != nil {
		return e.Data, err
	}
	if e.Success == nil {
		return e.Data, i18n.E("arcane.unexpected")
	}
	return e.Data, nil
}

// list blättert durch eine paginierte Liste (query ohne start/limit).
func list[T any](ctx context.Context, c *client, path string, query url.Values) ([]T, error) {
	var out []T
	for page := 0; page < maxPages; page++ {
		q := url.Values{}
		for k, v := range query {
			q[k] = v
		}
		q.Set("start", strconv.Itoa(len(out)))
		q.Set("limit", strconv.Itoa(pageSize))
		var e envelope[[]T]
		if err := c.get(ctx, path+"?"+q.Encode(), &e); err != nil {
			return nil, err
		}
		if e.Success == nil {
			return nil, i18n.E("arcane.unexpected")
		}
		out = append(out, e.Data...)
		if e.Pagination == nil || !e.Pagination.HasNextPage || len(e.Data) == 0 {
			break
		}
	}
	return out, nil
}

// env ist eine Arcane-Umgebung (Docker-Host).
type env struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"` // online, offline, error, pending
	Enabled   bool   `json:"enabled"`
	AgentHost *struct {
		Hostname      string `json:"hostname"`
		OS            string `json:"os"`
		DockerVersion string `json:"dockerVersion"`
		MemoryBytes   int64  `json:"memoryBytes"`
		CPUs          int32  `json:"cpus"`
	} `json:"agentHost"`
}

// connect findet die API-Variante und liefert die (gewählten) Umgebungen.
func connect(ctx context.Context, cfg integrations.Config) (*client, []env, error) {
	c := &client{cfg: cfg, base: cfg.BaseURL("base_url"), prefix: "/api/agents"}
	all, err := list[env](ctx, c, c.prefix, nil)
	if m := i18n.FromError(err); m.Code == "integration.not_found" {
		c.prefix = "/api/environments" // Arcane v2
		all, err = list[env](ctx, c, c.prefix, nil)
	}
	if err != nil {
		if m := i18n.FromError(err); m.Code == "integration.bad_json" || m.Code == "integration.not_found" {
			return nil, nil, i18n.E("arcane.unexpected")
		}
		return nil, nil, err
	}
	var want []string
	for _, n := range strings.Split(cfg.String("environments"), ",") {
		if n = strings.TrimSpace(n); n != "" {
			want = append(want, n)
		}
	}
	// leere Liste: meist fehlt dem Key das Recht, Umgebungen aufzulisten –
	// das sagen statt „Umgebung … nicht gefunden“
	if len(all) == 0 {
		return nil, nil, i18n.E("arcane.no_envs")
	}
	var out []env
	for _, e := range all {
		if e.ID == "" {
			return nil, nil, i18n.E("arcane.unexpected")
		}
		if len(want) == 0 || slices.Contains(want, e.Name) {
			out = append(out, e)
		}
	}
	if len(out) == 0 {
		if len(want) > 0 {
			names := make([]string, 0, len(all))
			for _, e := range all {
				names = append(names, e.Name)
			}
			return nil, nil, i18n.E("arcane.env_missing", "names", strings.Join(want, ", "), "available", strings.Join(names, ", "))
		}
		return nil, nil, i18n.E("arcane.no_envs")
	}
	return c, out, nil
}

func (*Integration) Test(ctx context.Context, cfg integrations.Config) error {
	_, _, err := connect(ctx, cfg)
	return err
}

type containerCounts struct {
	Running   int `json:"runningContainers"`
	Stopped   int `json:"stoppedContainers"`
	Total     int `json:"totalContainers"`
	Unhealthy int `json:"unhealthyContainers"`
	Updates   int `json:"updatesAvailable"`
}

type projectCounts struct {
	Running int `json:"runningProjects"`
	Stopped int `json:"stoppedProjects"`
	Total   int `json:"totalProjects"`
	Updates int `json:"updatesAvailable"`
}

type container struct {
	ID         string            `json:"id"`
	Names      []string          `json:"names"`
	Image      string            `json:"image"`
	State      string            `json:"state"`  // running, exited, restarting, paused, created, dead
	Status     string            `json:"status"` // „Up 3 hours (healthy)“
	Labels     map[string]string `json:"labels"`
	UpdateInfo *struct {
		HasUpdate bool `json:"hasUpdate"`
	} `json:"updateInfo"`
	ResourceSample *struct {
		CPUPercent       float64 `json:"cpuPercent"`
		MemoryUsageBytes int64   `json:"memoryUsageBytes"`
		MemoryLimitBytes int64   `json:"memoryLimitBytes"`
		NetworkRxBytes   int64   `json:"networkRxBytes"`
		NetworkTxBytes   int64   `json:"networkTxBytes"`
	} `json:"resourceSample"`
	Hidden bool `json:"hidden"`
}

func (c container) name() string {
	for _, n := range c.Names {
		if n = strings.TrimPrefix(n, "/"); n != "" {
			return n
		}
	}
	if len(c.ID) > 12 {
		return c.ID[:12]
	}
	return c.ID
}

// health aus dem Docker-Status („Up 2 hours (healthy)“).
func (c container) health() string {
	for _, h := range []string{"unhealthy", "healthy", "starting"} {
		if strings.Contains(c.Status, "("+h+")") || strings.Contains(c.Status, "(health: "+h+")") {
			return h
		}
	}
	return ""
}

type project struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"` // running, stopped, partially running
}

type version struct {
	CurrentVersion string `json:"currentVersion"`
	DisplayVersion string `json:"displayVersion"`
}

func ptr[T any](v T) *T { return &v }

func (*Integration) Collect(ctx context.Context, cfg integrations.Config) (integrations.Snapshot, error) {
	c, envs, err := connect(ctx, cfg)
	if err != nil {
		return integrations.Snapshot{}, err
	}
	withStats := !cfg.Has("container_stats") || cfg.Bool("container_stats")
	ok := integrations.StatusOK

	var snap integrations.Snapshot
	var running, total, unhealthy, updates, stacks, stacksRunning, online int
	var problems []integrations.Metric

	for _, e := range envs {
		node := integrations.Resource{ID: "env/" + e.ID, Name: e.Name, Kind: integrations.KindNode, State: e.Status, Status: ok}
		if h := e.AgentHost; h != nil {
			node.CPUs = float64(h.CPUs)
			node.Info = map[string]string{"hostname": h.Hostname, "os": h.OS, "docker": h.DockerVersion}
		}
		if !e.Enabled {
			node.State, node.Status = "disabled", integrations.StatusPaused
			snap.Resources = append(snap.Resources, node)
			continue
		}
		if e.Status != "online" {
			node.Status = integrations.StatusError
			if e.Status == "pending" {
				node.Status = integrations.StatusWarning
			}
			snap.Resources = append(snap.Resources, node)
			problems = append(problems, integrations.Metric{Key: "res/" + node.ID + "/online", Name: "online",
				Group: e.Name, Unit: "okfail", Resource: node.ID, Status: node.Status})
			continue
		}
		online++
		p := c.prefix + "/" + url.PathEscape(e.ID)

		counts, err := getData[containerCounts](ctx, c, p+"/containers/counts")
		if err != nil {
			return integrations.Snapshot{}, err
		}
		pc, err := getData[projectCounts](ctx, c, p+"/projects/counts")
		if err != nil {
			return integrations.Snapshot{}, err
		}
		updates += counts.Updates + pc.Updates
		stacks += pc.Total
		stacksRunning += pc.Running

		q := url.Values{"includeHidden": {"true"}}
		if withStats {
			// nur bei Sortierung nach Ressourcen misst Arcane CPU/RAM mit
			q.Set("sort", "cpuUsage")
			q.Set("order", "desc")
		}
		cts, err := list[container](ctx, c, p+"/containers", q)
		if err != nil {
			return integrations.Snapshot{}, err
		}
		var cpuSum float64
		var memSum int64
		var sampled bool
		for _, ct := range cts {
			total++
			name := ct.name()
			r := integrations.Resource{ID: "container/" + e.ID + "/" + name, Name: name, Kind: integrations.KindContainer,
				Parent: e.Name, State: ct.State, Info: map[string]string{"image": ct.Image, "uptime": ct.Status}}
			if s := ct.Labels["com.docker.compose.project"]; s != "" {
				r.Info["stack"] = s
			}
			if ct.UpdateInfo != nil && ct.UpdateInfo.HasUpdate {
				r.Info["update"] = "1"
			}
			hl := ct.health()
			switch {
			case ct.State == "running" && hl == "unhealthy":
				r.State, r.Status = "unhealthy", integrations.StatusError
				running++
				unhealthy++
			case ct.State == "running":
				r.Status = ok
				if hl == "healthy" {
					r.State = "healthy"
				}
				running++
			case ct.State == "restarting":
				r.Status = integrations.StatusWarning
			default: // exited, paused, created, dead
				r.Status = integrations.StatusPaused
			}
			if s := ct.ResourceSample; s != nil && ct.State == "running" {
				sampled = true
				r.CPU = ptr(s.CPUPercent)
				r.MemUsed, r.NetIn, r.NetOut = ptr(s.MemoryUsageBytes), ptr(s.NetworkRxBytes), ptr(s.NetworkTxBytes)
				if s.MemoryLimitBytes > 0 {
					r.MemTotal = ptr(s.MemoryLimitBytes)
				}
				cpuSum += s.CPUPercent
				memSum += s.MemoryUsageBytes
				snap.Metrics = append(snap.Metrics,
					integrations.Metric{Key: "res/" + r.ID + "/cpu", Name: "cpu", Group: name, Value: s.CPUPercent,
						Unit: "%", Status: ok, Resource: r.ID},
					integrations.Metric{Key: "res/" + r.ID + "/mem", Name: "mem", Group: name, Value: float64(s.MemoryUsageBytes),
						Unit: "bytes", Status: ok, Resource: r.ID})
			}
			// „unhealthy“ als Kennzahl: erscheint im Gesamtstatus und in Benachrichtigungen
			if r.Status == integrations.StatusError {
				problems = append(problems, integrations.Metric{Key: "res/" + r.ID + "/container_health",
					Name: "container_health", Group: name + " · " + e.Name, Unit: "okfail",
					Status: integrations.StatusError, Resource: r.ID})
			}
			snap.Resources = append(snap.Resources, r)
		}
		// Host-Last: Summe der Container (Arcane liefert keine Host-Auslastung)
		if sampled {
			node.CPU = ptr(cpuSum)
			if node.CPUs > 0 {
				node.CPU = ptr(cpuSum / node.CPUs)
			}
			node.MemUsed = ptr(memSum)
			if h := e.AgentHost; h != nil && h.MemoryBytes > 0 {
				node.MemTotal = ptr(h.MemoryBytes)
			}
		}
		node.Info = withInfo(node.Info, "containers", strconv.Itoa(counts.Total))
		snap.Resources = append(snap.Resources, node)

		// teilweise laufende Stacks: ein Dienst fehlt
		if pc.Total > 0 {
			projects, err := list[project](ctx, c, p+"/projects", url.Values{"status": {"partially running"}})
			if err != nil {
				return integrations.Snapshot{}, err
			}
			for _, pr := range projects {
				if pr.Status != "partially running" {
					continue
				}
				problems = append(problems, integrations.Metric{Key: "stack/" + e.ID + "/" + pr.Name, Name: "stack_partial",
					Group: pr.Name + " · " + e.Name, Unit: "okfail", Status: integrations.StatusWarning})
			}
		}
	}

	main := []integrations.Metric{
		{Key: "containers_running", Name: "containers_running", Value: float64(running), Status: ok},
		{Key: "containers", Name: "containers", Value: float64(total), Status: ok},
		{Key: "unhealthy", Name: "unhealthy", Value: float64(unhealthy), Status: ok, Derived: true},
		{Key: "stacks_running", Name: "stacks_running", Value: float64(stacksRunning), Status: ok},
		{Key: "stacks", Name: "stacks", Value: float64(stacks), Status: ok},
		{Key: "updates", Name: "updates", Value: float64(updates), Status: ok},
		{Key: "hosts", Name: "hosts", Value: float64(len(envs)), Status: ok},
	}
	if unhealthy > 0 {
		main[2].Status = integrations.StatusError
	}
	snap.Metrics = append(append(main, problems...), snap.Metrics...)

	// Version: mit oder ohne Hülle; fehlt sie, bleibt die Abfrage gültig
	var vr struct {
		Data *version `json:"data"`
		version
	}
	if c.get(ctx, "/api/app/version", &vr) == nil {
		v := vr.version
		if vr.Data != nil {
			v = *vr.Data
		}
		if ver := cmp.Or(v.DisplayVersion, v.CurrentVersion); ver != "" {
			snap.Info = map[string]string{"version": "Arcane " + strings.TrimPrefix(ver, "v")}
		}
	}
	return snap, nil
}

func withInfo(m map[string]string, k, v string) map[string]string {
	if m == nil {
		m = map[string]string{}
	}
	m[k] = v
	return m
}
