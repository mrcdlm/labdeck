// Package pve überwacht Proxmox VE: Auslastung der Nodes (CPU, RAM, Root-
// Platte), je VM und Container Status, CPU, RAM, Platte und Netzwerk, die
// Belegung der Storages, SMART-Status der Platten, ZFS-Pools und die
// Sicherungen (Backup-Storages, vzdump-Läufe).
//
// API (geprüft gegen das API-Schema von PVE 9):
//   - GET /api2/json/cluster/resources                  Nodes, Gäste, Storages
//   - GET /api2/json/nodes/{node}/disks/list            Platten mit SMART-Status
//   - GET /api2/json/nodes/{node}/disks/zfs             ZFS-Pools
//   - GET /api2/json/nodes/{node}/tasks?typefilter=vzdump  Backup-Läufe
//   - GET /api2/json/nodes/{node}/storage/{s}/content?content=backup
//   - GET /api2/json/version
//
// Anmeldung per API-Token; die Rolle PVEAuditor auf / genügt.
package pve

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
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

const Type = "pve"

const (
	defaultWarnPct     = 80
	defaultErrorPct    = 90
	defaultWearoutWarn = 20 // SMART: Warnung bei Restlebensdauer unter …
	wearoutError       = 5
)

type Integration struct {
	now func() time.Time
}

func New() *Integration { return &Integration{now: time.Now} }

func (*Integration) Type() string { return Type }

func (*Integration) Schema() integrations.ConfigSchema {
	type T = integrations.Text
	return integrations.ConfigSchema{
		Title: T{DE: "Proxmox VE", EN: "Proxmox VE"},
		Description: T{
			DE: "Auslastung der Nodes und je VM/Container, Storages, SMART, ZFS und Sicherungen.",
			EN: "Load of the nodes and per VM/container, storages, SMART, ZFS and backups.",
		},
		Icon: "server",
		Docs: "https://pve.proxmox.com/wiki/User_Management#pveum_tokens",
		Fields: []integrations.Field{
			{Key: "base_url", Type: integrations.FieldURL, Required: true, Default: "https://pve.lan:8006",
				Label: T{DE: "Adresse", EN: "Address"},
				Help: T{
					DE: "Adresse eines Nodes mit Port 8006. Bei selbst signiertem Zertifikat unter „Erweitert“ die TLS-Prüfung ausschalten.",
					EN: "Address of a node with port 8006. With a self-signed certificate turn off TLS verification under \"Advanced\".",
				},
				Placeholder: "https://pve.lan:8006"},
			{Key: "token_id", Type: integrations.FieldString, Required: true,
				Label: T{DE: "Token-ID", EN: "Token ID"},
				Help: T{
					DE: "Format benutzer@realm!tokenname. Das Token braucht die Rolle PVEAuditor auf / (bei Privilege Separation für das Token selbst).",
					EN: "Format user@realm!tokenname. The token needs the role PVEAuditor on / (with privilege separation for the token itself).",
				},
				Placeholder: "labdeck@pve!dashboard"},
			{Key: "token_secret", Type: integrations.FieldSecret, Required: true,
				Label: T{DE: "Token-Secret", EN: "Token secret"}},
			{Key: "nodes", Type: integrations.FieldString,
				Label:       T{DE: "Nodes", EN: "Nodes"},
				Help:        T{DE: "Komma-getrennt; leer = alle.", EN: "Comma-separated; empty = all."},
				Placeholder: "pve1, pve2"},
			{Key: "warn_pct", Type: integrations.FieldNumber, Default: float64(defaultWarnPct), Half: true,
				Min: integrations.Ptr(1), Max: integrations.Ptr(100),
				Label: T{DE: "Warnung ab %", EN: "Warning at %"},
				Help:  T{DE: "CPU, RAM, Platte der Nodes und Storages.", EN: "CPU, RAM, disk of the nodes and storages."}},
			{Key: "error_pct", Type: integrations.FieldNumber, Default: float64(defaultErrorPct), Half: true,
				Min: integrations.Ptr(1), Max: integrations.Ptr(100),
				Label: T{DE: "Fehler ab %", EN: "Error at %"}},
			{Key: "guest_thresholds", Type: integrations.FieldBool, Default: false,
				Label: T{DE: "Schwellwerte auch für VMs/Container", EN: "Thresholds for VMs/containers too"},
				Help: T{
					DE: "Aus: Gäste werden nur angezeigt. VMs belegen ihren RAM oft fast ganz (Cache) – das wäre sonst ein Dauerproblem.",
					EN: "Off: guests are only shown. VMs often use almost all of their RAM (cache) – this would otherwise be a permanent problem.",
				}},
			{Key: "wearout_warn", Type: integrations.FieldNumber, Default: float64(defaultWearoutWarn),
				Min: integrations.Ptr(0), Max: integrations.Ptr(100),
				Label: T{DE: "SMART: Warnung bei Restlebensdauer unter %", EN: "SMART: warning below remaining life %"}},
			{Key: "backups", Type: integrations.FieldBool, Default: true,
				Label: T{DE: "Sicherungen anzeigen", EN: "Show backups"},
				Help: T{
					DE: "Letzte Sicherung je VM/Container aus den Backup-Storages und die vzdump-Läufe. Ausschalten, wenn die Backups schon über die PBS-Integration kommen.",
					EN: "Last backup per VM/container from the backup storages and the vzdump runs. Turn off if the backups already come from the PBS integration.",
				}},
		},
	}
}

func header(cfg integrations.Config) http.Header {
	return http.Header{"Authorization": {"PVEAPIToken=" + strings.TrimSpace(cfg.String("token_id")) + "=" + cfg.Secret("token_secret")}}
}

func get[T any](ctx context.Context, cfg integrations.Config, path string, out *T) error {
	var env struct {
		Data *T `json:"data"`
	}
	if err := integrations.GetJSON(ctx, cfg.HTTP, cfg.BaseURL("base_url")+"/api2/json"+path, header(cfg), &env); err != nil {
		return err
	}
	if env.Data == nil {
		return i18n.E("pve.unexpected")
	}
	*out = *env.Data
	return nil
}

// flexBool liest 0/1 und true/false.
type flexBool bool

func (b *flexBool) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	*b = flexBool(s == "1" || s == "true")
	return nil
}

type resource struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Node     string   `json:"node"`
	Status   string   `json:"status"`
	Name     string   `json:"name"`
	VMID     int      `json:"vmid"`
	CPU      float64  `json:"cpu"`
	MaxCPU   float64  `json:"maxcpu"`
	Mem      int64    `json:"mem"`
	MaxMem   int64    `json:"maxmem"`
	Disk     int64    `json:"disk"`
	MaxDisk  int64    `json:"maxdisk"`
	NetIn    int64    `json:"netin"`
	NetOut   int64    `json:"netout"`
	Uptime   int64    `json:"uptime"`
	Template flexBool `json:"template"`
	Tags     string   `json:"tags"`
	Storage  string   `json:"storage"`
	Content  string   `json:"content"`
	Shared   flexBool `json:"shared"`
}

type settings struct {
	pct          integrations.ValueSpec
	guestPct     bool
	wearoutWarn  float64
	backups      bool
	nodes        []string
	now          time.Time
	backupWindow time.Time
}

func readSettings(cfg integrations.Config, now time.Time) settings {
	warn, fail := float64(defaultWarnPct), float64(defaultErrorPct)
	if cfg.Has("warn_pct") {
		warn = cfg.Float("warn_pct")
	}
	if cfg.Has("error_pct") {
		fail = cfg.Float("error_pct")
	}
	s := settings{pct: integrations.ValueSpec{Warn: &warn, Fail: &fail}, guestPct: cfg.Bool("guest_thresholds"),
		wearoutWarn: defaultWearoutWarn, backups: !cfg.Has("backups") || cfg.Bool("backups"), now: now,
		backupWindow: now.Add(-uptime.MaxRange)}
	if cfg.Has("wearout_warn") {
		s.wearoutWarn = cfg.Float("wearout_warn")
	}
	for _, n := range strings.Split(cfg.String("nodes"), ",") {
		if n = strings.TrimSpace(n); n != "" {
			s.nodes = append(s.nodes, n)
		}
	}
	return s
}

// clusterResources liefert die Ressourcen der gewählten Nodes.
func clusterResources(ctx context.Context, cfg integrations.Config, s settings) ([]resource, error) {
	var all []resource
	if err := get(ctx, cfg, "/cluster/resources", &all); err != nil {
		return nil, err
	}
	var out []resource
	nodes := 0
	for _, r := range all {
		if len(s.nodes) > 0 && !slices.Contains(s.nodes, r.Node) {
			continue
		}
		if r.Type == "node" {
			nodes++
		}
		out = append(out, r)
	}
	if nodes == 0 {
		if len(s.nodes) > 0 {
			return nil, i18n.E("pve.node_missing", "nodes", strings.Join(s.nodes, ", "))
		}
		return nil, i18n.E("pve.no_nodes")
	}
	return out, nil
}

func (i *Integration) Test(ctx context.Context, cfg integrations.Config) error {
	_, err := clusterResources(ctx, cfg, readSettings(cfg, i.now()))
	return err
}

func pct(used, total int64) (float64, bool) {
	if total <= 0 {
		return 0, false
	}
	return float64(used) / float64(total) * 100, true
}

func ptr[T any](v T) *T { return &v }

func (i *Integration) Collect(ctx context.Context, cfg integrations.Config) (integrations.Snapshot, error) {
	s := readSettings(cfg, i.now())
	list, err := clusterResources(ctx, cfg, s)
	if err != nil {
		return integrations.Snapshot{}, err
	}
	var snap integrations.Snapshot
	ok := integrations.StatusOK
	guests := map[int]resource{}
	var nodeNames []string
	var cpuSum, cpuCount float64
	var memUsed, memTotal int64
	running, total := 0, 0

	// Nodes und Gäste als Ressourcen; ihre Werte zusätzlich als Kennzahlen (Verlauf)
	for _, r := range list {
		switch r.Type {
		case "node":
			nodeNames = append(nodeNames, r.Node)
			res := integrations.Resource{ID: "node/" + r.Node, Name: r.Node, Kind: integrations.KindNode, State: r.Status}
			if r.Status != "online" {
				res.Status = integrations.StatusError
				snap.Resources = append(snap.Resources, res)
				snap.Metrics = append(snap.Metrics, integrations.Metric{Key: "res/node/" + r.Node + "/online", Name: "online",
					Group: r.Node, Unit: "okfail", Resource: res.ID, Status: integrations.StatusError})
				continue
			}
			cpuSum += r.CPU * r.MaxCPU
			cpuCount += r.MaxCPU
			memUsed += r.Mem
			memTotal += r.MaxMem
			snap.Resources = append(snap.Resources, i.usage(&snap, r, res, s.pct, true))
		case "qemu", "lxc":
			if r.Template {
				continue
			}
			guests[r.VMID] = r
			total++
			kind := integrations.KindVM
			if r.Type == "lxc" {
				kind = integrations.KindCT
			}
			res := integrations.Resource{ID: r.ID, Name: r.Name, Kind: kind, Parent: r.Node, State: r.Status,
				Status: integrations.StatusPaused, Info: map[string]string{"vmid": strconv.Itoa(r.VMID)}}
			if r.Tags != "" {
				res.Tags = strings.FieldsFunc(r.Tags, func(c rune) bool { return c == ';' || c == ',' || c == ' ' })
			}
			if r.Status != "running" {
				if r.MaxMem > 0 {
					res.MemTotal = ptr(r.MaxMem)
				}
				res.CPUs = r.MaxCPU
				snap.Resources = append(snap.Resources, res)
				continue
			}
			running++
			snap.Resources = append(snap.Resources, i.usage(&snap, r, res, s.pct, s.guestPct))
		}
	}

	// Storages: geteilte nur einmal
	seen := map[string]bool{}
	var backupStores []resource
	for _, r := range list {
		if r.Type != "storage" || seen[r.Storage] && bool(r.Shared) {
			continue
		}
		name := r.Storage
		if !r.Shared {
			name = r.Storage + " · " + r.Node
		}
		seen[r.Storage] = true
		key := "storage/" + strings.ReplaceAll(name, " · ", "/")
		if r.Status != "available" {
			snap.Metrics = append(snap.Metrics, integrations.Metric{Key: key + "/used_pct", Name: "used_pct", Group: name,
				Unit: "%", Status: integrations.StatusError})
			continue
		}
		if p, ok2 := pct(r.Disk, r.MaxDisk); ok2 {
			snap.Metrics = append(snap.Metrics,
				integrations.Metric{Key: key + "/used_pct", Name: "used_pct", Group: name, Value: p, Unit: "%", Status: s.pct.Status(p)},
				integrations.Metric{Key: key + "/used", Name: "used", Group: name, Value: float64(r.Disk), Unit: "bytes", Status: ok},
				integrations.Metric{Key: key + "/total", Name: "total", Group: name, Value: float64(r.MaxDisk), Unit: "bytes", Status: ok},
			)
		}
		if strings.Contains(r.Content, "backup") {
			backupStores = append(backupStores, r)
		}
	}

	// Platten (SMART) und ZFS: ohne Rechte (403) einfach weglassen
	var smartBad int
	for _, n := range nodeNames {
		if !slices.ContainsFunc(snap.Resources, func(r integrations.Resource) bool { return r.ID == "node/"+n && r.State == "online" }) {
			continue
		}
		smartBad += i.disks(ctx, cfg, &snap, n, s)
		i.zfs(ctx, cfg, &snap, n, s)
	}

	if s.backups {
		if err := i.backups(ctx, cfg, &snap, backupStores, guests, nodeNames, s); err != nil {
			return integrations.Snapshot{}, err
		}
	}

	// Hauptkennzahlen vorne (Kachel: „2 Nodes · 7 Gäste aktiv · CPU 12 %“)
	main := []integrations.Metric{
		{Key: "nodes", Name: "nodes", Value: float64(len(nodeNames)), Status: ok},
		{Key: "guests_running", Name: "guests_running", Value: float64(running), Status: ok},
	}
	if cpuCount > 0 {
		main = append(main, integrations.Metric{Key: "cpu", Name: "cpu", Value: cpuSum / cpuCount * 100, Unit: "%", Status: ok})
	}
	if p, ok2 := pct(memUsed, memTotal); ok2 {
		main = append(main, integrations.Metric{Key: "mem_pct", Name: "mem_pct", Value: p, Unit: "%", Status: ok})
	}
	main = append(main, integrations.Metric{Key: "guests", Name: "guests", Value: float64(total), Status: ok})
	if smartBad > 0 {
		main = append(main, integrations.Metric{Key: "smart_failed", Name: "smart_failed", Value: float64(smartBad),
			Status: integrations.StatusError, Derived: true})
	}
	snap.Metrics = append(main, snap.Metrics...)

	var v struct {
		Version string `json:"version"`
	}
	if err := get(ctx, cfg, "/version", &v); err == nil && v.Version != "" {
		snap.Info = map[string]string{"version": "v" + v.Version}
	}
	return snap, nil
}

// usage füllt CPU, RAM, Platte und Netzwerk einer laufenden Ressource und legt
// die Kennzahlen für den Verlauf an; thresholds: Status nach Schwellwerten.
func (i *Integration) usage(snap *integrations.Snapshot, r resource, res integrations.Resource, spec integrations.ValueSpec, thresholds bool) integrations.Resource {
	res.Status = integrations.StatusOK
	res.CPU = ptr(r.CPU * 100)
	res.CPUs = r.MaxCPU
	res.Uptime = r.Uptime
	if r.MaxMem > 0 {
		res.MemUsed, res.MemTotal = ptr(r.Mem), ptr(r.MaxMem)
	}
	// VMs ohne Gast-Agent melden keine belegte Platte (0) – dann nur die Größe
	if r.MaxDisk > 0 {
		res.DiskTotal = ptr(r.MaxDisk)
		if r.Disk > 0 {
			res.DiskUsed = ptr(r.Disk)
		}
	}
	if r.Type != "node" {
		res.NetIn, res.NetOut = ptr(r.NetIn), ptr(r.NetOut)
	}
	status := func(v float64) integrations.Status {
		if !thresholds {
			return integrations.StatusOK
		}
		return spec.Status(v)
	}
	p := "res/" + res.ID + "/"
	add := func(name string, v float64) {
		st := status(v)
		res.Status = integrations.Worst(res.Status, st)
		snap.Metrics = append(snap.Metrics, integrations.Metric{Key: p + name, Name: name, Group: res.Name,
			Value: v, Unit: "%", Status: st, Resource: res.ID})
	}
	add("cpu", r.CPU*100)
	if v, ok := pct(r.Mem, r.MaxMem); ok {
		add("mem_pct", v)
	}
	if res.DiskUsed != nil {
		if v, ok := pct(r.Disk, r.MaxDisk); ok {
			add("disk_pct", v)
		}
	}
	return res
}

// disks legt SMART-Kennzahlen je Platte an und liefert die Anzahl defekter.
func (i *Integration) disks(ctx context.Context, cfg integrations.Config, snap *integrations.Snapshot, node string, s settings) int {
	var list []struct {
		Dev     string          `json:"devpath"`
		Model   string          `json:"model"`
		Health  string          `json:"health"`
		Wearout json.RawMessage `json:"wearout"`
	}
	if err := get(ctx, cfg, "/nodes/"+url.PathEscape(node)+"/disks/list", &list); err != nil {
		return 0
	}
	bad := 0
	for _, d := range list {
		group := node + " · " + strings.TrimSpace(d.Model+" "+d.Dev)
		key := "disk/" + node + "/" + strings.TrimPrefix(d.Dev, "/dev/") + "/"
		health := strings.ToUpper(d.Health)
		st := integrations.StatusOK
		v := 1.0
		switch health {
		case "PASSED", "OK":
		case "", "UNKNOWN":
			st = integrations.StatusUnknown
		default:
			st, v = integrations.StatusError, 0
			bad++
		}
		snap.Metrics = append(snap.Metrics, integrations.Metric{Key: key + "health", Name: "health", Group: group, Unit: "okfail",
			Value: v, Status: st})
		if w, err := strconv.ParseFloat(strings.Trim(string(d.Wearout), `"`), 64); err == nil {
			spec := integrations.ValueSpec{Direction: integrations.DirectionBelow, Warn: &s.wearoutWarn, Fail: ptr(float64(wearoutError))}
			snap.Metrics = append(snap.Metrics, integrations.Metric{Key: key + "wearout", Name: "wearout", Group: group,
				Value: w, Unit: "%", Status: spec.Status(w)})
		}
	}
	return bad
}

// zfs legt je Pool Zustand und Belegung an.
func (i *Integration) zfs(ctx context.Context, cfg integrations.Config, snap *integrations.Snapshot, node string, s settings) {
	var pools []struct {
		Name   string `json:"name"`
		Health string `json:"health"`
		Size   int64  `json:"size"`
		Alloc  int64  `json:"alloc"`
	}
	if err := get(ctx, cfg, "/nodes/"+url.PathEscape(node)+"/disks/zfs", &pools); err != nil {
		return
	}
	for _, p := range pools {
		group := node + " · " + p.Name
		key := "zfs/" + node + "/" + p.Name + "/"
		st, v := integrations.StatusOK, 1.0
		if !strings.EqualFold(p.Health, "ONLINE") {
			st, v = integrations.StatusError, 0
		}
		snap.Metrics = append(snap.Metrics, integrations.Metric{Key: key + "zfs_health", Name: "zfs_health", Group: group, Unit: "okfail",
			Value: v, Status: st})
		if used, ok := pct(p.Alloc, p.Size); ok {
			snap.Metrics = append(snap.Metrics, integrations.Metric{Key: key + "used_pct", Name: "used_pct", Group: group,
				Value: used, Unit: "%", Status: s.pct.Status(used)})
		}
	}
}

type backupFile struct {
	VMID         int    `json:"vmid"`
	CTime        int64  `json:"ctime"`
	Size         *int64 `json:"size"`
	Verification *struct {
		State string `json:"state"`
	} `json:"verification"`
}

// backups: letzte Sicherung je VM/CT aus den Backup-Storages und je Node der
// letzte vzdump-Lauf.
func (i *Integration) backups(ctx context.Context, cfg integrations.Config, snap *integrations.Snapshot, stores []resource,
	guests map[int]resource, nodes []string, s settings) error {
	for _, st := range stores {
		var files []backupFile
		path := "/nodes/" + url.PathEscape(st.Node) + "/storage/" + url.PathEscape(st.Storage) + "/content?content=backup"
		if err := get(ctx, cfg, path, &files); err != nil {
			continue // Storage gerade nicht lesbar – die Kennzahl zeigt das
		}
		byVM := map[int][]backupFile{}
		for _, f := range files {
			if f.VMID > 0 && f.CTime > 0 {
				byVM[f.VMID] = append(byVM[f.VMID], f)
			}
		}
		group := st.Storage
		if !st.Shared {
			group += " · " + st.Node
		}
		for vmid, fs := range byVM {
			slices.SortFunc(fs, func(a, b backupFile) int { return cmp.Compare(a.CTime, b.CTime) })
			last := fs[len(fs)-1]
			t := time.Unix(last.CTime, 0)
			b := integrations.Backup{ID: "storage/" + group + "/" + strconv.Itoa(vmid), Name: guestName(vmid, guests),
				Group: group, LastRun: &t, Result: integrations.StatusOK, SizeBytes: last.Size}
			if last.Verification != nil {
				b.Verify = last.Verification.State
				if b.Verify == "failed" {
					b.Result = integrations.StatusError
				}
			}
			for _, f := range fs {
				at := time.Unix(f.CTime, 0)
				if at.Before(s.backupWindow) {
					continue
				}
				r := integrations.Run{At: at, Result: integrations.StatusOK}
				if f.Verification != nil && f.Verification.State == "failed" {
					r.Result = integrations.StatusError
				}
				b.Runs = append(b.Runs, r)
			}
			snap.Backups = append(snap.Backups, b)
		}
	}

	for _, n := range nodes {
		var tasks []struct {
			EndTime int64  `json:"endtime"`
			Status  string `json:"status"`
		}
		if err := get(ctx, cfg, "/nodes/"+url.PathEscape(n)+"/tasks?typefilter=vzdump&limit=200", &tasks); err != nil {
			continue
		}
		var runs []integrations.Run
		for _, t := range tasks {
			if t.EndTime == 0 || t.Status == "" {
				continue // läuft noch
			}
			runs = append(runs, integrations.Run{At: time.Unix(t.EndTime, 0), Result: taskResult(t.Status)})
		}
		if len(runs) == 0 {
			continue
		}
		slices.SortFunc(runs, func(a, b integrations.Run) int { return a.At.Compare(b.At) })
		last := runs[len(runs)-1]
		b := integrations.Backup{ID: "vzdump/" + n, Name: "vzdump", Group: n, LastRun: &last.At, Result: last.Result}
		for _, r := range runs {
			if !r.At.Before(s.backupWindow) {
				b.Runs = append(b.Runs, r)
			}
		}
		snap.Backups = append(snap.Backups, b)
	}
	slices.SortFunc(snap.Backups, func(a, b integrations.Backup) int {
		return cmp.Or(strings.Compare(a.Group, b.Group), strings.Compare(a.Name, b.Name))
	})
	return nil
}

// taskResult: „OK“, „WARNINGS: n“ oder eine Fehlermeldung.
func taskResult(status string) integrations.Status {
	switch {
	case status == "OK":
		return integrations.StatusOK
	case strings.HasPrefix(status, "WARNINGS"):
		return integrations.StatusWarning
	}
	return integrations.StatusError
}

func guestName(vmid int, guests map[int]resource) string {
	g, ok := guests[vmid]
	if !ok {
		return strconv.Itoa(vmid)
	}
	kind := "vm"
	if g.Type == "lxc" {
		kind = "ct"
	}
	return fmt.Sprintf("%s/%d (%s)", kind, vmid, g.Name)
}
