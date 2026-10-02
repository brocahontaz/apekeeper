package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/auth"
	"github.com/brocahontaz/apekeeper/backend/internal/db"
	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/brocahontaz/apekeeper/backend/internal/logger"
	"github.com/brocahontaz/apekeeper/backend/internal/store"
)

type Trigger interface {
	Trigger(context.Context) (int64, error)
}
type TriggerFunc func(context.Context) (int64, error)

func (f TriggerFunc) Trigger(c context.Context) (int64, error) {
	return f(c)
}

// ProgressReporter optionally supplies the live sync progress snapshot.
// Keeping it separate from Trigger lets existing trigger implementations and
// tests compile unchanged; the sync service provides the real source.
type ProgressReporter interface {
	Progress() domain.Progress
}
type SyncOperations interface {
	StartDryRun(context.Context) (store.SyncRun, error)
	StartRetry(context.Context, int64) (store.SyncRun, error)
	Cancel() error
}

type API struct {
	Stores    store.Store
	Auth      *auth.Manager
	GuildSlug string
	Trigger   Trigger
	// ScopedTrigger/ScopedOperations/ScopedProgress are used by multi-guild
	// deployments. The legacy fields remain as a compatibility path for the
	// configured default guild, but are never used for another selection.
	ScopedTrigger    func(context.Context, domain.Guild) (int64, error)
	ScopedOperations func(domain.Guild) SyncOperations
	ScopedProgress   func(domain.Guild) ProgressReporter
	// Progress optionally backs GET /api/sync/progress; when nil the
	// endpoint answers with an idle, zero-valued snapshot.
	Progress   ProgressReporter
	Operations SyncOperations
	Ping       db.Pinger
	Now        func() time.Time
	// SnapshotRetentionDays is surfaced with history responses. Zero preserves
	// the configured application's historical default of 90 days.
	SnapshotRetentionDays int
	// Frontend optionally supplies a built SPA filesystem. When nil, the
	// conventional ./frontend/dist directory is used if it exists.
	Frontend        fs.FS
	StaticDir       string
	CSRFEnabled     bool
	MaxRequestBody  int64
	RateLimit       int
	RateLimitWindow time.Duration
	Security        SecurityLimits
	ReadinessChecks map[string]func(context.Context) error
	ShuttingDown    func() bool
}
type contextKey struct{}
type guildContextKey struct{}

const guildCookie = "apekeeper_guild"

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
	m.Handle("GET /api/guilds", a.requireAuth(http.HandlerFunc(a.guilds)))
	m.Handle("POST /api/guilds/select", a.requireAuth(http.HandlerFunc(a.selectGuild)))
	m.HandleFunc("GET /healthz", a.health)
	m.HandleFunc("GET /api/healthz", a.health)
	m.HandleFunc("GET /readyz", a.readiness)
	m.HandleFunc("GET /api/readyz", a.readiness)
	m.Handle("GET /api/dashboard", a.requireGuild(http.HandlerFunc(a.dashboard)))
	m.Handle("GET /api/roster", a.requireGuild(http.HandlerFunc(a.roster)))
	m.Handle("GET /api/roster/export", a.requireGuild(http.HandlerFunc(a.rosterExport)))
	m.Handle("GET /api/characters/{id}", a.requireGuild(http.HandlerFunc(a.character)))
	m.Handle("GET /api/characters/{id}/history", a.requireGuild(http.HandlerFunc(a.characterHistory)))
	m.Handle("GET /api/sync/runs",
		a.requireRole([]string{"superadmin", "admin", "officer"}, http.HandlerFunc(a.syncRuns)))
	m.Handle("GET /api/sync/progress",
		a.requireRole([]string{"superadmin", "admin", "officer"}, http.HandlerFunc(a.syncProgress)))
	m.Handle("POST /api/sync/run",
		a.requireRole([]string{"superadmin", "admin"}, http.HandlerFunc(a.triggerSync)))
	m.Handle("POST /api/sync/dry-run", a.requireRole([]string{"superadmin", "admin"}, http.HandlerFunc(a.dryRun)))
	m.Handle("POST /api/sync/runs/{id}/retry", a.requireRole([]string{"superadmin", "admin"}, http.HandlerFunc(a.retrySync)))
	m.Handle("POST /api/sync/cancel", a.requireRole([]string{"superadmin", "admin"}, http.HandlerFunc(a.cancelSync)))
	officer := []string{"superadmin", "admin", "officer"}
	m.Handle("GET /api/officer/queue", a.requireRole(officer, http.HandlerFunc(a.officerQueue)))
	m.Handle("POST /api/officer/queue/complete", a.requireRole(officer, http.HandlerFunc(a.officerComplete)))
	m.Handle("POST /api/officer/bulk-tags", a.requireRole(officer, http.HandlerFunc(a.officerBulkTags)))
	m.Handle("GET /api/officer/characters/{id}", a.requireRole(officer, http.HandlerFunc(a.officerCharacterMetadata)))
	m.Handle("PUT /api/officer/characters/{id}", a.requireRole(officer, http.HandlerFunc(a.officerCharacter)))
	m.Handle("GET /api/officer/activity", a.requireRole(officer, http.HandlerFunc(a.officerActivity)))
	m.Handle("GET /api/guild/members", a.requireRole([]string{"superadmin", "owner", "admin"}, http.HandlerFunc(a.members)))
	m.Handle("GET /api/guild/members/search", a.requireRole([]string{"superadmin", "owner", "admin"}, http.HandlerFunc(a.memberSearch)))
	m.Handle("POST /api/guild/members", a.requireRole([]string{"superadmin", "owner", "admin"}, http.HandlerFunc(a.inviteMember)))
	m.Handle("PUT /api/guild/members/{id}", a.requireRole([]string{"superadmin", "owner", "admin"}, http.HandlerFunc(a.updateMember)))
	m.Handle("DELETE /api/guild/members/{id}", a.requireRole([]string{"superadmin", "owner", "admin"}, http.HandlerFunc(a.removeMember)))
	var h http.Handler = m
	if a.CSRFEnabled {
		h = csrfCookie(csrf(h))
	}
	if a.MaxRequestBody > 0 || a.Security.AuthBody > 0 {
		inner := h
		h = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			max := a.MaxRequestBody
			if _, body := a.Security.category(r); body > 0 {
				max = body
			}
			if max > 0 {
				r.Body = http.MaxBytesReader(w, r.Body, max)
			}
			inner.ServeHTTP(w, r)
		})
	}
	if a.Security.Auth > 0 || a.Security.Sync > 0 || a.Security.Export > 0 || a.Security.Mutation > 0 {
		a.Security.Window = a.RateLimitWindow
		h = limitRequestsByCategory(a.Security, h)
	} else if a.RateLimit > 0 {
		window := a.RateLimitWindow
		if window <= 0 {
			window = time.Minute
		}
		h = limitRequests(a.RateLimit, window, h)
	}
	return spa(h, a.Frontend, a.StaticDir)
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
		if r.Method != http.MethodGet || strings.HasPrefix(r.URL.Path, "/api") || r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
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
	return a.requireGuild(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := r.Context().Value(contextKey{}).(domain.User)
		membership := r.Context().Value(guildContextKey{}).(domain.GuildMembership)
		for _, role := range roles {
			if roleAllowed(u, membership, role) {
				next.ServeHTTP(w, r)
				return
			}
		}
		fail(w, 403, "forbidden")
	}))
}

// roleAllowed is the single authorization rule for guild-scoped endpoints.
// Guild roles are hierarchical: an owner retains every lower guild capability
// while the platform superadmin bypasses guild role boundaries.
func roleAllowed(u domain.User, membership domain.GuildMembership, required string) bool {
	if u.Role == "superadmin" {
		return true
	}
	if required == "superadmin" {
		return false
	}
	levels := map[string]int{"member": 1, "officer": 2, "admin": 3, "owner": 4}
	requiredLevel, ok := levels[required]
	if !ok {
		return false
	}
	return levels[membership.Role] >= requiredLevel
}
func (a API) requireGuild(next http.Handler) http.Handler {
	return a.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := r.Context().Value(contextKey{}).(domain.User)
		memberships, err := a.Stores.Memberships.List(r.Context(), u.ID)
		if err != nil {
			fail(w, 500, "guild membership lookup failed")
			return
		}
		var chosen domain.GuildMembership
		if c, e := r.Cookie(guildCookie); e == nil {
			for _, m := range memberships {
				if m.Guild.Slug == c.Value {
					chosen = m
					break
				}
			}
			if chosen.Guild.ID == 0 && u.Role == "superadmin" {
				chosen.Guild, err = a.Stores.Guilds.BySlug(r.Context(), c.Value)
				chosen.UserID = u.ID
				chosen.Role = "superadmin"
			}
		} else if a.GuildSlug != "" {
			for _, m := range memberships {
				if m.Guild.Slug == a.GuildSlug {
					chosen = m
					break
				}
			}
			if chosen.Guild.ID == 0 && u.Role == "superadmin" {
				chosen.Guild, err = a.Stores.Guilds.BySlug(r.Context(), a.GuildSlug)
				chosen.UserID = u.ID
				chosen.Role = "superadmin"
			}
		}
		if chosen.Guild.ID == 0 && len(memberships) > 0 {
			chosen = memberships[0]
		}
		if chosen.Guild.ID == 0 {
			fail(w, 403, "guild membership required")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), guildContextKey{}, chosen)))
	}))
}
func (a API) guild(ctx context.Context) (domain.Guild, error) {
	if g, ok := ctx.Value(guildContextKey{}).(domain.GuildMembership); ok {
		return g.Guild, nil
	}
	return a.Stores.Guilds.BySlug(ctx, a.GuildSlug)
}
func (a API) health(w http.ResponseWriter, r *http.Request) {
	if a.Ping != nil && a.Ping.Ping(r.Context()) != nil {
		jsonOut(w, 503, map[string]string{"status": "ok", "db": "degraded"})
		return
	}
	jsonOut(w, 200, map[string]string{"status": "ok", "db": "ok"})
}
func (a API) readiness(w http.ResponseWriter, r *http.Request) {
	components := map[string]string{"db": "ok"}
	status := http.StatusOK
	if a.Ping != nil && a.Ping.Ping(r.Context()) != nil {
		components["db"] = "degraded"
		status = http.StatusServiceUnavailable
	}
	for name, check := range a.ReadinessChecks {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		err := check(ctx)
		cancel()
		if err != nil {
			components[name] = "degraded"
			status = http.StatusServiceUnavailable
		} else {
			components[name] = "ok"
		}
	}
	if a.ShuttingDown != nil && a.ShuttingDown() {
		components["shutdown"] = "draining"
		status = http.StatusServiceUnavailable
	}
	jsonOut(w, status, map[string]any{"status": map[bool]string{true: "ready", false: "degraded"}[status == http.StatusOK], "components": components})
}
func (a API) audit(r *http.Request, event string) error {
	u, _ := r.Context().Value(contextKey{}).(domain.User)
	g, _ := a.guild(r.Context())
	return a.Stores.Users.Audit(r.Context(), u.ID, g.ID, event)
}
func (a API) audited(w http.ResponseWriter, r *http.Request, event string) bool {
	if err := a.audit(r, event); err != nil {
		fail(w, http.StatusInternalServerError, "audit recording failed")
		return false
	}
	return true
}
func (a API) me(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(contextKey{}).(domain.User)
	jsonOut(w, 200, map[string]any{
		"id":          u.ID,
		"displayName": u.DisplayName,
		"battletag":   u.BattleTag,
		"appRole":     u.Role,
	})
}
func (a API) guilds(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(contextKey{}).(domain.User)
	rows, err := a.Stores.Memberships.List(r.Context(), u.ID)
	if err != nil {
		fail(w, 500, "guild membership lookup failed")
		return
	}
	if u.Role == "superadmin" {
		all, e := a.Stores.Guilds.List(r.Context())
		if e != nil {
			fail(w, 500, "guild lookup failed")
			return
		}
		rows = make([]domain.GuildMembership, 0, len(all))
		for _, g := range all {
			rows = append(rows, domain.GuildMembership{Guild: g, UserID: u.ID, Role: "superadmin"})
		}
	}
	out := make([]map[string]any, 0, len(rows))
	for _, m := range rows {
		selected := false
		if c, e := r.Cookie(guildCookie); e == nil {
			selected = c.Value == m.Guild.Slug
		} else if a.GuildSlug != "" {
			selected = a.GuildSlug == m.Guild.Slug
		}
		out = append(out, map[string]any{"id": m.Guild.ID, "slug": m.Guild.Slug, "name": m.Guild.Name, "realm": m.Guild.Realm, "region": m.Guild.Region, "role": m.Role, "selected": selected})
	}
	jsonOut(w, 200, out)
}
func (a API) selectGuild(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Slug string `json:"slug"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil || in.Slug == "" {
		fail(w, 400, "guild slug is required")
		return
	}
	u := r.Context().Value(contextKey{}).(domain.User)
	rows, err := a.Stores.Memberships.List(r.Context(), u.ID)
	if err != nil {
		fail(w, 500, "guild membership lookup failed")
		return
	}
	var found bool
	for _, m := range rows {
		if m.Guild.Slug == in.Slug {
			found = true
			break
		}
	}
	if !found && u.Role != "superadmin" {
		fail(w, 403, "guild membership required")
		return
	}
	if !found {
		if _, err = a.Stores.Guilds.BySlug(r.Context(), in.Slug); err != nil {
			fail(w, 404, "guild not found")
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: guildCookie, Value: in.Slug, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil})
	jsonOut(w, 200, map[string]string{"slug": in.Slug})
}
func (a API) members(w http.ResponseWriter, r *http.Request) {
	g, _ := a.guild(r.Context())
	rows, err := a.Stores.Users.GuildMembers(r.Context(), g.ID)
	if err != nil {
		fail(w, 500, "member lookup failed")
		return
	}
	jsonOut(w, 200, rows)
}
func (a API) memberSearch(w http.ResponseWriter, r *http.Request) {
	g, _ := a.guild(r.Context())
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) > 120 {
		fail(w, 400, "search query is too long")
		return
	}
	rows, err := a.Stores.Users.SearchGuildCandidates(r.Context(), g.ID, query)
	if err != nil {
		fail(w, 500, "member search failed")
		return
	}
	jsonOut(w, 200, rows)
}
func validMembershipRole(role string) bool {
	return map[string]bool{"owner": true, "admin": true, "officer": true, "member": true}[role]
}
func membershipChangeAllowed(actor domain.GuildMembership, target domain.GuildMembership, role string) bool {
	if actor.Role == "superadmin" {
		return true
	}
	if role == "owner" && actor.Role != "owner" {
		return false
	}
	if target.Role == "owner" && actor.Role != "owner" {
		return false
	}
	return actor.Role == "owner" || actor.Role == "admin"
}
func (a API) inviteMember(w http.ResponseWriter, r *http.Request) {
	var in struct {
		BattleTag string `json:"battletag"`
		Role      string `json:"role"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil || strings.TrimSpace(in.BattleTag) == "" || !validMembershipRole(in.Role) {
		fail(w, 400, "battletag and valid membership role are required")
		return
	}
	g, _ := a.guild(r.Context())
	actor := r.Context().Value(guildContextKey{}).(domain.GuildMembership)
	if u := r.Context().Value(contextKey{}).(domain.User); u.Role == "superadmin" {
		actor.Role = "superadmin"
	}
	userID, err := a.Stores.Users.FindGuildCandidate(r.Context(), g.ID, strings.TrimSpace(in.BattleTag))
	if err != nil {
		fail(w, 404, "user not found or already a member")
		return
	}
	if !membershipChangeAllowed(actor, domain.GuildMembership{}, in.Role) {
		fail(w, 403, "forbidden")
		return
	}
	if err = a.Stores.Memberships.Grant(r.Context(), g.ID, userID, in.Role); err != nil {
		fail(w, 400, "could not grant membership")
		return
	}
	if !a.audited(w, r, "member_invite") {
		return
	}
	jsonOut(w, 201, map[string]any{"userId": userID, "role": in.Role})
}
func (a API) updateMember(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, 400, "invalid member id")
		return
	}
	var in struct {
		Role string `json:"role"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil || !validMembershipRole(in.Role) {
		fail(w, 400, "invalid membership role")
		return
	}
	g, _ := a.guild(r.Context())
	actor := r.Context().Value(guildContextKey{}).(domain.GuildMembership)
	if u := r.Context().Value(contextKey{}).(domain.User); u.Role == "superadmin" {
		actor.Role = "superadmin"
	}
	target, targetErr := a.Stores.Memberships.ForUser(r.Context(), id, g.ID)
	if targetErr != nil || !membershipChangeAllowed(actor, target, in.Role) {
		fail(w, http.StatusForbidden, "forbidden")
		return
	}
	if err = a.Stores.Memberships.Grant(r.Context(), g.ID, id, in.Role); err != nil {
		fail(w, 400, "could not update membership")
		return
	}
	if !a.audited(w, r, "role_change") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (a API) removeMember(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, 400, "invalid member id")
		return
	}
	g, _ := a.guild(r.Context())
	actor := r.Context().Value(guildContextKey{}).(domain.GuildMembership)
	if u := r.Context().Value(contextKey{}).(domain.User); u.Role == "superadmin" {
		actor.Role = "superadmin"
	}
	if target, e := a.Stores.Memberships.ForUser(r.Context(), id, g.ID); e != nil || !membershipChangeAllowed(actor, target, "member") {
		fail(w, http.StatusForbidden, "forbidden")
		return
	}
	if err = a.Stores.Memberships.Revoke(r.Context(), g.ID, id); err != nil {
		fail(w, 400, "could not remove membership")
		return
	}
	if !a.audited(w, r, "member_remove") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// parseRosterQuery validates the roster filters, sort column, and sort
// direction shared by the roster page and the CSV export. defaultSort applies
// when the query omits sort, so each surface keeps its own default ordering.
// An invalid sort or direction yields the same 400 messages the roster page
// has always sent.
func parseRosterQuery(q url.Values, staleBefore time.Time, defaultSort store.CharacterSort) (store.CharacterFilter, store.CharacterSort, bool, error) {
	f := store.CharacterFilter{
		Class: q.Get("class"),
		Spec:  q.Get("spec"),
		Name:  q.Get("search"),
	}
	if v, ok := integer(q.Get("rank")); ok {
		if v < 0 {
			return f, "", false, errors.New("invalid roster rank")
		}
		f.Rank = &v
	} else if q.Get("rank") != "" {
		return f, "", false, errors.New("invalid roster rank")
	}
	if v, ok := integer(q.Get("minLevel")); ok {
		if v < 0 || v > 100 {
			return f, "", false, errors.New("invalid minimum level")
		}
		f.MinLevel = &v
	} else if q.Get("minLevel") != "" {
		return f, "", false, errors.New("invalid minimum level")
	}
	if v, e := strconv.ParseFloat(q.Get("minRating"), 64); e == nil && q.Get("minRating") != "" {
		if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return f, "", false, errors.New("invalid minimum rating")
		}
		f.MinRating = &v
	} else if q.Get("minRating") != "" {
		return f, "", false, errors.New("invalid minimum rating")
	}
	if q.Get("stale") == "true" {
		f.StaleBefore = &staleBefore
	} else if q.Get("stale") != "" && q.Get("stale") != "false" {
		return f, "", false, errors.New("invalid stale filter")
	}
	var aliasErr error
	if f.MythicSeason, aliasErr = rosterAlias(q, "mythicSeason", "season"); aliasErr != nil {
		return f, "", false, aliasErr
	}
	if f.RaidTier, aliasErr = rosterAlias(q, "raidTier", "raid"); aliasErr != nil {
		return f, "", false, aliasErr
	}
	f.RaidDifficulty = q.Get("raidDifficulty")
	f.Role = q.Get("role")
	if f.OfficerStatus, aliasErr = rosterAlias(q, "officerStatus", "status"); aliasErr != nil {
		return f, "", false, aliasErr
	}
	if f.OfficerTag, aliasErr = rosterAlias(q, "officerTag", "tag"); aliasErr != nil {
		return f, "", false, aliasErr
	}
	if len(f.Name) > 120 || len(f.Class) > 80 || len(f.Spec) > 80 || len(f.MythicSeason) > 80 || len(f.RaidTier) > 120 || len(f.RaidDifficulty) > 40 || len(f.OfficerStatus) > 20 || len(f.OfficerTag) > 32 || len(f.Role) > 20 {
		return f, "", false, errors.New("roster filter is too long")
	}
	if f.Role != "" && !map[string]bool{"member": true, "officer": true, "admin": true, "superadmin": true}[f.Role] {
		return f, "", false, errors.New("invalid roster role")
	}
	if f.OfficerStatus != "" && !domain.LifecycleStatuses[f.OfficerStatus] {
		return f, "", false, errors.New("invalid officer status")
	}
	if f.RaidDifficulty == "" && q.Get("raidDifficulty") != "" {
		return f, "", false, errors.New("invalid raid difficulty")
	}
	progress, aliasErr := rosterAlias(q, "minRaidProgress", "raidProgress")
	if aliasErr != nil {
		return f, "", false, aliasErr
	}
	if v, ok := integer(progress); ok {
		if v < 0 {
			return f, "", false, errors.New("invalid raid progress")
		}
		f.MinRaidProgress = &v
	} else if progress != "" {
		return f, "", false, errors.New("invalid raid progress")
	}
	age, aliasErr := rosterAlias(q, "activityAge", "activityAgeDays")
	if aliasErr != nil {
		return f, "", false, aliasErr
	}
	if age != "" {
		v, ok := integer(age)
		if !ok || v < 0 || v > 3650 {
			return f, "", false, errors.New("invalid activity age")
		}
		f.MaxAge = ptrDuration(time.Duration(v) * 24 * time.Hour)
	}
	sort, ok := store.ParseCharacterSort(q.Get("sort"))
	if q.Get("sort") == "" {
		sort = defaultSort
	}
	if !ok && q.Get("sort") != "" {
		return f, sort, false, errors.New("invalid roster sort")
	}
	descending := q.Get("direction") == "descending"
	if direction := q.Get("direction"); direction != "" && direction != "ascending" && direction != "descending" {
		return f, sort, false, errors.New("invalid roster sort direction")
	}
	return f, sort, descending, nil
}
func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
func rosterAlias(q url.Values, canonical, alias string) (string, error) {
	a, b := q.Get(canonical), q.Get(alias)
	if a != "" && b != "" && a != b {
		return "", fmt.Errorf("contradictory roster parameters %q and %q", canonical, alias)
	}
	return first(a, b), nil
}
func ptrDuration(v time.Duration) *time.Duration { return &v }

func (a API) roster(w http.ResponseWriter, r *http.Request) {
	g, e := a.guild(r.Context())
	if e != nil {
		fail(w, 500, "guild lookup failed")
		return
	}
	q := r.URL.Query()
	f, sort, descending, e := parseRosterQuery(q, a.now().Add(-7*24*time.Hour), store.CharacterSortGuildRank)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	page, pageSize := 1, 25
	if q.Get("page") != "" {
		v, ok := integer(q.Get("page"))
		if !ok || v < 1 {
			fail(w, 400, "invalid roster page")
			return
		}
		page = v
	}
	if q.Get("pageSize") != "" {
		v, ok := integer(q.Get("pageSize"))
		if !ok || v < 1 || v > 100 {
			fail(w, 400, "invalid roster page size")
			return
		}
		pageSize = v
	}
	if page-1 > int(^uint(0)>>1)/pageSize {
		fail(w, 400, "roster page is too large")
		return
	}
	result, e := a.Stores.Characters.ListPageByGuild(r.Context(), g.ID, f, pageSize, (page-1)*pageSize, sort, descending)
	if e != nil {
		fail(w, 500, "roster lookup failed")
		return
	}
	classes, e := a.Stores.Characters.ListClassesByGuild(r.Context(), g.ID)
	if e != nil {
		fail(w, 500, "roster class lookup failed")
		return
	}
	specs, e := a.Stores.Characters.ListSpecsByGuild(r.Context(), g.ID)
	if e != nil {
		fail(w, 500, "roster spec lookup failed")
		return
	}
	out := make([]map[string]any, 0, len(result.Items))
	for _, c := range result.Items {
		out = append(out, characterJSON(c, a.now()))
	}
	jsonOut(w, 200, map[string]any{"items": out, "total": result.Total, "page": page, "pageSize": pageSize, "classes": classes, "specs": specs})
}
func integer(s string) (int, bool) {
	v, e := strconv.Atoi(s)
	return v, e == nil && s != ""
}
func characterJSON(c domain.Character, now time.Time) map[string]any {
	return map[string]any{
		"id":           c.ID,
		"name":         c.DisplayName,
		"realm":        c.Realm,
		"classId":      c.ClassID,
		"className":    c.ClassName,
		"specId":       c.SpecID,
		"specName":     c.SpecName,
		"role":         c.Role,
		"level":        c.Level,
		"itemLevel":    c.ItemLevel,
		"guildRank":    c.GuildRank,
		"raceName":     c.RaceName,
		"gender":       c.Gender,
		"mythicRating": c.MythicRating,
		"bestKeyLevel": c.BestKeyLevel,
		"syncedAt":     c.SyncedAt,
		"stale":        c.IsStale(now, 7*24*time.Hour),
	}
}
func (a API) character(w http.ResponseWriter, r *http.Request) {
	g, e := a.guild(r.Context())
	if e != nil {
		fail(w, 500, "guild lookup failed")
		return
	}
	identity := r.PathValue("id")
	var d store.CharacterDetail
	if id, parseErr := strconv.ParseInt(identity, 10, 64); parseErr == nil {
		d, e = a.Stores.Characters.Detail(r.Context(), g.ID, id, a.now().Add(-30*24*time.Hour))
	} else {
		d, e = a.Stores.Characters.DetailByName(r.Context(), g.ID, identity, a.now().Add(-30*24*time.Hour))
	}
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

const historyMaxDays = 90
const historyMaxLimit = 500

func parseHistoryQuery(q url.Values, now time.Time) (time.Time, time.Time, int, int64, int64, error) {
	to := now.UTC()
	from := to.AddDate(0, 0, -30)
	parseDate := func(key string, fallback time.Time) (time.Time, error) {
		v := q.Get(key)
		if v == "" {
			return fallback, nil
		}
		x, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return time.Time{}, errors.New("invalid history date")
		}
		return x.UTC(), nil
	}
	var err error
	if from, err = parseDate("from", from); err != nil {
		return from, to, 0, 0, 0, err
	}
	if to, err = parseDate("to", to); err != nil {
		return from, to, 0, 0, 0, err
	}
	if from.After(to) || to.Sub(from) > historyMaxDays*24*time.Hour || to.After(now.UTC().Add(24*time.Hour)) {
		return from, to, 0, 0, 0, errors.New("history range must be within 90 days")
	}
	limit := 200
	if v := q.Get("limit"); v != "" {
		var ok bool
		limit, ok = integer(v)
		if !ok || limit < 1 || limit > historyMaxLimit {
			return from, to, 0, 0, 0, errors.New("invalid history limit")
		}
	}
	parseID := func(key string) (int64, error) {
		v := q.Get(key)
		if v == "" {
			return 0, nil
		}
		id, e := strconv.ParseInt(v, 10, 64)
		if e != nil || id <= 0 {
			return 0, errors.New("invalid comparison snapshot id")
		}
		return id, nil
	}
	left, err := parseID("compareFrom")
	if err != nil {
		return from, to, 0, 0, 0, err
	}
	right, err := parseID("compareTo")
	if err != nil {
		return from, to, 0, 0, 0, err
	}
	if (left == 0) != (right == 0) || left == right && left != 0 {
		return from, to, 0, 0, 0, errors.New("two distinct comparison snapshot ids are required")
	}
	return from, to, limit, left, right, nil
}

func (a API) characterHistory(w http.ResponseWriter, r *http.Request) {
	g, err := a.guild(r.Context())
	if err != nil {
		fail(w, 500, "guild lookup failed")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, 400, "invalid character id")
		return
	}
	from, to, limit, left, right, err := parseHistoryQuery(r.URL.Query(), a.now())
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	snaps, err := a.Stores.Progression.History(r.Context(), g.ID, id, from, to, limit)
	if err != nil {
		fail(w, 500, "history lookup failed")
		return
	}
	// An empty result is also how an inaccessible character appears, preventing
	// history probing across guild boundaries while retaining a useful empty UI.
	var comparison any
	if left != 0 {
		var before, after domain.Snapshot
		var foundL, foundR bool
		for _, s := range snaps {
			if s.ID == left {
				before, foundL = s, true
			}
			if s.ID == right {
				after, foundR = s, true
			}
		}
		// Validate selected IDs even when the bounded range returns fewer than
		// two snapshots; otherwise an invalid selection silently looks like no
		// comparison.
		if !foundL || !foundR {
			fail(w, 400, "comparison snapshots are unavailable")
			return
		}
		comparison = map[string]any{"from": before, "to": after, "itemLevelDelta": after.ItemLevel - before.ItemLevel, "mythicRatingDelta": after.MythicRating - before.MythicRating, "bestKeyLevelDelta": after.BestKeyLevel - before.BestKeyLevel}
	} else if len(snaps) >= 2 {
		before, after := snaps[len(snaps)-2], snaps[len(snaps)-1]
		comparison = map[string]any{"from": before, "to": after, "itemLevelDelta": after.ItemLevel - before.ItemLevel, "mythicRatingDelta": after.MythicRating - before.MythicRating, "bestKeyLevelDelta": after.BestKeyLevel - before.BestKeyLevel}
	}
	retentionDays := a.SnapshotRetentionDays
	if retentionDays <= 0 {
		retentionDays = historyMaxDays
	}
	jsonOut(w, 200, map[string]any{"from": from, "to": to, "retentionDays": retentionDays, "snapshots": snaps, "comparison": comparison})
}
func (a API) dashboard(w http.ResponseWriter, r *http.Request) {
	g, e := a.guild(r.Context())
	if e != nil {
		fail(w, 500, "guild lookup failed")
		return
	}
	chars, e := a.Stores.Characters.DashboardByGuild(r.Context(), g.ID, a.now().Add(-30*24*time.Hour))
	if e != nil {
		fail(w, 500, "dashboard lookup failed")
		return
	}
	classes := map[string]map[string]any{}
	max, active, stale := 0, 0, 0
	staleList := []map[string]any{}
	top := []map[string]any{}
	var ratingTotal float64
	rated := 0
	raids := map[string]map[string]map[string]int{}
	notable := []map[string]any{}
	for _, row := range chars {
		c := row.Character
		if classes[c.ClassName] == nil {
			classes[c.ClassName] = map[string]any{
				"classId":   c.ClassID,
				"className": c.ClassName,
				"count":     0,
				"specs":     map[string]int{},
			}
		}
		classes[c.ClassName]["count"] = classes[c.ClassName]["count"].(int) + 1
		if c.SpecName != "" {
			classes[c.ClassName]["specs"].(map[string]int)[c.SpecName]++
		}
		if c.Level >= domain.CurrentMaxLevel {
			max++
		}
		if c.ActiveCharacter(a.now(), 7*24*time.Hour) {
			active++
		}
		if c.IsStale(a.now(), 7*24*time.Hour) {
			stale++
			if len(staleList) < 10 {
				staleList = append(staleList, map[string]any{
					"name":          c.DisplayName,
					"realm":         c.Realm,
					"syncedAt":      c.SyncedAt,
					"stalenessDays": int(a.now().Sub(c.SyncedAt).Hours() / 24),
				})
			}
		}
		// MythicRating is the character's best season rating and BestKey the
		// keystone level of that season, both fetched by the single dashboard
		// query instead of a per-character Detail call.
		if c.MythicRating > 0 {
			rated++
			ratingTotal += c.MythicRating
			top = append(top, map[string]any{
				"name":    c.DisplayName,
				"realm":   c.Realm,
				"rating":  c.MythicRating,
				"bestKey": row.BestKey,
			})
		}
		for _, rp := range row.Raids {
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
		if row.SnapshotCount > 1 {
			first, last := row.First, row.Last
			if first.CapturedAt.Before(a.now().Add(-14*24*time.Hour)) &&
				(last.MythicRating > first.MythicRating || last.ItemLevel > first.ItemLevel) {
				notable = append(notable, map[string]any{
					"name":           c.DisplayName,
					"ratingDelta":    last.MythicRating - first.MythicRating,
					"itemLevelDelta": last.ItemLevel - first.ItemLevel,
				})
			}
		}
	}
	classDist := make([]map[string]any, 0, len(classes))
	for _, v := range classes {
		specDist := make([]map[string]any, 0, len(v["specs"].(map[string]int)))
		for name, count := range v["specs"].(map[string]int) {
			specDist = append(specDist, map[string]any{"name": name, "count": count})
		}
		sort.Slice(specDist, func(i, j int) bool {
			return specDist[i]["name"].(string) < specDist[j]["name"].(string)
		})
		v["specs"] = specDist
		classDist = append(classDist, v)
	}
	sort.Slice(classDist, func(i, j int) bool {
		return classDist[i]["className"].(string) < classDist[j]["className"].(string)
	})
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
	trends, e := a.Stores.Characters.TrendsByGuild(r.Context(), g.ID, a.now().AddDate(0, 0, -30), a.now(), a.now().Add(-7*24*time.Hour), 31)
	if e != nil {
		fail(w, 500, "trend lookup failed")
		return
	}
	jsonOut(w, 200, map[string]any{
		"rosterSize":        len(chars),
		"maxLevelMembers":   max,
		"activeMembers":     active,
		"classDistribution": classDist,
		"mythicPlus": map[string]any{
			"top":           top,
			"averageRating": average,
			"ratedCount":    rated,
		},
		"raidProgression": raidOut,
		"staleCharacters": map[string]any{
			"count":      stale,
			"characters": staleList,
		},
		"lastSync":       last,
		"notableChanges": notable,
		"trends":         trends,
	})
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

// syncProgress serves exactly the service's live snapshot — never a stored
// history row — so the UI can poll the run that is actually in flight. The
// zero snapshot renders as an idle payload with the same shape.
func (a API) syncProgress(w http.ResponseWriter, r *http.Request) {
	var snap domain.Progress
	g, _ := a.guild(r.Context())
	progress := a.Progress
	if a.ScopedProgress != nil {
		progress = a.ScopedProgress(g)
	}
	if progress != nil {
		snap = progress.Progress()
	}
	jsonOut(w, 200, snap)
}
func (a API) triggerSync(w http.ResponseWriter, r *http.Request) {
	g, _ := a.guild(r.Context())
	trigger := a.Trigger
	if a.ScopedTrigger != nil {
		id, e := a.ScopedTrigger(r.Context(), g)
		if e != nil {
			if strings.Contains(e.Error(), "already running") {
				fail(w, 409, "sync already running")
			} else {
				fail(w, 500, "could not trigger sync")
			}
			return
		}
		jsonOut(w, 202, map[string]any{"runId": id})
		if !a.audited(w, r, "sync_trigger") {
			return
		}
		return
	}
	if trigger == nil || (a.GuildSlug != "" && g.Slug != a.GuildSlug) {
		fail(w, 503, "sync unavailable")
		return
	}
	id, e := trigger.Trigger(r.Context())
	if e != nil {
		if strings.Contains(e.Error(), "already running") {
			fail(w, 409, "sync already running")
			return
		}
		fail(w, 500, "could not trigger sync")
		return
	}
	jsonOut(w, 202, map[string]any{"runId": id})
	if !a.audited(w, r, "sync_trigger") {
		return
	}
}
func (a API) dryRun(w http.ResponseWriter, r *http.Request) {
	g, _ := a.guild(r.Context())
	operations := a.Operations
	if a.ScopedOperations != nil {
		operations = a.ScopedOperations(g)
	}
	if operations == nil || (a.ScopedOperations == nil && a.GuildSlug != "" && g.Slug != a.GuildSlug) {
		fail(w, 503, "sync unavailable")
		return
	}
	run, e := operations.StartDryRun(r.Context())
	a.operationResult(w, run, e, r, "sync_dry_run")
}
func (a API) retrySync(w http.ResponseWriter, r *http.Request) {
	g, _ := a.guild(r.Context())
	operations := a.Operations
	if a.ScopedOperations != nil {
		operations = a.ScopedOperations(g)
	}
	if operations == nil || (a.ScopedOperations == nil && a.GuildSlug != "" && g.Slug != a.GuildSlug) {
		fail(w, 503, "sync unavailable")
		return
	}
	id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if e != nil || id <= 0 {
		fail(w, 400, "invalid sync run id")
		return
	}
	run, e := operations.StartRetry(r.Context(), id)
	a.operationResult(w, run, e, r, "sync_retry")
}
func (a API) cancelSync(w http.ResponseWriter, r *http.Request) {
	g, _ := a.guild(r.Context())
	operations := a.Operations
	if a.ScopedOperations != nil {
		operations = a.ScopedOperations(g)
	}
	if operations == nil || (a.ScopedOperations == nil && a.GuildSlug != "" && g.Slug != a.GuildSlug) {
		fail(w, 503, "sync unavailable")
		return
	}
	if e := operations.Cancel(); e != nil {
		fail(w, 409, "no sync is running")
		return
	}
	if !a.audited(w, r, "sync_cancel") {
		return
	}
	jsonOut(w, 202, map[string]bool{"cancelled": true})
}
func (a API) operationResult(w http.ResponseWriter, run store.SyncRun, e error, r *http.Request, event string) {
	if e != nil {
		if strings.Contains(e.Error(), "already running") {
			fail(w, 409, "sync already running")
		} else if strings.Contains(e.Error(), "retry unavailable") {
			fail(w, 409, "retry unavailable")
		} else {
			fail(w, 500, "could not trigger sync")
		}
		return
	}
	if !a.audited(w, r, event) {
		return
	}
	jsonOut(w, 202, map[string]any{"runId": run.ID})
}

// Officer workflow limits are deliberately small: a note is capped at 2,000
// characters, tags are normalized lower-case labels matching
// [a-z0-9][a-z0-9_-]{0,31} (32 characters, 20 per
// character), and every transactional bulk request is capped at 100 IDs.
func decodeOfficerRequest(r *http.Request, v any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	return decoder.Decode(v)
}
func (a API) officerQueue(w http.ResponseWriter, r *http.Request) {
	g, err := a.guild(r.Context())
	if err != nil {
		fail(w, 500, "guild lookup failed")
		return
	}
	items, err := a.Stores.Officer.Queue(r.Context(), g.ID, r.URL.Query().Get("reason"), a.now())
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	jsonOut(w, 200, map[string]any{"items": items})
}
func officerIDs(ids []int64) bool {
	if len(ids) == 0 || len(ids) > domain.OfficerBatchMax {
		return false
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}
func (a API) officerBulkTags(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CharacterIDs []int64  `json:"characterIds"`
		AddTags      []string `json:"addTags"`
		RemoveTags   []string `json:"removeTags"`
		Confirm      bool     `json:"confirm"`
	}
	if err := decodeOfficerRequest(r, &in); err != nil || !in.Confirm || !officerIDs(in.CharacterIDs) {
		fail(w, 400, "invalid confirmed bulk tag request")
		return
	}
	g, e := a.guild(r.Context())
	if e != nil {
		fail(w, 500, "guild lookup failed")
		return
	}
	u := r.Context().Value(contextKey{}).(domain.User)
	if e = a.Stores.Officer.BulkTags(r.Context(), g.ID, u.ID, in.CharacterIDs, in.AddTags, in.RemoveTags); e != nil {
		officerBulkError(w, e)
		return
	}
	if !a.audited(w, r, "officer_bulk_tags") {
		return
	}
	jsonOut(w, 200, map[string]any{"updated": len(in.CharacterIDs)})
}
func (a API) officerComplete(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CharacterIDs []int64 `json:"characterIds"`
		SyncRunIDs   []int64 `json:"syncRunIds"`
		Confirm      bool    `json:"confirm"`
	}
	if e := decodeOfficerRequest(r, &in); e != nil || !in.Confirm || !officerCompletionIDs(in.CharacterIDs, in.SyncRunIDs) {
		fail(w, 400, "invalid confirmed review request")
		return
	}
	g, e := a.guild(r.Context())
	if e != nil {
		fail(w, 500, "guild lookup failed")
		return
	}
	u := r.Context().Value(contextKey{}).(domain.User)
	if e = a.Stores.Officer.Complete(r.Context(), g.ID, u.ID, in.CharacterIDs, in.SyncRunIDs); e != nil {
		officerBulkError(w, e)
		return
	}
	if !a.audited(w, r, "officer_complete") {
		return
	}
	jsonOut(w, 200, map[string]any{"completed": len(in.CharacterIDs) + len(in.SyncRunIDs)})
}
func officerCompletionIDs(characterIDs, syncRunIDs []int64) bool {
	return len(characterIDs)+len(syncRunIDs) <= domain.OfficerBatchMax && (len(characterIDs) > 0 || len(syncRunIDs) > 0) && (len(characterIDs) == 0 || officerIDs(characterIDs)) && (len(syncRunIDs) == 0 || officerIDs(syncRunIDs))
}
func officerBulkError(w http.ResponseWriter, err error) {
	var validation *domain.BulkValidationError
	if errors.As(err, &validation) {
		jsonOut(w, http.StatusBadRequest, map[string]any{"error": validation.Error(), "details": validation})
		return
	}
	// Upstream/database errors can contain request payloads or URLs; never
	// reflect those implementation details into an API response.
	fail(w, http.StatusBadRequest, logger.Redact("officer request rejected"))
}
func (a API) officerActivity(w http.ResponseWriter, r *http.Request) {
	g, e := a.guild(r.Context())
	if e != nil {
		fail(w, 500, "guild lookup failed")
		return
	}
	limit := 50
	if v, ok := integer(r.URL.Query().Get("limit")); ok && v > 0 && v <= 100 {
		limit = v
	}
	rows, e := a.Stores.Officer.Activity(r.Context(), g.ID, limit)
	if e != nil {
		fail(w, 500, "activity lookup failed")
		return
	}
	jsonOut(w, 200, rows)
}
func (a API) officerCharacter(w http.ResponseWriter, r *http.Request) {
	id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if e != nil || id <= 0 {
		fail(w, 400, "invalid character id")
		return
	}
	var in struct {
		Note            string   `json:"note"`
		LifecycleStatus string   `json:"lifecycleStatus"`
		Tags            []string `json:"tags"`
	}
	if e = decodeOfficerRequest(r, &in); e != nil {
		fail(w, 400, "invalid officer character request")
		return
	}
	g, e := a.guild(r.Context())
	if e != nil {
		fail(w, 500, "guild lookup failed")
		return
	}
	u := r.Context().Value(contextKey{}).(domain.User)
	if e = a.Stores.Officer.Update(r.Context(), g.ID, u.ID, id, in.Note, in.LifecycleStatus, in.Tags); e != nil {
		fail(w, 400, "officer update rejected")
		return
	}
	if !a.audited(w, r, "officer_character_update") {
		return
	}
	jsonOut(w, 200, map[string]bool{"updated": true})
}

func (a API) officerCharacterMetadata(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, 400, "invalid character id")
		return
	}
	g, err := a.guild(r.Context())
	if err != nil {
		fail(w, 500, "guild lookup failed")
		return
	}
	character, err := a.Stores.Officer.Character(r.Context(), g.ID, id)
	if err != nil {
		fail(w, 404, "character not found")
		return
	}
	jsonOut(w, 200, character)
}
