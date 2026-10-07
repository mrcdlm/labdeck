// Package immichtest bildet die benötigten Teile der Immich-API nach – für
// Tests (httptest) und den Entwicklungs-Mock (cmd/mock).
package immichtest

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// User ist ein nachgebildeter Benutzer.
type User struct {
	ID, Name       string
	Photos, Videos int64
	UsagePhotos    int64
	UsageVideos    int64
	Quota          *int64
}

type Fake struct {
	mu       sync.Mutex
	keys     map[string]bool // Key → hat server.statistics (Admin)
	users    []User
	version  [3]int
	fail     int
	broken   bool
	delay    time.Duration
	requests int
}

// New legt eine API mit einem Admin-Key (mit Berechtigung) und einem Key
// ohne Berechtigung an.
func New(adminKey, limitedKey string, users ...User) *Fake {
	f := &Fake{keys: map[string]bool{}, users: users, version: [3]int{1, 140, 1}}
	if adminKey != "" {
		f.keys[adminKey] = true
	}
	if limitedKey != "" {
		f.keys[limitedKey] = false
	}
	return f
}

const gb = int64(1_000_000_000)

// Demo liefert eine typische Familien-Instanz.
func Demo() []User {
	quota := 200 * gb
	return []User{
		{ID: "6f1c0b52-0d1e-4e6a-9d0e-000000000001", Name: "Marco", Photos: 31_502, Videos: 812,
			UsagePhotos: 190 * gb, UsageVideos: 120 * gb},
		{ID: "6f1c0b52-0d1e-4e6a-9d0e-000000000002", Name: "Anna", Photos: 16_711, Videos: 392,
			UsagePhotos: 70 * gb, UsageVideos: 32 * gb, Quota: &quota},
	}
}

// AddPhotos erhöht die Fotos des ersten Benutzers (für Verlaufstests).
func (f *Fake) AddPhotos(n int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.users) > 0 {
		f.users[0].Photos += n
		f.users[0].UsagePhotos += n * 4_000_000
	}
}

func (f *Fake) Fail(status int)       { f.mu.Lock(); f.fail = status; f.mu.Unlock() }
func (f *Fake) Broken(b bool)         { f.mu.Lock(); f.broken = b; f.mu.Unlock() }
func (f *Fake) Delay(d time.Duration) { f.mu.Lock(); f.delay = d; f.mu.Unlock() }
func (f *Fake) Requests() int         { f.mu.Lock(); defer f.mu.Unlock(); return f.requests }

func (f *Fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests++
	delay, fail, broken := f.delay, f.fail, f.broken
	admin, known := f.keys[r.Header.Get("x-api-key")]
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
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusNotFound, map[string]any{"message": "Not Found"})
		return
	}
	switch r.URL.Path {
	case "/api/server/version": // öffentlich
		f.mu.Lock()
		v := f.version
		f.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]int{"major": v[0], "minor": v[1], "patch": v[2]})
	case "/api/server/statistics":
		if !known {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"message": "Invalid API key", "statusCode": 401})
			return
		}
		if !admin {
			writeJSON(w, http.StatusForbidden, map[string]any{"message": "Missing required permission: server.statistics", "statusCode": 403})
			return
		}
		if broken {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"photos": 12, "usageByUser": [`))
			return
		}
		f.statistics(w)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"message": "Cannot GET " + r.URL.Path, "statusCode": 404})
	}
}

func (f *Fake) statistics(w http.ResponseWriter) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var photos, videos, up, uv int64
	users := []map[string]any{}
	for _, u := range f.users {
		photos += u.Photos
		videos += u.Videos
		up += u.UsagePhotos
		uv += u.UsageVideos
		users = append(users, map[string]any{"userId": u.ID, "userName": u.Name, "photos": u.Photos, "videos": u.Videos,
			"usage": u.UsagePhotos + u.UsageVideos, "usagePhotos": u.UsagePhotos, "usageVideos": u.UsageVideos,
			"quotaSizeInBytes": u.Quota})
	}
	writeJSON(w, http.StatusOK, map[string]any{"photos": photos, "videos": videos, "usage": up + uv,
		"usagePhotos": up, "usageVideos": uv, "usageByUser": users})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
