// Package pbs überwacht einen Proxmox Backup Server: Belegung der
// Datastores und je Backup-Gruppe (VM, Container, Host) den letzten
// Snapshot mit Größe und Verify-Status.
//
// API (geprüft gegen das API-Schema von PBS 4):
//   - GET /api2/json/status/datastore-usage           Belegung je Datastore
//   - GET /api2/json/admin/datastore/{store}/namespace Namespaces
//   - GET /api2/json/admin/datastore/{store}/snapshots Snapshots (je Namespace)
//
// Anmeldung per API-Token; die Rolle DatastoreAudit auf /datastore genügt.
package pbs

import (
	"cmp"
	"context"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/uptime"
)

const Type = "pbs"

// Standard-Schwellwerte der Datastore-Belegung in Prozent.
const (
	defaultWarnPct  = 80
	defaultErrorPct = 90
)

type Integration struct{}

func New() *Integration { return &Integration{} }

func (*Integration) Type() string { return Type }

func (*Integration) Schema() integrations.ConfigSchema {
	type T = integrations.Text
	return integrations.ConfigSchema{
		Title: T{DE: "Proxmox Backup Server", EN: "Proxmox Backup Server"},
		Description: T{
			DE: "Letzter Snapshot, Größe und Verify-Status je VM, Container und Host sowie die Belegung der Datastores.",
			EN: "Last snapshot, size and verify state per VM, container and host, plus datastore usage.",
		},
		Icon:     "database-backup",
		Category: integrations.CategoryStorage,
		Docs:     "https://pbs.proxmox.com/docs/user-management.html#api-tokens",
		Fields: []integrations.Field{
			{Key: "base_url", Type: integrations.FieldURL, Required: true, Default: "https://pbs.lan:8007",
				Label: T{DE: "Adresse", EN: "Address"},
				Help: T{
					DE: "Adresse mit Port 8007. Bei selbst signiertem Zertifikat unter „Erweitert“ die TLS-Prüfung ausschalten.",
					EN: "Address with port 8007. With a self-signed certificate turn off TLS verification under \"Advanced\".",
				},
				Placeholder: "https://pbs.lan:8007"},
			{Key: "token_id", Type: integrations.FieldString, Required: true,
				Label: T{DE: "Token-ID", EN: "Token ID"},
				Help: T{
					DE: "Format benutzer@realm!tokenname. Das Token braucht eigene Rechte: Rolle DatastoreAudit auf /datastore.",
					EN: "Format user@realm!tokenname. The token needs its own permissions: role DatastoreAudit on /datastore.",
				},
				Placeholder: "labdeck@pbs!dashboard"},
			{Key: "token_secret", Type: integrations.FieldSecret, Required: true,
				Label: T{DE: "Token-Secret", EN: "Token secret"}},
			{Key: "datastores", Type: integrations.FieldString,
				Label:       T{DE: "Datastores", EN: "Datastores"},
				Help:        T{DE: "Komma-getrennt; leer = alle, die das Token sehen darf.", EN: "Comma-separated; empty = all the token may see."},
				Placeholder: "local, offsite"},
			{Key: "warn_pct", Type: integrations.FieldNumber, Default: float64(defaultWarnPct), Half: true,
				Min: integrations.Ptr(1), Max: integrations.Ptr(100),
				Label: T{DE: "Belegung: Warnung ab %", EN: "Usage: warning at %"}},
			{Key: "error_pct", Type: integrations.FieldNumber, Default: float64(defaultErrorPct), Half: true,
				Min: integrations.Ptr(1), Max: integrations.Ptr(100),
				Label: T{DE: "Belegung: Fehler ab %", EN: "Usage: error at %"}},
		},
	}
}

func header(cfg integrations.Config) http.Header {
	return http.Header{"Authorization": {"PBSAPIToken=" + strings.TrimSpace(cfg.String("token_id")) + ":" + cfg.Secret("token_secret")}}
}

func get[T any](ctx context.Context, cfg integrations.Config, path string, out *T) error {
	var env struct {
		Data *T `json:"data"`
	}
	if err := integrations.GetJSON(ctx, cfg.HTTP, cfg.BaseURL("base_url")+"/api2/json"+path, header(cfg), &env); err != nil {
		return err
	}
	if env.Data == nil {
		return i18n.E("pbs.unexpected")
	}
	*out = *env.Data
	return nil
}

type usage struct {
	Store string `json:"store"`
	Total *int64 `json:"total"`
	Used  *int64 `json:"used"`
	Error string `json:"error"`
}

type snapshot struct {
	Type         string `json:"backup-type"`
	ID           string `json:"backup-id"`
	Time         int64  `json:"backup-time"`
	Size         *int64 `json:"size"`
	Comment      string `json:"comment"`
	Verification *struct {
		State string `json:"state"`
	} `json:"verification"`
}

// stores liefert die Datastores (gefiltert nach der Einstellung).
func stores(ctx context.Context, cfg integrations.Config) ([]usage, error) {
	var all []usage
	if err := get(ctx, cfg, "/status/datastore-usage", &all); err != nil {
		return nil, err
	}
	var want []string
	for _, s := range strings.Split(cfg.String("datastores"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			want = append(want, s)
		}
	}
	if len(want) == 0 {
		if len(all) == 0 {
			return nil, i18n.E("pbs.no_datastores")
		}
		return all, nil
	}
	var out []usage
	for _, w := range want {
		i := slices.IndexFunc(all, func(u usage) bool { return u.Store == w })
		if i < 0 {
			return nil, i18n.E("pbs.datastore_missing", "store", w)
		}
		out = append(out, all[i])
	}
	return out, nil
}

func (*Integration) Test(ctx context.Context, cfg integrations.Config) error {
	_, err := stores(ctx, cfg)
	return err
}

func (*Integration) Collect(ctx context.Context, cfg integrations.Config) (integrations.Snapshot, error) {
	list, err := stores(ctx, cfg)
	if err != nil {
		return integrations.Snapshot{}, err
	}
	warn, fail := float64(defaultWarnPct), float64(defaultErrorPct)
	if cfg.Has("warn_pct") {
		warn = cfg.Float("warn_pct")
	}
	if cfg.Has("error_pct") {
		fail = cfg.Float("error_pct")
	}
	pct := integrations.ValueSpec{Warn: &warn, Fail: &fail}
	ok := integrations.StatusOK

	var snap integrations.Snapshot
	var groupsTotal int
	for _, u := range list {
		p := u.Store + "/"
		if u.Error != "" || u.Total == nil || u.Used == nil || *u.Total == 0 {
			// Datastore nicht lesbar (z. B. Wechseldatenträger nicht eingehängt)
			snap.Metrics = append(snap.Metrics, integrations.Metric{Key: p + "used_pct", Name: "used_pct", Group: u.Store,
				Unit: "%", Status: integrations.StatusError})
			continue
		}
		used := float64(*u.Used) / float64(*u.Total) * 100
		snap.Metrics = append(snap.Metrics,
			integrations.Metric{Key: p + "used", Name: "used", Group: u.Store, Value: float64(*u.Used), Unit: "bytes", Status: ok},
			integrations.Metric{Key: p + "total", Name: "total", Group: u.Store, Value: float64(*u.Total), Unit: "bytes", Status: ok},
			integrations.Metric{Key: p + "used_pct", Name: "used_pct", Group: u.Store, Value: used, Unit: "%", Status: pct.Status(used)},
		)

		backups, snapshots, err := storeBackups(ctx, cfg, u.Store)
		if err != nil {
			return integrations.Snapshot{}, err
		}
		groupsTotal += len(backups)
		snap.Backups = append(snap.Backups, backups...)
		snap.Metrics = append(snap.Metrics,
			integrations.Metric{Key: p + "groups", Name: "groups", Group: u.Store, Value: float64(len(backups)), Status: ok},
			integrations.Metric{Key: p + "snapshots", Name: "snapshots", Group: u.Store, Value: float64(snapshots), Status: ok},
		)
	}
	// Hauptkennzahlen vorne
	failed := 0
	for _, b := range snap.Backups {
		if b.Result == integrations.StatusError {
			failed++
		}
	}
	verifyStatus := ok
	if failed > 0 {
		verifyStatus = integrations.StatusError
	}
	snap.Metrics = append([]integrations.Metric{
		{Key: "groups", Name: "groups", Value: float64(groupsTotal), Status: ok},
		{Key: "verify_failed", Name: "verify_failed", Value: float64(failed), Status: verifyStatus, Derived: true},
		{Key: "datastores", Name: "datastores", Value: float64(len(list)), Status: ok},
	}, snap.Metrics...)
	return snap, nil
}

// storeBackups liefert je Backup-Gruppe (Namespace, Typ, ID) den jüngsten
// Snapshot und die Anzahl aller Snapshots des Datastores.
func storeBackups(ctx context.Context, cfg integrations.Config, store string) ([]integrations.Backup, int, error) {
	base := "/admin/datastore/" + url.PathEscape(store)
	var nss []struct {
		NS string `json:"ns"`
	}
	if err := get(ctx, cfg, base+"/namespace", &nss); err != nil {
		// ältere Versionen ohne Namespaces: nur die Wurzel
		if i18n.FromError(err).Code != "integration.not_found" {
			return nil, 0, err
		}
		nss = append(nss, struct {
			NS string `json:"ns"`
		}{})
	}
	type key struct{ ns, typ, id string }
	latest := map[key]snapshot{}
	runs := map[key][]integrations.Run{}
	since := time.Now().Add(-uptime.MaxRange).Unix()
	count := 0
	for _, ns := range nss {
		path := base + "/snapshots"
		if ns.NS != "" {
			path += "?ns=" + url.QueryEscape(ns.NS)
		}
		var snaps []snapshot
		if err := get(ctx, cfg, path, &snaps); err != nil {
			return nil, 0, err
		}
		count += len(snaps)
		for _, s := range snaps {
			k := key{ns.NS, s.Type, s.ID}
			if cur, ok := latest[k]; !ok || s.Time > cur.Time {
				latest[k] = s
			}
			if s.Time >= since {
				r := integrations.Run{At: time.Unix(s.Time, 0), Result: integrations.StatusOK}
				if s.Verification != nil && s.Verification.State == "failed" {
					r.Result = integrations.StatusError
				}
				runs[k] = append(runs[k], r)
			}
		}
	}

	out := make([]integrations.Backup, 0, len(latest))
	for k, s := range latest {
		group := store
		if k.ns != "" {
			group += " · " + k.ns
		}
		t := time.Unix(s.Time, 0)
		b := integrations.Backup{
			ID:      store + "/" + k.ns + "/" + k.typ + "/" + k.id,
			Name:    k.typ + "/" + k.id,
			Group:   group,
			LastRun: &t,
			Result:  integrations.StatusOK,
			Runs:    runs[k],
		}
		slices.SortFunc(b.Runs, func(x, y integrations.Run) int { return x.At.Compare(y.At) })
		if s.Comment != "" {
			b.Name += " (" + s.Comment + ")"
		}
		b.SizeBytes = s.Size
		b.Verify = "none"
		if s.Verification != nil {
			b.Verify = s.Verification.State
			if b.Verify == "failed" {
				b.Result = integrations.StatusError
			}
		}
		out = append(out, b)
	}
	slices.SortFunc(out, func(a, b integrations.Backup) int {
		return cmp.Or(strings.Compare(a.Group, b.Group), natural(a.Name, b.Name))
	})
	return out, count, nil
}

// natural sortiert „vm/100“ vor „vm/1000“.
func natural(a, b string) int {
	ta, ia, _ := strings.Cut(a, "/")
	tb, ib, _ := strings.Cut(b, "/")
	if c := strings.Compare(ta, tb); c != 0 {
		return c
	}
	na, ea := strconv.Atoi(strings.Fields(ia + " ")[0])
	nb, eb := strconv.Atoi(strings.Fields(ib + " ")[0])
	if ea == nil && eb == nil && na != nb {
		return cmp.Compare(na, nb)
	}
	return strings.Compare(ia, ib)
}
