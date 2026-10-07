// Package dockhandtest bildet die benötigten Teile der Dockhand-API nach
// (Umgebungen, Dashboard-Statistik, Host, Container, Container-Statistik) –
// für Tests und den Entwicklungs-Mock.
package dockhandtest

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
	CPU             float64
	Mem, Limit      int64
}

// Env ist eine Dockhand-Umgebung (Docker-Host).
type Env struct {
	ID         int
	Name       string
	Online     bool
	CPU        float64
	MemUsed    int64
	MemTotal   int64
	Updates    int
	Containers []Container
}

type Fake struct {
	mu       sync.Mutex
	token    string // erwartetes Bearer-Token (leer = Anmeldung aus)
	envs     []Env
	fail     int
	broken   bool
	delay    time.Duration
	requests int
}

func New(token string) *Fake { return &Fake{token: token} }

const gb = int64(1_000_000_000)

// Demo: zwei Docker-Hosts, einer mit typischen HomeLab-Containern.
func (f *Fake) Demo() *Fake {
	f.envs = []Env{
		{ID: 1, Name: "docker01", Online: true, CPU: 23.5, MemUsed: 19_800_000_000, MemTotal: 32 * gb, Updates: 2,
			Containers: []Container{
				{ID: "a1", Name: "jellyfin", Image: "jellyfin/jellyfin:10.10", State: "running", Health: "healthy", CPU: 41.2, Mem: 1_400_000_000, Limit: 32 * gb},
				{ID: "a2", Name: "paperless", Image: "ghcr.io/paperless-ngx/paperless-ngx:2.15", State: "running", CPU: 1.8, Mem: 820_000_000, Limit: 32 * gb},
				{ID: "a3", Name: "vaultwarden", Image: "vaultwarden/server:1.34", State: "running", Health: "unhealthy", CPU: 0.1, Mem: 64_000_000, Limit: 32 * gb},
				{ID: "a4", Name: "old-wiki", Image: "requarks/wiki:2", State: "exited"},
			}},
		{ID: 2, Name: "pi-edge", Online: true, CPU: 6.2, MemUsed: 1_100_000_000, MemTotal: 4 * gb,
			Containers: []Container{
				{ID: "b1", Name: "zigbee2mqtt", Image: "koenkk/zigbee2mqtt:2.6", State: "running", CPU: 2.4, Mem: 180_000_000, Limit: 4 * gb},
			}},
	}
	return f
}

// SetEnvs ersetzt die Umgebungen (Tests).
func (f *Fake) SetEnvs(e ...Env) { f.mu.Lock(); f.envs = e; f.mu.Unlock() }

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
	fail, broken, delay, token := f.fail, f.broken, f.delay, f.token
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
	if token != "" && r.Header.Get("Authorization") != "Bearer "+token {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
		return
	}
	if broken {
		w.Write([]byte(`[{"id":`))
		return
	}
	env := func() *Env {
		id, _ := strconv.Atoi(r.URL.Query().Get("env"))
		for i := range envs {
			if envs[i].ID == id {
				return &envs[i]
			}
		}
		return nil
	}
	switch r.URL.Path {
	case "/api/environments":
		out := []map[string]any{}
		for _, e := range envs {
			out = append(out, map[string]any{"id": e.ID, "name": e.Name, "connectionType": "socket", "icon": "server",
				"hasTlsKey": false, "hasHawserToken": false})
		}
		writeJSON(w, http.StatusOK, out)
	case "/api/dashboard/stats":
		e := env()
		if e == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Environment not found"})
			return
		}
		running, stopped, unhealthy := 0, 0, 0
		for _, c := range e.Containers {
			switch c.State {
			case "running":
				running++
			case "exited":
				stopped++
			}
			if c.Health == "unhealthy" {
				unhealthy++
			}
		}
		m := map[string]any{"id": e.ID, "name": e.Name, "online": e.Online,
			"containers": map[string]int{"total": len(e.Containers), "running": running, "stopped": stopped, "paused": 0,
				"restarting": 0, "unhealthy": unhealthy, "pendingUpdates": e.Updates},
			"images": map[string]int64{"total": 23, "totalSize": 14 * gb}, "volumes": map[string]int64{"total": 9, "totalSize": 41 * gb},
			"stacks": map[string]int{"total": 4, "running": 3, "partial": 0, "stopped": 1}, "metrics": nil,
			"events": map[string]int{"total": 120, "today": 7}}
		if e.Online {
			m["metrics"] = map[string]any{"cpuPercent": e.CPU, "memoryPercent": float64(e.MemUsed) / float64(e.MemTotal) * 100,
				"memoryUsed": e.MemUsed, "memoryTotal": e.MemTotal}
		}
		writeJSON(w, http.StatusOK, m)
	case "/api/host":
		e := env()
		if e == nil || !e.Online {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to get host info"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"hostname": e.Name + ".lan", "ipAddress": "192.168.1.10", "platform": "linux",
			"arch": "x64", "cpus": 8, "totalMemory": e.MemTotal, "freeMemory": e.MemTotal - e.MemUsed, "uptime": 1_047_845,
			"dockerVersion": "28.4.0", "dockerContainers": len(e.Containers), "dockerContainersRunning": 0, "dockerImages": 23})
	case "/api/containers":
		e := env()
		if e == nil {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		out := []map[string]any{}
		for _, c := range e.Containers {
			status := "Exited (0) 3 days ago"
			if c.State == "running" {
				status = "Up 3 days"
				if c.Health != "" {
					status += " (" + c.Health + ")"
				}
			}
			m := map[string]any{"id": c.ID, "name": c.Name, "image": c.Image, "state": c.State, "status": status, "restartCount": 0}
			if c.Health != "" {
				m["health"] = c.Health
			}
			out = append(out, m)
		}
		writeJSON(w, http.StatusOK, out)
	case "/api/containers/stats":
		e := env()
		out := []map[string]any{}
		if e != nil {
			for _, c := range e.Containers {
				if c.State != "running" {
					continue
				}
				out = append(out, map[string]any{"id": c.ID, "name": c.Name, "stack": nil, "cpuPercent": c.CPU,
					"memoryUsage": c.Mem, "memoryRaw": c.Mem, "memoryCache": 0, "memoryLimit": c.Limit,
					"memoryPercent": float64(c.Mem) / float64(c.Limit) * 100, "networkRx": 12_000_000_000, "networkTx": 3_400_000_000,
					"blockRead": 0, "blockWrite": 0})
			}
		}
		writeJSON(w, http.StatusOK, out)
	default:
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Not found"})
			return
		}
		w.Write([]byte("<!doctype html><title>Dockhand</title>"))
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
