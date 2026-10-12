package redfish

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
)

// client fragt einen Redfish-Dienst (DMTF) mit Basic Auth ab. Pfade kommen
// aus den Verweisen (@odata.id) der Antworten – nichts ist fest verdrahtet,
// damit iLO, iDRAC, XCC, Supermicro und OpenBMC gleichermaßen passen.
type client struct {
	cfg    integrations.Config
	base   *url.URL
	prefix string // Pfad vor /redfish (z. B. hinter einem Reverse Proxy), meist leer
	header http.Header
}

func newClient(cfg integrations.Config) (*client, error) {
	raw := strings.TrimSpace(cfg.BaseURL("base_url"))
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, i18n.E("integration.field_url_invalid")
	}
	user := strings.TrimSpace(cfg.String("username"))
	h := http.Header{}
	h.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(user+":"+cfg.Secret("password"))))
	h.Set("OData-Version", "4.0")
	// „https://ilo/redfish/v1“ eingegeben: Pfade beginnen ohnehin mit /redfish
	prefix := strings.TrimSuffix(strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/redfish/v1"), "/redfish")
	return &client{cfg: cfg, base: u, prefix: prefix, header: h}, nil
}

// get lädt einen Pfad (z. B. "/redfish/v1/Systems/1/") nach out. Nur Pfade
// auf demselben Host – Verweise der Antwort können keine Anfrage umlenken.
func (c *client) get(ctx context.Context, path string, out any) error {
	ref, err := url.Parse(path)
	if err != nil || ref.Host != "" || !strings.HasPrefix(ref.Path, "/redfish/") {
		return i18n.E("redfish.bad_link")
	}
	u := *c.base
	u.Path, u.RawPath, u.RawQuery = c.prefix+ref.Path, "", ref.RawQuery
	return integrations.GetJSON(ctx, c.cfg.HTTP, u.String(), c.header, out)
}

type ref struct {
	ID string `json:"@odata.id"`
}

// members lädt die Einträge einer Collection – mit $expand in einer Anfrage,
// wenn der Dienst das kann, sonst einzeln.
func (c *client) members(ctx context.Context, path string) ([]json.RawMessage, error) {
	var coll struct {
		Members []json.RawMessage `json:"Members"`
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	if err := c.get(ctx, path+sep+"$expand=.", &coll); err != nil {
		if err := c.get(ctx, path, &coll); err != nil {
			return nil, err
		}
	}
	out := make([]json.RawMessage, 0, len(coll.Members))
	for _, m := range coll.Members {
		var fields map[string]json.RawMessage
		if json.Unmarshal(m, &fields) != nil {
			continue
		}
		if len(fields) > 1 { // schon aufgeklappt
			out = append(out, m)
			continue
		}
		var r ref
		if json.Unmarshal(m, &r) != nil || r.ID == "" {
			continue
		}
		var full json.RawMessage
		if err := c.get(ctx, r.ID, &full); err != nil {
			return nil, err
		}
		out = append(out, full)
	}
	return out, nil
}

// first liefert den ersten Eintrag einer Collection als Verweis.
func (c *client) first(ctx context.Context, path string) (string, error) {
	var coll struct {
		Members []ref `json:"Members"`
	}
	if err := c.get(ctx, path, &coll); err != nil {
		return "", err
	}
	if len(coll.Members) == 0 || coll.Members[0].ID == "" {
		return "", i18n.E("redfish.no_system")
	}
	return coll.Members[0].ID, nil
}
