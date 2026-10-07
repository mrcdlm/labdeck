package main

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
)

// hookRecorder nimmt Webhooks unter /hook/… an und zeichnet sie auf (für die
// Browser-Tests der Benachrichtigungen).
type hookRecorder struct {
	mu   sync.Mutex
	reqs []map[string]any
}

func (h *hookRecorder) mount(mux *http.ServeMux) {
	mux.HandleFunc("/hook/", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(io.LimitReader(r.Body, 64<<10))
		h.mu.Lock()
		h.reqs = append(h.reqs, map[string]any{"method": r.Method, "path": r.URL.Path, "headers": r.Header, "body": string(b)})
		h.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /_hooks", func(w http.ResponseWriter, _ *http.Request) {
		h.mu.Lock()
		defer h.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		out := h.reqs
		if out == nil {
			out = []map[string]any{}
		}
		json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("DELETE /_hooks", func(w http.ResponseWriter, _ *http.Request) {
		h.mu.Lock()
		h.reqs = nil
		h.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
}
