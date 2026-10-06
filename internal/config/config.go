// Package config lädt die Laufzeitkonfiguration aus Umgebungsvariablen.
// Secrets (APP_PASSWORD, LABDECK_SECRET_KEY) werden nur hier gelesen und
// dürfen niemals geloggt oder über die API ausgeliefert werden.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

// SecretKeyLen ist die Länge des Schlüssels für AES-256-GCM.
const SecretKeyLen = 32

type Config struct {
	Port        int
	LogLevel    slog.Level
	DataDir     string
	AppPassword string
	// SecretKey verschlüsselt die Zugangsdaten der Integrationen
	// (LABDECK_SECRET_KEY, 32 Byte Base64); nil = nicht gesetzt.
	SecretKey []byte
	// TrustedProxies: Adressen/Netze von Reverse Proxys, deren Angaben zur
	// Client-IP (CF-Connecting-IP, X-Forwarded-For) übernommen werden.
	TrustedProxies []netip.Prefix
}

// Load liest die Konfiguration über getenv (in Tests austauschbar).
func Load(getenv func(string) string) (*Config, error) {
	c := &Config{
		Port:        8080,
		LogLevel:    slog.LevelInfo,
		DataDir:     "/data",
		AppPassword: getenv("APP_PASSWORD"),
	}

	if v := getenv("PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p < 1 || p > 65535 {
			return nil, fmt.Errorf("invalid PORT: %q", v)
		}
		c.Port = p
	}
	if v := getenv("LOG_LEVEL"); v != "" {
		if err := c.LogLevel.UnmarshalText([]byte(strings.ToLower(v))); err != nil {
			return nil, fmt.Errorf("invalid LOG_LEVEL: %q", v)
		}
	}
	if v := getenv("DATA_DIR"); v != "" {
		c.DataDir = v
	}
	var err error
	if c.SecretKey, err = ParseSecretKey(getenv("LABDECK_SECRET_KEY")); err != nil {
		return nil, err
	}
	if c.TrustedProxies, err = ParseTrustedProxies(getenv("TRUSTED_PROXIES")); err != nil {
		return nil, err
	}
	if c.AppPassword == "" {
		return nil, errors.New("APP_PASSWORD must be set")
	}
	return c, nil
}

// FromEnv ist Load mit os.Getenv.
func FromEnv() (*Config, error) { return Load(os.Getenv) }

// PortFromEnv liefert nur den Port (für den healthcheck-Unterbefehl,
// der ohne APP_PASSWORD auskommen muss).
func PortFromEnv() int {
	if p, err := strconv.Atoi(os.Getenv("PORT")); err == nil && p > 0 {
		return p
	}
	return 8080
}

// LogValue sorgt dafür, dass beim Loggen der Config keine Secrets erscheinen.
func (c *Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("port", c.Port),
		slog.String("log_level", c.LogLevel.String()),
		slog.String("data_dir", c.DataDir),
		slog.Bool("secret_key_set", c.SecretKey != nil),
		slog.Int("trusted_proxies", len(c.TrustedProxies)),
	)
}

// ParseSecretKey liest LABDECK_SECRET_KEY: 32 Byte, Base64 (z. B. aus
// `openssl rand -base64 32`). Leer = nicht gesetzt. Die Fehlermeldung nennt
// den Wert nicht.
func ParseSecretKey(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			if len(b) != SecretKeyLen {
				return nil, fmt.Errorf("invalid LABDECK_SECRET_KEY: must be %d bytes, got %d "+
					"(create one with 'openssl rand -base64 32')", SecretKeyLen, len(b))
			}
			return b, nil
		}
	}
	return nil, errors.New("invalid LABDECK_SECRET_KEY: not valid Base64 " +
		"(create one with 'openssl rand -base64 32')")
}

// ParseTrustedProxies liest TRUSTED_PROXIES: kommagetrennte IP-Adressen oder
// Netze (CIDR), z. B. "172.16.0.0/12,127.0.0.1". Leer = keinem Proxy vertrauen.
func ParseTrustedProxies(s string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for part := range strings.SplitSeq(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if p, err := netip.ParsePrefix(part); err == nil {
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(part)
		if err != nil {
			return nil, fmt.Errorf("invalid TRUSTED_PROXIES entry %q: expected IP address or CIDR", part)
		}
		out = append(out, netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen()))
	}
	return out, nil
}
