package store

import (
	"context"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserStore struct{ pool *pgxpool.Pool }

func (s UserStore) SearchGuildCandidates(ctx context.Context, guildID int64, query string) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, `SELECT u.id,COALESCE(u.display_name,''),i.battletag,u.app_role
		FROM users u JOIN battle_net_identities i ON i.user_id=u.id
		WHERE NOT EXISTS (SELECT 1 FROM guild_memberships m WHERE m.guild_id=$1 AND m.user_id=u.id)
		AND ($2='' OR u.display_name ILIKE '%'||$2||'%' OR i.battletag ILIKE '%'||$2||'%')
		ORDER BY u.display_name,u.id LIMIT 20`, guildID, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id int64
		var name, tag, role string
		if err := rows.Scan(&id, &name, &tag, &role); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "displayName": name, "battletag": tag, "appRole": role})
	}
	return out, rows.Err()
}

func (s UserStore) FindGuildCandidate(ctx context.Context, guildID int64, battletag string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `SELECT u.id FROM users u JOIN battle_net_identities i ON i.user_id=u.id
		WHERE i.battletag=$2 AND NOT EXISTS (SELECT 1 FROM guild_memberships m WHERE m.guild_id=$1 AND m.user_id=u.id)`, guildID, battletag).Scan(&id)
	return id, err
}

func (s UserStore) GuildMembers(ctx context.Context, guildID int64) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, `SELECT u.id,COALESCE(u.display_name,''),i.battletag,m.role FROM guild_memberships m JOIN users u ON u.id=m.user_id LEFT JOIN battle_net_identities i ON i.user_id=u.id WHERE m.guild_id=$1 ORDER BY u.display_name,u.id`, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id int64
		var name, tag, role string
		if err := rows.Scan(&id, &name, &tag, &role); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "displayName": name, "battletag": tag, "role": role})
	}
	return out, rows.Err()
}

// UpsertBattleNetUser assigns the first user administrator access as a bootstrap
// rule and grants the platform-level superadmin role when requested. It never
// demotes an existing role. Guild-rank to application-role mapping deliberately
// belongs to future work.
func (s UserStore) UpsertBattleNetUser(
	ctx context.Context,
	bnetID, battletag, accessToken, refreshToken string,
	expiresAt time.Time,
	superadmin bool,
) (domain.User, error) {
	var u domain.User
	err := s.pool.QueryRow(ctx, `WITH existing AS (
		SELECT u.id FROM users u JOIN battle_net_identities i ON i.user_id=u.id WHERE i.bnet_id=$1
	), inserted AS (
		INSERT INTO users(display_name,app_role)
		SELECT $2, CASE WHEN $6 THEN 'superadmin' WHEN NOT EXISTS (SELECT 1 FROM users) THEN 'admin' ELSE 'member' END
		WHERE NOT EXISTS (SELECT 1 FROM existing) RETURNING id,display_name,app_role
	), selected AS (
		SELECT u.id,u.display_name,u.app_role
		FROM users u
		JOIN battle_net_identities i ON i.user_id=u.id
		WHERE i.bnet_id=$1
		UNION ALL SELECT id,display_name,app_role FROM inserted
	)
	INSERT INTO battle_net_identities(user_id,bnet_id,battletag,access_token,refresh_token,token_expires_at)
	SELECT id,$1,$2,$3,$4,$5 FROM selected
	ON CONFLICT(bnet_id) DO UPDATE SET
		battletag=EXCLUDED.battletag,
		access_token=EXCLUDED.access_token,
		refresh_token=EXCLUDED.refresh_token,
		token_expires_at=EXCLUDED.token_expires_at,
		updated_at=now()
	RETURNING user_id`, bnetID, battletag, accessToken, refreshToken, expiresAt, superadmin).
		Scan(&u.ID)
	if err != nil {
		return u, err
	}
	if superadmin {
		if _, err = s.pool.Exec(ctx,
			`UPDATE users SET app_role='superadmin' WHERE id=$1 AND app_role<>'superadmin'`, u.ID); err != nil {
			return u, err
		}
	}
	err = s.pool.QueryRow(ctx, `SELECT u.id,u.display_name,u.app_role FROM users u WHERE u.id=$1`, u.ID).
		Scan(&u.ID, &u.DisplayName, &u.Role)
	return u, err
}

func (s UserStore) CreateSession(ctx context.Context, id string, userID int64, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO sessions(id,user_id,expires_at) VALUES($1,$2,$3)`, id, userID, expiresAt)
	return err
}

func (s UserStore) SessionUser(ctx context.Context, id string) (domain.User, error) {
	var u domain.User
	err := s.pool.QueryRow(ctx, `WITH one_guild AS (SELECT min(id) id FROM guilds HAVING count(*)=1),
		candidate AS (SELECT u.id,u.app_role,g.id guild_id FROM sessions s JOIN users u ON u.id=s.user_id CROSS JOIN one_guild g WHERE s.id=$1 AND s.expires_at > now()),
		boot AS (INSERT INTO guild_memberships(guild_id,user_id,role) SELECT guild_id,id,CASE WHEN app_role IN ('admin','superadmin') THEN 'owner' ELSE app_role END FROM candidate ON CONFLICT DO NOTHING)
		SELECT u.id,COALESCE(u.display_name,''),i.battletag,u.app_role
		FROM sessions s
		JOIN users u ON u.id=s.user_id
		JOIN battle_net_identities i ON i.user_id=u.id
		WHERE s.id=$1 AND s.expires_at > now()`, id).
		Scan(&u.ID, &u.DisplayName, &u.BattleTag, &u.Role)
	return u, err
}

func (s UserStore) DeleteSession(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE id=$1`, id)
	return err
}

// DeleteExpiredSessions drops sessions whose validity window has closed and
// reports how many rows were removed.
func (s UserStore) DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= $1`, now)
	return tag.RowsAffected(), err
}
