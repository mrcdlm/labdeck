// Package hctest bildet die Healthchecks.io-Management-API v3 nach – für
// Tests (httptest) und den Entwicklungs-Mock (cmd/mock).
package hctest

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Check ist ein nachgebildeter Check.
type Check struct {
	Name     string
	Slug     string
	Tags     string // durch Leerzeichen getrennt, wie in der API
	Desc     string
	Status   string // new, up, grace, down, paused
	Started  bool
	LastPing time.Duration // vor jetzt; 0 = nie
	Timeout  int
	Grace    int
	Schedule string
	TZ       string
	UUID     string
	Flips    []Flip
}

// Flip ist ein Statuswechsel (up=false: down).
type Flip struct {
	Ago time.Duration
	Up  bool
}

type Fake struct {
	mu       sync.Mutex
	keys     map[string]bool // Key → read-only?
	checks   []Check
	fail     int           // != 0: jede Anfrage liefert diesen Status
	broken   bool          // kaputtes JSON
	delay    time.Duration // Antwortverzögerung
	requests int
	now      func() time.Time
}

// New legt eine API mit einem Read-only-Key und einem Read-write-Key an.
func New(readOnlyKey, readWriteKey string, checks ...Check) *Fake {
	f := &Fake{keys: map[string]bool{}, checks: checks, now: time.Now}
	if readOnlyKey != "" {
		f.keys[readOnlyKey] = true
	}
	if readWriteKey != "" {
		f.keys[readWriteKey] = false
	}
	return f
}

// Demo liefert typische Checks eines HomeLabs.
func Demo() []Check {
	return []Check{
		{Name: "Restic NAS → B2", Slug: "restic-nas", Tags: "backup restic", Status: "up", LastPing: 3 * time.Hour,
			Timeout: 86400, Grace: 3600, UUID: "0b6f0c1e-1d5a-4d0e-9a51-6f1c1f6a0001",
			Flips: []Flip{{Ago: 50 * time.Hour, Up: false}, {Ago: 47 * time.Hour, Up: true}}},
		{Name: "Borg Laptop", Slug: "borg-laptop", Tags: "backup", Status: "grace", LastPing: 25 * time.Hour,
			Timeout: 86400, Grace: 7200, UUID: "0b6f0c1e-1d5a-4d0e-9a51-6f1c1f6a0002"},
		{Name: "Proxmox vzdump", Slug: "vzdump", Tags: "backup proxmox", Status: "down", LastPing: 52 * time.Hour,
			Schedule: "0 2 * * *", TZ: "Europe/Berlin", Grace: 3600, UUID: "0b6f0c1e-1d5a-4d0e-9a51-6f1c1f6a0003",
			Flips: []Flip{{Ago: 26 * time.Hour, Up: false}}},
		{Name: "Zertifikate erneuern", Slug: "certs", Tags: "cron", Status: "up", LastPing: 7 * time.Hour,
			Schedule: "0 3 * * *", TZ: "Europe/Berlin", Grace: 3600, UUID: "0b6f0c1e-1d5a-4d0e-9a51-6f1c1f6a0004"},
		{Name: "DynDNS", Slug: "dyndns", Tags: "cron dns", Status: "up", LastPing: 4 * time.Minute,
			Timeout: 300, Grace: 300, UUID: "0b6f0c1e-1d5a-4d0e-9a51-6f1c1f6a0005"},
		{Name: "Alter Sync-Job", Slug: "old-sync", Tags: "", Status: "paused", LastPing: 30 * 24 * time.Hour,
			Timeout: 3600, Grace: 600, UUID: "0b6f0c1e-1d5a-4d0e-9a51-6f1c1f6a0006"},
		{Name: "Neuer Check", Slug: "new-check", Tags: "", Status: "new",
			Timeout: 86400, Grace: 3600, UUID: "0b6f0c1e-1d5a-4d0e-9a51-6f1c1f6a0007"},
	}
}

// SetStatus ändert den Status eines Checks (nach Name); false = unbekannt.
func (f *Fake) SetStatus(name, status string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.checks {
		if f.checks[i].Name == name {
			f.checks[i].Status = status
			if status == "up" {
				f.checks[i].LastPing = time.Second
			}
			return true
		}
	}
	return false
}

// Fail lässt alle Anfragen mit status scheitern (0 = normal).
func (f *Fake) Fail(status int) { f.mu.Lock(); f.fail = status; f.mu.Unlock() }

// Broken liefert kaputtes JSON.
func (f *Fake) Broken(b bool) { f.mu.Lock(); f.broken = b; f.mu.Unlock() }

// Delay verzögert jede Antwort.
func (f *Fake) Delay(d time.Duration) { f.mu.Lock(); f.delay = d; f.mu.Unlock() }

// Requests liefert die Anzahl der API-Anfragen.
func (f *Fake) Requests() int { f.mu.Lock(); defer f.mu.Unlock(); return f.requests }

func (f *Fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests++
	delay, fail, broken := f.delay, f.fail, f.broken
	readOnly, known := f.keys[r.Header.Get("X-Api-Key")]
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
	if !known {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing api key"})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if broken {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"checks": [{"name": `))
		return
	}

	path := r.URL.Path
	switch {
	case path == "/api/v3/checks/":
		f.list(w, r, readOnly)
	case strings.HasPrefix(path, "/api/v3/checks/") && strings.HasSuffix(path, "/flips/"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/api/v3/checks/"), "/flips/")
		f.flips(w, r, id)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

// uniqueKey wie bei Healthchecks: stabil, aber ohne die UUID preiszugeben.
func uniqueKey(c Check) string {
	sum := sha1.Sum([]byte(c.UUID))
	return "uk" + hex.EncodeToString(sum[:])[:22]
}

func (f *Fake) list(w http.ResponseWriter, r *http.Request, readOnly bool) {
	tags := r.URL.Query()["tag"]
	slug := r.URL.Query().Get("slug")
	now := f.now()
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []map[string]any{}
	for _, c := range f.checks {
		if slug != "" && c.Slug != slug {
			continue
		}
		have := strings.Fields(c.Tags)
		if !containsAll(have, tags) {
			continue
		}
		m := map[string]any{
			"name": c.Name, "slug": c.Slug, "tags": c.Tags, "desc": c.Desc, "grace": c.Grace,
			"n_pings": 0, "status": c.Status, "started": c.Started, "last_ping": nil, "next_ping": nil,
			"manual_resume": false, "methods": "", "start_kw": "", "success_kw": "", "failure_kw": "",
			"filter_subject": false, "filter_body": false, "filter_http_body": false, "filter_default_fail": false,
			"badge_url": "https://healthchecks.example/badge/x.svg",
		}
		if c.LastPing > 0 {
			last := now.Add(-c.LastPing).UTC()
			m["last_ping"] = last.Format(time.RFC3339)
			m["n_pings"] = 42
			if c.Status != "paused" && c.Timeout > 0 {
				m["next_ping"] = last.Add(time.Duration(c.Timeout) * time.Second).Format(time.RFC3339)
			}
		}
		if c.Schedule != "" {
			m["schedule"], m["tz"] = c.Schedule, c.TZ
		} else {
			m["timeout"] = c.Timeout
		}
		if readOnly {
			m["unique_key"] = uniqueKey(c)
		} else {
			base := "https://healthchecks.example/api/v3/checks/" + c.UUID
			m["uuid"] = c.UUID
			m["ping_url"] = "https://hc-ping.example/" + c.UUID
			m["update_url"], m["pause_url"], m["resume_url"] = base, base+"/pause", base+"/resume"
			m["channels"] = ""
		}
		out = append(out, m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"checks": out})
}

func (f *Fake) flips(w http.ResponseWriter, r *http.Request, id string) {
	now := f.now()
	var since time.Time
	if s, err := strconv.Atoi(r.URL.Query().Get("seconds")); err == nil {
		since = now.Add(-time.Duration(s) * time.Second)
	}
	if s, err := strconv.ParseInt(r.URL.Query().Get("start"), 10, 64); err == nil {
		since = time.Unix(s, 0)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.checks {
		if c.UUID != id && uniqueKey(c) != id {
			continue
		}
		out := []map[string]any{}
		for _, fl := range c.Flips {
			at := now.Add(-fl.Ago)
			if !since.IsZero() && at.Before(since) {
				continue
			}
			up := 0
			if fl.Up {
				up = 1
			}
			out = append(out, map[string]any{"timestamp": at.UTC().Format(time.RFC3339), "up": up})
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

func containsAll(have, want []string) bool {
	for _, t := range want {
		if !slices.Contains(have, t) {
			return false
		}
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
