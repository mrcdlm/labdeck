package nut

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
)

// client spricht das NUT-Netzwerkprotokoll (upsd, TCP 3493): eine Anfrage je
// Zeile, Listen als BEGIN LIST … END LIST, Fehler als „ERR <CODE>“.
type client struct {
	conn net.Conn
	r    *bufio.Reader
}

func dial(ctx context.Context, addr string, timeout time.Duration) (*client, error) {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, integrations.RequestError(err)
	}
	deadline := time.Now().Add(timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	conn.SetDeadline(deadline)
	return &client{conn: conn, r: bufio.NewReader(conn)}, nil
}

func (c *client) close() {
	fmt.Fprint(c.conn, "LOGOUT\n")
	c.conn.Close()
}

func (c *client) send(cmd string) error {
	if _, err := fmt.Fprint(c.conn, cmd+"\n"); err != nil {
		return integrations.RequestError(err)
	}
	return nil
}

func (c *client) line() (string, error) {
	s, err := c.r.ReadString('\n')
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return "", i18n.E("integration.timeout")
		}
		return "", i18n.E("nut.protocol")
	}
	s = strings.TrimRight(s, "\r\n")
	if code, ok := strings.CutPrefix(s, "ERR "); ok {
		return "", nutError(strings.Fields(code)[0])
	}
	return s, nil
}

func nutError(code string) error {
	switch code {
	case "ACCESS-DENIED", "USERNAME-REQUIRED", "PASSWORD-REQUIRED", "INVALID-USERNAME", "INVALID-PASSWORD":
		return i18n.E("nut.access_denied", "code", code)
	case "UNKNOWN-UPS":
		return i18n.E("nut.unknown_ups")
	case "DATA-STALE", "DRIVER-NOT-CONNECTED":
		return i18n.E("nut.driver", "code", code)
	}
	return i18n.E("nut.error", "code", code)
}

// login meldet an (optional; upsd verlangt das meist nur für Befehle).
func (c *client) login(user, pass string) error {
	for _, cmd := range []string{"USERNAME " + quote(user), "PASSWORD " + quote(pass)} {
		if err := c.send(cmd); err != nil {
			return err
		}
		s, err := c.line()
		if err != nil {
			return err
		}
		if !strings.HasPrefix(s, "OK") {
			return i18n.E("nut.protocol")
		}
	}
	return nil
}

// list liefert die Zeilen zwischen BEGIN LIST <what> und END LIST <what>.
func (c *client) list(what string) ([][]string, error) {
	if err := c.send("LIST " + what); err != nil {
		return nil, err
	}
	first, err := c.line()
	if err != nil {
		return nil, err
	}
	if first != "BEGIN LIST "+what {
		return nil, i18n.E("nut.protocol")
	}
	var out [][]string
	for {
		s, err := c.line()
		if err != nil {
			return nil, err
		}
		if s == "END LIST "+what {
			return out, nil
		}
		f, err := fields(s)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
}

// fields zerlegt eine Zeile; Anführungszeichen umschließen Werte mit
// Leerzeichen, \" und \\ sind maskiert.
func fields(s string) ([]string, error) {
	var out []string
	var b strings.Builder
	inQuote, escaped, have := false, false, false
	for _, r := range s {
		switch {
		case escaped:
			b.WriteRune(r)
			escaped = false
		case r == '\\' && inQuote:
			escaped = true
		case r == '"':
			inQuote = !inQuote
			have = true
		case r == ' ' && !inQuote:
			if have || b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
				have = false
			}
		default:
			b.WriteRune(r)
		}
	}
	if inQuote {
		return nil, i18n.E("nut.protocol")
	}
	if have || b.Len() > 0 {
		out = append(out, b.String())
	}
	return out, nil
}

func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
