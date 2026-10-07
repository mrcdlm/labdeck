package main

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// jsonStatus ist eine beliebige JSON-Schnittstelle für die HTTP-JSON-
// Integration: Statistik wie bei Paperless und der Status eines Backups.
type jsonStatus struct {
	mu       sync.Mutex
	fail     int
	broken   bool
	delay    time.Duration
	requests int
}

func (j *jsonStatus) Fail(status int)       { j.mu.Lock(); j.fail = status; j.mu.Unlock() }
func (j *jsonStatus) Broken(b bool)         { j.mu.Lock(); j.broken = b; j.mu.Unlock() }
func (j *jsonStatus) Delay(d time.Duration) { j.mu.Lock(); j.delay = d; j.mu.Unlock() }
func (j *jsonStatus) Requests() int         { j.mu.Lock(); defer j.mu.Unlock(); return j.requests }

func (j *jsonStatus) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	j.mu.Lock()
	j.requests++
	fail, broken, delay := j.fail, j.broken, j.delay
	j.mu.Unlock()
	time.Sleep(delay)
	switch {
	case fail != 0:
		http.Error(w, http.StatusText(fail), fail)
	case r.Header.Get("Authorization") != "Token json-token":
		http.Error(w, `{"detail":"Invalid token."}`, http.StatusUnauthorized)
	case r.URL.Path != "/stats":
		http.NotFound(w, r)
	case broken:
		w.Write([]byte(`{"documents_total": `))
	default:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"documents_total": 1523, "inbox": 12, "character_count": 9_812_334,
			"storage": map[string]any{"used_pct": 67.5},
			"backup":  map[string]any{"last_success": time.Now().Add(-3 * time.Hour).UTC().Format(time.RFC3339), "status": "ok"},
		})
	}
}
