package api

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/live"
	"github.com/mrcdlm/labdeck/internal/store"
)

// metricStore liefert den Verlauf von Kennzahlen.
type metricStore interface {
	MetricSamples(ctx context.Context, integrationID int64, key string, from time.Time) ([]store.Sample, error)
	MetricDaily(ctx context.Context, integrationID int64, key, from string) ([]store.DailyValue, error)
}

// metricRange: 1 h mit jedem Wert (Live-Kacheln), kurze Zeiträume aus den
// Rohdaten (letzter Wert je Abschnitt), lange aus den Tageswerten.
type metricRange struct {
	length time.Duration
	bucket time.Duration // 0 = Tageswerte
	raw    bool          // jeder Wert, ohne Abschnitte
}

// livePoints liefert die Werte der letzten Stunde aus dem Speicher.
type livePoints interface {
	Points(id int64, key string, from time.Time) []live.Point
}

var metricRanges = map[string]metricRange{
	"1h":  {length: time.Hour, raw: true},
	"24h": {length: 24 * time.Hour, bucket: 15 * time.Minute},
	"7d":  {length: 7 * 24 * time.Hour, bucket: time.Hour},
	"30d": {length: 30 * 24 * time.Hour},
	"1y":  {length: 365 * 24 * time.Hour},
}

type pointDTO struct {
	T   time.Time `json:"t"`
	V   float64   `json:"v"`
	Min *float64  `json:"min,omitempty"`
	Max *float64  `json:"max,omitempty"`
}

type historyDTO struct {
	Key    string     `json:"key"`
	Name   string     `json:"name"`
	Group  string     `json:"group,omitempty"`
	Unit   string     `json:"unit,omitempty"`
	Range  string     `json:"range"`
	Points []pointDTO `json:"points"`
}

// bucketLast verdichtet Rohwerte auf den letzten Wert je Abschnitt.
func bucketLast(samples []store.Sample, bucket time.Duration) []pointDTO {
	out := []pointDTO{}
	for _, s := range samples {
		t := s.At.Truncate(bucket)
		if n := len(out); n > 0 && out[n-1].T.Equal(t) {
			out[n-1].V = s.Value
			continue
		}
		out = append(out, pointDTO{T: t, V: s.Value})
	}
	return out
}

// handleMetricHistory: GET /api/integrations/{id}/history?key=photos&range=30d
func (s *Server) handleMetricHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := integrationID(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	rangeKey := q.Get("range")
	if rangeKey == "" {
		rangeKey = "30d"
	}
	rg, ok := metricRanges[rangeKey]
	if !ok {
		writeMsg(w, r, http.StatusBadRequest, i18n.M("api.bad_request"))
		return
	}
	v, err := s.integrations.Get(r.Context(), id)
	if err != nil {
		s.integrationError(w, r, "Integration laden", err)
		return
	}
	key := q.Get("key")
	out := historyDTO{Key: key, Name: key, Range: rangeKey, Points: []pointDTO{}}
	if v.Snapshot != nil {
		i := slices.IndexFunc(v.Snapshot.Metrics, func(m integrations.Metric) bool { return m.Key == key })
		if i >= 0 {
			m := v.Snapshot.Metrics[i]
			out.Name, out.Group, out.Unit = m.Name, m.Group, m.Unit
		}
	}

	from := time.Now().Add(-rg.length)
	if rg.bucket > 0 || rg.raw {
		samples, err := s.metrics.MetricSamples(r.Context(), id, key, from)
		if err != nil {
			s.internalError(w, r, "Verlauf laden", err)
			return
		}
		samples = s.withLive(id, key, from, samples)
		if rg.raw {
			for _, x := range samples {
				out.Points = append(out.Points, pointDTO{T: x.At, V: x.Value})
			}
		} else {
			out.Points = bucketLast(samples, rg.bucket)
		}
	} else {
		days, err := s.metrics.MetricDaily(r.Context(), id, key, from.UTC().Format(time.DateOnly))
		if err != nil {
			s.internalError(w, r, "Verlauf laden", err)
			return
		}
		for _, d := range days {
			t, err := time.Parse(time.DateOnly, d.Day)
			if err != nil {
				continue
			}
			p := pointDTO{T: t, V: d.Last}
			if d.Min != d.Max {
				p.Min, p.Max = &d.Min, &d.Max
			}
			out.Points = append(out.Points, p)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// withLive ergänzt die Rohwerte der DB (höchstens einer je Minute) um die
// neueren Werte aus dem Speicher – so endet jedes Diagramm beim aktuellen Wert.
func (s *Server) withLive(id int64, key string, from time.Time, samples []store.Sample) []store.Sample {
	if s.live == nil {
		return samples
	}
	last := from
	if n := len(samples); n > 0 {
		last = samples[n-1].At
	}
	for _, p := range s.live.Points(id, key, from) {
		if p.At.After(last) {
			samples = append(samples, store.Sample{At: p.At, Value: p.Value})
		}
	}
	return samples
}
