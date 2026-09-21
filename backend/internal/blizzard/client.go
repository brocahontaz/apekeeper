package blizzard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/brocahontaz/apekeeper/backend/internal/blizzard/dto"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type BlizzardClient interface {
	GuildRoster(context.Context, string, string) (dto.GuildRoster, error)
	CharacterProfileSummary(context.Context, string, string) (dto.ProfileSummary, error)
	CharacterEquipment(context.Context, string, string) (dto.Equipment, error)
	CharacterMythicPlusSeasonal(context.Context, string, string, string) (dto.MythicPlus, error)
	CharacterRaids(context.Context, string, string) (dto.Raids, error)
	ExchangeCode(context.Context, string) (dto.Token, error)
	UserProfile(context.Context, string) (dto.UserProfile, error)
	AuthorizationURL(string, string) string
}
type NotFoundError struct{ URL string }

func (e *NotFoundError) Error() string { return "Blizzard resource not found: " + e.URL }

type Client struct {
	Region, Locale, ClientID, ClientSecret string
	HTTP                                   *http.Client
	APIBase, OAuthBase                     string
	limiter                                *Limiter
	token                                  tokenCache
}

func NewClient(region, locale, id, secret string) *Client {
	return &Client{Region: region, Locale: locale, ClientID: id, ClientSecret: secret, HTTP: &http.Client{Timeout: 20 * time.Second}, APIBase: "https://" + region + ".api.blizzard.com", OAuthBase: "https://oauth.battle.net", limiter: NewLimiter(0, 0, time.Second)}
}
func (c *Client) get(ctx context.Context, path, namespace string, out any) error {
	return c.request(ctx, http.MethodGet, c.APIBase+path, namespace, nil, out)
}
func (c *Client) request(ctx context.Context, method, u, namespace string, body url.Values, out any) error {
	for i := 0; i < MaxAttempts; i++ {
		if e := c.limiter.Wait(ctx); e != nil {
			return e
		}
		var payload *strings.Reader
		if body != nil && method != http.MethodGet {
			payload = strings.NewReader(body.Encode())
		} else {
			payload = strings.NewReader("")
		}
		req, e := http.NewRequestWithContext(ctx, method, u, payload)
		if e != nil {
			return e
		}
		if body != nil && method == http.MethodGet {
			req.URL.RawQuery = body.Encode()
		}
		if body != nil && method != http.MethodGet {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		if namespace != "" {
			q := req.URL.Query()
			q.Set("namespace", namespace)
			q.Set("locale", c.Locale)
			req.URL.RawQuery = q.Encode()
			t, e := c.appToken(ctx)
			if e != nil {
				return e
			}
			req.Header.Set("Authorization", "Bearer "+t)
		}
		res, e := c.HTTP.Do(req)
		if e != nil {
			if i == MaxAttempts-1 {
				return e
			}
			time.Sleep(Backoff(i))
			continue
		}
		defer res.Body.Close()
		if res.StatusCode == 404 {
			return &NotFoundError{u}
		}
		if res.StatusCode == 429 || res.StatusCode >= 500 {
			d := RetryAfter(res)
			if d == 0 {
				d = Backoff(i)
			}
			if i == MaxAttempts-1 {
				return fmt.Errorf("Blizzard HTTP %d", res.StatusCode)
			}
			time.Sleep(d)
			continue
		}
		if res.StatusCode/100 != 2 {
			return fmt.Errorf("Blizzard HTTP %d", res.StatusCode)
		}
		return json.NewDecoder(res.Body).Decode(out)
	}
	return errors.New("Blizzard retries exhausted")
}
func (c *Client) GuildRoster(ctx context.Context, realm, guild string) (dto.GuildRoster, error) {
	var x dto.GuildRoster
	return x, c.get(ctx, "/data/wow/guild/"+RealmSlug(realm)+"/"+Slug(guild)+"/roster", DynamicNamespace(c.Region), &x)
}
func (c *Client) CharacterProfileSummary(ctx context.Context, realm, name string) (dto.ProfileSummary, error) {
	var x dto.ProfileSummary
	return x, c.get(ctx, "/profile/wow/character/"+RealmSlug(realm)+"/"+Slug(name), ProfileNamespace(c.Region), &x)
}
func (c *Client) CharacterEquipment(ctx context.Context, realm, name string) (dto.Equipment, error) {
	var x dto.Equipment
	return x, c.get(ctx, "/profile/wow/character/"+RealmSlug(realm)+"/"+Slug(name)+"/equipment", ProfileNamespace(c.Region), &x)
}
func (c *Client) CharacterMythicPlusSeasonal(ctx context.Context, realm, name, season string) (dto.MythicPlus, error) {
	var x dto.MythicPlus
	return x, c.get(ctx, "/profile/wow/character/"+RealmSlug(realm)+"/"+Slug(name)+"/mythic-keystone-profile/season/"+season, DynamicNamespace(c.Region), &x)
}
func (c *Client) CharacterRaids(ctx context.Context, realm, name string) (dto.Raids, error) {
	var x dto.Raids
	return x, c.get(ctx, "/profile/wow/character/"+RealmSlug(realm)+"/"+Slug(name)+"/encounters/raids", DynamicNamespace(c.Region), &x)
}
func Slug(s string) string      { return strings.ReplaceAll(strings.ToLower(s), " ", "-") }
func RealmSlug(s string) string { return Slug(s) }
