package config

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"sync"
	_ "time/tzdata" // Zeitzonen auch ohne tzdata im Container (distroless)

	"github.com/mrcdlm/labdeck/internal/i18n"
)

type settingsRW interface {
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
}

// ValidationError sind Eingabefehler, die dem Benutzer angezeigt werden.
type ValidationError struct{ Msgs []i18n.Msg }

func (e *ValidationError) Error() string { return i18n.Join(i18n.EN, e.Msgs) }

// SettingsService hält die aktuellen Einstellungen, prüft und speichert
// Änderungen und benachrichtigt Abonnenten, damit Änderungen ohne Neustart
// greifen.
type SettingsService struct {
	store settingsRW

	mu        sync.RWMutex
	cur       Settings
	listeners []func(old, cur Settings)
}

// NewSettingsService lädt die Einstellungen; ungültige gespeicherte Werte
// werden als Warnungen gemeldet und durch Defaults ersetzt.
func NewSettingsService(ctx context.Context, st settingsRW) (*SettingsService, []error) {
	cur, warnings := LoadSettings(ctx, st)
	return &SettingsService{store: st, cur: cur}, warnings
}

func (s *SettingsService) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

// OnChange registriert fn; es wird nach jeder erfolgreichen Änderung aufgerufen.
func (s *SettingsService) OnChange(fn func(old, cur Settings)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, fn)
}

// Update prüft und speichert die Einstellungen vollständig.
func (s *SettingsService) Update(ctx context.Context, next Settings) (Settings, error) {
	if err := validate(next); err != nil {
		return Settings{}, err
	}
	for _, kv := range [][2]string{
		{KeyTimezone, next.Timezone},
		{KeyRawRetentionDays, strconv.Itoa(next.RawRetentionDays)},
		{KeyDailyRetentionDays, strconv.Itoa(next.DailyRetentionDays)},
	} {
		if err := s.store.SetSetting(ctx, kv[0], kv[1]); err != nil {
			return Settings{}, err
		}
	}

	s.mu.Lock()
	old := s.cur
	s.cur = next
	listeners := slices.Clone(s.listeners)
	s.mu.Unlock()
	for _, fn := range listeners {
		fn(old, next)
	}
	return s.Get(), nil
}

func validate(n Settings) error {
	var msgs []i18n.Msg
	add := func(code string, kv ...string) { msgs = append(msgs, i18n.M(code, kv...)) }
	if !validTimezone(n.Timezone) {
		add("settings.timezone_invalid", "name", strconv.Quote(n.Timezone))
	}
	if n.RawRetentionDays < MinRawRetentionDays || n.RawRetentionDays > MaxRawRetentionDays {
		add("settings.raw_retention_range",
			"min", strconv.Itoa(MinRawRetentionDays), "max", strconv.Itoa(MaxRawRetentionDays))
	}
	if n.DailyRetentionDays < MinDailyRetentionDays || n.DailyRetentionDays > MaxDailyRetentionDays {
		add("settings.daily_retention_range",
			"min", strconv.Itoa(MinDailyRetentionDays), "max", strconv.Itoa(MaxDailyRetentionDays))
	}
	if len(msgs) > 0 {
		return &ValidationError{Msgs: msgs}
	}
	return nil
}

// IsValidation meldet, ob err ein Eingabefehler ist.
func IsValidation(err error) bool {
	var v *ValidationError
	return errors.As(err, &v)
}
