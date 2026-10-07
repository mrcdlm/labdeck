package integrations

import (
	"fmt"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/mrcdlm/labdeck/internal/i18n"
)

// Text ist ein zweisprachiger Text für Formulare.
type Text struct {
	DE string `json:"de"`
	EN string `json:"en"`
}

// In liefert den Text in lang (Englisch, wenn Deutsch fehlt).
func (t Text) In(lang i18n.Lang) string {
	if lang == i18n.DE && t.DE != "" {
		return t.DE
	}
	return t.EN
}

type FieldType string

const (
	FieldString FieldType = "string" // einzeilig
	FieldText   FieldType = "text"   // mehrzeilig
	FieldURL    FieldType = "url"    // http(s)-Adresse
	FieldNumber FieldType = "number"
	FieldBool   FieldType = "bool"
	FieldSelect FieldType = "select"
	// FieldSecret wird verschlüsselt gespeichert und nie zurückgegeben.
	FieldSecret FieldType = "secret"
	// FieldList ist eine Liste gleich aufgebauter Einträge (Unterfelder in
	// Item, ohne Secrets), z. B. mehrere Abfragen; Wert: []map[string]any.
	FieldList FieldType = "list"
)

type Option struct {
	Value string `json:"value"`
	Label Text   `json:"label"`
}

type Field struct {
	Key         string    `json:"key"`
	Type        FieldType `json:"type"`
	Label       Text      `json:"label"`
	Help        Text      `json:"help"`
	Placeholder string    `json:"placeholder,omitempty"`
	Required    bool      `json:"required"`
	// Default: string, float64 oder bool je nach Typ (nicht für Secrets)
	Default any      `json:"default,omitempty"`
	Options []Option `json:"options,omitempty"`
	Min     *float64 `json:"min,omitempty"`
	Max     *float64 `json:"max,omitempty"`
	// Half: im Formular halbe Breite (zwei Felder nebeneinander)
	Half bool `json:"half,omitempty"`
	// Item: Unterfelder je Eintrag (nur FieldList); Max begrenzt die Anzahl
	Item []Field `json:"item,omitempty"`
	// ShowIf: Feld nur anzeigen, wenn ein anderes (Auswahl-)Feld einen der
	// Werte hat; nur Darstellung, geprüft wird trotzdem
	ShowIf *Condition `json:"show_if,omitempty"`
}

// Condition: Feld Key hat einen der Werte.
type Condition struct {
	Key    string   `json:"key"`
	Values []string `json:"values"`
}

// maxListItems gilt, wenn eine Liste kein Max hat.
const maxListItems = 50

// ConfigSchema beschreibt eine Integration für das Einstellungsformular.
type ConfigSchema struct {
	Title       Text `json:"title"`
	Description Text `json:"description"`
	// Icon: Name eines lucide-Icons, z. B. "heart-pulse"
	Icon string `json:"icon"`
	// Docs: Link zur Dokumentation der Quelle
	Docs   string  `json:"docs,omitempty"`
	Fields []Field `json:"fields"`
}

// Field liefert das Feld mit key.
func (s ConfigSchema) Field(key string) (Field, bool) {
	for _, f := range s.Fields {
		if f.Key == key {
			return f, true
		}
	}
	return Field{}, false
}

// Ptr hilft beim Setzen von Min/Max.
func Ptr(v float64) *float64 { return &v }

const maxStringLen = 4000

// Normalize prüft die (nicht geheimen) Werte gegen das Schema, ergänzt
// Defaults und liefert bereinigte Werte. Secret-Felder werden hier nicht
// geprüft (siehe Manager). Unbekannte Schlüssel sind ein Fehler.
func (s ConfigSchema) Normalize(in map[string]any, lang i18n.Lang) (map[string]any, []i18n.Msg) {
	out := map[string]any{}
	var msgs []i18n.Msg
	bad := func(f Field, code string, kv ...string) {
		msgs = append(msgs, i18n.M(code, append([]string{"field", f.Label.In(lang)}, kv...)...))
	}

	for key := range in {
		if f, ok := s.Field(key); !ok || f.Type == FieldSecret {
			msgs = append(msgs, i18n.M("integration.field_unknown", "field", key))
		}
	}

	for _, f := range s.Fields {
		if f.Type == FieldSecret {
			continue
		}
		v, present := in[f.Key]
		// Default nur, wenn das Feld fehlt – ein bewusst geleertes Feld bleibt leer.
		if !present || v == nil {
			if f.Default != nil {
				out[f.Key] = f.Default
			} else if f.Required {
				bad(f, "integration.field_required")
			}
			continue
		}
		if v == "" {
			if f.Required {
				bad(f, "integration.field_required")
			}
			continue
		}
		switch f.Type {
		case FieldString, FieldText, FieldURL, FieldSelect:
			str, ok := v.(string)
			if !ok {
				bad(f, "integration.field_type")
				continue
			}
			if f.Type != FieldText {
				str = strings.TrimSpace(str)
			}
			if len(str) > maxStringLen {
				bad(f, "integration.field_too_long")
				continue
			}
			switch f.Type {
			case FieldURL:
				norm, code := normalizeURL(str)
				if code != "" {
					bad(f, code)
					continue
				}
				str = norm
			case FieldSelect:
				if !slices.ContainsFunc(f.Options, func(o Option) bool { return o.Value == str }) {
					bad(f, "integration.field_option")
					continue
				}
			case FieldString:
				if strings.ContainsAny(str, "\r\n") {
					bad(f, "integration.field_newline")
					continue
				}
			}
			if str == "" {
				if f.Required {
					bad(f, "integration.field_required")
				}
				continue
			}
			out[f.Key] = str
		case FieldNumber:
			n, ok := v.(float64)
			if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
				bad(f, "integration.field_type")
				continue
			}
			if (f.Min != nil && n < *f.Min) || (f.Max != nil && n > *f.Max) {
				bad(f, "integration.field_range", "min", fmtNum(f.Min), "max", fmtNum(f.Max))
				continue
			}
			out[f.Key] = n
		case FieldList:
			items, imsgs := normalizeList(f, v, lang)
			msgs = append(msgs, imsgs...)
			if len(imsgs) > 0 {
				continue
			}
			if len(items) == 0 {
				if f.Required {
					bad(f, "integration.field_required")
				}
				continue
			}
			out[f.Key] = items
		case FieldBool:
			b, ok := v.(bool)
			if !ok {
				bad(f, "integration.field_type")
				continue
			}
			out[f.Key] = b
		default:
			msgs = append(msgs, i18n.Raw(fmt.Sprintf("unknown field type %q", f.Type)))
		}
	}
	return out, msgs
}

// normalizeList prüft jeden Eintrag gegen f.Item; Meldungen nennen die Nummer.
func normalizeList(f Field, v any, lang i18n.Lang) ([]any, []i18n.Msg) {
	raw, ok := v.([]any)
	if !ok {
		return nil, []i18n.Msg{i18n.M("integration.field_type", "field", f.Label.In(lang))}
	}
	limit := maxListItems
	if f.Max != nil {
		limit = int(*f.Max)
	}
	if len(raw) > limit {
		return nil, []i18n.Msg{i18n.M("integration.field_list_max", "field", f.Label.In(lang), "max", strconv.Itoa(limit))}
	}
	sub := ConfigSchema{Fields: f.Item}
	var out []any
	var msgs []i18n.Msg
	for i, it := range raw {
		m, ok := it.(map[string]any)
		if !ok {
			msgs = append(msgs, i18n.M("integration.field_type", "field", f.Label.In(lang)))
			continue
		}
		vals, imsgs := sub.Normalize(m, lang)
		for _, im := range imsgs {
			msgs = append(msgs, i18n.M("integration.field_item", "field", f.Label.In(lang), "n", strconv.Itoa(i+1),
				"detail", i18n.Nest(im)))
		}
		out = append(out, vals)
	}
	return out, msgs
}

func fmtNum(p *float64) string {
	if p == nil {
		return "–"
	}
	return strconv.FormatFloat(*p, 'f', -1, 64)
}

// normalizeURL erlaubt nur http(s) mit Host und ohne Zugangsdaten in der
// Adresse (die gehören in ein Secret-Feld). Die Adresse bleibt sonst unverändert
// (manche APIs brauchen den Schrägstrich am Ende); Basisadressen kürzt
// Config.BaseURL.
func normalizeURL(s string) (string, string) {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", "integration.field_url"
	}
	if u.User != nil {
		return "", "integration.field_url_userinfo"
	}
	return s, ""
}
