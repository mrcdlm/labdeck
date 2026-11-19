// Package netbirdtest bildet die benötigten Teile der NetBird-Management-API
// nach (Peers, Reverse-Proxy-Dienste und -Cluster, Version) – für Tests und
// den Entwicklungs-Mock.
package netbirdtest

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Peer ist ein nachgebildeter NetBird-Peer.
type Peer struct {
	ID, Name, IP, OS string
	Groups           []string
	Connected        bool
	LastSeen         time.Time
	LoginExpired     bool
}

// Service ist ein Reverse-Proxy-Dienst.
type Service struct {
	ID, Domain, Status, Cluster string // Status: active, pending, certificate_failed …
	Target                      string // host
	Port                        int
	Enabled                     bool
}

type Fake struct {
	mu        sync.Mutex
	token     string
	Peers     []Peer
	Services  []Service
	Offline   []string // Adressen nicht erreichbarer Proxy-Cluster
	NoProxy   bool     // ältere Version ohne Reverse Proxy (404)
	fail      int
	broken    bool
	delay     time.Duration
	requests  int
	startedAt time.Time
}

func New(token string) *Fake { return &Fake{token: token, startedAt: time.Now()} }

// Demo: Server (Gruppe „server“), Laptop und Handy, drei Proxy-Dienste.
func (f *Fake) Demo() *Fake {
	now := time.Now().UTC().Truncate(time.Second)
	f.Peers = []Peer{
		{ID: "p1", Name: "docker01", IP: "100.92.10.1", OS: "Linux 6.8 (Ubuntu 24.04)", Groups: []string{"All", "server"}, Connected: true, LastSeen: now},
		{ID: "p2", Name: "nas", IP: "100.92.10.2", OS: "Linux 6.12 (TrueNAS 25.04)", Groups: []string{"All", "server"}, Connected: true, LastSeen: now},
		{ID: "p3", Name: "pi-edge", IP: "100.92.10.3", OS: "Linux 6.6 (Raspbian 12)", Groups: []string{"All", "server"}, Connected: false, LastSeen: now.Add(-3 * time.Hour)},
		{ID: "p4", Name: "marco-laptop", IP: "100.92.20.1", OS: "Darwin 15.4", Groups: []string{"All", "clients"}, Connected: false, LastSeen: now.Add(-26 * time.Hour)},
		{ID: "p5", Name: "pixel-9", IP: "100.92.20.2", OS: "Android 16", Groups: []string{"All", "clients"}, Connected: true, LastSeen: now},
	}
	f.Services = []Service{
		{ID: "s1", Domain: "photos.example.org", Status: "active", Cluster: "eu.proxy.netbird.io", Target: "docker01", Port: 2283, Enabled: true},
		{ID: "s2", Domain: "docs.example.org", Status: "active", Cluster: "eu.proxy.netbird.io", Target: "docker01", Port: 8000, Enabled: true},
		{ID: "s3", Domain: "ha.example.org", Status: "certificate_failed", Cluster: "eu.proxy.netbird.io", Target: "pi-edge", Port: 8123, Enabled: true},
	}
	return f
}

// Update ändert den Zustand (Mock-Steuerung, Tests).
func (f *Fake) Update(fn func(*Fake)) { f.mu.Lock(); fn(f); f.mu.Unlock() }

func (f *Fake) Fail(status int)       { f.mu.Lock(); f.fail = status; f.mu.Unlock() }
func (f *Fake) Broken(b bool)         { f.mu.Lock(); f.broken = b; f.mu.Unlock() }
func (f *Fake) Delay(d time.Duration) { f.mu.Lock(); f.delay = d; f.mu.Unlock() }
func (f *Fake) Requests() int         { f.mu.Lock(); defer f.mu.Unlock(); return f.requests }

func (f *Fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests++
	fail, broken, delay, token, noProxy := f.fail, f.broken, f.delay, f.token, f.NoProxy
	peers := append([]Peer(nil), f.Peers...)
	services := append([]Service(nil), f.Services...)
	offline := append([]string(nil), f.Offline...)
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
		w.Write([]byte("<!doctype html><title>NetBird</title>"))
		return
	}
	if r.Header.Get("Authorization") != "Token "+token {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"message": "token invalid", "code": 401})
		return
	}
	if broken {
		w.Write([]byte(`[{"id":`))
		return
	}
	switch r.URL.Path {
	case "/api/peers":
		out := []any{}
		for _, p := range peers {
			gs := []any{}
			for i, g := range p.Groups {
				gs = append(gs, map[string]any{"id": "g" + string(rune('0'+i)), "name": g, "peers_count": 3, "resources_count": 0, "issued": "api"})
			}
			label := p.Name + ".netbird.cloud"
			out = append(out, map[string]any{"id": p.ID, "name": p.Name, "ip": p.IP, "connected": p.Connected,
				"last_seen": p.LastSeen.Format(time.RFC3339Nano), "os": p.OS, "version": "0.66.2", "groups": gs,
				"ssh_enabled": false, "hostname": p.Name, "dns_label": label, "login_expiration_enabled": true,
				"login_expired": p.LoginExpired, "approval_required": false, "country_code": "DE", "city_name": "Berlin",
				"ephemeral": false, "accessible_peers_count": len(peers) - 1, "created_at": "2026-01-10T09:00:00Z"})
		}
		writeJSON(w, http.StatusOK, out)
	case "/api/reverse-proxies/services":
		if noProxy {
			writeJSON(w, http.StatusNotFound, map[string]any{"message": "not found", "code": 404})
			return
		}
		out := []any{}
		for _, s := range services {
			out = append(out, map[string]any{"id": s.ID, "name": s.Domain, "domain": s.Domain, "mode": "http",
				"proxy_cluster": s.Cluster, "enabled": s.Enabled, "terminated": false, "pass_host_header": true,
				"targets": []any{map[string]any{"target_id": "p1", "target_type": "peer", "host": s.Target, "port": s.Port,
					"protocol": "http", "path": "/", "enabled": true}},
				"meta": map[string]any{"created_at": "2026-09-01T10:00:00Z", "certificate_issued_at": "2026-11-01T10:00:00Z",
					"status": s.Status}, "private": false})
		}
		writeJSON(w, http.StatusOK, out)
	case "/api/reverse-proxies/clusters":
		if noProxy {
			writeJSON(w, http.StatusNotFound, map[string]any{"message": "not found", "code": 404})
			return
		}
		seen := map[string]bool{}
		out := []any{}
		for _, s := range services {
			if seen[s.Cluster] {
				continue
			}
			seen[s.Cluster] = true
			on := true
			for _, o := range offline {
				on = on && o != s.Cluster
			}
			out = append(out, map[string]any{"id": "c-" + s.Cluster, "address": s.Cluster, "type": "shared", "online": on,
				"connected_proxies": 3})
		}
		writeJSON(w, http.StatusOK, out)
	case "/api/instance/version":
		writeJSON(w, http.StatusOK, map[string]any{"management_current_version": "0.66.2",
			"management_available_version": "0.66.2", "management_update_available": false})
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"message": "not found", "code": 404})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
