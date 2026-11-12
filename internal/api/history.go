package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/store"
)

type historyStore interface {
	ListHistory(ctx context.Context, f store.HistoryFilter) ([]store.HistoryEntry, error)
	ProblemSince(ctx context.Context) (map[store.ProblemKey]time.Time, error)
}

type historyEntryDTO struct {
	ID              int64     `json:"id"`
	IntegrationID   int64     `json:"integration_id"`
	IntegrationName string    `json:"integration_name"`
	IntegrationType string    `json:"integration_type"`
	Kind            string    `json:"kind"` // integration, check, backup, metric
	ItemID          string    `json:"item_id,omitempty"`
	ItemName        string    `json:"item_name"`
	ItemGroup       string    `json:"item_group,omitempty"`
	From            string    `json:"from"`
	To              string    `json:"to"`
	Message         string    `json:"message,omitempty"`
	At              time.Time `json:"at"`
}

// handleHistory: GET /api/history?integration=1&kind=check&problems=1&before=123&limit=50
func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.HistoryFilter{Kind: q.Get("kind"), Problems: q.Get("problems") == "1"}
	switch f.Kind {
	case "", "integration", "check", "backup", "metric":
	default:
		writeMsg(w, r, http.StatusBadRequest, i18n.M("api.bad_request"))
		return
	}
	for key, dst := range map[string]*int64{"integration": &f.IntegrationID, "before": &f.BeforeID} {
		if v := q.Get(key); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n < 0 {
				writeMsg(w, r, http.StatusBadRequest, i18n.M("api.bad_request"))
				return
			}
			*dst = n
		}
	}
	f.Limit, _ = strconv.Atoi(q.Get("limit"))
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	list, err := s.history.ListHistory(r.Context(), f)
	if err != nil {
		s.internalError(w, r, "Verlauf laden", err)
		return
	}
	lang := i18n.FromRequest(r)
	out := make([]historyEntryDTO, 0, len(list))
	for _, e := range list {
		out = append(out, historyEntryDTO{ID: e.ID, IntegrationID: e.IntegrationID, IntegrationName: e.IntegrationName,
			IntegrationType: e.IntegrationType, Kind: e.Kind, ItemID: e.ItemID, ItemName: e.ItemName,
			ItemGroup: e.ItemGroup, From: e.From, To: e.To, Message: i18n.T(lang, i18n.Decode(e.Message)), At: e.At})
	}
	// Weitere Einträge vorhanden? (eine Seite voll)
	writeJSON(w, http.StatusOK, map[string]any{"entries": out, "more": len(out) == f.Limit})
}
