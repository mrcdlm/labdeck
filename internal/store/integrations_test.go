package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestIntegrationsCRUD(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	in := Integration{Type: "healthchecks", Name: "HC", Enabled: true, Interval: time.Minute,
		Timeout: 10 * time.Second, TLSVerify: true, Config: `{"base_url":"https://hc"}`}

	var sealedFor int64
	it, err := s.CreateIntegration(ctx, in, func(id int64) (string, error) {
		sealedFor = id
		return "v1:abc", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if it.ID == 0 || sealedFor != it.ID || it.Secrets != "v1:abc" || it.Status != "unknown" ||
		it.Interval != time.Minute || !it.TLSVerify || it.LastPollAt != nil {
		t.Fatalf("angelegt: %+v", it)
	}

	// seal schlägt fehl → nichts gespeichert
	if _, err := s.CreateIntegration(ctx, in, func(int64) (string, error) { return "", errors.New("kein Schlüssel") }); err == nil {
		t.Fatal("Fehler erwartet")
	}
	if list, _ := s.ListIntegrations(ctx); len(list) != 1 {
		t.Fatalf("nach Fehlschlag: %d", len(list))
	}

	it.Name, it.Enabled, it.TLSVerify, it.Secrets = "Neu", false, false, "v1:def"
	upd, err := s.UpdateIntegration(ctx, it)
	if err != nil || upd.Name != "Neu" || upd.Enabled || upd.TLSVerify || upd.Secrets != "v1:def" {
		t.Fatalf("geändert: %+v %v", upd, err)
	}
	if _, err := s.UpdateIntegration(ctx, Integration{ID: 999}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unbekannt: %v", err)
	}

	if err := s.DeleteIntegration(ctx, it.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetIntegration(ctx, it.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("nach Löschen: %v", err)
	}
	if err := s.DeleteIntegration(ctx, it.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("doppelt gelöscht: %v", err)
	}
}

func TestPollResults(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	it, _ := s.CreateIntegration(ctx, Integration{Type: "x", Name: "X", Enabled: true, Interval: time.Minute,
		Timeout: time.Second, Config: "{}"}, nil)
	t0 := time.Date(2026, 3, 1, 23, 0, 0, 0, time.UTC)

	err := s.SavePollResult(ctx, it.ID, PollResult{At: t0, Duration: 120 * time.Millisecond, Status: "ok",
		Snapshot: `{"metrics":[]}`,
		Changes:  []StatusChange{{Kind: "integration", From: "unknown", To: "ok", At: t0}},
		Samples:  []MetricSample{{Key: "photos", Value: 10}}})
	if err != nil {
		t.Fatal(err)
	}
	// gleicher Tag: min/max/last fortschreiben
	s.SavePollResult(ctx, it.ID, PollResult{At: t0.Add(30 * time.Minute), Status: "ok", Snapshot: "{}",
		Samples: []MetricSample{{Key: "photos", Value: 5}}})
	// Fehlschlag am nächsten Tag: Snapshot bleibt
	s.SavePollResult(ctx, it.ID, PollResult{At: t0.Add(2 * time.Hour), Status: "error", Error: `{"code":"x"}`,
		Changes: []StatusChange{{Kind: "integration", From: "ok", To: "error", Message: `{"code":"x"}`, At: t0.Add(2 * time.Hour)}},
		Samples: []MetricSample{{Key: "photos", Value: 12}}})

	got, _ := s.GetIntegration(ctx, it.ID)
	if got.Status != "error" || got.Snapshot != "{}" || got.LastError != `{"code":"x"}` ||
		!got.LastPollAt.Equal(t0.Add(2*time.Hour)) || !got.LastSuccessAt.Equal(t0.Add(30*time.Minute)) {
		t.Fatalf("Zustand: %+v", got)
	}

	days, _ := s.MetricDaily(ctx, it.ID, "photos", "2026-01-01")
	if len(days) != 2 || days[0] != (DailyValue{Day: "2026-03-01", Last: 5, Min: 5, Max: 10}) || days[1].Last != 12 {
		t.Fatalf("Tageswerte: %+v", days)
	}

	changes, _ := s.ListStatusChanges(ctx, it.ID, 10)
	if len(changes) != 2 || changes[0].To != "error" || changes[1].To != "ok" || changes[0].IntegrationID != it.ID {
		t.Fatalf("Wechsel: %+v", changes)
	}

	if err := s.SavePollResult(ctx, 999, PollResult{At: t0, Status: "ok"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("gelöschte Integration: %v", err)
	}

	// Rohwert einer Ressource: wird früher gelöscht
	if err := s.SavePollResult(ctx, it.ID, PollResult{At: t0.Add(-time.Minute), Status: "ok",
		Samples: []MetricSample{{Key: "res/vm/cpu", Value: 3, Short: true}}}); err != nil {
		t.Fatal(err)
	}
	if r, _ := s.PurgeOldData(ctx, t0.Add(-time.Hour), t0, t0.AddDate(-1, 0, 0)); r.Short != 1 || r.Samples != 0 {
		t.Fatalf("kurze Aufbewahrung: %+v", r)
	}

	// Aufräumen: Rohdaten nach 1 h, Tageswerte/Wechsel vor dem 2. März
	r, err := s.PurgeOldData(ctx, t0.Add(time.Hour), t0.Add(time.Hour), time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC))
	if err != nil || r.Samples != 2 || r.Daily != 2 || r.Changes != 1 { // Tageswerte: photos + res/vm/cpu
		t.Fatalf("purge: %+v %v", r, err)
	}

	// Löschen der Integration entfernt Verlauf und Kennzahlen
	s.DeleteIntegration(ctx, it.ID)
	if c, _ := s.ListStatusChanges(ctx, 0, 10); len(c) != 0 {
		t.Fatalf("Wechsel nach Löschen: %+v", c)
	}
	if d, _ := s.MetricDaily(ctx, it.ID, "photos", ""); len(d) != 0 {
		t.Fatalf("Tageswerte nach Löschen: %+v", d)
	}
}

func TestProblemSince(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	it, _ := s.CreateIntegration(ctx, Integration{Type: "x", Name: "X", Enabled: true, Interval: time.Minute,
		Timeout: time.Second, Config: "{}"}, nil)
	t0 := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	err := s.AddStatusChanges(ctx, it.ID, []StatusChange{
		// Kennzahl: ok → Warnung → Fehler; Beginn bleibt bei der Warnung
		{Kind: "metric", ItemID: "disk", From: "ok", To: "warning", At: t0},
		{Kind: "metric", ItemID: "disk", From: "warning", To: "error", At: t0.Add(time.Hour)},
		// Check: fällt aus, erholt sich, fällt erneut aus; Beginn = zweiter Ausfall
		{Kind: "check", ItemID: "c1", From: "new", To: "error", At: t0},
		{Kind: "check", ItemID: "c1", From: "error", To: "ok", At: t0.Add(time.Hour)},
		{Kind: "check", ItemID: "c1", From: "ok", To: "error", At: t0.Add(2 * time.Hour)},
		// Integration ohne Element
		{Kind: "integration", From: "ok", To: "error", At: t0.Add(3 * time.Hour)},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.ProblemSince(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := map[ProblemKey]time.Time{
		{it.ID, "metric", "disk"}:  t0,
		{it.ID, "check", "c1"}:     t0.Add(2 * time.Hour),
		{it.ID, "integration", ""}: t0.Add(3 * time.Hour),
	}
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for k, w := range want {
		if !got[k].Equal(w) {
			t.Errorf("%v: %v, erwartet %v", k, got[k], w)
		}
	}
}
