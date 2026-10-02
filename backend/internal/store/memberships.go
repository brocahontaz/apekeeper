package store

import (
	"context"
	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MembershipStore struct{ pool *pgxpool.Pool }

func (s MembershipStore) List(ctx context.Context, userID int64) ([]domain.GuildMembership, error) {
	rows, err := s.pool.Query(ctx, `SELECT g.id,g.slug,g.name,g.realm,g.region,g.created_at,g.sync_schedule,g.discord_webhook_url,m.user_id,m.role
		FROM guild_memberships m JOIN guilds g ON g.id=m.guild_id WHERE m.user_id=$1 ORDER BY g.name,g.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.GuildMembership
	for rows.Next() {
		var m domain.GuildMembership
		if err := rows.Scan(&m.Guild.ID, &m.Guild.Slug, &m.Guild.Name, &m.Guild.Realm, &m.Guild.Region, &m.Guild.CreatedAt, &m.Guild.SyncSchedule, &m.Guild.DiscordWebhookURL, &m.UserID, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s MembershipStore) ForUser(ctx context.Context, userID, guildID int64) (domain.GuildMembership, error) {
	var m domain.GuildMembership
	err := s.pool.QueryRow(ctx, `SELECT g.id,g.slug,g.name,g.realm,g.region,g.created_at,g.sync_schedule,g.discord_webhook_url,m.user_id,m.role FROM guild_memberships m JOIN guilds g ON g.id=m.guild_id WHERE m.user_id=$1 AND g.id=$2`, userID, guildID).
		Scan(&m.Guild.ID, &m.Guild.Slug, &m.Guild.Name, &m.Guild.Realm, &m.Guild.Region, &m.Guild.CreatedAt, &m.Guild.SyncSchedule, &m.Guild.DiscordWebhookURL, &m.UserID, &m.Role)
	return m, err
}

func (s MembershipStore) Grant(ctx context.Context, guildID, userID int64, role string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO guild_memberships(guild_id,user_id,role) VALUES($1,$2,$3) ON CONFLICT(guild_id,user_id) DO UPDATE SET role=EXCLUDED.role`, guildID, userID, role)
	return err
}
func (s MembershipStore) Revoke(ctx context.Context, guildID, userID int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM guild_memberships WHERE guild_id=$1 AND user_id=$2`, guildID, userID)
	return err
}
