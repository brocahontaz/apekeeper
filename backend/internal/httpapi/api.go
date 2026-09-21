package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/auth"
	"github.com/brocahontaz/apekeeper/backend/internal/db"
	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/brocahontaz/apekeeper/backend/internal/store"
)

type Trigger interface {
	Trigger(context.Context) (int64, error)
}
type TriggerFunc func(context.Context) (int64, error)

func (f TriggerFunc) Trigger(c context.Context) (int64, error) { return f(c) }

type API struct {
	Stores    store.Store
	Auth      *auth.Manager
	GuildSlug string
	Trigger   Trigger
	Ping      db.Pinger
	Now       func() time.Time
	// Frontend optionally supplies a built SPA filesystem. When nil, the
	// conventional ./frontend/dist directory is used if it exists.
	Frontend  fs.FS
	StaticDir string
}
type contextKey struct{}

func (a API) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}
func New(a API) http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/auth/login", a.Auth.Login)
	m.HandleFunc("GET /api/auth/callback", a.Auth.Callback)
	m.Handle("POST /api/auth/logout", a.requireAuth(http.HandlerFunc(a.Auth.Logout)))
	m.Handle("GET /api/auth/me", a.requireAuth(http.HandlerFunc(a.me)))
	m.HandleFunc("GET /healthz", a.health)
	m.HandleFunc("GET /api/healthz", a.health)
	m.Handle("GET /api/dashboard", a.requireAuth(http.HandlerFunc(a.dashboard)))
	m.Handle("GET /api/roster", a.requireAuth(http.HandlerFunc(a.roster)))
	m.Handle("GET /api/characters/{id}", a.requireAuth(http.HandlerFunc(a.character)))
	m.Handle("GET /api/sync/runs", a.requireRole([]string{"admin", "officer"}, http.HandlerFunc(a.syncRuns)))
	m.Handle("POST /api/sync/run", a.requireRole([]string{"admin"}, http.HandlerFunc(a.triggerSync)))
	return spa(m, a.Frontend, a.StaticDir)
}
func spa(api http.Handler, frontend fs.FS, staticDir string) http.Handler {
	if frontend == nil {
		if staticDir == "" {
			staticDir = "./frontend/dist"
		}
		if _, err := os.Stat(staticDir); err != nil {
			return api
		}
		frontend = os.DirFS(staticDir)
	}
	files := http.FileServer(http.FS(frontend))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || strings.HasPrefix(r.URL.Path, "/api") || r.URL.Path == "/healthz" {
			api.ServeHTTP(w, r)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "." || path.Ext(name) == "" {
			body, err := fs.ReadFile(frontend, "index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(body)
			return
		}
		files.ServeHTTP(w, r)
	})
}
func jsonOut(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) {
	jsonOut(w, status, map[string]string{"error": msg})
}
func (a API) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, e := a.Auth.CurrentUser(r.Context(), r)
		if e != nil {
			fail(w, 401, "unauthenticated")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, u)))
	})
}
func (a API) requireRole(roles []string, next http.Handler) http.Handler {
	return a.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := r.Context().Value(contextKey{}).(domain.User)
		for _, role := range roles {
			if u.Role == role {
				next.ServeHTTP(w, r)
				return
			}
		}
		fail(w, 403, "forbidden")
	}))
}
func (a API) guild(ctx context.Context) (domain.Guild, error) {
	return a.Stores.Guilds.BySlug(ctx, a.GuildSlug)
}
func (a API) health(w http.ResponseWriter, r *http.Request) {
	if a.Ping != nil && a.Ping.Ping(r.Context()) != nil {
		jsonOut(w, 503, map[string]string{"status": "ok", "db": "degraded"})
		return
	}
	jsonOut(w, 200, map[string]string{"status": "ok", "db": "ok"})
}
func (a API) me(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(contextKey{}).(domain.User)
	jsonOut(w, 200, map[string]any{"id": u.ID, "displayName": u.DisplayName, "battletag": u.BattleTag, "appRole": u.Role})
}
func (a API) roster(w http.ResponseWriter, r *http.Request) {
	g, e := a.guild(r.Context())
	if e != nil {
		fail(w, 500, "guild lookup failed")
		return
	}
	q := r.URL.Query()
	f := store.CharacterFilter{Class: q.Get("class"), Spec: q.Get("spec"), Name: q.Get("search")}
	if v, ok := integer(q.Get("rank")); ok {
		f.Rank = &v
	}
	if v, ok := integer(q.Get("minLevel")); ok {
		f.MinLevel = &v
	}
	if v, e := strconv.ParseFloat(q.Get("minRating"), 64); e == nil && q.Get("minRating") != "" {
		f.MinRating = &v
	}
	if q.Get("stale") == "true" {
		x := a.now().Add(-7 * 24 * time.Hour)
		f.StaleBefore = &x
	}
	rows, e := a.Stores.Characters.ListByGuild(r.Context(), g.ID, f)
	if e != nil {
		fail(w, 500, "roster lookup failed")
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, c := range rows {
		out = append(out, characterJSON(c, a.now()))
	}
	jsonOut(w, 200, out)
}
func integer(s string) (int, bool) { v, e := strconv.Atoi(s); return v, e == nil && s != "" }
func characterJSON(c domain.Character, now time.Time) map[string]any {
	return map[string]any{"id": c.ID, "name": c.DisplayName, "realm": c.Realm, "classId": c.ClassID, "className": c.ClassName, "specId": c.SpecID, "specName": c.SpecName, "level": c.Level, "itemLevel": c.ItemLevel, "guildRank": c.GuildRank, "mythicRating": c.MythicRating, "bestKeyLevel": c.BestKeyLevel, "syncedAt": c.SyncedAt, "stale": c.IsStale(now, 7*24*time.Hour)}
}
func (a API) character(w http.ResponseWriter, r *http.Request) {
	g, e := a.guild(r.Context())
	if e != nil {
		fail(w, 500, "guild lookup failed")
		return
	}
	id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if e != nil {
		fail(w, 400, "invalid character id")
		return
	}
	d, e := a.Stores.Characters.Detail(r.Context(), g.ID, id, a.now().Add(-30*24*time.Hour))
	if e != nil {
		fail(w, 404, "character not found")
		return
	}
	out := characterJSON(d.Character, a.now())
	out["avatarUrl"] = d.AvatarURL
	out["mythicPlus"] = d.Mythic
	out["raidProgression"] = d.Raids
	out["snapshots"] = d.Snapshots
	jsonOut(w, 200, out)
}
func (a API) dashboard(w http.ResponseWriter, r *http.Request) {
	g, e := a.guild(r.Context())
	if e != nil {
		fail(w, 500, "guild lookup failed")
		return
	}
	chars, e := a.Stores.Characters.ListByGuild(r.Context(), g.ID, store.CharacterFilter{})
	if e != nil {
		fail(w, 500, "dashboard lookup failed")
		return
	}
	classes := map[string]map[string]any{}
	specs := map[string]int{}
	max, active, stale := 0, 0, 0
	staleList := []map[string]any{}
	top := []map[string]any{}
	var ratingTotal float64
	rated := 0
	raids := map[string]map[string]map[string]int{}
	notable := []map[string]any{}
	for _, c := range chars {
		if classes[c.ClassName] == nil {
			classes[c.ClassName] = map[string]any{"classId": c.ClassID, "className": c.ClassName, "count": 0}
		}
		classes[c.ClassName]["count"] = classes[c.ClassName]["count"].(int) + 1
		specs[c.SpecName]++
		if c.Level >= domain.CurrentMaxLevel {
			max++
		}
		if c.ActiveCharacter(a.now(), 7*24*time.Hour) {
			active++
		}
		if c.IsStale(a.now(), 7*24*time.Hour) {
			stale++
			if len(staleList) < 10 {
				staleList = append(staleList, map[string]any{"name": c.DisplayName, "realm": c.Realm, "syncedAt": c.SyncedAt, "stalenessDays": int(a.now().Sub(c.SyncedAt).Hours() / 24)})
			}
		}
		d, err := a.Stores.Characters.Detail(r.Context(), g.ID, c.ID, a.now().Add(-30*24*time.Hour))
		if err != nil {
			continue
		}
		bestRating, bestKey := 0.0, 0
		for _, m := range d.Mythic {
			if m.OverallRating > bestRating {
				bestRating, bestKey = m.OverallRating, m.BestKeyLevel
			}
		}
		if bestRating > 0 {
			rated++
			ratingTotal += bestRating
			top = append(top, map[string]any{"name": c.DisplayName, "realm": c.Realm, "rating": bestRating, "bestKey": bestKey})
		}
		for _, rp := range d.Raids {
			if raids[rp.RaidName] == nil {
				raids[rp.RaidName] = map[string]map[string]int{}
			}
			if raids[rp.RaidName][rp.Difficulty] == nil {
				raids[rp.RaidName][rp.Difficulty] = map[string]int{"progress": 0, "totalBosses": rp.TotalBosses}
			}
			if rp.Progress > raids[rp.RaidName][rp.Difficulty]["progress"] {
				raids[rp.RaidName][rp.Difficulty]["progress"] = rp.Progress
			}
		}
		if len(d.Snapshots) > 1 {
			first, last := d.Snapshots[0], d.Snapshots[len(d.Snapshots)-1]
			if first.CapturedAt.Before(a.now().Add(-14*24*time.Hour)) && (last.MythicRating > first.MythicRating || last.ItemLevel > first.ItemLevel) {
				notable = append(notable, map[string]any{"name": c.DisplayName, "ratingDelta": last.MythicRating - first.MythicRating, "itemLevelDelta": last.ItemLevel - first.ItemLevel})
			}
		}
	}
	dist := func(m map[string]int) []map[string]any {
		x := make([]map[string]any, 0, len(m))
		for n, c := range m {
			x = append(x, map[string]any{"className": n, "count": c})
		}
		return x
	}
	classDist := make([]map[string]any, 0, len(classes))
	for _, v := range classes {
		classDist = append(classDist, v)
	}
	for i := 0; i < len(top); i++ {
		for j := i + 1; j < len(top); j++ {
			if top[j]["rating"].(float64) > top[i]["rating"].(float64) {
				top[i], top[j] = top[j], top[i]
			}
		}
	}
	if len(top) > 10 {
		top = top[:10]
	}
	raidOut := []map[string]any{}
	for name, diffs := range raids {
		raidOut = append(raidOut, map[string]any{"raidName": name, "difficulties": diffs})
	}
	if len(notable) > 10 {
		notable = notable[:10]
	}
	runs, _ := a.Stores.SyncRuns.History(r.Context(), g.ID, 1)
	var last any = map[string]any{}
	if len(runs) > 0 {
		last = runs[0]
	}
	average := 0.0
	if rated > 0 {
		average = ratingTotal / float64(rated)
	}
	jsonOut(w, 200, map[string]any{"rosterSize": len(chars), "maxLevelMembers": max, "activeMembers": active, "classDistribution": classDist, "specDistribution": dist(specs), "mythicPlus": map[string]any{"top": top, "averageRating": average, "ratedCount": rated}, "raidProgression": raidOut, "staleCharacters": map[string]any{"count": stale, "characters": staleList}, "lastSync": last, "notableChanges": notable})
}
func (a API) syncRuns(w http.ResponseWriter, r *http.Request) {
	g, e := a.guild(r.Context())
	if e != nil {
		fail(w, 500, "guild lookup failed")
		return
	}
	limit := 20
	if n, ok := integer(r.URL.Query().Get("limit")); ok && n > 0 && n <= 100 {
		limit = n
	}
	rows, e := a.Stores.SyncRuns.History(r.Context(), g.ID, limit)
	if e != nil {
		fail(w, 500, "sync history lookup failed")
		return
	}
	jsonOut(w, 200, rows)
}
func (a API) triggerSync(w http.ResponseWriter, r *http.Request) {
	if a.Trigger == nil {
		fail(w, 503, "sync unavailable")
		return
	}
	id, e := a.Trigger.Trigger(r.Context())
	if e != nil {
		if strings.Contains(e.Error(), "already running") {
			fail(w, 409, "sync already running")
			return
		}
		fail(w, 500, "could not trigger sync")
		return
	}
	jsonOut(w, 202, map[string]any{"runId": id})
}

var _ = errors.New
