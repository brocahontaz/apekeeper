package auth

import (
	"context"
	"net/http/httptest"
	"net/url"
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
