package i18n

import (
	"errors"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var placeholderRe = regexp.MustCompile(`\{[a-z_]+\}`)

// Jeder Eintrag hat beide Sprachen und dieselben Platzhalter.
func TestCatalogComplete(t *testing.T) {
	for code, e := range catalog {
		if e.de == "" || e.en == "" {
			t.Errorf("%s: Übersetzung fehlt", code)
		}
		de, en := placeholderRe.FindAllString(e.de, -1), placeholderRe.FindAllString(e.en, -1)
		slices.Sort(de)
		slices.Sort(en)
		if strings.Join(de, ",") != strings.Join(en, ",") {
			t.Errorf("%s: Platzhalter unterschiedlich: %v vs. %v", code, de, en)
		}
	}
}

// Testeinträge, damit die Tests unabhängig vom Inhalt des Katalogs sind.
func init() {
	catalog["test.state.on"] = entry{"an", "on"}
	catalog["test.state.off"] = entry{"aus", "off"}
	catalog["test.toggle"] = entry{"Schalter {name}: {from} → {to}", "Switch {name}: {from} → {to}"}
	catalog["test.failed"] = entry{"fehlgeschlagen: {detail}", "failed: {detail}"}
	catalog["test.http"] = entry{"HTTP {status}", "HTTP {status}"}
	catalog["test.env_missing"] = entry{"Env-Variable {name} ist nicht gesetzt", "environment variable {name} is not set"}
}

func TestRender(t *testing.T) {
	m := M("test.toggle", "name", "A", "from", Ref("test.state.off"), "to", Ref("test.state.on"))
	if got := T(DE, m); got != "Schalter A: aus → an" {
		t.Fatalf("de: %q", got)
	}
	if got := T(EN, m); got != "Switch A: off → on" {
		t.Fatalf("en: %q", got)
	}

	nested := M("test.failed", "detail", Nest(M("test.http", "status", "500")))
	if got := T(DE, nested); got != "fehlgeschlagen: HTTP 500" {
		t.Fatalf("nested: %q", got)
	}

	j := JoinMsgs(M("test.state.on"), M("test.http", "status", "401"))
	if got := T(EN, j); got != "on; HTTP 401" {
		t.Fatalf("join: %q", got)
	}
	if !Equal(Decode(Encode(j)), j) || !Decode("").IsZero() || Encode(Msg{}) != "" {
		t.Fatal("Encode/Decode")
	}
	if JoinMsgs(M("api.internal")).Code != "api.internal" || !JoinMsgs().IsZero() {
		t.Fatal("JoinMsgs mit 0/1 Meldung")
	}
	if T(EN, M("gibtsnicht")) != "gibtsnicht" {
		t.Fatal("unbekannter Code")
	}
}

type locErr struct{}

func (locErr) Error() string     { return "api" }
func (locErr) LocalizedMsg() Msg { return M("test.http", "status", "403") }

func TestErrors(t *testing.T) {
	err := Wrap(E("test.env_missing", "name", "SECRET_X"), "test.failed")
	if err.Error() != "failed: environment variable SECRET_X is not set" {
		t.Fatalf("Error(): %q", err.Error())
	}
	if got := T(DE, FromError(err)); got != "fehlgeschlagen: Env-Variable SECRET_X ist nicht gesetzt" {
		t.Fatalf("de: %q", got)
	}
	if got := T(DE, FromError(errors.Join(locErr{}))); got != "HTTP 403" {
		t.Fatalf("Localizer: %q", got)
	}
	if got := T(DE, FromError(errors.New("dial tcp: timeout"))); got != "dial tcp: timeout" {
		t.Fatalf("roh: %q", got)
	}
}

func TestFromRequest(t *testing.T) {
	for header, want := range map[string]Lang{"de-DE,de;q=0.9": DE, "en-US": EN, "": EN, "fr": EN, "DE": DE} {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Accept-Language", header)
		if got := FromRequest(r); got != want {
			t.Errorf("%q → %s", header, got)
		}
	}
}
