// Copyright 2016 The LUCI Authors.
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

package delegation

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"
	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/api/minter/v1"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

var testingRequestID = trace.TraceID{1, 2, 3, 4, 5}

func mockedFetchLUCIServiceIdentity(c context.Context, u string) (identity.Identity, error) {
	l, err := url.Parse(u)
	if err != nil {
		return "", err
	}
	if l.Scheme != "https" {
		return "", fmt.Errorf("wrong scheme")
	}
	if l.Host == "crash" {
		return "", fmt.Errorf("boom")
	}
	return identity.MakeIdentity("service:" + l.Host)
}

func init() {
	fetchLUCIServiceIdentity = mockedFetchLUCIServiceIdentity
}

func testingContext() context.Context {
	ctx := gaetesting.TestingContext()
	ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: testingRequestID,
	}))
	ctx, _ = testclock.UseTime(ctx, time.Date(2015, time.February, 3, 4, 5, 6, 0, time.UTC))
	return auth.WithState(ctx, &authtest.FakeState{
		Identity:       "user:requestor@example.com",
		PeerIPOverride: net.ParseIP("127.10.10.10"),
		FakeDB:         &authdb.SnapshotDB{Rev: 1234},
	})
}

func testingSigner() *signingtest.Signer {
	return signingtest.NewSigner(&signing.ServiceInfo{
		ServiceAccountName: "signer@testing.host",
		AppID:              "unit-tests",
		AppVersion:         "mocked-ver",
	})
}

func TestBuildRulesQuery(t *testing.T) {
	t.Parallel()

	ctx := testingContext()

	ftt.Run("Happy path", t, func(t *ftt.Test) {
		q, err := buildRulesQuery(ctx, &minter.MintDelegationTokenRequest{
			DelegatedIdentity: "user:delegated@example.com",
			Audience:          []string{"group:A", "group:B", "user:c@example.com"},
			Services:          []string{"service:A", "*"},
		}, "user:requestor@example.com")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, q, should.NotBeNil)

		assert.Loosely(t, q.Requestor, should.Equal(identity.Identity("user:requestor@example.com")))
		assert.Loosely(t, q.Delegator, should.Equal(identity.Identity("user:delegated@example.com")))
		assert.Loosely(t, q.Audience.ToStrings(), should.Resemble([]string{"group:A", "group:B", "user:c@example.com"}))
		assert.Loosely(t, q.Services.ToStrings(), should.Resemble([]string{"*"}))
	})

	ftt.Run("REQUESTOR usage works", t, func(t *ftt.Test) {
		q, err := buildRulesQuery(ctx, &minter.MintDelegationTokenRequest{
			DelegatedIdentity: "REQUESTOR",
			Audience:          []string{"group:A", "group:B", "REQUESTOR"},
			Services:          []string{"*"},
		}, "user:requestor@example.com")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, q, should.NotBeNil)

		assert.Loosely(t, q.Requestor, should.Equal(identity.Identity("user:requestor@example.com")))
		assert.Loosely(t, q.Delegator, should.Equal(identity.Identity("user:requestor@example.com")))
		assert.Loosely(t, q.Audience.ToStrings(), should.Resemble([]string{"group:A", "group:B", "user:requestor@example.com"}))
	})

	ftt.Run("bad 'delegated_identity'", t, func(t *ftt.Test) {
		_, err := buildRulesQuery(ctx, &minter.MintDelegationTokenRequest{
			Audience: []string{"REQUESTOR"},
			Services: []string{"*"},
		}, "user:requestor@example.com")
		assert.Loosely(t, err, should.ErrLike(`'delegated_identity' is required`))

		_, err = buildRulesQuery(ctx, &minter.MintDelegationTokenRequest{
			DelegatedIdentity: "junk",
			Audience:          []string{"REQUESTOR"},
			Services:          []string{"*"},
		}, "user:requestor@example.com")
		assert.Loosely(t, err, should.ErrLike(`bad 'delegated_identity' - auth: bad identity string "junk"`))
	})

	ftt.Run("bad 'audience'", t, func(t *ftt.Test) {
		_, err := buildRulesQuery(ctx, &minter.MintDelegationTokenRequest{
			DelegatedIdentity: "REQUESTOR",
			Services:          []string{"*"},
		}, "user:requestor@example.com")
		assert.Loosely(t, err, should.ErrLike(`'audience' is required`))

		_, err = buildRulesQuery(ctx, &minter.MintDelegationTokenRequest{
			DelegatedIdentity: "REQUESTOR",
			Audience:          []string{"REQUESTOR", "junk"},
			Services:          []string{"*"},
		}, "user:requestor@example.com")
		assert.Loosely(t, err, should.ErrLike(`bad 'audience' - auth: bad identity string "junk"`))
	})

	ftt.Run("bad 'services'", t, func(t *ftt.Test) {
		_, err := buildRulesQuery(ctx, &minter.MintDelegationTokenRequest{
			DelegatedIdentity: "REQUESTOR",
			Audience:          []string{"REQUESTOR"},
		}, "user:requestor@example.com")
		assert.Loosely(t, err, should.ErrLike(`'services' is required`))

		_, err = buildRulesQuery(ctx, &minter.MintDelegationTokenRequest{
			DelegatedIdentity: "REQUESTOR",
			Audience:          []string{"REQUESTOR"},
			Services:          []string{"junk"},
		}, "user:requestor@example.com")
		assert.Loosely(t, err, should.ErrLike(`bad 'services' - auth: bad identity string "junk"`))

		_, err = buildRulesQuery(ctx, &minter.MintDelegationTokenRequest{
			DelegatedIdentity: "REQUESTOR",
			Audience:          []string{"REQUESTOR"},
			Services:          []string{"user:abc@example.com"},
		}, "user:requestor@example.com")
		assert.Loosely(t, err, should.ErrLike(`bad 'services' - "user:abc@example.com" is not a service ID`))

		_, err = buildRulesQuery(ctx, &minter.MintDelegationTokenRequest{
			DelegatedIdentity: "REQUESTOR",
			Audience:          []string{"REQUESTOR"},
			Services:          []string{"group:abc"},
		}, "user:requestor@example.com")
		assert.Loosely(t, err, should.ErrLike(`bad 'services' - can't specify groups`))
	})

	ftt.Run("resolves https:// service refs", t, func(t *ftt.Test) {
		q, err := buildRulesQuery(ctx, &minter.MintDelegationTokenRequest{
			DelegatedIdentity: "user:delegated@example.com",
			Audience:          []string{"*"},
			Services: []string{
				"service:A",
				"service:B",
				"https://C",
				"https://B",
				"https://A",
			},
		}, "user:requestor@example.com")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, q, should.NotBeNil)

		assert.Loosely(t, q.Services.ToStrings(), should.Resemble([]string{
			"service:A",
			"service:B",
			"service:C",
		}))
	})

	ftt.Run("handles errors when resolving https:// service refs", t, func(t *ftt.Test) {
		_, err := buildRulesQuery(ctx, &minter.MintDelegationTokenRequest{
			DelegatedIdentity: "user:delegated@example.com",
			Audience:          []string{"*"},
			Services: []string{
				"https://A",
				"https://B",
				"https://crash",
			},
		}, "user:requestor@example.com")
		assert.Loosely(t, err, should.ErrLike(`could not resolve "https://crash" to service ID - boom`))
	})
}

func TestMintDelegationToken(t *testing.T) {
	t.Parallel()

	ctx := testingContext()

	ftt.Run("with mocked config and state", t, func(t *ftt.Test) {
		cfg, err := loadConfig(ctx, `
			rules {
				name: "requstor for itself"
				requestor: "user:requestor@example.com"
				target_service: "*"
				allowed_to_impersonate: "REQUESTOR"
				allowed_audience: "REQUESTOR"
				max_validity_duration: 3600
			}
		`)
		assert.Loosely(t, err, should.BeNil)

		mintMock := func(c context.Context, p *mintParams) (*minter.MintDelegationTokenResponse, error) {
			return &minter.MintDelegationTokenResponse{Token: "valid_token", ServiceVersion: p.serviceVer}, nil
		}

		var loggedInfo *MintedTokenInfo
		rpc := MintDelegationTokenRPC{
			Signer: testingSigner(),
			Rules:  func(context.Context) (*Rules, error) { return cfg, nil },
			LogToken: func(c context.Context, i *MintedTokenInfo) error {
				loggedInfo = i
				return nil
			},
			mintMock: mintMock,
		}

		t.Run("Happy path", func(t *ftt.Test) {
			req := &minter.MintDelegationTokenRequest{
				DelegatedIdentity: "REQUESTOR",
				Audience:          []string{"REQUESTOR"},
				Services:          []string{"*"},
				Tags:              []string{"k:v"},
			}
			resp, err := rpc.MintDelegationToken(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.Token, should.Equal("valid_token"))
			assert.Loosely(t, resp.ServiceVersion, should.Equal("unit-tests/mocked-ver"))

			// LogToken called.
			assert.Loosely(t, loggedInfo.Request, should.Resemble(req))
			assert.Loosely(t, loggedInfo.Response, should.Resemble(resp))
			assert.Loosely(t, loggedInfo.ConfigRev, should.Equal(cfg.ConfigRevision()))
			assert.Loosely(t, loggedInfo.Rule, should.Resemble(&admin.DelegationRule{
				Name:                 "requstor for itself",
				Requestor:            []string{"user:requestor@example.com"},
				AllowedToImpersonate: []string{"REQUESTOR"},
				AllowedAudience:      []string{"REQUESTOR"},
				TargetService:        []string{"*"},
				MaxValidityDuration:  3600,
			}))
			assert.Loosely(t, loggedInfo.PeerIP, should.Resemble(net.ParseIP("127.10.10.10")))
			assert.Loosely(t, loggedInfo.RequestID, should.Equal(testingRequestID.String()))
			assert.Loosely(t, loggedInfo.AuthDBRev, should.Equal(1234))
		})

		t.Run("Using delegated identity for auth is forbidden", func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity:             "user:requestor@example.com",
				PeerIdentityOverride: "user:impersonator@example.com",
			})
			_, err := rpc.MintDelegationToken(ctx, &minter.MintDelegationTokenRequest{
				DelegatedIdentity: "REQUESTOR",
				Audience:          []string{"REQUESTOR"},
				Services:          []string{"*"},
			})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("delegation is forbidden for this API call"))
		})

		t.Run("Anonymous calls are forbidden", func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := rpc.MintDelegationToken(ctx, &minter.MintDelegationTokenRequest{
				DelegatedIdentity: "REQUESTOR",
				Audience:          []string{"REQUESTOR"},
				Services:          []string{"*"},
			})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCUnauthenticated)("authentication required"))
		})

		t.Run("Unauthorized requestor", func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:unknown@example.com",
			})
			_, err := rpc.MintDelegationToken(ctx, &minter.MintDelegationTokenRequest{
				DelegatedIdentity: "REQUESTOR",
				Audience:          []string{"REQUESTOR"},
				Services:          []string{"*"},
			})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("not authorized"))
		})

		t.Run("Negative validity duration", func(t *ftt.Test) {
			_, err := rpc.MintDelegationToken(ctx, &minter.MintDelegationTokenRequest{
				DelegatedIdentity: "REQUESTOR",
				Audience:          []string{"REQUESTOR"},
				Services:          []string{"*"},
				ValidityDuration:  -1,
			})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("bad request - invalid 'validity_duration' (-1)"))
		})

		t.Run("Bad tags", func(t *ftt.Test) {
			_, err := rpc.MintDelegationToken(ctx, &minter.MintDelegationTokenRequest{
				DelegatedIdentity: "REQUESTOR",
				Audience:          []string{"REQUESTOR"},
				Services:          []string{"*"},
				Tags:              []string{"not key value"},
			})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("bad request - invalid 'tags': tag #1: not in <key>:<value> form"))
		})

		t.Run("Malformed request", func(t *ftt.Test) {
			_, err := rpc.MintDelegationToken(ctx, &minter.MintDelegationTokenRequest{
				DelegatedIdentity: "REQUESTOR",
				Audience:          []string{"junk"},
				Services:          []string{"*"},
			})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)(`bad request - bad 'audience' - auth: bad identity string "junk"`))
		})

		t.Run("No matching rules", func(t *ftt.Test) {
			_, err := rpc.MintDelegationToken(ctx, &minter.MintDelegationTokenRequest{
				DelegatedIdentity: "REQUESTOR",
				Audience:          []string{"user:someone-else@example.com"},
				Services:          []string{"*"},
			})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("forbidden - no matching delegation rules in the config"))
		})

		t.Run("Forbidden validity duration", func(t *ftt.Test) {
			_, err := rpc.MintDelegationToken(ctx, &minter.MintDelegationTokenRequest{
				DelegatedIdentity: "REQUESTOR",
				Audience:          []string{"REQUESTOR"},
				Services:          []string{"*"},
				ValidityDuration:  3601,
			})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("forbidden - the requested validity duration (3601 sec) exceeds the maximum allowed one (3600 sec)"))
		})

	})
}
