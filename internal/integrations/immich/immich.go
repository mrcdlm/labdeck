// Package immich liest Kennzahlen einer Immich-Instanz: Anzahl Fotos und
// Videos sowie die Speicherbelegung, gesamt und je Benutzer.
//
// API (geprüft gegen die OpenAPI-Spezifikation von Immich 3.3):
//   - GET /api/server/statistics – nur Admins, Berechtigung server.statistics
//   - GET /api/server/version    – öffentlich (optional, nur zur Anzeige)
package immich

import (
	"context"
	"fmt"
	"net/http"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
)

const Type = "immich"

// Ab diesem Anteil der Quota eines Benutzers: Warnung bzw. Fehler.
const (
	quotaWarn  = 0.9
	quotaError = 1.0
)

type Integration struct{}

func New() *Integration { return &Integration{} }

func (*Integration) Type() string { return Type }

func (*Integration) Schema() integrations.ConfigSchema {
	type T = integrations.Text
	return integrations.ConfigSchema{
		Title: T{DE: "Immich", EN: "Immich"},
		Description: T{
			DE: "Fotos, Videos und Speicherbelegung – gesamt und je Benutzer.",
			EN: "Photos, videos and storage usage – in total and per user.",
		},
		Icon:     "image",
		Category: integrations.CategoryApps,
		Docs:     "https://api.immich.app/endpoints/server/getServerStatistics",
		Fields: []integrations.Field{
			{
				Key: "base_url", Type: integrations.FieldURL, Required: true,
				Label:       T{DE: "Adresse", EN: "Address"},
				Help:        T{DE: "Adresse der Immich-Instanz, z. B. http://immich.lan:2283", EN: "Address of the Immich instance, e.g. http://immich.lan:2283"},
				Placeholder: "http://immich.lan:2283",
			},
			{
				Key: "api_key", Type: integrations.FieldSecret, Required: true,
				Label: T{DE: "API-Key", EN: "API key"},
				Help: T{
					DE: "Als Admin unter Kontoeinstellungen → API-Schlüssel anlegen. Benötigte Berechtigung: server.statistics.",
					EN: "Create it as an admin under Account Settings → API Keys. Required permission: server.statistics.",
				},
			},
		},
	}
}

// stats ist die Antwort von GET /api/server/statistics.
type stats struct {
	Photos      *int64 `json:"photos"`
	Videos      int64  `json:"videos"`
	Usage       int64  `json:"usage"`
	UsagePhotos int64  `json:"usagePhotos"`
	UsageVideos int64  `json:"usageVideos"`
	UsageByUser []struct {
		UserID      string `json:"userId"`
		UserName    string `json:"userName"`
		Photos      int64  `json:"photos"`
		Videos      int64  `json:"videos"`
		Usage       int64  `json:"usage"`
		UsagePhotos int64  `json:"usagePhotos"`
		UsageVideos int64  `json:"usageVideos"`
		Quota       *int64 `json:"quotaSizeInBytes"`
	} `json:"usageByUser"`
}

func header(cfg integrations.Config) http.Header {
	return http.Header{"x-api-key": {cfg.Secret("api_key")}}
}

func (*Integration) fetch(ctx context.Context, cfg integrations.Config) (stats, error) {
	var s stats
	err := integrations.GetJSON(ctx, cfg.HTTP, cfg.BaseURL("base_url")+"/api/server/statistics", header(cfg), &s)
	if m := i18n.FromError(err); m.Code == "integration.unauthorized" && m.Params["status"] == "403" {
		return s, i18n.E("immich.forbidden")
	}
	if err != nil {
		return s, err
	}
	if s.Photos == nil {
		return s, i18n.E("immich.unexpected")
	}
	return s, nil
}

func (i *Integration) Test(ctx context.Context, cfg integrations.Config) error {
	_, err := i.fetch(ctx, cfg)
	return err
}

func (i *Integration) Collect(ctx context.Context, cfg integrations.Config) (integrations.Snapshot, error) {
	s, err := i.fetch(ctx, cfg)
	if err != nil {
		return integrations.Snapshot{}, err
	}
	ok := integrations.StatusOK
	snap := integrations.Snapshot{Metrics: []integrations.Metric{
		{Key: "photos", Name: "photos", Value: float64(*s.Photos), Status: ok},
		{Key: "videos", Name: "videos", Value: float64(s.Videos), Status: ok},
		{Key: "usage", Name: "usage", Value: float64(s.Usage), Unit: "bytes", Status: ok},
		{Key: "usage_photos", Name: "usage_photos", Value: float64(s.UsagePhotos), Unit: "bytes", Status: ok},
		{Key: "usage_videos", Name: "usage_videos", Value: float64(s.UsageVideos), Unit: "bytes", Status: ok},
		{Key: "users", Name: "users", Value: float64(len(s.UsageByUser)), Status: ok},
	}}
	for _, u := range s.UsageByUser {
		p := "user/" + u.UserID + "/"
		usage := integrations.Metric{Key: p + "usage", Name: "usage", Group: u.UserName, Value: float64(u.Usage),
			Unit: "bytes", Status: ok}
		snap.Metrics = append(snap.Metrics,
			integrations.Metric{Key: p + "photos", Name: "photos", Group: u.UserName, Value: float64(u.Photos), Status: ok},
			integrations.Metric{Key: p + "videos", Name: "videos", Group: u.UserName, Value: float64(u.Videos), Status: ok},
		)
		if u.Quota != nil && *u.Quota > 0 {
			ratio := float64(u.Usage) / float64(*u.Quota)
			switch {
			case ratio >= quotaError:
				usage.Status = integrations.StatusError
			case ratio >= quotaWarn:
				usage.Status = integrations.StatusWarning
			}
			snap.Metrics = append(snap.Metrics, usage,
				integrations.Metric{Key: p + "quota", Name: "quota", Group: u.UserName, Value: float64(*u.Quota),
					Unit: "bytes", Status: ok},
				integrations.Metric{Key: p + "quota_used", Name: "quota_used", Group: u.UserName, Value: ratio * 100,
					Unit: "%", Status: usage.Status})
		} else {
			snap.Metrics = append(snap.Metrics, usage)
		}
	}
	// Version: öffentlich, ohne Berechtigung – Fehler sind hier egal.
	var v struct{ Major, Minor, Patch int }
	if err := integrations.GetJSON(ctx, cfg.HTTP, cfg.BaseURL("base_url")+"/api/server/version", nil, &v); err == nil &&
		(v.Major > 0 || v.Minor > 0) {
		snap.Info = map[string]string{"version": fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)}
	}
	return snap, nil
}
