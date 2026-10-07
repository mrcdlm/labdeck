package config

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/mrcdlm/labdeck/internal/store"
)

// Keys der in der DB gespeicherten Einstellungen.
const (
	KeyTimezone           = "timezone"
	KeyRawRetentionDays   = "raw_retention_days"
	KeyDailyRetentionDays = "daily_retention_days"
	KeyBackupMaxAgeHours  = "backup_max_age_hours"
)

const (
	// Rohdaten (jede Abfrage): 30 Tage
	DefaultRawRetentionDays = 30
	MinRawRetentionDays     = 1
	MaxRawRetentionDays     = 365

	// Tageswerte (für Diagramme wie „Fotos pro Monat“): 1 Jahr
	DefaultDailyRetentionDays = 365
	MinDailyRetentionDays     = 30
	MaxDailyRetentionDays     = 3650

	// Backups gelten nach so vielen Stunden ohne Lauf als überfällig (je
	// Backup änderbar); täglich + 2 h Puffer
	DefaultBackupMaxAgeHours = 26
	MinBackupMaxAgeHours     = 1
	MaxBackupMaxAgeHours     = 24 * 366
)

// Settings sind zur Laufzeit änderbare Einstellungen (keine Secrets).
type Settings struct {
	// Timezone: IANA-Zeitzone für die Anzeige (z. B. "Europe/Berlin");
	// leer = Zeitzone des Browsers.
	Timezone string
	// RawRetentionDays: Aufbewahrung der Rohdaten in Tagen.
	RawRetentionDays int
	// DailyRetentionDays: Aufbewahrung der Tageswerte in Tagen.
	DailyRetentionDays int
	// BackupMaxAgeHours: Standard-Höchstalter von Backups in Stunden.
	BackupMaxAgeHours int
}

type settingsStore interface {
	GetSetting(ctx context.Context, key string) (string, error)
}

// LoadSettings liest die Einstellungen aus der DB und füllt Defaults auf.
// Ungültige Werte werden ignoriert (Default), damit die App immer startet.
func LoadSettings(ctx context.Context, s settingsStore) (Settings, []error) {
	out := Settings{RawRetentionDays: DefaultRawRetentionDays, DailyRetentionDays: DefaultDailyRetentionDays,
		BackupMaxAgeHours: DefaultBackupMaxAgeHours}
	var warnings []error

	if v, err := s.GetSetting(ctx, KeyTimezone); err == nil {
		if validTimezone(v) {
			out.Timezone = v
		} else {
			warnings = append(warnings, fmt.Errorf("%s invalid (%q), using default", KeyTimezone, v))
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		warnings = append(warnings, err)
	}

	loadInt := func(key string, minV, maxV int, dst *int) {
		v, err := s.GetSetting(ctx, key)
		if errors.Is(err, store.ErrNotFound) {
			return
		}
		if err != nil {
			warnings = append(warnings, err)
			return
		}
		n, perr := strconv.Atoi(v)
		if perr != nil || n < minV || n > maxV {
			warnings = append(warnings, fmt.Errorf("%s invalid (%q), using default", key, v))
			return
		}
		*dst = n
	}
	loadInt(KeyRawRetentionDays, MinRawRetentionDays, MaxRawRetentionDays, &out.RawRetentionDays)
	loadInt(KeyDailyRetentionDays, MinDailyRetentionDays, MaxDailyRetentionDays, &out.DailyRetentionDays)
	loadInt(KeyBackupMaxAgeHours, MinBackupMaxAgeHours, MaxBackupMaxAgeHours, &out.BackupMaxAgeHours)

	return out, warnings
}

// validTimezone: leer (Browser) oder eine bekannte IANA-Zeitzone.
func validTimezone(tz string) bool {
	if tz == "" {
		return true
	}
	if tz == "Local" {
		return false
	}
	_, err := time.LoadLocation(tz)
	return err == nil
}
