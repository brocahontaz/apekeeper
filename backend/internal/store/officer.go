package store

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var officerTagName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

type OfficerStore struct{ pool *pgxpool.Pool }

func (s OfficerStore) Queue(ctx context.Context, guildID int64, reason string, now time.Time) ([]domain.OfficerQueueItem, error) {
	if reason != "" && reason != "stale" && reason != "missing_progression" && reason != "unreviewed_change" && reason != "recent_sync_failure" {
		return nil, errors.New("invalid queue reason")
	}
	rows, err := s.pool.Query(ctx, `WITH items AS (
	 SELECT c.id,c.display_name,'stale'::text reason,COALESCE(os.tags,'{}') tags,COALESCE(c.synced_at,'epoch') synced_at,NULL::bigint sync_run_id FROM characters c LEFT JOIN officer_character_state os ON os.character_id=c.id WHERE c.guild_id=$1 AND (c.synced_at IS NULL OR c.synced_at < $2)
	 UNION ALL SELECT c.id,c.display_name,'missing_progression',COALESCE(os.tags,'{}'),COALESCE(c.synced_at,'epoch'),NULL::bigint FROM characters c LEFT JOIN officer_character_state os ON os.character_id=c.id WHERE c.guild_id=$1 AND NOT EXISTS (SELECT 1 FROM progression_snapshots p WHERE p.character_id=c.id)
	 UNION ALL SELECT c.id,c.display_name,'unreviewed_change',COALESCE(os.tags,'{}'),COALESCE(c.synced_at,'epoch'),NULL::bigint FROM characters c LEFT JOIN officer_character_state os ON os.character_id=c.id WHERE c.guild_id=$1 AND (os.reviewed_at IS NULL OR c.updated_at > os.reviewed_at)
	 UNION ALL SELECT NULL::bigint,'Sync run #' || r.id::text,'recent_sync_failure','{}'::text[],r.started_at,r.id FROM sync_runs r LEFT JOIN officer_sync_run_review sr ON sr.sync_run_id=r.id WHERE r.guild_id=$1 AND r.characters_failed > 0 AND r.started_at >= $3 AND sr.sync_run_id IS NULL
	) SELECT COALESCE(id,0),display_name,reason,tags,synced_at,sync_run_id FROM items WHERE $4='' OR reason=$4 ORDER BY reason, synced_at, display_name, id`, guildID, now.Add(-7*24*time.Hour), now.Add(-7*24*time.Hour), reason)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.OfficerQueueItem{}
	for rows.Next() {
		var i domain.OfficerQueueItem
		if err := rows.Scan(&i.CharacterID, &i.Name, &i.Reason, &i.Tags, &i.SyncedAt, &i.SyncRunID); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

func cleanTags(tags []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, tag := range tags {
		tag = strings.TrimSpace(strings.ToLower(tag))
		if !officerTagName.MatchString(tag) {
			return nil, errors.New("invalid tag")
		}
		if !seen[tag] {
			seen[tag] = true
			out = append(out, tag)
		}
	}
	if len(out) > domain.OfficerTagsMax {
		return nil, errors.New("too many tags")
	}
	sort.Strings(out)
	return out, nil
}

// Character returns the officer-maintained metadata for a character in the
// supplied guild. The note author is a display identity, never note content.
func (s OfficerStore) Character(ctx context.Context, guildID, id int64) (domain.OfficerCharacter, error) {
	var result domain.OfficerCharacter
	var author *string
	err := s.pool.QueryRow(ctx, `SELECT c.id,c.guild_id,c.name,c.display_name,c.normalized_name,c.realm,c.realm_slug,c.region,c.class_id,c.class_name,c.spec_id,c.spec_name,c.level,c.item_level,c.guild_rank,c.race_name,c.gender,c.synced_at,
		COALESCE(os.note,''),COALESCE(os.lifecycle_status,''),COALESCE(os.tags,'{}'),os.reviewed_at,u.display_name
		FROM characters c LEFT JOIN officer_character_state os ON os.character_id=c.id LEFT JOIN users u ON u.id=os.note_author_id WHERE c.guild_id=$1 AND c.id=$2`, guildID, id).Scan(
		&result.ID, &result.GuildID, &result.Name, &result.DisplayName, &result.NormalizedName, &result.Realm, &result.RealmSlug, &result.Region, &result.ClassID, &result.ClassName, &result.SpecID, &result.SpecName, &result.Level, &result.ItemLevel, &result.GuildRank, &result.RaceName, &result.Gender, &result.SyncedAt,
		&result.Note, &result.LifecycleStatus, &result.Tags, &result.ReviewedAt, &author)
	if err != nil {
		return result, err
	}
	result.NoteAuthor = author
	return result, nil
}
func (s OfficerStore) BulkTags(ctx context.Context, guildID, actorID int64, ids []int64, add, remove []string) error {
	if !validOfficerIDs(ids) {
		return errors.New("invalid character batch")
	}
	add, e := cleanTags(add)
	if e != nil {
		return e
	}
	remove, e = cleanTags(remove)
	if e != nil {
		return e
	}
	if len(add) == 0 && len(remove) == 0 {
		return errors.New("tag change required")
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if validation, e := validateCharacterIDs(ctx, tx, guildID, ids); e != nil {
		return e
	} else if validation != nil {
		return validation
	}
	for _, id := range ids {
		var before []string
		_ = tx.QueryRow(ctx, `SELECT tags FROM officer_character_state WHERE character_id=$1`, id).Scan(&before)
		set := map[string]bool{}
		for _, x := range before {
			set[x] = true
		}
		for _, x := range add {
			set[x] = true
		}
		for _, x := range remove {
			delete(set, x)
		}
		after := make([]string, 0, len(set))
		for x := range set {
			after = append(after, x)
		}
		sort.Strings(after)
		if len(after) > domain.OfficerTagsMax {
			return errors.New("too many tags")
		}
		_, e = tx.Exec(ctx, `INSERT INTO officer_character_state(character_id,tags,updated_at) VALUES($1,$2,now()) ON CONFLICT(character_id) DO UPDATE SET tags=EXCLUDED.tags,updated_at=now()`, id, after)
		if e != nil {
			return e
		}
		changes, _ := json.Marshal(map[string]any{"tags": map[string]any{"before": before, "after": after}})
		_, e = tx.Exec(ctx, `INSERT INTO officer_activity(guild_id,actor_id,action,target_type,target_id,before_after) VALUES($1,$2,'bulk_tags','character',$3,$4)`, guildID, actorID, id, changes)
		if e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
func (s OfficerStore) Complete(ctx context.Context, guildID, actorID int64, characterIDs, syncRunIDs []int64) error {
	if (len(characterIDs) == 0 && len(syncRunIDs) == 0) || len(characterIDs)+len(syncRunIDs) > domain.OfficerBatchMax || (len(characterIDs) > 0 && !validOfficerIDs(characterIDs)) || (len(syncRunIDs) > 0 && !validOfficerIDs(syncRunIDs)) {
		return errors.New("invalid character batch")
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	validation, e := validateCharacterIDs(ctx, tx, guildID, characterIDs)
	if e != nil {
		return e
	}
	if validation == nil {
		validation = &domain.BulkValidationError{}
	}
	if e = validateSyncRunIDs(ctx, tx, guildID, syncRunIDs, validation); e != nil {
		return e
	}
	if len(validation.InvalidCharacterIDs)+len(validation.CrossGuildCharacterIDs)+len(validation.InvalidSyncRunIDs)+len(validation.CrossGuildSyncRunIDs) > 0 {
		return validation
	}
	for _, id := range characterIDs {
		_, e = tx.Exec(ctx, `INSERT INTO officer_character_state(character_id,reviewed_at,updated_at) VALUES($1,now(),now()) ON CONFLICT(character_id) DO UPDATE SET reviewed_at=now(),updated_at=now()`, id)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO officer_activity(guild_id,actor_id,action,target_type,target_id) VALUES($1,$2,'review_complete','character',$3)`, guildID, actorID, id)
		if e != nil {
			return e
		}
	}
	for _, id := range syncRunIDs {
		_, e = tx.Exec(ctx, `INSERT INTO officer_sync_run_review(sync_run_id,guild_id,reviewed_at) VALUES($1,$2,now()) ON CONFLICT(sync_run_id) DO UPDATE SET reviewed_at=now()`, id, guildID)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO officer_activity(guild_id,actor_id,action,target_type,target_id) VALUES($1,$2,'review_complete','sync_run',$3)`, guildID, actorID, id)
		if e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}

func validateCharacterIDs(ctx context.Context, tx pgx.Tx, guildID int64, ids []int64) (*domain.BulkValidationError, error) {
	result := &domain.BulkValidationError{}
	rows, err := tx.Query(ctx, `SELECT id,guild_id FROM characters WHERE id=ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	found := map[int64]int64{}
	for rows.Next() {
		var id, owner int64
		if err := rows.Scan(&id, &owner); err != nil {
			return nil, err
		}
		found[id] = owner
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, id := range ids {
		if owner, ok := found[id]; !ok {
			result.InvalidCharacterIDs = append(result.InvalidCharacterIDs, id)
		} else if owner != guildID {
			result.CrossGuildCharacterIDs = append(result.CrossGuildCharacterIDs, id)
		}
	}
	if len(result.InvalidCharacterIDs)+len(result.CrossGuildCharacterIDs) == 0 {
		return nil, nil
	}
	return result, nil
}

func validateSyncRunIDs(ctx context.Context, tx pgx.Tx, guildID int64, ids []int64, result *domain.BulkValidationError) error {
	rows, err := tx.Query(ctx, `SELECT id,guild_id FROM sync_runs WHERE id=ANY($1)`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	found := map[int64]int64{}
	for rows.Next() {
		var id, owner int64
		if err := rows.Scan(&id, &owner); err != nil {
			return err
		}
		found[id] = owner
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if owner, ok := found[id]; !ok {
			result.InvalidSyncRunIDs = append(result.InvalidSyncRunIDs, id)
		} else if owner != guildID {
			result.CrossGuildSyncRunIDs = append(result.CrossGuildSyncRunIDs, id)
		}
	}
	return nil
}

func validOfficerIDs(ids []int64) bool {
	if len(ids) == 0 || len(ids) > domain.OfficerBatchMax {
		return false
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}
func (s OfficerStore) Update(ctx context.Context, guildID, actorID, id int64, note, status string, tags []string) error {
	if len(note) > domain.OfficerNoteMaxLength {
		return errors.New("note too long")
	}
	if status != "" && !domain.LifecycleStatuses[status] {
		return errors.New("invalid lifecycle status")
	}
	tags, e := cleanTags(tags)
	if e != nil {
		return e
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var n int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM characters WHERE guild_id=$1 AND id=$2`, guildID, id).Scan(&n); e != nil {
		return e
	}
	if n != 1 {
		return errors.New("character not found")
	}
	var beforeNote, beforeStatus string
	var beforeTags []string
	_ = tx.QueryRow(ctx, `SELECT note,COALESCE(lifecycle_status,''),tags FROM officer_character_state WHERE character_id=$1`, id).Scan(&beforeNote, &beforeStatus, &beforeTags)
	var noteAuthorID any
	if note != "" {
		noteAuthorID = actorID
	}
	_, e = tx.Exec(ctx, `INSERT INTO officer_character_state(character_id,note,note_author_id,lifecycle_status,tags,updated_at) VALUES($1,$2,$3,NULLIF($4,''),$5,now()) ON CONFLICT(character_id) DO UPDATE SET note=EXCLUDED.note,note_author_id=CASE WHEN officer_character_state.note IS DISTINCT FROM EXCLUDED.note THEN EXCLUDED.note_author_id ELSE officer_character_state.note_author_id END,lifecycle_status=EXCLUDED.lifecycle_status,tags=EXCLUDED.tags,updated_at=now()`, id, note, noteAuthorID, status, tags)
	if e != nil {
		return e
	}
	changes, _ := json.Marshal(map[string]any{"noteChanged": beforeNote != note, "lifecycleStatus": map[string]string{"before": beforeStatus, "after": status}, "tags": map[string]any{"before": beforeTags, "after": tags}})
	_, e = tx.Exec(ctx, `INSERT INTO officer_activity(guild_id,actor_id,action,target_type,target_id,before_after) VALUES($1,$2,'character_update','character',$3,$4)`, guildID, actorID, id, changes)
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (s OfficerStore) Activity(ctx context.Context, guildID int64, limit int) ([]domain.OfficerActivity, error) {
	rows, e := s.pool.Query(ctx, `SELECT a.id,u.display_name,a.action,a.target_id,a.created_at,a.before_after FROM officer_activity a JOIN users u ON u.id=a.actor_id WHERE a.guild_id=$1 ORDER BY a.created_at DESC,a.id DESC LIMIT $2`, guildID, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.OfficerActivity{}
	for rows.Next() {
		var x domain.OfficerActivity
		var raw []byte
		if e = rows.Scan(&x.ID, &x.Actor, &x.Action, &x.TargetID, &x.CreatedAt, &raw); e != nil {
			return nil, e
		}
		_ = json.Unmarshal(raw, &x.Changes)
		out = append(out, x)
	}
	return out, rows.Err()
}
