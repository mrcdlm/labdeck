// Package promtest bildet GET /api/v1/query von Prometheus nach – für Tests
// und den Entwicklungs-Mock. Es versteht keine echte PromQL, sondern kennt
// feste Abfragen (Series), wie sie ein HomeLab typischerweise stellt.
package promtest

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Sample ist eine Zeitreihe im Ergebnis.
type Sample struct {
	Labels map[string]string
	Value  float64
}

type Fake struct {
	mu       sync.Mutex
	user, pw string // Basic Auth (leer = keine)
	token    string // Bearer (leer = keiner)
	series   map[string][]Sample
	scalars  map[string]float64
	fail     int
	broken   bool
	delay    time.Duration
	requests int
}

// New legt einen Fake an; Zugangsdaten leer = ohne Anmeldung.
func New(user, pw, token string) *Fake {
	return &Fake{user: user, pw: pw, token: token, series: map[string][]Sample{}, scalars: map[string]float64{}}
}

// Set legt das Ergebnis einer Abfrage fest (Vektor).
func (f *Fake) Set(query string, samples ...Sample) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.series[query] = samples
}

// SetScalar legt ein Skalar-Ergebnis fest.
func (f *Fake) SetScalar(query string, v float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scalars[query] = v
}

// Demo füllt typische Abfragen (node_exporter, restic-Exporter, SMART).
func (f *Fake) Demo(now time.Time) *Fake {
	f.Set(`100 - avg(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100`, Sample{Value: 12.4})
	f.Set(`node_filesystem_avail_bytes{mountpoint="/"} / node_filesystem_size_bytes{mountpoint="/"} * 100`,
		Sample{Labels: map[string]string{"instance": "nas:9100"}, Value: 38.2},
		Sample{Labels: map[string]string{"instance": "pve:9100"}, Value: 9.6})
	f.Set(`node_memory_MemAvailable_bytes`,
		Sample{Labels: map[string]string{"instance": "nas:9100"}, Value: 6.2e9})
	f.Set(`restic_backup_timestamp`,
		Sample{Labels: map[string]string{"client_hostname": "nas", "snapshot_hash": "a1b2"}, Value: float64(now.Add(-5 * time.Hour).Unix())},
		Sample{Labels: map[string]string{"client_hostname": "laptop", "snapshot_hash": "c3d4"}, Value: float64(now.Add(-40 * time.Hour).Unix())})
	f.Set(`up == 0`)
	f.Set(`max(smartctl_device_temperature)`, Sample{Value: 41})
	f.SetScalar(`scalar(count(up))`, 7)
	return f
}

func (f *Fake) Fail(status int)       { f.mu.Lock(); f.fail = status; f.mu.Unlock() }
func (f *Fake) Broken(b bool)         { f.mu.Lock(); f.broken = b; f.mu.Unlock() }
func (f *Fake) Delay(d time.Duration) { f.mu.Lock(); f.delay = d; f.mu.Unlock() }
func (f *Fake) Requests() int         { f.mu.Lock(); defer f.mu.Unlock(); return f.requests }

func (f *Fake) authorized(r *http.Request) bool {
	if f.user != "" {
		u, p, ok := r.BasicAuth()
		return ok && u == f.user && p == f.pw
	}
	if f.token != "" {
		return r.Header.Get("Authorization") == "Bearer "+f.token
	}
	return true
}

func (f *Fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests++
	delay, fail, broken, ok := f.delay, f.fail, f.broken, f.authorized(r)
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
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if r.URL.Path != "/api/v1/query" {
		http.Error(w, "404 page not found", http.StatusNotFound)
		return
	}
	if broken {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","data":{`))
		return
	}
	q := r.FormValue("query")
	ts := float64(time.Now().UnixMilli()) / 1000
	f.mu.Lock()
	samples, isVec := f.series[q]
	scalar, isScalar := f.scalars[q]
	f.mu.Unlock()
	switch {
	case isVec:
		res := []map[string]any{}
		for _, s := range samples {
			labels := s.Labels
			if labels == nil {
				labels = map[string]string{}
			}
			res = append(res, map[string]any{"metric": labels, "value": []any{ts, strconv.FormatFloat(s.Value, 'f', -1, 64)}})
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "success", "data": map[string]any{"resultType": "vector", "result": res}})
	case isScalar:
		writeJSON(w, http.StatusOK, map[string]any{"status": "success",
			"data": map[string]any{"resultType": "scalar", "result": []any{ts, strconv.FormatFloat(scalar, 'f', -1, 64)}}})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": "error", "errorType": "bad_data",
			"error": `invalid parameter "query": 1:1: parse error: unexpected identifier "` + q + `"`})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
