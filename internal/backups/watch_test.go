package backups

import (
	"context"
	"testing"
	"time"

	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/store"
)

func TestWatcher(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	it, _ := st.CreateIntegration(ctx, store.Integration{Type: "x", Name: "X", Enabled: true, Interval: time.Minute,
		Timeout: time.Second, Config: "{}"}, nil)

	clock := now
	last := now.Add(-3 * time.Hour)
	result := integrations.StatusOK
	var changes []Change
	w := &Watcher{Store: st, Now: func() time.Time { return clock },
		Load: func(_ context.Context, at time.Time) ([]Item, error) {
			return Evaluate([]Source{{IntegrationID: it.ID, IntegrationName: "X", Backups: []integrations.Backup{
				{ID: "b", Name: "Restic", Group: "nas", LastRun: &last, Result: result},
			}}}, nil, 26, at), nil
		},
		OnChange: func(c Change) { changes = append(changes, c) },
	}
	step := func(d time.Duration) {
		t.Helper()
		clock = clock.Add(d)
		if err := w.Run(ctx); err != nil {
			t.Fatal(err)
		}
	}
	step(0) // erstmals: ok, nichts melden, nichts im Verlauf
	if len(changes) != 0 {
		t.Fatalf("erstmals: %+v", changes)
	}
	step(18 * time.Hour) // 21 h alt → bald fällig
	step(time.Hour)      // unverändert
	step(5 * time.Hour)  // 27 h → überfällig
	last = clock         // neuer Lauf
	step(time.Minute)    // ok
	result = integrations.StatusError
	step(time.Minute) // fehlgeschlagen

	got := []string{}
	for _, c := range changes {
		got = append(got, string(c.From)+"→"+string(c.Item.State))
	}
	want := []string{"ok→due_soon", "due_soon→overdue", "overdue→ok", "ok→failed"}
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%v, erwartet %v", got, want)
		}
	}
	hist, _ := st.ListHistory(ctx, store.HistoryFilter{Kind: "backup"})
	if len(hist) != 4 || hist[0].To != "failed" || hist[0].ItemGroup != "nas" || hist[0].IntegrationName != "X" {
		t.Fatalf("Verlauf: %+v", hist)
	}
	if p, _ := st.ListHistory(ctx, store.HistoryFilter{Problems: true}); len(p) != 4 {
		t.Fatalf("nur Probleme: %d", len(p))
	}

	// Erstmals gesehen und schon überfällig: in den Verlauf, aber keine Meldung
	st.DeleteIntegration(ctx, it.ID)
	it2, _ := st.CreateIntegration(ctx, store.Integration{Type: "x", Name: "Y", Enabled: true, Interval: time.Minute,
		Timeout: time.Second, Config: "{}"}, nil)
	old := clock.Add(-100 * time.Hour)
	changes = nil
	w.Load = func(_ context.Context, at time.Time) ([]Item, error) {
		return Evaluate([]Source{{IntegrationID: it2.ID, Backups: []integrations.Backup{
			{ID: "c", Name: "Alt", LastRun: &old, Result: integrations.StatusOK}}}}, nil, 26, at), nil
	}
	step(0)
	hist, _ = st.ListHistory(ctx, store.HistoryFilter{IntegrationID: it2.ID})
	if len(changes) != 0 || len(hist) != 1 || hist[0].From != "unknown" || hist[0].To != "overdue" {
		t.Fatalf("erstmals überfällig: %+v %+v", changes, hist)
	}
}
