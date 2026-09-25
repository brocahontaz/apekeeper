package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
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
	alpha := seedCharacter(t, ctx, stores, guild.ID, "Alpha", "Mage", now)
	_ = seedCharacter(t, ctx, stores, guild.ID, "Beta", "Rogue", now)
	if err := stores.Progression.UpsertMythicPlus(ctx, domain.MythicPlus{
		CharacterID:   alpha.ID,
		Season:        "Season 1",
		SeasonSlug:    "season-1",
		OverallRating: 2500,
		BestKeyLevel:  12,
		Dungeons:      []byte("[]"),
		SyncedAt:      now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := stores.Progression.UpsertRaid(ctx, domain.RaidProgression{
		CharacterID: alpha.ID,
		RaidSlug:    "raid",
		RaidName:    "Raid",
		Difficulty:  "heroic",
		Progress:    4,
		TotalBosses: 8,
		Summary:     []byte("{}"),
		SyncedAt:    now,
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
		body := decodeBody(t, r)
		if len(body.([]any)) != 2 {
			t.Fatalf("roster=%v", body)
		}
		alphaJSON := body.([]any)[0].(map[string]any)
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
		if body["rosterSize"] != float64(2) || len(body["classDistribution"].([]any)) == 0 || body["lastSync"] == nil {
			t.Fatalf("dashboard=%v", body)
		}
	})
	t.Run("roster filters", func(t *testing.T) {
		for _, path := range []string{"/api/roster?search=lph", "/api/roster?class=Mage"} {
			r := request(h, http.MethodGet, path, member)
			if r.Code != http.StatusOK {
				t.Fatalf("path=%s status=%d body=%s", path, r.Code, r.Body.String())
			}
			body := decodeBody(t, r).([]any)
			if len(body) != 1 || body[0].(map[string]any)["name"] != "Alpha" {
				t.Fatalf("path=%s roster=%v", path, body)
			}
		}
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
		assertStatus(t, h, http.MethodGet, "/api/characters/999999", member, http.StatusNotFound)
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
}

func seedCharacter(
	t *testing.T,
	ctx context.Context,
	stores store.Store,
	guildID int64,
	name, class string,
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
		SpecName:       "Arcane",
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
