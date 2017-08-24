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
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"testing"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMintAccessTokenForServiceAccount(t *testing.T) {
	t.Parallel()

	Convey("MintAccessTokenForServiceAccount works", t, func() {
		ctx := context.Background()
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(12345)))

		// Create an LRU large enough that it will never cycle during test.
		tokenCache := NewMemoryCache(1024)

		returnedToken := "token1"
		transport := &clientRPCTransportMock{
			cb: func(r *http.Request, body string) string {
				switch r.URL.String() {
				// IAM request to sign the assertion.
				case "https://iam.googleapis.com/v1/projects/-/serviceAccounts/abc@example.com:signJwt?alt=json":
					// Check the valid claimset is being passed.
					var req struct {
						Payload string `json:"payload"`
					}
					json.Unmarshal([]byte(body), &req)
					claimSet := map[string]interface{}{}
					json.Unmarshal([]byte(req.Payload), &claimSet)
					So(claimSet["iss"], ShouldEqual, "abc@example.com")
					So(claimSet["scope"], ShouldEqual, "scope_a scope_b")
					return `{"keyId":"key_id","signature":"c2lnbmF0dXJl"}`
				// Exchange of the assertion for the access token.
				case "https://www.googleapis.com/oauth2/v4/token":
					return fmt.Sprintf(`{"access_token":"%s","token_type":"Bearer","expires_in":3600}`, returnedToken)
				default:
					t.Fatalf("Unexpected request to %s", r.URL)
					return "unknown URL"
				}
			},
		}

		ctx = ModifyConfig(ctx, func(cfg Config) Config {
			cfg.AccessTokenProvider = transport.getAccessToken
			cfg.AnonymousTransport = transport.getTransport
			cfg.Cache = tokenCache
			return cfg
		})

		tok, err := MintAccessTokenForServiceAccount(ctx, MintAccessTokenParams{
			ServiceAccount: "abc@example.com",
			Scopes:         []string{"scope_b", "scope_a"},
		})
		So(err, ShouldBeNil)
		So(tok, ShouldResemble, &oauth2.Token{
			AccessToken: "token1",
			TokenType:   "Bearer",
			Expiry:      clock.Now(ctx).Add(3600 * time.Second).UTC(),
		})

		// Cached now.
		So(tokenCache.(*MemoryCache).LRU.Len(), ShouldEqual, 1)
		v, _ := tokenCache.Get(ctx, fmt.Sprintf("as_actor_tokens/%d/b16kofTATGlqFdw3fKVf2-pyMEs", actorTokenCache.Version))
		So(v, ShouldNotBeNil)

		// On subsequence request the cached token is used.
		returnedToken = "token2"
		tok, err = MintAccessTokenForServiceAccount(ctx, MintAccessTokenParams{
			ServiceAccount: "abc@example.com",
			Scopes:         []string{"scope_b", "scope_a"},
		})
		So(err, ShouldBeNil)
		So(tok.AccessToken, ShouldEqual, "token1") // old one

		// Unless it expires sooner than requested TTL.
		clock.Get(ctx).(testclock.TestClock).Add(40 * time.Minute)
		tok, err = MintAccessTokenForServiceAccount(ctx, MintAccessTokenParams{
			ServiceAccount: "abc@example.com",
			Scopes:         []string{"scope_b", "scope_a"},
			MinTTL:         30 * time.Minute,
		})
		So(err, ShouldBeNil)
		So(tok.AccessToken, ShouldResemble, "token2") // new one
	})
}
