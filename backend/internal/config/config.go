package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL          string
	ServerPort           string
	LogLevel             string
	BlizzardClientID     string
	BlizzardClientSecret string
	BlizzardRegion       string
	BlizzardLocale       string
	GuildName            string
	GuildRealm           string
	GuildRegion          string
	SyncSchedule         string
	OAuthRedirectURL     string
	SessionSecret        string
	StaticDir            string
}

func Load() (Config, error) {
	c := Config{
		DatabaseURL:          os.Getenv("DATABASE_URL"),
		ServerPort:           env("SERVER_PORT", "8080"),
		LogLevel:             env("LOG_LEVEL", "info"),
		BlizzardClientID:     os.Getenv("BLIZZARD_CLIENT_ID"),
		BlizzardClientSecret: os.Getenv("BLIZZARD_CLIENT_SECRET"),
		BlizzardRegion:       env("BLIZZARD_REGION", "us"),
		BlizzardLocale:       env("BLIZZARD_LOCALE", "en_US"),
		GuildName:            env("GUILD_NAME", "Ape Enclosure"),
		GuildRealm:           os.Getenv("GUILD_REALM"),
		GuildRegion:          env("GUILD_REGION", "us"),
		SyncSchedule:         env("SYNC_SCHEDULE", "03:00"),
		OAuthRedirectURL:     os.Getenv("OAUTH_REDIRECT_URL"),
		SessionSecret:        os.Getenv("SESSION_SECRET"),
		StaticDir:            os.Getenv("STATIC_DIR"),
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
	return c, nil
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
