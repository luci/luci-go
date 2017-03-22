// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/data/rand/mathrand"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMintAccessTokenForServiceAccount(t *testing.T) {
	t.Parallel()

	Convey("MintAccessTokenForServiceAccount works", t, func() {
		ctx := context.Background()
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(12345)))

		// Create an LRU large enough that it will never cycle during test.
		tokenCache := MemoryCache(1024)

		returnedToken := "token1"
		transport := &clientRPCTransportMock{
			cb: func(r *http.Request, body string) string {
				switch r.URL.String() {
				// IAM request to sign the assertion.
				case "https://iam.googleapis.com/v1/projects/-/serviceAccounts/abc@example.com:signBlob?alt=json":
					// Check the valid claimset is being passed.
					var req struct {
						BytesToSign []byte `json:"bytesToSign"`
					}
					json.Unmarshal([]byte(body), &req)
					claimSetBlob, _ := base64.RawURLEncoding.DecodeString(strings.Split(string(req.BytesToSign), ".")[1])
					claimSet := map[string]interface{}{}
					json.Unmarshal(claimSetBlob, &claimSet)
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

		ctx = ModifyConfig(ctx, func(cfg *Config) {
			cfg.AccessTokenProvider = transport.getAccessToken
			cfg.AnonymousTransport = transport.getTransport
			cfg.Cache = tokenCache
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
		So(tokenCache.(*memoryCache).cache.Len(), ShouldEqual, 1)
		v, _ := tokenCache.Get(ctx, "as_actor_tokens/1/b16kofTATGlqFdw3fKVf2-pyMEs")
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
