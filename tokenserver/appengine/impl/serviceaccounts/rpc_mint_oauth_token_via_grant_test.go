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

package serviceaccounts

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/tokenserver/api"
	"go.chromium.org/luci/tokenserver/api/minter/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMintOAuthTokenViaGrant(t *testing.T) {
	ctx := context.Background()
	ctx = logging.SetLevel(ctx, logging.Debug)
	ctx, _ = testclock.UseTime(ctx, testclock.TestTimeUTC)
	ctx = auth.WithState(ctx, &authtest.FakeState{
		Identity: "user:proxy@example.com",
	})

	var lastMintParams auth.MintAccessTokenParams
	rpc := MintOAuthTokenViaGrantRPC{
		Signer: testingSigner(),
		Rules: func(context.Context) (*Rules, error) {
			return loadConfig(`rules {
				name: "rule 1"
				service_account: "serviceaccount@robots.com"
				proxy: "user:proxy@example.com"
				end_user: "user:enduser@example.com"
				allowed_scope: "https://www.googleapis.com/scope1"
				allowed_scope: "https://www.googleapis.com/scope2"
				max_grant_validity_duration: 7200
			}`)
		},
		MintAccessToken: func(ctx context.Context, params auth.MintAccessTokenParams) (*oauth2.Token, error) {
			lastMintParams = params
			return &oauth2.Token{
				AccessToken: "access-token-for-" + params.ServiceAccount,
				Expiry:      clock.Now(ctx).Add(time.Hour),
			}, nil
		},
	}

	grant, _ := SignGrant(ctx, rpc.Signer, &tokenserver.OAuthTokenGrantBody{
		TokenId:          123,
		ServiceAccount:   "serviceaccount@robots.com",
		Proxy:            "user:proxy@example.com",
		EndUser:          "user:enduser@example.com",
		IssuedAt:         google.NewTimestamp(clock.Now(ctx)),
		ValidityDuration: 3600,
	})

	Convey("Happy path", t, func() {
		resp, err := rpc.MintOAuthTokenViaGrant(ctx, &minter.MintOAuthTokenViaGrantRequest{
			GrantToken: grant,
			OauthScope: []string{"https://www.googleapis.com/scope1"},
		})
		So(err, ShouldBeNil)
		So(resp, ShouldResemble, &minter.MintOAuthTokenViaGrantResponse{
			AccessToken:    "access-token-for-serviceaccount@robots.com",
			Expiry:         google.NewTimestamp(testclock.TestTimeUTC.Add(time.Hour)),
			ServiceVersion: "unit-tests/mocked-ver",
		})
		So(lastMintParams, ShouldResemble, auth.MintAccessTokenParams{
			ServiceAccount: "serviceaccount@robots.com",
			Scopes:         []string{"https://www.googleapis.com/scope1"},
			MinTTL:         defaultMinValidityDuration,
		})
	})

	Convey("Negative validity duration", t, func() {
		_, err := rpc.MintOAuthTokenViaGrant(ctx, &minter.MintOAuthTokenViaGrantRequest{
			GrantToken:          grant,
			OauthScope:          []string{"https://www.googleapis.com/scope1"},
			MinValidityDuration: -100,
		})
		So(err, ShouldBeRPCInvalidArgument, "min_validity_duration must be positive")
	})

	Convey("Huge validity duration", t, func() {
		_, err := rpc.MintOAuthTokenViaGrant(ctx, &minter.MintOAuthTokenViaGrantRequest{
			GrantToken:          grant,
			OauthScope:          []string{"https://www.googleapis.com/scope1"},
			MinValidityDuration: 1801,
		})
		So(err, ShouldBeRPCInvalidArgument, "min_validity_duration must not exceed 1800")
	})

	Convey("No oauth_scope", t, func() {
		_, err := rpc.MintOAuthTokenViaGrant(ctx, &minter.MintOAuthTokenViaGrantRequest{
			GrantToken: grant,
		})
		So(err, ShouldBeRPCInvalidArgument, "oauth_scope is required")
	})

	Convey("Broken body", t, func() {
		_, err := rpc.MintOAuthTokenViaGrant(ctx, &minter.MintOAuthTokenViaGrantRequest{
			GrantToken: "lalala-bad-token",
			OauthScope: []string{"https://www.googleapis.com/scope1"},
		})
		So(err, ShouldBeRPCInvalidArgument, "malformed grant token - can't unmarshal the envelope")
	})

	Convey("Broken signature", t, func() {
		env, _, _ := deserializeForTest(ctx, grant, rpc.Signer)
		env.Pkcs1Sha256Sig = []byte("lalala-bad-signature")
		blob, _ := proto.Marshal(env)
		tok := base64.RawURLEncoding.EncodeToString(blob)

		_, err := rpc.MintOAuthTokenViaGrant(ctx, &minter.MintOAuthTokenViaGrantRequest{
			GrantToken: tok,
			OauthScope: []string{"https://www.googleapis.com/scope1"},
		})
		So(err, ShouldBeRPCInvalidArgument, "invalid grant token - bad signature")
	})

	Convey("Expired", t, func() {
		ctx, _ := testclock.UseTime(ctx, testclock.TestTimeUTC.Add(3601*time.Second))
		_, err := rpc.MintOAuthTokenViaGrant(ctx, &minter.MintOAuthTokenViaGrantRequest{
			GrantToken: grant,
			OauthScope: []string{"https://www.googleapis.com/scope1"},
		})
		So(err, ShouldBeRPCInvalidArgument, "invalid grant token - expired")
	})

	Convey("Wrong caller", t, func() {
		ctx := auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:not-proxy@example.com",
		})
		_, err := rpc.MintOAuthTokenViaGrant(ctx, &minter.MintOAuthTokenViaGrantRequest{
			GrantToken: grant,
			OauthScope: []string{"https://www.googleapis.com/scope1"},
		})
		So(err, ShouldBeRPCPermissionDenied, "unauthorized caller (expecting user:proxy@example.com)")
	})

	Convey("Doesn't pass rules anymore", t, func() {
		rpc := rpc
		rpc.Rules = func(context.Context) (*Rules, error) {
			return loadConfig("")
		}
		_, err := rpc.MintOAuthTokenViaGrant(ctx, &minter.MintOAuthTokenViaGrantRequest{
			GrantToken: grant,
			OauthScope: []string{"https://www.googleapis.com/scope1"},
		})
		So(err, ShouldBeRPCPermissionDenied, "unknown service account or not enough permissions to use it")
	})

	Convey("Forbidden scopes", t, func() {
		_, err := rpc.MintOAuthTokenViaGrant(ctx, &minter.MintOAuthTokenViaGrantRequest{
			GrantToken: grant,
			OauthScope: []string{
				"https://www.googleapis.com/scope1",
				"https://www.googleapis.com/unknown",
			},
		})
		So(err, ShouldBeRPCPermissionDenied,
			`scopes are not allowed by the rule "rule 1" - ["https://www.googleapis.com/unknown"]`)
	})
}
