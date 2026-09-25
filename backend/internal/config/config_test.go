package config

import (
	"strings"
	"testing"
)

var configVariables = []string{
	"DATABASE_URL",
	"SERVER_PORT",
	"LOG_LEVEL",
	"BLIZZARD_CLIENT_ID",
	"BLIZZARD_CLIENT_SECRET",
	"BLIZZARD_REGION",
	"BLIZZARD_LOCALE",
	"GUILD_NAME",
	"GUILD_REALM",
	"GUILD_REGION",
	"SYNC_SCHEDULE",
	"OAUTH_REDIRECT_URL",
	"SESSION_SECRET",
	"SUPER_ADMIN_BATTLETAGS",
	"STATIC_DIR",
}

func setConfigEnv(t *testing.T, values map[string]string) {
	t.Helper()
	for _, key := range configVariables {
		t.Setenv(key, "")
	}
	for key, value := range values {
		t.Setenv(key, value)
	}
}

func validConfigEnv() map[string]string {
	return map[string]string{
		"DATABASE_URL":           "postgres://user:password@localhost:5432/apekeeper?sslmode=disable",
		"BLIZZARD_CLIENT_ID":     "client-id",
		"BLIZZARD_CLIENT_SECRET": "client-secret",
		"GUILD_REALM":            "area-52",
		"OAUTH_REDIRECT_URL":     "http://localhost:8080/api/auth/callback",
		"SESSION_SECRET":         strings.Repeat("s", 32),
	}
}

func TestLoadReportsAllMissingRequiredConfiguration(t *testing.T) {
	const secret = "secret-that-must-not-appear-in-errors"
	setConfigEnv(t, map[string]string{"BLIZZARD_CLIENT_SECRET": secret})

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want missing required configuration error")
	}

	message := err.Error()
	for _, name := range []string{
		"DATABASE_URL",
		"BLIZZARD_CLIENT_ID",
		"GUILD_REALM",
		"OAUTH_REDIRECT_URL",
		"SESSION_SECRET",
	} {
		if !strings.Contains(message, name) {
			t.Errorf("Load() error = %q, missing %s", message, name)
		}
	}
	if strings.Contains(message, secret) {
		t.Errorf("Load() error leaked a secret: %q", message)
	}
}

func TestLoadAcceptsCompleteConfigurationAndDefaults(t *testing.T) {
	setConfigEnv(t, validConfigEnv())

	c, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if c.ServerPort != "8080" ||
		c.LogLevel != "info" ||
		c.BlizzardRegion != "us" ||
		c.BlizzardLocale != "en_US" ||
		c.GuildName != "Ape Enclosure" ||
		c.GuildRegion != "us" ||
		c.SyncSchedule != "03:00" ||
		c.StaticDir != "" {
		t.Errorf("Load() defaults = %+v, want documented defaults", c)
	}
}

func TestLoadRejectsInvalidDatabaseURL(t *testing.T) {
	for _, databaseURL := range []string{"mysql://localhost/apekeeper", "postgres://"} {
		t.Run(databaseURL, func(t *testing.T) {
			values := validConfigEnv()
			values["DATABASE_URL"] = databaseURL
			setConfigEnv(t, values)

			_, err := Load()
			if err == nil || err.Error() != "DATABASE_URL must be a valid PostgreSQL URL with a host" {
				t.Errorf("Load() error = %v, want PostgreSQL URL validation error", err)
			}
		})
	}
}

func TestLoadRejectsInvalidOAuthRedirectURL(t *testing.T) {
	values := validConfigEnv()
	values["OAUTH_REDIRECT_URL"] = "/api/auth/callback"
	setConfigEnv(t, values)

	_, err := Load()
	if err == nil || err.Error() != "OAUTH_REDIRECT_URL must be an absolute http/https URL with a host" {
		t.Errorf("Load() error = %v, want OAuth redirect URL validation error", err)
	}
}

func TestLoadRejectsShortSessionSecret(t *testing.T) {
	values := validConfigEnv()
	values["SESSION_SECRET"] = "too-short"
	setConfigEnv(t, values)

	_, err := Load()
	if err == nil || err.Error() != "SESSION_SECRET must be at least 32 characters" {
		t.Errorf("Load() error = %v, want session secret validation error", err)
	}
}

func TestLoadRejectsInvalidScheduleAndPort(t *testing.T) {
	for name, test := range map[string]struct {
		value string
		want  string
	}{
		"SYNC_SCHEDULE": {value: "not-a-time", want: "SYNC_SCHEDULE:"},
		"SERVER_PORT":   {value: "not-a-port", want: "SERVER_PORT:"},
	} {
		t.Run(name, func(t *testing.T) {
			values := validConfigEnv()
			values[name] = test.value
			setConfigEnv(t, values)

			_, err := Load()
			if err == nil || !strings.HasPrefix(err.Error(), test.want) {
				t.Errorf("Load() error = %v, want error starting with %q", err, test.want)
			}
		})
	}
}

func TestLoadRejectsInvalidLogLevel(t *testing.T) {
	values := validConfigEnv()
	values["LOG_LEVEL"] = "not-a-level"
	setConfigEnv(t, values)

	_, err := Load()
	if err == nil || !strings.HasPrefix(err.Error(), "LOG_LEVEL:") {
		t.Errorf("Load() error = %v, want error starting with %q", err, "LOG_LEVEL:")
	}
}

func TestLoadParsesSuperAdminBattleTags(t *testing.T) {
	t.Run("empty env means nobody", func(t *testing.T) {
		setConfigEnv(t, validConfigEnv())

		c, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if c.SuperAdminBattleTags != nil {
			t.Errorf("SuperAdminBattleTags = %q, want nil", c.SuperAdminBattleTags)
		}
	})
	t.Run("comma separated tags are trimmed and empties dropped", func(t *testing.T) {
		values := validConfigEnv()
		values["SUPER_ADMIN_BATTLETAGS"] = "a#1, b#2 ,,"
		setConfigEnv(t, values)

		c, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		want := []string{"a#1", "b#2"}
		if len(c.SuperAdminBattleTags) != len(want) {
			t.Fatalf("SuperAdminBattleTags = %q, want %q", c.SuperAdminBattleTags, want)
		}
		for i, tag := range want {
			if c.SuperAdminBattleTags[i] != tag {
				t.Errorf("SuperAdminBattleTags[%d] = %q, want %q", i, c.SuperAdminBattleTags[i], tag)
			}
		}
	})
}
