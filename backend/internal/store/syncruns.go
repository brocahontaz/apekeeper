package store

import (
	"context"
	"encoding/json"
	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type SyncRun struct {
	ID           int64            `json:"id"`
	GuildID      int64            `json:"guildId"`
	StartedAt    time.Time        `json:"startedAt"`
	FinishedAt   *time.Time       `json:"finishedAt"`
	Trigger      string           `json:"trigger"`
	Status       domain.RunStatus `json:"status"`
	Total        int              `json:"total"`
	Updated      int              `json:"updated"`
	Failed       int              `json:"failed"`
	ErrorSummary string           `json:"errorSummary"`
	Detail       json.RawMessage  `json:"detail"`
}
type SyncRunStore struct{ pool *pgxpool.Pool }

func (s SyncRunStore) Create(ctx context.Context, guildID int64, trigger string) (SyncRun, error) {
	r := SyncRun{GuildID: guildID, Trigger: trigger, Status: domain.RunRunning}
	e := s.pool.QueryRow(ctx, `INSERT INTO sync_runs(guild_id,trigger,status)
		VALUES($1,$2,'running')
		RETURNING id,started_at`, guildID, trigger).
		Scan(&r.ID, &r.StartedAt)
	return r, e
}

func (s SyncRunStore) Finish(ctx context.Context, r SyncRun) error {
	_, e := s.pool.Exec(ctx, `UPDATE sync_runs SET
		finished_at=now(),
		status=$2,
		characters_total=$3,
		characters_updated=$4,
		characters_failed=$5,
		error_summary=$6,
		detail=$7
		WHERE id=$1`,
		r.ID, r.Status, r.Total, r.Updated, r.Failed, r.ErrorSummary, r.Detail)
	return e
}

func (s SyncRunStore) History(ctx context.Context, guildID int64, limit int) ([]SyncRun, error) {
	rows, e := s.pool.Query(ctx, `SELECT
		id,guild_id,started_at,finished_at,trigger,status,
		COALESCE(characters_total,0),
		COALESCE(characters_updated,0),
		COALESCE(characters_failed,0),
		COALESCE(error_summary,''),
		COALESCE(detail,'{}')
		FROM sync_runs
		WHERE guild_id=$1
		ORDER BY started_at DESC
		LIMIT $2`, guildID, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []SyncRun
	for rows.Next() {
		var r SyncRun
		e = rows.Scan(
			&r.ID, &r.GuildID, &r.StartedAt, &r.FinishedAt, &r.Trigger, &r.Status,
			&r.Total, &r.Updated, &r.Failed, &r.ErrorSummary, &r.Detail,
		)
		if e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
