// Package api stellt die HTTP-Schnittstelle und das eingebettete Frontend bereit.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/notify"
	"github.com/mrcdlm/labdeck/internal/secrets"
)

type dataStore interface {
	sessionStore
	Ping(ctx context.Context) error
}

// Deps sind die Abhängigkeiten des HTTP-Servers.
type Deps struct {
	Store  dataStore
	Events eventSource // nil = keine Live-Updates
	// Settings: Laufzeit-Einstellungen (nil = nicht änderbar)
	Settings settingsService
	// Integrations: Datenquellen verwalten und abfragen
	Integrations integrationService
	// Backups: Einstellungen je Backup (Höchstalter)
	Backups backupStore
	// Metrics: Verlauf der Kennzahlen
	Metrics metricStore
	// Live: Kennzahlen der letzten Stunde in Abfrageauflösung (nil = nur DB)
	Live livePoints
	// History: Statuswechsel aller Integrationen (Verlauf-Seite)
	History historyStore
	// Prefs: weitere gespeicherte Einstellungen (Dashboard-Anordnung)
	Prefs prefsStore
	// Webhooks: Speicherung, Test und Vorschau (nil = keine Benachrichtigungen)
	WebhookStore webhookStore
	Webhooks     webhookTester
	WebhookEnv   notify.Env
	// Box verschlüsselt Webhook-Tokens (nil = kein LABDECK_SECRET_KEY)
	Box *secrets.Box
	// TrustedProxies: Proxys, deren Client-IP-Header gelten (leer = keine)
	TrustedProxies []netip.Prefix
	Info           Info
	Auth           *Auth
	Log            *slog.Logger
	Static         fs.FS
}

type Server struct {
	store          dataStore
	events         eventSource
	settings       settingsService
	integrations   integrationService
	backups        backupStore
	metrics        metricStore
	live           livePoints
	dashMu         sync.Mutex // Dashboards lesen–ändern–speichern
	history        historyStore
	prefs          prefsStore
	webhookStore   webhookStore
	webhooks       webhookTester
	webhookEnv     notify.Env
	box            *secrets.Box
	trustedProxies []netip.Prefix
	info           Info
	auth           *Auth
	log            *slog.Logger
	static         fs.FS
}

func NewServer(d Deps) *Server {
	return &Server{store: d.Store, events: d.Events, settings: d.Settings, integrations: d.Integrations, backups: d.Backups, metrics: d.Metrics, live: d.Live,
		history: d.History, prefs: d.Prefs, webhookStore: d.WebhookStore, webhooks: d.Webhooks, webhookEnv: d.WebhookEnv,
		box: d.Box, trustedProxies: d.TrustedProxies,
		info: d.Info, auth: d.Auth, log: d.Log, static: d.Static}
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)

	r.Get("/healthz", s.handleHealth)

	r.Route("/api", func(r chi.Router) {
		r.Use(noStore)
		// CSRF: schreibende Anfragen nur von derselben Origin. SameSite=Strict
		// allein schützt nicht vor Seiten auf Nachbar-Subdomains (gleiche Site).
		r.Use(http.NewCrossOriginProtection().Handler)
		r.Post("/login", s.handleLogin)
		r.Post("/logout", s.handleLogout)
		r.Get("/session", s.handleSession)

		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Get("/events", s.handleEvents)
			r.Get("/settings", s.handleGetSettings)
			r.Put("/settings", s.handlePutSettings)
			r.Get("/info", s.handleInfo)

			r.Get("/integration-types", s.handleIntegrationTypes)
			r.Get("/integrations", s.handleListIntegrations)
			r.Post("/integrations", s.handleCreateIntegration)
			r.Post("/integrations/test", s.handleTestIntegration)
			r.Get("/integrations/{id}", s.handleGetIntegration)
			r.Put("/integrations/{id}", s.handleUpdateIntegration)
			r.Delete("/integrations/{id}", s.handleDeleteIntegration)
			r.Post("/integrations/{id}/poll", s.handlePollIntegration)
			r.Get("/integrations/{id}/history", s.handleMetricHistory)

			r.Get("/overview", s.handleOverview)
			r.Get("/checks", s.handleChecks)
			r.Get("/backups", s.handleBackups)
			r.Put("/backups/max-age", s.handleSetBackupMaxAge)
			r.Get("/history", s.handleHistory)
			r.Get("/dashboard", s.handleGetDashboard)
			r.Put("/dashboard", s.handlePutDashboard)
			r.Get("/dashboards", s.handleListDashboards)
			r.Post("/dashboards", s.handleCreateDashboard)
			r.Put("/dashboards/order", s.handleOrderDashboards)
			r.Put("/dashboards/{id}", s.handlePutDashboardByID)
			r.Delete("/dashboards/{id}", s.handleDeleteDashboard)

			r.Get("/webhooks", s.handleListWebhooks)
			r.Post("/webhooks", s.handleCreateWebhook)
			r.Post("/webhooks/preview", s.handlePreviewWebhook)
			r.Put("/webhooks/{id}", s.handleUpdateWebhook)
			r.Delete("/webhooks/{id}", s.handleDeleteWebhook)
			r.Post("/webhooks/{id}/test", s.handleTestWebhook)
		})

		r.NotFound(func(w http.ResponseWriter, req *http.Request) {
			writeMsg(w, req, http.StatusNotFound, i18n.M("api.not_found"))
		})
	})

	r.NotFound(spaHandler(s.static).ServeHTTP)
	return r
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		s.log.Error("healthz: DB nicht erreichbar", "err", err)
		http.Error(w, "db unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("ok"))
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	client := s.clientIP(r)
	if !s.auth.beginAttempt(client) {
		writeMsg(w, r, http.StatusTooManyRequests, i18n.M("auth.too_many_attempts"))
		return
	}

	var body struct {
		Password string `json:"password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeMsg(w, r, http.StatusBadRequest, i18n.M("api.bad_request"))
		return
	}

	if !s.auth.checkPassword(body.Password) {
		s.log.Warn("Login fehlgeschlagen", "client", client)
		select {
		case <-time.After(s.auth.delay):
		case <-r.Context().Done():
		}
		writeMsg(w, r, http.StatusUnauthorized, i18n.M("auth.wrong_password"))
		return
	}

	s.auth.loginSucceeded(client)
	token, exp, err := s.auth.newSession(r.Context())
	if err != nil {
		s.log.Error("Session anlegen fehlgeschlagen", "err", err)
		writeMsg(w, r, http.StatusInternalServerError, i18n.M("api.internal"))
		return
	}
	http.SetCookie(w, sessionCookieFor(r, token, exp))
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		if err := s.store.DeleteSession(r.Context(), s.auth.hashToken(c.Value)); err != nil {
			s.log.Error("Session löschen fehlgeschlagen", "err", err)
		}
	}
	http.SetCookie(w, sessionCookieFor(r, "", time.Time{}))
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	ok, err := s.auth.valid(r)
	if errors.Is(err, context.Canceled) {
		return // Browser hat die Anfrage abgebrochen
	}
	if err != nil {
		s.log.Error("Session prüfen fehlgeschlagen", "err", err)
		writeMsg(w, r, http.StatusInternalServerError, i18n.M("api.internal"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": ok})
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, err := s.auth.valid(r)
		if errors.Is(err, context.Canceled) {
			return // Browser hat die Anfrage abgebrochen (z. B. Seitenwechsel)
		}
		if err != nil {
			s.log.Error("Session prüfen fehlgeschlagen", "err", err)
			writeMsg(w, r, http.StatusInternalServerError, i18n.M("api.internal"))
			return
		}
		if !ok {
			writeMsg(w, r, http.StatusUnauthorized, i18n.M("api.unauthorized"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeMsg schreibt eine Fehlerantwort in der Sprache der Anfrage. Neben dem
// übersetzten Text stehen Code und Parameter (für Clients und Tests).
func writeMsg(w http.ResponseWriter, r *http.Request, status int, m i18n.Msg) {
	writeJSON(w, status, map[string]any{"error": i18n.T(i18n.FromRequest(r), m), "code": m.Code, "params": m.Params})
}

// writeMsgs schreibt mehrere Meldungen (z. B. Validierungsfehler) als eine Antwort.
func writeMsgs(w http.ResponseWriter, r *http.Request, status int, msgs []i18n.Msg) {
	writeMsg(w, r, status, i18n.JoinMsgs(msgs...))
}

// decodeJSON liest den Body streng (unbekannte Felder sind ein Fehler).
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	return decodeJSONMax(w, r, v, 16<<10)
}

// decodeJSONMax wie decodeJSON, mit eigener Größengrenze (z. B. Dashboards).
func decodeJSONMax(w http.ResponseWriter, r *http.Request, v any, maxBytes int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeMsg(w, r, http.StatusBadRequest, i18n.M("api.bad_request"))
		return false
	}
	return true
}

func (s *Server) internalError(w http.ResponseWriter, r *http.Request, what string, err error) {
	s.log.Error(what+" fehlgeschlagen", "err", err)
	writeMsg(w, r, http.StatusInternalServerError, i18n.M("api.internal"))
}

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; "+
				"connect-src 'self'; manifest-src 'self'; worker-src 'self'; "+
				"frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}
