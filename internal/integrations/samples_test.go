package integrations

import (
	"testing"
	"time"
)

// Rohwerte höchstens einmal je Minute je Kennzahl; neue Kennzahlen sofort.
func TestSamplesThrottle(t *testing.T) {
	m := &Manager{sampled: map[int64]map[string]time.Time{}}
	t0 := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	raw := func(at time.Time, keys ...string) []bool {
		var ms []Metric
		for _, k := range keys {
			ms = append(ms, Metric{Key: k})
		}
		var out []bool
		for _, s := range m.samples(1, ms, at) {
			out = append(out, !s.NoRaw)
		}
		return out
	}
	if r := raw(t0, "a"); !r[0] {
		t.Error("erster Wert")
	}
	if r := raw(t0.Add(10*time.Second), "a", "b"); r[0] || !r[1] {
		t.Errorf("nach 10 s: %v", r)
	}
	if r := raw(t0.Add(56*time.Second), "a", "b"); !r[0] || r[1] {
		t.Errorf("nach 56 s: %v", r)
	}
	if r := raw(t0.Add(56*time.Second), "a"); r[0] {
		t.Error("gleicher Zeitpunkt doppelt")
	}
}
