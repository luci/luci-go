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

package serviceaccounts

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/trace/tracetest"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"

	"go.chromium.org/luci/tokenserver/api/minter/v1"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

const (
	testAppID      = "unit-tests"
	testAppVer     = "mocked-ver"
	testServiceVer = testAppID + "/" + testAppVer
	testCaller     = identity.Identity("project:something")
	testPeer       = identity.Identity("user:service@example.com")
	testPeerIP     = "127.10.10.10"
	testAccount    = identity.Identity("user:sa@example.com")
	testProject    = "test-proj"
	testRealm      = testProject + ":test-realm"
	testRequestID  = "gae-request-id"
)

func init() {
	tracetest.Enable()
}

func TestMintServiceAccountToken(t *testing.T) {
	ctx := gaetesting.TestingContext()
	ctx = tracetest.WithSpanContext(ctx, "gae-request-id")
	ctx = logging.SetLevel(ctx, logging.Debug) // coverage for logRequest
	ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

	// Will be changed on per test case basis.
	ctx = auth.WithState(ctx, &authtest.FakeState{
		Identity:             testCaller,
		PeerIdentityOverride: testPeer,
		PeerIPOverride:       net.ParseIP(testPeerIP),
		FakeDB: authtest.NewFakeDB(
			authtest.MockPermission(testCaller, testRealm, permMintToken),
			authtest.MockPermission(testAccount, testRealm, permExistInRealm),
		),
	})
	mapping, _ := loadMapping(ctx, fmt.Sprintf(`mapping {
		project: "%s"
		service_account: "%s"
	}`, testProject, testAccount.Email()))

	// Records last received arguments of Mint*Token.
	var lastAccessTokenCall auth.MintAccessTokenParams
	var lastIDTokenCall auth.MintIDTokenParams

	// Records last call to LogToken.
	var loggedTok *MintedTokenInfo

	rpc := MintServiceAccountTokenRPC{
		Signer: signingtest.NewSigner(&signing.ServiceInfo{
			AppID:      testAppID,
			AppVersion: testAppVer,
		}),
		Mapping: func(context.Context) (*Mapping, error) {
			return mapping, nil
		},
		MintAccessToken: func(ctx context.Context, params auth.MintAccessTokenParams) (*auth.Token, error) {
			lastAccessTokenCall = params
			return &auth.Token{
				Token:  "access-token-for-" + params.ServiceAccount,
				Expiry: clock.Now(ctx).Add(time.Hour).Truncate(time.Second),
			}, nil
		},
		MintIDToken: func(ctx context.Context, params auth.MintIDTokenParams) (*auth.Token, error) {
			lastIDTokenCall = params
			return &auth.Token{
				Token:  "id-token-for-" + params.ServiceAccount,
				Expiry: clock.Now(ctx).Add(time.Hour).Truncate(time.Second),
			}, nil
		},
		LogToken: func(ctx context.Context, info *MintedTokenInfo) error {
			loggedTok = info
			return nil
		},
	}

	Convey("Happy path", t, func() {
		Convey("Access token", func() {
			req := &minter.MintServiceAccountTokenRequest{
				TokenKind:      minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
				ServiceAccount: testAccount.Email(),
				Realm:          testRealm,
				OauthScope:     []string{"scope-z", "scope-a", "scope-a"},
				AuditTags:      []string{"k:v1", "k:v2"},
			}
			resp, err := rpc.MintServiceAccountToken(ctx, req)
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, &minter.MintServiceAccountTokenResponse{
				Token:          "access-token-for-" + testAccount.Email(),
				Expiry:         timestamppb.New(testclock.TestRecentTimeUTC.Add(time.Hour).Truncate(time.Second)),
				ServiceVersion: testServiceVer,
			})

			So(lastAccessTokenCall, ShouldResemble, auth.MintAccessTokenParams{
				ServiceAccount: testAccount.Email(),
				Scopes:         []string{"scope-a", "scope-z"},
				MinTTL:         5 * time.Minute,
			})

			// We can't use ShouldResemble here because it contains proto messages.
			// Compare field-by-field instead.
			So(loggedTok.Request, ShouldEqual, req)
			So(loggedTok.Response, ShouldEqual, resp)
			So(loggedTok.RequestedAt, ShouldResemble, clock.Now(ctx))
			So(loggedTok.OAuthScopes, ShouldResemble, []string{"scope-a", "scope-z"})
			So(loggedTok.RequestIdentity, ShouldEqual, testCaller)
			So(loggedTok.PeerIdentity, ShouldEqual, testPeer)
			So(loggedTok.ConfigRev, ShouldEqual, "fake-revision")
			So(loggedTok.PeerIP.String(), ShouldEqual, testPeerIP)
			So(loggedTok.RequestID, ShouldEqual, testRequestID)
			So(loggedTok.AuthDBRev, ShouldEqual, 0) // FakeDB is always 0
		})

		Convey("ID token", func() {
			req := &minter.MintServiceAccountTokenRequest{
				TokenKind:       minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ID_TOKEN,
				ServiceAccount:  testAccount.Email(),
				Realm:           testRealm,
				IdTokenAudience: "test-audience",
				AuditTags:       []string{"k:v1", "k:v2"},
			}
			resp, err := rpc.MintServiceAccountToken(ctx, req)
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, &minter.MintServiceAccountTokenResponse{
				Token:          "id-token-for-" + testAccount.Email(),
				Expiry:         timestamppb.New(testclock.TestRecentTimeUTC.Add(time.Hour).Truncate(time.Second)),
				ServiceVersion: testServiceVer,
			})

			So(lastIDTokenCall, ShouldResemble, auth.MintIDTokenParams{
				ServiceAccount: testAccount.Email(),
				Audience:       "test-audience",
				MinTTL:         5 * time.Minute,
			})
		})
	})

	Convey("Request validation", t, func() {
		call := func(req *minter.MintServiceAccountTokenRequest) error {
			resp, err := rpc.MintServiceAccountToken(ctx, req)
			So(err, ShouldNotBeNil)
			So(resp, ShouldBeNil)
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			return err
		}

		Convey("Bad token kind", func() {
			So(call(&minter.MintServiceAccountTokenRequest{
				TokenKind: 0,
			}), ShouldErrLike, "token_kind is required")

			So(call(&minter.MintServiceAccountTokenRequest{
				TokenKind: 1234,
			}), ShouldErrLike, "unrecognized token_kind")
		})

		Convey("Bad service account", func() {
			So(call(&minter.MintServiceAccountTokenRequest{
				TokenKind: minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
			}), ShouldErrLike, "service_account is required")

			So(call(&minter.MintServiceAccountTokenRequest{
				TokenKind:      minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
				ServiceAccount: "bad email",
			}), ShouldErrLike, "bad service_account")
		})

		Convey("Bad realm", func() {
			So(call(&minter.MintServiceAccountTokenRequest{
				TokenKind:      minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
				ServiceAccount: testAccount.Email(),
			}), ShouldErrLike, "realm is required")

			So(call(&minter.MintServiceAccountTokenRequest{
				TokenKind:      minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
				ServiceAccount: testAccount.Email(),
				Realm:          "not-global",
			}), ShouldErrLike, "bad realm")
		})

		Convey("Bad access token request", func() {
			So(call(&minter.MintServiceAccountTokenRequest{
				TokenKind:      minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
				ServiceAccount: testAccount.Email(),
				Realm:          testRealm,
			}), ShouldErrLike, "oauth_scope is required")

			So(call(&minter.MintServiceAccountTokenRequest{
				TokenKind:      minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
				ServiceAccount: testAccount.Email(),
				Realm:          testRealm,
				OauthScope:     []string{"zzz", ""},
			}), ShouldErrLike, "bad oauth_scope: got an empty string")

			So(call(&minter.MintServiceAccountTokenRequest{
				TokenKind:       minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
				ServiceAccount:  testAccount.Email(),
				Realm:           testRealm,
				OauthScope:      []string{"zzz"},
				IdTokenAudience: "aud",
			}), ShouldErrLike, "id_token_audience must not be used")
		})

		Convey("Bad ID token request", func() {
			So(call(&minter.MintServiceAccountTokenRequest{
				TokenKind:      minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ID_TOKEN,
				ServiceAccount: testAccount.Email(),
				Realm:          testRealm,
			}), ShouldErrLike, "id_token_audience is required")

			So(call(&minter.MintServiceAccountTokenRequest{
				TokenKind:       minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ID_TOKEN,
				ServiceAccount:  testAccount.Email(),
				Realm:           testRealm,
				OauthScope:      []string{"zzz"},
				IdTokenAudience: "aud",
			}), ShouldErrLike, "oauth_scope must not be used")
		})

		Convey("Bad min_validity_duration", func() {
			So(call(&minter.MintServiceAccountTokenRequest{
				TokenKind:           minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
				ServiceAccount:      testAccount.Email(),
				Realm:               testRealm,
				OauthScope:          []string{"zzz"},
				MinValidityDuration: -1,
			}), ShouldErrLike, "must be positive")

			So(call(&minter.MintServiceAccountTokenRequest{
				TokenKind:           minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
				ServiceAccount:      testAccount.Email(),
				Realm:               testRealm,
				OauthScope:          []string{"zzz"},
				MinValidityDuration: 3601,
			}), ShouldErrLike, "must be not greater than 3600")
		})

		Convey("Bad audit_tags", func() {
			So(call(&minter.MintServiceAccountTokenRequest{
				TokenKind:      minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
				ServiceAccount: testAccount.Email(),
				Realm:          testRealm,
				OauthScope:     []string{"zzz"},
				AuditTags:      []string{"not kv"},
			}), ShouldErrLike, "bad audit_tags")
		})
	})

	Convey("ACL checks", t, func() {
		call := func(ctx context.Context) error {
			resp, err := rpc.MintServiceAccountToken(ctx, &minter.MintServiceAccountTokenRequest{
				TokenKind:      minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
				ServiceAccount: testAccount.Email(),
				Realm:          testRealm,
				OauthScope:     []string{"scope"},
			})
			So(err, ShouldNotBeNil)
			So(resp, ShouldBeNil)
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
			return err
		}

		Convey("No mint permission", func() {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: testCaller,
				FakeDB: authtest.NewFakeDB(
					// Note: no perm permMintToken here.
					authtest.MockPermission(testAccount, testRealm, permExistInRealm),
				),
			})
			So(call(ctx), ShouldErrLike, "unknown realm or no permission to use service accounts there")
		})

		Convey("No existInRealm permission", func() {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: testCaller,
				FakeDB: authtest.NewFakeDB(
					authtest.MockPermission(testCaller, testRealm, permMintToken),
					// Note: no perm permExistInRealm here.
				),
			})
			So(call(ctx), ShouldErrLike, "is not in the realm")
		})

		Convey("Not in the mapping", func() {
			mapping, _ = loadMapping(ctx, fmt.Sprintf(`mapping {}`))
			So(call(ctx), ShouldErrLike, "is not allowed to be used")
		})
	})
}
