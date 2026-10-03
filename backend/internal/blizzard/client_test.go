package blizzard

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/blizzard/dto"
)

type trackingBody struct {
	io.Reader
	closed bool
}

func (b *trackingBody) Close() error {
	b.closed = true
	return nil
}

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

func TestRequestClosesRetryBodiesPromptly(t *testing.T) {
	firstBody := &trackingBody{Reader: strings.NewReader("busy")}
	attempts := 0
	c := NewClient("us", "en_US", "", "", "")
	defer c.limiter.Close()
	c.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return &http.Response{StatusCode: http.StatusTooManyRequests, Header: make(http.Header), Body: firstBody}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":"yes"}`))}, nil
	})}
	var out struct {
		OK string `json:"ok"`
	}
	if err := c.request(context.Background(), http.MethodGet, "http://blizzard.test", "", nil, &out); err != nil {
		t.Fatal(err)
	}
	if !firstBody.closed {
		t.Fatal("retry response body was not closed before the next attempt")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRequestBackoffHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hit := make(chan struct{})
	c := NewClient("us", "en_US", "", "", "")
	defer c.limiter.Close()
	c.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		close(hit)
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("down"))}, nil
	})}
	done := make(chan error, 1)
	go func() { done <- c.request(ctx, http.MethodGet, "http://blizzard.test", "", nil, &struct{}{}) }()
	<-hit
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("error=%v, want context canceled", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("request did not stop during backoff")
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
	fixture := `{"name":"Thrall","realm":{"name":"Area 52","slug":"area-52"},"level":70,"character_class":{"id":7,"name":"Shaman"},"active_spec":{"id":262,"name":"Elemental"},"race":{"key":{"href":"https://us.api.blizzard.com/data/wow/playable-race/8"},"name":"Tauren","id":8},"gender":{"type":"MALE","name":"Male"},"equipped_item_level":621,"average_item_level":619}`
	if err := json.Unmarshal([]byte(fixture), &p); err != nil {
		t.Fatal(err)
	}
	if p.Name != "Thrall" || p.Realm.Slug != "area-52" || p.CharacterClass.ID != 7 || p.ActiveSpec.Name != "Elemental" {
		t.Fatalf("unexpected DTO: %#v", p)
	}
	if p.EquippedItemLevel != 621 || p.AverageItemLevel != 619 {
		t.Fatalf("item levels = %v/%v, want 621/619", p.EquippedItemLevel, p.AverageItemLevel)
	}
	// Live profiles now carry localized names as plain strings.
	if p.Race.Name != "Tauren" || p.Gender.Name != "Male" {
		t.Fatalf("race/gender=%q/%q, want Tauren/Male", p.Race.Name, p.Gender.Name)
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

func TestCharacterProgressionUsesProfileNamespace(t *testing.T) {
	gotNamespaces := map[string]string{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_ = json.NewEncoder(w).Encode(dto.Token{AccessToken: "test-token", ExpiresIn: 120})
			return
		}
		gotNamespaces[r.URL.Path] = r.URL.Query().Get("namespace")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer s.Close()
	c := NewClient("eu", "en_GB", "", "", "")
	defer c.limiter.Close()
	c.APIBase = s.URL
	c.OAuthBase = s.URL

	if _, err := c.CharacterMythicPlusSeasonal(context.Background(), "Tarren Mill", "Ape Enclosure", "current"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CharacterMythicPlusProfile(context.Background(), "Tarren Mill", "Ape Enclosure"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CharacterRaids(context.Background(), "Tarren Mill", "Ape Enclosure"); err != nil {
		t.Fatal(err)
	}
	for path, namespace := range gotNamespaces {
		if namespace != "profile-eu" {
			t.Errorf("namespace for %s = %q, want profile-eu", path, namespace)
		}
	}
	if len(gotNamespaces) != 3 {
		t.Fatalf("progression requests=%d, want 3", len(gotNamespaces))
	}
}

func TestDataEndpointsUseGameNamespaces(t *testing.T) {
	gotNamespaces := map[string]string{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_ = json.NewEncoder(w).Encode(dto.Token{AccessToken: "test-token", ExpiresIn: 120})
			return
		}
		gotNamespaces[r.URL.Path] = r.URL.Query().Get("namespace")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer s.Close()
	c := NewClient("eu", "en_GB", "", "", "")
	defer c.limiter.Close()
	c.APIBase = s.URL
	c.OAuthBase = s.URL

	if _, err := c.MythicKeystoneSeasonIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.JournalExpansionIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if namespace := gotNamespaces["/data/wow/mythic-keystone/season/index"]; namespace != "dynamic-eu" {
		t.Fatalf("season index namespace=%q, want dynamic-eu", namespace)
	}
	if namespace := gotNamespaces["/data/wow/journal-expansion/index"]; namespace != "static-eu" {
		t.Fatalf("journal expansion namespace=%q, want static-eu", namespace)
	}
}

func TestCharacterMediaUsesProfileNamespace(t *testing.T) {
	var gotNamespace string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_ = json.NewEncoder(w).Encode(dto.Token{AccessToken: "test-token", ExpiresIn: 120})
			return
		}
		gotNamespace = r.URL.Query().Get("namespace")
		_, _ = w.Write([]byte(`{"assets":[{"key":"avatar","value":"https://render.worldofwarcraft.com/avatar.jpg"},{"key":"main","value":"https://render.worldofwarcraft.com/main.jpg"}]}`))
	}))
	defer s.Close()
	c := NewClient("eu", "en_GB", "", "", "")
	defer c.limiter.Close()
	c.APIBase = s.URL
	c.OAuthBase = s.URL

	m, err := c.CharacterMedia(context.Background(), "Tarren Mill", "Ape Enclosure")
	if err != nil {
		t.Fatal(err)
	}
	if gotNamespace != "profile-eu" {
		t.Fatalf("namespace=%q, want profile-eu", gotNamespace)
	}
	if len(m.Assets) != 2 {
		t.Fatalf("assets=%d, want 2", len(m.Assets))
	}
	var avatar string
	for _, a := range m.Assets {
		if a.Key == "avatar" {
			avatar = a.Value
		}
	}
	if avatar != "https://render.worldofwarcraft.com/avatar.jpg" {
		t.Fatalf("avatar=%q", avatar)
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
