package integrations

import (
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Werte für generische Quellen (HTTP-JSON, Prometheus): je Wert Name, Quelle
// (JSON-Pfad bzw. PromQL), Art, Einheit, Schwellwerte und optional „als
// Backup werten“. Die Felder sind für alle generischen Quellen gleich.

const (
	KindNumber    = "number"
	KindTimestamp = "timestamp" // Wert ist ein Zeitpunkt → Kennzahl ist sein Alter in Sekunden

	DirectionAbove = "above" // höher ist schlechter
	DirectionBelow = "below" // niedriger ist schlechter
)

// ValueFields liefert die Unterfelder eines Werts; source ist das Feld, aus
// dem der Wert kommt (JSON-Pfad, PromQL …), extra folgen danach.
func ValueFields(source Field, extra ...Field) []Field {
	type T = Text
	fields := []Field{
		{Key: "name", Type: FieldString, Required: true,
			Label: T{DE: "Anzeigename", EN: "Display name"}, Placeholder: "Dokumente"},
		source,
	}
	fields = append(fields, extra...)
	return append(fields,
		Field{Key: "kind", Type: FieldSelect, Default: KindNumber, Half: true,
			Label: T{DE: "Art", EN: "Kind"},
			Options: []Option{
				{Value: KindNumber, Label: T{DE: "Zahl", EN: "Number"}},
				{Value: KindTimestamp, Label: T{DE: "Zeitstempel → Alter", EN: "Timestamp → age"}},
			}},
		Field{Key: "unit", Type: FieldString, Half: true, ShowIf: &Condition{Key: "kind", Values: []string{KindNumber}},
			Label:       T{DE: "Einheit", EN: "Unit"},
			Help:        T{DE: "bytes, s und % werden umgerechnet, sonst beliebiger Text.", EN: "bytes, s and % are converted, otherwise any text."},
			Placeholder: "bytes"},
		Field{Key: "warn", Type: FieldNumber, Half: true,
			Label: T{DE: "Warnung ab", EN: "Warning at"},
			Help:  T{DE: "Bei Zeitstempeln: Alter in Stunden.", EN: "For timestamps: age in hours."}},
		Field{Key: "error", Type: FieldNumber, Half: true,
			Label: T{DE: "Fehler ab", EN: "Error at"}},
		Field{Key: "direction", Type: FieldSelect, Default: DirectionAbove,
			Label: T{DE: "Schwellwerte gelten", EN: "Thresholds apply"},
			Options: []Option{
				{Value: DirectionAbove, Label: T{DE: "ab diesem Wert aufwärts (höher ist schlechter)", EN: "from this value upwards (higher is worse)"}},
				{Value: DirectionBelow, Label: T{DE: "ab diesem Wert abwärts (niedriger ist schlechter)", EN: "from this value downwards (lower is worse)"}},
			}},
		Field{Key: "backup", Type: FieldBool, ShowIf: &Condition{Key: "kind", Values: []string{KindTimestamp}},
			Label: T{DE: "Als Backup werten", EN: "Treat as backup"},
			Help: T{
				DE: "Nur für Zeitstempel: erscheint auf der Backups-Seite und wird nach dem Höchstalter überfällig.",
				EN: "Timestamps only: appears on the Backups page and becomes overdue after the maximum age.",
			}},
	)
}

// ValueSpec ist ein konfigurierter Wert.
type ValueSpec struct {
	Name       string
	Kind       string
	Unit       string
	Direction  string
	Warn, Fail *float64
	Backup     bool
}

// ParseValueSpec liest einen Eintrag der Werte-Liste.
func ParseValueSpec(c Config) ValueSpec {
	v := ValueSpec{Name: c.String("name"), Kind: c.String("kind"), Unit: c.String("unit"),
		Direction: c.String("direction"), Backup: c.Bool("backup")}
	if v.Kind == "" {
		v.Kind = KindNumber
	}
	if c.Has("warn") {
		w := c.Float("warn")
		v.Warn = &w
	}
	if c.Has("error") {
		e := c.Float("error")
		v.Fail = &e
	}
	return v
}

// MetricUnit: Zeitstempel werden als Alter in Sekunden angezeigt.
func (v ValueSpec) MetricUnit() string {
	if v.Kind == KindTimestamp {
		return "s"
	}
	return v.Unit
}

// Status bewertet value mit den Schwellwerten (Zeitstempel: Alter in
// Sekunden, Schwellwerte in Stunden).
func (v ValueSpec) Status(value float64) Status {
	if v.Kind == KindTimestamp {
		value /= 3600
	}
	worse := func(limit float64) bool {
		if v.Direction == DirectionBelow {
			return value <= limit
		}
		return value >= limit
	}
	switch {
	case v.Fail != nil && worse(*v.Fail):
		return StatusError
	case v.Warn != nil && worse(*v.Warn):
		return StatusWarning
	}
	return StatusOK
}

// Slug macht aus einem Namen einen stabilen Schlüssel ("Freier Speicher" → "freier_speicher").
func Slug(s string) string {
	var b strings.Builder
	under := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			under = false
		} else if !under && b.Len() > 0 {
			b.WriteByte('_')
			under = true
		}
	}
	out := strings.TrimRight(b.String(), "_")
	if out == "" {
		return "value"
	}
	return out
}

// UniqueKeys vergibt je Name einen eindeutigen Schlüssel (doppelte → _2, _3 …).
func UniqueKeys(names []string) []string {
	seen := map[string]int{}
	out := make([]string, len(names))
	for i, n := range names {
		k := Slug(n)
		seen[k]++
		if seen[k] > 1 {
			k += "_" + strconv.Itoa(seen[k])
		}
		out[i] = k
	}
	return out
}

var timeLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05Z07:00",
	"2006-01-02 15:04:05",
	time.RFC1123Z,
	time.RFC1123,
	time.DateOnly,
}

// ParseTimestamp liest Unix-Zeit (Sekunden, Millisekunden oder
// Mikrosekunden, auch als Text) oder gängige Datumsformate.
func ParseTimestamp(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return UnixAny(f)
	}
	for _, l := range timeLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// UnixAny deutet eine Zahl als Unix-Zeit in s, ms oder µs.
func UnixAny(f float64) (time.Time, bool) {
	if f <= 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		return time.Time{}, false
	}
	switch {
	case f > 1e14: // µs
		return time.UnixMicro(int64(f)), true
	case f > 1e11: // ms
		return time.UnixMilli(int64(f)), true
	}
	sec, frac := math.Modf(f)
	return time.Unix(int64(sec), int64(frac*1e9)), true
}
