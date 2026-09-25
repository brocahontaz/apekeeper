package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
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
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "db": dbStatus})
	}
}
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
	guild, err := stores.Guilds.EnsureGuild(ctx, domain.Guild{Slug: blizzard.Slug(cfg.GuildName + "-" + cfg.GuildRealm), Name: cfg.GuildName, Realm: cfg.GuildRealm, Region: cfg.GuildRegion})
	if err != nil {
		log.Fatal(err)
	}
	client := blizzard.NewClient(cfg.BlizzardRegion, cfg.BlizzardLocale, cfg.BlizzardClientID, cfg.BlizzardClientSecret, cfg.OAuthRedirectURL)
	service := &appsync.Service{Engine: appsync.Engine{Client: client, Stores: stores}, Guild: guild}
	scheduler := sched.New(cfg.SyncSchedule, logg)
	go scheduler.Run(ctx, func(c context.Context) { _, _ = service.Start(c, "scheduled") })
	manager := auth.New(client, stores.Users, cfg.OAuthRedirectURL, []byte(cfg.SessionSecret))
	trigger := httpapi.TriggerFunc(func(c context.Context) (int64, error) { r, e := service.StartManual(c); return r.ID, e })
	api := httpapi.New(httpapi.API{Stores: stores, Auth: manager, GuildSlug: guild.Slug, Trigger: trigger, Ping: pool, StaticDir: cfg.StaticDir})
	server := &http.Server{Addr: ":" + cfg.ServerPort, Handler: logger.RequestLog(api, logg), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		_ = server.Shutdown(shutdown)
	}()
	logg.Info("server starting", "port", cfg.ServerPort)
	if err = server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Print(err)
	}
}
