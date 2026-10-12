package redfish

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mrcdlm/labdeck/internal/integrations"
)

// item ist ein Controller, Laufwerk oder logisches Laufwerk.
type item struct {
	key   string // z. B. "drive/1I:1:1", "volume/1", "controller/0"
	label string
	state integrations.Status
	life  *float64 // Restlebensdauer in % (SSDs)
}

// fetchStorage liest die Laufwerke: zuerst DMTF Storage (iDRAC, XCC,
// Supermicro, neuere iLO-Controller), sonst HPE SmartStorage (iLO 5 mit
// Smart-Array-Controllern).
func fetchStorage(ctx context.Context, c *client, sysPath string, d *data) error {
	if d.sys.Storage != nil {
		items, err := dmtfStorage(ctx, c, d.sys.Storage.ID)
		if err != nil {
			return err
		}
		if hasDrives(items) {
			d.storage, d.storageOK = items, true
			return nil
		}
	}
	if d.sys.Oem.Hpe == nil {
		return nil
	}
	items, err := smartStorage(ctx, c, strings.TrimRight(sysPath, "/")+"/SmartStorage/ArrayControllers")
	if err != nil {
		return err
	}
	if hasDrives(items) {
		d.storage, d.storageOK = items, true
	}
	return nil
}

func hasDrives(items []item) bool {
	for _, it := range items {
		if strings.HasPrefix(it.key, "drive/") {
			return true
		}
	}
	return false
}

// size kurz für Beschriftungen, z. B. "960 GB", "1,9 TB".
func size(bytes float64) string {
	switch {
	case bytes <= 0:
		return ""
	case bytes >= 1e12:
		return strings.Replace(fmt.Sprintf("%.1f TB", bytes/1e12), ".0 TB", " TB", 1)
	}
	return fmt.Sprintf("%.0f GB", bytes/1e9)
}

func label(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p = strings.Join(strings.Fields(p), " "); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

func dmtfStorage(ctx context.Context, c *client, path string) ([]item, error) {
	list, err := c.members(ctx, path)
	if err != nil {
		return nil, err
	}
	var out []item
	for _, raw := range list {
		var st struct {
			ID      string `json:"Id"`
			Name    string `json:"Name"`
			Status  status `json:"Status"`
			Drives  []ref  `json:"Drives"`
			Volumes *ref   `json:"Volumes"`
		}
		if json.Unmarshal(raw, &st) != nil {
			continue
		}
		if s, present := health(st.Status); present {
			out = append(out, item{key: "controller/" + st.ID, label: cmp.Or(st.Name, st.ID), state: s})
		}
		for _, r := range st.Drives {
			var dr struct {
				ID                            string   `json:"Id"`
				Name                          string   `json:"Name"`
				Model                         string   `json:"Model"`
				CapacityBytes                 float64  `json:"CapacityBytes"`
				MediaType                     string   `json:"MediaType"`
				Status                        status   `json:"Status"`
				PredictedMediaLifeLeftPercent *float64 `json:"PredictedMediaLifeLeftPercent"`
				PhysicalLocation              struct {
					PartLocation struct {
						ServiceLabel string `json:"ServiceLabel"`
					} `json:"PartLocation"`
				} `json:"PhysicalLocation"`
			}
			if err := c.get(ctx, r.ID, &dr); err != nil {
				return nil, err
			}
			s, present := health(dr.Status)
			if !present {
				continue
			}
			loc := cmp.Or(dr.PhysicalLocation.PartLocation.ServiceLabel, dr.Name, dr.ID)
			out = append(out, item{key: "drive/" + cmp.Or(dr.ID, loc), label: label(loc, dr.Model, size(dr.CapacityBytes)),
				state: s, life: dr.PredictedMediaLifeLeftPercent})
		}
		if st.Volumes != nil {
			vols, err := c.members(ctx, st.Volumes.ID)
			if err != nil {
				continue // Volumes sind nicht überall lesbar
			}
			for _, raw := range vols {
				var v struct {
					ID       string `json:"Id"`
					Name     string `json:"Name"`
					RAIDType string `json:"RAIDType"`
					Status   status `json:"Status"`
				}
				if json.Unmarshal(raw, &v) != nil {
					continue
				}
				if s, present := health(v.Status); present {
					out = append(out, item{key: "volume/" + v.ID, label: label(cmp.Or(v.Name, v.ID), v.RAIDType), state: s})
				}
			}
		}
	}
	return out, nil
}

// smartStorage liest HPE-Smart-Array-Controller mit Disk- und Logical-Drives.
func smartStorage(ctx context.Context, c *client, path string) ([]item, error) {
	ctrls, err := c.members(ctx, path)
	if err != nil {
		return nil, err
	}
	var out []item
	for _, raw := range ctrls {
		var ctl struct {
			ID       string `json:"Id"`
			Model    string `json:"Model"`
			Location string `json:"Location"`
			Status   status `json:"Status"`
			Links    struct {
				PhysicalDrives *ref `json:"PhysicalDrives"`
				LogicalDrives  *ref `json:"LogicalDrives"`
			} `json:"Links"`
		}
		if json.Unmarshal(raw, &ctl) != nil {
			continue
		}
		if s, present := health(ctl.Status); present {
			out = append(out, item{key: "controller/" + ctl.ID, label: label(ctl.Model, ctl.Location), state: s})
		}
		base := strings.TrimRight(cmp.Or(refID(raw), path+"/"+ctl.ID), "/")
		drivesPath, volsPath := base+"/DiskDrives", base+"/LogicalDrives"
		if ctl.Links.PhysicalDrives != nil {
			drivesPath = ctl.Links.PhysicalDrives.ID
		}
		if ctl.Links.LogicalDrives != nil {
			volsPath = ctl.Links.LogicalDrives.ID
		}
		drives, err := c.members(ctx, drivesPath)
		if err != nil {
			return nil, err
		}
		for _, raw := range drives {
			var dr struct {
				ID                                string   `json:"Id"`
				Model                             string   `json:"Model"`
				Location                          string   `json:"Location"`
				CapacityMiB                       float64  `json:"CapacityMiB"`
				Status                            status   `json:"Status"`
				SSDEnduranceUtilizationPercentage *float64 `json:"SSDEnduranceUtilizationPercentage"`
			}
			if json.Unmarshal(raw, &dr) != nil {
				continue
			}
			s, present := health(dr.Status)
			if !present {
				continue
			}
			it := item{key: "drive/" + cmp.Or(dr.Location, dr.ID), label: label(dr.Location, dr.Model, size(dr.CapacityMiB*1024*1024)), state: s}
			if u := dr.SSDEnduranceUtilizationPercentage; u != nil {
				left := 100 - *u
				it.life = &left
			}
			out = append(out, it)
		}
		vols, err := c.members(ctx, volsPath)
		if err != nil {
			continue
		}
		for _, raw := range vols {
			var v struct {
				ID                 string `json:"Id"`
				LogicalDriveNumber int    `json:"LogicalDriveNumber"`
				LogicalDriveName   string `json:"LogicalDriveName"`
				Raid               string `json:"Raid"`
				CapacityMiB        float64
				Status             status `json:"Status"`
			}
			if json.Unmarshal(raw, &v) != nil {
				continue
			}
			if s, present := health(v.Status); present {
				name := fmt.Sprintf("LD %d", v.LogicalDriveNumber)
				raid := ""
				if v.Raid != "" {
					raid = "RAID " + v.Raid
				}
				out = append(out, item{key: "volume/" + ctl.ID + "-" + cmp.Or(v.ID, name), label: label(name, v.LogicalDriveName, raid, size(v.CapacityMiB*1024*1024)), state: s})
			}
		}
	}
	return out, nil
}

// refID liest @odata.id eines Eintrags.
func refID(raw json.RawMessage) string {
	var r ref
	_ = json.Unmarshal(raw, &r)
	return r.ID
}
