package store

import (
	"context"
	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GuildStore struct{ pool *pgxpool.Pool }

func (s GuildStore) EnsureGuild(ctx context.Context, g domain.Guild) (domain.Guild, error) {
	e := s.pool.QueryRow(ctx, `INSERT INTO guilds(slug,name,realm,region)
		VALUES($1,$2,$3,$4)
		ON CONFLICT(slug) DO UPDATE SET
			name=EXCLUDED.name,
			realm=EXCLUDED.realm,
			region=EXCLUDED.region
		RETURNING id,created_at`, g.Slug, g.Name, g.Realm, g.Region).
		Scan(&g.ID, &g.CreatedAt)
	return g, e
}

func (s GuildStore) BySlug(ctx context.Context, slug string) (domain.Guild, error) {
	var g domain.Guild
	err := s.pool.QueryRow(ctx, `SELECT id,slug,name,realm,region,created_at
		FROM guilds
		WHERE slug=$1`, slug).
		Scan(&g.ID, &g.Slug, &g.Name, &g.Realm, &g.Region, &g.CreatedAt)
	return g, err
}
