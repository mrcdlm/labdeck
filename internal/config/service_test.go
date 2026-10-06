package config

import (
	"context"
	"testing"

	"github.com/mrcdlm/labdeck/internal/store"
)

type memSettings map[string]string

func (m memSettings) GetSetting(_ context.Context, k string) (string, error) {
	if v, ok := m[k]; ok {
		return v, nil
	}
	return "", store.ErrNotFound
}

func (m memSettings) SetSetting(_ context.Context, k, v string) error { m[k] = v; return nil }

func TestLoadSettingsDefaultsAndInvalid(t *testing.T) {
	st := memSettings{KeyTimezone: "Mars/Olympus", KeyRawRetentionDays: "0", KeyDailyRetentionDays: "400"}
	got, warnings := LoadSettings(context.Background(), st)
	if len(warnings) != 2 {
		t.Fatalf("warnings = %v", warnings)
	}
	if got.Timezone != "" || got.RawRetentionDays != DefaultRawRetentionDays || got.DailyRetentionDays != 400 {
		t.Fatalf("got = %+v", got)
	}
}

func TestSettingsService(t *testing.T) {
	ctx := context.Background()
	st := memSettings{}
	svc, warnings := NewSettingsService(ctx, st)
	if len(warnings) != 0 || svc.Get().RawRetentionDays != DefaultRawRetentionDays {
		t.Fatalf("warnings=%v settings=%+v", warnings, svc.Get())
	}

	var called Settings
	svc.OnChange(func(_, cur Settings) { called = cur })

	next := svc.Get()
	next.Timezone = "Europe/Berlin"
	next.RawRetentionDays = 14
	got, err := svc.Update(ctx, next)
	if err != nil {
		t.Fatal(err)
	}
	if called.Timezone != "Europe/Berlin" || got.RawRetentionDays != 14 {
		t.Fatalf("got=%+v called=%+v", got, called)
	}
	if st[KeyTimezone] != "Europe/Berlin" || st[KeyRawRetentionDays] != "14" {
		t.Fatalf("gespeichert: %v", st)
	}

	// neu laden liefert denselben Stand
	svc2, w := NewSettingsService(ctx, st)
	if len(w) != 0 || svc2.Get() != got {
		t.Fatalf("neu geladen: %+v %v", svc2.Get(), w)
	}

	for _, bad := range []func(*Settings){
		func(s *Settings) { s.Timezone = "Mars/Olympus" },
		func(s *Settings) { s.Timezone = "Local" },
		func(s *Settings) { s.RawRetentionDays = 0 },
		func(s *Settings) { s.RawRetentionDays = MaxRawRetentionDays + 1 },
		func(s *Settings) { s.DailyRetentionDays = MinDailyRetentionDays - 1 },
	} {
		n := svc.Get()
		bad(&n)
		if _, err := svc.Update(ctx, n); !IsValidation(err) {
			t.Errorf("Validierungsfehler erwartet: %+v → %v", n, err)
		}
	}
}
