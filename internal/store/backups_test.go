package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBackupMaxAges(t *testing.T) {
	s, ctx := openTest(t), context.Background()
	it, _ := s.CreateIntegration(ctx, Integration{Type: "x", Name: "X", Enabled: true, Interval: time.Minute,
		Timeout: time.Second, Config: "{}"}, nil)
	k := BackupKey{IntegrationID: it.ID, ItemID: "uk123"}

	if err := s.SetBackupMaxAge(ctx, k, 170); err != nil {
		t.Fatal(err)
	}
	if err := s.SetBackupMaxAge(ctx, k, 50); err != nil {
		t.Fatal(err)
	}
	got, err := s.BackupMaxAges(ctx)
	if err != nil || len(got) != 1 || got[k] != 50 {
		t.Fatalf("%v %v", got, err)
	}

	if err := s.SetBackupMaxAge(ctx, BackupKey{IntegrationID: 999, ItemID: "x"}, 5); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unbekannte Integration: %v", err)
	}

	// 0 = Standard
	if err := s.SetBackupMaxAge(ctx, k, 0); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.BackupMaxAges(ctx); len(got) != 0 {
		t.Fatalf("nach Zurücksetzen: %v", got)
	}

	// Löschen der Integration entfernt die Einstellungen
	_ = s.SetBackupMaxAge(ctx, k, 12)
	if err := s.DeleteIntegration(ctx, it.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.BackupMaxAges(ctx); len(got) != 0 {
		t.Fatalf("nach Löschen der Integration: %v", got)
	}
}
