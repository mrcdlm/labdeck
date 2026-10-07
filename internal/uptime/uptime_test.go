package uptime

import (
	"math"
	"testing"
	"time"

	"github.com/mrcdlm/labdeck/internal/integrations"
)

const (
	ok      = integrations.StatusOK
	warning = integrations.StatusWarning
	errS    = integrations.StatusError
	paused  = integrations.StatusPaused
	unknown = integrations.StatusUnknown
)

var now = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

func flip(ago time.Duration, from, to integrations.Status) integrations.Transition {
	return integrations.Transition{At: now.Add(-ago), From: from, To: to}
}

func pct(u Uptime) float64 {
	if u.Percent == nil {
		return math.NaN()
	}
	return *u.Percent
}

func TestNoHistoryUsesCurrentStatus(t *testing.T) {
	since := now.Add(-MaxRange)
	u := Compute(nil, since, ok, now.Add(-24*time.Hour), now, 24)
	if pct(u) != 100 || u.Observed != 1 || u.BucketSeconds != 3600 {
		t.Fatalf("%+v", u)
	}
	for i, b := range u.Buckets {
		if b != ok {
			t.Fatalf("bucket %d: %s", i, b)
		}
	}

	// Karenzzeit: rückwirkend ok, nur der letzte Abschnitt gelb
	u = Compute(nil, since, warning, now.Add(-24*time.Hour), now, 24)
	if pct(u) != 100 || u.Buckets[0] != ok || u.Buckets[23] != warning {
		t.Fatalf("grace: %+v", u)
	}

	// neu (noch nie gepingt): nichts bekannt
	u = Compute(nil, since, unknown, now.Add(-24*time.Hour), now, 24)
	if u.Percent != nil || u.Observed != 0 || u.Buckets[0] != unknown {
		t.Fatalf("new: %+v", u)
	}

	// pausiert: grau, zählt nicht zur Uptime
	u = Compute(nil, since, paused, now.Add(-24*time.Hour), now, 24)
	if u.Percent != nil || u.Buckets[5] != paused {
		t.Fatalf("paused: %+v", u)
	}
}

func TestFlips(t *testing.T) {
	since := now.Add(-MaxRange)
	// vor 10 h ausgefallen, vor 4 h wieder da
	h := []integrations.Transition{flip(4*time.Hour, errS, ok), flip(10*time.Hour, ok, errS)} // unsortiert
	u := Compute(h, since, ok, now.Add(-24*time.Hour), now, 24)
	if got := pct(u); math.Abs(got-75) > 1e-9 {
		t.Fatalf("percent %v", got)
	}
	for i, b := range u.Buckets {
		want := ok
		if i >= 14 && i < 20 {
			want = errS
		}
		if b != want {
			t.Errorf("bucket %d: %s, want %s", i, b, want)
		}
	}

	// Ausfall innerhalb eines Abschnitts färbt ihn rot (schlechtester gewinnt)
	h = []integrations.Transition{flip(90*time.Minute, ok, errS), flip(80*time.Minute, errS, ok)}
	u = Compute(h, since, ok, now.Add(-24*time.Hour), now, 24)
	if u.Buckets[22] != errS || u.Buckets[21] != ok || u.Buckets[23] != ok {
		t.Fatalf("%v", u.Buckets)
	}
}

func TestHistorySinceAndCurrentDown(t *testing.T) {
	// Verlauf erst seit 6 h bekannt, seit 2 h down
	since := now.Add(-6 * time.Hour)
	h := []integrations.Transition{flip(2*time.Hour, ok, errS)}
	u := Compute(h, since, errS, now.Add(-24*time.Hour), now, 24)
	if math.Abs(u.Observed-0.25) > 1e-9 || math.Abs(pct(u)-200.0/3) > 1e-9 {
		t.Fatalf("%+v", u)
	}
	if u.Buckets[17] != unknown || u.Buckets[18] != ok || u.Buckets[22] != errS || u.Buckets[23] != errS {
		t.Fatalf("%v", u.Buckets)
	}
}

func TestAll(t *testing.T) {
	if All(nil, nil, ok, now) != nil {
		t.Fatal("ohne Verlauf: nil erwartet")
	}
	since := now.Add(-MaxRange)
	all := All([]integrations.Transition{flip(3*24*time.Hour, errS, ok)}, &since, ok, now)
	if len(all) != 3 || len(all["24h"].Buckets) != 48 || len(all["7d"].Buckets) != 56 || len(all["30d"].Buckets) != 60 {
		t.Fatalf("%+v", all)
	}
	if pct(all["24h"]) != 100 || pct(all["7d"]) >= 100 || all["30d"].Buckets[0] != errS {
		t.Fatalf("24h %v, 7d %v, 30d %v", pct(all["24h"]), pct(all["7d"]), all["30d"].Buckets[0])
	}
}
