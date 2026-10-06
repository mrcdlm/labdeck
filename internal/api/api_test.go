package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/mrcdlm/labdeck/internal/config"
	"github.com/mrcdlm/labdeck/internal/events"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/integrations/healthchecks"
	"github.com/mrcdlm/labdeck/internal/integrations/healthchecks/hctest"
	"github.com/mrcdlm/labdeck/internal/secrets"
	"github.com/mrcdlm/labdeck/internal/store"
)

type testEnv struct {
	srv      *httptest.Server
	api      *Server
	auth     *Auth
	broker   *events.Broker
	hc       *hctest.Fake
	hcURL    string
	language string // Accept-Language der Anfragen (Standard "de")
}

func (e *testEnv) lang() string {
	if e.language == "" {
		return "de"
	}
	return e.language
}

func newTestEnv(t *testing.T, static fstest.MapFS) *testEnv {
	t.Helper()
	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	auth, err := NewAuth("richtig", st)
	if err != nil {
		t.Fatal(err)
	}
	auth.delay = 0
	if static == nil {
		static = fstest.MapFS{}
	}
	log := slog.New(slog.DiscardHandler)
	broker := events.NewBroker()
	t.Cleanup(broker.Close)
	settings, _ := config.NewSettingsService(context.Background(), st)

	hc := hctest.New("ro-key", "", hctest.Demo()...)
	hcSrv := httptest.NewServer(hc)
	t.Cleanup(hcSrv.Close)
	box, _ := secrets.New(bytes.Repeat([]byte{1}, 32))
	mgr := integrations.NewManager(integrations.NewRegistry(healthchecks.New()), st, box,
		func(string) (string, bool) { return "", false }, log)
	mgr.Pub = broker

	s := NewServer(Deps{Store: st, Events: broker, Settings: settings, Integrations: mgr,
		Info: Info{Version: "v1.2.3", SecretKeySet: true}, Auth: auth, Log: log, Static: static})
	srv := httptest.NewServer(s.Routes())
	t.Cleanup(srv.Close)
	return &testEnv{srv: srv, api: s, auth: auth, broker: broker, hc: hc, hcURL: hcSrv.URL}
}

func (e *testEnv) do(t *testing.T, method, path, body string, cookie *http.Cookie) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	req.Header.Set("Accept-Language", e.lang())
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func (e *testEnv) login(t *testing.T) *http.Cookie {
	t.Helper()
	resp := e.do(t, "POST", "/api/login", `{"password":"richtig"}`, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("login: %d", resp.StatusCode)
	}
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookie {
			return c
		}
	}
	t.Fatal("kein Session-Cookie")
	return nil
}

func TestHealthz(t *testing.T) {
	e := newTestEnv(t, nil)
	resp := e.do(t, "GET", "/healthz", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestAuthRequired(t *testing.T) {
	e := newTestEnv(t, nil)
	for _, p := range []struct{ m, path string }{
		{"GET", "/api/info"}, {"GET", "/api/settings"}, {"PUT", "/api/settings"}, {"GET", "/api/events"},
	} {
		if resp := e.do(t, p.m, p.path, "", nil); resp.StatusCode != 401 {
			t.Errorf("%s %s: %d", p.m, p.path, resp.StatusCode)
		}
	}
	bogus := &http.Cookie{Name: sessionCookie, Value: "erfunden"}
	if resp := e.do(t, "GET", "/api/info", "", bogus); resp.StatusCode != 401 {
		t.Errorf("erfundenes Token akzeptiert: %d", resp.StatusCode)
	}
}

func TestLoginFlow(t *testing.T) {
	e := newTestEnv(t, nil)

	resp := e.do(t, "POST", "/api/login", `{"password":"falsch"}`, nil)
	if resp.StatusCode != 401 {
		t.Fatalf("falsches Passwort: %d", resp.StatusCode)
	}
	var msg map[string]any
	json.NewDecoder(resp.Body).Decode(&msg)
	if msg["code"] != "auth.wrong_password" || msg["error"] != "Passwort falsch" {
		t.Fatalf("Fehlermeldung: %v", msg)
	}

	c := e.login(t)
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" {
		t.Fatalf("Cookie-Attribute: %+v", c)
	}

	resp = e.do(t, "GET", "/api/info", "", c)
	var info Info
	json.NewDecoder(resp.Body).Decode(&info)
	if resp.StatusCode != 200 || info.Version != "v1.2.3" || !info.SecretKeySet {
		t.Fatalf("/api/info: %d %+v", resp.StatusCode, info)
	}

	resp = e.do(t, "GET", "/api/session", "", c)
	var sess map[string]bool
	json.NewDecoder(resp.Body).Decode(&sess)
	if !sess["authenticated"] {
		t.Fatal("session nicht authentifiziert")
	}

	e.do(t, "POST", "/api/logout", "", c)
	if resp := e.do(t, "GET", "/api/info", "", c); resp.StatusCode != 401 {
		t.Fatalf("nach Logout: %d", resp.StatusCode)
	}
}

func TestLoginRateLimit(t *testing.T) {
	e := newTestEnv(t, nil)
	for range maxFailures {
		e.do(t, "POST", "/api/login", `{"password":"falsch"}`, nil)
	}
	if resp := e.do(t, "POST", "/api/login", `{"password":"richtig"}`, nil); resp.StatusCode != 429 {
		t.Fatalf("429 erwartet, bekam %d", resp.StatusCode)
	}
	// nach Ablauf des Fensters wieder erlaubt
	e.auth.now = func() time.Time { return time.Now().Add(2 * failureWindow) }
	if resp := e.do(t, "POST", "/api/login", `{"password":"richtig"}`, nil); resp.StatusCode != 200 {
		t.Fatalf("nach Fenster: %d", resp.StatusCode)
	}
}

func TestNewAuthAcceptsBcryptHash(t *testing.T) {
	h, _ := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	a, err := NewAuth(string(h), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !a.checkPassword("pw") || a.checkPassword(string(h)) {
		t.Fatal("Hash-Modus funktioniert nicht")
	}
}

func TestSecureCookieBehindProxy(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/login", nil)
	if sessionCookieFor(r, "x", time.Now()).Secure {
		t.Fatal("Secure ohne HTTPS")
	}
	r.Header.Set("X-Forwarded-Proto", "https")
	if !sessionCookieFor(r, "x", time.Now()).Secure {
		t.Fatal("Secure hinter HTTPS-Proxy fehlt")
	}
}

func TestSettingsAPI(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)

	resp := e.do(t, "GET", "/api/settings", "", c)
	var got settingsDTO
	json.NewDecoder(resp.Body).Decode(&got)
	if got.RawRetentionDays != config.DefaultRawRetentionDays || got.Timezone != "" ||
		got.Limits["raw_retention_days_max"] != config.MaxRawRetentionDays {
		t.Fatalf("GET: %+v", got)
	}

	// Rückgabe von GET inkl. limits lässt sich unverändert zurückschicken
	got.Timezone = "Europe/Berlin"
	got.DailyRetentionDays = 730
	body, _ := json.Marshal(got)
	rd := openStream(t, e, c)
	resp = e.do(t, "PUT", "/api/settings", string(body), c)
	if resp.StatusCode != 200 {
		t.Fatalf("PUT: %d", resp.StatusCode)
	}
	if ev, data := readEvent(t, rd); ev != "change" || data != events.TopicSettings {
		t.Fatalf("event=%q data=%q", ev, data)
	}
	json.NewDecoder(e.do(t, "GET", "/api/settings", "", c).Body).Decode(&got)
	if got.Timezone != "Europe/Berlin" || got.DailyRetentionDays != 730 {
		t.Fatalf("nach PUT: %+v", got)
	}

	e.language = "en"
	resp = e.do(t, "PUT", "/api/settings", `{"timezone":"Mars/Olympus","raw_retention_days":0,"daily_retention_days":365}`, c)
	var msg map[string]any
	json.NewDecoder(resp.Body).Decode(&msg)
	if resp.StatusCode != 400 || !strings.Contains(msg["error"].(string), "unknown time zone") ||
		!strings.Contains(msg["error"].(string), "Raw data retention") {
		t.Fatalf("Validierung: %d %v", resp.StatusCode, msg)
	}

	if resp := e.do(t, "PUT", "/api/settings", `{"unbekannt":1}`, c); resp.StatusCode != 400 {
		t.Fatalf("unbekanntes Feld: %d", resp.StatusCode)
	}
}

func TestSPA(t *testing.T) {
	e := newTestEnv(t, fstest.MapFS{
		"index.html":    {Data: []byte("<html>app</html>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	})

	resp := e.do(t, "GET", "/assets/app.js", "", nil)
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
		t.Fatalf("asset: %d %q", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}

	resp = e.do(t, "GET", "/verlauf", "", nil)
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	if resp.StatusCode != 200 || buf.String() != "<html>app</html>" {
		t.Fatalf("SPA-Fallback: %d %q", resp.StatusCode, buf.String())
	}

	if resp := e.do(t, "GET", "/api/gibtsnicht", "", nil); resp.StatusCode != 404 {
		t.Fatalf("API-404 erwartet: %d", resp.StatusCode)
	}
}

func TestPWAFiles(t *testing.T) {
	e := newTestEnv(t, fstest.MapFS{
		"index.html":           {Data: []byte("<html>app</html>")},
		"sw.js":                {Data: []byte("self.addEventListener('fetch', () => {})")},
		"manifest.webmanifest": {Data: []byte(`{"name":"labdeck"}`)},
	})

	resp := e.do(t, "GET", "/sw.js", "", nil)
	if resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != "no-cache" {
		t.Fatalf("sw.js: %d %q", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "worker-src 'self'") ||
		!strings.Contains(csp, "manifest-src 'self'") {
		t.Fatalf("CSP: %q", csp)
	}

	resp = e.do(t, "GET", "/manifest.webmanifest", "", nil)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/manifest+json" ||
		resp.Header.Get("Cache-Control") != "no-cache" {
		t.Fatalf("Manifest: %d %q %q", resp.StatusCode, resp.Header.Get("Content-Type"), resp.Header.Get("Cache-Control"))
	}
}

func TestNotBuiltPage(t *testing.T) {
	e := newTestEnv(t, nil)
	resp := e.do(t, "GET", "/", "", nil)
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(buf.String(), "nicht gebaut") {
		t.Fatalf("%d %q", resp.StatusCode, buf.String())
	}
}
