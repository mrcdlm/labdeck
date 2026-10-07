// Package pbstest bildet die benötigten Teile der Proxmox-Backup-Server-API
// nach (Datastores, Namespaces, Snapshots) – für Tests und den Mock.
package pbstest

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Snapshot ist ein nachgebildeter Snapshot.
type Snapshot struct {
	NS      string // Namespace ("" = Wurzel)
	Type    string // vm, ct, host
	ID      string
	Ago     time.Duration
	Size    int64
	Verify  string // "", ok, failed
	Comment string
}

// Store ist ein Datastore.
type Store struct {
	Name        string
	Total, Used int64
	Error       string
	Snapshots   []Snapshot
}

type Fake struct {
	mu       sync.Mutex
	token    string // erwarteter Header-Wert nach "PBSAPIToken="
	stores   []Store
	fail     int
	broken   bool
	delay    time.Duration
	requests int
	now      func() time.Time
}

// New: token im Format user@realm!tokenid:secret.
func New(token string, stores ...Store) *Fake {
	return &Fake{token: token, stores: stores, now: time.Now}
}

const tb = int64(1_000_000_000_000)

// Demo: ein Datastore im LAN mit VMs, Containern und Host-Backup, einer offsite.
func Demo() []Store {
	day := 24 * time.Hour
	var local []Snapshot
	for i := range 5 {
		ago := time.Duration(i)*day + 5*time.Hour
		local = append(local,
			Snapshot{Type: "vm", ID: "100", Ago: ago, Size: 34_000_000_000, Verify: "ok", Comment: "homeassistant"},
			Snapshot{Type: "ct", ID: "101", Ago: ago + 10*time.Minute, Size: 2_100_000_000, Verify: "ok", Comment: "pihole"},
			Snapshot{Type: "vm", ID: "102", Ago: ago + 20*time.Minute, Size: 210_000_000_000, Comment: "nextcloud"},
		)
	}
	local[2].Verify = "failed" // jüngster Snapshot von vm/102: Verify fehlgeschlagen
	local = append(local,
		Snapshot{Type: "host", ID: "nas", Ago: 3*day + 2*time.Hour, Size: 820_000_000_000, Verify: "ok"},
		Snapshot{NS: "lab", Type: "ct", ID: "200", Ago: 7 * time.Hour, Size: 1_400_000_000, Comment: "testbox"},
	)
	return []Store{
		{Name: "local", Total: 4 * tb, Used: 2_920_000_000_000, Snapshots: local},
		{Name: "offsite", Total: 8 * tb, Used: 7_480_000_000_000, Snapshots: []Snapshot{
			{Type: "vm", ID: "100", Ago: 20 * time.Hour, Size: 34_000_000_000, Verify: "ok", Comment: "homeassistant"},
		}},
	}
}

func (f *Fake) Fail(status int)       { f.mu.Lock(); f.fail = status; f.mu.Unlock() }
func (f *Fake) Broken(b bool)         { f.mu.Lock(); f.broken = b; f.mu.Unlock() }
func (f *Fake) Delay(d time.Duration) { f.mu.Lock(); f.delay = d; f.mu.Unlock() }
func (f *Fake) Requests() int         { f.mu.Lock(); defer f.mu.Unlock(); return f.requests }

func (f *Fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests++
	fail, broken, delay := f.fail, f.broken, f.delay
	authOK := r.Header.Get("Authorization") == "PBSAPIToken="+f.token
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
	if !authOK {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"data": nil, "message": "authentication failed - invalid token\n"})
		return
	}
	if broken {
		w.Write([]byte(`{"data":[{"store":`))
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api2/json")
	switch {
	case path == "/version":
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"version": "4.0", "release": "14", "repoid": "abc"}})
	case path == "/status/datastore-usage":
		f.usage(w)
	case strings.HasPrefix(path, "/admin/datastore/"):
		rest := strings.Split(strings.TrimPrefix(path, "/admin/datastore/"), "/")
		if len(rest) != 2 {
			http.NotFound(w, r)
			return
		}
		f.datastore(w, r, rest[0], rest[1])
	default:
		writeJSON(w, http.StatusNotImplemented, map[string]any{"data": nil, "message": "Method '" + path + "' not implemented\n"})
	}
}

func (f *Fake) usage(w http.ResponseWriter) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []map[string]any{}
	for _, s := range f.stores {
		m := map[string]any{"store": s.Name, "backend-type": "filesystem", "mount-status": "nonremovable"}
		if s.Error != "" {
			m["error"] = s.Error
		} else {
			m["total"], m["used"], m["avail"] = s.Total, s.Used, s.Total-s.Used
		}
		out = append(out, m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

func (f *Fake) datastore(w http.ResponseWriter, r *http.Request, store, what string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var st *Store
	for i := range f.stores {
		if f.stores[i].Name == store {
			st = &f.stores[i]
		}
	}
	if st == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"data": nil, "message": "no such datastore '" + store + "'\n"})
		return
	}
	switch what {
	case "namespace":
		seen := map[string]bool{"": true}
		out := []map[string]string{{"ns": ""}}
		for _, s := range st.Snapshots {
			if !seen[s.NS] {
				seen[s.NS] = true
				out = append(out, map[string]string{"ns": s.NS})
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": out})
	case "snapshots":
		ns := r.URL.Query().Get("ns")
		now := f.now()
		out := []map[string]any{}
		for _, s := range st.Snapshots {
			if s.NS != ns {
				continue
			}
			m := map[string]any{"backup-type": s.Type, "backup-id": s.ID, "backup-time": now.Add(-s.Ago).Unix(),
				"size": s.Size, "protected": false, "owner": "root@pam", "files": []string{"index.json.blob"}}
			if s.Comment != "" {
				m["comment"] = s.Comment
			}
			if s.Verify != "" {
				m["verification"] = map[string]string{"state": s.Verify, "upid": "UPID:pbs:00000001:00000001:00000001:00000001:verify:x:root@pam:"}
			}
			out = append(out, m)
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": out})
	default:
		http.NotFound(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
