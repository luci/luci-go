// Copyright 2017 The LUCI Authors.
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
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	"go.chromium.org/luci/server/caching"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMintAccessTokenForServiceAccount(t *testing.T) {
	t.Parallel()

	Convey("MintAccessTokenForServiceAccount works", t, func() {
		ctx := context.Background()
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = caching.WithEmptyProcessCache(ctx)

		returnedToken := "token1"
		lastRequest := ""

		generateTokenURL := fmt.Sprintf("https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/%s:generateAccessToken?alt=json",
			url.QueryEscape("abc@example.com"))

		transport := &clientRPCTransportMock{
			cb: func(r *http.Request, body string) string {
				lastRequest = body

				expireTime, err := time.Parse(time.RFC3339, clock.Now(ctx).Add(time.Hour).UTC().Format(time.RFC3339))
				if err != nil {
					t.Fatalf("Unable to parse/format time: %v", err)
				}

				if r.URL.String() == generateTokenURL {
					return fmt.Sprintf(`{"accessToken":"%s","expireTime":"%s"}`, returnedToken, expireTime.Format(time.RFC3339))
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

		tok, err := MintAccessTokenForServiceAccount(ctx, MintAccessTokenParams{
			ServiceAccount: "abc@example.com",
			Scopes:         []string{"scope_b", "scope_a"},
		})
		So(err, ShouldBeNil)

		expectedExpireTime, err := time.Parse(time.RFC3339, clock.Now(ctx).Add(time.Hour).UTC().Format(time.RFC3339))
		So(err, ShouldBeNil)

		So(tok, ShouldResemble, &Token{
			Token:  "token1",
			Expiry: expectedExpireTime,
		})

		// Cached now.
		So(actorAccessTokenCache.lc.CachedLocally(ctx), ShouldEqual, 1)

		// On subsequence request the cached token is used.
		returnedToken = "token2"
		tok, err = MintAccessTokenForServiceAccount(ctx, MintAccessTokenParams{
			ServiceAccount: "abc@example.com",
			Scopes:         []string{"scope_b", "scope_a"},
		})
		So(err, ShouldBeNil)
		So(tok.Token, ShouldEqual, "token1") // old one

		// Unless it expires sooner than requested TTL.
		clock.Get(ctx).(testclock.TestClock).Add(40 * time.Minute)
		tok, err = MintAccessTokenForServiceAccount(ctx, MintAccessTokenParams{
			ServiceAccount: "abc@example.com",
			Scopes:         []string{"scope_b", "scope_a"},
			MinTTL:         30 * time.Minute,
		})
		So(err, ShouldBeNil)
		So(tok.Token, ShouldResemble, "token2") // new one

		// Using delegates results in a different cache key.
		returnedToken = "token3"
		tok, err = MintAccessTokenForServiceAccount(ctx, MintAccessTokenParams{
			ServiceAccount: "abc@example.com",
			Scopes:         []string{"scope_b", "scope_a"},
			Delegates:      []string{"d2@example.com", "d1@example.com"},
			MinTTL:         30 * time.Minute,
		})
		So(err, ShouldBeNil)
		So(tok.Token, ShouldResemble, "token3") // new one
		So(lastRequest, ShouldEqual,
			`{"delegates":["projects/-/serviceAccounts/d2@example.com","projects/-/serviceAccounts/d1@example.com"],"scope":["scope_a","scope_b"]}`)
	})
}
