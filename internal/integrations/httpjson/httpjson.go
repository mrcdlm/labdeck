// Package httpjson liest Werte aus beliebigen JSON-Antworten (GJSON-Pfade).
// Damit lassen sich Dienste wie Uptime Kuma, Paperless, Nextcloud oder Home
// Assistant ohne eigenen Code anbinden – und jedes Backup-Werkzeug, das einen
// Zeitstempel des letzten Laufs als JSON liefert.
package httpjson

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
)

const Type = "httpjson"

const maxValues = 20

type Integration struct {
	now func() time.Time
}

func New() *Integration { return &Integration{now: time.Now} }

func (*Integration) Type() string { return Type }

func (*Integration) Schema() integrations.ConfigSchema {
	type T = integrations.Text
	path := integrations.Field{
		Key: "path", Type: integrations.FieldString, Required: true,
		Label: T{DE: "JSON-Pfad", EN: "JSON path"},
		Help: T{
			DE: "GJSON-Syntax, z. B. documents_total, data.items.# (Anzahl) oder results.0.last_run.",
			EN: "GJSON syntax, e.g. documents_total, data.items.# (count) or results.0.last_run.",
		},
		Placeholder: "data.count",
	}
	return integrations.ConfigSchema{
		Title: T{DE: "HTTP-JSON", EN: "HTTP JSON"},
		Description: T{
			DE: "Beliebige Werte aus einer JSON-Schnittstelle – mit Schwellwerten oder als Backup-Zeitstempel.",
			EN: "Any values from a JSON API – with thresholds or as backup timestamp.",
		},
		Icon: "braces",
		Docs: "https://github.com/tidwall/gjson/blob/master/SYNTAX.md",
		Fields: []integrations.Field{
			{Key: "url", Type: integrations.FieldURL, Required: true,
				Label:       T{DE: "Adresse", EN: "Address"},
				Help:        T{DE: "Adresse, die JSON liefert.", EN: "Address that returns JSON."},
				Placeholder: "http://paperless.lan:8000/api/statistics/"},
			{Key: "method", Type: integrations.FieldSelect, Default: "GET", Half: true,
				Label:   T{DE: "Methode", EN: "Method"},
				Options: []integrations.Option{{Value: "GET"}, {Value: "POST"}}},
			{Key: "body", Type: integrations.FieldText, ShowIf: &integrations.Condition{Key: "method", Values: []string{"POST"}},
				Label: T{DE: "Body (nur POST)", EN: "Body (POST only)"},
				Help:  T{DE: "Wird als JSON gesendet.", EN: "Sent as JSON."}},
			{Key: "headers", Type: integrations.FieldText,
				Label: T{DE: "Header", EN: "Headers"},
				Help: T{
					DE: "Ein Header je Zeile, z. B. Accept: application/json. Zugangsdaten gehören ins Feld darunter.",
					EN: "One header per line, e.g. Accept: application/json. Credentials belong in the field below.",
				},
				Placeholder: "Accept: application/json"},
			{Key: "auth_header", Type: integrations.FieldSecret,
				Label: T{DE: "Header mit Zugangsdaten", EN: "Header with credentials"},
				Help: T{
					DE: "Wird verschlüsselt gespeichert, z. B. Authorization: Token abc123 oder X-Api-Key: …",
					EN: "Stored encrypted, e.g. Authorization: Token abc123 or X-Api-Key: …",
				}},
			{Key: "values", Type: integrations.FieldList, Required: true, Max: integrations.Ptr(maxValues),
				Label: T{DE: "Werte", EN: "Values"},
				Item:  integrations.ValueFields(path)},
		},
	}
}

// parseHeaders liest „Name: Wert“-Zeilen.
func parseHeaders(text string, into http.Header) error {
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		name = strings.TrimSpace(name)
		if !ok || name == "" || strings.ContainsAny(name, " \t") {
			return i18n.E("httpjson.header_invalid", "line", strconv.Itoa(i+1))
		}
		into.Add(name, strings.TrimSpace(value))
	}
	return nil
}

func (i *Integration) fetch(ctx context.Context, cfg integrations.Config) (json.RawMessage, error) {
	h := http.Header{}
	if err := parseHeaders(cfg.String("headers"), h); err != nil {
		return nil, err
	}
	if s := cfg.Secret("auth_header"); s != "" {
		if err := parseHeaders(s, h); err != nil {
			return nil, i18n.E("httpjson.auth_header_invalid")
		}
	}
	method := cfg.String("method")
	var body []byte
	if method == http.MethodPost && strings.TrimSpace(cfg.String("body")) != "" {
		body = []byte(cfg.String("body"))
	} else if method != http.MethodPost {
		method = http.MethodGet
	}
	var raw json.RawMessage
	if err := integrations.FetchJSON(ctx, cfg.HTTP, method, cfg.String("url"), h, body, &raw, nil); err != nil {
		return nil, err
	}
	return raw, nil
}

func (i *Integration) Test(ctx context.Context, cfg integrations.Config) error {
	_, err := i.Collect(ctx, cfg)
	return err
}

func (i *Integration) Collect(ctx context.Context, cfg integrations.Config) (integrations.Snapshot, error) {
	raw, err := i.fetch(ctx, cfg)
	if err != nil {
		return integrations.Snapshot{}, err
	}
	now := i.now()
	items := cfg.List("values")
	names := make([]string, len(items))
	for n, it := range items {
		names[n] = it.String("name")
	}
	keys := integrations.UniqueKeys(names)

	var snap integrations.Snapshot
	for n, it := range items {
		spec := integrations.ParseValueSpec(it)
		path := it.String("path")
		res := gjson.GetBytes(raw, path)
		if !res.Exists() {
			return integrations.Snapshot{}, i18n.E("httpjson.path_missing", "name", spec.Name, "path", path)
		}
		v, ts, err := value(res, spec, now)
		if err != nil {
			return integrations.Snapshot{}, err
		}
		snap.Metrics = append(snap.Metrics, integrations.Metric{Key: keys[n], Name: spec.Name, Value: v,
			Unit: spec.MetricUnit(), Status: spec.Status(v)})
		if spec.Backup && spec.Kind == integrations.KindTimestamp {
			snap.Backups = append(snap.Backups, integrations.Backup{ID: keys[n], Name: spec.Name, LastRun: &ts,
				Result: integrations.StatusOK})
		}
	}
	return snap, nil
}

// value liest den Wert; bei Zeitstempeln das Alter in Sekunden und den Zeitpunkt.
func value(res gjson.Result, spec integrations.ValueSpec, now time.Time) (float64, time.Time, error) {
	if spec.Kind == integrations.KindTimestamp {
		var ts time.Time
		ok := false
		switch res.Type {
		case gjson.Number:
			ts, ok = integrations.UnixAny(res.Float())
		case gjson.String:
			ts, ok = integrations.ParseTimestamp(res.String())
		}
		if !ok {
			return 0, ts, i18n.E("httpjson.not_timestamp", "name", spec.Name, "value", short(res.Raw))
		}
		return max(now.Sub(ts).Seconds(), 0), ts, nil
	}
	switch res.Type {
	case gjson.Number:
		return res.Float(), time.Time{}, nil
	case gjson.True:
		return 1, time.Time{}, nil
	case gjson.False:
		return 0, time.Time{}, nil
	case gjson.String:
		if f, err := strconv.ParseFloat(strings.TrimSpace(res.String()), 64); err == nil {
			return f, time.Time{}, nil
		}
	}
	return 0, time.Time{}, i18n.E("httpjson.not_number", "name", spec.Name, "value", short(res.Raw))
}

// short kürzt fremde Werte für Fehlermeldungen.
func short(s string) string {
	if r := []rune(s); len(r) > 60 {
		return string(r[:60]) + "…"
	}
	return s
}
