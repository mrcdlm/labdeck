// Package pihole liest Statistiken aus Pi-hole v6 über dessen REST-API:
// Anfragen, geblockte Anfragen, aktive Clients, Blockliste (Gravity),
// Blocking an/aus und verfügbare Updates.
//
// Endpunkte (geprüft gegen die OpenAPI-Spezifikation in pi-hole/FTL,
// src/api/docs/content/specs): POST /api/auth (Session, Header X-FTL-SID),
// GET /api/stats/summary, GET /api/dns/blocking, GET /api/info/version.
//
// Pi-hole begrenzt die Zahl gleichzeitiger Sessions. Die Integration meldet
// sich deshalb nur einmal an und verwendet die Session weiter, solange sie
// gilt; erst ein 401 führt zu genau einer neuen Anmeldung.
package pihole

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
)

const Type = "pihole"

const defaultGravityWarn = 8 // Tage

type Integration struct {
	mu       sync.Mutex
	sessions map[string]*session // Basis-URL + Hash des Passworts →
	now      func() time.Time
}

type session struct {
	sid      string
	validity time.Duration // 0 = läuft nicht ab
	expires  time.Time
}

// touch: Pi-hole verlängert die Session bei jeder Nutzung um ihre Gültigkeit.
func (s *session) touch(now time.Time) {
	if s.validity > 0 {
		// etwas früher erneuern, damit keine Abfrage ins Leere läuft
		s.expires = now.Add(s.validity - 10*time.Second)
	}
}

func New() *Integration { return &Integration{sessions: map[string]*session{}, now: time.Now} }

func (*Integration) Type() string { return Type }

func (*Integration) Schema() integrations.ConfigSchema {
	type T = integrations.Text
	return integrations.ConfigSchema{
		Title: T{DE: "Pi-hole", EN: "Pi-hole"},
		Description: T{
			DE: "Anfragen, geblockte Anfragen, Clients und Blockliste von Pi-hole v6 – und ob das Blocken aktiv ist.",
			EN: "Queries, blocked queries, clients and blocklist of Pi-hole v6 – and whether blocking is active.",
		},
		Icon: "shield-check",
		Docs: "https://docs.pi-hole.net/api/",
		Fields: []integrations.Field{
			{Key: "base_url", Type: integrations.FieldURL, Required: true,
				Label:       T{DE: "Adresse", EN: "Address"},
				Help:        T{DE: "Adresse der Pi-hole-Oberfläche ohne /admin.", EN: "Address of the Pi-hole web interface without /admin."},
				Placeholder: "http://pi.hole"},
			{Key: "password", Type: integrations.FieldSecret,
				Label: T{DE: "App-Passwort", EN: "App password"},
				Help: T{
					DE: "In Pi-hole unter Einstellungen → Web-Oberfläche / API → „App-Passwort konfigurieren“ erzeugen – nicht das Admin-Passwort verwenden. Leer, wenn Pi-hole kein Passwort hat.",
					EN: "Create it in Pi-hole under Settings → Web interface / API → \"Configure app password\" – don't use the admin password. Empty if Pi-hole has no password.",
				}},
			{Key: "gravity_warn", Type: integrations.FieldNumber, Default: float64(defaultGravityWarn),
				Min: integrations.Ptr(1), Max: integrations.Ptr(90),
				Label: T{DE: "Warnung, wenn die Blockliste älter ist als (Tage)", EN: "Warn if the blocklist is older than (days)"},
				Help: T{
					DE: "Pi-hole aktualisiert die Blockliste (Gravity) normalerweise wöchentlich.",
					EN: "Pi-hole normally updates the blocklist (gravity) weekly.",
				}},
		},
	}
}

func sessionKey(cfg integrations.Config) string {
	h := sha256.Sum256([]byte(cfg.Secret("password")))
	return cfg.BaseURL("base_url") + "|" + hex.EncodeToString(h[:8])
}

// login meldet sich an und merkt sich die Session.
func (i *Integration) login(ctx context.Context, cfg integrations.Config) (*session, error) {
	body, _ := json.Marshal(map[string]string{"password": cfg.Secret("password")})
	var resp struct {
		Session struct {
			Valid    bool    `json:"valid"`
			SID      *string `json:"sid"`
			Validity float64 `json:"validity"`
			TOTP     bool    `json:"totp"`
		} `json:"session"`
	}
	err := integrations.FetchJSON(ctx, cfg.HTTP, http.MethodPost, cfg.BaseURL("base_url")+"/api/auth", nil, body, &resp,
		func(status int, _ []byte) error {
			if status == http.StatusUnauthorized {
				if cfg.Secret("password") == "" {
					return i18n.E("pihole.password_required")
				}
				return i18n.E("pihole.login_failed")
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	if !resp.Session.Valid {
		if resp.Session.TOTP {
			return nil, i18n.E("pihole.totp")
		}
		return nil, i18n.E("pihole.login_failed")
	}
	s := &session{}
	if resp.Session.SID != nil {
		s.sid = *resp.Session.SID
	}
	if resp.Session.Validity > 0 {
		s.validity = time.Duration(resp.Session.Validity) * time.Second
	}
	s.touch(i.now())
	i.mu.Lock()
	i.sessions[sessionKey(cfg)] = s
	i.mu.Unlock()
	return s, nil
}

func (i *Integration) cached(cfg integrations.Config) *session {
	i.mu.Lock()
	defer i.mu.Unlock()
	s := i.sessions[sessionKey(cfg)]
	if s == nil || s.validity > 0 && i.now().After(s.expires) {
		return nil
	}
	return s
}

func (i *Integration) drop(cfg integrations.Config, s *session) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.sessions[sessionKey(cfg)] == s {
		delete(i.sessions, sessionKey(cfg))
	}
}

// get ruft path mit Session ab; 401 → einmal neu anmelden und wiederholen.
func (i *Integration) get(ctx context.Context, cfg integrations.Config, path string, out any) error {
	s := i.cached(cfg)
	fresh := false
	if s == nil {
		var err error
		if s, err = i.login(ctx, cfg); err != nil {
			return err
		}
		fresh = true
	}
	for {
		h := http.Header{}
		if s.sid != "" {
			h.Set("X-FTL-SID", s.sid)
		}
		err := integrations.GetJSON(ctx, cfg.HTTP, cfg.BaseURL("base_url")+path, h, out)
		if err == nil {
			i.mu.Lock()
			s.touch(i.now())
			i.mu.Unlock()
			return nil
		}
		if i18n.FromError(err).Code != "integration.unauthorized" {
			return err
		}
		i.drop(cfg, s)
		if fresh {
			if cfg.Secret("password") == "" {
				return i18n.E("pihole.password_required")
			}
			return i18n.E("pihole.login_failed")
		}
		if s, err = i.login(ctx, cfg); err != nil {
			return err
		}
		fresh = true
	}
}

func (i *Integration) Test(ctx context.Context, cfg integrations.Config) error {
	var out json.RawMessage
	return i.get(ctx, cfg, "/api/dns/blocking", &out)
}

type summary struct {
	Queries struct {
		Total          float64 `json:"total"`
		Blocked        float64 `json:"blocked"`
		PercentBlocked float64 `json:"percent_blocked"`
		UniqueDomains  float64 `json:"unique_domains"`
	} `json:"queries"`
	Clients struct {
		Active float64 `json:"active"`
		Total  float64 `json:"total"`
	} `json:"clients"`
	Gravity struct {
		DomainsBeingBlocked float64 `json:"domains_being_blocked"`
		LastUpdate          int64   `json:"last_update"`
	} `json:"gravity"`
}

type blocking struct {
	Blocking string   `json:"blocking"`
	Timer    *float64 `json:"timer"`
}

type versionPart struct {
	Local struct {
		Version *string `json:"version"`
	} `json:"local"`
	Remote struct {
		Version *string `json:"version"`
	} `json:"remote"`
}

type versions struct {
	Version struct {
		Core   versionPart `json:"core"`
		Web    versionPart `json:"web"`
		FTL    versionPart `json:"ftl"`
		Docker struct {
			Local  *string `json:"local"`
			Remote *string `json:"remote"`
		} `json:"docker"`
	} `json:"version"`
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

func (i *Integration) Collect(ctx context.Context, cfg integrations.Config) (integrations.Snapshot, error) {
	var sum summary
	if err := i.get(ctx, cfg, "/api/stats/summary", &sum); err != nil {
		return integrations.Snapshot{}, err
	}
	var bl blocking
	if err := i.get(ctx, cfg, "/api/dns/blocking", &bl); err != nil {
		return integrations.Snapshot{}, err
	}
	var ver versions
	verErr := i.get(ctx, cfg, "/api/info/version", &ver) // optional

	ok := integrations.StatusOK
	snap := integrations.Snapshot{Info: map[string]string{}}
	// Blocken: aus = Warnung (mit Timer: läuft bald wieder an), Fehler = Fehler
	bst, bv := integrations.StatusUnknown, 0.0
	switch bl.Blocking {
	case "enabled":
		bst, bv = ok, 1
	case "disabled":
		bst = integrations.StatusWarning
		if bl.Timer != nil && *bl.Timer > 0 {
			snap.Info["blocking_timer"] = time.Duration(*bl.Timer * float64(time.Second)).Round(time.Second).String()
		}
	case "failed":
		bst = integrations.StatusError
	}
	snap.Info["blocking"] = bl.Blocking

	gravityWarn := float64(defaultGravityWarn)
	if cfg.Has("gravity_warn") {
		gravityWarn = cfg.Float("gravity_warn")
	}
	// Blocken an, aber leere Blockliste: es wird nichts geblockt
	listSt := ok
	if sum.Gravity.DomainsBeingBlocked == 0 && bl.Blocking == "enabled" {
		listSt = integrations.StatusWarning
	}
	snap.Metrics = []integrations.Metric{
		{Key: "blocked_pct", Name: "blocked_pct", Value: sum.Queries.PercentBlocked, Unit: "%", Status: ok},
		{Key: "queries", Name: "queries", Value: sum.Queries.Total, Status: ok},
		{Key: "blocked", Name: "blocked", Value: sum.Queries.Blocked, Status: ok},
		{Key: "clients", Name: "clients", Value: sum.Clients.Active, Status: ok},
		{Key: "domains", Name: "domains", Value: sum.Queries.UniqueDomains, Status: ok},
		{Key: "gravity_domains", Name: "gravity_domains", Value: sum.Gravity.DomainsBeingBlocked, Status: listSt},
		{Key: "blocking", Name: "blocking", Value: bv, Unit: "onoff", Status: bst},
	}
	if sum.Gravity.LastUpdate > 0 {
		age := i.now().Sub(time.Unix(sum.Gravity.LastUpdate, 0)).Seconds()
		st := ok
		if age > gravityWarn*86400 {
			st = integrations.StatusWarning
		}
		snap.Metrics = append(snap.Metrics, integrations.Metric{Key: "gravity_age", Name: "gravity_age", Value: max(age, 0), Unit: "s", Status: st})
	}

	if verErr == nil {
		v := ver.Version
		updates := 0
		if d := str(v.Docker.Local); d != "" {
			snap.Info["version"] = "Docker " + d
			if r := str(v.Docker.Remote); r != "" && r != d {
				updates, snap.Info["update"] = 1, r
			}
		} else {
			snap.Info["version"] = "Core " + cmp.Or(str(v.Core.Local.Version), "?")
			for _, p := range []versionPart{v.Core, v.Web, v.FTL} {
				if l, r := str(p.Local.Version), str(p.Remote.Version); l != "" && r != "" && l != r {
					updates++
				}
			}
		}
		snap.Info["ftl"] = str(v.FTL.Local.Version)
		snap.Metrics = append(snap.Metrics, integrations.Metric{Key: "updates", Name: "updates", Value: float64(updates), Status: ok})
	} else if code := i18n.FromError(verErr).Code; code == "integration.timeout" || code == "integration.canceled" {
		return integrations.Snapshot{}, verErr
	}
	return snap, nil
}
