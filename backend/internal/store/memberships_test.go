package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/brocahontaz/apekeeper/backend/internal/store"
	"github.com/brocahontaz/apekeeper/backend/internal/testdb"
)

func TestMembershipsIsolateGuildsWithOverlappingCharacterNames(t *testing.T) {
	ctx := context.Background()
	p, _ := testdb.New(t)
	s := store.New(p)
	one, err := s.Guilds.EnsureGuild(ctx, domain.Guild{Slug: "membership-one", Name: "One", Realm: "Area 52", Region: "us"})
	if err != nil {
		t.Fatal(err)
	}
	two, err := s.Guilds.EnsureGuild(ctx, domain.Guild{Slug: "membership-two", Name: "Two", Realm: "Area 52", Region: "us"})
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.Users.UpsertBattleNetUser(ctx, "membership-user", "Membership User", "", "", time.Now().Add(time.Hour), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Memberships.Grant(ctx, one.ID, u.ID, "officer"); err != nil {
		t.Fatal(err)
	}
	if err := s.Memberships.Grant(ctx, two.ID, u.ID, "member"); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Memberships.List(ctx, u.ID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("memberships=%+v err=%v", rows, err)
	}
	for _, g := range []domain.Guild{one, two} {
		_, err = s.Characters.UpsertByGuildIdentity(ctx, domain.Character{GuildID: g.ID, Name: "Twin", DisplayName: "Twin", NormalizedName: "twin", Realm: "Area 52", RealmSlug: "area-52", Region: "us", SyncedAt: time.Now()}, []byte("{}"))
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Memberships.ForUser(ctx, u.ID, one.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Memberships.Revoke(ctx, two.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Memberships.ForUser(ctx, u.ID, two.ID); err == nil {
		t.Fatal("revoked membership still authorizes guild")
	}
}
