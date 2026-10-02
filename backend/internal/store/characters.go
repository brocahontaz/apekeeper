package store

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"time"
)

type CharacterStore struct{ pool *pgxpool.Pool }
type CharacterFilter struct {
	Class, Spec                                                             string
	Rank, MinLevel                                                          *int
	MinRating                                                               *float64
	Name                                                                    string
	StaleBefore                                                             *time.Time
	MythicSeason, RaidTier, RaidDifficulty, OfficerStatus, OfficerTag, Role string
	MinRaidProgress                                                         *int
	MaxAge                                                                  *time.Duration
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
type GuildTrend struct {
	CapturedAt       time.Time        `json:"capturedAt"`
	AverageItemLevel float64          `json:"averageItemLevel"`
	AverageRating    float64          `json:"averageRating"`
	StaleCount       int              `json:"staleCount"`
	RaidProgress     []GuildTrendRaid `json:"raidProgress"`
}
type GuildTrendRaid struct {
	RaidName    string `json:"raidName"`
	Difficulty  string `json:"difficulty"`
	Progress    int    `json:"progress"`
	TotalBosses int    `json:"totalBosses"`
}

// DashboardCharacter pairs one character with the aggregates the dashboard
// used to load through a per-character Detail call: the best keystone level
// of the highest-rated mythic+ season, every raid progression row, and the
// oldest and newest snapshots inside the retention window.
type DashboardCharacter struct {
	Character     domain.Character
	BestKey       int
	Raids         []domain.RaidProgression
	SnapshotCount int
	First         domain.Snapshot
	Last          domain.Snapshot
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

// raceGenderFromProfile extracts the localized race and gender names from a
// Blizzard profile summary. Any parse failure yields empty strings; it must
// never fail the upsert.
func raceGenderFromProfile(profile []byte) (raceName, gender string) {
	var p struct {
		Race struct {
			Name json.RawMessage `json:"name"`
		} `json:"race"`
		Gender struct {
			Name json.RawMessage `json:"name"`
		} `json:"gender"`
	}
	if json.Unmarshal(profile, &p) != nil {
		return "", ""
	}
	localized := func(raw json.RawMessage) string {
		var m struct {
			EnUS string `json:"en_US"`
		}
		if json.Unmarshal(raw, &m) == nil && m.EnUS != "" {
			return m.EnUS
		}
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
		return ""
	}
	return localized(p.Race.Name), localized(p.Gender.Name)
}

func (s CharacterStore) UpsertByGuildIdentity(
	ctx context.Context,
	c domain.Character,
	profile []byte,
) (domain.Character, error) {
	raceName, gender := raceGenderFromProfile(profile)
	q := `INSERT INTO characters(
		guild_id,name,display_name,normalized_name,realm,realm_slug,region,
		class_id,class_name,spec_id,spec_name,level,item_level,guild_rank,
		race_name,gender,profile_json,synced_at
	)
	VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
	ON CONFLICT(guild_id,region,realm_slug,normalized_name) DO UPDATE SET
		display_name=EXCLUDED.display_name,
		class_id=EXCLUDED.class_id,
		class_name=EXCLUDED.class_name,
		spec_id=EXCLUDED.spec_id,
		spec_name=EXCLUDED.spec_name,
		level=EXCLUDED.level,
		item_level=EXCLUDED.item_level,
		guild_rank=EXCLUDED.guild_rank,
		race_name=EXCLUDED.race_name,
		gender=EXCLUDED.gender,
		profile_json=EXCLUDED.profile_json,
		synced_at=EXCLUDED.synced_at,
		updated_at=now()
	RETURNING id`
	e := s.pool.QueryRow(ctx, q,
		c.GuildID, c.Name, c.DisplayName, c.NormalizedName,
		c.Realm, c.RealmSlug, c.Region,
		c.ClassID, c.ClassName, c.SpecID, c.SpecName,
		c.Level, c.ItemLevel, c.GuildRank,
		raceName, gender,
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
		COALESCE(race_name,''),COALESCE(gender,''),
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
			&d.Character.RaceName, &d.Character.Gender,
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
		var runs []byte
		x.CharacterID = id
		if err = rows.Scan(
			&x.Season, &x.SeasonSlug, &x.OverallRating,
			&x.BestKeyLevel, &x.BestRunScore, &runs, &x.SyncedAt,
		); err != nil {
			return d, err
		}
		// Legacy rows store no runs snapshot; a null or unmarshalable value
		// must never fail the detail read, so Runs simply stays nil.
		if len(runs) > 0 {
			var parsed domain.MythicRuns
			if json.Unmarshal(runs, &parsed) == nil && (parsed.Best != nil || parsed.Recent != nil) {
				x.Runs = &parsed
			}
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
		id,captured_at,
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
		if err = rows.Scan(&x.ID, &x.CapturedAt, &x.ItemLevel, &x.MythicRating, &x.BestKeyLevel, &x.RaidProgress); err != nil {
			return d, err
		}
		d.Snapshots = append(d.Snapshots, x)
	}
	return d, rows.Err()
}

// TrendsByGuild uses one set-based snapshot query for the entire guild. Each
// character contributes only its latest snapshot on a day, so repeat syncs do
// not overweight a roster member. Characters are intentionally not filtered by
// freshness; inactive members with retained snapshots remain in the averages.
// Raid progress is expanded and reduced in SQL from the same representative
// snapshots, avoiding per-character history queries.
//
// existing progression_snapshots(character_id,captured_at) index serves the
// character lookup and time ordering; a guild-leading snapshot index would not
// be selective without duplicating guild_id on immutable snapshot rows.
func (s CharacterStore) TrendsByGuild(ctx context.Context, guildID int64, from, to, staleBefore time.Time, limit int) ([]GuildTrend, error) {
	rows, err := s.pool.Query(ctx, `WITH latest AS (
		SELECT DISTINCT ON (date_trunc('day',p.captured_at AT TIME ZONE 'UTC'),p.character_id)
			date_trunc('day',p.captured_at AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' AS captured_at,p.character_id,
			p.item_level,p.mythic_rating,p.raid_progress
		FROM progression_snapshots p JOIN characters c ON c.id=p.character_id
		WHERE c.guild_id=$1 AND p.captured_at >= $2 AND p.captured_at <= $3
		ORDER BY date_trunc('day',p.captured_at AT TIME ZONE 'UTC'),p.character_id,p.captured_at DESC,p.id DESC
	), daily AS (
		SELECT captured_at,AVG(COALESCE(item_level,0)) AS average_item_level,
			AVG(COALESCE(mythic_rating,0)) AS average_rating
		FROM latest GROUP BY captured_at
	), raid_rows AS (
		SELECT l.captured_at,instance->'instance'->>'name' AS raid_name,
			mode->'difficulty'->>'name' AS difficulty,
			MAX(COALESCE((mode->'progress'->>'completed_count')::int,0)) AS progress,
			MAX(COALESCE((mode->'progress'->>'total_count')::int,0)) AS total_bosses
		FROM latest l
		CROSS JOIN LATERAL jsonb_array_elements(COALESCE(l.raid_progress,'[]'::jsonb)->'expansions') expansion
		CROSS JOIN LATERAL jsonb_array_elements(COALESCE(expansion->'instances','[]'::jsonb)) instance
		CROSS JOIN LATERAL jsonb_array_elements(COALESCE(instance->'modes','[]'::jsonb)) mode
		WHERE COALESCE(instance->'instance'->>'name','') <> '' AND COALESCE(mode->'difficulty'->>'name','') <> ''
		GROUP BY l.captured_at,instance->'instance'->>'name',mode->'difficulty'->>'name'
	), raids AS (
		SELECT captured_at,jsonb_agg(jsonb_build_object('raidName',raid_name,'difficulty',difficulty,
			'progress',progress,'totalBosses',total_bosses) ORDER BY raid_name,difficulty) AS raid_progress
		FROM raid_rows GROUP BY captured_at
	), stale AS (
		SELECT COUNT(*) AS count FROM characters WHERE guild_id=$1 AND (synced_at IS NULL OR synced_at < $4)
	) SELECT daily.captured_at,daily.average_item_level,daily.average_rating,stale.count,
		COALESCE(raids.raid_progress,'[]'::jsonb)
	FROM daily CROSS JOIN stale LEFT JOIN raids ON raids.captured_at=daily.captured_at
	ORDER BY daily.captured_at LIMIT $5`, guildID, from, to, staleBefore, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GuildTrend{}
	for rows.Next() {
		var x GuildTrend
		var raids []byte
		if err := rows.Scan(&x.CapturedAt, &x.AverageItemLevel, &x.AverageRating, &x.StaleCount, &raids); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raids, &x.RaidProgress); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
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

// DashboardByGuild loads every character of a guild plus the dashboard's
// aggregates in ONE query: a single round trip replaces the per-character
// Detail lookups. Rows come back grouped by character, ordered by display
// name like the list view, so callers can aggregate in the same order.
//
// The aggregates are computed in per-character CTEs that are joined to the
// characters LAST. The previous shape joined raid rows before the correlated
// mythic+ subselects and the snapshot laterals, so a character with N raid
// rows paid those aggregates N times; grouping first makes each character's
// aggregates cost exactly once regardless of its raid count.
//
// No migration is required: mythic_plus UNIQUE(character_id,season_slug),
// raid_progression UNIQUE(character_id,raid_slug,difficulty), and
// progression_snapshots INDEX(character_id,captured_at) already give every
// CTE a character-leading index to aggregate through.
func (s CharacterStore) DashboardByGuild(ctx context.Context, guildID int64, since time.Time) ([]DashboardCharacter, error) {
	rows, e := s.pool.Query(ctx, `WITH guild_chars AS (
			SELECT id FROM characters WHERE guild_id=$1
		),
		mythic AS (
			SELECT m.character_id,
				MAX(m.overall_rating) AS best_rating,
				(array_agg(m.best_key_level ORDER BY m.overall_rating DESC,m.best_key_level DESC))[1] AS best_key
			FROM mythic_plus m
			JOIN guild_chars gc ON gc.id=m.character_id
			GROUP BY m.character_id
		),
		raids AS (
			SELECT r.character_id,
				array_agg(r ORDER BY r.raid_slug,r.raid_name,r.difficulty) AS raid_rows
			FROM raid_progression r
			JOIN guild_chars gc ON gc.id=r.character_id
			GROUP BY r.character_id
		),
		snaps AS (
			SELECT p.character_id,
				COUNT(*) AS cnt,
				(array_agg(p ORDER BY p.captured_at))[1] AS first_snap,
				(array_agg(p ORDER BY p.captured_at DESC))[1] AS last_snap
			FROM progression_snapshots p
			JOIN guild_chars gc ON gc.id=p.character_id
			WHERE p.captured_at >= $2
			GROUP BY p.character_id
		)
		SELECT
			c.id,c.guild_id,c.name,c.display_name,c.normalized_name,c.realm,c.realm_slug,c.region,
			COALESCE(c.class_id,0),COALESCE(c.class_name,''),
			COALESCE(c.spec_id,0),COALESCE(c.spec_name,''),
			COALESCE(c.level,0),COALESCE(c.item_level,0),COALESCE(c.guild_rank,0),
			COALESCE(c.race_name,''),COALESCE(c.gender,''),
			COALESCE(my.best_rating,0),
			COALESCE(my.best_key,0),
			COALESCE(c.synced_at,'epoch'),
			COALESCE(rx.raid_slug,''),COALESCE(rx.raid_name,''),COALESCE(rx.difficulty,''),
			COALESCE(rx.progress,0),COALESCE(rx.total_bosses,0),
			COALESCE(sn.cnt,0),
			COALESCE((sn.first_snap).captured_at,'epoch'),COALESCE((sn.first_snap).item_level,0),COALESCE((sn.first_snap).mythic_rating,0),
			COALESCE((sn.last_snap).captured_at,'epoch'),COALESCE((sn.last_snap).item_level,0),COALESCE((sn.last_snap).mythic_rating,0)
		FROM characters c
		LEFT JOIN mythic my ON my.character_id=c.id
		LEFT JOIN raids rd ON rd.character_id=c.id
		LEFT JOIN snaps sn ON sn.character_id=c.id
		-- The raid array expands after every aggregate has been attached, so
		-- characters without raids survive the LEFT JOIN with a NULL composite.
		LEFT JOIN LATERAL unnest(rd.raid_rows) AS rx ON true
		WHERE c.guild_id=$1
		ORDER BY c.display_name,c.id`, guildID, since)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []DashboardCharacter{}
	for rows.Next() {
		var (
			c                    DashboardCharacter
			raid                 domain.RaidProgression
			raidSlug, raidName   string
			firstAt, lastAt      time.Time
			firstIl, firstRating float64
			lastIl, lastRating   float64
		)
		if e = rows.Scan(
			&c.Character.ID, &c.Character.GuildID,
			&c.Character.Name, &c.Character.DisplayName, &c.Character.NormalizedName,
			&c.Character.Realm, &c.Character.RealmSlug, &c.Character.Region,
			&c.Character.ClassID, &c.Character.ClassName,
			&c.Character.SpecID, &c.Character.SpecName,
			&c.Character.Level, &c.Character.ItemLevel, &c.Character.GuildRank,
			&c.Character.RaceName, &c.Character.Gender,
			&c.Character.MythicRating, &c.BestKey, &c.Character.SyncedAt,
			&raidSlug, &raidName, &raid.Difficulty, &raid.Progress, &raid.TotalBosses,
			&c.SnapshotCount,
			&firstAt, &firstIl, &firstRating,
			&lastAt, &lastIl, &lastRating,
		); e != nil {
			return nil, e
		}
		// Rows are grouped by character with the raid array expanded after
		// each one, so only the first row of each character opens its group;
		// raid rows attach to the current one.
		if len(out) == 0 || out[len(out)-1].Character.ID != c.Character.ID {
			c.First = domain.Snapshot{CharacterID: c.Character.ID, CapturedAt: firstAt, ItemLevel: firstIl, MythicRating: firstRating}
			c.Last = domain.Snapshot{CharacterID: c.Character.ID, CapturedAt: lastAt, ItemLevel: lastIl, MythicRating: lastRating}
			out = append(out, c)
		}
		if raidSlug != "" {
			raid.RaidSlug, raid.RaidName, raid.CharacterID = raidSlug, raidName, c.Character.ID
			out[len(out)-1].Raids = append(out[len(out)-1].Raids, raid)
		}
	}
	return out, rows.Err()
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

func (s CharacterStore) ListSpecsByGuild(ctx context.Context, guildID int64) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT spec_name FROM characters WHERE guild_id=$1 AND COALESCE(spec_name,'') <> '' ORDER BY spec_name`, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	specs := []string{}
	for rows.Next() {
		var spec string
		if err := rows.Scan(&spec); err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	return specs, rows.Err()
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
		if f.MythicSeason != "" {
			args = append(args, f.MythicSeason, *f.MinRating)
			where = append(where, fmt.Sprintf("EXISTS (SELECT 1 FROM mythic_plus m WHERE m.character_id=c.id AND m.season_slug=$%d AND m.overall_rating >= $%d)", len(args)-1, len(args)))
		} else {
			add("EXISTS (SELECT 1 FROM mythic_plus m WHERE m.character_id=c.id AND m.overall_rating >= $%d)", *f.MinRating)
		}
	} else if f.MythicSeason != "" {
		add("EXISTS (SELECT 1 FROM mythic_plus m WHERE m.character_id=c.id AND m.season_slug=$%d)", f.MythicSeason)
	}
	if f.RaidTier != "" || f.RaidDifficulty != "" || f.MinRaidProgress != nil {
		conditions := []string{"r.character_id=c.id"}
		if f.RaidTier != "" {
			args = append(args, f.RaidTier, f.RaidTier)
			conditions = append(conditions, fmt.Sprintf("(r.raid_slug=$%d OR r.raid_name=$%d)", len(args)-1, len(args)))
		}
		if f.RaidDifficulty != "" {
			args = append(args, f.RaidDifficulty)
			conditions = append(conditions, fmt.Sprintf("r.difficulty=$%d", len(args)))
		}
		if f.MinRaidProgress != nil {
			args = append(args, *f.MinRaidProgress)
			conditions = append(conditions, fmt.Sprintf("COALESCE(r.progress,0)>=$%d", len(args)))
		}
		where = append(where, "EXISTS (SELECT 1 FROM raid_progression r WHERE "+strings.Join(conditions, " AND ")+")")
	}
	if f.MaxAge != nil {
		add("(c.synced_at IS NOT NULL AND c.synced_at >= now() - ($%d * interval '1 day'))", f.MaxAge.Hours()/24)
	}
	if f.OfficerStatus != "" {
		add("EXISTS (SELECT 1 FROM officer_character_state os WHERE os.character_id=c.id AND os.lifecycle_status=$%d)", f.OfficerStatus)
	}
	if f.OfficerTag != "" {
		add("EXISTS (SELECT 1 FROM officer_character_state os WHERE os.character_id=c.id AND $%d = ANY(os.tags))", f.OfficerTag)
	}
	if f.Role != "" {
		add("COALESCE(u.app_role,'member')=$%d", f.Role)
	}
	if f.Name != "" {
		add("c.normalized_name LIKE $%d", "%"+domain.NormalizeCharacterName(f.Name)+"%")
	}
	if f.StaleBefore != nil {
		add("(c.synced_at IS NULL OR c.synced_at < $%d)", *f.StaleBefore)
	}
	whereClause := strings.Join(where, " AND ")
	var total int
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM characters c LEFT JOIN users u ON u.id=c.user_id WHERE "+whereClause, args...).Scan(&total); err != nil {
		return CharacterPage{}, err
	}
	q := `SELECT
		c.id,c.guild_id,c.name,c.display_name,c.normalized_name,c.realm,c.realm_slug,c.region,
		COALESCE(c.class_id,0),COALESCE(c.class_name,''),
		COALESCE(c.spec_id,0),COALESCE(c.spec_name,''),
		COALESCE(c.level,0),COALESCE(c.item_level,0),COALESCE(c.guild_rank,0),
		COALESCE(c.race_name,''),COALESCE(c.gender,''),
		COALESCE(u.app_role,'member'),
		COALESCE((SELECT MAX(overall_rating) FROM mythic_plus m WHERE m.character_id=c.id),0),
		COALESCE((SELECT MAX(best_key_level) FROM mythic_plus m WHERE m.character_id=c.id),0),
		COALESCE(c.synced_at,'epoch')
		FROM characters c LEFT JOIN users u ON u.id=c.user_id
		WHERE ` + whereClause + " ORDER BY " + sort.SQL()
	if descending {
		q += " DESC"
	} else {
		q += " ASC"
	}
	q += ", c.display_name ASC, c.id ASC"
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
			&c.RaceName, &c.Gender,
			&c.Role,
			&c.MythicRating, &c.BestKeyLevel, &c.SyncedAt,
		)
		if e != nil {
			return CharacterPage{}, e
		}
		out = append(out, c)
	}
	return CharacterPage{Items: out, Total: total}, rows.Err()
}
