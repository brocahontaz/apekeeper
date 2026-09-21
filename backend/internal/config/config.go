package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct{ DatabaseURL, ServerPort, LogLevel, BlizzardClientID, BlizzardClientSecret, BlizzardRegion, BlizzardLocale, GuildName, GuildRealm, GuildRegion, SyncSchedule, OAuthRedirectURL, SessionSecret, StaticDir string }

func Load() (Config, error) {
	c := Config{DatabaseURL: os.Getenv("DATABASE_URL"), ServerPort: env("SERVER_PORT", "8080"), LogLevel: env("LOG_LEVEL", "info"), BlizzardClientID: os.Getenv("BLIZZARD_CLIENT_ID"), BlizzardClientSecret: os.Getenv("BLIZZARD_CLIENT_SECRET"), BlizzardRegion: env("BLIZZARD_REGION", "us"), BlizzardLocale: env("BLIZZARD_LOCALE", "en_US"), GuildName: env("GUILD_NAME", "Ape Enclosure"), GuildRealm: os.Getenv("GUILD_REALM"), GuildRegion: env("GUILD_REGION", "us"), SyncSchedule: env("SYNC_SCHEDULE", "03:00"), OAuthRedirectURL: env("OAUTH_REDIRECT_URL", "http://localhost:8080/api/auth/callback"), SessionSecret: os.Getenv("SESSION_SECRET"), StaticDir: os.Getenv("STATIC_DIR")}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}
	if c.GuildRealm == "" {
		return c, fmt.Errorf("GUILD_REALM is required")
	}
	if _, err := time.Parse("15:04", c.SyncSchedule); err != nil {
		return c, fmt.Errorf("SYNC_SCHEDULE: %w", err)
	}
	if _, err := strconv.Atoi(c.ServerPort); err != nil {
		return c, fmt.Errorf("SERVER_PORT: %w", err)
	}
	return c, nil
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
