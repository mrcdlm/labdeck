package backups

import (
	"testing"
	"time"

	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/store"
)

var now = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

func ago(d time.Duration) *time.Time {
	t := now.Add(-d)
	return &t
}

func TestStates(t *testing.T) {
	const h = time.Hour
	maxAge := 26 * h
	for _, tc := range []struct {
		name string
		b    integrations.Backup
		want State
	}{
		{"frisch", integrations.Backup{LastRun: ago(3 * h), Result: integrations.StatusOK}, StateOK},
		{"80 %", integrations.Backup{LastRun: ago(21 * h), Result: integrations.StatusOK}, StateDueSoon},
		{"knapp unter 80 %", integrations.Backup{LastRun: ago(20 * h), Result: integrations.StatusOK}, StateOK},
		{"überfällig", integrations.Backup{LastRun: ago(27 * h), Result: integrations.StatusOK}, StateOverdue},
		{"Karenzzeit der Quelle", integrations.Backup{LastRun: ago(2 * h), Result: integrations.StatusWarning}, StateDueSoon},
		{"fehlgeschlagen", integrations.Backup{LastRun: ago(2 * h), Result: integrations.StatusError}, StateFailed},
		{"down und zu alt", integrations.Backup{LastRun: ago(52 * h), Result: integrations.StatusError}, StateOverdue},
		{"down ohne Lauf", integrations.Backup{Result: integrations.StatusError}, StateFailed},
		{"nie gelaufen", integrations.Backup{Result: integrations.StatusUnknown}, StateUnknown},
		{"pausiert", integrations.Backup{LastRun: ago(500 * h), Result: integrations.StatusPaused}, StatePaused},
		{"Ergebnis unbekannt, aber frisch", integrations.Backup{LastRun: ago(h), Result: integrations.StatusUnknown}, StateOK},
	} {
		if got := state(tc.b, maxAge, now); got != tc.want {
			t.Errorf("%s: %s, erwartet %s", tc.name, got, tc.want)
		}
	}
}

func TestEvaluate(t *testing.T) {
	since := now.Add(-30 * 24 * time.Hour)
	size := int64(1 << 30)
	dur := 90 * time.Second
	sources := []Source{
		{IntegrationID: 1, IntegrationName: "HC", IntegrationType: "healthchecks", Backups: []integrations.Backup{
			{ID: "a", Name: "Restic", LastRun: ago(3 * time.Hour), Result: integrations.StatusOK, HistorySince: &since},
			{ID: "b", Name: "borg", LastRun: ago(30 * time.Hour), Result: integrations.StatusOK},
			{ID: "c", Name: "Weekly", LastRun: ago(100 * time.Hour), Result: integrations.StatusOK,
				SizeBytes: &size, Duration: &dur},
		}},
		{IntegrationID: 2, IntegrationName: "PBS", IntegrationType: "pbs", Stale: true, Backups: []integrations.Backup{
			{ID: "vm/100", Name: "vm/100", Result: integrations.StatusError, LastRun: ago(time.Hour)},
		}},
	}
	items := Evaluate(sources, map[store.BackupKey]int{{IntegrationID: 1, ItemID: "c"}: 170}, 26, now)
	if len(items) != 4 {
		t.Fatalf("%d", len(items))
	}
	names := []string{}
	for _, it := range items {
		names = append(names, it.Name)
	}
	// fehlgeschlagen, überfällig, dann ok (alphabetisch)
	if got := names; got[0] != "vm/100" || got[1] != "borg" || got[2] != "Restic" || got[3] != "Weekly" {
		t.Fatalf("Reihenfolge: %v", names)
	}
	vm, borg, restic, weekly := items[0], items[1], items[2], items[3]
	if !vm.Stale || vm.State != StateFailed || vm.Status != integrations.StatusError || vm.IntegrationType != "pbs" {
		t.Errorf("vm: %+v", vm)
	}
	if borg.State != StateOverdue || borg.MaxAgeHours != 26 || borg.MaxAgeCustom || borg.Uptime != nil {
		t.Errorf("borg: %+v", borg)
	}
	if restic.DueAt == nil || !restic.DueAt.Equal(now.Add(23*time.Hour)) || len(restic.Uptime) != 3 {
		t.Errorf("restic: %+v", restic)
	}
	if weekly.State != StateOK || weekly.MaxAgeHours != 170 || !weekly.MaxAgeCustom ||
		*weekly.SizeBytes != size || *weekly.DurationSeconds != 90 {
		t.Errorf("weekly: %+v", weekly)
	}

	s := Summarize(items)
	if s.Total != 4 || s.States[StateOK] != 2 || s.States[StateFailed] != 1 || s.Status != integrations.StatusError {
		t.Errorf("summary: %+v", s)
	}
	if Summarize(nil).Status != integrations.StatusUnknown {
		t.Error("leer: unknown erwartet")
	}
}
