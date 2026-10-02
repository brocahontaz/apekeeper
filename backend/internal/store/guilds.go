package store

import (
	"context"
	"fmt"

	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GuildStore struct{ pool *pgxpool.Pool }

func legacyDefaults(currentSchedule, currentWebhook, schedule, webhook string) (string, string) {
	if currentSchedule == "03:00" && schedule != "" {
		currentSchedule = schedule
	}
	if currentWebhook == "" && webhook != "" {
		currentWebhook = webhook
	}
	return currentSchedule, currentWebhook
}

func (s GuildStore) EnsureGuild(ctx context.Context, g domain.Guild) (domain.Guild, error) {
	e := s.pool.QueryRow(ctx, `INSERT INTO guilds(slug,name,realm,region)
		VALUES($1,$2,$3,$4)
		ON CONFLICT(slug) DO UPDATE SET name=EXCLUDED.name, realm=EXCLUDED.realm, region=EXCLUDED.region
		RETURNING id,created_at,sync_schedule,discord_webhook_url`, g.Slug, g.Name, g.Realm, g.Region).
		Scan(&g.ID, &g.CreatedAt, &g.SyncSchedule, &g.DiscordWebhookURL)
	return g, e
}

// ApplyLegacyDefaults preserves the pre-multi-guild environment settings for
// the bootstrap guild, while leaving any non-default guild-local setting
// untouched. The migration's defaults are the only values that are eligible.
func (s GuildStore) ApplyLegacyDefaults(ctx context.Context, guildID int64, schedule, webhook string) error {
	_, err := s.pool.Exec(ctx, `UPDATE guilds
		SET sync_schedule=CASE WHEN sync_schedule='03:00' AND $2<>'' THEN $2 ELSE sync_schedule END,
			discord_webhook_url=CASE WHEN discord_webhook_url='' AND $3<>'' THEN $3 ELSE discord_webhook_url END
		WHERE id=$1`, guildID, schedule, webhook)
	return err
}

func (s GuildStore) BySlug(ctx context.Context, slug string) (domain.Guild, error) {
	var g domain.Guild
	err := s.pool.QueryRow(ctx, `SELECT id,slug,name,realm,region,created_at,sync_schedule,discord_webhook_url FROM guilds WHERE slug=$1`, slug).
		Scan(&g.ID, &g.Slug, &g.Name, &g.Realm, &g.Region, &g.CreatedAt, &g.SyncSchedule, &g.DiscordWebhookURL)
	return g, err
}

func (s GuildStore) List(ctx context.Context) ([]domain.Guild, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,slug,name,realm,region,created_at,sync_schedule,discord_webhook_url FROM guilds ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Guild
	for rows.Next() {
		var g domain.Guild
		if err := rows.Scan(&g.ID, &g.Slug, &g.Name, &g.Realm, &g.Region, &g.CreatedAt, &g.SyncSchedule, &g.DiscordWebhookURL); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no guilds configured")
	}
	return out, nil
}
