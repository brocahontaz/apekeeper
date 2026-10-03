package store

import "fmt"

// ReadPlan is the explainable SQL shape used by a high-volume read. The
// hooks are intentionally exported for integration tests and diagnostics;
// production callers still use the typed methods on the stores.
type ReadPlan struct {
	Name string
	SQL  string
}

const DashboardQuery = `WITH guild_chars AS (
	SELECT id FROM characters WHERE guild_id=$1
),
mythic AS (
	SELECT m.character_id,
		MAX(m.overall_rating) AS best_rating,
		(array_agg(m.best_key_level ORDER BY m.overall_rating DESC,m.best_key_level DESC))[1] AS best_key
	FROM mythic_plus m JOIN guild_chars gc ON gc.id=m.character_id GROUP BY m.character_id
),
raids AS (
	SELECT r.character_id, array_agg(r ORDER BY r.raid_slug,r.raid_name,r.difficulty) AS raid_rows
	FROM raid_progression r JOIN guild_chars gc ON gc.id=r.character_id GROUP BY r.character_id
),
snaps AS (
	SELECT p.character_id, COUNT(*) AS cnt,
		(array_agg(p ORDER BY p.captured_at))[1] AS first_snap,
		(array_agg(p ORDER BY p.captured_at DESC))[1] AS last_snap
	FROM progression_snapshots p JOIN guild_chars gc ON gc.id=p.character_id
	WHERE p.captured_at >= $2 GROUP BY p.character_id
)
SELECT c.id,c.guild_id,c.name,c.display_name,c.normalized_name,c.realm,c.realm_slug,c.region,
	COALESCE(c.class_id,0),COALESCE(c.class_name,''),COALESCE(c.spec_id,0),COALESCE(c.spec_name,''),
	COALESCE(c.level,0),COALESCE(c.item_level,0),COALESCE(c.guild_rank,0),COALESCE(c.race_name,''),COALESCE(c.gender,''),
	COALESCE(my.best_rating,0),COALESCE(my.best_key,0),COALESCE(c.synced_at,'epoch'),
	COALESCE(rx.raid_slug,''),COALESCE(rx.raid_name,''),COALESCE(rx.difficulty,''),COALESCE(rx.progress,0),COALESCE(rx.total_bosses,0),
	COALESCE(sn.cnt,0),COALESCE((sn.first_snap).captured_at,'epoch'),COALESCE((sn.first_snap).item_level,0),COALESCE((sn.first_snap).mythic_rating,0),
	COALESCE((sn.last_snap).captured_at,'epoch'),COALESCE((sn.last_snap).item_level,0),COALESCE((sn.last_snap).mythic_rating,0)
FROM characters c LEFT JOIN mythic my ON my.character_id=c.id LEFT JOIN raids rd ON rd.character_id=c.id LEFT JOIN snaps sn ON sn.character_id=c.id
LEFT JOIN LATERAL unnest(rd.raid_rows) AS rx ON true WHERE c.guild_id=$1 ORDER BY c.display_name,c.id`

const HistoryQuery = `SELECT p.id,p.character_id,p.captured_at,
	COALESCE(p.item_level,0),COALESCE(p.mythic_rating,0),COALESCE(p.best_key_level,0),COALESCE(p.raid_progress,'[]')
	FROM progression_snapshots p JOIN characters c ON c.id=p.character_id
	WHERE c.guild_id=$1 AND p.character_id=$2 AND p.captured_at >= $3 AND p.captured_at <= $4
	ORDER BY p.captured_at DESC,p.id DESC LIMIT $5`

const SyncHistoryQuery = `SELECT id,guild_id,started_at,finished_at,trigger,status,COALESCE(characters_total,0),COALESCE(characters_updated,0),COALESCE(characters_failed,0),COALESCE(error_summary,''),COALESCE(detail,'{}'),notify_status FROM sync_runs WHERE guild_id=$1 ORDER BY started_at DESC LIMIT $2`

func RosterCountQuery(where string) string {
	return "SELECT COUNT(*) FROM characters c LEFT JOIN users u ON u.id=c.user_id WHERE " + where
}
func RosterSelectQuery(where string, sort CharacterSort, descending bool, argCount int, paginated bool) string {
	q := `SELECT c.id,c.guild_id,c.name,c.display_name,c.normalized_name,c.realm,c.realm_slug,c.region,
		COALESCE(c.class_id,0),COALESCE(c.class_name,''),COALESCE(c.spec_id,0),COALESCE(c.spec_name,''),
		COALESCE(c.level,0),COALESCE(c.item_level,0),COALESCE(c.guild_rank,0),COALESCE(c.race_name,''),COALESCE(c.gender,''),
		COALESCE(u.app_role,'member'),COALESCE((SELECT MAX(overall_rating) FROM mythic_plus m WHERE m.character_id=c.id),0),
		COALESCE((SELECT MAX(best_key_level) FROM mythic_plus m WHERE m.character_id=c.id),0),COALESCE(c.synced_at,'epoch')
		FROM characters c LEFT JOIN users u ON u.id=c.user_id WHERE ` + where + " ORDER BY " + sort.SQL()
	if descending {
		q += " DESC"
	} else {
		q += " ASC"
	}
	q += ", c.display_name ASC, c.id ASC"
	if paginated {
		q += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argCount+1, argCount+2)
	}
	return q
}

// ReadPlans returns the stable, guild-scoped plan shapes used by the read
// methods. Keeping these shapes in one place prevents plan tests from drifting
// into unrelated hand-written SQL.
func ReadPlans() []ReadPlan {
	return []ReadPlan{
		{Name: "dashboard", SQL: DashboardQuery},
		{Name: "roster", SQL: RosterSelectQuery("c.guild_id=$1", CharacterSortName, false, 1, true)},
		{Name: "export", SQL: RosterSelectQuery("c.guild_id=$1", CharacterSortName, false, 1, false)},
		{Name: "history", SQL: HistoryQuery},
		{Name: "sync history", SQL: SyncHistoryQuery},
	}
}
