package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type counter struct {
	mu sync.Mutex
	n  map[int64]int
}

func (c *counter) run(_ context.Context, id int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n[id]++
	return nil
}

func (c *counter) get(id int64) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n[id]
}

func eventually(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("Bedingung nicht erreicht")
}

func TestPoller(t *testing.T) {
	c := &counter{n: map[int64]int{}}
	p := NewPoller(slog.New(slog.DiscardHandler), c.run)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// vor Start: keine Wirkung
	p.Sync(map[int64]time.Duration{1: time.Hour})
	if p.Running() != 0 {
		t.Fatal("läuft vor Start")
	}

	p.Start(ctx, map[int64]time.Duration{1: time.Hour, 2: 20 * time.Millisecond})
	// sofortige erste Abfrage, 2 läuft im kurzen Takt weiter
	eventually(t, func() bool { return c.get(1) == 1 && c.get(2) >= 3 })

	// Trigger fragt sofort ab
	if !p.Trigger(1) {
		t.Fatal("Trigger")
	}
	eventually(t, func() bool { return c.get(1) == 2 })
	if p.Trigger(99) {
		t.Fatal("Trigger für Unbekanntes")
	}

	// 2 entfernen, 3 hinzufügen, 1 unverändert (keine zusätzliche Abfrage)
	p.Sync(map[int64]time.Duration{1: time.Hour, 3: time.Hour})
	eventually(t, func() bool { return c.get(3) == 1 })
	n2 := c.get(2)
	time.Sleep(60 * time.Millisecond)
	if c.get(2) > n2+1 || c.get(1) != 2 || p.Running() != 2 {
		t.Fatalf("nach Sync: %v, running %d", c.n, p.Running())
	}

	// geändertes Intervall startet neu (mit sofortiger Abfrage)
	p.Sync(map[int64]time.Duration{1: 20 * time.Millisecond, 3: time.Hour})
	eventually(t, func() bool { return c.get(1) >= 4 })

	cancel()
	p.Wait()
	p.Sync(map[int64]time.Duration{4: time.Hour}) // nach Ende harmlos
	if c.get(4) != 0 {
		t.Fatal("nach Ende gestartet")
	}
}
