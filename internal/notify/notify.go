// Package notify verschickt Benachrichtigungen über frei konfigurierbare
// Webhooks (Methode, URL, Header, Body-Template).
//
// Übernommen aus dnsdeck. Geheimnisse stehen nie im Klartext in der
// Konfiguration, sondern als Platzhalter: ${TOKEN} bzw. {{token}} für das
// verschlüsselt gespeicherte Token des Webhooks, ${WEBHOOK_NAME} bzw.
// {{env "WEBHOOK_NAME"}} für Env-Variablen. Erlaubt sind nur Variablen mit
// den Präfixen WEBHOOK_ und SECRET_, damit über die Oberfläche z. B.
// APP_PASSWORD nicht an einen fremden Server geschickt werden kann.
package notify

import (
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
)

// Ereignistypen, die einzeln an- und abgeschaltet werden können.
const (
	EventCheckDown            = "check_down"
	EventCheckUp              = "check_up"
	EventBackupOverdue        = "backup_overdue"
	EventBackupFailed         = "backup_failed"
	EventBackupRecovered      = "backup_recovered"
	EventIntegrationDown      = "integration_down"
	EventIntegrationRecovered = "integration_recovered"
	EventThreshold            = "threshold"
	EventTest                 = "test" // Testnachricht, immer zugestellt
)

// EventTypes in Anzeigereihenfolge.
var EventTypes = []string{EventCheckDown, EventCheckUp, EventBackupOverdue, EventBackupFailed, EventBackupRecovered,
	EventIntegrationDown, EventIntegrationRecovered, EventThreshold}

// Priorität (angelehnt an ntfy: 1 min … 5 max).
const (
	PriorityLow     = 2
	PriorityDefault = 3
	PriorityHigh    = 4
)

// Event ist ein Ereignis. TitleMsg/MessageMsg sind übersetzbar; Title und
// Message werden vor der Zustellung in der Benachrichtigungssprache daraus
// gerendert (siehe Localize).
type Event struct {
	Type       string            `json:"type"`
	Title      string            `json:"title"`
	Message    string            `json:"message"`
	TitleMsg   i18n.Msg          `json:"-"`
	MessageMsg i18n.Msg          `json:"-"`
	Priority   int               `json:"priority"`
	Time       time.Time         `json:"time"`
	Data       map[string]string `json:"data,omitempty"`
}

// Notifier nimmt Ereignisse entgegen (nil-sicher über Send).
type Notifier interface {
	Notify(ev Event)
}

// Send leitet ev an n weiter, wenn n gesetzt ist.
func Send(n Notifier, ev Event) {
	if n != nil {
		n.Notify(ev)
	}
}

// Localize rendert Title und Message in lang (sofern übersetzbar angegeben).
func Localize(ev Event, lang i18n.Lang) Event {
	if !ev.TitleMsg.IsZero() {
		ev.Title = i18n.T(lang, ev.TitleMsg)
	}
	if !ev.MessageMsg.IsZero() {
		ev.Message = i18n.T(lang, ev.MessageMsg)
	}
	return ev
}
