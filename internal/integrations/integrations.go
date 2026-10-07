// Package integrations definiert die einheitliche Schnittstelle aller
// Datenquellen (Healthchecks.io, Immich, PBS, HTTP-JSON, Prometheus …), ihr
// Konfigurationsschema und die normierten Ergebnisse (Snapshot).
//
// Für eine neue Datenquelle genügt eine Integration: Das Frontend erzeugt
// das Einstellungsformular aus Schema(), Polling, Speicherung, Verlauf und
// Benachrichtigungen übernimmt das Framework.
package integrations

import (
	"context"
	"time"
)

type Integration interface {
	// Type ist der stabile Bezeichner, z. B. "healthchecks".
	Type() string
	// Schema beschreibt die Felder des Einstellungsformulars.
	Schema() ConfigSchema
	// Test prüft Erreichbarkeit und Zugangsdaten („Verbindung testen“).
	Test(ctx context.Context, cfg Config) error
	// Collect fragt die aktuellen Daten ab.
	Collect(ctx context.Context, cfg Config) (Snapshot, error)
}

// Status ist der einheitliche Zustand von Integrationen, Kennzahlen, Checks
// und Backups: grün = ok, gelb = warning, rot = error, grau = unknown/paused.
type Status string

const (
	StatusOK      Status = "ok"
	StatusWarning Status = "warning"
	StatusError   Status = "error"
	StatusUnknown Status = "unknown"
	StatusPaused  Status = "paused"
)

// Snapshot sind die normierten Daten einer Abfrage.
type Snapshot struct {
	Metrics []Metric `json:"metrics,omitempty"`
	Checks  []Check  `json:"checks,omitempty"`
	Backups []Backup `json:"backups,omitempty"`
	// Info: zusätzliche Angaben zur Anzeige, z. B. "version".
	Info map[string]string `json:"info,omitempty"`
}

// Metric ist eine Kennzahl, z. B. Anzahl Fotos oder belegter Speicher.
type Metric struct {
	// Key ist innerhalb der Integration eindeutig und stabil (für den Verlauf).
	Key  string `json:"key"`
	Name string `json:"name"`
	// Group fasst Kennzahlen zusammen, z. B. je Benutzer (leer = Hauptkennzahl).
	Group string  `json:"group,omitempty"`
	Value float64 `json:"value"`
	// Unit: z. B. "bytes", "s", "%", "" (Anzahl)
	Unit   string `json:"unit,omitempty"`
	Status Status `json:"status"`
	// Derived: aus Checks/Backups abgeleitet (z. B. Anzahl down) – zählt im
	// Gesamtstatus nicht zusätzlich als Problem
	Derived bool `json:"derived,omitempty"`
}

// Check ist eine überwachte Aufgabe, z. B. ein Healthchecks.io-Check.
type Check struct {
	// ID ist innerhalb der Integration eindeutig und stabil.
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Group    string     `json:"group,omitempty"`
	Tags     []string   `json:"tags,omitempty"`
	Status   Status     `json:"status"`
	Desc     string     `json:"desc,omitempty"`
	LastPing *time.Time `json:"last_ping,omitempty"`
	NextPing *time.Time `json:"next_ping,omitempty"`
	// Raw ist der Status der Quelle, z. B. "grace" (zur Anzeige).
	Raw string `json:"raw,omitempty"`
	// History: Statuswechsel laut Quelle ab HistorySince (für Uptime-Balken);
	// HistorySince nil = Quelle liefert keinen Verlauf.
	History      []Transition `json:"history,omitempty"`
	HistorySince *time.Time   `json:"history_since,omitempty"`
}

// Backup ist ein Backup-Auftrag bzw. eine Backup-Gruppe.
type Backup struct {
	// ID ist innerhalb der Integration eindeutig und stabil.
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Group   string     `json:"group,omitempty"`
	LastRun *time.Time `json:"last_run,omitempty"`
	// Result des letzten Laufs: ok, error (fehlgeschlagen), warning, unknown, paused
	Result      Status         `json:"result"`
	SizeBytes   *int64         `json:"size_bytes,omitempty"`
	Duration    *time.Duration `json:"duration,omitempty"`
	Description string         `json:"description,omitempty"`
	// History/HistorySince wie bei Check (z. B. Healthchecks-Flips).
	History      []Transition `json:"history,omitempty"`
	HistorySince *time.Time   `json:"history_since,omitempty"`
}

// Transition ist ein Statuswechsel zum Zeitpunkt At.
type Transition struct {
	At   time.Time `json:"at"`
	From Status    `json:"from"`
	To   Status    `json:"to"`
}

// Worst liefert den schlechtesten Status (error > warning > unknown > ok;
// paused zählt nicht).
func Worst(statuses ...Status) Status {
	rank := map[Status]int{StatusOK: 1, StatusUnknown: 2, StatusWarning: 3, StatusError: 4}
	out := StatusOK
	seen := false
	for _, s := range statuses {
		if s == StatusPaused {
			continue
		}
		if !seen || rank[s] > rank[out] {
			out, seen = s, true
		}
	}
	if !seen {
		return StatusUnknown
	}
	return out
}
