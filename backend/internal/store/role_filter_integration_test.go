package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/brocahontaz/apekeeper/backend/internal/store"
	"github.com/brocahontaz/apekeeper/backend/internal/testdb"
)

func TestListPageByGuildFiltersStoredAccountRoleAndCombinations(t *testing.T) {
	ctx := context.Background()
	p, _ := testdb.New(t)
	s := store.New(p)
	g, err := s.Guilds.EnsureGuild(ctx, domain.Guild{Slug: "role-filter", Name: "Ape", Realm: "Area 52", Region: "us"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, row := range []struct {
		name, class, role string
	}{
		{"AdminMage", "Mage", "admin"},
		{"OfficerMage", "Mage", "officer"},
		{"OfficerRogue", "Rogue", "officer"},
	} {
		c, err := s.Characters.UpsertByGuildIdentity(ctx, domain.Character{
			GuildID: g.ID, Name: row.name, DisplayName: row.name,
			NormalizedName: domain.NormalizeCharacterName(row.name), Realm: "Area 52",
			RealmSlug: "area-52", Region: "us", ClassName: row.class, SyncedAt: now,
		}, []byte("{}"))
		if err != nil {
			t.Fatal(err)
		}
		var userID int64
		if err := p.QueryRow(ctx, `INSERT INTO users(display_name,app_role) VALUES($1,$2) RETURNING id`, row.name, row.role).Scan(&userID); err != nil {
			t.Fatal(err)
		}
		if _, err := p.Exec(ctx, `UPDATE characters SET user_id=$1 WHERE id=$2`, userID, c.ID); err != nil {
			t.Fatal(err)
		}
	}
	page, err := s.Characters.ListPageByGuild(ctx, g.ID, store.CharacterFilter{Role: "officer", Class: "Mage"}, 1, 0, store.CharacterSortName, false)
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].DisplayName != "OfficerMage" || page.Items[0].Role != "officer" {
		t.Fatalf("filtered page=%+v err=%v", page, err)
	}
	page, err = s.Characters.ListPageByGuild(ctx, g.ID, store.CharacterFilter{Role: "officer"}, 1, 1, store.CharacterSortName, false)
	if err != nil || page.Total != 2 || len(page.Items) != 1 || page.Items[0].DisplayName != "OfficerRogue" {
		t.Fatalf("paginated role page=%+v err=%v", page, err)
	}
}
