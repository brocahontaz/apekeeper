package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/auth"
	"github.com/brocahontaz/apekeeper/backend/internal/blizzard"
	"github.com/brocahontaz/apekeeper/backend/internal/config"
	"github.com/brocahontaz/apekeeper/backend/internal/db"
	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/brocahontaz/apekeeper/backend/internal/httpapi"
	"github.com/brocahontaz/apekeeper/backend/internal/logger"
	"github.com/brocahontaz/apekeeper/backend/internal/migrations"
	"github.com/brocahontaz/apekeeper/backend/internal/notify"
	"github.com/brocahontaz/apekeeper/backend/internal/sched"
	"github.com/brocahontaz/apekeeper/backend/internal/store"
	appsync "github.com/brocahontaz/apekeeper/backend/internal/sync"
)

func health(p db.Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, dbStatus := http.StatusOK, "ok"
		if p.Ping(r.Context()) != nil {
			status, dbStatus = http.StatusServiceUnavailable, "degraded"
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		overall := "ok"
		if status != http.StatusOK {
			overall = "degraded"
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": overall, "db": dbStatus})
	}
}

type schedulerHealth struct {
	expected int64
	started  atomic.Int64
	stopped  atomic.Int64
}

func (s *schedulerHealth) check(ctx context.Context) error {
	if ctx.Err() != nil || s.started.Load() != s.expected || s.stopped.Load() != 0 {
		return errors.New("scheduler unavailable")
	}
	return nil
}

// sweepInterval drives the nightly cleanup of snapshot history and sessions.
const sweepInterval = 24 * time.Hour

func main() {
	migrateOnly := flag.Bool("migrate", false, "apply database migrations")
	flag.Parse()
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if err = db.Migrate(cfg.DatabaseURL, migrations.FS); err != nil {
		log.Fatal(err)
	}
	if *migrateOnly {
		return
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	logg := logger.New(cfg.LogLevel)
	stores := store.New(pool)
	guild, err := stores.Guilds.EnsureGuild(ctx, domain.Guild{
		Slug:   blizzard.Slug(cfg.GuildName + "-" + cfg.GuildRealm),
		Name:   cfg.GuildName,
		Realm:  cfg.GuildRealm,
		Region: cfg.GuildRegion,
	})
	if err != nil {
		log.Fatal(err)
	}
	if err = stores.Guilds.ApplyLegacyDefaults(ctx, guild.ID, cfg.SyncSchedule, cfg.DiscordWebhookURL); err != nil {
		log.Fatal(err)
	}
	client := blizzard.NewClient(
		cfg.BlizzardRegion,
		cfg.BlizzardLocale,
		cfg.BlizzardClientID,
		cfg.BlizzardClientSecret,
		cfg.OAuthRedirectURL,
	)
	client.Log = logg
	allGuilds, err := stores.Guilds.List(ctx)
	if err != nil {
		log.Fatal(err)
	}
	services := make(map[int64]*appsync.Service, len(allGuilds))
	var servicesMu sync.RWMutex
	schedulerStatus := &schedulerHealth{expected: int64(len(allGuilds))}
	for _, g := range allGuilds {
		g := g
		service := &appsync.Service{Engine: appsync.Engine{Client: client, Stores: stores, Log: logg}, Guild: g, Log: logg, Notifier: notify.NewDiscord(g.DiscordWebhookURL, logg)}
		service.Engine.Progress = service.ReportProgress
		servicesMu.Lock()
		services[g.ID] = service
		servicesMu.Unlock()
		schedule := g.SyncSchedule
		if schedule == "" {
			schedule = cfg.SyncSchedule
		}
		scheduler := sched.New(schedule, logg)
		go func() {
			schedulerStatus.started.Add(1)
			defer schedulerStatus.stopped.Add(1)
			scheduler.Run(ctx, func(c context.Context) {
				if _, err := service.Start(c, "scheduled"); err != nil {
					logg.Warn("scheduled sync failed", "guild", g.Slug, "error", err)
				}
			})
		}()
	}
	defaultService := services[guild.ID]
	sweeper := sched.NewSweeper(sweepInterval, logg)
	var backgroundStopped atomic.Bool
	backgroundDone := make(chan struct{})
	go func() {
		defer close(backgroundDone)
		defer backgroundStopped.Store(true)
		sweeper.Run(ctx, func(c context.Context) {
			cutoff := time.Now().AddDate(0, 0, -cfg.SnapshotRetentionDays)
			deleted, err := stores.Progression.DeleteSnapshotsBefore(c, cutoff)
			if err != nil {
				logg.Warn("snapshot cleanup failed", "error", err)
			} else {
				logg.Info("snapshot cleanup complete", "deleted", deleted, "retention_days", cfg.SnapshotRetentionDays)
			}
			if deleted, err = stores.Users.DeleteExpiredSessions(c, time.Now()); err != nil {
				logg.Warn("session cleanup failed", "error", err)
			} else {
				logg.Info("session cleanup complete", "deleted", deleted)
			}
			if deleted, err = stores.Users.DeleteExpiredOAuthStates(c, time.Now()); err != nil {
				logg.Warn("OAuth state cleanup failed", "error", err)
			} else {
				logg.Info("OAuth state cleanup complete", "deleted", deleted)
			}
		})
	}()
	manager := auth.New(client, stores.Users, cfg.OAuthRedirectURL, []byte(cfg.SessionSecret))
	manager.StateStore = stores.Users
	manager.Audit = func(c context.Context, event string) error { return stores.Users.Audit(c, 0, guild.ID, event) }
	manager.Log = logg
	manager.SuperAdminBattleTags = cfg.SuperAdminBattleTags
	trigger := httpapi.TriggerFunc(func(c context.Context) (int64, error) {
		r, e := defaultService.StartManual(c)
		return r.ID, e
	})
	var shuttingDown atomic.Bool
	api := httpapi.New(httpapi.API{
		Stores:     stores,
		Auth:       manager,
		GuildSlug:  guild.Slug,
		Trigger:    trigger,
		Progress:   defaultService,
		Operations: defaultService,
		ScopedTrigger: func(c context.Context, g domain.Guild) (int64, error) {
			servicesMu.RLock()
			s := services[g.ID]
			servicesMu.RUnlock()
			if s == nil {
				return 0, errors.New("sync unavailable for guild")
			}
			r, err := s.StartManual(c)
			return r.ID, err
		},
		ScopedOperations: func(g domain.Guild) httpapi.SyncOperations {
			servicesMu.RLock()
			defer servicesMu.RUnlock()
			return services[g.ID]
		},
		ScopedProgress: func(g domain.Guild) httpapi.ProgressReporter {
			servicesMu.RLock()
			defer servicesMu.RUnlock()
			return services[g.ID]
		},
		Ping:                  pool,
		SnapshotRetentionDays: cfg.SnapshotRetentionDays,
		StaticDir:             cfg.StaticDir,
		CSRFEnabled:           cfg.CSRFEnabled,
		MaxRequestBody:        cfg.MaxRequestBody,
		RateLimit:             cfg.RateLimit,
		RateLimitWindow:       cfg.RateLimitWindow,
		Security:              httpapi.SecurityLimits{Auth: cfg.AuthRateLimit, Sync: cfg.SyncRateLimit, Export: cfg.ExportRateLimit, Mutation: cfg.MutationRateLimit, AuthBody: cfg.AuthBodyLimit, SyncBody: cfg.SyncBodyLimit, ExportBody: cfg.ExportBodyLimit, MutationBody: cfg.MutationBodyLimit, Window: cfg.RateLimitWindow},
		ReadinessChecks: map[string]func(context.Context) error{
			"blizzard": func(c context.Context) error { _, err := client.MythicKeystoneSeasonIndex(c); return err },
			"background": func(c context.Context) error {
				if c.Err() != nil {
					return c.Err()
				}
				if shuttingDown.Load() || backgroundStopped.Load() {
					return errors.New("background workers stopping")
				}
				return nil
			},
			"scheduler": schedulerStatus.check,
			"notifications": func(c context.Context) error {
				if c.Err() != nil || shuttingDown.Load() {
					return errors.New("notification workers stopping")
				}
				servicesMu.RLock()
				defer servicesMu.RUnlock()
				for _, service := range services {
					if !service.NotificationHealthy() {
						return errors.New("notification workers stopping")
					}
				}
				return nil
			},
		},
		ShuttingDown: shuttingDown.Load,
	})
	server := &http.Server{
		Addr:              ":" + cfg.ServerPort,
		Handler:           logger.RequestLog(api, logg),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shuttingDown.Store(true)
		shutdown, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		_ = server.Shutdown(shutdown)
		servicesMu.RLock()
		for _, service := range services {
			_ = service.Shutdown(shutdown)
		}
		servicesMu.RUnlock()
		select {
		case <-backgroundDone:
		case <-shutdown.Done():
			logg.Warn("background workers did not stop before shutdown deadline")
		}
	}()
	logg.Info("server starting", "port", cfg.ServerPort)
	if err = server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Print(err)
	}
}
