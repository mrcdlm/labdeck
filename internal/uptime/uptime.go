// Package uptime berechnet Uptime-Balken aus Statuswechseln (z. B.
// Healthchecks-Flips) – für Checks und Backups gleichermaßen.
package uptime

import (
	"slices"
	"time"

	"github.com/mrcdlm/labdeck/internal/integrations"
)

// Uptime beschreibt einen Zeitraum als Balken aus gleich langen Abschnitten.
type Uptime struct {
	// Percent: Anteil erreichbarer Zeit (ok, warning) an der bekannten Zeit;
	// nil, wenn im Zeitraum nichts bekannt ist.
	Percent *float64 `json:"percent"`
	// Observed: Anteil bekannter Zeit am Zeitraum (0..1).
	Observed float64   `json:"observed"`
	Start    time.Time `json:"start"`
	// BucketSeconds: Länge eines Abschnitts.
	BucketSeconds int `json:"bucket_seconds"`
	// Buckets: schlechtester Status je Abschnitt (unknown = keine Daten).
	Buckets []integrations.Status `json:"buckets"`
}

// Range ist ein Zeitraum der Balken.
type Range struct {
	Key     string
	Length  time.Duration
	Buckets int
}

// Ranges: 24 h in halben Stunden, 7 Tage in 3 Stunden, 30 Tage in 12 Stunden.
var Ranges = []Range{
	{Key: "24h", Length: 24 * time.Hour, Buckets: 48},
	{Key: "7d", Length: 7 * 24 * time.Hour, Buckets: 56},
	{Key: "30d", Length: 30 * 24 * time.Hour, Buckets: 60},
}

// MaxRange ist der längste Zeitraum (so weit reicht der Verlauf der Quellen).
const MaxRange = 30 * 24 * time.Hour

// severity: in einem Abschnitt mit mehreren Status gewinnt der schlechteste.
var severity = map[integrations.Status]int{
	integrations.StatusUnknown: 0, integrations.StatusOK: 1, integrations.StatusPaused: 2,
	integrations.StatusWarning: 3, integrations.StatusError: 4,
}

func worse(a, b integrations.Status) bool { return severity[a] > severity[b] }

func isUp(s integrations.Status) bool {
	return s == integrations.StatusOK || s == integrations.StatusWarning
}

func known(s integrations.Status) bool { return isUp(s) || s == integrations.StatusError }

type segment struct {
	start, end time.Time
	status     integrations.Status
}

// segments macht aus den Wechseln zusammenhängende Abschnitte ab since bis to.
// Vor dem ersten Wechsel gilt dessen From; ohne Wechsel der aktuelle Status
// (eine laufende Karenzzeit zählt dann rückwirkend als ok).
func segments(history []integrations.Transition, since time.Time, current integrations.Status, to time.Time) []segment {
	h := slices.Clone(history)
	slices.SortStableFunc(h, func(a, b integrations.Transition) int { return a.At.Compare(b.At) })

	if len(h) == 0 {
		st := current
		if st == integrations.StatusWarning {
			st = integrations.StatusOK
		}
		return []segment{{since, to, st}}
	}
	out := []segment{{since, h[0].At, h[0].From}}
	for i, t := range h {
		end := to
		if i+1 < len(h) {
			end = h[i+1].At
		}
		out = append(out, segment{t.At, end, t.To})
	}
	return out
}

// Compute verteilt den Verlauf auf n Abschnitte im Zeitraum [from, to].
// since ist der Beginn des Verlaufs (davor: unbekannt); current färbt den
// letzten Abschnitt mit ein (z. B. Karenzzeit, pausiert).
func Compute(history []integrations.Transition, since time.Time, current integrations.Status, from, to time.Time, n int) Uptime {
	bucket := to.Sub(from) / time.Duration(n)
	u := Uptime{Start: from, BucketSeconds: int(bucket.Seconds()), Buckets: make([]integrations.Status, n)}
	for i := range u.Buckets {
		u.Buckets[i] = integrations.StatusUnknown
	}

	var up, seen time.Duration
	for _, seg := range segments(history, since, current, to) {
		s, e := later(seg.start, from), earlier(seg.end, to)
		if !e.After(s) || !known(seg.status) && seg.status != integrations.StatusPaused {
			continue
		}
		if known(seg.status) {
			seen += e.Sub(s)
			if isUp(seg.status) {
				up += e.Sub(s)
			}
		}
		first := int(s.Sub(from) / bucket)
		last := int((e.Sub(from) - 1) / bucket)
		for b := max(first, 0); b <= min(last, n-1); b++ {
			if worse(seg.status, u.Buckets[b]) {
				u.Buckets[b] = seg.status
			}
		}
	}
	if n > 0 && worse(current, u.Buckets[n-1]) {
		u.Buckets[n-1] = current
	}
	if seen > 0 {
		p := float64(up) / float64(seen) * 100
		u.Percent = &p
	}
	u.Observed = float64(seen) / float64(to.Sub(from))
	return u
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func earlier(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// All berechnet alle Zeiträume bis now; since nil = kein Verlauf (nil).
func All(history []integrations.Transition, since *time.Time, current integrations.Status, now time.Time) map[string]Uptime {
	if since == nil {
		return nil
	}
	out := make(map[string]Uptime, len(Ranges))
	for _, r := range Ranges {
		out[r.Key] = Compute(history, *since, current, now.Add(-r.Length), now, r.Buckets)
	}
	return out
}
