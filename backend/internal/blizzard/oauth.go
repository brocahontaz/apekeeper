package blizzard

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/brocahontaz/apekeeper/backend/internal/blizzard/dto"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type tokenCache struct {
	sync.Mutex
	value   string
	expires time.Time
}

func (c *Client) AuthorizationURL(redirect, state string) string {
	u := url.URL{Scheme: "https", Host: "oauth.battle.net", Path: "/authorize"}
	q := u.Query()
	q.Set("client_id", c.ClientID)
	q.Set("redirect_uri", redirect)
	q.Set("response_type", "code")
	q.Set("scope", "wow.profile openid")
	q.Set("state", state)
	u.RawQuery = q.Encode()
	return u.String()
}
func (c *Client) appToken(ctx context.Context) (string, error) {
	c.token.Lock()
	defer c.token.Unlock()
	if c.token.value != "" && time.Until(c.token.expires) > 60*time.Second {
		return c.token.value, nil
	}
	var x dto.Token
	e := c.request(ctx, "POST", c.OAuthBase+"/token", "", url.Values{"grant_type": {"client_credentials"}, "client_id": {c.ClientID}, "client_secret": {c.ClientSecret}}, &x)
	if e != nil {
		return "", e
	}
	c.token.value = x.AccessToken
	c.token.expires = time.Now().Add(time.Duration(x.ExpiresIn) * time.Second)
	return x.AccessToken, nil
}
func (c *Client) ExchangeCode(ctx context.Context, code string) (dto.Token, error) {
	var x dto.Token
	v := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {c.ClientID}, "client_secret": {c.ClientSecret}}
	if c.Redirect != "" {
		v.Set("redirect_uri", c.Redirect)
	}
	return x, c.request(ctx, "POST", c.OAuthBase+"/token", "", v, &x)
}
func (c *Client) UserProfile(ctx context.Context, accessToken string) (dto.UserProfile, error) {
	var x dto.UserProfile
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.OAuthBase+"/userinfo", nil)
	if err != nil {
		return x, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return x, err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return x, fmt.Errorf("Blizzard HTTP %d", res.StatusCode)
	}
	return x, json.NewDecoder(res.Body).Decode(&x)
}
