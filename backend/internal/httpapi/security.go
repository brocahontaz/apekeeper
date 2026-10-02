package httpapi

import (
	"crypto/rand"
	"encoding/base64"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const csrfCookieName = "apekeeper_csrf"

func csrfToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		c, err := r.Cookie(csrfCookieName)
		token := r.Header.Get("X-CSRF-Token")
		if err != nil || token == "" || token != c.Value {
			fail(w, http.StatusForbidden, "csrf validation failed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func csrfCookie(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie(csrfCookieName); err != nil {
			token := csrfToken()
			if token == "" {
				fail(w, http.StatusServiceUnavailable, "security token unavailable")
				return
			}
			http.SetCookie(w, &http.Cookie{Name: csrfCookieName, Value: token, Path: "/", HttpOnly: false, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil, MaxAge: 12 * 60 * 60})
		}
		next.ServeHTTP(w, r)
	})
}

type rateLimiter struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	limit  int
	window time.Duration
}

type SecurityLimits struct {
	Auth, Sync, Export, Mutation                 int
	AuthBody, SyncBody, ExportBody, MutationBody int64
	Window                                       time.Duration
}

func (l SecurityLimits) category(r *http.Request) (int, int64) {
	p := r.URL.Path
	if strings.HasPrefix(p, "/api/auth/") {
		return l.Auth, l.AuthBody
	}
	if strings.HasPrefix(p, "/api/sync/") {
		return l.Sync, l.SyncBody
	}
	if r.Method == http.MethodGet && strings.HasSuffix(p, "/export") {
		return l.Export, l.ExportBody
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
		return l.Mutation, l.MutationBody
	}
	return 0, 0
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{hits: map[string][]time.Time{}, limit: limit, window: window}
}
func (l *rateLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cut := now.Add(-l.window)
	old := l.hits[key]
	i := 0
	for i < len(old) && !old[i].After(cut) {
		i++
	}
	old = old[i:]
	if len(old) >= l.limit {
		l.hits[key] = old
		return false
	}
	l.hits[key] = append(old, now)
	return true
}
func limitRequests(limit int, window time.Duration, next http.Handler) http.Handler {
	if limit <= 0 {
		return next
	}
	l := newRateLimiter(limit, window)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.RemoteAddr
		if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			key = host
		}
		if !l.allow(key, time.Now()) {
			w.Header().Set("Retry-After", "60")
			fail(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func limitRequestsByCategory(limits SecurityLimits, next http.Handler) http.Handler {
	window := limits.Window
	if window <= 0 {
		window = time.Minute
	}
	limiters := map[string]*rateLimiter{
		"auth": newRateLimiter(limits.Auth, window), "sync": newRateLimiter(limits.Sync, window),
		"export": newRateLimiter(limits.Export, window), "mutation": newRateLimiter(limits.Mutation, window),
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit, _ := limits.category(r)
		if limit <= 0 {
			next.ServeHTTP(w, r)
			return
		}
		key := r.RemoteAddr
		if host, _, err := net.SplitHostPort(key); err == nil {
			key = host
		}
		name := r.URL.Path
		if r.Method == http.MethodGet && strings.HasSuffix(name, "/export") {
			name = "export"
		} else if strings.HasPrefix(name, "/api/auth/") {
			name = "auth"
		} else if strings.HasPrefix(name, "/api/sync/") {
			name = "sync"
		} else {
			name = "mutation"
		}
		// All buckets are created before the handler is published, so this map is
		// immutable and concurrent requests only contend inside their bucket.
		if !limiters[name].allow(key, time.Now()) {
			w.Header().Set("Retry-After", "60")
			fail(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}
