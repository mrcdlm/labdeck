package config

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// Testschlüssel: 32 Byte 0x01 – kein echtes Geheimnis.
const testKey = "AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE="

func TestLoadDefaults(t *testing.T) {
	c, err := Load(env(map[string]string{"APP_PASSWORD": "x"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != 8080 || c.DataDir != "/data" || c.LogLevel != slog.LevelInfo || c.SecretKey != nil {
		t.Fatalf("unerwartete Defaults: %+v", c)
	}
}

func TestLoadRequiresPassword(t *testing.T) {
	if _, err := Load(env(nil)); err == nil {
		t.Fatal("Fehler erwartet ohne APP_PASSWORD")
	}
}

func TestLoadInvalid(t *testing.T) {
	for _, m := range []map[string]string{
		{"APP_PASSWORD": "x", "PORT": "abc"},
		{"APP_PASSWORD": "x", "PORT": "70000"},
		{"APP_PASSWORD": "x", "LOG_LEVEL": "laut"},
		{"APP_PASSWORD": "x", "LABDECK_SECRET_KEY": "kein base64!"},
		{"APP_PASSWORD": "x", "LABDECK_SECRET_KEY": "AQID"}, // 3 Byte
	} {
		if _, err := Load(env(m)); err == nil {
			t.Errorf("Fehler erwartet für %v", m)
		}
	}
}

func TestSecretKey(t *testing.T) {
	c, err := Load(env(map[string]string{"APP_PASSWORD": "x", "LABDECK_SECRET_KEY": " " + testKey + "\n"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.SecretKey) != SecretKeyLen || c.SecretKey[0] != 1 {
		t.Fatalf("Schlüssel: %v", c.SecretKey)
	}
	// ohne Padding / URL-Alphabet ebenfalls erlaubt
	if k, err := ParseSecretKey(strings.TrimRight(testKey, "=")); err != nil || len(k) != SecretKeyLen {
		t.Fatalf("ohne Padding: %v %v", k, err)
	}
	// Fehlermeldung enthält den Wert nicht
	if _, err := ParseSecretKey("geheim-aber-falsch"); err == nil || strings.Contains(err.Error(), "geheim") {
		t.Fatalf("Fehler: %v", err)
	}
}

func TestLogValueHidesSecrets(t *testing.T) {
	c, _ := Load(env(map[string]string{
		"APP_PASSWORD": "geheim-pw", "LABDECK_SECRET_KEY": testKey, "LOG_LEVEL": "DEBUG",
	}))
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("cfg", "config", c)
	out := buf.String()
	if strings.Contains(out, "geheim") || strings.Contains(out, testKey) {
		t.Fatalf("Secret im Log: %s", out)
	}
	if !strings.Contains(out, "secret_key_set=true") {
		t.Fatalf("Flag fehlt: %s", out)
	}
}

func TestParseTrustedProxies(t *testing.T) {
	got, err := ParseTrustedProxies(" 172.16.0.0/12, 127.0.0.1,::1 ,10.1.2.3/8")
	want := []string{"172.16.0.0/12", "127.0.0.1/32", "::1/128", "10.0.0.0/8"}
	if err != nil || len(got) != len(want) {
		t.Fatalf("%v %v", got, err)
	}
	for i, p := range got {
		if p.String() != want[i] {
			t.Errorf("%d: %s, erwartet %s", i, p, want[i])
		}
	}
	if p, err := ParseTrustedProxies(""); err != nil || p != nil {
		t.Errorf("leer: %v %v", p, err)
	}
	if _, err := ParseTrustedProxies("cloudflared"); err == nil {
		t.Error("Hostname: Fehler erwartet")
	}
}
