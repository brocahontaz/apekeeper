package store

import (
	"context"
	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProgressionStore struct{ pool *pgxpool.Pool }

func (s ProgressionStore) UpsertMythicPlus(ctx context.Context, m domain.MythicPlus) error {
	_, e := s.pool.Exec(ctx, `INSERT INTO mythic_plus(character_id,season,season_slug,overall_rating,best_key_level,best_run_score,dungeons,synced_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(character_id,season_slug) DO UPDATE SET overall_rating=EXCLUDED.overall_rating,best_key_level=EXCLUDED.best_key_level,best_run_score=EXCLUDED.best_run_score,dungeons=EXCLUDED.dungeons,synced_at=EXCLUDED.synced_at`, m.CharacterID, m.Season, m.SeasonSlug, m.OverallRating, m.BestKeyLevel, m.BestRunScore, m.Dungeons, m.SyncedAt)
	return e
}
func (s ProgressionStore) UpsertRaid(ctx context.Context, r domain.RaidProgression) error {
	_, e := s.pool.Exec(ctx, `INSERT INTO raid_progression(character_id,raid_slug,raid_name,difficulty,progress,total_bosses,summary,synced_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(character_id,raid_slug,difficulty) DO UPDATE SET progress=EXCLUDED.progress,total_bosses=EXCLUDED.total_bosses,summary=EXCLUDED.summary,synced_at=EXCLUDED.synced_at`, r.CharacterID, r.RaidSlug, r.RaidName, r.Difficulty, r.Progress, r.TotalBosses, r.Summary, r.SyncedAt)
	return e
}
func (s ProgressionStore) LastSnapshot(ctx context.Context, id int64) (domain.Snapshot, error) {
	var x domain.Snapshot
	e := s.pool.QueryRow(ctx, `SELECT character_id,captured_at,COALESCE(item_level,0),COALESCE(mythic_rating,0),COALESCE(best_key_level,0),COALESCE(raid_progress,'[]') FROM progression_snapshots WHERE character_id=$1 ORDER BY captured_at DESC LIMIT 1`, id).Scan(&x.CharacterID, &x.CapturedAt, &x.ItemLevel, &x.MythicRating, &x.BestKeyLevel, &x.RaidProgress)
	return x, e
}
func (s ProgressionStore) InsertSnapshot(ctx context.Context, x domain.Snapshot) error {
	_, e := s.pool.Exec(ctx, `INSERT INTO progression_snapshots(character_id,captured_at,item_level,mythic_rating,best_key_level,raid_progress) VALUES($1,$2,$3,$4,$5,$6)`, x.CharacterID, x.CapturedAt, x.ItemLevel, x.MythicRating, x.BestKeyLevel, x.RaidProgress)
	return e
}
