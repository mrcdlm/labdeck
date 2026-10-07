package notify

import (
	"strconv"
	"time"

	"github.com/mrcdlm/labdeck/internal/backups"
	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/store"
)

// Source beschreibt die Integration eines Ereignisses.
type Source struct {
	Name string
	Type string
}

// TimeFormat formatiert Zeitpunkte für Nachrichten (Zeitzone der Anzeige).
type TimeFormat func(t time.Time) string

func data(src Source, kv ...string) map[string]string {
	m := map[string]string{"integration": src.Name, "integration_type": src.Type}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] != "" {
			m[kv[i]] = kv[i+1]
		}
	}
	return m
}

func itemName(c store.StatusChange) string {
	if c.ItemGroup != "" {
		return c.ItemName + " · " + c.ItemGroup
	}
	return c.ItemName
}

// FromPoll macht aus den Statuswechseln einer Abfrage Ereignisse:
// Integration nicht erreichbar bzw. wieder erreichbar, Check down bzw.
// wieder up, Schwellwert überschritten. Checks, die zum ersten Mal auftauchen
// (vorher unbekannt), werden nicht gemeldet – sonst käme beim Einrichten für
// jeden Check eine Nachricht. metrics liefert Wert und Einheit je Schlüssel.
func FromPoll(src Source, changes []store.StatusChange, metrics map[string]integrations.Metric, lastPing map[string]*time.Time, tf TimeFormat) []Event {
	var out []Event
	for _, c := range changes {
		from, to := integrations.Status(c.From), integrations.Status(c.To)
		switch c.Kind {
		case "integration":
			switch {
			case to == integrations.StatusError:
				msg := i18n.Decode(c.Message)
				out = append(out, Event{Type: EventIntegrationDown, Priority: PriorityHigh, Time: c.At,
					TitleMsg: i18n.M("notify.integration_down.title", "name", src.Name), MessageMsg: msg,
					Data: data(src, "error", i18n.T(i18n.EN, msg))})
			case from == integrations.StatusError && to == integrations.StatusOK:
				out = append(out, Event{Type: EventIntegrationRecovered, Priority: PriorityDefault, Time: c.At,
					TitleMsg:   i18n.M("notify.integration_recovered.title", "name", src.Name),
					MessageMsg: i18n.M("notify.integration_recovered.message"), Data: data(src)})
			}
		case "check":
			switch {
			case to == integrations.StatusError && from != integrations.StatusUnknown:
				last := i18n.Ref("notify.backup_never")
				lastISO := ""
				if t := lastPing[c.ItemID]; t != nil {
					last, lastISO = tf(*t), t.UTC().Format(time.RFC3339)
				}
				out = append(out, Event{Type: EventCheckDown, Priority: PriorityHigh, Time: c.At,
					TitleMsg:   i18n.M("notify.check_down.title", "name", c.ItemName),
					MessageMsg: i18n.M("notify.check_down.message", "integration", src.Name, "last", last),
					Data:       data(src, "check", c.ItemName, "status", "down", "last_ping", lastISO)})
			case from == integrations.StatusError && to == integrations.StatusOK:
				out = append(out, Event{Type: EventCheckUp, Priority: PriorityDefault, Time: c.At,
					TitleMsg:   i18n.M("notify.check_up.title", "name", c.ItemName),
					MessageMsg: i18n.M("notify.check_up.message", "integration", src.Name),
					Data:       data(src, "check", c.ItemName, "status", "up")})
			}
		case "metric":
			worse := (to == integrations.StatusWarning || to == integrations.StatusError) &&
				(from == integrations.StatusOK || from == integrations.StatusUnknown ||
					from == integrations.StatusWarning && to == integrations.StatusError)
			if !worse {
				continue
			}
			m := metrics[c.ItemID]
			value := strconv.FormatFloat(m.Value, 'f', -1, 64)
			shown := value
			if m.Unit != "" {
				shown += " " + m.Unit
			}
			out = append(out, Event{Type: EventThreshold, Priority: PriorityDefault, Time: c.At,
				TitleMsg:   i18n.M("notify.threshold.title", "name", itemName(c), "status", i18n.Ref("status."+string(to))),
				MessageMsg: i18n.M("notify.threshold.message", "integration", src.Name, "value", shown),
				Data: data(src, "metric", c.ItemName, "key", c.ItemID, "group", c.ItemGroup, "value", value,
					"unit", m.Unit, "status", string(to))})
		}
	}
	return out
}

// FromBackup macht aus einem Zustandswechsel eines Backups ein Ereignis
// (überfällig, fehlgeschlagen, wieder ok); andere Wechsel liefern false.
func FromBackup(c backups.Change, at time.Time, tf TimeFormat) (Event, bool) {
	it := c.Item
	src := Source{Name: it.IntegrationName, Type: it.IntegrationType}
	name := it.Name
	if it.Group != "" {
		name += " · " + it.Group
	}
	last := i18n.Ref("notify.backup_never")
	lastISO := ""
	if it.LastRun != nil {
		last, lastISO = tf(*it.LastRun), it.LastRun.UTC().Format(time.RFC3339)
	}
	d := data(src, "backup", it.Name, "group", it.Group, "state", string(it.State), "last_run", lastISO,
		"max_age_hours", strconv.Itoa(it.MaxAgeHours))
	switch {
	case it.State == backups.StateOverdue:
		return Event{Type: EventBackupOverdue, Priority: PriorityHigh, Time: at, Data: d,
			TitleMsg: i18n.M("notify.backup_overdue.title", "name", name),
			MessageMsg: i18n.M("notify.backup_overdue.message", "integration", src.Name, "last", last,
				"max", strconv.Itoa(it.MaxAgeHours))}, true
	case it.State == backups.StateFailed:
		return Event{Type: EventBackupFailed, Priority: PriorityHigh, Time: at, Data: d,
			TitleMsg:   i18n.M("notify.backup_failed.title", "name", name),
			MessageMsg: i18n.M("notify.backup_failed.message", "integration", src.Name)}, true
	case it.State == backups.StateOK && (c.From == backups.StateOverdue || c.From == backups.StateFailed):
		return Event{Type: EventBackupRecovered, Priority: PriorityDefault, Time: at, Data: d,
			TitleMsg:   i18n.M("notify.backup_recovered.title", "name", name),
			MessageMsg: i18n.M("notify.backup_recovered.message", "integration", src.Name, "last", last)}, true
	}
	return Event{}, false
}
