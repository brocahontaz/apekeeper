package store_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/brocahontaz/apekeeper/backend/internal/store"
	"github.com/brocahontaz/apekeeper/backend/internal/testdb"
)

// These plans come from store read-plan hooks, while the calls below exercise
// the typed application methods. Thus a test cannot pass by explaining an
// unrelated hand-written query shape.
func TestReadQueryPlansUseStoreMethodsAndRemainGuildScoped(t *testing.T) {
	p, _ := testdb.New(t)
	ctx := context.Background()
	s := store.New(p)
	g, err := s.Guilds.EnsureGuild(ctx, domain.Guild{Slug: "plans", Name: "Plans", Realm: "Area 52", Region: "us"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	c, err := s.Characters.UpsertByGuildIdentity(ctx, domain.Character{GuildID: g.ID, Name: "Plan Ape", DisplayName: "Plan Ape", NormalizedName: "plan-ape", Realm: "Area 52", RealmSlug: "area-52", Region: "us", SyncedAt: now}, []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Characters.DashboardByGuild(ctx, g.ID, now.Add(-30*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Characters.ListPageByGuild(ctx, g.ID, store.CharacterFilter{}, 25, 0, store.CharacterSortName, false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Progression.History(ctx, g.ID, c.ID, now.Add(-24*time.Hour), now.Add(time.Hour), 25); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SyncRuns.History(ctx, g.ID, 25); err != nil {
		t.Fatal(err)
	}
	for _, plan := range store.ReadPlans() {
		t.Run(plan.Name, func(t *testing.T) {
			var raw []byte
			args := []any{g.ID}
			switch plan.Name {
			case "dashboard":
				args = []any{g.ID, now.Add(-30 * 24 * time.Hour)}
			case "roster":
				args = []any{g.ID, 25, 0}
			case "history":
				args = []any{g.ID, c.ID, now.Add(-24 * time.Hour), now.Add(time.Hour), 25}
			case "sync history":
				args = []any{g.ID, 25}
			}
			if err := p.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+plan.SQL, args...).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var decoded []any
			if json.Unmarshal(raw, &decoded) != nil || len(decoded) == 0 || !strings.Contains(string(raw), `"Plan"`) {
				t.Fatalf("invalid executable plan: %s", raw)
			}
			if !strings.Contains(plan.SQL, "guild_id=$1") {
				t.Fatalf("plan lost guild scope: %s", plan.SQL)
			}
		})
	}
}
