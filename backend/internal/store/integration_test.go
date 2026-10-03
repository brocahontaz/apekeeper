package store_test

import (
	"context"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/brocahontaz/apekeeper/backend/internal/store"
	"github.com/brocahontaz/apekeeper/backend/internal/testdb"
)

func TestOAuthStateConsumeIsAtomicAndSingleUse(t *testing.T) {
	ctx := context.Background()
	p, _ := testdb.New(t)
	s := store.New(p)
	state := "integration-oauth-state-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	now := time.Now().Truncate(time.Microsecond)
	if err := s.Users.CreateOAuthState(ctx, state, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	results := make(chan bool, 16)
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := s.Users.ConsumeOAuthState(ctx, state, now)
			if err != nil {
				t.Errorf("ConsumeOAuthState error = %v", err)
			}
			results <- ok
		}()
	}
	wg.Wait()
	close(results)
	accepted := 0
	for ok := range results {
		if ok {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted consumes = %d, want exactly one", accepted)
	}
}

// This intentionally skips in unit-only environments; docker compose supplies DATABASE_URL.
func TestMigrationAndEnsureGuild(t *testing.T) {
	p, _ := testdb.New(t)
	s := store.New(p)
	g := domain.Guild{Slug: "test-us-area-52-ape", Name: "Ape", Realm: "Area 52", Region: "us"}
	one, err := s.Guilds.EnsureGuild(context.Background(), g)
	if err != nil {
		t.Fatal(err)
	}
	two, err := s.Guilds.EnsureGuild(context.Background(), g)
	if err != nil {
		t.Fatal(err)
	}
	if one.ID != two.ID {
		t.Fatal("EnsureGuild was not idempotent")
	}
}

func TestListByGuildReturnsEachCharacterOnceAcrossMythicSeasons(t *testing.T) {
	ctx := context.Background()
	p, _ := testdb.New(t)
	s := store.New(p)
	g, err := s.Guilds.EnsureGuild(ctx, domain.Guild{Slug: "character-list", Name: "Ape", Realm: "Area 52", Region: "us"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	c, err := s.Characters.UpsertByGuildIdentity(ctx, domain.Character{
		GuildID:        g.ID,
		Name:           "One",
		DisplayName:    "One",
		NormalizedName: "one",
		Realm:          "Area 52",
		RealmSlug:      "area-52",
		Region:         "us",
		SyncedAt:       now,
	}, []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []domain.MythicPlus{
		{CharacterID: c.ID, SeasonSlug: "one", OverallRating: 1000, BestKeyLevel: 12, SyncedAt: now},
		{CharacterID: c.ID, SeasonSlug: "two", OverallRating: 2000, BestKeyLevel: 15, SyncedAt: now},
	} {
		if err := s.Progression.UpsertMythicPlus(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	minRating := 1500.0
	rows, err := s.Characters.ListByGuild(ctx, g.ID, store.CharacterFilter{MinRating: &minRating})
	if err != nil || len(rows) != 1 || rows[0].ID != c.ID ||
		rows[0].MythicRating != 2000 || rows[0].BestKeyLevel != 15 {
		t.Fatalf("qualified rows=%+v err=%v", rows, err)
	}
	minRating = 2500
	rows, err = s.Characters.ListByGuild(ctx, g.ID, store.CharacterFilter{MinRating: &minRating})
	if err != nil || len(rows) != 0 {
		t.Fatalf("unqualified rows=%+v err=%v", rows, err)
	}
	rows, err = s.Characters.ListByGuild(ctx, g.ID, store.CharacterFilter{})
	if err != nil || len(rows) != 1 || rows[0].ID != c.ID {
		t.Fatalf("unfiltered rows=%+v err=%v", rows, err)
	}
}

// DashboardByGuild must return every character once, grouped with its best
// season key, all raid rows, and the snapshot window's first and last entries
// — the aggregates the dashboard used to load with one Detail call each.
func TestDashboardByGuildAggregatesCharactersAndProgression(t *testing.T) {
	ctx := context.Background()
	p, _ := testdb.New(t)
	s := store.New(p)
	g, err := s.Guilds.EnsureGuild(ctx, domain.Guild{Slug: "dashboard-by-guild", Name: "Ape", Realm: "Area 52", Region: "us"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	upsert := func(name string) domain.Character {
		t.Helper()
		c, err := s.Characters.UpsertByGuildIdentity(ctx, domain.Character{
			GuildID:        g.ID,
			Name:           name,
			DisplayName:    name,
			NormalizedName: name,
			Realm:          "Area 52",
			RealmSlug:      "area-52",
			Region:         "us",
			ClassName:      "Mage",
			SpecName:       "Arcane",
			Level:          70,
			SyncedAt:       now,
		}, []byte("{}"))
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	one, two, three := upsert("One"), upsert("Two"), upsert("Three")
	// Season one carries the higher keystone level but the lower rating; the
	// dashboard's BestKey must come from the best-rated row, not the max key.
	for _, m := range []domain.MythicPlus{
		{CharacterID: one.ID, SeasonSlug: "one", OverallRating: 1000, BestKeyLevel: 15, SyncedAt: now},
		{CharacterID: one.ID, SeasonSlug: "two", OverallRating: 2000, BestKeyLevel: 12, SyncedAt: now},
		{CharacterID: three.ID, SeasonSlug: "one", OverallRating: 1000, BestKeyLevel: 20, SyncedAt: now},
		{CharacterID: three.ID, SeasonSlug: "two", OverallRating: 3000, BestKeyLevel: 8, SyncedAt: now},
	} {
		if err := s.Progression.UpsertMythicPlus(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	// Three's raid rows are inserted out of slug order so the dashboard's
	// deterministic raid ordering is observable, and Three carries raid rows
	// AND multiple mythic seasons at once — the shape that repeated the
	// correlated aggregates per raid row before the grouped rewrite.
	if err := s.Progression.ReplaceRaids(ctx, two.ID, []domain.RaidProgression{
		{CharacterID: two.ID, RaidSlug: "raid-a", RaidName: "Raid A", Difficulty: "heroic", Progress: 4, TotalBosses: 8, SyncedAt: now},
		{CharacterID: two.ID, RaidSlug: "raid-b", RaidName: "Raid B", Difficulty: "normal", Progress: 2, TotalBosses: 5, SyncedAt: now},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Progression.ReplaceRaids(ctx, three.ID, []domain.RaidProgression{
		{CharacterID: three.ID, RaidSlug: "raid-zeta", RaidName: "Raid Zeta", Difficulty: "normal", Progress: 1, TotalBosses: 5, SyncedAt: now},
		{CharacterID: three.ID, RaidSlug: "raid-alpha", RaidName: "Raid Alpha", Difficulty: "heroic", Progress: 3, TotalBosses: 8, SyncedAt: now},
	}); err != nil {
		t.Fatal(err)
	}
	old, mid, fresh := now.AddDate(0, 0, -40), now.AddDate(0, 0, -20), now.AddDate(0, 0, -2)
	for _, snap := range []domain.Snapshot{
		{CharacterID: two.ID, CapturedAt: old, ItemLevel: 400, MythicRating: 100, RaidProgress: []byte("[]")},
		{CharacterID: two.ID, CapturedAt: mid, ItemLevel: 500, MythicRating: 1500, RaidProgress: []byte("[]")},
		{CharacterID: two.ID, CapturedAt: fresh, ItemLevel: 600, MythicRating: 2000, RaidProgress: []byte("[]")},
		{CharacterID: three.ID, CapturedAt: old, ItemLevel: 400, MythicRating: 100, RaidProgress: []byte("[]")},
		{CharacterID: three.ID, CapturedAt: now.AddDate(0, 0, -10), ItemLevel: 500, MythicRating: 1200, RaidProgress: []byte("[]")},
		{CharacterID: three.ID, CapturedAt: now.AddDate(0, 0, -1), ItemLevel: 700, MythicRating: 2500, RaidProgress: []byte("[]")},
	} {
		if err := s.Progression.InsertSnapshot(ctx, snap); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.Characters.DashboardByGuild(ctx, g.ID, now.AddDate(0, 0, -30))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 ||
		rows[0].Character.DisplayName != "One" ||
		rows[1].Character.DisplayName != "Three" ||
		rows[2].Character.DisplayName != "Two" {
		t.Fatalf("rows=%+v", rows)
	}
	if rows[0].Character.MythicRating != 2000 || rows[0].BestKey != 12 {
		t.Fatalf("One mythic=%v bestKey=%d, want rating 2000 from the season-two row", rows[0].Character.MythicRating, rows[0].BestKey)
	}
	if len(rows[0].Raids) != 0 || rows[0].SnapshotCount != 0 {
		t.Fatalf("One raids=%d snapshots=%d, want none", len(rows[0].Raids), rows[0].SnapshotCount)
	}
	if rows[1].Character.MythicRating != 3000 || rows[1].BestKey != 8 {
		t.Fatalf("Three mythic=%v bestKey=%d, want rating 3000 with key 8 from the season-two row", rows[1].Character.MythicRating, rows[1].BestKey)
	}
	if len(rows[1].Raids) != 2 ||
		rows[1].Raids[0].RaidSlug != "raid-alpha" || rows[1].Raids[1].RaidSlug != "raid-zeta" {
		t.Fatalf("Three raids=%+v, want both rows in slug order", rows[1].Raids)
	}
	if rows[1].SnapshotCount != 2 ||
		!rows[1].First.CapturedAt.Equal(now.AddDate(0, 0, -10)) || rows[1].First.ItemLevel != 500 ||
		!rows[1].Last.CapturedAt.Equal(now.AddDate(0, 0, -1)) || rows[1].Last.ItemLevel != 700 {
		t.Fatalf("Three snapshots count=%d first=%+v last=%+v, want the two in-window snapshots",
			rows[1].SnapshotCount, rows[1].First, rows[1].Last)
	}
	if len(rows[2].Raids) != 2 ||
		rows[2].Raids[0].RaidSlug != "raid-a" || rows[2].Raids[1].RaidSlug != "raid-b" {
		t.Fatalf("Two raids=%+v", rows[2].Raids)
	}
	if rows[2].SnapshotCount != 2 ||
		!rows[2].First.CapturedAt.Equal(mid) || rows[2].First.ItemLevel != 500 ||
		!rows[2].Last.CapturedAt.Equal(fresh) || rows[2].Last.MythicRating != 2000 {
		t.Fatalf("Two snapshots count=%d first=%+v last=%+v, want the two in-window snapshots",
			rows[2].SnapshotCount, rows[2].First, rows[2].Last)
	}
}

func TestUpsertByGuildIdentityPopulatesRaceAndGender(t *testing.T) {
	ctx := context.Background()
	p, _ := testdb.New(t)
	s := store.New(p)
	g, err := s.Guilds.EnsureGuild(ctx, domain.Guild{Slug: "race-gender", Name: "Ape", Realm: "Area 52", Region: "us"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	dwarf := []byte(`{
		"name": "One",
		"race": {"key": {"href": "https://us.api.blizzard.com/data/wow/playable-race/5"}, "id": 5, "name": {"en_US": "Dwarf", "es_MX": "Enano"}},
		"gender": {"type": "MALE", "name": {"en_US": "Male", "es_MX": "Masculino"}}
	}`)
	c, err := s.Characters.UpsertByGuildIdentity(ctx, domain.Character{
		GuildID:        g.ID,
		Name:           "One",
		DisplayName:    "One",
		NormalizedName: "one",
		Realm:          "Area 52",
		RealmSlug:      "area-52",
		Region:         "us",
		SyncedAt:       now,
	}, dwarf)
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.Characters.Detail(ctx, g.ID, c.ID, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if d.Character.RaceName != "Dwarf" || d.Character.Gender != "Male" {
		t.Fatalf("detail race/gender=%q/%q, want Dwarf/Male", d.Character.RaceName, d.Character.Gender)
	}
	// A re-sync with a different profile refreshes the columns via EXCLUDED.
	orc := []byte(`{
		"race": {"id": 2, "name": {"en_US": "Orc"}},
		"gender": {"type": "FEMALE", "name": {"en_US": "Female"}}
	}`)
	if _, err := s.Characters.UpsertByGuildIdentity(ctx, domain.Character{
		GuildID:        g.ID,
		Name:           "One",
		DisplayName:    "One",
		NormalizedName: "one",
		Realm:          "Area 52",
		RealmSlug:      "area-52",
		Region:         "us",
		SyncedAt:       now,
	}, orc); err != nil {
		t.Fatal(err)
	}
	d, err = s.Characters.Detail(ctx, g.ID, c.ID, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if d.Character.RaceName != "Orc" || d.Character.Gender != "Female" {
		t.Fatalf("refreshed race/gender=%q/%q, want Orc/Female", d.Character.RaceName, d.Character.Gender)
	}
	// An empty profile clears the columns instead of failing the upsert.
	if _, err := s.Characters.UpsertByGuildIdentity(ctx, domain.Character{
		GuildID:        g.ID,
		Name:           "One",
		DisplayName:    "One",
		NormalizedName: "one",
		Realm:          "Area 52",
		RealmSlug:      "area-52",
		Region:         "us",
		SyncedAt:       now,
	}, []byte("{}")); err != nil {
		t.Fatal(err)
	}
	d, err = s.Characters.Detail(ctx, g.ID, c.ID, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if d.Character.RaceName != "" || d.Character.Gender != "" {
		t.Fatalf("cleared race/gender=%q/%q, want empty", d.Character.RaceName, d.Character.Gender)
	}
	// The list view carries the same columns.
	orcAgain := []byte(`{"race": {"id": 2, "name": {"en_US": "Orc"}}, "gender": {"type": "FEMALE", "name": {"en_US": "Female"}}}`)
	if _, err := s.Characters.UpsertByGuildIdentity(ctx, domain.Character{
		GuildID:        g.ID,
		Name:           "One",
		DisplayName:    "One",
		NormalizedName: "one",
		Realm:          "Area 52",
		RealmSlug:      "area-52",
		Region:         "us",
		SyncedAt:       now,
	}, orcAgain); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Characters.ListByGuild(ctx, g.ID, store.CharacterFilter{})
	if err != nil || len(rows) != 1 {
		t.Fatalf("list rows=%+v err=%v", rows, err)
	}
	if rows[0].RaceName != "Orc" || rows[0].Gender != "Female" {
		t.Fatalf("list race/gender=%q/%q, want Orc/Female", rows[0].RaceName, rows[0].Gender)
	}
}

func TestMythicPlusRunsRoundTrip(t *testing.T) {
	ctx := context.Background()
	p, _ := testdb.New(t)
	s := store.New(p)
	g, err := s.Guilds.EnsureGuild(ctx, domain.Guild{Slug: "mythic-runs", Name: "Ape", Realm: "Area 52", Region: "us"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	c, err := s.Characters.UpsertByGuildIdentity(ctx, domain.Character{
		GuildID:        g.ID,
		Name:           "One",
		DisplayName:    "One",
		NormalizedName: "one",
		Realm:          "Area 52",
		RealmSlug:      "area-52",
		Region:         "us",
		SyncedAt:       now,
	}, []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	runs := &domain.MythicRuns{
		Best: []domain.MythicRun{
			{Dungeon: "Apex Canopy", Level: 5, Score: 320, Timed: true, CompletedAt: 1700000000000},
			{Dungeon: "Voidscar Arena", Level: 10, Score: 310, Timed: false, CompletedAt: 1700000001000},
		},
		Recent: []domain.MythicRun{
			{Dungeon: "Cinderbrew Meadery", Level: 12, Score: 305, Timed: true, CompletedAt: 1700000009000},
		},
	}
	if err := s.Progression.UpsertMythicPlus(ctx, domain.MythicPlus{
		CharacterID:   c.ID,
		Season:        "Season 2",
		SeasonSlug:    "season-2",
		OverallRating: 2500,
		BestRunScore:  320,
		BestKeyLevel:  10,
		Runs:          runs,
		SyncedAt:      now,
	}); err != nil {
		t.Fatal(err)
	}
	// A row without a runs snapshot is the legacy shape: Runs must stay nil.
	if err := s.Progression.UpsertMythicPlus(ctx, domain.MythicPlus{
		CharacterID:   c.ID,
		Season:        "Season 1",
		SeasonSlug:    "season-1",
		OverallRating: 1000,
		BestKeyLevel:  8,
		SyncedAt:      now,
	}); err != nil {
		t.Fatal(err)
	}
	d, err := s.Characters.Detail(ctx, g.ID, c.ID, time.Time{})
	if err != nil || len(d.Mythic) != 2 {
		t.Fatalf("detail=%+v err=%v", d, err)
	}
	for _, m := range d.Mythic {
		if m.SeasonSlug == "season-2" {
			if !reflect.DeepEqual(m.Runs, runs) {
				t.Fatalf("season-2 runs=%+v, want the stored snapshot", m.Runs)
			}
		} else if m.SeasonSlug == "season-1" && m.Runs != nil {
			t.Fatalf("season-1 runs=%+v, want nil without a snapshot", m.Runs)
		}
	}
}

// UpsertMythicPlusRuns refreshes run lists without clobbering a stored rating
// summary, and a fresh insert for an unseen season still writes the zeroed
// summary.
func TestUpsertMythicPlusRunsPreservesStoredSummary(t *testing.T) {
	ctx := context.Background()
	p, _ := testdb.New(t)
	s := store.New(p)
	g, err := s.Guilds.EnsureGuild(ctx, domain.Guild{Slug: "mythic-runs", Name: "Ape", Realm: "Area 52", Region: "us"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c, err := s.Characters.UpsertByGuildIdentity(ctx, domain.Character{
		GuildID:        g.ID,
		Name:           "One",
		DisplayName:    "One",
		NormalizedName: "one",
		Realm:          "Area 52",
		RealmSlug:      "area-52",
		Region:         "us",
		SyncedAt:       now,
	}, []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Progression.UpsertMythicPlus(ctx, domain.MythicPlus{
		CharacterID:   c.ID,
		Season:        "Season 2",
		SeasonSlug:    "18",
		OverallRating: 2500,
		BestRunScore:  500,
		BestKeyLevel:  12,
		SyncedAt:      now,
	}); err != nil {
		t.Fatal(err)
	}
	newer := now.Add(time.Hour)
	runs := &domain.MythicRuns{Recent: []domain.MythicRun{
		{Dungeon: "Cinderbrew Meadery", Level: 12, Score: 110, Timed: true, CompletedAt: 1700000007000},
	}}
	if err := s.Progression.UpsertMythicPlusRuns(ctx, domain.MythicPlus{
		CharacterID: c.ID,
		Season:      "Season 2",
		SeasonSlug:  "18",
		Runs:        runs,
		SyncedAt:    newer,
	}); err != nil {
		t.Fatal(err)
	}
	d, err := s.Characters.Detail(ctx, g.ID, c.ID, time.Time{})
	if err != nil || len(d.Mythic) != 1 {
		t.Fatalf("detail=%+v err=%v", d, err)
	}
	m := d.Mythic[0]
	if m.OverallRating != 2500 || m.BestKeyLevel != 12 || m.BestRunScore != 500 {
		t.Fatalf("mythic=%+v, want the stored rating summary untouched", m)
	}
	if !reflect.DeepEqual(m.Runs, runs) {
		t.Fatalf("runs=%+v, want the refreshed run lists", m.Runs)
	}
	if !m.SyncedAt.Equal(newer) {
		t.Fatalf("synced_at=%v, want %v", m.SyncedAt, newer)
	}
	if err := s.Progression.UpsertMythicPlusRuns(ctx, domain.MythicPlus{
		CharacterID: c.ID,
		Season:      "Season 3",
		SeasonSlug:  "19",
		Runs:        runs,
		SyncedAt:    newer,
	}); err != nil {
		t.Fatal(err)
	}
	d, err = s.Characters.Detail(ctx, g.ID, c.ID, time.Time{})
	if err != nil || len(d.Mythic) != 2 {
		t.Fatalf("detail=%+v err=%v", d, err)
	}
	for _, m := range d.Mythic {
		if m.SeasonSlug == "19" && (m.OverallRating != 0 || m.BestKeyLevel != 0 || m.BestRunScore != 0) {
			t.Fatalf("season 19 mythic=%+v, want a zeroed summary on fresh insert", m)
		}
	}
}

func TestReplaceRaidsSwapsCharacterRows(t *testing.T) {
	ctx := context.Background()
	p, _ := testdb.New(t)
	s := store.New(p)
	g, err := s.Guilds.EnsureGuild(ctx, domain.Guild{Slug: "raid-replace", Name: "Ape", Realm: "Area 52", Region: "us"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	c, err := s.Characters.UpsertByGuildIdentity(ctx, domain.Character{
		GuildID:        g.ID,
		Name:           "One",
		DisplayName:    "One",
		NormalizedName: "one",
		Realm:          "Area 52",
		RealmSlug:      "area-52",
		Region:         "us",
		SyncedAt:       now,
	}, []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	legacy := []domain.RaidProgression{
		{CharacterID: c.ID, RaidSlug: "emerald-nightmare", RaidName: "Emerald Nightmare",
			Difficulty: "Normal", Progress: 7, TotalBosses: 7, Summary: []byte("{}"), SyncedAt: now},
		{CharacterID: c.ID, RaidSlug: "nerubar-palace", RaidName: "Nerub-ar Palace",
			Difficulty: "Heroic", Progress: 8, TotalBosses: 8, Summary: []byte("{}"), SyncedAt: now},
	}
	if err := s.Progression.ReplaceRaids(ctx, c.ID, legacy); err != nil {
		t.Fatal(err)
	}
	replacement := []domain.RaidProgression{
		{CharacterID: c.ID, RaidSlug: "midnight-raid", RaidName: "Midnight Raid",
			Difficulty: "Normal", Progress: 1, TotalBosses: 4, Summary: []byte("{}"), SyncedAt: now},
	}
	if err := s.Progression.ReplaceRaids(ctx, c.ID, replacement); err != nil {
		t.Fatal(err)
	}
	d, err := s.Characters.Detail(ctx, g.ID, c.ID, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Raids) != 1 || d.Raids[0].RaidSlug != "midnight-raid" ||
		d.Raids[0].RaidName != "Midnight Raid" || d.Raids[0].Progress != 1 || d.Raids[0].TotalBosses != 4 {
		t.Fatalf("raids=%+v, want only the replacement row", d.Raids)
	}
	// An empty replacement set clears every stored row.
	if err := s.Progression.ReplaceRaids(ctx, c.ID, nil); err != nil {
		t.Fatal(err)
	}
	d, err = s.Characters.Detail(ctx, g.ID, c.ID, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Raids) != 0 {
		t.Fatalf("raids=%+v, want zero rows after an empty replacement", d.Raids)
	}
}

func TestUpsertBattleNetUserAssignsRoles(t *testing.T) {
	ctx := context.Background()
	p, _ := testdb.New(t)
	s := store.New(p)
	expires := time.Now().Add(time.Hour)
	upsert := func(bnetID, battletag string, superadmin bool) domain.User {
		t.Helper()
		u, err := s.Users.UpsertBattleNetUser(ctx, bnetID, battletag, "", "", expires, superadmin)
		if err != nil {
			t.Fatalf("UpsertBattleNetUser(%s) error = %v", battletag, err)
		}
		return u
	}

	first := upsert("1", "First#1", false)
	if first.Role != "admin" {
		t.Fatalf("first user role = %q, want admin", first.Role)
	}
	second := upsert("2", "Second#2", false)
	if second.Role != "member" {
		t.Fatalf("second user role = %q, want member", second.Role)
	}
	super := upsert("3", "Super#3", true)
	if super.Role != "superadmin" {
		t.Fatalf("flagged new user role = %q, want superadmin", super.Role)
	}
	promoted := upsert("1", "First#1", true)
	if promoted.ID != first.ID || promoted.Role != "superadmin" {
		t.Fatalf("flagged existing user = %+v, want upgraded superadmin", promoted)
	}
	staysMember := upsert("2", "Second#2", false)
	if staysMember.ID != second.ID || staysMember.Role != "member" {
		t.Fatalf("unflagged member = %+v, want untouched member", staysMember)
	}
	keepsRole := upsert("3", "Super#3", false)
	if keepsRole.ID != super.ID || keepsRole.Role != "superadmin" {
		t.Fatalf("unflagged superadmin = %+v, want untouched superadmin", keepsRole)
	}
}

func TestUpsertBattleNetUserFirstSuperAdminPrecedesBootstrapAdmin(t *testing.T) {
	ctx := context.Background()
	p, _ := testdb.New(t)
	s := store.New(p)
	u, err := s.Users.UpsertBattleNetUser(
		ctx, "1", "Owner#2576", "", "", time.Now().Add(time.Hour), true)
	if err != nil {
		t.Fatal(err)
	}
	if u.Role != "superadmin" {
		t.Fatalf("first flagged user role = %q, want superadmin", u.Role)
	}
}

func TestDeleteSnapshotsBeforeRemovesOnlyOldSnapshots(t *testing.T) {
	ctx := context.Background()
	p, _ := testdb.New(t)
	s := store.New(p)
	g, err := s.Guilds.EnsureGuild(ctx, domain.Guild{Slug: "snapshot-retention", Name: "Ape", Realm: "Area 52", Region: "us"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	c, err := s.Characters.UpsertByGuildIdentity(ctx, domain.Character{
		GuildID:        g.ID,
		Name:           "One",
		DisplayName:    "One",
		NormalizedName: "one",
		Realm:          "Area 52",
		RealmSlug:      "area-52",
		Region:         "us",
		SyncedAt:       now,
	}, []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	stale := now.AddDate(0, 0, -120)
	recent := now.AddDate(0, 0, -1)
	for _, x := range []domain.Snapshot{
		{CharacterID: c.ID, CapturedAt: stale, ItemLevel: 400, RaidProgress: []byte("[]")},
		{CharacterID: c.ID, CapturedAt: recent, ItemLevel: 420, RaidProgress: []byte("[]")},
	} {
		if err := s.Progression.InsertSnapshot(ctx, x); err != nil {
			t.Fatal(err)
		}
	}
	deleted, err := s.Progression.DeleteSnapshotsBefore(ctx, now.AddDate(0, 0, -90))
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	d, err := s.Characters.Detail(ctx, g.ID, c.ID, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Snapshots) != 1 || !d.Snapshots[0].CapturedAt.Equal(recent) {
		t.Fatalf("snapshots=%+v, want only the recent snapshot", d.Snapshots)
	}
}

func TestTrendsByGuildUsesLatestDailySnapshotsAndAggregatesRaids(t *testing.T) {
	ctx := context.Background()
	p, _ := testdb.New(t)
	s := store.New(p)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	g, err := s.Guilds.EnsureGuild(ctx, domain.Guild{Slug: "trends", Name: "Ape", Realm: "Area 52", Region: "us"})
	if err != nil {
		t.Fatal(err)
	}
	upsert := func(name string, syncedAt time.Time) domain.Character {
		t.Helper()
		c, err := s.Characters.UpsertByGuildIdentity(ctx, domain.Character{GuildID: g.ID, Name: name, DisplayName: name, NormalizedName: name, Realm: "Area 52", RealmSlug: "area-52", Region: "us", SyncedAt: syncedAt}, []byte("{}"))
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	active, inactive := upsert("Active", now), upsert("Inactive", now.AddDate(0, 0, -10))
	raid := func(progress int) []byte {
		return []byte(`{"expansions":[{"instances":[{"instance":{"name":"Vault"},"modes":[{"difficulty":{"name":"Heroic"},"progress":{"completed_count":` + strconv.Itoa(progress) + `,"total_count":8}}]}]}]}`)
	}
	for _, snap := range []domain.Snapshot{
		{CharacterID: active.ID, CapturedAt: now.Add(-3 * time.Hour), ItemLevel: 600, MythicRating: 2000, RaidProgress: raid(3)},
		{CharacterID: active.ID, CapturedAt: now.Add(-time.Hour), ItemLevel: 620, MythicRating: 2200, RaidProgress: raid(4)},
		{CharacterID: inactive.ID, CapturedAt: now.Add(-2 * time.Hour), ItemLevel: 580, MythicRating: 1800, RaidProgress: []byte(`{"expansions":[{"instances":[{"instance":{"name":"Vault"},"modes":[]}]}]}`)},
	} {
		if err := s.Progression.InsertSnapshot(ctx, snap); err != nil {
			t.Fatal(err)
		}
	}
	trends, err := s.Characters.TrendsByGuild(ctx, g.ID, now.AddDate(0, 0, -1), now, now.Add(-7*24*time.Hour), 31)
	if err != nil {
		t.Fatal(err)
	}
	if len(trends) != 1 || trends[0].AverageItemLevel != 600 || trends[0].AverageRating != 2000 || trends[0].StaleCount != 1 {
		t.Fatalf("trends=%+v, want latest snapshots for both active and inactive characters", trends)
	}
	if len(trends[0].RaidProgress) != 1 || trends[0].RaidProgress[0] != (store.GuildTrendRaid{RaidName: "Vault", Difficulty: "Heroic", Progress: 4, TotalBosses: 8}) {
		t.Fatalf("raid progress=%+v, want aggregated Vault Heroic 4/8", trends[0].RaidProgress)
	}
}

func TestTrendsByGuildIgnoresMalformedRaidProgressCounts(t *testing.T) {
	ctx := context.Background()
	p, _ := testdb.New(t)
	s := store.New(p)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	g, err := s.Guilds.EnsureGuild(ctx, domain.Guild{Slug: "trends-malformed-progress", Name: "Ape", Realm: "Area 52", Region: "us"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.Characters.UpsertByGuildIdentity(ctx, domain.Character{
		GuildID: g.ID, Name: "Malformed", DisplayName: "Malformed", NormalizedName: "malformed",
		Realm: "Area 52", RealmSlug: "area-52", Region: "us", SyncedAt: now,
	}, []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	progress := []byte(`{"expansions":[{"instances":[{"instance":{"name":"Vault"},"modes":[
		{"difficulty":{"name":"Normal"},"progress":{}},
		{"difficulty":{"name":"Heroic"},"progress":{"completed_count":"","total_count":""}},
		{"difficulty":{"name":"Mythic"},"progress":null},
		{"difficulty":{"name":"LFR"},"progress":{"completed_count":"999999999999999999999999","total_count":"also-not-a-count"}}
	]}]}]}`)
	if err := s.Progression.InsertSnapshot(ctx, domain.Snapshot{CharacterID: c.ID, CapturedAt: now, RaidProgress: progress}); err != nil {
		t.Fatal(err)
	}

	trends, err := s.Characters.TrendsByGuild(ctx, g.ID, now.AddDate(0, 0, -1), now, now.Add(-7*24*time.Hour), 31)
	if err != nil {
		t.Fatal(err)
	}
	if len(trends) != 1 || len(trends[0].RaidProgress) != 4 {
		t.Fatalf("trends=%+v, want one trend with four raid rows", trends)
	}
	for _, raid := range trends[0].RaidProgress {
		if raid.Progress != 0 || raid.TotalBosses != 0 {
			t.Errorf("raid %q/%q = %d/%d, want 0/0", raid.RaidName, raid.Difficulty, raid.Progress, raid.TotalBosses)
		}
	}
}

func TestDeleteExpiredSessionsRemovesOnlyExpiredSessions(t *testing.T) {
	ctx := context.Background()
	p, _ := testdb.New(t)
	s := store.New(p)
	now := time.Now().Truncate(time.Second)
	u, err := s.Users.UpsertBattleNetUser(ctx, "1", "Sweeper#1", "", "", now.Add(time.Hour), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range []struct {
		id        string
		expiresAt time.Time
	}{
		{id: "expired", expiresAt: now.Add(-time.Hour)},
		{id: "valid", expiresAt: now.Add(time.Hour)},
	} {
		if err := s.Users.CreateSession(ctx, session.id, u.ID, session.expiresAt); err != nil {
			t.Fatal(err)
		}
	}
	deleted, err := s.Users.DeleteExpiredSessions(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	if _, err := s.Users.SessionUser(ctx, "expired"); err == nil {
		t.Fatal("SessionUser(expired) error = nil, want not-found error")
	}
	if _, err := s.Users.SessionUser(ctx, "valid"); err != nil {
		t.Fatalf("SessionUser(valid) error = %v, want the session to resolve", err)
	}
}
