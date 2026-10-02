package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/auth"
	"github.com/brocahontaz/apekeeper/backend/internal/blizzard/dto"
	"github.com/brocahontaz/apekeeper/backend/internal/store"
)

type securityOAuthFake struct{}

func (securityOAuthFake) AuthorizationURL(_, _ string) string { return "" }
func (securityOAuthFake) ExchangeCode(context.Context, string) (dto.Token, error) {
	return dto.Token{}, nil
}
func (securityOAuthFake) UserProfile(context.Context, string) (dto.UserProfile, error) {
	return dto.UserProfile{}, nil
}

func TestAPIStateChangingRequestsRequireCSRFBeforeAuthorization(t *testing.T) {
	h := New(API{Auth: auth.New(securityOAuthFake{}, store.UserStore{}, "", []byte("test")), CSRFEnabled: true})
	get := httptest.NewRecorder()
	h.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/healthz", nil))
	csrf := get.Result().Cookies()[0]

	post := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	post.AddCookie(csrf)
	withoutHeader := httptest.NewRecorder()
	h.ServeHTTP(withoutHeader, post)
	if withoutHeader.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF token status=%d, want 403", withoutHeader.Code)
	}

	post = httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	post.AddCookie(csrf)
	post.Header.Set("X-CSRF-Token", csrf.Value)
	withHeader := httptest.NewRecorder()
	h.ServeHTTP(withHeader, post)
	if withHeader.Code != http.StatusUnauthorized {
		t.Fatalf("authorized CSRF request status=%d, want auth failure 401", withHeader.Code)
	}
}

func TestCSRFRequiresDoubleSubmitAndAllowsMatchingToken(t *testing.T) {
	h := csrfCookie(csrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })))
	get := httptest.NewRecorder()
	h.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/", nil))
	cookie := get.Result().Cookies()[0]
	bad := httptest.NewRequest(http.MethodPost, "/", nil)
	bad.AddCookie(cookie)
	out := httptest.NewRecorder()
	h.ServeHTTP(out, bad)
	if out.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403", out.Code)
	}
	good := httptest.NewRequest(http.MethodPost, "/", nil)
	good.AddCookie(cookie)
	good.Header.Set("X-CSRF-Token", cookie.Value)
	out = httptest.NewRecorder()
	h.ServeHTTP(out, good)
	if out.Code != http.StatusNoContent {
		t.Fatalf("status=%d, want 204", out.Code)
	}
}

func TestRateLimiterBoundary(t *testing.T) {
	l := newRateLimiter(2, time.Minute)
	now := time.Now()
	if !l.allow("client", now) || !l.allow("client", now.Add(time.Second)) || l.allow("client", now.Add(2*time.Second)) {
		t.Fatal("rate limit boundary incorrect")
	}
	if !l.allow("client", now.Add(time.Minute+time.Second)) {
		t.Fatal("expired window was not released")
	}
}

func TestSecurityLimitsAreIndependentByCategory(t *testing.T) {
	limits := SecurityLimits{Auth: 1, Sync: 1, Export: 1, Mutation: 1, Window: time.Minute}
	h := limitRequestsByCategory(limits, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	for _, item := range []struct{ method, path string }{{http.MethodGet, "/api/auth/login"}, {http.MethodPost, "/api/sync/run"}, {http.MethodGet, "/api/roster/export"}, {http.MethodPost, "/api/guilds/select"}} {
		r := httptest.NewRequest(item.method, item.path, nil)
		r.RemoteAddr = "same-client:1234"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusNoContent {
			t.Fatalf("first %s status=%d", item.path, w.Code)
		}
	}
	// A second authentication request is limited, while the other buckets were
	// not consumed by it.
	r := httptest.NewRequest(http.MethodGet, "/api/auth/login", nil)
	r.RemoteAddr = "same-client:1234"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("auth status=%d", w.Code)
	}
}

func TestReadinessIsNotAnAliasForHealth(t *testing.T) {
	h := New(API{ReadinessChecks: map[string]func(context.Context) error{"scheduler": func(context.Context) error { return errors.New("stopped") }}})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "scheduler") {
		t.Fatalf("readiness=%d body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("health=%d", w.Code)
	}
}
