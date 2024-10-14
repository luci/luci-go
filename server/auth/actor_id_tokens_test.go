// Copyright 2020 The LUCI Authors.
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
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/caching"
)

func TestMintIDTokenForServiceAccount(t *testing.T) {
	t.Parallel()

	ftt.Run("MintIDTokenForServiceAccount works", t, func(t *ftt.Test) {
		ctx := context.Background()
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = caching.WithEmptyProcessCache(ctx)

		// Will be changed throughout the test.
		var returnedToken *Token
		var lastRequest string

		generateTokenURL := fmt.Sprintf(
			"https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/%s:generateIdToken?alt=json",
			url.QueryEscape("abc@example.com"))

		transport := &clientRPCTransportMock{
			cb: func(r *http.Request, body string) string {
				lastRequest = body
				if r.URL.String() == generateTokenURL {
					return fmt.Sprintf(`{"token":"%s"}`, returnedToken.Token)
				}
				t.Fatalf("Unexpected request to %s", r.URL.String())
				return "unknown URL"
			},
		}

		ctx = ModifyConfig(ctx, func(cfg Config) Config {
			cfg.AccessTokenProvider = transport.getAccessToken
			cfg.AnonymousTransport = transport.getTransport
			return cfg
		})

		token1 := genFakeIDToken(ctx, "token1", time.Hour)
		token2 := genFakeIDToken(ctx, "token2", 2*time.Hour)
		token3 := genFakeIDToken(ctx, "token3", 2*time.Hour)

		returnedToken = token1
		tok, err := MintIDTokenForServiceAccount(ctx, MintIDTokenParams{
			ServiceAccount: "abc@example.com",
			Audience:       "aud",
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tok, should.Resemble(token1))

		// Cached now.
		assert.Loosely(t, actorIDTokenCache.lc.CachedLocally(ctx), should.Equal(1))

		// On subsequence request the cached token is used.
		returnedToken = token2
		tok, err = MintIDTokenForServiceAccount(ctx, MintIDTokenParams{
			ServiceAccount: "abc@example.com",
			Audience:       "aud",
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tok, should.Resemble(token1)) // old one

		// Unless it expires sooner than requested TTL.
		clock.Get(ctx).(testclock.TestClock).Add(40 * time.Minute)
		tok, err = MintIDTokenForServiceAccount(ctx, MintIDTokenParams{
			ServiceAccount: "abc@example.com",
			Audience:       "aud",
			MinTTL:         30 * time.Minute,
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tok, should.Resemble(token2))

		// Using delegates results in a different cache key.
		returnedToken = token3
		tok, err = MintIDTokenForServiceAccount(ctx, MintIDTokenParams{
			ServiceAccount: "abc@example.com",
			Audience:       "aud",
			Delegates:      []string{"d2@example.com", "d1@example.com"},
			MinTTL:         30 * time.Minute,
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tok, should.Resemble(token3)) // new one
		assert.Loosely(t, lastRequest, should.Equal(
			`{"delegates":["projects/-/serviceAccounts/d2@example.com","projects/-/serviceAccounts/d1@example.com"],"audience":"aud","includeEmail":true}`))
	})
}

func genFakeIDToken(ctx context.Context, name string, exp time.Duration) *Token {
	expiry := clock.Now(ctx).Add(exp).Round(time.Second).UTC()
	payload := fmt.Sprintf(`{"exp":%d}`, expiry.Unix())
	return &Token{
		Token:  fmt.Sprintf("%s.%s.zzz", name, base64.RawURLEncoding.EncodeToString([]byte(payload))),
		Expiry: expiry,
	}
}
