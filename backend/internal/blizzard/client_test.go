package blizzard

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/blizzard/dto"
)

func TestLimiterWaitsForRefill(t *testing.T) {
	l := NewLimiter(1, 1, 20*time.Millisecond)
	defer l.Close()
	if err := l.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if err := l.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if time.Since(started) < 15*time.Millisecond {
		t.Fatal("limiter did not wait")
	}
}

func TestRequestRetries429(t *testing.T) {
	attempts := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"ok": "yes"})
	}))
	defer s.Close()
	c := NewClient("us", "en_US", "", "", "")
	defer c.limiter.Close()
	var out struct {
		OK string `json:"ok"`
	}
	if err := c.request(context.Background(), http.MethodGet, s.URL, "", nil, &out); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || out.OK != "yes" {
		t.Fatalf("attempts=%d output=%q", attempts, out.OK)
	}
}

func TestRequestLogsFailedAttemptsAtDebug(t *testing.T) {
	attempts := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"ok": "yes"})
	}))
	defer s.Close()
	var buf bytes.Buffer
	c := NewClient("us", "en_US", "", "", "")
	defer c.limiter.Close()
	c.Log = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	var out struct {
		OK string `json:"ok"`
	}
	// The URL carries a query value that must never reach the log.
	u := s.URL + "/data/wow/guild/x/y/roster?secret=do-not-log"
	if err := c.request(context.Background(), http.MethodGet, u, "", nil, &out); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("attempts=%d", attempts)
	}
	if !strings.Contains(buf.String(), `"level":"DEBUG"`) ||
		!strings.Contains(buf.String(), "blizzard request failed") {
		t.Errorf("missing failed attempt log: %q", buf.String())
	}
	if !strings.Contains(buf.String(), `"path":"/data/wow/guild/x/y/roster"`) {
		t.Errorf("failure log missing URL path only: %q", buf.String())
	}
	if strings.Contains(buf.String(), "do-not-log") {
		t.Errorf("failure log leaked the query string: %q", buf.String())
	}
}

func TestProfileDTOFixture(t *testing.T) {
	var p dto.ProfileSummary
	fixture := `{"name":"Thrall","realm":{"name":"Area 52","slug":"area-52"},"level":70,"character_class":{"id":7,"name":"Shaman"},"active_spec":{"id":262,"name":"Elemental"}}`
	if err := json.Unmarshal([]byte(fixture), &p); err != nil {
		t.Fatal(err)
	}
	if p.Name != "Thrall" || p.Realm.Slug != "area-52" || p.CharacterClass.ID != 7 || p.ActiveSpec.Name != "Elemental" {
		t.Fatalf("unexpected DTO: %#v", p)
	}
}

func TestGuildRosterUsesProfileNamespace(t *testing.T) {
	var gotNamespace string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_ = json.NewEncoder(w).Encode(dto.Token{AccessToken: "test-token", ExpiresIn: 120})
			return
		}
		gotNamespace = r.URL.Query().Get("namespace")
		_ = json.NewEncoder(w).Encode(dto.GuildRoster{})
	}))
	defer s.Close()
	c := NewClient("eu", "en_GB", "", "", "")
	defer c.limiter.Close()
	c.APIBase = s.URL
	c.OAuthBase = s.URL

	if _, err := c.GuildRoster(context.Background(), "Tarren Mill", "Ape Enclosure"); err != nil {
		t.Fatal(err)
	}
	if gotNamespace != "profile-eu" {
		t.Fatalf("namespace=%q, want profile-eu", gotNamespace)
	}
}

func TestTokenCacheReusesValidToken(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = json.NewEncoder(w).Encode(dto.Token{AccessToken: "token", ExpiresIn: 120})
	}))
	defer s.Close()
	c := NewClient("us", "en_US", "id", "secret", "")
	defer c.limiter.Close()
	c.OAuthBase = s.URL
	first, err := c.appToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.appToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first != "token" || second != "token" || calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestExchangeCodeSendsRedirectURI(t *testing.T) {
	var form url.Values
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		form = r.PostForm
		_ = json.NewEncoder(w).Encode(dto.Token{AccessToken: "token", ExpiresIn: 120})
	}))
	defer s.Close()
	c := NewClient("us", "en_US", "id", "secret", "https://example.test/callback")
	defer c.limiter.Close()
	c.OAuthBase = s.URL
	if _, err := c.ExchangeCode(context.Background(), "code"); err != nil {
		t.Fatal(err)
	}
	if form.Get("redirect_uri") != "https://example.test/callback" ||
		form.Get("code") != "code" ||
		form.Get("grant_type") != "authorization_code" {
		t.Fatalf("form=%v", form)
	}
}

func TestExchangeCodeSurfacesErrorBody(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer s.Close()
	c := NewClient("us", "en_US", "id", "secret", "https://example.test/callback")
	defer c.limiter.Close()
	c.OAuthBase = s.URL
	_, err := c.ExchangeCode(context.Background(), "bad")
	if err == nil ||
		!strings.Contains(err.Error(), "Blizzard HTTP 400") ||
		!strings.Contains(err.Error(), "invalid_grant") {
		t.Fatalf("err=%v", err)
	}
}
