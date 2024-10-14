// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/cachingtest"
)

const testScope = "https://example.com/scopes/user.email"

type tokenInfo struct {
	Audience      string `json:"aud"`
	Email         string `json:"email"`
	EmailVerified string `json:"email_verified"`
	Error         string `json:"error_description"`
	ExpiresIn     string `json:"expires_in"`
	Scope         string `json:"scope"`
}

func TestGoogleOAuth2Method(t *testing.T) {
	t.Parallel()

	ftt.Run("with mock backend", t, func(c *ftt.Test) {
		ctx := caching.WithEmptyProcessCache(context.Background())
		ctx = cachingtest.WithGlobalCache(ctx, map[string]caching.BlobCache{
			oauthValidationCache.Parameters().GlobalNamespace: cachingtest.NewBlobCache(),
		})
		ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

		tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
			if testclock.HasTags(t, "oauth-tokeninfo-retry") {
				tc.Add(d)
			}
		})

		goodUser := &User{
			Identity: "user:abc@example.com",
			Email:    "abc@example.com",
			ClientID: "client_id",
		}

		checks := 0
		info := tokenInfo{
			Audience:      "client_id",
			Email:         "abc@example.com",
			EmailVerified: "true",
			ExpiresIn:     "3600",
			Scope:         testScope + " other stuff",
		}
		status := http.StatusOK
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			checks++
			w.WriteHeader(status)
			assert.Loosely(c, json.NewEncoder(w).Encode(&info), should.BeNil)
		}))
		defer ts.Close()

		ctx = ModifyConfig(ctx, func(cfg Config) Config {
			cfg.AnonymousTransport = func(context.Context) http.RoundTripper {
				return http.DefaultTransport
			}
			return cfg
		})

		call := func(header string) (*User, error) {
			m := GoogleOAuth2Method{
				Scopes:            []string{testScope},
				tokenInfoEndpoint: ts.URL,
			}
			req := makeRequest()
			req.FakeHeader.Set("Authorization", header)
			u, _, err := m.Authenticate(ctx, req)
			return u, err
		}

		c.Run("Works with end users", func(c *ftt.Test) {
			u, err := call("Bearer access_token")
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, u, should.Resemble(goodUser))
		})

		c.Run("Works with service accounts", func(c *ftt.Test) {
			info.Email = "something@example.gserviceaccount.com"
			info.Audience = "ignored"
			u, err := call("Bearer access_token")
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, u, should.Resemble(&User{
				Identity: "user:something@example.gserviceaccount.com",
				Email:    "something@example.gserviceaccount.com",
			}))
		})

		c.Run("Valid tokens are cached", func(c *ftt.Test) {
			u, err := call("Bearer access_token")
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, u, should.Resemble(goodUser))
			assert.Loosely(c, checks, should.Equal(1))

			// Hit the process cache.
			u, err = call("Bearer access_token")
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, u, should.Resemble(goodUser))

			// Hit the global cache by clearing the local one.
			ctx = caching.WithEmptyProcessCache(ctx)
			u, err = call("Bearer access_token")
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, u, should.Resemble(goodUser))

			// No new calls to the token endpoints.
			assert.Loosely(c, checks, should.Equal(1))

			// Advance time until the token expires, but the validation outcome is
			// still cached.
			tc.Add(time.Hour + time.Second)
			status = http.StatusBadRequest

			// Correctly identified as expired, no new calls to the token endpoint.
			_, err = call("Bearer access_token")
			assert.Loosely(c, err, should.Equal(ErrBadOAuthToken))
			assert.Loosely(c, checks, should.Equal(1))

			// Advance time a bit more until the token is evicted from the cache.
			tc.Add(15 * time.Minute)

			// Correctly identified as expired, via a call to the token endpoint.
			_, err = call("Bearer access_token")
			assert.Loosely(c, err, should.Equal(ErrBadOAuthToken))
			assert.Loosely(c, checks, should.Equal(2))
		})

		c.Run("Bad tokens are cached", func(c *ftt.Test) {
			status = http.StatusBadRequest

			_, err := call("Bearer access_token")
			assert.Loosely(c, err, should.Equal(ErrBadOAuthToken))
			assert.Loosely(c, checks, should.Equal(1))

			// Hit the process cache.
			_, err = call("Bearer access_token")
			assert.Loosely(c, err, should.Equal(ErrBadOAuthToken))

			// Hit the global cache by clearing the local one.
			ctx = caching.WithEmptyProcessCache(ctx)
			_, err = call("Bearer access_token")
			assert.Loosely(c, err, should.Equal(ErrBadOAuthToken))

			// Advance time a little bit, the outcome is still cached.
			tc.Add(5 * time.Minute)
			_, err = call("Bearer access_token")
			assert.Loosely(c, err, should.Equal(ErrBadOAuthToken))
			assert.Loosely(c, checks, should.Equal(1))

			// Advance time until the cache entry expire, the token is rechecked.
			tc.Add(15 * time.Minute)
			_, err = call("Bearer access_token")
			assert.Loosely(c, err, should.Equal(ErrBadOAuthToken))
			assert.Loosely(c, checks, should.Equal(2))
		})

		c.Run("Bad header", func(c *ftt.Test) {
			_, err := call("broken")
			assert.Loosely(c, err, should.ErrLike("oauth: bad Authorization header"))
		})

		c.Run("HTTP 500", func(c *ftt.Test) {
			status = http.StatusInternalServerError
			_, err := call("Bearer access_token")
			assert.Loosely(c, err, should.ErrLike("transient error"))
		})

		c.Run("Error response", func(c *ftt.Test) {
			status = http.StatusBadRequest
			info.Error = "OMG, error"
			_, err := call("Bearer access_token")
			assert.Loosely(c, err, should.Equal(ErrBadOAuthToken))
		})

		c.Run("No email", func(c *ftt.Test) {
			info.Email = ""
			_, err := call("Bearer access_token")
			assert.Loosely(c, err, should.Equal(ErrBadOAuthToken))
		})

		c.Run("Email not verified", func(c *ftt.Test) {
			info.EmailVerified = "false"
			_, err := call("Bearer access_token")
			assert.Loosely(c, err, should.Equal(ErrBadOAuthToken))
		})

		c.Run("Bad expires_in", func(c *ftt.Test) {
			info.ExpiresIn = "not a number"
			_, err := call("Bearer access_token")
			assert.Loosely(c, err, should.ErrLike("transient error")) // see the comment in GetTokenInfo
		})

		c.Run("Zero expires_in", func(c *ftt.Test) {
			info.ExpiresIn = "0"
			_, err := call("Bearer access_token")
			assert.Loosely(c, err, should.Equal(ErrBadOAuthToken))
		})

		c.Run("No audience", func(c *ftt.Test) {
			info.Audience = ""
			_, err := call("Bearer access_token")
			assert.Loosely(c, err, should.Equal(ErrBadOAuthToken))
		})

		c.Run("No scope", func(c *ftt.Test) {
			info.Scope = ""
			_, err := call("Bearer access_token")
			assert.Loosely(c, err, should.Equal(ErrBadOAuthToken))
		})

		c.Run("Bad email", func(c *ftt.Test) {
			info.Email = "@@@@"
			_, err := call("Bearer access_token")
			assert.Loosely(c, err, should.Equal(ErrBadOAuthToken))
		})

		c.Run("Missing required scope", func(c *ftt.Test) {
			info.Scope = "some other scopes"
			_, err := call("Bearer access_token")
			assert.Loosely(c, err, should.Equal(ErrBadOAuthToken))
		})
	})
}

func TestGetUserCredentials(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	m := GoogleOAuth2Method{}

	call := func(hdr string) (*oauth2.Token, error) {
		req := makeRequest()
		req.FakeHeader.Set("Authorization", hdr)
		return m.GetUserCredentials(ctx, req)
	}

	ftt.Run("Works", t, func(t *ftt.Test) {
		tok, err := call("Bearer abc.def")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tok, should.Resemble(&oauth2.Token{
			AccessToken: "abc.def",
			TokenType:   "Bearer",
		}))
	})

	ftt.Run("Normalizes header", t, func(t *ftt.Test) {
		tok, err := call("  bearer    abc.def")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tok, should.Resemble(&oauth2.Token{
			AccessToken: "abc.def",
			TokenType:   "Bearer",
		}))
	})

	ftt.Run("Bad headers", t, func(t *ftt.Test) {
		_, err := call("")
		assert.Loosely(t, err, should.Equal(ErrBadAuthorizationHeader))
		_, err = call("abc.def")
		assert.Loosely(t, err, should.Equal(ErrBadAuthorizationHeader))
		_, err = call("Basic abc.def")
		assert.Loosely(t, err, should.Equal(ErrBadAuthorizationHeader))
	})
}
