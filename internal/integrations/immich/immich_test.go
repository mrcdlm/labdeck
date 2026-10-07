package immich

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/integrations/immich/immichtest"
)

func setup(t *testing.T) (*immichtest.Fake, *httptest.Server) {
	t.Helper()
	fake := immichtest.New("admin-key", "user-key", immichtest.Demo()...)
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return fake, srv
}

func cfg(url, key string, timeout time.Duration) integrations.Config {
	values, msgs := New().Schema().Normalize(map[string]any{"base_url": url}, i18n.EN)
	if len(msgs) != 0 {
		panic(msgs)
	}
	return integrations.Config{Values: values, Secrets: map[string]string{"api_key": key},
		HTTP: integrations.NewHTTPClient(timeout, true), Timeout: timeout}
}

func code(err error) string { return i18n.FromError(err).Code }

func TestCollect(t *testing.T) {
	_, srv := setup(t)
	snap, err := New().Collect(context.Background(), cfg(srv.URL+"/", "admin-key", time.Second))
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]integrations.Metric{}
	for _, x := range snap.Metrics {
		m[x.Key] = x
	}
	if m["photos"].Value != 48_213 || m["videos"].Value != 1_204 || m["usage"].Value != 412e9 ||
		m["usage"].Unit != "bytes" || m["users"].Value != 2 || m["photos"].Group != "" {
		t.Fatalf("Gesamt: %+v", m)
	}
	marco := "user/6f1c0b52-0d1e-4e6a-9d0e-000000000001/"
	anna := "user/6f1c0b52-0d1e-4e6a-9d0e-000000000002/"
	if m[marco+"photos"].Value != 31_502 || m[marco+"photos"].Group != "Marco" || m[marco+"usage"].Value != 310e9 {
		t.Errorf("Marco: %+v", m[marco+"photos"])
	}
	if _, ok := m[marco+"quota"]; ok {
		t.Error("Marco hat keine Quota")
	}
	// Anna: 102 GB von 200 GB
	if m[anna+"quota"].Value != 200e9 || m[anna+"quota_used"].Value != 51 || m[anna+"usage"].Status != integrations.StatusOK {
		t.Errorf("Anna: %+v %+v", m[anna+"quota"], m[anna+"quota_used"])
	}
	if snap.Info["version"] != "v1.140.1" {
		t.Errorf("Version: %v", snap.Info)
	}
}

func TestQuotaStatus(t *testing.T) {
	q := int64(100)
	fake := immichtest.New("k", "", immichtest.User{ID: "a", Name: "A", Photos: 1, UsagePhotos: 95, Quota: &q},
		immichtest.User{ID: "b", Name: "B", Photos: 1, UsagePhotos: 120, Quota: &q})
	srv := httptest.NewServer(fake)
	defer srv.Close()
	snap, err := New().Collect(context.Background(), cfg(srv.URL, "k", time.Second))
	if err != nil {
		t.Fatal(err)
	}
	st := map[string]integrations.Status{}
	for _, x := range snap.Metrics {
		st[x.Key] = x.Status
	}
	if st["user/a/usage"] != integrations.StatusWarning || st["user/b/usage"] != integrations.StatusError ||
		st["user/b/quota_used"] != integrations.StatusError {
		t.Fatalf("%v", st)
	}
}

func TestErrors(t *testing.T) {
	fake, srv := setup(t)
	ctx := context.Background()
	im := New()

	if err := im.Test(ctx, cfg(srv.URL, "admin-key", time.Second)); err != nil {
		t.Fatalf("Test: %v", err)
	}
	if err := im.Test(ctx, cfg(srv.URL, "falsch", time.Second)); code(err) != "integration.unauthorized" {
		t.Errorf("401: %v", err)
	}
	if err := im.Test(ctx, cfg(srv.URL, "user-key", time.Second)); code(err) != "immich.forbidden" {
		t.Errorf("403: %v", err)
	}

	fake.Broken(true)
	if _, err := im.Collect(ctx, cfg(srv.URL, "admin-key", time.Second)); code(err) != "integration.bad_json" {
		t.Errorf("kaputtes JSON: %v", err)
	}
	fake.Broken(false)

	fake.Fail(http.StatusBadGateway)
	if _, err := im.Collect(ctx, cfg(srv.URL, "admin-key", time.Second)); code(err) != "integration.http" {
		t.Errorf("502: %v", err)
	}
	fake.Fail(0)

	fake.Delay(300 * time.Millisecond)
	if err := im.Test(ctx, cfg(srv.URL, "admin-key", 100*time.Millisecond)); code(err) != "integration.timeout" {
		t.Errorf("Timeout: %v", err)
	}
	fake.Delay(0)

	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer other.Close()
	if err := im.Test(ctx, cfg(other.URL, "admin-key", time.Second)); code(err) != "immich.unexpected" {
		t.Errorf("fremdes JSON: %v", err)
	}
}

func TestVersionOptional(t *testing.T) {
	fake := immichtest.New("k", "", immichtest.Demo()...)
	mux := http.NewServeMux()
	mux.Handle("/api/server/statistics", fake)
	mux.HandleFunc("/api/server/version", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	snap, err := New().Collect(context.Background(), cfg(srv.URL, "k", time.Second))
	if err != nil || snap.Info != nil {
		t.Fatalf("%v %v", err, snap.Info)
	}
}
