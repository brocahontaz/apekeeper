package store

import (
	"context"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserStore struct{ pool *pgxpool.Pool }

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
	err := s.pool.QueryRow(ctx, `SELECT u.id,COALESCE(u.display_name,''),i.battletag,u.app_role
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
