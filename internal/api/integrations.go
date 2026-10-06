package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
)

type integrationService interface {
	Registry() *integrations.Registry
	HasKey() bool
	List(ctx context.Context) ([]integrations.View, error)
	Get(ctx context.Context, id int64) (integrations.View, error)
	Create(ctx context.Context, in integrations.Input, lang i18n.Lang) (integrations.View, error)
	Update(ctx context.Context, id int64, in integrations.Input, lang i18n.Lang) (integrations.View, error)
	Delete(ctx context.Context, id int64) error
	Test(ctx context.Context, id int64, in integrations.Input, lang i18n.Lang) (integrations.TestResult, error)
	Poll(ctx context.Context, id int64) error
}

// Abfrage per Button: großzügiges Zeitlimit, auch zu Ende führen, wenn der
// Browser die Verbindung schließt.
const pollTimeout = 5 * time.Minute

// --- Typen (Formular-Schemas) ---------------------------------------------

type optionDTO struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type fieldDTO struct {
	Key         string      `json:"key"`
	Type        string      `json:"type"`
	Label       string      `json:"label"`
	Help        string      `json:"help,omitempty"`
	Placeholder string      `json:"placeholder,omitempty"`
	Required    bool        `json:"required"`
	Default     any         `json:"default,omitempty"`
	Options     []optionDTO `json:"options,omitempty"`
	Min         *float64    `json:"min,omitempty"`
	Max         *float64    `json:"max,omitempty"`
}

type typeDTO struct {
	Type        string     `json:"type"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Icon        string     `json:"icon"`
	Docs        string     `json:"docs,omitempty"`
	Fields      []fieldDTO `json:"fields"`
}

type typesDTO struct {
	Types []typeDTO `json:"types"`
	// SecretKeySet: ohne LABDECK_SECRET_KEY nur ${SECRET_…}-Verweise möglich
	SecretKeySet bool           `json:"secret_key_set"`
	Limits       map[string]int `json:"limits"`
}

func (s *Server) handleIntegrationTypes(w http.ResponseWriter, r *http.Request) {
	lang := i18n.FromRequest(r)
	out := typesDTO{Types: []typeDTO{}, SecretKeySet: s.integrations.HasKey(), Limits: map[string]int{
		"interval_min": int(integrations.MinInterval.Seconds()), "interval_max": int(integrations.MaxInterval.Seconds()),
		"interval_default": int(integrations.DefaultInterval.Seconds()),
		"timeout_min":      int(integrations.MinTimeout.Seconds()), "timeout_max": int(integrations.MaxTimeout.Seconds()),
		"timeout_default": int(integrations.DefaultTimeout.Seconds()),
	}}
	for _, integ := range s.integrations.Registry().All() {
		sc := integ.Schema()
		t := typeDTO{Type: integ.Type(), Title: sc.Title.In(lang), Description: sc.Description.In(lang),
			Icon: sc.Icon, Docs: sc.Docs, Fields: []fieldDTO{}}
		for _, f := range sc.Fields {
			fd := fieldDTO{Key: f.Key, Type: string(f.Type), Label: f.Label.In(lang), Help: f.Help.In(lang),
				Placeholder: f.Placeholder, Required: f.Required, Default: f.Default, Min: f.Min, Max: f.Max}
			for _, o := range f.Options {
				fd.Options = append(fd.Options, optionDTO{Value: o.Value, Label: o.Label.In(lang)})
			}
			t.Fields = append(t.Fields, fd)
		}
		out.Types = append(out.Types, t)
	}
	writeJSON(w, http.StatusOK, out)
}

// --- Integrationen ---------------------------------------------------------

type integrationDTO struct {
	integrations.View
	LastError     string `json:"last_error,omitempty"`
	LastErrorCode string `json:"last_error_code,omitempty"`
}

func toDTO(v integrations.View, lang i18n.Lang) integrationDTO {
	return integrationDTO{View: v, LastError: i18n.T(lang, v.LastError), LastErrorCode: v.LastError.Code}
}

type testResultDTO struct {
	integrations.TestResult
	Error string `json:"error,omitempty"`
}

func (s *Server) handleListIntegrations(w http.ResponseWriter, r *http.Request) {
	list, err := s.integrations.List(r.Context())
	if err != nil {
		s.internalError(w, r, "Integrationen laden", err)
		return
	}
	lang := i18n.FromRequest(r)
	out := make([]integrationDTO, 0, len(list))
	for _, v := range list {
		out = append(out, toDTO(v, lang))
	}
	writeJSON(w, http.StatusOK, out)
}

func integrationID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeMsg(w, r, http.StatusNotFound, i18n.M("integration.not_found_id"))
		return 0, false
	}
	return id, true
}

// integrationError schreibt Validierungs-, 404- und interne Fehler.
func (s *Server) integrationError(w http.ResponseWriter, r *http.Request, what string, err error) {
	var verr *integrations.ValidationError
	switch {
	case errors.As(err, &verr):
		writeMsgs(w, r, http.StatusBadRequest, verr.Msgs)
	case errors.Is(err, integrations.ErrNotFound):
		writeMsg(w, r, http.StatusNotFound, i18n.M("integration.not_found_id"))
	default:
		s.internalError(w, r, what, err)
	}
}

func (s *Server) handleGetIntegration(w http.ResponseWriter, r *http.Request) {
	id, ok := integrationID(w, r)
	if !ok {
		return
	}
	v, err := s.integrations.Get(r.Context(), id)
	if err != nil {
		s.integrationError(w, r, "Integration laden", err)
		return
	}
	writeJSON(w, http.StatusOK, toDTO(v, i18n.FromRequest(r)))
}

func (s *Server) handleCreateIntegration(w http.ResponseWriter, r *http.Request) {
	var in integrations.Input
	if !decodeJSON(w, r, &in) {
		return
	}
	lang := i18n.FromRequest(r)
	v, err := s.integrations.Create(r.Context(), in, lang)
	if err != nil {
		s.integrationError(w, r, "Integration anlegen", err)
		return
	}
	writeJSON(w, http.StatusCreated, toDTO(v, lang))
}

func (s *Server) handleUpdateIntegration(w http.ResponseWriter, r *http.Request) {
	id, ok := integrationID(w, r)
	if !ok {
		return
	}
	var in integrations.Input
	if !decodeJSON(w, r, &in) {
		return
	}
	lang := i18n.FromRequest(r)
	v, err := s.integrations.Update(r.Context(), id, in, lang)
	if err != nil {
		s.integrationError(w, r, "Integration ändern", err)
		return
	}
	writeJSON(w, http.StatusOK, toDTO(v, lang))
}

func (s *Server) handleDeleteIntegration(w http.ResponseWriter, r *http.Request) {
	id, ok := integrationID(w, r)
	if !ok {
		return
	}
	if err := s.integrations.Delete(r.Context(), id); err != nil {
		s.integrationError(w, r, "Integration löschen", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleTestIntegration prüft die Eingabe des Formulars, ohne zu speichern.
// Mit id ergänzt die gespeicherte Integration leer gelassene Secrets. Die
// Antwort enthält nur Erfolg/Fehler und Anzahlen – nie Inhalte der Quelle.
func (s *Server) handleTestIntegration(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID int64 `json:"id"`
		integrations.Input
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	lang := i18n.FromRequest(r)
	res, err := s.integrations.Test(r.Context(), in.ID, in.Input, lang)
	if err != nil {
		s.integrationError(w, r, "Integration testen", err)
		return
	}
	writeJSON(w, http.StatusOK, testResultDTO{TestResult: res, Error: i18n.T(lang, res.Error)})
}

// handlePollIntegration fragt sofort ab und liefert den neuen Stand.
func (s *Server) handlePollIntegration(w http.ResponseWriter, r *http.Request) {
	id, ok := integrationID(w, r)
	if !ok {
		return
	}
	if _, err := s.integrations.Get(r.Context(), id); err != nil {
		s.integrationError(w, r, "Integration laden", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), pollTimeout)
	defer cancel()
	if err := s.integrations.Poll(ctx, id); err != nil {
		s.internalError(w, r, "Integration abfragen", err)
		return
	}
	v, err := s.integrations.Get(ctx, id)
	if err != nil {
		s.integrationError(w, r, "Integration laden", err)
		return
	}
	writeJSON(w, http.StatusOK, toDTO(v, i18n.FromRequest(r)))
}
