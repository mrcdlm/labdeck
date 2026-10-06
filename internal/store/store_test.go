package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrationsIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "app.db")
	for range 2 {
		s, err := Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		s.Close()
	}
}

func TestSettings(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	if _, err := s.GetSetting(ctx, "a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ErrNotFound erwartet, bekam %v", err)
	}
	if err := s.SetSetting(ctx, "a", "1"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetting(ctx, "a", "2"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.GetSetting(ctx, "a"); v != "2" {
		t.Fatalf("v = %q", v)
	}
}

func TestSessions(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if err := s.CreateSession(ctx, "a", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession(ctx, "b", now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.SessionValid(ctx, "a", now); !ok || err != nil {
		t.Fatalf("a: %v %v", ok, err)
	}
	if ok, _ := s.SessionValid(ctx, "b", now); ok {
		t.Fatal("abgelaufene Session gültig")
	}
	if ok, _ := s.SessionValid(ctx, "x", now); ok {
		t.Fatal("unbekannte Session gültig")
	}
	if n, err := s.PurgeExpiredSessions(ctx, now); n != 1 || err != nil {
		t.Fatalf("purge: %d %v", n, err)
	}
	if err := s.DeleteSession(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.SessionValid(ctx, "a", now); ok {
		t.Fatal("gelöschte Session gültig")
	}
}
