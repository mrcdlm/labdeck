// Package healthchecks bindet Healthchecks.io (Cloud oder self-hosted) über
// die Management-API v3 an. Ein Read-only-Key genügt.
package healthchecks

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
)

const Type = "healthchecks"

// DefaultBackupTag: Checks mit diesem Tag gelten zusätzlich als Backup.
const DefaultBackupTag = "backup"

type Integration struct{}

func New() Integration { return Integration{} }

func (Integration) Type() string { return Type }

func (Integration) Schema() integrations.ConfigSchema {
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

func (Integration) fetch(ctx context.Context, cfg integrations.Config) ([]check, error) {
	var resp listResponse
	err := integrations.GetJSON(ctx, cfg.HTTP, cfg.String("base_url")+"/api/v3/checks/",
		http.Header{"X-Api-Key": {cfg.Secret("api_key")}}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Checks == nil {
		return nil, i18n.E("healthchecks.unexpected")
	}
	return *resp.Checks, nil
}

func (i Integration) Test(ctx context.Context, cfg integrations.Config) error {
	_, err := i.fetch(ctx, cfg)
	return err
}

func (i Integration) Collect(ctx context.Context, cfg integrations.Config) (integrations.Snapshot, error) {
	checks, err := i.fetch(ctx, cfg)
	if err != nil {
		return integrations.Snapshot{}, err
	}
	backupTag := strings.TrimSpace(cfg.String("backup_tag"))

	var snap integrations.Snapshot
	counts := map[integrations.Status]int{}
	for _, c := range checks {
		st := status(c.Status)
		counts[st]++
		tags := strings.Fields(c.Tags)
		id := checkID(c)
		snap.Checks = append(snap.Checks, integrations.Check{
			ID: id, Name: c.Name, Tags: tags, Status: st, Desc: c.Desc,
			LastPing: c.LastPing, NextPing: c.NextPing, Raw: c.Status,
		})
		if backupTag != "" && slices.Contains(tags, backupTag) {
			snap.Backups = append(snap.Backups, integrations.Backup{
				ID: id, Name: c.Name, LastRun: c.LastPing, Result: st, Description: c.Desc,
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
		{Key: "checks_grace", Name: "grace", Value: float64(counts[integrations.StatusWarning]),
			Status: countStatus(counts[integrations.StatusWarning], integrations.StatusWarning)},
		{Key: "checks_down", Name: "down", Value: float64(counts[integrations.StatusError]),
			Status: countStatus(counts[integrations.StatusError], integrations.StatusError)},
		{Key: "checks_paused", Name: "paused", Value: float64(counts[integrations.StatusPaused]), Status: integrations.StatusOK},
	}
	return snap, nil
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
