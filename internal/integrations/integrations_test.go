package integrations

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
)

var testSchema = ConfigSchema{
	Fields: []Field{
		{Key: "base_url", Type: FieldURL, Label: Text{DE: "Adresse", EN: "Address"}, Required: true, Default: "https://example.com"},
		{Key: "api_key", Type: FieldSecret, Label: Text{EN: "API key"}, Required: true},
		{Key: "tag", Type: FieldString, Label: Text{EN: "Tag"}, Default: "backup"},
		{Key: "note", Type: FieldText, Label: Text{EN: "Note"}},
		{Key: "limit", Type: FieldNumber, Label: Text{EN: "Limit"}, Min: Ptr(1), Max: Ptr(10)},
		{Key: "flag", Type: FieldBool, Label: Text{EN: "Flag"}, Default: false},
		{Key: "mode", Type: FieldSelect, Label: Text{EN: "Mode"}, Required: true,
			Options: []Option{{Value: "a"}, {Value: "b"}}},
	},
}

func codes(msgs []i18n.Msg) string {
	var out []string
	for _, m := range msgs {
		out = append(out, m.Code)
	}
	return strings.Join(out, ",")
}

func TestNormalize(t *testing.T) {
	got, msgs := testSchema.Normalize(map[string]any{
		"base_url": " https://hc.example.com/ ", "limit": 3.0, "mode": "b", "note": " zwei\nZeilen ",
	}, i18n.DE)
	if len(msgs) != 0 {
		t.Fatalf("msgs: %v", msgs)
	}
	want := map[string]any{"base_url": "https://hc.example.com", "tag": "backup", "flag": false,
		"limit": 3.0, "mode": "b", "note": " zwei\nZeilen "}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %#v, erwartet %#v", k, got[k], v)
		}
	}
	if _, ok := got["api_key"]; ok {
		t.Error("Secret in Werten")
	}

	// Defaults füllen fehlende Felder; Pflichtfeld ohne Default fehlt
	got, msgs = testSchema.Normalize(map[string]any{}, i18n.DE)
	if codes(msgs) != "integration.field_required" || got["base_url"] != "https://example.com" {
		t.Fatalf("leer: %v %v", got, msgs)
	}
	if !strings.Contains(i18n.T(i18n.DE, msgs[0]), "Mode") {
		t.Fatalf("Feldname fehlt: %q", i18n.T(i18n.DE, msgs[0]))
	}

	for name, tc := range map[string]struct {
		in   map[string]any
		code string
	}{
		"unbekannt":     {map[string]any{"mode": "a", "x": 1.0}, "integration.field_unknown"},
		"secret direkt": {map[string]any{"mode": "a", "api_key": "x"}, "integration.field_unknown"},
		"Typ":           {map[string]any{"mode": "a", "limit": "3"}, "integration.field_type"},
		"Bereich":       {map[string]any{"mode": "a", "limit": 11.0}, "integration.field_range"},
		"Auswahl":       {map[string]any{"mode": "c"}, "integration.field_option"},
		"Schema":        {map[string]any{"mode": "a", "base_url": "ftp://x"}, "integration.field_url"},
		"ohne Host":     {map[string]any{"mode": "a", "base_url": "https://"}, "integration.field_url"},
		"Userinfo":      {map[string]any{"mode": "a", "base_url": "https://u:p@x"}, "integration.field_url_userinfo"},
		"Zeilenumbruch": {map[string]any{"mode": "a", "tag": "a\nb"}, "integration.field_newline"},
		"bool":          {map[string]any{"mode": "a", "flag": "ja"}, "integration.field_type"},
		"zu lang":       {map[string]any{"mode": "a", "tag": strings.Repeat("x", maxStringLen+1)}, "integration.field_too_long"},
	} {
		if _, msgs := testSchema.Normalize(tc.in, i18n.EN); codes(msgs) != tc.code {
			t.Errorf("%s: %q, erwartet %q", name, codes(msgs), tc.code)
		}
	}
}

func TestWorst(t *testing.T) {
	for _, tc := range []struct {
		in   []Status
		want Status
	}{
		{nil, StatusUnknown},
		{[]Status{StatusPaused}, StatusUnknown},
		{[]Status{StatusOK, StatusPaused}, StatusOK},
		{[]Status{StatusOK, StatusUnknown}, StatusUnknown},
		{[]Status{StatusWarning, StatusOK}, StatusWarning},
		{[]Status{StatusOK, StatusError, StatusWarning}, StatusError},
	} {
		if got := Worst(tc.in...); got != tc.want {
			t.Errorf("Worst(%v) = %s, erwartet %s", tc.in, got, tc.want)
		}
	}
}

func TestRegistry(t *testing.T) {
	r := NewRegistry(fakeIntegration{"b"}, fakeIntegration{"a"})
	if all := r.All(); len(all) != 2 || all[0].Type() != "a" {
		t.Fatalf("All: %v", all)
	}
	if _, ok := r.Get("a"); !ok {
		t.Fatal("Get")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("doppelter Typ ohne panic")
		}
	}()
	r.Register(fakeIntegration{"a"})
}

type fakeIntegration struct{ typ string }

func (f fakeIntegration) Type() string                                    { return f.typ }
func (fakeIntegration) Schema() ConfigSchema                              { return ConfigSchema{} }
func (fakeIntegration) Test(context.Context, Config) error                { return nil }
func (fakeIntegration) Collect(context.Context, Config) (Snapshot, error) { return Snapshot{}, nil }

func errCode(err error) string { return i18n.FromError(err).Code }

func TestGetJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			if r.Header.Get("X-Api-Key") != "k" {
				w.WriteHeader(401)
				return
			}
			w.Write([]byte(`{"n":1}`))
		case "/broken":
			w.Write([]byte(`{"n":`))
		case "/html":
			w.Write([]byte(`<html>`))
		case "/slow":
			time.Sleep(300 * time.Millisecond)
		case "/forbidden":
			w.WriteHeader(403)
		case "/error":
			w.WriteHeader(502)
		case "/limit":
			w.WriteHeader(429)
		case "/away":
			http.Redirect(w, r, "http://other.invalid/x", http.StatusFound)
		case "/same":
			http.Redirect(w, r, "/ok", http.StatusFound)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := NewHTTPClient(time.Second, true)
	key := http.Header{"X-Api-Key": {"k"}}
	ctx := context.Background()

	var out struct{ N int }
	if err := GetJSON(ctx, c, srv.URL+"/ok", key, &out); err != nil || out.N != 1 {
		t.Fatalf("ok: %v %v", out, err)
	}
	if err := GetJSON(ctx, c, srv.URL+"/same", key, &out); err != nil {
		t.Fatalf("Weiterleitung auf denselben Host: %v", err)
	}

	for path, code := range map[string]string{
		"/ok":        "integration.unauthorized", // ohne Key
		"/forbidden": "integration.unauthorized",
		"/missing":   "integration.not_found",
		"/error":     "integration.http",
		"/limit":     "integration.rate_limited",
		"/broken":    "integration.bad_json",
		"/html":      "integration.bad_json",
		"/away":      "integration.redirect_host",
	} {
		if err := GetJSON(ctx, c, srv.URL+path, nil, &out); errCode(err) != code {
			t.Errorf("%s: %q (%v), erwartet %s", path, errCode(err), err, code)
		}
	}

	slow := NewHTTPClient(100*time.Millisecond, true)
	if err := GetJSON(ctx, slow, srv.URL+"/slow", nil, &out); errCode(err) != "integration.timeout" {
		t.Errorf("Timeout: %v", err)
	}

	// Verbindung abgelehnt: freier Port
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	if err := GetJSON(ctx, c, "http://"+addr+"/", nil, &out); errCode(err) != "integration.refused" {
		t.Errorf("refused: %v", err)
	}
	if err := GetJSON(ctx, c, "http://gibtsnicht.invalid/", nil, &out); errCode(err) != "integration.dns" {
		t.Errorf("dns: %v", err)
	}
}

func TestGetJSONTLS(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	var out map[string]any

	// selbst signiert: mit Prüfung abgelehnt, ohne Prüfung erlaubt
	if err := GetJSON(context.Background(), NewHTTPClient(time.Second, true), srv.URL, nil, &out); errCode(err) != "integration.tls_untrusted" {
		t.Fatalf("Prüfung an: %v", err)
	}
	if err := GetJSON(context.Background(), NewHTTPClient(time.Second, false), srv.URL, nil, &out); err != nil {
		t.Fatalf("Prüfung aus: %v", err)
	}

	// https gegen einen http-Server
	plain := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer plain.Close()
	u := strings.Replace(plain.URL, "http://", "https://", 1)
	if err := GetJSON(context.Background(), NewHTTPClient(time.Second, true), u, nil, &out); errCode(err) != "integration.tls_not_https" {
		t.Fatalf("kein HTTPS: %v", err)
	}
}

func TestRequestErrorHidesURL(t *testing.T) {
	err := RequestError(&url.Error{Op: "Get", URL: "https://x/?token=geheim", Err: errors.New("boom")})
	if strings.Contains(err.Error(), "geheim") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Meldung: %v", err)
	}
}
