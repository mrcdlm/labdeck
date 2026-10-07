// Package healthchecks bindet Healthchecks.io (Cloud oder self-hosted) über
// die Management-API v3 an. Ein Read-only-Key genügt.
package healthchecks

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/uptime"
)

const Type = "healthchecks"

// DefaultBackupTag: Checks mit diesem Tag gelten zusätzlich als Backup.
const DefaultBackupTag = "backup"

// Verlauf (Flips) je Check: höchstens so alt, bevor er neu geholt wird – bei
// einem Statuswechsel sofort. So bleibt es bei wenigen Anfragen, auch mit
// vielen Checks und kurzem Abfrageintervall.
const (
	flipsMaxAge      = 10 * time.Minute
	flipsPeriod      = uptime.MaxRange
	flipsConcurrency = 4
)

type Integration struct {
	now func() time.Time

	mu    sync.Mutex
	flips map[string]flipsEntry // base_url + Check-ID
}

type flipsEntry struct {
	fetched time.Time
	status  string // Status des Checks beim Abruf
	history []integrations.Transition
	since   time.Time
}

func New() *Integration { return &Integration{now: time.Now, flips: map[string]flipsEntry{}} }

func (*Integration) Type() string { return Type }

func (*Integration) Schema() integrations.ConfigSchema {
	type T = integrations.Text
	return integrations.ConfigSchema{
		Title: T{DE: "Healthchecks.io", EN: "Healthchecks.io"},
		Description: T{
			DE: "Cron-Jobs und Backups überwachen – Cloud oder selbst gehostet.",
			EN: "Monitor cron jobs and backups – cloud or self-hosted.",
		},
		Icon: "heart-pulse",
		Docs: "https://healthchecks.io/docs/api/",
		Fields: []integrations.Field{
			{
				Key: "base_url", Type: integrations.FieldURL, Required: true, Default: "https://healthchecks.io",
				Label: T{DE: "Adresse", EN: "Address"},
				Help: T{
					DE: "https://healthchecks.io oder die Adresse der eigenen Instanz, z. B. https://hc.example.com",
					EN: "https://healthchecks.io or the address of your own instance, e.g. https://hc.example.com",
				},
				Placeholder: "https://healthchecks.io",
			},
			{
				Key: "api_key", Type: integrations.FieldSecret, Required: true,
				Label: T{DE: "API-Key", EN: "API key"},
				Help: T{
					DE: "Projekt → Settings → API Access. Ein Read-only-Key genügt. Gilt für ein Projekt – für mehrere Projekte je eine Integration anlegen.",
					EN: "Project → Settings → API Access. A read-only key is enough. Valid for one project – add one integration per project.",
				},
			},
			{
				Key: "backup_tag", Type: integrations.FieldString, Default: DefaultBackupTag,
				Label: T{DE: "Backup-Tag", EN: "Backup tag"},
				Help: T{
					DE: "Checks mit diesem Tag erscheinen zusätzlich auf der Backups-Seite. Leer = keine.",
					EN: "Checks with this tag also appear on the Backups page. Empty = none.",
				},
				Placeholder: DefaultBackupTag,
			},
		},
	}
}

// check ist ein Check aus GET /api/v3/checks/ (nur die genutzten Felder).
type check struct {
	Name      string     `json:"name"`
	Slug      string     `json:"slug"`
	Tags      string     `json:"tags"`
	Desc      string     `json:"desc"`
	Status    string     `json:"status"`
	Started   bool       `json:"started"`
	LastPing  *time.Time `json:"last_ping"`
	NextPing  *time.Time `json:"next_ping"`
	UUID      string     `json:"uuid"`       // nur Read-write-Key
	UniqueKey string     `json:"unique_key"` // nur Read-only-Key
}

type listResponse struct {
	Checks *[]check `json:"checks"`
}

func (*Integration) fetch(ctx context.Context, cfg integrations.Config) ([]check, error) {
	var resp listResponse
	err := integrations.GetJSON(ctx, cfg.HTTP, cfg.BaseURL("base_url")+"/api/v3/checks/",
		http.Header{"X-Api-Key": {cfg.Secret("api_key")}}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Checks == nil {
		return nil, i18n.E("healthchecks.unexpected")
	}
	return *resp.Checks, nil
}

func (i *Integration) Test(ctx context.Context, cfg integrations.Config) error {
	_, err := i.fetch(ctx, cfg)
	return err
}

func (i *Integration) Collect(ctx context.Context, cfg integrations.Config) (integrations.Snapshot, error) {
	checks, err := i.fetch(ctx, cfg)
	if err != nil {
		return integrations.Snapshot{}, err
	}
	backupTag := strings.TrimSpace(cfg.String("backup_tag"))
	flips := i.history(ctx, cfg, checks)

	var snap integrations.Snapshot
	counts := map[integrations.Status]int{}
	for _, c := range checks {
		st := status(c.Status)
		counts[st]++
		tags := strings.Fields(c.Tags)
		id := checkID(c)
		h, ok := flips[id]
		var since *time.Time
		if ok {
			since = &h.since
		}
		snap.Checks = append(snap.Checks, integrations.Check{
			ID: id, Name: c.Name, Tags: tags, Status: st, Desc: c.Desc,
			LastPing: c.LastPing, NextPing: c.NextPing, Raw: c.Status,
			History: h.history, HistorySince: since,
		})
		if backupTag != "" && slices.Contains(tags, backupTag) {
			snap.Backups = append(snap.Backups, integrations.Backup{
				ID: id, Name: c.Name, LastRun: c.LastPing, Result: st, Description: c.Desc,
				History: h.history, HistorySince: since,
			})
		}
	}

	countStatus := func(n int, bad integrations.Status) integrations.Status {
		if n > 0 {
			return bad
		}
		return integrations.StatusOK
	}
	snap.Metrics = []integrations.Metric{
		{Key: "checks_total", Name: "Checks", Value: float64(len(checks)), Status: integrations.StatusOK},
		{Key: "checks_up", Name: "up", Value: float64(counts[integrations.StatusOK]), Status: integrations.StatusOK},
		{Key: "checks_grace", Name: "grace", Value: float64(counts[integrations.StatusWarning]), Derived: true,
			Status: countStatus(counts[integrations.StatusWarning], integrations.StatusWarning)},
		{Key: "checks_down", Name: "down", Value: float64(counts[integrations.StatusError]), Derived: true,
			Status: countStatus(counts[integrations.StatusError], integrations.StatusError)},
		{Key: "checks_paused", Name: "paused", Value: float64(counts[integrations.StatusPaused]), Status: integrations.StatusOK},
	}
	return snap, nil
}

// flip ist ein Eintrag aus GET /api/v3/checks/{id}/flips/.
type flip struct {
	Timestamp time.Time `json:"timestamp"`
	Up        int       `json:"up"`
}

// history liefert den Verlauf je Check-ID: aus dem Zwischenspeicher oder neu
// abgerufen (veraltet bzw. Status geändert). Scheitert der Abruf, bleibt der
// bisherige Verlauf – oder der Check hat keinen; die Abfrage selbst scheitert
// daran nicht.
func (i *Integration) history(ctx context.Context, cfg integrations.Config, checks []check) map[string]flipsEntry {
	base := cfg.BaseURL("base_url")
	now := i.now()
	out := map[string]flipsEntry{}
	var stale []check

	i.mu.Lock()
	for k, e := range i.flips {
		if now.Sub(e.fetched) > 6*flipsMaxAge {
			delete(i.flips, k) // Check gelöscht oder Integration entfernt
		}
	}
	for _, c := range checks {
		if c.UUID == "" && c.UniqueKey == "" {
			continue // Flips-Endpunkt braucht UUID bzw. unique_key
		}
		e, ok := i.flips[base+"\x00"+checkID(c)]
		if ok {
			out[checkID(c)] = e
		}
		if !ok || e.status != c.Status || now.Sub(e.fetched) >= flipsMaxAge {
			stale = append(stale, c)
		}
	}
	i.mu.Unlock()

	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		sem = make(chan struct{}, flipsConcurrency)
	)
	for _, c := range stale {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			h, err := i.fetchFlips(ctx, cfg, checkID(c))
			if err != nil {
				return
			}
			e := flipsEntry{fetched: now, status: c.Status, history: h, since: now.Add(-flipsPeriod)}
			mu.Lock()
			out[checkID(c)] = e
			mu.Unlock()
			i.mu.Lock()
			i.flips[base+"\x00"+checkID(c)] = e
			i.mu.Unlock()
		})
	}
	wg.Wait()
	return out
}

// fetchFlips holt die Statuswechsel der letzten 30 Tage, aufsteigend.
func (*Integration) fetchFlips(ctx context.Context, cfg integrations.Config, id string) ([]integrations.Transition, error) {
	var flips []flip
	u := cfg.BaseURL("base_url") + "/api/v3/checks/" + url.PathEscape(id) + "/flips/?seconds=" +
		strconv.Itoa(int(flipsPeriod.Seconds()))
	if err := integrations.GetJSON(ctx, cfg.HTTP, u, http.Header{"X-Api-Key": {cfg.Secret("api_key")}}, &flips); err != nil {
		return nil, err
	}
	slices.SortStableFunc(flips, func(a, b flip) int { return a.Timestamp.Compare(b.Timestamp) })
	out := make([]integrations.Transition, 0, len(flips))
	for _, f := range flips {
		t := integrations.Transition{At: f.Timestamp, From: integrations.StatusOK, To: integrations.StatusError}
		if f.Up == 1 {
			t.From, t.To = t.To, t.From
		}
		out = append(out, t)
	}
	return out, nil
}

// checkID: UUID (Read-write-Key) bzw. unique_key (Read-only-Key); beide
// akzeptiert auch der Flips-Endpunkt.
func checkID(c check) string {
	switch {
	case c.UUID != "":
		return c.UUID
	case c.UniqueKey != "":
		return c.UniqueKey
	case c.Slug != "":
		return "slug:" + c.Slug
	}
	return "name:" + c.Name
}

func status(s string) integrations.Status {
	switch s {
	case "up":
		return integrations.StatusOK
	case "grace":
		return integrations.StatusWarning
	case "down":
		return integrations.StatusError
	case "paused":
		return integrations.StatusPaused
	}
	return integrations.StatusUnknown // new und Unbekanntes
}
