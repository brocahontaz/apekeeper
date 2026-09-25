package store

import (
	"context"
	"fmt"
	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"time"
)

type CharacterStore struct{ pool *pgxpool.Pool }
type CharacterFilter struct {
	Class, Spec    string
	Rank, MinLevel *int
	MinRating      *float64
	Name           string
	StaleBefore    *time.Time
}
type CharacterDetail struct {
	Character domain.Character
	AvatarURL string
	Mythic    []domain.MythicPlus
	Raids     []domain.RaidProgression
	Snapshots []domain.Snapshot
}
type CharacterPage struct {
	Items []domain.Character
	Total int
}
type CharacterSort string

const (
	CharacterSortName         CharacterSort = "name"
	CharacterSortLevel        CharacterSort = "level"
	CharacterSortItemLevel    CharacterSort = "itemLevel"
	CharacterSortGuildRank    CharacterSort = "guildRank"
	CharacterSortRealm        CharacterSort = "realm"
	CharacterSortClassSpec    CharacterSort = "classSpec"
	CharacterSortMythicRating CharacterSort = "mythicRating"
	CharacterSortSyncedAt     CharacterSort = "syncedAt"
)

func ParseCharacterSort(v string) (CharacterSort, bool) {
	sort := CharacterSort(v)
	switch sort {
	case CharacterSortName, CharacterSortLevel, CharacterSortItemLevel, CharacterSortGuildRank,
		CharacterSortRealm, CharacterSortClassSpec, CharacterSortMythicRating, CharacterSortSyncedAt:
		return sort, true
	default:
		return CharacterSortName, false
	}
}

func (s CharacterSort) SQL() string {
	switch s {
	case CharacterSortLevel:
		return "COALESCE(c.level,0)"
	case CharacterSortItemLevel:
		return "COALESCE(c.item_level,0)"
	case CharacterSortGuildRank:
		return "COALESCE(c.guild_rank,0)"
	case CharacterSortRealm:
		return "c.realm"
	case CharacterSortClassSpec:
		return "(COALESCE(c.class_name,'') || ' ' || COALESCE(c.spec_name,''))"
	case CharacterSortMythicRating:
		return "COALESCE((SELECT MAX(overall_rating) FROM mythic_plus m WHERE m.character_id=c.id),0)"
	case CharacterSortSyncedAt:
		return "COALESCE(c.synced_at,'epoch')"
	default:
		return "c.display_name"
	}
}

func (s CharacterStore) UpsertByGuildIdentity(
	ctx context.Context,
	c domain.Character,
	profile []byte,
) (domain.Character, error) {
	q := `INSERT INTO characters(
		guild_id,name,display_name,normalized_name,realm,realm_slug,region,
		class_id,class_name,spec_id,spec_name,level,item_level,guild_rank,
		profile_json,synced_at
	)
	VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
	ON CONFLICT(guild_id,region,realm_slug,normalized_name) DO UPDATE SET
		display_name=EXCLUDED.display_name,
		class_id=EXCLUDED.class_id,
		class_name=EXCLUDED.class_name,
		spec_id=EXCLUDED.spec_id,
		spec_name=EXCLUDED.spec_name,
		level=EXCLUDED.level,
		item_level=EXCLUDED.item_level,
		guild_rank=EXCLUDED.guild_rank,
		profile_json=EXCLUDED.profile_json,
		synced_at=EXCLUDED.synced_at,
		updated_at=now()
	RETURNING id`
	e := s.pool.QueryRow(ctx, q,
		c.GuildID, c.Name, c.DisplayName, c.NormalizedName,
		c.Realm, c.RealmSlug, c.Region,
		c.ClassID, c.ClassName, c.SpecID, c.SpecName,
		c.Level, c.ItemLevel, c.GuildRank,
		profile, c.SyncedAt,
	).Scan(&c.ID)
	return c, e
}

// UpdateAvatar stores a character portrait URL without touching sync state.
func (s CharacterStore) UpdateAvatar(ctx context.Context, characterID int64, avatarURL string) error {
	_, e := s.pool.Exec(ctx,
		`UPDATE characters SET avatar_url=$2, updated_at=now() WHERE id=$1`,
		characterID, avatarURL,
	)
	return e
}

func (s CharacterStore) Detail(ctx context.Context, guildID, id int64, since time.Time) (CharacterDetail, error) {
	var d CharacterDetail
	err := s.pool.QueryRow(ctx, `SELECT
		id,guild_id,name,display_name,normalized_name,realm,realm_slug,region,
		COALESCE(class_id,0),COALESCE(class_name,''),
		COALESCE(spec_id,0),COALESCE(spec_name,''),
		COALESCE(level,0),COALESCE(item_level,0),COALESCE(guild_rank,0),
		COALESCE((SELECT MAX(overall_rating) FROM mythic_plus m WHERE m.character_id=characters.id),0),
		COALESCE((SELECT MAX(best_key_level) FROM mythic_plus m WHERE m.character_id=characters.id),0),
		COALESCE(synced_at,'epoch'),COALESCE(avatar_url,'')
		FROM characters
		WHERE guild_id=$1 AND id=$2`, guildID, id).
		Scan(
			&d.Character.ID, &d.Character.GuildID,
			&d.Character.Name, &d.Character.DisplayName, &d.Character.NormalizedName,
			&d.Character.Realm, &d.Character.RealmSlug, &d.Character.Region,
			&d.Character.ClassID, &d.Character.ClassName,
			&d.Character.SpecID, &d.Character.SpecName,
			&d.Character.Level, &d.Character.ItemLevel, &d.Character.GuildRank,
			&d.Character.MythicRating, &d.Character.BestKeyLevel, &d.Character.SyncedAt,
			&d.AvatarURL,
		)
	if err != nil {
		return d, err
	}
	rows, err := s.pool.Query(ctx, `SELECT
		season,season_slug,
		COALESCE(overall_rating,0),COALESCE(best_key_level,0),COALESCE(best_run_score,0),
		dungeons,synced_at
		FROM mythic_plus
		WHERE character_id=$1`, id)
	if err != nil {
		return d, err
	}
	defer rows.Close()
	for rows.Next() {
		var x domain.MythicPlus
		x.CharacterID = id
		if err = rows.Scan(
			&x.Season, &x.SeasonSlug, &x.OverallRating,
			&x.BestKeyLevel, &x.BestRunScore, &x.Dungeons, &x.SyncedAt,
		); err != nil {
			return d, err
		}
		d.Mythic = append(d.Mythic, x)
	}
	if err = rows.Err(); err != nil {
		return d, err
	}
	rows, err = s.pool.Query(ctx, `SELECT
		raid_slug,COALESCE(raid_name,''),difficulty,
		COALESCE(progress,0),COALESCE(total_bosses,0),
		summary,synced_at
		FROM raid_progression
		WHERE character_id=$1`, id)
	if err != nil {
		return d, err
	}
	defer rows.Close()
	for rows.Next() {
		var x domain.RaidProgression
		x.CharacterID = id
		if err = rows.Scan(
			&x.RaidSlug, &x.RaidName, &x.Difficulty,
			&x.Progress, &x.TotalBosses, &x.Summary, &x.SyncedAt,
		); err != nil {
			return d, err
		}
		d.Raids = append(d.Raids, x)
	}
	rows, err = s.pool.Query(ctx, `SELECT
		captured_at,
		COALESCE(item_level,0),COALESCE(mythic_rating,0),COALESCE(best_key_level,0),
		COALESCE(raid_progress,'[]')
		FROM progression_snapshots
		WHERE character_id=$1 AND captured_at >= $2
		ORDER BY captured_at`, id, since)
	if err != nil {
		return d, err
	}
	defer rows.Close()
	for rows.Next() {
		var x domain.Snapshot
		x.CharacterID = id
		if err = rows.Scan(&x.CapturedAt, &x.ItemLevel, &x.MythicRating, &x.BestKeyLevel, &x.RaidProgress); err != nil {
			return d, err
		}
		d.Snapshots = append(d.Snapshots, x)
	}
	return d, rows.Err()
}

// DetailByName resolves a URL-safe display name within its guild before
// loading the same complete profile as the legacy numeric-ID lookup.
func (s CharacterStore) DetailByName(ctx context.Context, guildID int64, name string, since time.Time) (CharacterDetail, error) {
	var id int64
	if err := s.pool.QueryRow(ctx, `SELECT id FROM characters WHERE guild_id=$1 AND normalized_name=$2`, guildID, domain.NormalizeCharacterName(name)).Scan(&id); err != nil {
		return CharacterDetail{}, err
	}
	return s.Detail(ctx, guildID, id, since)
}

func (s CharacterStore) ListByGuild(ctx context.Context, guildID int64, f CharacterFilter) ([]domain.Character, error) {
	page, err := s.listByGuild(ctx, guildID, f, 0, 0, CharacterSortName, false)
	return page.Items, err
}

func (s CharacterStore) ListPageByGuild(ctx context.Context, guildID int64, f CharacterFilter, limit, offset int, sort CharacterSort, descending bool) (CharacterPage, error) {
	return s.listByGuild(ctx, guildID, f, limit, offset, sort, descending)
}

func (s CharacterStore) ListClassesByGuild(ctx context.Context, guildID int64) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT class_name FROM characters WHERE guild_id=$1 AND COALESCE(class_name,'') <> '' ORDER BY class_name`, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	classes := []string{}
	for rows.Next() {
		var class string
		if err := rows.Scan(&class); err != nil {
			return nil, err
		}
		classes = append(classes, class)
	}
	return classes, rows.Err()
}

func (s CharacterStore) listByGuild(ctx context.Context, guildID int64, f CharacterFilter, limit, offset int, sort CharacterSort, descending bool) (CharacterPage, error) {
	args := []any{guildID}
	where := []string{"c.guild_id=$1"}
	add := func(q string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(q, len(args)))
	}
	if f.Class != "" {
		add("c.class_name=$%d", f.Class)
	}
	if f.Spec != "" {
		add("c.spec_name=$%d", f.Spec)
	}
	if f.Rank != nil {
		add("c.guild_rank=$%d", *f.Rank)
	}
	if f.MinLevel != nil {
		add("c.level >= $%d", *f.MinLevel)
	}
	if f.MinRating != nil {
		add("EXISTS (SELECT 1 FROM mythic_plus m WHERE m.character_id=c.id AND m.overall_rating >= $%d)", *f.MinRating)
	}
	if f.Name != "" {
		add("c.normalized_name LIKE $%d", "%"+domain.NormalizeCharacterName(f.Name)+"%")
	}
	if f.StaleBefore != nil {
		add("(c.synced_at IS NULL OR c.synced_at < $%d)", *f.StaleBefore)
	}
	whereClause := strings.Join(where, " AND ")
	var total int
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM characters c WHERE "+whereClause, args...).Scan(&total); err != nil {
		return CharacterPage{}, err
	}
	q := `SELECT
		c.id,c.guild_id,c.name,c.display_name,c.normalized_name,c.realm,c.realm_slug,c.region,
		COALESCE(c.class_id,0),COALESCE(c.class_name,''),
		COALESCE(c.spec_id,0),COALESCE(c.spec_name,''),
		COALESCE(c.level,0),COALESCE(c.item_level,0),COALESCE(c.guild_rank,0),
		COALESCE((SELECT MAX(overall_rating) FROM mythic_plus m WHERE m.character_id=c.id),0),
		COALESCE((SELECT MAX(best_key_level) FROM mythic_plus m WHERE m.character_id=c.id),0),
		COALESCE(c.synced_at,'epoch')
		FROM characters c
		WHERE ` + whereClause + " ORDER BY " + sort.SQL()
	if descending {
		q += " DESC"
	} else {
		q += " ASC"
	}
	q += ", c.display_name ASC"
	if limit > 0 {
		args = append(args, limit, offset)
		q += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	}
	rows, e := s.pool.Query(ctx, q, args...)
	if e != nil {
		return CharacterPage{}, e
	}
	defer rows.Close()
	out := []domain.Character{}
	for rows.Next() {
		var c domain.Character
		e = rows.Scan(
			&c.ID, &c.GuildID,
			&c.Name, &c.DisplayName, &c.NormalizedName,
			&c.Realm, &c.RealmSlug, &c.Region,
			&c.ClassID, &c.ClassName,
			&c.SpecID, &c.SpecName,
			&c.Level, &c.ItemLevel, &c.GuildRank,
			&c.MythicRating, &c.BestKeyLevel, &c.SyncedAt,
		)
		if e != nil {
			return CharacterPage{}, e
		}
		out = append(out, c)
	}
	return CharacterPage{Items: out, Total: total}, rows.Err()
}
