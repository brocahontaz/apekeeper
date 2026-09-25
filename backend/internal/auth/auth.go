package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/blizzard/dto"
	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/brocahontaz/apekeeper/backend/internal/store"
)

const CookieName = "apekeeper_session"

type OAuthClient interface {
	AuthorizationURL(string, string) string
	ExchangeCode(context.Context, string) (dto.Token, error)
	UserProfile(context.Context, string) (dto.UserProfile, error)
}
type Manager struct {
	OAuth       OAuthClient
	Users       store.UserStore
	RedirectURL string
	Secret      []byte
	// SuperAdminBattleTags optionally lists platform-level super admin
	// BattleTags; a matching sign-in is stored with the superadmin role.
	SuperAdminBattleTags []string
	// Log is optional; when nil failures fall back to slog.Default().
	Log    *slog.Logger
	Now    func() time.Time
	mu     sync.Mutex
	states map[string]time.Time
}

func New(o OAuthClient, users store.UserStore, redirect string, secret []byte) *Manager {
	if len(secret) == 0 {
		secret = randomBytes(32)
	}
	return &Manager{
		OAuth:       o,
		Users:       users,
		RedirectURL: redirect,
		Secret:      secret,
		states:      map[string]time.Time{},
	}
}
func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}
func (m *Manager) logger() *slog.Logger {
	if m.Log != nil {
		return m.Log
	}
	return slog.Default()
}
func isSuperAdmin(tags []string, battletag string) bool {
	for _, tag := range tags {
		if strings.EqualFold(tag, battletag) {
			return true
		}
	}
	return false
}
func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}
func token() string {
	return base64.RawURLEncoding.EncodeToString(randomBytes(32))
}
func (m *Manager) AuthorizationURL() string {
	state := token()
	m.mu.Lock()
	m.states[state] = m.now().Add(10 * time.Minute)
	m.mu.Unlock()
	return m.OAuth.AuthorizationURL(m.RedirectURL, state)
}
func (m *Manager) consumeState(state string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	expires, ok := m.states[state]
	delete(m.states, state)
	return ok && m.now().Before(expires)
}
func (m *Manager) Login(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, m.AuthorizationURL(), http.StatusFound)
}
func (m *Manager) Callback(w http.ResponseWriter, r *http.Request) {
	if !m.consumeState(r.URL.Query().Get("state")) {
		http.Error(w, "invalid OAuth state", http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing OAuth code", http.StatusBadRequest)
		return
	}
	t, err := m.OAuth.ExchangeCode(r.Context(), code)
	if err != nil {
		m.logger().Warn("OAuth token exchange failed", "error", err)
		http.Error(w, "OAuth token exchange failed", http.StatusBadGateway)
		return
	}
	p, err := m.OAuth.UserProfile(r.Context(), t.AccessToken)
	if err != nil {
		m.logger().Warn("OAuth profile fetch failed", "error", err)
		http.Error(w, "OAuth profile fetch failed", http.StatusBadGateway)
		return
	}
	u, err := m.Users.UpsertBattleNetUser(
		r.Context(),
		formatID(p.ID),
		p.BattleTag,
		t.AccessToken,
		t.RefreshToken,
		m.now().Add(time.Duration(t.ExpiresIn)*time.Second),
		isSuperAdmin(m.SuperAdminBattleTags, p.BattleTag),
	)
	if err != nil {
		http.Error(w, "could not save user", http.StatusInternalServerError)
		return
	}
	id := token()
	expires := m.now().Add(7 * 24 * time.Hour)
	if err = m.Users.CreateSession(r.Context(), id, u.ID, expires); err != nil {
		http.Error(w, "could not create session", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, m.cookie(r, id, expires))
	http.Redirect(w, r, "/", http.StatusFound)
}
func formatID(id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(stringInt(id)))
}
func stringInt(v int64) string {
	if v == 0 {
		return "0"
	}
	b := make([]byte, 0, 20)
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}
func (m *Manager) sign(id string) string {
	h := hmac.New(sha256.New, m.Secret)
	h.Write([]byte(id))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
func (m *Manager) cookie(r *http.Request, id string, expires time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     CookieName,
		Value:    id + "." + m.sign(id),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
		Expires:  expires,
	}
}
func (m *Manager) sessionID(r *http.Request) (string, error) {
	c, e := r.Cookie(CookieName)
	if e != nil {
		return "", e
	}
	var id, sig string
	for i := 0; i < len(c.Value); i++ {
		if c.Value[i] == '.' {
			id = c.Value[:i]
			sig = c.Value[i+1:]
			break
		}
	}
	if id == "" || !hmac.Equal([]byte(sig), []byte(m.sign(id))) {
		return "", errors.New("invalid session cookie")
	}
	return id, nil
}
func (m *Manager) CurrentUser(ctx context.Context, r *http.Request) (domain.User, error) {
	id, e := m.sessionID(r)
	if e != nil {
		return domain.User{}, e
	}
	return m.Users.SessionUser(ctx, id)
}
func (m *Manager) Logout(w http.ResponseWriter, r *http.Request) {
	if id, e := m.sessionID(r); e == nil {
		_ = m.Users.DeleteSession(r.Context(), id)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	})
	w.WriteHeader(http.StatusNoContent)
}
