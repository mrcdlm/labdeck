package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mrcdlm/labdeck/internal/events"
	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/notify"
	"github.com/mrcdlm/labdeck/internal/secrets"
	"github.com/mrcdlm/labdeck/internal/store"
)

// Webhooks wie in dnsdeck; zusätzlich ein verschlüsseltes Token je Webhook,
// das als ${TOKEN} bzw. {{token}} eingesetzt wird.

type webhookStore interface {
	ListWebhooks(ctx context.Context) ([]store.Webhook, error)
	GetWebhook(ctx context.Context, id int64) (store.Webhook, error)
	CreateWebhook(ctx context.Context, w store.Webhook, seal func(id int64) (string, error)) (store.Webhook, error)
	UpdateWebhook(ctx context.Context, w store.Webhook) (store.Webhook, error)
	DeleteWebhook(ctx context.Context, id int64) error
}

type webhookTester interface {
	SendTest(ctx context.Context, id int64) (notify.TestResult, error)
	Preview(w store.Webhook, eventType string) (notify.Request, error)
}

// WebhookSealAD bindet die Verschlüsselung des Tokens an den Webhook.
func WebhookSealAD(id int64) string { return "labdeck/webhook/" + strconv.FormatInt(id, 10) }

// webhookDTO ergänzt die Konfiguration um verwendete, aber nicht gesetzte
// Env-Variablen (nur Namen, nie Werte) und ob ein Token gespeichert ist.
type webhookDTO struct {
	store.Webhook
	MissingEnv []string `json:"missing_env"`
	TokenSet   bool     `json:"token_set"`
	// TokenMissing: ${TOKEN} wird verwendet, aber keins ist gespeichert
	TokenMissing bool `json:"token_missing"`
}

func (s *Server) toWebhookDTO(w store.Webhook, lang i18n.Lang) webhookDTO {
	if !w.LastErrorMsg.IsZero() {
		w.LastError = i18n.T(lang, w.LastErrorMsg)
	}
	missing := []string{}
	if s.webhookEnv != nil {
		if m := notify.MissingEnv(w, s.webhookEnv); m != nil {
			missing = m
		}
	}
	return webhookDTO{Webhook: w, MissingEnv: missing, TokenSet: w.Token != "",
		TokenMissing: w.Token == "" && notify.UsesToken(w)}
}

type webhookInput struct {
	Name         string         `json:"name"`
	Enabled      *bool          `json:"enabled"`
	Method       string         `json:"method"`
	URL          string         `json:"url"`
	Headers      []store.Header `json:"headers"`
	ContentType  string         `json:"content_type"`
	BodyTemplate string         `json:"body_template"`
	Events       []string       `json:"events"`
	// Token: neuer Wert (leer = unverändert); ClearToken entfernt ihn
	Token      string `json:"token"`
	ClearToken bool   `json:"clear_token"`
}

func (in webhookInput) toWebhook() store.Webhook {
	w := store.Webhook{
		Name: strings.TrimSpace(in.Name), Enabled: in.Enabled == nil || *in.Enabled,
		Method: strings.ToUpper(strings.TrimSpace(in.Method)), URL: strings.TrimSpace(in.URL),
		ContentType: strings.TrimSpace(in.ContentType), BodyTemplate: in.BodyTemplate,
		Headers: []store.Header{}, Events: []string{},
	}
	if w.Method == "" {
		w.Method = http.MethodPost
	}
	for _, h := range in.Headers {
		if n := strings.TrimSpace(h.Name); n != "" {
			w.Headers = append(w.Headers, store.Header{Name: n, Value: strings.TrimSpace(h.Value)})
		}
	}
	// Reihenfolge wie in notify.EventTypes, doppelte entfernen
	for _, t := range append(slices.Clone(notify.EventTypes), in.Events...) {
		if slices.Contains(in.Events, t) && !slices.Contains(w.Events, t) {
			w.Events = append(w.Events, t)
		}
	}
	return w
}

func (s *Server) webhookChanged() { events.Publish(s.events, events.TopicWebhooks) }

func webhookID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeMsg(w, r, http.StatusNotFound, i18n.M("webhook.not_found"))
		return 0, false
	}
	return id, true
}

func (s *Server) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	list, err := s.webhookStore.ListWebhooks(r.Context())
	if err != nil {
		s.internalError(w, r, "Webhooks lesen", err)
		return
	}
	out := make([]webhookDTO, len(list))
	for i, wh := range list {
		out[i] = s.toWebhookDTO(wh, i18n.FromRequest(r))
	}
	writeJSON(w, http.StatusOK, out)
}

// decodeWebhook liest und prüft die Eingabe; bei Fehlern ist die Antwort geschrieben.
func (s *Server) decodeWebhook(w http.ResponseWriter, r *http.Request) (webhookInput, store.Webhook, bool) {
	var in webhookInput
	if !decodeJSON(w, r, &in) {
		return in, store.Webhook{}, false
	}
	wh := in.toWebhook()
	var verr *notify.ValidationError
	if err := notify.Validate(wh); errors.As(err, &verr) {
		writeMsgs(w, r, http.StatusBadRequest, verr.Msgs)
		return in, store.Webhook{}, false
	}
	if strings.TrimSpace(in.Token) != "" && s.box == nil {
		writeMsg(w, r, http.StatusBadRequest, i18n.M("webhook.no_key"))
		return in, store.Webhook{}, false
	}
	return in, wh, true
}

func (s *Server) sealToken(token string, id int64) (string, error) {
	return s.box.Seal([]byte(strings.TrimSpace(token)), WebhookSealAD(id))
}

func (s *Server) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	in, wh, ok := s.decodeWebhook(w, r)
	if !ok {
		return
	}
	var seal func(int64) (string, error)
	if strings.TrimSpace(in.Token) != "" {
		seal = func(id int64) (string, error) { return s.sealToken(in.Token, id) }
	}
	created, err := s.webhookStore.CreateWebhook(r.Context(), wh, seal)
	if err != nil {
		s.internalError(w, r, "Webhook anlegen", err)
		return
	}
	s.log.Info("Webhook angelegt", "webhook", created.Name)
	s.webhookChanged()
	writeJSON(w, http.StatusCreated, s.toWebhookDTO(created, i18n.FromRequest(r)))
}

func (s *Server) handleUpdateWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := webhookID(w, r)
	if !ok {
		return
	}
	in, wh, ok := s.decodeWebhook(w, r)
	if !ok {
		return
	}
	old, err := s.webhookStore.GetWebhook(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeMsg(w, r, http.StatusNotFound, i18n.M("webhook.not_found"))
		return
	}
	if err != nil {
		s.internalError(w, r, "Webhook lesen", err)
		return
	}
	wh.ID, wh.Token = id, old.Token
	// Neues Ziel: das gespeicherte Token nicht stillschweigend mitschicken –
	// sonst ließe es sich über eine geänderte URL abgreifen.
	if old.Token != "" && strings.TrimSpace(in.Token) == "" && !in.ClearToken &&
		integrations.URLOrigin(old.URL) != integrations.URLOrigin(wh.URL) {
		writeMsg(w, r, http.StatusBadRequest, i18n.M("webhook.token_reenter"))
		return
	}
	switch {
	case strings.TrimSpace(in.Token) != "":
		if wh.Token, err = s.sealToken(in.Token, id); err != nil {
			s.internalError(w, r, "Token verschlüsseln", err)
			return
		}
	case in.ClearToken:
		wh.Token = ""
	}
	updated, err := s.webhookStore.UpdateWebhook(r.Context(), wh)
	if errors.Is(err, store.ErrNotFound) {
		writeMsg(w, r, http.StatusNotFound, i18n.M("webhook.not_found"))
		return
	}
	if err != nil {
		s.internalError(w, r, "Webhook ändern", err)
		return
	}
	s.webhookChanged()
	writeJSON(w, http.StatusOK, s.toWebhookDTO(updated, i18n.FromRequest(r)))
}

func (s *Server) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := webhookID(w, r)
	if !ok {
		return
	}
	err := s.webhookStore.DeleteWebhook(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeMsg(w, r, http.StatusNotFound, i18n.M("webhook.not_found"))
		return
	}
	if err != nil {
		s.internalError(w, r, "Webhook löschen", err)
		return
	}
	s.webhookChanged()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTestWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := webhookID(w, r)
	if !ok {
		return
	}
	if s.webhooks == nil {
		writeMsg(w, r, http.StatusServiceUnavailable, i18n.M("webhook.unavailable"))
		return
	}
	// zu Ende senden, auch wenn der Browser die Verbindung schließt
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), time.Minute)
	defer cancel()
	res, err := s.webhooks.SendTest(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		writeMsg(w, r, http.StatusNotFound, i18n.M("webhook.not_found"))
		return
	}
	if err != nil {
		s.internalError(w, r, "Webhook testen", err)
		return
	}
	s.webhookChanged()
	writeJSON(w, http.StatusOK, map[string]any{"ok": res.OK, "error": i18n.T(i18n.FromRequest(r), res.Error)})
}

// handlePreviewWebhook rendert eine (auch ungespeicherte) Konfiguration mit
// einem Beispielereignis. Geheimnisse bleiben Platzhalter.
func (s *Server) handlePreviewWebhook(w http.ResponseWriter, r *http.Request) {
	var in struct {
		webhookInput
		EventType string `json:"event_type"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	var (
		req notify.Request
		err error
	)
	if s.webhooks != nil {
		req, err = s.webhooks.Preview(in.toWebhook(), in.EventType) // in der Benachrichtigungssprache
	} else {
		req, err = notify.Render(in.toWebhook(), notify.Localize(notify.SampleEvent(in.EventType), i18n.DE), nil, "")
	}
	if err != nil {
		writeMsg(w, r, http.StatusBadRequest, i18n.FromError(err))
		return
	}
	writeJSON(w, http.StatusOK, req)
}

// OpenWebhookToken entschlüsselt das Token eines Webhooks (für den Dispatcher).
func OpenWebhookToken(box *secrets.Box) func(w store.Webhook) (string, error) {
	return func(w store.Webhook) (string, error) {
		if box == nil {
			return "", secrets.ErrNoKey
		}
		b, err := box.Open(w.Token, WebhookSealAD(w.ID))
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
}
