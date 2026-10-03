package httpapi

import "net/http"

// Route is the single source of truth for registrations made by New. The
// manifest metadata is also consumed by the route-drift test.
type Route struct {
	Pattern  string
	Roles    []string
	Register func(*http.ServeMux, API)
}

func RouteManifest() []Route {
	public := func(pattern string, handler func(API) http.HandlerFunc) Route {
		return Route{Pattern: pattern, Register: func(m *http.ServeMux, a API) { m.HandleFunc(pattern, handler(a)) }}
	}
	authenticated := func(pattern string, handler func(API) http.HandlerFunc) Route {
		return Route{Pattern: pattern, Roles: []string{"authenticated"}, Register: func(m *http.ServeMux, a API) { m.Handle(pattern, a.requireAuth(http.HandlerFunc(handler(a)))) }}
	}
	role := func(pattern string, roles []string, handler func(API) http.HandlerFunc) Route {
		return Route{Pattern: pattern, Roles: roles, Register: func(m *http.ServeMux, a API) { m.Handle(pattern, a.requireRole(roles, http.HandlerFunc(handler(a)))) }}
	}
	member := []string{"member"}
	officer := []string{"superadmin", "admin", "officer"}
	admin := []string{"superadmin", "admin"}
	manager := []string{"superadmin", "owner", "admin"}
	return []Route{
		public("GET /api/auth/login", func(a API) http.HandlerFunc { return a.Auth.Login }),
		public("GET /api/auth/callback", func(a API) http.HandlerFunc { return a.Auth.Callback }),
		authenticated("POST /api/auth/logout", func(a API) http.HandlerFunc { return a.Auth.Logout }),
		authenticated("GET /api/auth/me", func(a API) http.HandlerFunc { return a.me }),
		authenticated("GET /api/guilds", func(a API) http.HandlerFunc { return a.guilds }),
		authenticated("POST /api/guilds/select", func(a API) http.HandlerFunc { return a.selectGuild }),
		public("GET /healthz", func(a API) http.HandlerFunc { return a.health }),
		public("GET /api/healthz", func(a API) http.HandlerFunc { return a.health }),
		public("GET /readyz", func(a API) http.HandlerFunc { return a.readiness }),
		public("GET /api/readyz", func(a API) http.HandlerFunc { return a.readiness }),
		role("GET /api/dashboard", member, func(a API) http.HandlerFunc { return a.dashboard }),
		role("GET /api/roster", member, func(a API) http.HandlerFunc { return a.roster }),
		role("GET /api/roster/export", member, func(a API) http.HandlerFunc { return a.rosterExport }),
		role("GET /api/characters/{id}", member, func(a API) http.HandlerFunc { return a.character }),
		role("GET /api/characters/{id}/history", member, func(a API) http.HandlerFunc { return a.characterHistory }),
		role("GET /api/sync/runs", officer, func(a API) http.HandlerFunc { return a.syncRuns }),
		role("GET /api/sync/progress", officer, func(a API) http.HandlerFunc { return a.syncProgress }),
		role("GET /api/diagnostics/sync-metrics", admin, func(a API) http.HandlerFunc { return a.syncMetrics }),
		role("POST /api/sync/run", admin, func(a API) http.HandlerFunc { return a.triggerSync }),
		role("POST /api/sync/dry-run", admin, func(a API) http.HandlerFunc { return a.dryRun }),
		role("POST /api/sync/runs/{id}/retry", admin, func(a API) http.HandlerFunc { return a.retrySync }),
		role("POST /api/sync/cancel", admin, func(a API) http.HandlerFunc { return a.cancelSync }),
		role("GET /api/officer/queue", officer, func(a API) http.HandlerFunc { return a.officerQueue }),
		role("POST /api/officer/queue/complete", officer, func(a API) http.HandlerFunc { return a.officerComplete }),
		role("POST /api/officer/bulk-tags", officer, func(a API) http.HandlerFunc { return a.officerBulkTags }),
		role("GET /api/officer/characters/{id}", officer, func(a API) http.HandlerFunc { return a.officerCharacterMetadata }),
		role("PUT /api/officer/characters/{id}", officer, func(a API) http.HandlerFunc { return a.officerCharacter }),
		role("GET /api/officer/activity", officer, func(a API) http.HandlerFunc { return a.officerActivity }),
		role("GET /api/guild/members", manager, func(a API) http.HandlerFunc { return a.members }),
		role("GET /api/guild/members/search", manager, func(a API) http.HandlerFunc { return a.memberSearch }),
		role("POST /api/guild/members", manager, func(a API) http.HandlerFunc { return a.inviteMember }),
		role("PUT /api/guild/members/{id}", manager, func(a API) http.HandlerFunc { return a.updateMember }),
		role("DELETE /api/guild/members/{id}", manager, func(a API) http.HandlerFunc { return a.removeMember }),
	}
}
