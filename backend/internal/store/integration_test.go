package store_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/brocahontaz/apekeeper/backend/internal/store"
	"github.com/brocahontaz/apekeeper/backend/internal/testdb"
)

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
