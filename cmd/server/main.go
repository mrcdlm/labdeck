package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mrcdlm/labdeck/internal/api"
	"github.com/mrcdlm/labdeck/internal/config"
	"github.com/mrcdlm/labdeck/internal/events"
	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/integrations/healthchecks"
	"github.com/mrcdlm/labdeck/internal/scheduler"
	"github.com/mrcdlm/labdeck/internal/secrets"
	"github.com/mrcdlm/labdeck/internal/store"
	"github.com/mrcdlm/labdeck/web"
)

// version wird beim Release-Build per -ldflags "-X main.version=…" gesetzt.
var version = "dev"

// Kürzere Klartext-Passwörter werden beim Start mit einer Warnung gemeldet.
const minPasswordLen = 12

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	if err := run(); err != nil {
		slog.Error("Abbruch", "err", err)
		os.Exit(1)
	}
}

// healthcheck wird vom Docker-Healthcheck aufgerufen (distroless hat kein curl).
func healthcheck() int {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", config.PortFromEnv()))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "status", resp.StatusCode)
		return 1
	}
	return 0
}

func run() error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)
	log.Info("labdeck startet", "version", version, "config", cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := ensureWritable(cfg.DataDir); err != nil {
		return err
	}
	st, err := store.Open(ctx, filepath.Join(cfg.DataDir, "app.db"))
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer st.Close()

	settings, warnings := config.NewSettingsService(ctx, st)
	for _, w := range warnings {
		log.Warn("Einstellung", "warning", w)
	}

	if cfg.SecretKey == nil {
		log.Warn("LABDECK_SECRET_KEY nicht gesetzt – Zugangsdaten von Integrationen können nicht gespeichert werden " +
			"(erzeugen mit: openssl rand -base64 32)")
	}

	broker := events.NewBroker()

	// Integrationen: Zugangsdaten verschlüsselt (LABDECK_SECRET_KEY), Verweise
	// ${SECRET_…} aus Env-Variablen.
	box, err := secrets.New(cfg.SecretKey)
	if err != nil {
		return err
	}
	cfg.SecretKey = nil // steckt ab hier nur noch in box
	registry := integrations.NewRegistry(healthchecks.New())
	manager := integrations.NewManager(registry, st, box, os.LookupEnv, log)
	manager.Pub = broker
	poller := scheduler.NewPoller(log, manager.Poll)
	// Nach Anlegen/Ändern/Löschen: Zeitpläne abgleichen und sofort abfragen.
	manager.OnChange = func(id int64) {
		if sched, err := manager.Schedules(ctx); err == nil {
			poller.Sync(sched)
			poller.Trigger(id)
		} else {
			log.Error("Zeitpläne laden fehlgeschlagen", "err", err)
		}
	}

	if !strings.HasPrefix(cfg.AppPassword, "$2") && len(cfg.AppPassword) < minPasswordLen {
		log.Warn(fmt.Sprintf("APP_PASSWORD ist kürzer als %d Zeichen – bitte ein langes, zufälliges Passwort verwenden",
			minPasswordLen))
	}
	auth, err := api.NewAuth(cfg.AppPassword, st)
	if err != nil {
		return err
	}
	cfg.AppPassword = "" // wird ab hier nicht mehr benötigt

	sched := scheduler.New(log)
	sched.Add(scheduler.Job{
		Name:     "session-cleanup",
		Interval: func() time.Duration { return time.Hour },
		Run: func(ctx context.Context) error {
			_, err := st.PurgeExpiredSessions(ctx, time.Now())
			return err
		},
	})
	sched.Add(scheduler.Job{
		Name:     "retention",
		Interval: func() time.Duration { return time.Hour },
		Run: func(ctx context.Context) error {
			s := settings.Get()
			now := time.Now()
			r, err := st.PurgeOldData(ctx, now.AddDate(0, 0, -s.RawRetentionDays), now.AddDate(0, 0, -s.DailyRetentionDays))
			if err == nil && r != (store.PurgeResult{}) {
				log.Info("Alte Daten gelöscht", "samples", r.Samples, "daily", r.Daily, "changes", r.Changes)
			}
			return err
		},
	})

	// Geänderte Einstellungen ohne Neustart anwenden.
	settings.OnChange(func(_, _ config.Settings) { sched.Reschedule() })

	srv := &http.Server{
		Addr: fmt.Sprintf(":%d", cfg.Port),
		Handler: api.NewServer(api.Deps{
			Store: st, Events: broker, Settings: settings, Integrations: manager,
			TrustedProxies: cfg.TrustedProxies,
			Info:           api.Info{Version: version, DataDir: cfg.DataDir, SecretKeySet: box != nil},
			Auth:           auth, Log: log, Static: web.Dist(),
		}).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("HTTP-Server lauscht", "addr", srv.Addr)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	sched.Start(ctx)
	schedules, err := manager.Schedules(ctx)
	if err != nil {
		stop()
		return fmt.Errorf("load integrations: %w", err)
	}
	poller.Start(ctx, schedules)
	log.Info("Integrationen geladen", "active", len(schedules))

	select {
	case <-ctx.Done():
		log.Info("Beende …")
	case err := <-errCh:
		stop()
		sched.Wait()
		poller.Wait()
		return err
	}

	// SSE-Verbindungen zuerst beenden – Shutdown wartet sonst auf sie.
	broker.Close()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("Shutdown", "err", err)
	}
	sched.Wait()
	poller.Wait()
	return nil
}

// ensureWritable liefert eine verständliche Meldung, wenn das Datenverzeichnis
// fehlt oder (typisch bei Bind-Mounts) dem Container-User nicht gehört.
func ensureWritable(dir string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("cannot create data directory %s: %w", dir, err)
	}
	f, err := os.CreateTemp(dir, ".write-test-*")
	if err != nil {
		return fmt.Errorf("data directory %s is not writable for UID %d – "+
			"on the host run e.g. 'sudo chown %d:%d <path>/data': %w",
			dir, os.Getuid(), os.Getuid(), os.Getgid(), err)
	}
	f.Close()
	return os.Remove(f.Name())
}
