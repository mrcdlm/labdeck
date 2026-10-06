package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Poller fragt Integrationen je in eigenem Takt ab. Die Menge der
// Integrationen ändert sich zur Laufzeit (Sync), einzelne lassen sich sofort
// abfragen (Trigger).
type Poller struct {
	log *slog.Logger
	run func(ctx context.Context, id int64) error

	mu    sync.Mutex
	ctx   context.Context // nil = noch nicht gestartet
	loops map[int64]*pollLoop
	wg    sync.WaitGroup
}

type pollLoop struct {
	interval time.Duration
	cancel   context.CancelFunc
	trigger  chan struct{}
}

func NewPoller(log *slog.Logger, run func(ctx context.Context, id int64) error) *Poller {
	return &Poller{log: log, run: run, loops: map[int64]*pollLoop{}}
}

// Start startet die Schleifen; sie enden mit ctx. Vorher per Sync gemeldete
// Integrationen laufen sofort los.
func (p *Poller) Start(ctx context.Context, schedules map[int64]time.Duration) {
	p.mu.Lock()
	p.ctx = ctx
	p.mu.Unlock()
	p.Sync(schedules)
}

// Sync gleicht die laufenden Schleifen mit schedules (ID → Intervall) ab:
// neue starten (mit sofortiger Abfrage), entfernte enden, geänderte
// Intervalle gelten ab sofort.
func (p *Poller) Sync(schedules map[int64]time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ctx == nil || p.ctx.Err() != nil {
		return
	}
	for id, l := range p.loops {
		if _, ok := schedules[id]; !ok {
			l.cancel()
			delete(p.loops, id)
		}
	}
	for id, interval := range schedules {
		if l, ok := p.loops[id]; ok {
			if l.interval == interval {
				continue
			}
			l.cancel()
		}
		ctx, cancel := context.WithCancel(p.ctx)
		l := &pollLoop{interval: interval, cancel: cancel, trigger: make(chan struct{}, 1)}
		p.loops[id] = l
		p.wg.Go(func() { p.loop(ctx, id, l) })
	}
}

// Trigger fragt eine laufende Integration sofort ab (false = läuft nicht).
func (p *Poller) Trigger(id int64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	l, ok := p.loops[id]
	if !ok {
		return false
	}
	select {
	case l.trigger <- struct{}{}:
	default: // bereits angefordert
	}
	return true
}

// Running liefert die Anzahl laufender Schleifen.
func (p *Poller) Running() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.loops)
}

func (p *Poller) Wait() { p.wg.Wait() }

func (p *Poller) loop(ctx context.Context, id int64, l *pollLoop) {
	for {
		if err := p.run(ctx, id); err != nil && ctx.Err() == nil {
			p.log.Error("Abfrage fehlgeschlagen", "integration", id, "err", err)
		}
		t := time.NewTimer(l.interval)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		case <-l.trigger:
			t.Stop()
		}
	}
}
