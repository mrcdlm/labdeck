package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/mrcdlm/labdeck/internal/config"
	"github.com/mrcdlm/labdeck/internal/events"
	"github.com/mrcdlm/labdeck/internal/i18n"
)

type settingsService interface {
	Get() config.Settings
	Update(ctx context.Context, s config.Settings) (config.Settings, error)
}

// Info sind nicht geheime Angaben zur Installation.
type Info struct {
	Version string `json:"version"`
	DataDir string `json:"data_dir"`
	// SecretKeySet: LABDECK_SECRET_KEY ist gesetzt (nötig für Zugangsdaten
	// von Integrationen)
	SecretKeySet bool `json:"secret_key_set"`
}

type settingsDTO struct {
	Timezone           string         `json:"timezone"` // leer = Zeitzone des Browsers
	RawRetentionDays   int            `json:"raw_retention_days"`
	DailyRetentionDays int            `json:"daily_retention_days"`
	Limits             map[string]int `json:"limits"`
}

func settingsToDTO(c config.Settings) settingsDTO {
	return settingsDTO{
		Timezone:           c.Timezone,
		RawRetentionDays:   c.RawRetentionDays,
		DailyRetentionDays: c.DailyRetentionDays,
		Limits: map[string]int{
			"raw_retention_days_min":   config.MinRawRetentionDays,
			"raw_retention_days_max":   config.MaxRawRetentionDays,
			"daily_retention_days_min": config.MinDailyRetentionDays,
			"daily_retention_days_max": config.MaxDailyRetentionDays,
		},
	}
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	if s.settings == nil {
		writeMsg(w, r, http.StatusServiceUnavailable, i18n.M("api.settings_unavailable"))
		return
	}
	writeJSON(w, http.StatusOK, settingsToDTO(s.settings.Get()))
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	if s.settings == nil {
		writeMsg(w, r, http.StatusServiceUnavailable, i18n.M("api.settings_unavailable"))
		return
	}
	var in settingsDTO // Limits wird ignoriert (Rückgabe von GET)
	if !decodeJSON(w, r, &in) {
		return
	}
	next := config.Settings{
		Timezone:           in.Timezone,
		RawRetentionDays:   in.RawRetentionDays,
		DailyRetentionDays: in.DailyRetentionDays,
	}

	saved, err := s.settings.Update(r.Context(), next)
	var verr *config.ValidationError
	if errors.As(err, &verr) {
		writeMsgs(w, r, http.StatusBadRequest, verr.Msgs)
		return
	}
	if err != nil {
		s.internalError(w, r, "Einstellungen speichern", err)
		return
	}
	s.log.Info("Einstellungen geändert", "timezone", saved.Timezone,
		"raw_retention_days", saved.RawRetentionDays, "daily_retention_days", saved.DailyRetentionDays)
	if s.events != nil {
		s.events.Publish(events.TopicSettings)
	}
	writeJSON(w, http.StatusOK, settingsToDTO(saved))
}

func (s *Server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.info)
}
