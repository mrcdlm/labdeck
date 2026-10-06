package integrations

import (
	"net/http"
	"time"
)

// Config ist die aufbereitete Konfiguration einer Instanz, wie sie Test und
// Collect erhalten: geprüfte Werte, entschlüsselte Secrets und ein
// HTTP-Client mit Timeout und TLS-Einstellung der Instanz.
type Config struct {
	Values  map[string]any
	Secrets map[string]string
	// HTTP ist für alle Anfragen der Integration zu verwenden.
	HTTP    *http.Client
	Timeout time.Duration
}

func (c Config) String(key string) string {
	s, _ := c.Values[key].(string)
	return s
}

func (c Config) Bool(key string) bool {
	b, _ := c.Values[key].(bool)
	return b
}

func (c Config) Float(key string) float64 {
	f, _ := c.Values[key].(float64)
	return f
}

func (c Config) Int(key string) int { return int(c.Float(key)) }

// Secret liefert den entschlüsselten (und aufgelösten) Wert eines Secret-Felds.
func (c Config) Secret(key string) string { return c.Secrets[key] }
