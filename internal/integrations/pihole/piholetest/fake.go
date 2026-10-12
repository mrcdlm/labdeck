// Package piholetest bildet die benötigten Teile der Pi-hole-v6-API nach
// (Anmeldung mit Sessions, Statistik, Blocking, Versionen) – für Tests und
// den Entwicklungs-Mock. Wie Pi-hole begrenzt der Fake die Zahl
// gleichzeitiger Sessions.
package piholetest

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// MaxSessions wie webserver.api.max_sessions in Pi-hole (Standard 16).
const MaxSessions = 16

type Fake struct {
	mu       sync.Mutex
	password string // leer = ohne Anmeldung
	validity int    // Sekunden
	sessions map[string]time.Time
	logins   int

	Queries, Blocked, Clients, Domains, Gravity float64
	GravityUpdated                              time.Time
	Blocking                                    string
	Timer                                       *float64
	Core, CoreRemote, FTL                       string
	Docker, DockerRemote                        string

	fail     int
	broken   bool
	delay    time.Duration
	requests int
}

// New erstellt einen Fake mit Beispielwerten.
func New(password string) *Fake {
	return &Fake{
		password: password, validity: 1800, sessions: map[string]time.Time{},
		Queries: 48213, Blocked: 7391, Clients: 23, Domains: 4127, Gravity: 182334,
		GravityUpdated: time.Now().Add(-26 * time.Hour), Blocking: "enabled",
		Core: "v6.1", CoreRemote: "v6.1", FTL: "v6.2.3",
	}
}

func (f *Fake) Fail(status int)       { f.mu.Lock(); f.fail = status; f.mu.Unlock() }
func (f *Fake) Broken(b bool)         { f.mu.Lock(); f.broken = b; f.mu.Unlock() }
func (f *Fake) Delay(d time.Duration) { f.mu.Lock(); f.delay = d; f.mu.Unlock() }
func (f *Fake) Requests() int         { f.mu.Lock(); defer f.mu.Unlock(); return f.requests }

// Logins zählt erfolgreiche Anmeldungen.
func (f *Fake) Logins() int { f.mu.Lock(); defer f.mu.Unlock(); return f.logins }

// Restart verwirft alle Sessions (wie ein Neustart von FTL).
func (f *Fake) Restart() { f.mu.Lock(); f.sessions = map[string]time.Time{}; f.mu.Unlock() }

// Update ändert Werte unter Sperre.
func (f *Fake) Update(fn func(f *Fake)) { f.mu.Lock(); fn(f); f.mu.Unlock() }

// SetValidity setzt die Gültigkeit neuer Sessions in Sekunden.
func (f *Fake) SetValidity(s int) { f.mu.Lock(); f.validity = s; f.mu.Unlock() }

type obj = map[string]any

func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func apiError(w http.ResponseWriter, status int, key, msg string) {
	write(w, status, obj{"error": obj{"key": key, "message": msg, "hint": nil}, "took": 0.001})
}

func (f *Fake) authorized(r *http.Request) bool {
	if f.password == "" {
		return true
	}
	exp, ok := f.sessions[r.Header.Get("X-FTL-SID")]
	if !ok || time.Now().After(exp) {
		return false
	}
	f.sessions[r.Header.Get("X-FTL-SID")] = time.Now().Add(time.Duration(f.validity) * time.Second)
	return true
}

func (f *Fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests++
	fail, broken, delay := f.fail, f.broken, f.delay
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
	if broken {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"queries": {`))
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	if r.URL.Path == "/api/auth" && r.Method == http.MethodPost {
		var in struct {
			Password string `json:"password"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		if f.password == "" {
			write(w, http.StatusOK, obj{"session": obj{"valid": true, "totp": false, "sid": nil, "validity": -1,
				"message": "no password set"}, "took": 0.001})
			return
		}
		if in.Password != f.password {
			apiError(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
			return
		}
		for sid, exp := range f.sessions {
			if time.Now().After(exp) {
				delete(f.sessions, sid)
			}
		}
		if len(f.sessions) >= MaxSessions {
			apiError(w, http.StatusTooManyRequests, "api_seats_exceeded", "API seats exceeded")
			return
		}
		b := make([]byte, 16)
		rand.Read(b)
		sid := base64.StdEncoding.EncodeToString(b)
		f.sessions[sid] = time.Now().Add(time.Duration(f.validity) * time.Second)
		f.logins++
		write(w, http.StatusOK, obj{"session": obj{"valid": true, "totp": false, "sid": sid, "csrf": "x",
			"validity": f.validity, "message": "app-password correct"}, "took": 0.001})
		return
	}
	if r.Method != http.MethodGet {
		apiError(w, http.StatusMethodNotAllowed, "bad_request", "Method not allowed")
		return
	}
	if !f.authorized(r) {
		apiError(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}
	switch r.URL.Path {
	case "/api/stats/summary":
		pct := 0.0
		if f.Queries > 0 {
			pct = f.Blocked / f.Queries * 100
		}
		write(w, http.StatusOK, obj{
			"queries": obj{"total": f.Queries, "blocked": f.Blocked, "percent_blocked": pct, "unique_domains": f.Domains,
				"forwarded": f.Queries * 0.6, "cached": f.Queries * 0.25, "frequency": 0.6},
			"clients": obj{"active": f.Clients, "total": f.Clients + 7},
			"gravity": obj{"domains_being_blocked": f.Gravity, "last_update": f.GravityUpdated.Unix()},
			"took":    0.002,
		})
	case "/api/dns/blocking":
		write(w, http.StatusOK, obj{"blocking": f.Blocking, "timer": f.Timer, "took": 0.001})
	case "/api/info/version":
		docker := obj{"local": nil, "remote": nil}
		if f.Docker != "" {
			docker = obj{"local": f.Docker, "remote": f.DockerRemote}
		}
		part := func(local, remote string) obj {
			return obj{"local": obj{"branch": "master", "version": local, "hash": "abc"}, "remote": obj{"version": remote, "hash": "abc"}}
		}
		write(w, http.StatusOK, obj{"version": obj{
			"core": part(f.Core, f.CoreRemote), "web": part("v6.1", "v6.1"), "ftl": part(f.FTL, f.FTL), "docker": docker,
		}, "took": 0.001})
	default:
		apiError(w, http.StatusNotFound, "not_found", "Not found")
	}
}
