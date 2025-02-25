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

	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"

	"go.chromium.org/luci/tokenserver/api/minter/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/projectidentity"
)

const (
	testAppID         = "unit-tests"
	testAppVer        = "mocked-ver"
	testServiceVer    = testAppID + "/" + testAppVer
	testCaller        = identity.Identity("project:something")
	testPeer          = identity.Identity("user:service@example.com")
	testPeerIP        = "127.10.10.10"
	testAccount       = identity.Identity("user:sa@example.com")
	testProject       = "test-proj"
	testRealm         = testProject + ":test-realm"
	testProjectScoped = "test-proj-scoped"
	testRealmScoped   = testProjectScoped + ":test-realm"
	testInternalRealm = realms.InternalProject + ":test-realm"
)

var testRequestID = trace.TraceID{1, 2, 3, 4, 5}

func TestMintServiceAccountToken(t *testing.T) {
	ctx := gaetesting.TestingContext()
	ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: testRequestID,
	}))
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
			authtest.MockPermission(testCaller, testRealmScoped, permMintToken),
			authtest.MockPermission(testAccount, testRealmScoped, permExistInRealm),
			authtest.MockPermission(testCaller, testInternalRealm, permMintToken),
			authtest.MockPermission(testAccount, testInternalRealm, permExistInRealm),
		),
	})
	mapping, _ := loadMapping(ctx, fmt.Sprintf(`
		mapping {
			project: "%s"
			service_account: "%s"
		}

		use_project_scoped_account: "%s"
		use_project_scoped_account: "%s"
	`, testProject, testAccount.Email(), testProjectScoped, realms.InternalProject))

	_, err := projectidentity.ProjectIdentities(ctx).Create(ctx, &projectidentity.ProjectIdentity{
		Project: testProjectScoped,
		Email:   "scoped@example.com",
	})
	if err != nil {
		panic(err)
	}

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
		ProjectIdentities: projectidentity.ProjectIdentities,
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

	ftt.Run("Happy path", t, func(t *ftt.Test) {
		t.Run("Access token", func(t *ftt.Test) {
			req := &minter.MintServiceAccountTokenRequest{
				TokenKind:      minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
				ServiceAccount: testAccount.Email(),
				Realm:          testRealm,
				OauthScope:     []string{"scope-z", "scope-a", "scope-a"},
				AuditTags:      []string{"k:v1", "k:v2"},
			}
			resp, err := rpc.MintServiceAccountToken(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Match(&minter.MintServiceAccountTokenResponse{
				Token:          "access-token-for-" + testAccount.Email(),
				Expiry:         timestamppb.New(testclock.TestRecentTimeUTC.Add(time.Hour).Truncate(time.Second)),
				ServiceVersion: testServiceVer,
			}))

			assert.Loosely(t, lastAccessTokenCall, should.Match(auth.MintAccessTokenParams{
				ServiceAccount: testAccount.Email(),
				Scopes:         []string{"scope-a", "scope-z"},
				MinTTL:         5 * time.Minute,
			}))

			// We can't use ShouldResemble here because it contains proto messages.
			// Compare field-by-field instead.
			assert.Loosely(t, loggedTok.Request, should.Equal(req))
			assert.Loosely(t, loggedTok.Response, should.Equal(resp))
			assert.Loosely(t, loggedTok.RequestedAt, should.Match(clock.Now(ctx)))
			assert.Loosely(t, loggedTok.OAuthScopes, should.Match([]string{"scope-a", "scope-z"}))
			assert.Loosely(t, loggedTok.RequestIdentity, should.Equal(testCaller))
			assert.Loosely(t, loggedTok.PeerIdentity, should.Equal(testPeer))
			assert.Loosely(t, loggedTok.ConfigRev, should.Equal("fake-revision"))
			assert.Loosely(t, loggedTok.PeerIP.String(), should.Equal(testPeerIP))
			assert.Loosely(t, loggedTok.RequestID, should.Equal(testRequestID.String()))
			assert.Loosely(t, loggedTok.AuthDBRev, should.BeZero) // FakeDB is always 0
		})

		t.Run("ID token", func(t *ftt.Test) {
			req := &minter.MintServiceAccountTokenRequest{
				TokenKind:       minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ID_TOKEN,
				ServiceAccount:  testAccount.Email(),
				Realm:           testRealm,
				IdTokenAudience: "test-audience",
				AuditTags:       []string{"k:v1", "k:v2"},
			}
			resp, err := rpc.MintServiceAccountToken(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Match(&minter.MintServiceAccountTokenResponse{
				Token:          "id-token-for-" + testAccount.Email(),
				Expiry:         timestamppb.New(testclock.TestRecentTimeUTC.Add(time.Hour).Truncate(time.Second)),
				ServiceVersion: testServiceVer,
			}))

			assert.Loosely(t, lastIDTokenCall, should.Match(auth.MintIDTokenParams{
				ServiceAccount: testAccount.Email(),
				Audience:       "test-audience",
				MinTTL:         5 * time.Minute,
			}))
		})

		t.Run("Delegation through project-scoped account", func(t *ftt.Test) {
			req := &minter.MintServiceAccountTokenRequest{
				TokenKind:       minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ID_TOKEN,
				ServiceAccount:  testAccount.Email(),
				Realm:           testRealmScoped,
				IdTokenAudience: "test-audience",
				AuditTags:       []string{"k:v1", "k:v2"},
			}
			resp, err := rpc.MintServiceAccountToken(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Match(&minter.MintServiceAccountTokenResponse{
				Token:          "id-token-for-" + testAccount.Email(),
				Expiry:         timestamppb.New(testclock.TestRecentTimeUTC.Add(time.Hour).Truncate(time.Second)),
				ServiceVersion: testServiceVer,
			}))

			assert.Loosely(t, lastIDTokenCall, should.Match(auth.MintIDTokenParams{
				ServiceAccount: testAccount.Email(),
				Audience:       "test-audience",
				Delegates:      []string{"scoped@example.com"},
				MinTTL:         5 * time.Minute,
			}))
		})

		t.Run("Delegation through project-scoped account in @internal", func(t *ftt.Test) {
			req := &minter.MintServiceAccountTokenRequest{
				TokenKind:       minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ID_TOKEN,
				ServiceAccount:  testAccount.Email(),
				Realm:           testInternalRealm,
				IdTokenAudience: "test-audience",
				AuditTags:       []string{"k:v1", "k:v2"},
			}
			resp, err := rpc.MintServiceAccountToken(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Match(&minter.MintServiceAccountTokenResponse{
				Token:          "id-token-for-" + testAccount.Email(),
				Expiry:         timestamppb.New(testclock.TestRecentTimeUTC.Add(time.Hour).Truncate(time.Second)),
				ServiceVersion: testServiceVer,
			}))

			assert.Loosely(t, lastIDTokenCall, should.Match(auth.MintIDTokenParams{
				ServiceAccount: testAccount.Email(),
				Audience:       "test-audience",
				MinTTL:         5 * time.Minute,
			}))
		})
	})

	ftt.Run("Request validation", t, func(t *ftt.Test) {
		call := func(req *minter.MintServiceAccountTokenRequest) error {
			resp, err := rpc.MintServiceAccountToken(ctx, req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, resp, should.BeNil)
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			return err
		}

		t.Run("Bad token kind", func(t *ftt.Test) {
			assert.Loosely(t, call(&minter.MintServiceAccountTokenRequest{
				TokenKind: 0,
			}), should.ErrLike("token_kind is required"))

			assert.Loosely(t, call(&minter.MintServiceAccountTokenRequest{
				TokenKind: 1234,
			}), should.ErrLike("unrecognized token_kind"))
		})

		t.Run("Bad service account", func(t *ftt.Test) {
			assert.Loosely(t, call(&minter.MintServiceAccountTokenRequest{
				TokenKind: minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
			}), should.ErrLike("service_account is required"))

			assert.Loosely(t, call(&minter.MintServiceAccountTokenRequest{
				TokenKind:      minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
				ServiceAccount: "bad email",
			}), should.ErrLike("bad service_account"))
		})

		t.Run("Bad realm", func(t *ftt.Test) {
			assert.Loosely(t, call(&minter.MintServiceAccountTokenRequest{
				TokenKind:      minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
				ServiceAccount: testAccount.Email(),
			}), should.ErrLike("realm is required"))

			assert.Loosely(t, call(&minter.MintServiceAccountTokenRequest{
				TokenKind:      minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
				ServiceAccount: testAccount.Email(),
				Realm:          "not-global",
			}), should.ErrLike("bad realm"))
		})

		t.Run("Bad access token request", func(t *ftt.Test) {
			assert.Loosely(t, call(&minter.MintServiceAccountTokenRequest{
				TokenKind:      minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
				ServiceAccount: testAccount.Email(),
				Realm:          testRealm,
			}), should.ErrLike("oauth_scope is required"))

			assert.Loosely(t, call(&minter.MintServiceAccountTokenRequest{
				TokenKind:      minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
				ServiceAccount: testAccount.Email(),
				Realm:          testRealm,
				OauthScope:     []string{"zzz", ""},
			}), should.ErrLike("bad oauth_scope: got an empty string"))

			assert.Loosely(t, call(&minter.MintServiceAccountTokenRequest{
				TokenKind:       minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
				ServiceAccount:  testAccount.Email(),
				Realm:           testRealm,
				OauthScope:      []string{"zzz"},
				IdTokenAudience: "aud",
			}), should.ErrLike("id_token_audience must not be used"))
		})

		t.Run("Bad ID token request", func(t *ftt.Test) {
			assert.Loosely(t, call(&minter.MintServiceAccountTokenRequest{
				TokenKind:      minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ID_TOKEN,
				ServiceAccount: testAccount.Email(),
				Realm:          testRealm,
			}), should.ErrLike("id_token_audience is required"))

			assert.Loosely(t, call(&minter.MintServiceAccountTokenRequest{
				TokenKind:       minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ID_TOKEN,
				ServiceAccount:  testAccount.Email(),
				Realm:           testRealm,
				OauthScope:      []string{"zzz"},
				IdTokenAudience: "aud",
			}), should.ErrLike("oauth_scope must not be used"))
		})

		t.Run("Bad min_validity_duration", func(t *ftt.Test) {
			assert.Loosely(t, call(&minter.MintServiceAccountTokenRequest{
				TokenKind:           minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
				ServiceAccount:      testAccount.Email(),
				Realm:               testRealm,
				OauthScope:          []string{"zzz"},
				MinValidityDuration: -1,
			}), should.ErrLike("must be positive"))

			assert.Loosely(t, call(&minter.MintServiceAccountTokenRequest{
				TokenKind:           minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
				ServiceAccount:      testAccount.Email(),
				Realm:               testRealm,
				OauthScope:          []string{"zzz"},
				MinValidityDuration: 3601,
			}), should.ErrLike("must be not greater than 3600"))
		})

		t.Run("Bad audit_tags", func(t *ftt.Test) {
			assert.Loosely(t, call(&minter.MintServiceAccountTokenRequest{
				TokenKind:      minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
				ServiceAccount: testAccount.Email(),
				Realm:          testRealm,
				OauthScope:     []string{"zzz"},
				AuditTags:      []string{"not kv"},
			}), should.ErrLike("bad audit_tags"))
		})

		t.Run("Missing project-scoped identity", func(t *ftt.Test) {
			assert.Loosely(t, projectidentity.ProjectIdentities(ctx).Delete(ctx, &projectidentity.ProjectIdentity{
				Project: testProjectScoped,
			}), should.BeNil)
			assert.Loosely(t, call(&minter.MintServiceAccountTokenRequest{
				TokenKind:      minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
				ServiceAccount: testAccount.Email(),
				Realm:          testRealmScoped,
				OauthScope:     []string{"zzz"},
			}), should.ErrLike("project-scoped account for project test-proj-scoped is not configured"))
		})
	})

	ftt.Run("ACL checks", t, func(t *ftt.Test) {
		call := func(ctx context.Context) error {
			resp, err := rpc.MintServiceAccountToken(ctx, &minter.MintServiceAccountTokenRequest{
				TokenKind:      minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN,
				ServiceAccount: testAccount.Email(),
				Realm:          testRealm,
				OauthScope:     []string{"scope"},
			})
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, resp, should.BeNil)
			assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
			return err
		}

		t.Run("No mint permission", func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: testCaller,
				FakeDB: authtest.NewFakeDB(
					// Note: no perm permMintToken here.
					authtest.MockPermission(testAccount, testRealm, permExistInRealm),
				),
			})
			assert.Loosely(t, call(ctx), should.ErrLike("unknown realm or no permission to use service accounts there"))
		})

		t.Run("No existInRealm permission", func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: testCaller,
				FakeDB: authtest.NewFakeDB(
					authtest.MockPermission(testCaller, testRealm, permMintToken),
					// Note: no perm permExistInRealm here.
				),
			})
			assert.Loosely(t, call(ctx), should.ErrLike("is not in the realm"))
		})

		t.Run("Not in the mapping", func(t *ftt.Test) {
			mapping, _ = loadMapping(ctx, fmt.Sprintf(`mapping {}`))
			assert.Loosely(t, call(ctx), should.ErrLike("is not allowed to be used"))
		})
	})
}
