package store

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProgressionStore struct{ pool *pgxpool.Pool }

// History is a bounded, oldest-first snapshot series. The character join is
// intentional: it makes the guild boundary part of the history query rather
// than relying on a caller having checked it separately.
func (s ProgressionStore) History(ctx context.Context, guildID, characterID int64, from, to time.Time, limit int) ([]domain.Snapshot, error) {
	rows, err := s.pool.Query(ctx, `SELECT p.id,p.character_id,p.captured_at,
		COALESCE(p.item_level,0),COALESCE(p.mythic_rating,0),COALESCE(p.best_key_level,0),COALESCE(p.raid_progress,'[]')
		FROM progression_snapshots p JOIN characters c ON c.id=p.character_id
		WHERE c.guild_id=$1 AND p.character_id=$2 AND p.captured_at >= $3 AND p.captured_at <= $4
		ORDER BY p.captured_at DESC,p.id DESC LIMIT $5`, guildID, characterID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Snapshot{}
	for rows.Next() {
		var x domain.Snapshot
		if err := rows.Scan(&x.ID, &x.CharacterID, &x.CapturedAt, &x.ItemLevel, &x.MythicRating, &x.BestKeyLevel, &x.RaidProgress); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Fetching the newest bounded window preserves a useful default for busy
	// characters; the API contract remains chronological for chart consumers.
	sort.Slice(out, func(i, j int) bool {
		if out[i].CapturedAt.Equal(out[j].CapturedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CapturedAt.Before(out[j].CapturedAt)
	})
	return out, nil
}

func (s ProgressionStore) UpsertMythicPlus(ctx context.Context, m domain.MythicPlus) error {
	// The runs snapshot rides in the legacy dungeons jsonb column; a nil
	// pointer marshals to null.
	runs, e := json.Marshal(m.Runs)
	if e != nil {
		return e
	}
	_, e = s.pool.Exec(ctx, `INSERT INTO mythic_plus(
		character_id,season,season_slug,overall_rating,best_key_level,best_run_score,dungeons,synced_at
	)
	VALUES($1,$2,$3,$4,$5,$6,$7,$8)
	ON CONFLICT(character_id,season_slug) DO UPDATE SET
		overall_rating=EXCLUDED.overall_rating,
		best_key_level=EXCLUDED.best_key_level,
		best_run_score=EXCLUDED.best_run_score,
		dungeons=EXCLUDED.dungeons,
		synced_at=EXCLUDED.synced_at`,
		m.CharacterID, m.Season, m.SeasonSlug, m.OverallRating,
		m.BestKeyLevel, m.BestRunScore, runs, m.SyncedAt)
	return e
}

// UpsertMythicPlusRuns refreshes only the run lists for a season, leaving a
// previously stored rating summary untouched when the row already exists.
func (s ProgressionStore) UpsertMythicPlusRuns(ctx context.Context, m domain.MythicPlus) error {
	runs, e := json.Marshal(m.Runs)
	if e != nil {
		return e
	}
	_, e = s.pool.Exec(ctx, `INSERT INTO mythic_plus(
		character_id,season,season_slug,overall_rating,best_key_level,best_run_score,dungeons,synced_at
	)
	VALUES($1,$2,$3,$4,$5,$6,$7,$8)
	ON CONFLICT(character_id,season_slug) DO UPDATE SET
		season=EXCLUDED.season,
		dungeons=EXCLUDED.dungeons,
		synced_at=EXCLUDED.synced_at`,
		m.CharacterID, m.Season, m.SeasonSlug, m.OverallRating,
		m.BestKeyLevel, m.BestRunScore, runs, m.SyncedAt)
	return e
}

// ReplaceRaids atomically swaps a character's stored raid progression: every
// existing row is removed and the given rows are inserted in one transaction,
// so a failed swap never wipes a character's progression data.
func (s ProgressionStore) ReplaceRaids(ctx context.Context, characterID int64, raids []domain.RaidProgression) error {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	// Rollback through a fresh context so a cancelled caller cannot strand the
	// transaction's connection.
	defer tx.Rollback(context.Background())
	if _, e = tx.Exec(ctx, `DELETE FROM raid_progression WHERE character_id=$1`, characterID); e != nil {
		return e
	}
	for _, r := range raids {
		if _, e = tx.Exec(ctx, `INSERT INTO raid_progression(
			character_id,raid_slug,raid_name,difficulty,progress,total_bosses,summary,synced_at
		)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
			r.CharacterID, r.RaidSlug, r.RaidName, r.Difficulty,
			r.Progress, r.TotalBosses, r.Summary, r.SyncedAt); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}

func (s ProgressionStore) LastSnapshot(ctx context.Context, id int64) (domain.Snapshot, error) {
	var x domain.Snapshot
	e := s.pool.QueryRow(ctx, `SELECT
		id,character_id,captured_at,
		COALESCE(item_level,0),COALESCE(mythic_rating,0),COALESCE(best_key_level,0),
		COALESCE(raid_progress,'[]')
		FROM progression_snapshots
		WHERE character_id=$1
		ORDER BY captured_at DESC,id DESC
		LIMIT 1`, id).
		Scan(&x.ID, &x.CharacterID, &x.CapturedAt, &x.ItemLevel, &x.MythicRating, &x.BestKeyLevel, &x.RaidProgress)
	return x, e
}

func (s ProgressionStore) InsertSnapshot(ctx context.Context, x domain.Snapshot) error {
	_, e := s.pool.Exec(ctx, `INSERT INTO progression_snapshots(
		character_id,captured_at,item_level,mythic_rating,best_key_level,raid_progress
	)
	VALUES($1,$2,$3,$4,$5,$6)`,
		x.CharacterID, x.CapturedAt, x.ItemLevel, x.MythicRating, x.BestKeyLevel, x.RaidProgress)
	return e
}

// DeleteSnapshotsBefore drops snapshot history captured before the retention
// cutoff and reports how many rows were removed.
func (s ProgressionStore) DeleteSnapshotsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, e := s.pool.Exec(ctx, `DELETE FROM progression_snapshots WHERE captured_at < $1`, cutoff)
	return tag.RowsAffected(), e
}
