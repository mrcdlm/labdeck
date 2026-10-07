package integrations

import (
	"testing"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
)

func TestListField(t *testing.T) {
	s := ConfigSchema{Fields: []Field{
		{Key: "values", Type: FieldList, Required: true, Max: Ptr(2), Label: Text{DE: "Werte", EN: "Values"},
			Item: ValueFields(Field{Key: "path", Type: FieldString, Required: true, Label: Text{DE: "Pfad", EN: "Path"}})},
	}}
	out, msgs := s.Normalize(map[string]any{"values": []any{
		map[string]any{"name": "Dokumente", "path": "count", "warn": 10.0},
	}}, i18n.DE)
	if len(msgs) != 0 {
		t.Fatal(msgs)
	}
	item := out["values"].([]any)[0].(map[string]any)
	if item["kind"] != KindNumber || item["direction"] != DirectionAbove || item["warn"] != 10.0 {
		t.Fatalf("Defaults: %v", item)
	}
	c := Config{Values: out}
	spec := ParseValueSpec(c.List("values")[0])
	if spec.Name != "Dokumente" || spec.Warn == nil || spec.Fail != nil || spec.Status(12) != StatusWarning {
		t.Fatalf("%+v", spec)
	}

	// Fehler je Eintrag mit Nummer
	_, msgs = s.Normalize(map[string]any{"values": []any{
		map[string]any{"name": "a", "path": "x"}, map[string]any{"name": "b"},
	}}, i18n.DE)
	if len(msgs) != 1 || i18n.T(i18n.DE, msgs[0]) != "Werte 2: Pfad: Pflichtfeld" {
		t.Fatalf("%v", msgs)
	}
	// zu viele, fehlend, falscher Typ
	_, msgs = s.Normalize(map[string]any{"values": []any{map[string]any{}, map[string]any{}, map[string]any{}}}, i18n.EN)
	if len(msgs) != 1 || msgs[0].Code != "integration.field_list_max" {
		t.Fatalf("%v", msgs)
	}
	if _, msgs = s.Normalize(map[string]any{"values": []any{}}, i18n.EN); len(msgs) != 1 || msgs[0].Code != "integration.field_required" {
		t.Fatalf("leer: %v", msgs)
	}
	if _, msgs = s.Normalize(map[string]any{"values": "x"}, i18n.EN); len(msgs) != 1 || msgs[0].Code != "integration.field_type" {
		t.Fatalf("Typ: %v", msgs)
	}
}

func TestValueStatus(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	up := ValueSpec{Warn: f(80), Fail: f(90)}
	if up.Status(50) != StatusOK || up.Status(80) != StatusWarning || up.Status(95) != StatusError {
		t.Error("above")
	}
	down := ValueSpec{Direction: DirectionBelow, Warn: f(20), Fail: f(10)}
	if down.Status(50) != StatusOK || down.Status(15) != StatusWarning || down.Status(5) != StatusError {
		t.Error("below")
	}
	// Zeitstempel: Alter in Sekunden, Schwellwerte in Stunden
	ts := ValueSpec{Kind: KindTimestamp, Warn: f(24), Fail: f(48)}
	if ts.Status(3600) != StatusOK || ts.Status(30*3600) != StatusWarning || ts.Status(50*3600) != StatusError ||
		ts.MetricUnit() != "s" {
		t.Error("timestamp")
	}
	if (ValueSpec{}).Status(1e9) != StatusOK {
		t.Error("ohne Schwellwerte")
	}
}

func TestParseTimestamp(t *testing.T) {
	want := time.Date(2026, 10, 7, 2, 0, 0, 0, time.UTC)
	for _, s := range []string{"1791338400", "1791338400000", "1791338400000000", "2026-10-07T02:00:00Z",
		"2026-10-07T04:00:00+02:00", "2026-10-07 02:00:00", "Wed, 07 Oct 2026 02:00:00 +0000"} {
		got, ok := ParseTimestamp(s)
		if !ok || !got.Equal(want) {
			t.Errorf("%q → %v %v", s, got, ok)
		}
	}
	for _, s := range []string{"", "gestern", "0", "-5"} {
		if _, ok := ParseTimestamp(s); ok {
			t.Errorf("%q gilt als Zeitstempel", s)
		}
	}
}

func TestSlug(t *testing.T) {
	if got := UniqueKeys([]string{"Freier Speicher", "freier speicher!", "", "Ölstand °C"}); got[0] != "freier_speicher" ||
		got[1] != "freier_speicher_2" || got[2] != "value" || got[3] != "ölstand_c" {
		t.Fatal(got)
	}
}
