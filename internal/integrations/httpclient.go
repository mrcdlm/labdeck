package integrations

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
)

// maxBody begrenzt Antworten fremder Dienste.
const maxBody = 16 << 20

// NewHTTPClient liefert den Client einer Instanz. tlsVerify=false erlaubt
// selbst signierte Zertifikate (typisch im LAN). Weiterleitungen auf einen
// anderen Host werden nicht verfolgt: eigene Header wie X-Api-Key würde Go
// sonst mitschicken.
func NewHTTPClient(timeout time.Duration, tlsVerify bool) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if !tlsVerify {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // bewusst einstellbar
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errRedirect
			}
			if req.URL.Host != via[0].URL.Host {
				return errRedirect
			}
			return nil
		},
	}
}

var errRedirect = errors.New("redirect to another host")

// GetJSON ruft u per GET ab und dekodiert die JSON-Antwort nach out. Fehler
// sind übersetzbar und enthalten weder Header noch Query-Parameter.
func GetJSON(ctx context.Context, c *http.Client, u string, header http.Header, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return i18n.E("integration.field_url_invalid")
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return RequestError(err)
	}
	defer resp.Body.Close()
	if err := HTTPStatusError(resp); err != nil {
		return err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return RequestError(err)
	}
	if len(body) > maxBody {
		return i18n.E("integration.too_large")
	}
	if err := json.Unmarshal(body, out); err != nil {
		return i18n.E("integration.bad_json", "detail", jsonDetail(err))
	}
	return nil
}

// HTTPStatusError übersetzt HTTP-Fehlerstatus (nil bei 2xx).
func HTTPStatusError(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	status := strconv.Itoa(resp.StatusCode)
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return i18n.E("integration.unauthorized", "status", status)
	case http.StatusNotFound:
		return i18n.E("integration.not_found")
	case http.StatusTooManyRequests:
		return i18n.E("integration.rate_limited")
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return i18n.E("integration.redirect", "status", status)
	}
	return i18n.E("integration.http", "status", status)
}

// RequestError übersetzt Verbindungsfehler. Die URL (ggf. mit Query) wird
// bewusst weggelassen.
func RequestError(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		err = uerr.Err
	}
	var dnsErr *net.DNSError
	var certErr *tls.CertificateVerificationError
	var unknownAuth x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	var invalidErr x509.CertificateInvalidError
	switch {
	case errors.Is(err, errRedirect):
		return i18n.E("integration.redirect_host")
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) || isTimeout(err):
		return i18n.E("integration.timeout")
	case errors.Is(err, context.Canceled):
		return i18n.E("integration.canceled")
	case errors.As(err, &dnsErr):
		return i18n.E("integration.dns", "host", dnsErr.Name)
	case errors.As(err, &unknownAuth):
		return i18n.E("integration.tls_untrusted")
	case errors.As(err, &hostErr):
		return i18n.E("integration.tls_hostname", "host", hostErr.Host)
	case errors.As(err, &invalidErr):
		return i18n.E("integration.tls_invalid", "detail", invalidErr.Error())
	case errors.As(err, &certErr):
		return i18n.E("integration.tls_invalid", "detail", certErr.Err.Error())
	case isRefused(err):
		return i18n.E("integration.refused")
	}
	var recErr tls.RecordHeaderError
	if errors.As(err, &recErr) || strings.Contains(err.Error(), "server gave HTTP response to HTTPS client") {
		return i18n.E("integration.tls_not_https")
	}
	return i18n.E("integration.unreachable", "detail", err.Error())
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func isRefused(err error) bool {
	var op *net.OpError
	if !errors.As(err, &op) {
		return false
	}
	var se *os.SyscallError
	return op.Op == "dial" && errors.As(op.Err, &se) && se.Syscall == "connect"
}

func jsonDetail(err error) string {
	var se *json.SyntaxError
	if errors.As(err, &se) {
		return fmt.Sprintf("offset %d", se.Offset)
	}
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) {
		return fmt.Sprintf("%s: %s", te.Field, te.Value)
	}
	return err.Error()
}
