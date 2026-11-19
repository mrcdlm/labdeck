// Package netbird liest Peers (Clients), Reverse-Proxy-Dienste und
// Proxy-Cluster aus der Management-API von NetBird – Cloud oder
// selbst gehostet.
//
// API (geprüft gegen shared/management/http/api/openapi.yml): Anmeldung per
// Personal Access Token im Header „Authorization: Token …“.
//
//	GET /api/peers                          Peers mit connected, last_seen, groups …
//	GET /api/reverse-proxies/services       Reverse-Proxy-Dienste mit meta.status
//	GET /api/reverse-proxies/clusters       Proxy-Cluster mit online
//	GET /api/instance/version               Version des Management-Servers
//
// Nur gewählte Peers (Gruppe oder Namen) werden überwacht: offline ist dann
// ein Problem. Alle anderen – Laptops, Handys – erscheinen nur zur Anzeige.
package netbird

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
)

const Type = "netbird"

type Integration struct{}

func New() *Integration { return &Integration{} }

func (*Integration) Type() string { return Type }

func (*Integration) Schema() integrations.ConfigSchema {
	type T = integrations.Text
	return integrations.ConfigSchema{
		Title: T{DE: "NetBird", EN: "NetBird"},
		Description: T{
			DE: "Peers (Clients) und Reverse-Proxy-Dienste aus NetBird: online/offline, Version, Zertifikate; gewählte Peers werden überwacht.",
			EN: "Peers (clients) and reverse proxy services from NetBird: online/offline, version, certificates; selected peers are monitored.",
		},
		Icon:     "network",
		Category: integrations.CategoryNetwork,
		Docs:     "https://docs.netbird.io/api",
		Fields: []integrations.Field{
			{Key: "base_url", Type: integrations.FieldURL, Required: true, Default: "https://api.netbird.io",
				Label: T{DE: "Management-Adresse", EN: "Management address"},
				Help: T{
					DE: "NetBird Cloud: https://api.netbird.io – selbst gehostet: Adresse des Management-Servers (wie im Dashboard).",
					EN: "NetBird Cloud: https://api.netbird.io – self-hosted: address of the management server (as used by the dashboard).",
				},
				Placeholder: "https://api.netbird.io"},
			{Key: "api_token", Type: integrations.FieldSecret, Required: true,
				Label: T{DE: "Access Token", EN: "Access token"},
				Help: T{
					DE: "Im NetBird-Dashboard unter Team → Service Users einen Service-Benutzer anlegen (Leserechte genügen) und dort ein Access Token erzeugen.",
					EN: "In the NetBird dashboard under Team → Service Users, create a service user (read access is enough) and generate an access token there.",
				}},
			{Key: "watch_group", Type: integrations.FieldString,
				Label:       T{DE: "Überwachte Gruppe", EN: "Monitored group"},
				Help:        T{DE: "Peers dieser NetBird-Gruppe(n) werden überwacht – offline ist ein Problem. Komma-getrennt; leer = keine.", EN: "Peers of this NetBird group(s) are monitored – offline is a problem. Comma-separated; empty = none."},
				Placeholder: "server"},
			{Key: "watch_peers", Type: integrations.FieldString,
				Label:       T{DE: "Überwachte Peers", EN: "Monitored peers"},
				Help:        T{DE: "Zusätzlich einzelne Peers nach Name oder Hostname, komma-getrennt. Alle übrigen Peers werden nur angezeigt.", EN: "Additional single peers by name or hostname, comma-separated. All other peers are only displayed."},
				Placeholder: "nas, docker01"},
			{Key: "reverse_proxy", Type: integrations.FieldBool, Default: true,
				Label: T{DE: "Reverse-Proxy-Dienste", EN: "Reverse proxy services"},
				Help: T{
					DE: "Dienste und Proxy-Cluster des NetBird Reverse Proxy abfragen (fehlt die Funktion, wird der Teil übersprungen).",
					EN: "Query services and proxy clusters of the NetBird reverse proxy (skipped if the feature is not available).",
				}},
		},
	}
}

func header(cfg integrations.Config) http.Header {
	return http.Header{"Authorization": {"Token " + strings.TrimSpace(cfg.Secret("api_token"))}}
}

func get(ctx context.Context, cfg integrations.Config, path string, out any) error {
	return integrations.GetJSON(ctx, cfg.HTTP, cfg.BaseURL("base_url")+path, header(cfg), out)
}

type group struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type peer struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	IP               string     `json:"ip"`
	Connected        bool       `json:"connected"`
	LastSeen         *time.Time `json:"last_seen"`
	OS               string     `json:"os"`
	Version          string     `json:"version"`
	Hostname         string     `json:"hostname"`
	DNSLabel         string     `json:"dns_label"`
	Groups           []group    `json:"groups"`
	LoginExpired     bool       `json:"login_expired"`
	ApprovalRequired bool       `json:"approval_required"`
	CountryCode      string     `json:"country_code"`
	CityName         string     `json:"city_name"`
	Ephemeral        bool       `json:"ephemeral"`
}

type target struct {
	TargetType string `json:"target_type"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Protocol   string `json:"protocol"`
	Path       string `json:"path"`
	Enabled    bool   `json:"enabled"`
}

type service struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Domain       string   `json:"domain"`
	Mode         string   `json:"mode"`
	ProxyCluster string   `json:"proxy_cluster"`
	Targets      []target `json:"targets"`
	Enabled      bool     `json:"enabled"`
	Private      bool     `json:"private"`
	Meta         struct {
		Status              string     `json:"status"`
		CertificateIssuedAt *time.Time `json:"certificate_issued_at"`
	} `json:"meta"`
}

type cluster struct {
	ID               string `json:"id"`
	Address          string `json:"address"`
	Online           bool   `json:"online"`
	ConnectedProxies int    `json:"connected_proxies"`
}

func peers(ctx context.Context, cfg integrations.Config) ([]peer, error) {
	var ps []peer
	if err := get(ctx, cfg, "/api/peers", &ps); err != nil {
		if m := i18n.FromError(err); m.Code == "integration.bad_json" || m.Code == "integration.not_found" {
			return nil, i18n.E("netbird.unexpected")
		}
		return nil, err
	}
	for _, p := range ps {
		if p.ID == "" {
			return nil, i18n.E("netbird.unexpected")
		}
	}
	return ps, nil
}

func (*Integration) Test(ctx context.Context, cfg integrations.Config) error {
	_, err := peers(ctx, cfg)
	return err
}

// list teilt eine komma-getrennte Eingabe (klein geschrieben).
func list(s string) []string {
	var out []string
	for _, n := range strings.Split(s, ",") {
		if n = strings.ToLower(strings.TrimSpace(n)); n != "" {
			out = append(out, n)
		}
	}
	return out
}

func (p peer) watched(groups, names []string) bool {
	for _, g := range p.Groups {
		if slices.Contains(groups, strings.ToLower(g.Name)) {
			return true
		}
	}
	short := strings.ToLower(p.DNSLabel)
	if i := strings.IndexByte(short, '.'); i > 0 {
		short = short[:i]
	}
	return slices.Contains(names, strings.ToLower(p.Name)) || slices.Contains(names, strings.ToLower(p.Hostname)) ||
		(short != "" && slices.Contains(names, short))
}

// proxyStatus: Ampel aus meta.status (pending, active, tunnel_not_created,
// certificate_pending, certificate_failed, error).
func proxyStatus(s service) integrations.Status {
	if !s.Enabled {
		return integrations.StatusPaused
	}
	switch s.Meta.Status {
	case "active":
		return integrations.StatusOK
	case "pending", "certificate_pending":
		return integrations.StatusWarning
	case "tunnel_not_created", "certificate_failed", "error":
		return integrations.StatusError
	}
	return integrations.StatusUnknown
}

func (*Integration) Collect(ctx context.Context, cfg integrations.Config) (integrations.Snapshot, error) {
	ps, err := peers(ctx, cfg)
	if err != nil {
		return integrations.Snapshot{}, err
	}
	groups, names := list(cfg.String("watch_group")), list(cfg.String("watch_peers"))
	ok := integrations.StatusOK

	var snap integrations.Snapshot
	var problems []integrations.Metric
	var online, watched, watchedOff int
	slices.SortFunc(ps, func(a, b peer) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	for _, p := range ps {
		w := p.watched(groups, names)
		r := integrations.Resource{ID: "peer/" + p.ID, Name: p.Name, Kind: integrations.KindPeer, State: "offline",
			Status: integrations.StatusPaused, Info: map[string]string{"ip": p.IP, "dns": p.DNSLabel, "os": p.OS,
				"version": p.Version, "hostname": p.Hostname}}
		var gs []string
		for _, g := range p.Groups {
			if g.Name != "All" {
				gs = append(gs, g.Name)
			}
		}
		r.Tags = gs
		if p.LastSeen != nil && !p.LastSeen.IsZero() {
			r.Info["last_seen"] = p.LastSeen.UTC().Format(time.RFC3339)
		}
		if p.CityName != "" || p.CountryCode != "" {
			r.Info["location"] = strings.TrimPrefix(p.CityName+", "+p.CountryCode, ", ")
		}
		if p.Connected {
			online++
			r.State, r.Status = "online", ok
		}
		if w {
			watched++
			r.Info["watched"] = "1"
			switch {
			case !p.Connected:
				r.Status = integrations.StatusError
				watchedOff++
			case p.LoginExpired:
				r.State, r.Status = "login_expired", integrations.StatusWarning
			case p.ApprovalRequired:
				r.State, r.Status = "approval_required", integrations.StatusWarning
			}
			if r.Status != ok {
				problems = append(problems, integrations.Metric{Key: "res/" + r.ID + "/peer", Name: "peer",
					Group: p.Name, Unit: "okfail", Status: r.Status, Resource: r.ID})
			}
		}
		snap.Resources = append(snap.Resources, r)
	}

	main := []integrations.Metric{
		{Key: "peers_online", Name: "peers_online", Value: float64(online), Status: ok},
		{Key: "peers", Name: "peers", Value: float64(len(ps)), Status: ok},
	}
	if watched > 0 {
		st := ok
		if watchedOff > 0 {
			st = integrations.StatusError
		}
		main = append(main, integrations.Metric{Key: "watched_offline", Name: "watched_offline", Value: float64(watchedOff),
			Status: st, Derived: true})
	}

	if !cfg.Has("reverse_proxy") || cfg.Bool("reverse_proxy") {
		var svcs []service
		err := get(ctx, cfg, "/api/reverse-proxies/services", &svcs)
		switch code := i18n.FromError(err).Code; {
		case err == nil:
			var active int
			for _, s := range svcs {
				st := proxyStatus(s)
				state := s.Meta.Status
				if !s.Enabled {
					state = "disabled"
				}
				name := s.Domain
				if name == "" {
					name = s.Name
				}
				r := integrations.Resource{ID: "proxy/" + s.ID, Name: name, Kind: integrations.KindProxy, State: state,
					Status: st, Info: map[string]string{"cluster": s.ProxyCluster, "mode": s.Mode}}
				var ts []string
				for _, t := range s.Targets {
					if !t.Enabled {
						continue
					}
					h := t.Host
					if t.Port > 0 {
						h += ":" + strconv.Itoa(t.Port)
					}
					if t.Path != "" && t.Path != "/" {
						h += t.Path
					}
					ts = append(ts, h)
				}
				r.Info["targets"] = strings.Join(ts, ", ")
				if c := s.Meta.CertificateIssuedAt; c != nil && !c.IsZero() {
					r.Info["cert_issued"] = c.UTC().Format(time.RFC3339)
				}
				if s.Private {
					r.Info["private"] = "1"
				}
				if st == ok {
					active++
				}
				if st == integrations.StatusWarning || st == integrations.StatusError {
					problems = append(problems, integrations.Metric{Key: "res/" + r.ID + "/proxy", Name: "proxy",
						Group: name, Unit: "okfail", Status: st, Resource: r.ID})
				}
				snap.Resources = append(snap.Resources, r)
			}
			main = append(main,
				integrations.Metric{Key: "proxies_active", Name: "proxies_active", Value: float64(active), Status: ok},
				integrations.Metric{Key: "proxies", Name: "proxies", Value: float64(len(svcs)), Status: ok})

			var cls []cluster
			if get(ctx, cfg, "/api/reverse-proxies/clusters", &cls) == nil {
				for _, c := range cls {
					// nur Cluster, auf denen Dienste laufen
					used := slices.ContainsFunc(svcs, func(s service) bool { return s.ProxyCluster == c.Address })
					if used && !c.Online {
						problems = append(problems, integrations.Metric{Key: "cluster/" + c.Address + "/online", Name: "cluster",
							Group: c.Address, Unit: "okfail", Status: integrations.StatusError})
					}
				}
			}
		case code == "integration.not_found", code == "integration.bad_json":
			// ältere NetBird-Version ohne Reverse Proxy
		case code == "integration.unauthorized":
			// Token darf den Reverse Proxy nicht sehen – Peers genügen
		default:
			return integrations.Snapshot{}, err
		}
	}

	snap.Metrics = append(main, problems...)

	var v struct {
		Current string `json:"management_current_version"`
	}
	if get(ctx, cfg, "/api/instance/version", &v) == nil && v.Current != "" {
		snap.Info = map[string]string{"version": "NetBird " + strings.TrimPrefix(v.Current, "v")}
	}
	return snap, nil
}
