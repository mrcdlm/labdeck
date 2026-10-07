// Package pvetest bildet die benötigten Teile der Proxmox-VE-API nach
// (Cluster-Ressourcen, Storages, Backups, Platten, ZFS, vzdump-Tasks) – für
// Tests und den Entwicklungs-Mock.
package pvetest

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Guest ist eine VM (qemu) oder ein Container (lxc).
type Guest struct {
	Type      string // qemu, lxc
	VMID      int
	Name      string
	Node      string
	Running   bool
	CPU       float64 // Anteil 0..1 der zugewiesenen Kerne
	CPUs      int
	Mem, Max  int64
	Disk      int64 // bei VMs meist 0 (ohne Gast-Agent)
	MaxDisk   int64
	Tags      string
	BackupAgo []time.Duration // Sicherungen auf dem Backup-Storage
}

// Node ist ein PVE-Node.
type Node struct {
	Name      string
	Online    bool
	CPU       float64
	CPUs      int
	Mem, Max  int64
	Disk      int64
	MaxDisk   int64
	Disks     []Disk
	Pools     []Pool
	Tasks     []Task
	Forbidden bool // Platten/Tasks ohne Rechte (403)
}

// Disk ist eine physische Platte (SMART).
type Disk struct {
	Dev, Model, Health string
	Wearout            any // Zahl oder "N/A"
	Size               int64
}

// Pool ist ein ZFS-Pool.
type Pool struct {
	Name, Health      string
	Size, Alloc, Free int64
}

// Task ist ein vzdump-Lauf.
type Task struct {
	Ago    time.Duration
	Status string // OK, WARNINGS: 1, Fehlermeldung
}

// Storage ist ein Storage (shared = auf allen Nodes gleich).
type Storage struct {
	Name, Type, Content string
	Shared              bool
	Used, Total         int64
	Active              bool
}

type Fake struct {
	mu       sync.Mutex
	token    string // erwarteter Wert nach "PVEAPIToken="
	nodes    []Node
	guests   []Guest
	storages []Storage
	fail     int
	broken   bool
	delay    time.Duration
	requests int
	now      func() time.Time
}

// New: token im Format user@realm!tokenid=secret.
func New(token string) *Fake { return &Fake{token: token, now: time.Now} }

const (
	gb = int64(1_000_000_000)
	tb = 1000 * gb
)

// Demo: zwei Nodes mit typischen HomeLab-Gästen.
func (f *Fake) Demo() *Fake {
	day := 24 * time.Hour
	daily := func(n int, off time.Duration) []time.Duration {
		out := make([]time.Duration, n)
		for i := range out {
			out[i] = time.Duration(i)*day + off
		}
		return out
	}
	f.nodes = []Node{
		{Name: "pve1", Online: true, CPU: 0.18, CPUs: 16, Mem: 51 * gb, Max: 64 * gb, Disk: 38 * gb, MaxDisk: 100 * gb,
			Disks: []Disk{
				{Dev: "/dev/nvme0n1", Model: "Samsung SSD 980 PRO 1TB", Health: "PASSED", Wearout: 96, Size: tb},
				{Dev: "/dev/sda", Model: "WDC WD80EFZZ", Health: "PASSED", Wearout: "N/A", Size: 8 * tb},
				{Dev: "/dev/sdb", Model: "WDC WD80EFZZ", Health: "FAILED", Wearout: "N/A", Size: 8 * tb},
			},
			Pools: []Pool{{Name: "tank", Health: "DEGRADED", Size: 16 * tb, Alloc: 9 * tb, Free: 7 * tb}},
			Tasks: []Task{{Ago: 9 * time.Hour, Status: "OK"}, {Ago: 33 * time.Hour, Status: "OK"},
				{Ago: 57 * time.Hour, Status: "job errors"}}},
		{Name: "pve2", Online: true, CPU: 0.06, CPUs: 4, Mem: 7 * gb, Max: 16 * gb, Disk: 12 * gb, MaxDisk: 60 * gb,
			Disks: []Disk{{Dev: "/dev/sda", Model: "Crucial MX500 500GB", Health: "PASSED", Wearout: 12, Size: 500 * gb}},
			Tasks: []Task{{Ago: 8 * time.Hour, Status: "WARNINGS: 1"}}},
	}
	f.guests = []Guest{
		{Type: "qemu", VMID: 100, Name: "homeassistant", Node: "pve1", Running: true, CPU: 0.07, CPUs: 2,
			Mem: 3 * gb, Max: 4 * gb, MaxDisk: 32 * gb, Tags: "smarthome", BackupAgo: daily(5, 9*time.Hour)},
		{Type: "lxc", VMID: 101, Name: "pihole", Node: "pve1", Running: true, CPU: 0.01, CPUs: 1,
			Mem: 180_000_000, Max: 512_000_000, Disk: 2 * gb, MaxDisk: 8 * gb, Tags: "dns", BackupAgo: daily(5, 9*time.Hour)},
		{Type: "qemu", VMID: 102, Name: "nextcloud", Node: "pve1", Running: true, CPU: 0.34, CPUs: 4,
			Mem: 7_600_000_000, Max: 8 * gb, MaxDisk: 200 * gb, BackupAgo: daily(3, 57*time.Hour)},
		{Type: "lxc", VMID: 103, Name: "immich", Node: "pve1", Running: true, CPU: 0.52, CPUs: 4,
			Mem: 5 * gb, Max: 8 * gb, Disk: 41 * gb, MaxDisk: 50 * gb, Tags: "media", BackupAgo: daily(5, 9*time.Hour)},
		{Type: "lxc", VMID: 104, Name: "paperless", Node: "pve1", Running: true, CPU: 0.03, CPUs: 2,
			Mem: 1_200_000_000, Max: 2 * gb, Disk: 9 * gb, MaxDisk: 20 * gb, BackupAgo: daily(5, 9*time.Hour)},
		{Type: "qemu", VMID: 105, Name: "win11", Node: "pve1", Running: false, CPUs: 4, Max: 8 * gb, MaxDisk: 128 * gb},
		{Type: "lxc", VMID: 200, Name: "unifi", Node: "pve2", Running: true, CPU: 0.04, CPUs: 2,
			Mem: 1_500_000_000, Max: 2 * gb, Disk: 6 * gb, MaxDisk: 16 * gb, BackupAgo: daily(2, 8*time.Hour)},
		{Type: "lxc", VMID: 201, Name: "monitoring", Node: "pve2", Running: true, CPU: 0.11, CPUs: 2,
			Mem: 2_800_000_000, Max: 4 * gb, Disk: 14 * gb, MaxDisk: 30 * gb, BackupAgo: daily(2, 8*time.Hour)},
	}
	f.storages = []Storage{
		{Name: "local", Type: "dir", Content: "iso,vztmpl,backup", Used: 38 * gb, Total: 100 * gb, Active: true},
		{Name: "local-lvm", Type: "lvmthin", Content: "images,rootdir", Used: 610 * gb, Total: 800 * gb, Active: true},
		{Name: "nas-backup", Type: "nfs", Content: "backup", Shared: true, Used: 5_100 * gb, Total: 6 * tb, Active: true},
	}
	return f
}

// SetGuestCPU ändert die Auslastung eines Gasts (für Live-Tests).
func (f *Fake) SetGuestCPU(vmid int, cpu float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.guests {
		if f.guests[i].VMID == vmid {
			f.guests[i].CPU = cpu
		}
	}
}

// SetNodes ersetzt die Nodes (Tests).
func (f *Fake) SetNodes(n ...Node) { f.mu.Lock(); f.nodes = n; f.mu.Unlock() }

func (f *Fake) Fail(status int)       { f.mu.Lock(); f.fail = status; f.mu.Unlock() }
func (f *Fake) Broken(b bool)         { f.mu.Lock(); f.broken = b; f.mu.Unlock() }
func (f *Fake) Delay(d time.Duration) { f.mu.Lock(); f.delay = d; f.mu.Unlock() }
func (f *Fake) Requests() int         { f.mu.Lock(); defer f.mu.Unlock(); return f.requests }

func (f *Fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests++
	fail, broken, delay := f.fail, f.broken, f.delay
	authOK := r.Header.Get("Authorization") == "PVEAPIToken="+f.token
	f.mu.Unlock()
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
	}
	if fail != 0 {
		http.Error(w, http.StatusText(fail), fail)
		return
	}
	if !authOK {
		w.WriteHeader(http.StatusUnauthorized) // PVE: 401 ohne Body
		return
	}
	if broken {
		w.Write([]byte(`{"data":[{"type":`))
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api2/json")
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case path == "/version":
		data(w, map[string]string{"version": "9.0.10", "release": "9.0", "repoid": "abc123"})
	case path == "/cluster/resources":
		f.resources(w)
	case strings.HasPrefix(path, "/nodes/"):
		f.node(w, r, strings.Split(strings.TrimPrefix(path, "/nodes/"), "/"))
	default:
		http.Error(w, `{"data":null}`, http.StatusNotImplemented)
	}
}

func (f *Fake) resources(w http.ResponseWriter) {
	out := []map[string]any{}
	for _, n := range f.nodes {
		m := map[string]any{"id": "node/" + n.Name, "type": "node", "node": n.Name, "status": "offline"}
		if n.Online {
			m["status"], m["cpu"], m["maxcpu"], m["mem"], m["maxmem"], m["disk"], m["maxdisk"], m["uptime"] =
				"online", n.CPU, n.CPUs, n.Mem, n.Max, n.Disk, n.MaxDisk, 1_234_567
		}
		out = append(out, m)
		for _, s := range f.storages {
			out = append(out, map[string]any{"id": "storage/" + n.Name + "/" + s.Name, "type": "storage", "node": n.Name,
				"storage": s.Name, "plugintype": s.Type, "content": s.Content, "shared": boolInt(s.Shared),
				"status": map[bool]string{true: "available", false: "unknown"}[s.Active], "disk": s.Used, "maxdisk": s.Total})
		}
	}
	for _, g := range f.guests {
		m := map[string]any{"id": g.Type + "/" + itoa(g.VMID), "type": g.Type, "vmid": g.VMID, "name": g.Name,
			"node": g.Node, "status": "stopped", "maxcpu": g.CPUs, "maxmem": g.Max, "maxdisk": g.MaxDisk, "template": 0,
			"cpu": 0, "mem": 0, "disk": 0, "netin": 0, "netout": 0, "uptime": 0}
		if g.Tags != "" {
			m["tags"] = g.Tags
		}
		if g.Running {
			m["status"], m["cpu"], m["mem"], m["disk"], m["netin"], m["netout"], m["uptime"] =
				"running", g.CPU, g.Mem, g.Disk, 12_000_000_000, 3_400_000_000, 864_000
		}
		out = append(out, m)
	}
	data(w, out)
}

func (f *Fake) node(w http.ResponseWriter, r *http.Request, parts []string) {
	var n *Node
	for i := range f.nodes {
		if f.nodes[i].Name == parts[0] {
			n = &f.nodes[i]
		}
	}
	if n == nil || len(parts) < 2 {
		http.Error(w, `{"data":null}`, http.StatusNotFound)
		return
	}
	now := f.now()
	rest := strings.Join(parts[1:], "/")
	if n.Forbidden && (rest == "disks/list" || rest == "disks/zfs" || rest == "tasks") {
		http.Error(w, `{"data":null}`, http.StatusForbidden)
		return
	}
	switch {
	case rest == "disks/list":
		out := []map[string]any{}
		for _, d := range n.Disks {
			out = append(out, map[string]any{"devpath": d.Dev, "model": d.Model, "health": d.Health, "wearout": d.Wearout,
				"size": d.Size, "gpt": 1, "mounted": 0, "osdid": -1, "osdid-list": nil})
		}
		data(w, out)
	case rest == "disks/zfs":
		out := []map[string]any{}
		for _, p := range n.Pools {
			out = append(out, map[string]any{"name": p.Name, "health": p.Health, "size": p.Size, "alloc": p.Alloc,
				"free": p.Free, "frag": 3, "dedup": 1.0})
		}
		data(w, out)
	case rest == "tasks":
		if r.URL.Query().Get("typefilter") != "vzdump" {
			data(w, []any{})
			return
		}
		out := []map[string]any{}
		for _, t := range n.Tasks {
			end := now.Add(-t.Ago)
			out = append(out, map[string]any{"type": "vzdump", "id": "", "node": n.Name, "user": "root@pam",
				"upid": "UPID:" + n.Name + ":00001234:00005678:00000000:vzdump::root@pam:", "pid": 1, "pstart": 1,
				"starttime": end.Add(-20 * time.Minute).Unix(), "endtime": end.Unix(), "status": t.Status})
		}
		data(w, out)
	case strings.HasPrefix(rest, "storage/") && strings.HasSuffix(rest, "/content"):
		name := strings.TrimSuffix(strings.TrimPrefix(rest, "storage/"), "/content")
		out := []map[string]any{}
		for _, s := range f.storages {
			if s.Name != name || !strings.Contains(s.Content, "backup") {
				continue
			}
			for _, g := range f.guests {
				if !s.Shared && g.Node != n.Name {
					continue
				}
				// NFS-Backups gehen auf nas-backup, lokale auf local (nur pve2)
				if (s.Name == "local") != (g.Node == "pve2") {
					continue
				}
				for _, ago := range g.BackupAgo {
					t := now.Add(-ago)
					kind := map[string]string{"qemu": "qemu", "lxc": "lxc"}[g.Type]
					out = append(out, map[string]any{"volid": s.Name + ":backup/vzdump-" + kind + "-" + itoa(g.VMID) + "-" +
						t.UTC().Format("2006_01_02-15_04_05") + ".vma.zst", "vmid": g.VMID, "ctime": t.Unix(),
						"size": g.MaxDisk / 4, "format": "vma.zst", "content": "backup", "notes": g.Name})
				}
			}
		}
		data(w, out)
	default:
		http.Error(w, `{"data":null}`, http.StatusNotImplemented)
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func data(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": v})
}
