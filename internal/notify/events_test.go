package notify

import (
	"testing"
	"time"

	"github.com/mrcdlm/labdeck/internal/backups"
	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/store"
)

var at = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

func tf(t time.Time) string { return t.Format("2006-01-02 15:04") }

func TestFromPoll(t *testing.T) {
	src := Source{Name: "Homelab", Type: "healthchecks"}
	ping := at.Add(-time.Hour)
	ch := func(kind, id, name, group, from, to string) store.StatusChange {
		return store.StatusChange{Kind: kind, ItemID: id, ItemName: name, ItemGroup: group, From: from, To: to, At: at}
	}
	evs := FromPoll(src, []store.StatusChange{
		{Kind: "integration", ItemName: "Homelab", From: "ok", To: "error", At: at,
			Message: i18n.Encode(i18n.M("integration.timeout"))},
		ch("check", "a", "DynDNS", "", "ok", "error"),
		ch("check", "b", "Neu", "", "unknown", "error"), // erstmals gesehen: nicht melden
		ch("check", "c", "Certs", "", "error", "ok"),
		ch("check", "d", "Borg", "", "ok", "warning"), // Karenzzeit: nicht melden
		ch("metric", "frei/nas/frei", "Frei", "nas", "ok", "warning"),
		ch("metric", "cpu", "CPU", "", "warning", "ok"), // Erholung: nicht melden
		ch("metric", "temp", "Temp", "", "unknown", "error"),
	}, map[string]integrations.Metric{"frei/nas/frei": {Value: 18.4, Unit: "%"}, "temp": {Value: 71}},
		map[string]*time.Time{"a": &ping}, tf)

	types := []string{}
	for _, e := range evs {
		types = append(types, e.Type)
	}
	want := []string{EventIntegrationDown, EventCheckDown, EventCheckUp, EventThreshold, EventThreshold}
	if len(types) != len(want) {
		t.Fatalf("%v", types)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Fatalf("%v, erwartet %v", types, want)
		}
	}
	de := func(e Event) Event { return Localize(e, i18n.DE) }
	if e := de(evs[0]); e.Title != "Nicht erreichbar: Homelab" || e.Message != "Zeitüberschreitung – keine Antwort" ||
		e.Data["error"] != "Timed out – no response" {
		t.Errorf("integration: %+v", e)
	}
	if e := de(evs[1]); e.Message != "Homelab – letzter Ping 2026-10-07 11:00" || e.Data["last_ping"] != "2026-10-07T11:00:00Z" {
		t.Errorf("check: %+v", e)
	}
	if e := de(evs[3]); e.Title != "Schwellwert: Frei · nas – Warnung" || e.Message != "Homelab – Wert 18.4 %" ||
		e.Data["group"] != "nas" || e.Data["value"] != "18.4" {
		t.Errorf("threshold: %+v", e)
	}
	if e := Localize(evs[4], i18n.EN); e.Title != "Threshold: Temp – error" {
		t.Errorf("threshold en: %+v", e)
	}
}

func TestFromBackup(t *testing.T) {
	last := at.Add(-30 * time.Hour)
	item := backups.Item{IntegrationName: "HC", IntegrationType: "healthchecks", Name: "Restic", Group: "nas",
		LastRun: &last, MaxAgeHours: 26}
	cases := []struct {
		from, to backups.State
		want     string
	}{
		{backups.StateDueSoon, backups.StateOverdue, EventBackupOverdue},
		{backups.StateOK, backups.StateFailed, EventBackupFailed},
		{backups.StateOverdue, backups.StateOK, EventBackupRecovered},
		{backups.StateOK, backups.StateDueSoon, ""},
		{backups.StateDueSoon, backups.StateOK, ""},
	}
	for _, c := range cases {
		it := item
		it.State = c.to
		ev, ok := FromBackup(backups.Change{Item: it, From: c.from}, at, tf)
		if (c.want == "") == ok || ev.Type != c.want {
			t.Errorf("%s→%s: %v %s", c.from, c.to, ok, ev.Type)
		}
	}
	it := item
	it.State = backups.StateOverdue
	ev, _ := FromBackup(backups.Change{Item: it, From: backups.StateOK}, at, tf)
	ev = Localize(ev, i18n.DE)
	if ev.Title != "Backup überfällig: Restic · nas" || ev.Message != "HC – letzter Lauf 2026-10-06 06:00 (Höchstalter 26 h)" ||
		ev.Data["max_age_hours"] != "26" {
		t.Errorf("%+v", ev)
	}
	it.LastRun = nil
	ev, _ = FromBackup(backups.Change{Item: it}, at, tf)
	if Localize(ev, i18n.EN).Message != "HC – last run never (maximum age 26 h)" {
		t.Errorf("nie: %+v", Localize(ev, i18n.EN))
	}
}
