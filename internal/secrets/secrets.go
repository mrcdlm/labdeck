// Package secrets verschlüsselt Zugangsdaten der Integrationen (AES-256-GCM
// mit LABDECK_SECRET_KEY) und löst Verweise auf Env-Variablen (${SECRET_…}) auf.
//
// Klartext-Geheimnisse dürfen nie in Logs, Fehlermeldungen oder API-Antworten
// erscheinen; Fehler dieses Pakets nennen daher nur Namen, nie Werte.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/mrcdlm/labdeck/internal/i18n"
)

// version kennzeichnet das Format verschlüsselter Werte (für spätere Wechsel).
const version = "v1:"

// ErrNoKey: LABDECK_SECRET_KEY ist nicht gesetzt.
var ErrNoKey = i18n.E("secret.no_key")

// ErrDecrypt: Wert lässt sich nicht entschlüsseln (anderer Schlüssel oder beschädigt).
var ErrDecrypt = i18n.E("secret.decrypt_failed")

// Box ver- und entschlüsselt Werte. Eine nil-Box (kein Schlüssel) liefert ErrNoKey.
type Box struct {
	aead cipher.AEAD
}

// New erwartet einen 32-Byte-Schlüssel; nil/leer ergibt eine nil-Box.
func New(key []byte) (*Box, error) {
	if len(key) == 0 {
		return nil, nil
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("secret key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal verschlüsselt plaintext. ad (zusätzliche Daten) bindet den Wert an
// seinen Zweck, z. B. "integration:12"; beim Öffnen muss es übereinstimmen.
func (b *Box) Seal(plaintext []byte, ad string) (string, error) {
	if b == nil {
		return "", ErrNoKey
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := b.aead.Seal(nonce, nonce, plaintext, []byte(ad))
	return version + base64.StdEncoding.EncodeToString(out), nil
}

// Open entschlüsselt einen Wert von Seal mit denselben zusätzlichen Daten.
func (b *Box) Open(sealed, ad string) ([]byte, error) {
	if b == nil {
		return nil, ErrNoKey
	}
	raw, ok := strings.CutPrefix(sealed, version)
	if !ok {
		return nil, ErrDecrypt
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(data) < b.aead.NonceSize() {
		return nil, ErrDecrypt
	}
	n := b.aead.NonceSize()
	plain, err := b.aead.Open(nil, data[:n], data[n:], []byte(ad))
	if err != nil {
		return nil, ErrDecrypt
	}
	return plain, nil
}

// EnvPrefix: nur Env-Variablen mit diesem Präfix sind als Verweis erlaubt,
// damit z. B. APP_PASSWORD oder LABDECK_SECRET_KEY nie auslesbar sind.
const EnvPrefix = "SECRET_"

var refRe = regexp.MustCompile(`\$\{([^}]*)\}`)
var nameRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// Env liefert den Wert einer Env-Variablen (in Tests austauschbar).
type Env func(name string) (string, bool)

// IsRef meldet, ob s ausschließlich aus einem Verweis ${SECRET_…} besteht.
func IsRef(s string) bool {
	m := refRe.FindStringSubmatchIndex(s)
	return m != nil && m[0] == 0 && m[1] == len(s)
}

// HasRef meldet, ob s mindestens einen Verweis ${…} enthält.
func HasRef(s string) bool { return refRe.MatchString(s) }

// CheckRefs prüft die Namen aller Verweise in s (Präfix, Schreibweise), ohne
// sie aufzulösen.
func CheckRefs(s string) error {
	var errs []error
	for _, m := range refRe.FindAllStringSubmatch(s, -1) {
		name := m[1]
		if !nameRe.MatchString(name) || !strings.HasPrefix(name, EnvPrefix) || name == EnvPrefix {
			errs = append(errs, i18n.E("secret.env_prefix", "name", name, "prefix", EnvPrefix))
		}
	}
	return errors.Join(errs...)
}

// Expand ersetzt alle Verweise ${SECRET_…} in s durch die Werte der
// Env-Variablen. Fehlermeldungen nennen nur Variablennamen.
func Expand(s string, env Env) (string, error) {
	if err := CheckRefs(s); err != nil {
		return "", err
	}
	var missing []error
	out := refRe.ReplaceAllStringFunc(s, func(m string) string {
		name := m[2 : len(m)-1]
		v, ok := env(name)
		if !ok {
			missing = append(missing, i18n.E("secret.env_missing", "name", name))
			return ""
		}
		return v
	})
	if len(missing) > 0 {
		return "", errors.Join(missing...)
	}
	return out, nil
}
