package blizzard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	c := NewClient("us", "en_US", "", "")
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

func TestTokenCacheReusesValidToken(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = json.NewEncoder(w).Encode(dto.Token{AccessToken: "token", ExpiresIn: 120})
	}))
	defer s.Close()
	c := NewClient("us", "en_US", "id", "secret")
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
