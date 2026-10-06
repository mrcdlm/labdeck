package secrets

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func testBox(t *testing.T, b byte) *Box {
	t.Helper()
	box, err := New(bytes.Repeat([]byte{b}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return box
}

func TestSealOpen(t *testing.T) {
	box := testBox(t, 1)
	sealed, err := box.Seal([]byte("geheim-123"), "integration:1")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sealed, "geheim") || !strings.HasPrefix(sealed, "v1:") {
		t.Fatalf("sealed = %q", sealed)
	}
	// zufällige Nonce: gleicher Klartext ergibt anderen Wert
	again, _ := box.Seal([]byte("geheim-123"), "integration:1")
	if again == sealed {
		t.Fatal("Nonce wird wiederverwendet")
	}

	plain, err := box.Open(sealed, "integration:1")
	if err != nil || string(plain) != "geheim-123" {
		t.Fatalf("Open: %q %v", plain, err)
	}

	for name, fn := range map[string]func() ([]byte, error){
		"anderer Zweck":     func() ([]byte, error) { return box.Open(sealed, "integration:2") },
		"anderer Schlüssel": func() ([]byte, error) { return testBox(t, 2).Open(sealed, "integration:1") },
		"verändert":         func() ([]byte, error) { return box.Open(sealed[:len(sealed)-4]+"AAA=", "integration:1") },
		"kein Präfix":       func() ([]byte, error) { return box.Open(strings.TrimPrefix(sealed, "v1:"), "integration:1") },
		"kaputt":            func() ([]byte, error) { return box.Open("v1:%%%", "integration:1") },
	} {
		if _, err := fn(); !errors.Is(err, ErrDecrypt) {
			t.Errorf("%s: ErrDecrypt erwartet, bekam %v", name, err)
		}
	}
}

func TestNilBox(t *testing.T) {
	box, err := New(nil)
	if err != nil || box != nil {
		t.Fatalf("New(nil) = %v, %v", box, err)
	}
	if _, err := box.Seal([]byte("x"), ""); !errors.Is(err, ErrNoKey) {
		t.Fatalf("Seal: %v", err)
	}
	if _, err := box.Open("v1:x", ""); !errors.Is(err, ErrNoKey) {
		t.Fatalf("Open: %v", err)
	}
	if _, err := New([]byte("kurz")); err == nil {
		t.Fatal("kurzer Schlüssel akzeptiert")
	}
}

func TestExpand(t *testing.T) {
	env := func(name string) (string, bool) {
		v, ok := map[string]string{"SECRET_TOKEN": "abc", "SECRET_EMPTY": "", "APP_PASSWORD": "pw"}[name]
		return v, ok
	}
	got, err := Expand("Bearer ${SECRET_TOKEN}", env)
	if err != nil || got != "Bearer abc" {
		t.Fatalf("Expand: %q %v", got, err)
	}
	if got, err := Expand("${SECRET_EMPTY}", env); err != nil || got != "" {
		t.Fatalf("leer: %q %v", got, err)
	}
	if got, _ := Expand("ohne Verweis", env); got != "ohne Verweis" {
		t.Fatalf("ohne: %q", got)
	}

	// fremde Variablen sind nie auflösbar, auch wenn sie existieren
	for _, s := range []string{"${APP_PASSWORD}", "${LABDECK_SECRET_KEY}", "${SECRET_}", "${secret_x}", "${}"} {
		_, err := Expand(s, env)
		if err == nil {
			t.Errorf("%s: Fehler erwartet", s)
			continue
		}
		if strings.Contains(err.Error(), "pw") && s == "${APP_PASSWORD}" {
			t.Errorf("Wert in Fehlermeldung: %v", err)
		}
	}
	_, err = Expand("${SECRET_FEHLT}", env)
	if err == nil || !strings.Contains(err.Error(), "SECRET_FEHLT") {
		t.Fatalf("fehlend: %v", err)
	}
}

func TestIsRef(t *testing.T) {
	for s, want := range map[string]bool{
		"${SECRET_X}": true, " ${SECRET_X}": false, "a${SECRET_X}": false,
		"${SECRET_X}${SECRET_Y}": false, "geheim": false, "": false,
	} {
		if IsRef(s) != want {
			t.Errorf("IsRef(%q) != %v", s, want)
		}
	}
}
