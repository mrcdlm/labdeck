package pve

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/integrations/pve/pvetest"
)

const token = "labdeck@pve!dash"

func setup(t *testing.T) (*pvetest.Fake, *httptest.Server) {
	t.Helper()
	fake := pvetest.New(token + "=geheim").Demo()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return fake, srv
}

func cfg(t *testing.T, url string, extra map[string]any, secret string, timeout time.Duration) integrations.Config {
	t.Helper()
	in := map[string]any{"base_url": url, "token_id": token}
	for k, v := range extra {
		in[k] = v
	}
	values, msgs := New().Schema().Normalize(in, i18n.EN)
	if len(msgs) != 0 {
		t.Fatal(msgs)
	}
	return integrations.Config{Values: values, Secrets: map[string]string{"token_secret": secret},
		HTTP: integrations.NewHTTPClient(timeout, true), Timeout: timeout}
}

func TestCollect(t *testing.T) {
	_, srv := setup(t)
	snap, err := New().Collect(context.Background(), cfg(t, srv.URL+"/", nil, "geheim", time.Second))
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]integrations.Metric{}
	for _, x := range snap.Metrics {
		m[x.Key] = x
	}
	res := map[string]integrations.Resource{}
	for _, r := range snap.Resources {
		res[r.ID] = r
	}

	// Übersicht
	if m["nodes"].Value != 2 || m["guests_running"].Value != 7 || m["guests"].Value != 8 || m["smart_failed"].Value != 1 ||
		!m["smart_failed"].Derived {
		t.Errorf("Übersicht: %+v %+v %+v", m["nodes"], m["guests_running"], m["smart_failed"])
	}
	// CPU des Clusters gewichtet nach Kernen: (0.18·16 + 0.06·4) / 20
	if c := m["cpu"].Value; c < 15.5 || c > 15.7 {
		t.Errorf("Cluster-CPU: %v", c)
	}
	if snap.Info["version"] != "v9.0.10" {
		t.Errorf("Version: %v", snap.Info)
	}

	// Node mit Schwellwerten (RAM 51/64 = 79,7 % → ok)
	n := res["node/pve1"]
	if n.Kind != integrations.KindNode || n.State != "online" || *n.CPU != 18 || n.CPUs != 16 || *n.MemTotal != 64e9 ||
		n.Status != integrations.StatusOK {
		t.Errorf("Node: %+v", n)
	}
	if mm := m["res/node/pve1/mem_pct"]; mm.Resource != "node/pve1" || mm.Group != "pve1" || mm.Unit != "%" {
		t.Errorf("Node-Kennzahl: %+v", mm)
	}

	// Gäste: ohne Gast-Schwellwerte ok, gestoppte grau, VMs ohne belegte Platte
	nc := res["qemu/102"]
	if nc.Kind != integrations.KindVM || nc.Parent != "pve1" || nc.Status != integrations.StatusOK || nc.DiskUsed != nil ||
		*nc.DiskTotal != 200e9 || *nc.NetIn == 0 || nc.Info["vmid"] != "102" {
		t.Errorf("VM: %+v", nc)
	}
	if im := res["lxc/103"]; im.Kind != integrations.KindCT || *im.DiskUsed != 41e9 || len(im.Tags) != 1 || im.Tags[0] != "media" {
		t.Errorf("CT: %+v", im)
	}
	if w := res["qemu/105"]; w.State != "stopped" || w.Status != integrations.StatusPaused || w.CPU != nil {
		t.Errorf("gestoppt: %+v", w)
	}
	if _, ok := m["res/qemu/102/disk_pct"]; ok {
		t.Error("VM ohne Gast-Agent hat disk_pct")
	}

	// Storages: geteilter nur einmal, lokale je Node
	if s := m["storage/nas-backup/used_pct"]; s.Group != "nas-backup" || s.Value != 85 || s.Status != integrations.StatusWarning {
		t.Errorf("NFS: %+v", s)
	}
	if _, ok := m["storage/local/pve2/used_pct"]; !ok {
		t.Error("lokales Storage je Node fehlt")
	}

	// SMART und ZFS
	if d := m["disk/pve1/sdb/health"]; d.Status != integrations.StatusError || d.Group != "pve1 · WDC WD80EFZZ /dev/sdb" {
		t.Errorf("SMART: %+v", d)
	}
	if w := m["disk/pve2/sda/wearout"]; w.Value != 12 || w.Status != integrations.StatusWarning {
		t.Errorf("Wearout: %+v", w)
	}
	if _, ok := m["disk/pve1/sda/wearout"]; ok {
		t.Error("Wearout N/A als Kennzahl")
	}
	if z := m["zfs/pve1/tank/zfs_health"]; z.Status != integrations.StatusError {
		t.Errorf("ZFS: %+v", z)
	}

	// Sicherungen: je VM aus dem Backup-Storage, je Node die vzdump-Läufe
	b := map[string]integrations.Backup{}
	for _, x := range snap.Backups {
		b[x.ID] = x
	}
	if ha := b["storage/nas-backup/100"]; ha.Name != "vm/100 (homeassistant)" || len(ha.Runs) != 5 || time.Since(*ha.LastRun) > 10*time.Hour {
		t.Errorf("Backup HA: %+v", ha)
	}
	if u := b["storage/local · pve2/200"]; u.Name != "ct/200 (unifi)" {
		t.Errorf("lokales Backup: %+v", b)
	}
	if v := b["vzdump/pve1"]; v.Result != integrations.StatusOK || len(v.Runs) != 3 || v.Runs[0].Result != integrations.StatusError {
		t.Errorf("vzdump pve1: %+v", v)
	}
	if v := b["vzdump/pve2"]; v.Result != integrations.StatusWarning {
		t.Errorf("vzdump pve2: %+v", v)
	}
	if _, ok := b["storage/nas-backup/105"]; ok {
		t.Error("win11 hat kein Backup")
	}
}

func TestOptions(t *testing.T) {
	_, srv := setup(t)
	ctx := context.Background()
	snap, err := New().Collect(ctx, cfg(t, srv.URL, map[string]any{"nodes": "pve2", "backups": false,
		"guest_thresholds": true, "warn_pct": 60.0, "error_pct": 95.0}, "geheim", time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Backups) != 0 {
		t.Errorf("Backups trotz aus: %d", len(snap.Backups))
	}
	for _, r := range snap.Resources {
		if r.Parent != "" && r.Parent != "pve2" || r.Kind == integrations.KindNode && r.Name != "pve2" {
			t.Errorf("Filter: %+v", r)
		}
		// monitoring: RAM 2,8/4 = 70 % ≥ 60 → Warnung (Gast-Schwellwerte an)
		if r.ID == "lxc/201" && r.Status != integrations.StatusWarning {
			t.Errorf("Gast-Schwellwert: %+v", r)
		}
	}
}

func TestErrors(t *testing.T) {
	fake, srv := setup(t)
	ctx := context.Background()
	code := func(c integrations.Config) string { return i18n.FromError(New().Test(ctx, c)).Code }

	if c := code(cfg(t, srv.URL, nil, "geheim", time.Second)); c != "" {
		t.Fatalf("ok erwartet: %s", c)
	}
	if c := code(cfg(t, srv.URL, nil, "falsch", time.Second)); c != "integration.unauthorized" {
		t.Errorf("401: %s", c)
	}
	if c := code(cfg(t, srv.URL, map[string]any{"nodes": "gibtsnicht"}, "geheim", time.Second)); c != "pve.node_missing" {
		t.Errorf("Node: %s", c)
	}
	fake.Broken(true)
	if c := code(cfg(t, srv.URL, nil, "geheim", time.Second)); c != "integration.bad_json" {
		t.Errorf("kaputtes JSON: %s", c)
	}
	fake.Broken(false)
	fake.Delay(300 * time.Millisecond)
	if c := code(cfg(t, srv.URL, nil, "geheim", 100*time.Millisecond)); c != "integration.timeout" {
		t.Errorf("Timeout: %s", c)
	}
	fake.Delay(0)

	// Token ohne Rechte: leere Liste
	fake.SetNodes()
	if c := code(cfg(t, srv.URL, nil, "geheim", time.Second)); c != "pve.no_nodes" {
		t.Errorf("keine Nodes: %s", c)
	}
	// Platten/Tasks ohne Rechte (403): Abfrage klappt trotzdem
	fake.SetNodes(pvetest.Node{Name: "pve1", Online: true, CPUs: 4, Max: 1, MaxDisk: 1, Forbidden: true})
	if _, err := New().Collect(ctx, cfg(t, srv.URL, nil, "geheim", time.Second)); err != nil {
		t.Errorf("403 bei Platten: %v", err)
	}
	// Node offline
	fake.SetNodes(pvetest.Node{Name: "pve1", Online: false})
	snap, _ := New().Collect(ctx, cfg(t, srv.URL, nil, "geheim", time.Second))
	if len(snap.Resources) == 0 || snap.Resources[0].Status != integrations.StatusError {
		t.Errorf("offline: %+v", snap.Resources)
	}
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"x":1}`)) }))
	defer other.Close()
	if c := code(cfg(t, other.URL, nil, "geheim", time.Second)); c != "pve.unexpected" {
		t.Errorf("fremd: %s", c)
	}
}
