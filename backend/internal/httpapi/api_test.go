package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/auth"
	"github.com/brocahontaz/apekeeper/backend/internal/blizzard/dto"
	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/brocahontaz/apekeeper/backend/internal/store"
	"github.com/brocahontaz/apekeeper/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

type oauthFake struct{}

func (oauthFake) AuthorizationURL(_, _ string) string {
	return ""
}
func (oauthFake) ExchangeCode(context.Context, string) (dto.Token, error) {
	return dto.Token{}, nil
}
func (oauthFake) UserProfile(context.Context, string) (dto.UserProfile, error) {
	return dto.UserProfile{}, nil
}

func TestSPAFallbackDoesNotInterceptAPI(t *testing.T) {
	h := New(API{
		Auth: auth.New(oauthFake{}, store.UserStore{}, "", []byte("test")),
		Frontend: fstest.MapFS{
			"index.html": {Data: []byte("<main>ApeKeeper</main>")},
		},
	})
	page := httptest.NewRecorder()
	h.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/roster/42", nil))
	if page.Code != http.StatusOK || page.Body.String() != "<main>ApeKeeper</main>" {
		t.Fatalf("spa=%d %q", page.Code, page.Body.String())
	}
	api := httptest.NewRecorder()
	h.ServeHTTP(api, httptest.NewRequest(http.MethodGet, "/api/healthz", nil))
	if api.Code != http.StatusOK {
		t.Fatalf("api=%d", api.Code)
	}
}

func TestSPAFallbackUsesStaticDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/index.html", []byte("<main>static ApeKeeper</main>"), 0600); err != nil {
		t.Fatal(err)
	}
	h := New(API{
		Auth:      auth.New(oauthFake{}, store.UserStore{}, "", []byte("test")),
		StaticDir: dir,
	})
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/roster/42", nil))
	if r.Code != http.StatusOK || r.Body.String() != "<main>static ApeKeeper</main>" {
		t.Fatalf("spa=%d %q", r.Code, r.Body.String())
	}
}

func TestHealthz(t *testing.T) {
	h := New(API{Auth: auth.New(oauthFake{}, store.UserStore{}, "", []byte("test"))})
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/healthz", nil))
	if r.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" || body["db"] != "ok" {
		t.Fatalf("body=%v", body)
	}
}

func TestStoreBackedHandlers(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.New(t)
	stores := store.New(pool)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	guild, err := stores.Guilds.EnsureGuild(ctx, domain.Guild{Slug: "ape", Name: "Ape", Realm: "Area 52", Region: "us"})
	if err != nil {
		t.Fatal(err)
	}
	alpha := seedCharacter(t, ctx, stores, guild.ID, "Alpha", "Mage", "Arcane", now)
	beta := seedCharacter(t, ctx, stores, guild.ID, "Beta", "Rogue", "Combat", now)
	gamma := seedCharacter(t, ctx, stores, guild.ID, "Gamma", "Warrior", "Arms", now)
	otherGuild, err := stores.Guilds.EnsureGuild(ctx, domain.Guild{Slug: "other", Name: "Other", Realm: "Area 52", Region: "us"})
	if err != nil {
		t.Fatal(err)
	}
	otherAlpha := seedCharacter(t, ctx, stores, otherGuild.ID, "Alpha", "Warrior", "Arcane", now)
	// Give Alpha a profile summary carrying the localized race/gender maps the
	// sync engine stores; the detail handler must surface them.
	alpha, err = stores.Characters.UpsertByGuildIdentity(ctx, alpha, []byte(
		`{"race":{"id":5,"name":{"en_US":"Dwarf"}},"gender":{"type":"MALE","name":{"en_US":"Male"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := stores.Progression.UpsertMythicPlus(ctx, domain.MythicPlus{
		CharacterID:   alpha.ID,
		Season:        "Season 1",
		SeasonSlug:    "season-1",
		OverallRating: 2500,
		BestRunScore:  315,
		BestKeyLevel:  12,
		Runs: &domain.MythicRuns{Best: []domain.MythicRun{{
			Dungeon:     "The Rookery",
			Level:       12,
			Score:       315,
			Timed:       true,
			CompletedAt: 1700000000000,
		}}},
		SyncedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := stores.Progression.ReplaceRaids(ctx, alpha.ID, []domain.RaidProgression{{
		CharacterID: alpha.ID,
		RaidSlug:    "raid",
		RaidName:    "Raid",
		Difficulty:  "heroic",
		Progress:    4,
		TotalBosses: 8,
		Summary:     []byte("{}"),
		SyncedAt:    now,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := stores.Progression.InsertSnapshot(ctx, domain.Snapshot{
		CharacterID:  alpha.ID,
		CapturedAt:   now.Add(-24 * time.Hour),
		ItemLevel:    610,
		MythicRating: 2400,
		BestKeyLevel: 10,
		RaidProgress: []byte(`{"expansions":[{"instances":[{"instance":{"name":"Raid"},"modes":[{"difficulty":{"name":"Heroic"},"progress":{"completed_count":4,"total_count":8}}]}]}]}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := stores.Progression.InsertSnapshot(ctx, domain.Snapshot{
		CharacterID:  alpha.ID,
		CapturedAt:   now,
		ItemLevel:    620,
		MythicRating: 2500,
		BestKeyLevel: 12,
		RaidProgress: []byte("[]"),
	}); err != nil {
		t.Fatal(err)
	}
	run, err := stores.SyncRuns.Create(ctx, guild.ID, "manual")
	if err != nil {
		t.Fatal(err)
	}
	run.Status = domain.RunSuccess
	run.Updated = 7
	if err := stores.SyncRuns.Finish(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := stores.Characters.UpdateAvatar(ctx, alpha.ID, "https://render.worldofwarcraft.com/us/alpha.jpg"); err != nil {
		t.Fatal(err)
	}

	secret := []byte("test-secret")
	manager := auth.New(oauthFake{}, stores.Users, "", secret)
	admin := seedSession(t, ctx, pool, stores, "admin", "admin")
	member := seedSession(t, ctx, pool, stores, "member", "member")
	officer := seedSession(t, ctx, pool, stores, "officer", "officer")
	superadmin := seedSession(t, ctx, pool, stores, "superadmin", "superadmin")
	h := New(API{
		Stores:    stores,
		Auth:      manager,
		GuildSlug: guild.Slug,
		Now: func() time.Time {
			return now
		},
		Trigger: TriggerFunc(func(context.Context) (int64, error) {
			return 42, nil
		}),
	})

	t.Run("roster requires authentication", func(t *testing.T) {
		assertStatus(t, h, http.MethodGet, "/api/roster", "", http.StatusUnauthorized)
		r := request(h, http.MethodGet, "/api/roster", member)
		if r.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
		}
		body := decodeBody(t, r).(map[string]any)
		if body["total"] != float64(3) || len(body["items"].([]any)) != 3 {
			t.Fatalf("roster=%v", body)
		}
		alphaJSON := body["items"].([]any)[0].(map[string]any)
		if alphaJSON["mythicRating"] != float64(2500) || alphaJSON["bestKeyLevel"] != float64(12) {
			t.Fatalf("roster=%v", alphaJSON)
		}
	})
	t.Run("sync trigger roles", func(t *testing.T) {
		assertStatus(t, h, http.MethodPost, "/api/sync/run", "", http.StatusUnauthorized)
		assertStatus(t, h, http.MethodPost, "/api/sync/run", member, http.StatusForbidden)
		assertStatus(t, h, http.MethodPost, "/api/sync/run", officer, http.StatusForbidden)
		r := request(h, http.MethodPost, "/api/sync/run", admin)
		if r.Code != http.StatusAccepted ||
			decodeBody(t, r).(map[string]any)["runId"] != float64(42) {
			t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
		}
		assertStatus(t, h, http.MethodPost, "/api/sync/run", superadmin, http.StatusAccepted)
	})
	t.Run("sync history roles", func(t *testing.T) {
		assertStatus(t, h, http.MethodGet, "/api/sync/runs", member, http.StatusForbidden)
		r := request(h, http.MethodGet, "/api/sync/runs", officer)
		if r.Code != http.StatusOK ||
			decodeBody(t, r).([]any)[0].(map[string]any)["updated"] != float64(7) {
			t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
		}
		assertStatus(t, h, http.MethodGet, "/api/sync/runs", admin, http.StatusOK)
		assertStatus(t, h, http.MethodGet, "/api/sync/runs", superadmin, http.StatusOK)
	})
	t.Run("dashboard aggregates seeded data", func(t *testing.T) {
		assertStatus(t, h, http.MethodGet, "/api/dashboard", "", http.StatusUnauthorized)
		r := request(h, http.MethodGet, "/api/dashboard", member)
		if r.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
		}
		body := decodeBody(t, r).(map[string]any)
		if body["rosterSize"] != float64(3) || body["lastSync"] == nil {
			t.Fatalf("dashboard=%v", body)
		}
		if _, ok := body["specDistribution"]; ok {
			t.Fatalf("dashboard must not expose specDistribution: %v", body)
		}
		trends := body["trends"].([]any)
		if len(trends) != 2 || trends[0].(map[string]any)["averageItemLevel"] != float64(610) || trends[1].(map[string]any)["averageRating"] != float64(2500) || trends[0].(map[string]any)["staleCount"] != float64(0) {
			t.Fatalf("trends=%v", trends)
		}
		dist := body["classDistribution"].([]any)
		wantClasses := []string{"Mage", "Rogue", "Warrior"}
		wantSpecs := []string{"Arcane", "Combat", "Arms"}
		if len(dist) != len(wantClasses) {
			t.Fatalf("classDistribution=%v", dist)
		}
		for i, e := range dist {
			entry := e.(map[string]any)
			if entry["className"] != wantClasses[i] {
				t.Fatalf("classDistribution[%d]=%v, want class %s", i, entry, wantClasses[i])
			}
			specs, ok := entry["specs"].([]any)
			if !ok || len(specs) != 1 {
				t.Fatalf("class %s specs=%v", entry["className"], entry["specs"])
			}
			spec := specs[0].(map[string]any)
			if spec["name"] != wantSpecs[i] || spec["count"] != float64(1) {
				t.Fatalf("class %s specs=%v, want %s", entry["className"], specs, wantSpecs[i])
			}
		}
	})
	t.Run("roster export streams csv", func(t *testing.T) {
		assertStatus(t, h, http.MethodGet, "/api/roster/export", "", http.StatusUnauthorized)
		r := request(h, http.MethodGet, "/api/roster/export", member)
		if r.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
		}
		if ct := r.Header().Get("Content-Type"); ct != "text/csv; charset=utf-8" {
			t.Fatalf("content-type=%q", ct)
		}
		wantDisposition := `attachment; filename="roster-ape-20260115.csv"`
		if cd := r.Header().Get("Content-Disposition"); cd != wantDisposition {
			t.Fatalf("disposition=%q, want %q", cd, wantDisposition)
		}
		lines := strings.Split(strings.TrimSuffix(r.Body.String(), "\n"), "\n")
		if len(lines) != 4 || !strings.HasPrefix(lines[0], "Name,Realm,Class") {
			t.Fatalf("csv lines=%d header=%q", len(lines), lines[0])
		}
		for i, want := range []string{"Alpha", "Beta", "Gamma"} {
			if !strings.HasPrefix(lines[i+1], want+",") {
				t.Fatalf("csv row %d=%q, want it to start with %s", i+1, lines[i+1], want)
			}
		}
	})
	t.Run("roster filters", func(t *testing.T) {
		for _, path := range []string{"/api/roster?search=lph", "/api/roster?class=Mage"} {
			r := request(h, http.MethodGet, path, member)
			if r.Code != http.StatusOK {
				t.Fatalf("path=%s status=%d body=%s", path, r.Code, r.Body.String())
			}
			body := decodeBody(t, r).(map[string]any)
			items := body["items"].([]any)
			if body["total"] != float64(1) || len(items) != 1 || items[0].(map[string]any)["name"] != "Alpha" {
				t.Fatalf("path=%s roster=%v", path, body)
			}
		}
		r := request(h, http.MethodGet, "/api/roster?sort=classSpec&direction=descending&page=1&pageSize=1", member)
		body := decodeBody(t, r).(map[string]any)
		items := body["items"].([]any)
		if body["total"] != float64(3) || body["page"] != float64(1) || len(items) != 1 || items[0].(map[string]any)["name"] != "Gamma" {
			t.Fatalf("paginated roster=%v", body)
		}
		classes := body["classes"].([]any)
		if len(classes) != 3 || classes[0] != "Mage" || classes[1] != "Rogue" || classes[2] != "Warrior" {
			t.Fatalf("roster classes=%v", classes)
		}
		specs := body["specs"].([]any)
		if len(specs) != 3 || specs[0] != "Arcane" || specs[1] != "Arms" || specs[2] != "Combat" {
			t.Fatalf("roster specs=%v", specs)
		}
		for _, path := range []string{"/api/roster?spec=Arcane", "/api/roster?minRating=1000"} {
			r := request(h, http.MethodGet, path, member)
			if r.Code != http.StatusOK {
				t.Fatalf("path=%s status=%d body=%s", path, r.Code, r.Body.String())
			}
			body := decodeBody(t, r).(map[string]any)
			items := body["items"].([]any)
			if body["total"] != float64(1) || len(items) != 1 || items[0].(map[string]any)["name"] != "Alpha" {
				t.Fatalf("path=%s roster=%v", path, body)
			}
		}
		r = request(h, http.MethodGet, "/api/roster?minLevel=70", member)
		body = decodeBody(t, r).(map[string]any)
		if body["total"] != float64(3) || len(body["items"].([]any)) != 3 {
			t.Fatalf("minLevel roster=%v", body)
		}
		assertStatus(t, h, http.MethodGet, "/api/roster?sort=drop%20table", member, http.StatusBadRequest)
	})
	t.Run("character detail and unknown character", func(t *testing.T) {
		assertStatus(t, h, http.MethodGet, "/api/characters/"+intString(alpha.ID), "", http.StatusUnauthorized)
		r := request(h, http.MethodGet, "/api/characters/"+intString(alpha.ID), member)
		body := decodeBody(t, r).(map[string]any)
		if r.Code != http.StatusOK ||
			len(body["mythicPlus"].([]any)) == 0 ||
			len(body["raidProgression"].([]any)) == 0 ||
			len(body["snapshots"].([]any)) == 0 {
			t.Fatalf("status=%d character=%v", r.Code, body)
		}
		if body["avatarUrl"] != "https://render.worldofwarcraft.com/us/alpha.jpg" {
			t.Fatalf("avatarUrl=%v", body["avatarUrl"])
		}
		if body["raceName"] != "Dwarf" || body["gender"] != "Male" {
			t.Fatalf("raceName=%v gender=%v, want Dwarf/Male", body["raceName"], body["gender"])
		}
		mythic := body["mythicPlus"].([]any)[0].(map[string]any)
		if mythic["overallRating"] != float64(2500) || mythic["seasonSlug"] != "season-1" {
			t.Fatalf("mythicPlus=%v", mythic)
		}
		runs, ok := mythic["runs"].(map[string]any)
		if !ok || len(runs["best"].([]any)) != 1 ||
			runs["best"].([]any)[0].(map[string]any)["dungeon"] != "The Rookery" {
			t.Fatalf("mythicPlus runs=%v, want one best entry", mythic["runs"])
		}
		raid := body["raidProgression"].([]any)[0].(map[string]any)
		if raid["raidName"] != "Raid" || raid["difficulty"] != "heroic" ||
			raid["progress"] != float64(4) || raid["totalBosses"] != float64(8) {
			t.Fatalf("raidProgression=%v", raid)
		}
		snap := body["snapshots"].([]any)[0].(map[string]any)
		if snap["itemLevel"] != float64(610) {
			t.Fatalf("snapshots=%v", snap)
		}
		if _, ok := snap["capturedAt"].(string); !ok {
			t.Fatalf("snapshots capturedAt=%v, want a string", snap["capturedAt"])
		}
		if _, ok := snap["raidProgress"].(map[string]any); !ok {
			t.Fatalf("snapshots raidProgress=%T, want JSON object rather than base64", snap["raidProgress"])
		}
		byName := request(h, http.MethodGet, "/api/characters/Alpha", member)
		if byName.Code != http.StatusOK || decodeBody(t, byName).(map[string]any)["id"] != float64(alpha.ID) {
			t.Fatalf("named character=%d %s", byName.Code, byName.Body.String())
		}
		assertStatus(t, h, http.MethodGet, "/api/characters/999999", member, http.StatusNotFound)
	})
	t.Run("character history is bounded, ordered, and guild scoped", func(t *testing.T) {
		assertStatus(t, h, http.MethodGet, "/api/characters/"+intString(alpha.ID)+"/history", "", http.StatusUnauthorized)
		base := "/api/characters/" + intString(alpha.ID) + "/history?from=2026-01-14T12:00:00Z&to=2026-01-15T12:00:00Z&limit=2"
		r := request(h, http.MethodGet, base, member)
		if r.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
		}
		body := decodeBody(t, r).(map[string]any)
		snaps := body["snapshots"].([]any)
		if len(snaps) != 2 || snaps[0].(map[string]any)["itemLevel"] != float64(610) || snaps[1].(map[string]any)["itemLevel"] != float64(620) {
			t.Fatalf("history=%v", body)
		}
		// Date endpoints are inclusive; moving either endpoint one second inside
		// the range excludes the snapshot that was exactly on that boundary.
		atStart := request(h, http.MethodGet, "/api/characters/"+intString(alpha.ID)+"/history?from=2026-01-14T12:00:00Z&to=2026-01-14T12:00:00Z", member)
		if atStart.Code != http.StatusOK || len(decodeBody(t, atStart).(map[string]any)["snapshots"].([]any)) != 1 {
			t.Fatalf("start boundary=%d %s", atStart.Code, atStart.Body.String())
		}
		atEnd := request(h, http.MethodGet, "/api/characters/"+intString(alpha.ID)+"/history?from=2026-01-15T12:00:00Z&to=2026-01-15T12:00:00Z", member)
		if atEnd.Code != http.StatusOK || len(decodeBody(t, atEnd).(map[string]any)["snapshots"].([]any)) != 1 {
			t.Fatalf("end boundary=%d %s", atEnd.Code, atEnd.Body.String())
		}
		inside := request(h, http.MethodGet, "/api/characters/"+intString(alpha.ID)+"/history?from=2026-01-14T12:00:01Z&to=2026-01-15T11:59:59Z", member)
		if inside.Code != http.StatusOK || len(decodeBody(t, inside).(map[string]any)["snapshots"].([]any)) != 0 {
			t.Fatalf("exclusive boundaries=%d %s", inside.Code, inside.Body.String())
		}
		comparison := body["comparison"].(map[string]any)
		if comparison["itemLevelDelta"] != float64(10) || comparison["mythicRatingDelta"] != float64(100) {
			t.Fatalf("comparison=%v", comparison)
		}
		first, second := snaps[0].(map[string]any)["id"], snaps[1].(map[string]any)["id"]
		r = request(h, http.MethodGet, base+"&compareFrom="+strconv.Itoa(int(first.(float64)))+"&compareTo="+strconv.Itoa(int(second.(float64))), member)
		if r.Code != http.StatusOK {
			t.Fatalf("selected comparison=%d %s", r.Code, r.Body.String())
		}
		// A selected ID must be checked even if this range only has one result,
		// and IDs outside the range must not be mistaken for no comparison.
		assertStatus(t, h, http.MethodGet, "/api/characters/"+intString(alpha.ID)+"/history?from=2026-01-15T12:00:00Z&to=2026-01-15T12:00:00Z&compareFrom="+strconv.Itoa(int(first.(float64)))+"&compareTo="+strconv.Itoa(int(second.(float64))), member, http.StatusBadRequest)
		assertStatus(t, h, http.MethodGet, base+"&compareFrom=999999&compareTo="+strconv.Itoa(int(second.(float64))), member, http.StatusBadRequest)
		one := request(h, http.MethodGet, "/api/characters/"+intString(alpha.ID)+"/history?from=2026-01-15T12:00:00Z&to=2026-01-15T12:00:00Z", member)
		if one.Code != http.StatusOK || decodeBody(t, one).(map[string]any)["comparison"] != nil {
			t.Fatalf("insufficient history=%d %s", one.Code, one.Body.String())
		}
		empty := request(h, http.MethodGet, "/api/characters/"+intString(alpha.ID)+"/history?from=2026-01-13T12:00:00Z&to=2026-01-13T12:00:00Z", member)
		if empty.Code != http.StatusOK || len(decodeBody(t, empty).(map[string]any)["snapshots"].([]any)) != 0 {
			t.Fatalf("empty history=%d %s", empty.Code, empty.Body.String())
		}
		if err := stores.Progression.InsertSnapshot(ctx, domain.Snapshot{CharacterID: alpha.ID, CapturedAt: now.Add(-time.Hour), ItemLevel: 620, MythicRating: 2500, BestKeyLevel: 12, RaidProgress: []byte("[]")}); err != nil {
			t.Fatal(err)
		}
		identical := request(h, http.MethodGet, "/api/characters/"+intString(alpha.ID)+"/history?from=2026-01-15T10:00:00Z&to=2026-01-15T12:00:00Z", member)
		identicalBody := decodeBody(t, identical).(map[string]any)
		if identical.Code != http.StatusOK || identicalBody["comparison"].(map[string]any)["itemLevelDelta"] != float64(0) || identicalBody["comparison"].(map[string]any)["mythicRatingDelta"] != float64(0) {
			t.Fatalf("identical history=%d %v", identical.Code, identicalBody)
		}
		for _, bad := range []string{"?from=nope", "?from=2026-01-01T00:00:00Z&to=2026-05-01T00:00:00Z", "?limit=501", "?compareFrom=1"} {
			assertStatus(t, h, http.MethodGet, "/api/characters/"+intString(alpha.ID)+"/history"+bad, member, http.StatusBadRequest)
		}
		// The other guild's same-name character has no observable history here.
		other := request(h, http.MethodGet, "/api/characters/"+intString(otherAlpha.ID)+"/history", member)
		if other.Code != http.StatusOK || len(decodeBody(t, other).(map[string]any)["snapshots"].([]any)) != 0 {
			t.Fatalf("cross-guild history=%d %s", other.Code, other.Body.String())
		}
	})
	t.Run("me requires a session", func(t *testing.T) {
		assertStatus(t, h, http.MethodGet, "/api/auth/me", "", http.StatusUnauthorized)
		r := request(h, http.MethodGet, "/api/auth/me", admin)
		if r.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
		}
		body := decodeBody(t, r).(map[string]any)
		if body["appRole"] != "admin" || body["displayName"] != "admin" {
			t.Fatalf("me=%v", body)
		}
	})
	t.Run("roster defaults to guild rank order", func(t *testing.T) {
		ranks := map[string]int{"Alpha": 1, "Beta": 0, "Gamma": 3}
		for _, c := range []domain.Character{alpha, beta, gamma} {
			c.GuildRank = ranks[c.DisplayName]
			if _, err := stores.Characters.UpsertByGuildIdentity(ctx, c, []byte("{}")); err != nil {
				t.Fatal(err)
			}
		}
		r := request(h, http.MethodGet, "/api/roster", member)
		if r.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
		}
		body := decodeBody(t, r).(map[string]any)
		if body["total"] != float64(3) {
			t.Fatalf("roster=%v", body)
		}
		items := body["items"].([]any)
		for i, want := range []string{"Beta", "Alpha", "Gamma"} {
			if got := items[i].(map[string]any)["name"]; got != want {
				t.Fatalf("roster[%d]=%v, want %v", i, got, want)
			}
		}
	})
	t.Run("roster export honors roster filters and sort", func(t *testing.T) {
		// Distinct levels make the level sort observable in the CSV order;
		// ranks keep the seeded values so the default-order subtest above and
		// this one stay independent.
		assertStatus(t, h, http.MethodGet, "/api/roster/export?sort=level", "", http.StatusUnauthorized)
		levels := map[string]int{"Alpha": 70, "Beta": 60, "Gamma": 50}
		for _, c := range []domain.Character{alpha, beta, gamma} {
			c.Level = levels[c.DisplayName]
			if _, err := stores.Characters.UpsertByGuildIdentity(ctx, c, []byte("{}")); err != nil {
				t.Fatal(err)
			}
		}
		r := request(h, http.MethodGet, "/api/roster/export?class=Mage", member)
		if r.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
		}
		lines := strings.Split(strings.TrimSuffix(r.Body.String(), "\n"), "\n")
		if len(lines) != 2 || !strings.HasPrefix(lines[1], "Alpha,") {
			t.Fatalf("class-filtered csv=%q, want only Alpha", r.Body.String())
		}
		for _, tc := range []struct {
			direction string
			want      []string
		}{
			{"descending", []string{"Alpha", "Beta", "Gamma"}},
			{"ascending", []string{"Gamma", "Beta", "Alpha"}},
		} {
			r := request(h, http.MethodGet, "/api/roster/export?sort=level&direction="+tc.direction, member)
			if r.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
			}
			lines := strings.Split(strings.TrimSuffix(r.Body.String(), "\n"), "\n")
			if len(lines) != 4 {
				t.Fatalf("csv lines=%d", len(lines))
			}
			for i, want := range tc.want {
				if !strings.HasPrefix(lines[i+1], want+",") {
					t.Fatalf("%s csv row %d=%q, want it to start with %s", tc.direction, i+1, lines[i+1], want)
				}
			}
		}
		// Invalid sort or direction answers with the roster JSON 400 error
		// before any CSV header is written.
		for _, tc := range []struct {
			query, wantErr string
		}{
			{"sort=drop%20table", "invalid roster sort"},
			{"direction=sideways", "invalid roster sort direction"},
		} {
			r := request(h, http.MethodGet, "/api/roster/export?"+tc.query, member)
			if r.Code != http.StatusBadRequest {
				t.Fatalf("%s status=%d body=%s", tc.query, r.Code, r.Body.String())
			}
			if ct := r.Header().Get("Content-Type"); ct != "application/json" {
				t.Fatalf("%s content-type=%q, want the JSON error", tc.query, ct)
			}
			if got := decodeBody(t, r).(map[string]any)["error"]; got != tc.wantErr {
				t.Fatalf("%s error=%v, want %q", tc.query, got, tc.wantErr)
			}
		}
	})
	t.Run("roster export of an empty roster", func(t *testing.T) {
		empty, err := stores.Guilds.EnsureGuild(ctx, domain.Guild{Slug: "empty", Name: "Empty", Realm: "Area 52", Region: "us"})
		if err != nil {
			t.Fatal(err)
		}
		h := New(API{
			Stores:    stores,
			Auth:      manager,
			GuildSlug: empty.Slug,
			Now: func() time.Time {
				return now
			},
		})
		r := request(h, http.MethodGet, "/api/roster/export", member)
		if r.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
		}
		want := "Name,Realm,Class,Spec,Level,Guild Rank,Item Level,M+ Rating,Best Key,Stale,Last Seen\n"
		if r.Body.String() != want {
			t.Fatalf("csv=%q, want a header-only export", r.Body.String())
		}
	})
}

// fakeProgress stands in for the sync service's live snapshot so the handler
// can be checked without wiring the whole engine.
type fakeProgress struct{ snap domain.Progress }

func (f fakeProgress) Progress() domain.Progress { return f.snap }

type fakeOperations struct{ cancelErr error }

func (f fakeOperations) StartDryRun(context.Context) (store.SyncRun, error) {
	return store.SyncRun{ID: 43}, nil
}
func (f fakeOperations) StartRetry(_ context.Context, id int64) (store.SyncRun, error) {
	if id == 99 {
		return store.SyncRun{}, errors.New("retry unavailable")
	}
	return store.SyncRun{ID: 44}, nil
}
func (f fakeOperations) Cancel() error { return f.cancelErr }

func TestSyncProgressEndpoint(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.New(t)
	stores := store.New(pool)
	guild, err := stores.Guilds.EnsureGuild(ctx, domain.Guild{Slug: "progress", Name: "Ape", Realm: "Area 52", Region: "us"})
	if err != nil {
		t.Fatal(err)
	}
	manager := auth.New(oauthFake{}, stores.Users, "", []byte("test-secret"))
	member := seedSession(t, ctx, pool, stores, "member", "member")
	officer := seedSession(t, ctx, pool, stores, "officer", "officer")
	admin := seedSession(t, ctx, pool, stores, "admin", "admin")
	superadmin := seedSession(t, ctx, pool, stores, "superadmin", "superadmin")

	// No progress source wired: the endpoint still answers for the roles the
	// history endpoint allows, with the idle shape.
	h := New(API{Stores: stores, Auth: manager, GuildSlug: guild.Slug})

	t.Run("progress requires officer and above", func(t *testing.T) {
		assertStatus(t, h, http.MethodGet, "/api/sync/progress", "", http.StatusUnauthorized)
		assertStatus(t, h, http.MethodGet, "/api/sync/progress", member, http.StatusForbidden)
		assertStatus(t, h, http.MethodGet, "/api/sync/progress", officer, http.StatusOK)
		assertStatus(t, h, http.MethodGet, "/api/sync/progress", admin, http.StatusOK)
		assertStatus(t, h, http.MethodGet, "/api/sync/progress", superadmin, http.StatusOK)
	})

	t.Run("idle shape without a wired source", func(t *testing.T) {
		r := request(h, http.MethodGet, "/api/sync/progress", officer)
		if r.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
		}
		body := decodeBody(t, r).(map[string]any)
		want := map[string]any{
			"active": false, "runId": float64(0), "status": "", "phase": "",
			"total": float64(0), "updated": float64(0), "failed": float64(0),
			"dryRun": false,
		}
		for k, v := range want {
			if body[k] != v {
				t.Fatalf("idle progress[%s]=%v, want %v", k, body[k], v)
			}
		}
	})

	t.Run("live snapshot shape from a wired source", func(t *testing.T) {
		live := New(API{
			Stores:    stores,
			Auth:      manager,
			GuildSlug: guild.Slug,
			Progress: fakeProgress{domain.Progress{
				Active:    true,
				RunID:     7,
				Status:    domain.RunRunning,
				Phase:     domain.PhaseCharacters,
				Total:     42,
				Updated:   30,
				Failed:    2,
				StartedAt: time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC),
			}},
		})
		r := request(live, http.MethodGet, "/api/sync/progress", officer)
		if r.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
		}
		body := decodeBody(t, r).(map[string]any)
		want := map[string]any{
			"active": true, "runId": float64(7), "status": "running", "phase": "characters",
			"total": float64(42), "updated": float64(30), "failed": float64(2),
			"dryRun": false,
		}
		for k, v := range want {
			if body[k] != v {
				t.Fatalf("progress[%s]=%v, want %v", k, body[k], v)
			}
		}
		if _, ok := body["startedAt"].(string); !ok {
			t.Fatalf("progress startedAt=%v, want a string timestamp", body["startedAt"])
		}
	})
}

func TestSyncOperationEndpointsRequireAdmin(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.New(t)
	stores := store.New(pool)
	guild, err := stores.Guilds.EnsureGuild(ctx, domain.Guild{Slug: "operations", Name: "Ape", Realm: "Area 52", Region: "us"})
	if err != nil {
		t.Fatal(err)
	}
	manager := auth.New(oauthFake{}, stores.Users, "", []byte("test-secret"))
	member := seedSession(t, ctx, pool, stores, "member", "member")
	admin := seedSession(t, ctx, pool, stores, "admin", "admin")
	h := New(API{Stores: stores, Auth: manager, GuildSlug: guild.Slug, Operations: fakeOperations{}})
	for _, path := range []string{"/api/sync/dry-run", "/api/sync/runs/1/retry", "/api/sync/cancel"} {
		assertStatus(t, h, http.MethodPost, path, "", http.StatusUnauthorized)
		assertStatus(t, h, http.MethodPost, path, member, http.StatusForbidden)
	}
	for _, path := range []string{"/api/sync/dry-run", "/api/sync/runs/1/retry", "/api/sync/cancel"} {
		assertStatus(t, h, http.MethodPost, path, admin, http.StatusAccepted)
	}
	assertStatus(t, h, http.MethodPost, "/api/sync/runs/nope/retry", admin, http.StatusBadRequest)
	assertStatus(t, h, http.MethodPost, "/api/sync/runs/99/retry", admin, http.StatusConflict)
	assertStatus(t, New(API{Stores: stores, Auth: manager, GuildSlug: guild.Slug, Operations: fakeOperations{cancelErr: errors.New("none")}}), http.MethodPost, "/api/sync/cancel", admin, http.StatusConflict)
}

func seedCharacter(
	t *testing.T,
	ctx context.Context,
	stores store.Store,
	guildID int64,
	name, class, spec string,
	now time.Time,
) domain.Character {
	t.Helper()
	c, err := stores.Characters.UpsertByGuildIdentity(ctx, domain.Character{
		GuildID:        guildID,
		Name:           name,
		DisplayName:    name,
		NormalizedName: domain.NormalizeCharacterName(name),
		Realm:          "Area 52",
		RealmSlug:      "area-52",
		Region:         "us",
		ClassID:        8,
		ClassName:      class,
		SpecID:         62,
		SpecName:       spec,
		Level:          70,
		ItemLevel:      620,
		SyncedAt:       now,
	}, []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func seedSession(t *testing.T, ctx context.Context, pool *pgxpool.Pool, stores store.Store, name, role string) string {
	t.Helper()
	expires := time.Now().Add(time.Hour)
	u, err := stores.Users.UpsertBattleNetUser(ctx, name, name, "", "", expires, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE users SET app_role=$1 WHERE id=$2", role, u.ID); err != nil {
		t.Fatal(err)
	}
	id := "session-" + name
	if err := stores.Users.CreateSession(ctx, id, u.ID, expires); err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha256.New, []byte("test-secret"))
	mac.Write([]byte(id))
	return id + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func request(h http.Handler, method, path, cookie string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func assertStatus(t *testing.T, h http.Handler, method, path, cookie string, want int) {
	t.Helper()
	if got := request(h, method, path, cookie).Code; got != want {
		t.Fatalf("%s %s status=%d want=%d", method, path, got, want)
	}
}

func decodeBody(t *testing.T, r *httptest.ResponseRecorder) any {
	t.Helper()
	var body any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func intString(v int64) string {
	return strconv.FormatInt(v, 10)
}
