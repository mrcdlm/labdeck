// Package arcanetest bildet die benötigten Teile der Arcane-API nach
// (Umgebungen, Container, Stacks, Zähler, Version) – für Tests und den
// Entwicklungs-Mock. V2 schaltet auf die Pfade von Arcane 2.x
// (/api/environments statt /api/agents).
package arcanetest

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Container ist ein nachgebildeter Docker-Container.
type Container struct {
	ID, Name, Image string
	State           string // running, exited, restarting …
	Health          string // healthy, unhealthy, "" = ohne Healthcheck
	Stack           string
	Update          bool
	CPU             float64
	Mem, Limit      int64
}

// Project ist ein Compose-Stack.
type Project struct {
	Name   string
	Status string // running, stopped, partially running
	Update bool
}

// Env ist eine Arcane-Umgebung (Docker-Host).
type Env struct {
	ID         string
	Name       string
	Status     string // online, offline, error, pending
	Disabled   bool
	CPUs       int
	Memory     int64
	NoHostInfo bool // agentHost fehlt (RAM/CPUs nur über docker info)
	Containers []Container
	Projects   []Project
}

type Fake struct {
	mu       sync.Mutex
	key      string // erwarteter API-Key
	envs     []Env
	v2       bool
	fail     int
	broken   bool
	delay    time.Duration
	requests int
}

func New(key string) *Fake { return &Fake{key: key} }

const gb = int64(1_000_000_000)

// Demo: zwei Docker-Hosts mit typischen HomeLab-Containern.
func (f *Fake) Demo() *Fake {
	f.envs = []Env{
		{ID: "0", Name: "docker01", Status: "online", CPUs: 8, Memory: 32 * gb,
			Containers: []Container{
				{ID: "a1a1a1a1a1a1a1", Name: "jellyfin", Image: "jellyfin/jellyfin:10.10", State: "running", Health: "healthy", Stack: "media", CPU: 41.2, Mem: 1_400_000_000, Limit: 32 * gb},
				{ID: "a2a2a2a2a2a2a2", Name: "paperless", Image: "ghcr.io/paperless-ngx/paperless-ngx:2.15", State: "running", Stack: "paperless", Update: true, CPU: 1.8, Mem: 820_000_000, Limit: 32 * gb},
				{ID: "a3a3a3a3a3a3a3", Name: "vaultwarden", Image: "vaultwarden/server:1.34", State: "running", Health: "unhealthy", Stack: "vault", CPU: 0.1, Mem: 64_000_000, Limit: 32 * gb},
				{ID: "a4a4a4a4a4a4a4", Name: "old-wiki", Image: "requarks/wiki:2", State: "exited", Stack: "wiki"},
			},
			Projects: []Project{{Name: "media", Status: "running"}, {Name: "paperless", Status: "running", Update: true},
				{Name: "vault", Status: "running"}, {Name: "wiki", Status: "stopped"}, {Name: "monitoring", Status: "partially running"}}},
		{ID: "6f1c2a9e-0b7d-4f2e-9a55-3c1d2e4f5a6b", Name: "pi-edge", Status: "online", CPUs: 4, Memory: 4 * gb, NoHostInfo: true,
			Containers: []Container{
				{ID: "b1b1b1b1b1b1b1", Name: "zigbee2mqtt", Image: "koenkk/zigbee2mqtt:2.6", State: "running", Stack: "home", CPU: 2.4, Mem: 180_000_000, Limit: 4 * gb},
			},
			Projects: []Project{{Name: "home", Status: "running"}}},
	}
	return f
}

// SetEnvs ersetzt die Umgebungen (Tests).
func (f *Fake) SetEnvs(e ...Env) { f.mu.Lock(); f.envs = e; f.mu.Unlock() }

// V2 schaltet auf die Pfade von Arcane 2.x.
func (f *Fake) V2(on bool) { f.mu.Lock(); f.v2 = on; f.mu.Unlock() }

// SetHealth ändert den Health-Status eines Containers.
func (f *Fake) SetHealth(name, health string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.envs {
		for j := range f.envs[i].Containers {
			if f.envs[i].Containers[j].Name == name {
				f.envs[i].Containers[j].Health = health
			}
		}
	}
}

func (f *Fake) Fail(status int)       { f.mu.Lock(); f.fail = status; f.mu.Unlock() }
func (f *Fake) Broken(b bool)         { f.mu.Lock(); f.broken = b; f.mu.Unlock() }
func (f *Fake) Delay(d time.Duration) { f.mu.Lock(); f.delay = d; f.mu.Unlock() }
func (f *Fake) Requests() int         { f.mu.Lock(); defer f.mu.Unlock(); return f.requests }

func (f *Fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests++
	fail, broken, delay, key, v2 := f.fail, f.broken, f.delay, f.key, f.v2
	envs := append([]Env(nil), f.envs...)
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
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		w.Write([]byte("<!doctype html><title>Arcane</title>"))
		return
	}
	if r.Header.Get("X-Api-Key") != key {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"success": false, "error": "Unauthorized"})
		return
	}
	if broken {
		w.Write([]byte(`{"success":true,"data":[{"id":`))
		return
	}
	if r.URL.Path == "/api/app/version" {
		writeJSON(w, http.StatusOK, map[string]any{"currentVersion": "v3.1.0", "displayVersion": "v3.1.0", "revision": "abc123"})
		return
	}

	prefix := "/api/agents"
	if v2 {
		prefix = "/api/environments"
	}
	rest, found := strings.CutPrefix(r.URL.Path, prefix)
	if !found {
		notFound(w)
		return
	}
	q := r.URL.Query()
	if rest == "" {
		out := []any{}
		for _, e := range envs {
			m := map[string]any{"id": e.ID, "name": e.Name, "apiUrl": "http://" + e.Name + ":3553",
				"status": e.Status, "enabled": !e.Disabled}
			if !e.NoHostInfo {
				m["agentHost"] = map[string]any{"hostname": e.Name + ".lan", "os": "Ubuntu 24.04", "dockerVersion": "28.4.0",
					"memoryBytes": e.Memory, "cpus": e.CPUs}
			}
			out = append(out, m)
		}
		page(w, q, out)
		return
	}
	parts := strings.Split(strings.TrimPrefix(rest, "/"), "/")
	var env *Env
	for i := range envs {
		if envs[i].ID == parts[0] {
			env = &envs[i]
		}
	}
	if env == nil || len(parts) < 2 {
		notFound(w)
		return
	}
	if env.Status != "online" {
		writeJSON(w, http.StatusBadGateway, map[string]any{"success": false, "error": "environment offline"})
		return
	}
	switch strings.Join(parts[1:], "/") {
	case "system/docker/info":
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "MemTotal": env.Memory, "NCPU": env.CPUs,
			"OperatingSystem": "Debian GNU/Linux 13", "ServerVersion": "28.5.1"})
	case "containers/counts":
		var running, stopped, unhealthy, updates int
		for _, c := range env.Containers {
			if c.State == "running" {
				running++
			} else {
				stopped++
			}
			if c.Health == "unhealthy" {
				unhealthy++
			}
			if c.Update {
				updates++
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]int{"runningContainers": running,
			"stoppedContainers": stopped, "totalContainers": len(env.Containers), "unhealthyContainers": unhealthy,
			"updatesAvailable": updates}})
	case "projects/counts":
		var running, stopped, updates int
		for _, p := range env.Projects {
			switch p.Status {
			case "running":
				running++
			case "stopped":
				stopped++
			}
			if p.Update {
				updates++
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]int{"runningProjects": running,
			"stoppedProjects": stopped, "totalProjects": len(env.Projects), "archivedProjects": 0, "updatesAvailable": updates}})
	case "containers":
		withStats := q.Get("sort") == "cpuUsage"
		out := []any{}
		for _, c := range env.Containers {
			status := "Exited (0) 3 days ago"
			if c.State == "running" {
				status = "Up 3 days"
				if c.Health != "" {
					status += " (" + c.Health + ")"
				}
			}
			m := map[string]any{"id": c.ID, "names": []string{"/" + c.Name}, "image": c.Image, "imageId": "sha256:" + c.ID,
				"state": c.State, "status": status, "created": 1_760_000_000,
				"labels":     map[string]string{"com.docker.compose.project": c.Stack},
				"hostConfig": map[string]any{"restartPolicy": "unless-stopped"}, "ports": []any{}, "mounts": []any{},
				"networkSettings": map[string]any{}, "autoUpdateEnabled": false,
				"updateInfo": map[string]any{"hasUpdate": c.Update, "updateType": "digest"}}
			if withStats && c.State == "running" {
				m["resourceSample"] = map[string]any{"sampleTime": time.Now().UTC().Format(time.RFC3339), "cpuPercent": c.CPU,
					"memoryUsageBytes": c.Mem, "memoryLimitBytes": c.Limit, "networkRxBytes": 12_000_000_000,
					"networkTxBytes": 3_400_000_000, "blockReadBytes": 0, "blockWriteBytes": 0, "pids": 12}
			}
			out = append(out, m)
		}
		page(w, q, out)
	case "projects":
		out := []any{}
		for i, p := range env.Projects {
			if s := q.Get("status"); s != "" && s != p.Status {
				continue
			}
			out = append(out, map[string]any{"id": strconv.Itoa(i + 1), "name": p.Name, "status": p.Status,
				"path": "/opt/stacks/" + p.Name, "createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-11-01T00:00:00Z",
				"serviceCount": 2, "runningCount": 1, "tags": []any{}, "isArchived": false})
		}
		page(w, q, out)
	default:
		notFound(w)
	}
}

// page liefert einen Ausschnitt (start/limit) wie Arcane.
func page(w http.ResponseWriter, q map[string][]string, all []any) {
	get := func(k string, def int) int {
		if v, ok := q[k]; ok && len(v) > 0 {
			if n, err := strconv.Atoi(v[0]); err == nil {
				return n
			}
		}
		return def
	}
	start, limit := get("start", 0), get("limit", 20)
	start = min(max(start, 0), len(all))
	end := min(start+max(limit, 1), len(all))
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": all[start:end],
		"pagination": map[string]any{"totalPages": (len(all) + limit - 1) / max(limit, 1), "totalItems": len(all),
			"currentPage": start/max(limit, 1) + 1, "itemsPerPage": limit, "hasNextPage": end < len(all)}})
}

func notFound(w http.ResponseWriter) {
	writeJSON(w, http.StatusNotFound, map[string]any{"title": "Not Found", "status": 404, "detail": "no route"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
