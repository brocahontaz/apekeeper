package store_test

import (
	"context"
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
	c, err := s.Characters.UpsertByGuildIdentity(ctx, domain.Character{GuildID: g.ID, Name: "One", DisplayName: "One", NormalizedName: "one", Realm: "Area 52", RealmSlug: "area-52", Region: "us", SyncedAt: now}, []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []domain.MythicPlus{{CharacterID: c.ID, SeasonSlug: "one", OverallRating: 1000, BestKeyLevel: 12, SyncedAt: now}, {CharacterID: c.ID, SeasonSlug: "two", OverallRating: 2000, BestKeyLevel: 15, SyncedAt: now}} {
		if err := s.Progression.UpsertMythicPlus(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	minRating := 1500.0
	rows, err := s.Characters.ListByGuild(ctx, g.ID, store.CharacterFilter{MinRating: &minRating})
	if err != nil || len(rows) != 1 || rows[0].ID != c.ID || rows[0].MythicRating != 2000 || rows[0].BestKeyLevel != 15 {
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
