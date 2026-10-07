// Package glancestest bildet die REST-API v4 von Glances nach (Host-Werte und
// Docker-Container) – für Tests und den Entwicklungs-Mock.
package glancestest

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Container ist ein nachgebildeter Docker-Container.
type Container struct {
	Name, Image, Status string // running, healthy, unhealthy, exited …
	CPU                 float64
	Mem, Limit          int64
}

type Fake struct {
	mu         sync.Mutex
	user, pw   string // Basic Auth (leer = keine)
	containers []Container
	noDocker   bool
	cpu        float64
	fail       int
	broken     bool
	delay      time.Duration
	requests   int
}

func New(user, pw string) *Fake {
	return &Fake{user: user, pw: pw, cpu: 23.5, containers: []Container{
		{Name: "jellyfin", Image: "jellyfin/jellyfin:10.10", Status: "healthy", CPU: 41.2, Mem: 1_400_000_000, Limit: 32_000_000_000},
		{Name: "paperless", Image: "ghcr.io/paperless-ngx/paperless-ngx:2.15", Status: "running", CPU: 1.8, Mem: 820_000_000, Limit: 32_000_000_000},
		{Name: "vaultwarden", Image: "vaultwarden/server:1.34", Status: "unhealthy", CPU: 0.1, Mem: 64_000_000, Limit: 32_000_000_000},
		{Name: "old-wiki", Image: "requarks/wiki:2", Status: "exited"},
	}}
}

// SetCPU ändert die CPU-Last des Hosts.
func (f *Fake) SetCPU(v float64) { f.mu.Lock(); f.cpu = v; f.mu.Unlock() }

// NoDocker: Container-Plugin deaktiviert (404).
func (f *Fake) NoDocker(b bool) { f.mu.Lock(); f.noDocker = b; f.mu.Unlock() }

func (f *Fake) Fail(status int)       { f.mu.Lock(); f.fail = status; f.mu.Unlock() }
func (f *Fake) Broken(b bool)         { f.mu.Lock(); f.broken = b; f.mu.Unlock() }
func (f *Fake) Delay(d time.Duration) { f.mu.Lock(); f.delay = d; f.mu.Unlock() }
func (f *Fake) Requests() int         { f.mu.Lock(); defer f.mu.Unlock(); return f.requests }

func (f *Fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests++
	fail, broken, delay, noDocker, cpu := f.fail, f.broken, f.delay, f.noDocker, f.cpu
	containers := append([]Container(nil), f.containers...)
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
	if f.user != "" && r.Header.Get("Authorization") != "Basic "+base64.StdEncoding.EncodeToString([]byte(f.user+":"+f.pw)) {
		w.Header().Set("WWW-Authenticate", `Basic realm="glances"`)
		http.Error(w, `{"detail":"Incorrect username or password"}`, http.StatusUnauthorized)
		return
	}
	if broken {
		w.Write([]byte(`{"total": `))
		return
	}
	switch strings.TrimPrefix(r.URL.Path, "/api/4") {
	case "/cpu":
		writeJSON(w, map[string]any{"total": cpu, "user": cpu * 0.7, "system": cpu * 0.3, "idle": 100 - cpu, "cpucore": 8})
	case "/mem":
		writeJSON(w, map[string]any{"total": 32_000_000_000, "used": 19_840_000_000, "available": 12_160_000_000, "percent": 62.0})
	case "/load":
		writeJSON(w, map[string]any{"min1": 1.2, "min5": 0.9, "min15": 0.7, "cpucore": 8})
	case "/system":
		writeJSON(w, map[string]any{"hostname": "docker01", "hr_name": "Debian GNU/Linux 13 64bit / Linux 6.12", "os_name": "Linux"})
	case "/uptime":
		writeJSON(w, "12 days, 3:04:05")
	case "/fs":
		writeJSON(w, []map[string]any{
			{"device_name": "/dev/sda1", "mnt_point": "/", "fs_type": "ext4", "size": 250_000_000_000, "used": 120_000_000_000, "free": 130_000_000_000, "percent": 48.0},
			{"device_name": "/dev/sdb1", "mnt_point": "/srv/data", "fs_type": "ext4", "size": 4_000_000_000_000, "used": 3_720_000_000_000, "free": 280_000_000_000, "percent": 93.0},
			{"device_name": "overlay", "mnt_point": "/var/lib/docker/overlay2/x/merged", "fs_type": "overlay", "size": 1, "used": 1, "percent": 100.0},
		})
	case "/containers":
		if noDocker {
			http.Error(w, `{"detail":"Unknown plugin containers"}`, http.StatusNotFound)
			return
		}
		out := []map[string]any{}
		for _, c := range containers {
			m := map[string]any{"name": c.Name, "id": strings.Repeat("a", 12) + c.Name, "image": c.Image, "status": c.Status,
				"engine": "docker", "cpu_percent": nil, "memory_usage": nil, "uptime": nil, "network_rx": nil, "network_tx": nil}
			if c.Status != "exited" {
				m["cpu_percent"], m["memory_usage"], m["memory_limit"], m["network_rx"], m["network_tx"], m["uptime"] =
					c.CPU, c.Mem, c.Limit, 12_000, 3_400, "3 days"
			}
			out = append(out, m)
		}
		writeJSON(w, out)
	default:
		http.Error(w, `{"detail":"Not Found"}`, http.StatusNotFound)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
