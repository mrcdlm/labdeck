// Package prometheus wertet beliebige PromQL-Abfragen aus (GET
// /api/v1/query). Damit ist alles nutzbar, was Exporter liefern:
// node_exporter, restic-/borg-Exporter, SMART …
package prometheus

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
)

const Type = "prometheus"

const maxQueries = 30

type Integration struct {
	now func() time.Time
}

func New() *Integration { return &Integration{now: time.Now} }

func (*Integration) Type() string { return Type }

func (*Integration) Schema() integrations.ConfigSchema {
	type T = integrations.Text
	query := integrations.Field{
		Key: "query", Type: integrations.FieldString, Required: true,
		Label: T{DE: "PromQL-Abfrage", EN: "PromQL query"},
		Help: T{
			DE: "Wird als Instant Query ausgewertet. Ein leeres Ergebnis zählt als 0 (z. B. up == 0).",
			EN: "Evaluated as an instant query. An empty result counts as 0 (e.g. up == 0).",
		},
		Placeholder: `node_filesystem_avail_bytes{mountpoint="/"}`,
	}
	label := integrations.Field{
		Key: "label", Type: integrations.FieldString,
		Label: T{DE: "Label je Ergebnis", EN: "Label per result"},
		Help: T{
			DE: "Liefert die Abfrage mehrere Zeitreihen, wird je Wert dieses Labels eine Zeile gezeigt, z. B. instance.",
			EN: "If the query returns several series, one row is shown per value of this label, e.g. instance.",
		},
		Placeholder: "instance",
	}
	basic := &integrations.Condition{Key: "auth", Values: []string{"basic"}}
	return integrations.ConfigSchema{
		Title: T{DE: "Prometheus", EN: "Prometheus"},
		Description: T{
			DE: "Beliebige PromQL-Abfragen mit Einheit und Schwellwerten – alles, was Exporter liefern.",
			EN: "Any PromQL queries with unit and thresholds – everything exporters provide.",
		},
		Icon: "flame",
		Docs: "https://prometheus.io/docs/prometheus/latest/querying/api/#instant-queries",
		Fields: []integrations.Field{
			{Key: "base_url", Type: integrations.FieldURL, Required: true,
				Label:       T{DE: "Adresse", EN: "Address"},
				Help:        T{DE: "Adresse von Prometheus (oder VictoriaMetrics, Thanos …).", EN: "Address of Prometheus (or VictoriaMetrics, Thanos …)."},
				Placeholder: "http://prometheus.lan:9090"},
			{Key: "auth", Type: integrations.FieldSelect, Default: "none",
				Label: T{DE: "Anmeldung", EN: "Authentication"},
				Options: []integrations.Option{
					{Value: "none", Label: T{DE: "Keine", EN: "None"}},
					{Value: "basic", Label: T{DE: "Benutzer und Passwort (Basic Auth)", EN: "User and password (basic auth)"}},
					{Value: "bearer", Label: T{DE: "Token (Bearer)", EN: "Token (bearer)"}},
				}},
			{Key: "username", Type: integrations.FieldString, ShowIf: basic,
				Label: T{DE: "Benutzer (Basic Auth)", EN: "User (basic auth)"}},
			{Key: "password", Type: integrations.FieldSecret, ShowIf: basic,
				Label: T{DE: "Passwort (Basic Auth)", EN: "Password (basic auth)"}},
			{Key: "token", Type: integrations.FieldSecret, ShowIf: &integrations.Condition{Key: "auth", Values: []string{"bearer"}},
				Label: T{DE: "Token (Bearer)", EN: "Token (bearer)"}},
			{Key: "queries", Type: integrations.FieldList, Required: true, Max: integrations.Ptr(maxQueries),
				Label: T{DE: "Abfragen", EN: "Queries"},
				Item:  integrations.ValueFields(query, label)},
		},
	}
}

func header(cfg integrations.Config) http.Header {
	h := http.Header{}
	switch cfg.String("auth") {
	case "basic":
		cred := cfg.String("username") + ":" + cfg.Secret("password")
		h.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(cred)))
	case "bearer":
		h.Set("Authorization", "Bearer "+cfg.Secret("token"))
	}
	return h
}

type response struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string          `json:"resultType"`
		Result     json.RawMessage `json:"result"`
	} `json:"data"`
	Error string `json:"error"`
}

// series ist eine Zeitreihe des Ergebnisses (Skalar: ohne Labels).
type series struct {
	labels map[string]string
	value  float64
}

func (*Integration) query(ctx context.Context, cfg integrations.Config, name, q string) ([]series, error) {
	u := cfg.BaseURL("base_url") + "/api/v1/query?query=" + url.QueryEscape(q)
	var resp response
	err := integrations.FetchJSON(ctx, cfg.HTTP, http.MethodGet, u, header(cfg), nil, &resp,
		func(status int, body []byte) error {
			var e response
			if json.Unmarshal(body, &e) == nil && e.Error != "" {
				return i18n.E("prometheus.query_error", "name", name, "detail", shorten(e.Error))
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	if resp.Status != "success" {
		if resp.Error != "" {
			return nil, i18n.E("prometheus.query_error", "name", name, "detail", shorten(resp.Error))
		}
		return nil, i18n.E("prometheus.unexpected")
	}
	switch resp.Data.ResultType {
	case "vector":
		var vec []struct {
			Metric map[string]string `json:"metric"`
			Value  [2]any            `json:"value"`
		}
		if err := json.Unmarshal(resp.Data.Result, &vec); err != nil {
			return nil, i18n.E("prometheus.unexpected")
		}
		out := make([]series, 0, len(vec))
		for _, s := range vec {
			out = append(out, series{labels: s.Metric, value: sampleValue(s.Value)})
		}
		return out, nil
	case "scalar":
		var sc [2]any
		if err := json.Unmarshal(resp.Data.Result, &sc); err != nil {
			return nil, i18n.E("prometheus.unexpected")
		}
		return []series{{value: sampleValue(sc)}}, nil
	case "matrix":
		return nil, i18n.E("prometheus.matrix", "name", name)
	}
	return nil, i18n.E("prometheus.unexpected")
}

// sampleValue: [Zeitpunkt, "Wert"]; "NaN"/"+Inf" bleiben NaN bzw. ±Inf.
func sampleValue(v [2]any) float64 {
	s, _ := v[1].(string)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return math.NaN()
	}
	return f
}

func shorten(s string) string {
	if r := []rune(s); len(r) > 200 {
		return string(r[:200]) + "…"
	}
	return s
}

// labelText beschreibt eine Zeitreihe: Wert des gewählten Labels, sonst alle
// Labels außer __name__.
func labelText(labels map[string]string, label string) string {
	if label != "" {
		if v, ok := labels[label]; ok {
			return v
		}
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		if k != "__name__" {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + labels[k]
	}
	return strings.Join(parts, ", ")
}

func (i *Integration) Test(ctx context.Context, cfg integrations.Config) error {
	_, err := i.Collect(ctx, cfg)
	return err
}

func (i *Integration) Collect(ctx context.Context, cfg integrations.Config) (integrations.Snapshot, error) {
	now := i.now()
	items := cfg.List("queries")
	names := make([]string, len(items))
	for n, it := range items {
		names[n] = it.String("name")
	}
	keys := integrations.UniqueKeys(names)

	var snap integrations.Snapshot
	for n, it := range items {
		spec := integrations.ParseValueSpec(it)
		res, err := i.query(ctx, cfg, spec.Name, it.String("query"))
		if err != nil {
			return integrations.Snapshot{}, err
		}
		label := strings.TrimSpace(it.String("label"))
		grouped := label != "" || len(res) > 1
		if len(res) == 0 && spec.Kind == integrations.KindNumber {
			res = []series{{value: 0}} // leeres Ergebnis = 0 (z. B. „up == 0“)
		}
		for _, s := range res {
			m := integrations.Metric{Key: keys[n], Name: spec.Name, Unit: spec.MetricUnit()}
			if grouped {
				m.Group = labelText(s.labels, label)
				m.Key = fmt.Sprintf("%s/%s/%s", keys[n], integrations.Slug(m.Group), keys[n])
			}
			var last time.Time
			switch {
			case math.IsNaN(s.value) || math.IsInf(s.value, 0):
				m.Status = integrations.StatusUnknown
			case spec.Kind == integrations.KindTimestamp:
				t, ok := integrations.UnixAny(s.value)
				if !ok {
					m.Status = integrations.StatusUnknown
					break
				}
				last = t
				m.Value = max(now.Sub(t).Seconds(), 0)
				m.Status = spec.Status(m.Value)
			default:
				m.Value = s.value
				m.Status = spec.Status(m.Value)
			}
			snap.Metrics = append(snap.Metrics, m)
			if spec.Backup && spec.Kind == integrations.KindTimestamp {
				b := integrations.Backup{ID: m.Key, Name: spec.Name, Group: m.Group, Result: integrations.StatusOK}
				if last.IsZero() {
					b.Result = integrations.StatusUnknown
				} else {
					b.LastRun = &last
				}
				snap.Backups = append(snap.Backups, b)
			}
		}
	}
	return snap, nil
}
