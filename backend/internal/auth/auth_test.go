package auth

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/blizzard/dto"
	"github.com/brocahontaz/apekeeper/backend/internal/store"
)

type oauthFake struct{}

func (oauthFake) AuthorizationURL(_, state string) string {
	return "https://oauth.battle.net/authorize?state=" + url.QueryEscape(state)
}
func (oauthFake) ExchangeCode(context.Context, string) (dto.Token, error) { return dto.Token{}, nil }
func (oauthFake) UserProfile(context.Context, string) (dto.UserProfile, error) {
	return dto.UserProfile{}, nil
}

type profileErrFake struct{ oauthFake }

func (profileErrFake) UserProfile(context.Context, string) (dto.UserProfile, error) {
	return dto.UserProfile{}, errors.New("userinfo unavailable")
}
func TestStateIsSingleUseAndExpires(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := New(oauthFake{}, store.UserStore{}, "http://callback", []byte("test"))
	m.Now = func() time.Time { return now }
	u, _ := url.Parse(m.AuthorizationURL())
	state := u.Query().Get("state")
	if !m.consumeState(state) || m.consumeState(state) {
		t.Fatal("state should be valid once")
	}
	state = "expired"
	m.states[state] = now.Add(-time.Second)
	if m.consumeState(state) {
		t.Fatal("expired state accepted")
	}
}
func TestCookieSignatureRoundTrip(t *testing.T) {
	m := New(oauthFake{}, store.UserStore{}, "", []byte("test"))
	r := httptest.NewRequest("GET", "http://example.test", nil)
	c := m.cookie(r, "session", time.Now().Add(time.Hour))
	r.AddCookie(c)
	id, err := m.sessionID(r)
	if err != nil || id != "session" {
		t.Fatalf("id=%q err=%v", id, err)
	}
}

func TestCallbackLogsProfileFetchFailure(t *testing.T) {
	var buf bytes.Buffer
	m := New(profileErrFake{}, store.UserStore{}, "", []byte("test"))
	m.Log = slog.New(slog.NewJSONHandler(&buf, nil))
	m.states["s"] = time.Now().Add(time.Minute)
	r := httptest.NewRequest(http.MethodGet, "http://example.test/callback?code=c&state=s", nil)
	w := httptest.NewRecorder()
	m.Callback(w, r)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status=%d, want 502", w.Code)
	}
	if !strings.Contains(buf.String(), `"level":"WARN"`) ||
		!strings.Contains(buf.String(), "OAuth profile fetch failed") {
		t.Errorf("missing profile fetch failure log: %q", buf.String())
	}
}

func TestIsSuperAdminMatchesConfiguredBattleTags(t *testing.T) {
	tags := []string{"Brocahontaz#2576"}
	if !isSuperAdmin(tags, "Brocahontaz#2576") {
		t.Error("exact BattleTag should match")
	}
	if !isSuperAdmin(tags, "brocahontaz#2576") {
		t.Error("BattleTag matching should be case-insensitive")
	}
	if isSuperAdmin(tags, "Brocahontaz#2577") {
		t.Error("different BattleTag number must not match")
	}
	if isSuperAdmin(tags, "Brocahontaz#257") {
		t.Error("partial BattleTag number must not match")
	}
	if isSuperAdmin(nil, "Brocahontaz#2576") {
		t.Error("empty tag list should never match")
	}
}
