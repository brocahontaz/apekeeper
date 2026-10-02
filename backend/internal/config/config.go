package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL           string
	ServerPort            string
	LogLevel              string
	BlizzardClientID      string
	BlizzardClientSecret  string
	BlizzardRegion        string
	BlizzardLocale        string
	GuildName             string
	GuildRealm            string
	GuildRegion           string
	SyncSchedule          string
	SnapshotRetentionDays int
	OAuthRedirectURL      string
	SessionSecret         string
	SuperAdminBattleTags  []string
	DiscordWebhookURL     string
	StaticDir             string
	CSRFEnabled           bool
	MaxRequestBody        int64
	RateLimit             int
	RateLimitWindow       time.Duration
	AuthRateLimit         int
	SyncRateLimit         int
	ExportRateLimit       int
	MutationRateLimit     int
	AuthBodyLimit         int64
	SyncBodyLimit         int64
	ExportBodyLimit       int64
	MutationBodyLimit     int64
}

func Load() (Config, error) {
	retention := 90
	if v := os.Getenv("SNAPSHOT_RETENTION_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("SNAPSHOT_RETENTION_DAYS: must be a positive integer")
		}
		retention = n
	}
	c := Config{
		DatabaseURL:           os.Getenv("DATABASE_URL"),
		ServerPort:            env("SERVER_PORT", "8080"),
		LogLevel:              env("LOG_LEVEL", "info"),
		BlizzardClientID:      os.Getenv("BLIZZARD_CLIENT_ID"),
		BlizzardClientSecret:  os.Getenv("BLIZZARD_CLIENT_SECRET"),
		BlizzardRegion:        env("BLIZZARD_REGION", "us"),
		BlizzardLocale:        env("BLIZZARD_LOCALE", "en_US"),
		GuildName:             env("GUILD_NAME", "Ape Enclosure"),
		GuildRealm:            os.Getenv("GUILD_REALM"),
		GuildRegion:           env("GUILD_REGION", "us"),
		SyncSchedule:          env("SYNC_SCHEDULE", "03:00"),
		SnapshotRetentionDays: retention,
		OAuthRedirectURL:      os.Getenv("OAUTH_REDIRECT_URL"),
		SessionSecret:         os.Getenv("SESSION_SECRET"),
		SuperAdminBattleTags:  parseBattleTags(os.Getenv("SUPER_ADMIN_BATTLETAGS")),
		DiscordWebhookURL:     os.Getenv("DISCORD_WEBHOOK_URL"),
		StaticDir:             os.Getenv("STATIC_DIR"),
		CSRFEnabled:           envBool("CSRF_ENABLED", true),
		MaxRequestBody:        envInt64("MAX_REQUEST_BODY_BYTES", 1<<20),
		RateLimit:             envInt("RATE_LIMIT_PER_MINUTE", 120),
		RateLimitWindow:       time.Minute,
		AuthRateLimit:         envInt("AUTH_RATE_LIMIT_PER_MINUTE", 20),
		SyncRateLimit:         envInt("SYNC_RATE_LIMIT_PER_MINUTE", 30),
		ExportRateLimit:       envInt("EXPORT_RATE_LIMIT_PER_MINUTE", 30),
		MutationRateLimit:     envInt("MUTATION_RATE_LIMIT_PER_MINUTE", 60),
		AuthBodyLimit:         envInt64("AUTH_MAX_BODY_BYTES", 256<<10),
		SyncBodyLimit:         envInt64("SYNC_MAX_BODY_BYTES", 256<<10),
		ExportBodyLimit:       envInt64("EXPORT_MAX_BODY_BYTES", 4<<20),
		MutationBodyLimit:     envInt64("MUTATION_MAX_BODY_BYTES", 256<<10),
	}

	missing := make([]string, 0, 6)
	for _, required := range []struct {
		name  string
		value string
	}{
		{"DATABASE_URL", c.DatabaseURL},
		{"BLIZZARD_CLIENT_ID", c.BlizzardClientID},
		{"BLIZZARD_CLIENT_SECRET", c.BlizzardClientSecret},
		{"GUILD_REALM", c.GuildRealm},
		{"OAUTH_REDIRECT_URL", c.OAuthRedirectURL},
		{"SESSION_SECRET", c.SessionSecret},
	} {
		if required.value == "" {
			missing = append(missing, required.name)
		}
	}
	if len(missing) > 0 {
		return c, fmt.Errorf("required configuration missing: %s", strings.Join(missing, ", "))
	}

	if !isPostgresURL(c.DatabaseURL) {
		return c, fmt.Errorf("DATABASE_URL must be a valid PostgreSQL URL with a host")
	}
	if !isOAuthRedirectURL(c.OAuthRedirectURL) {
		return c, fmt.Errorf("OAUTH_REDIRECT_URL must be an absolute http/https URL with a host")
	}
	if len(c.SessionSecret) < 32 {
		return c, fmt.Errorf("SESSION_SECRET must be at least 32 characters")
	}
	if _, err := time.Parse("15:04", c.SyncSchedule); err != nil {
		return c, fmt.Errorf("SYNC_SCHEDULE: %w", err)
	}
	if _, err := strconv.Atoi(c.ServerPort); err != nil {
		return c, fmt.Errorf("SERVER_PORT: %w", err)
	}
	if c.MaxRequestBody <= 0 || c.MaxRequestBody > 16<<20 {
		return c, fmt.Errorf("MAX_REQUEST_BODY_BYTES must be between 1 and 16777216")
	}
	if c.RateLimit <= 0 || c.RateLimit > 10000 {
		return c, fmt.Errorf("RATE_LIMIT_PER_MINUTE must be between 1 and 10000")
	}
	for name, value := range map[string]int{"AUTH_RATE_LIMIT_PER_MINUTE": c.AuthRateLimit, "SYNC_RATE_LIMIT_PER_MINUTE": c.SyncRateLimit, "EXPORT_RATE_LIMIT_PER_MINUTE": c.ExportRateLimit, "MUTATION_RATE_LIMIT_PER_MINUTE": c.MutationRateLimit} {
		if value <= 0 || value > 10000 {
			return c, fmt.Errorf("%s must be between 1 and 10000", name)
		}
	}
	for name, value := range map[string]int64{"AUTH_MAX_BODY_BYTES": c.AuthBodyLimit, "SYNC_MAX_BODY_BYTES": c.SyncBodyLimit, "EXPORT_MAX_BODY_BYTES": c.ExportBodyLimit, "MUTATION_MAX_BODY_BYTES": c.MutationBodyLimit} {
		if value <= 0 || value > 16<<20 {
			return c, fmt.Errorf("%s must be between 1 and 16777216", name)
		}
	}
	var logLevel slog.Level
	if err := logLevel.UnmarshalText([]byte(c.LogLevel)); err != nil {
		return c, fmt.Errorf("LOG_LEVEL: %w", err)
	}
	return c, nil
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return fallback
}
func envInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			return n
		}
	}
	return fallback
}
func envBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b
		}
	}
	return fallback
}

func isPostgresURL(value string) bool {
	u, err := url.Parse(value)
	return err == nil &&
		(strings.EqualFold(u.Scheme, "postgres") || strings.EqualFold(u.Scheme, "postgresql")) &&
		u.Hostname() != ""
}

func isOAuthRedirectURL(value string) bool {
	u, err := url.Parse(value)
	return err == nil &&
		(strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https")) &&
		u.Hostname() != ""
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// parseBattleTags splits a comma-separated BattleTag list, trimming spaces and
// dropping empty entries; an empty list means nobody is a super admin.
func parseBattleTags(value string) []string {
	tags := make([]string, 0, strings.Count(value, ",")+1)
	for _, part := range strings.Split(value, ",") {
		if tag := strings.TrimSpace(part); tag != "" {
			tags = append(tags, tag)
		}
	}
	if len(tags) == 0 {
		return nil
	}
	return tags
}
