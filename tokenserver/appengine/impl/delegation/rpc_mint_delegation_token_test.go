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
	"fmt"
	"net"
	"net/url"
	"testing"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/identity"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"
	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/api/minter/v1"

	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

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
	ctx = info.GetTestable(ctx).SetRequestID("gae-request-id")
	ctx, _ = testclock.UseTime(ctx, time.Date(2015, time.February, 3, 4, 5, 6, 0, time.UTC))
	return auth.WithState(ctx, &authtest.FakeState{
		Identity:       "user:requestor@example.com",
		PeerIPOverride: net.ParseIP("127.10.10.10"),
		FakeDB:         &authdb.SnapshotDB{Rev: 1234},
	})
}

func testingSigner() signing.Signer {
	return signingtest.NewSigner(0, &signing.ServiceInfo{
		ServiceAccountName: "signer@testing.host",
		AppID:              "unit-tests",
		AppVersion:         "mocked-ver",
	})
}

func TestBuildRulesQuery(t *testing.T) {
	t.Parallel()

	ctx := testingContext()

	Convey("Happy path", t, func() {
		q, err := buildRulesQuery(ctx, &minter.MintDelegationTokenRequest{
			DelegatedIdentity: "user:delegated@example.com",
			Audience:          []string{"group:A", "group:B", "user:c@example.com"},
			Services:          []string{"service:A", "*"},
		}, "user:requestor@example.com")
		So(err, ShouldBeNil)
		So(q, ShouldNotBeNil)

		So(q.Requestor, ShouldEqual, "user:requestor@example.com")
		So(q.Delegator, ShouldEqual, "user:delegated@example.com")
		So(q.Audience.ToStrings(), ShouldResemble, []string{"group:A", "group:B", "user:c@example.com"})
		So(q.Services.ToStrings(), ShouldResemble, []string{"*"})
	})

	Convey("REQUESTOR usage works", t, func() {
		q, err := buildRulesQuery(ctx, &minter.MintDelegationTokenRequest{
			DelegatedIdentity: "REQUESTOR",
			Audience:          []string{"group:A", "group:B", "REQUESTOR"},
			Services:          []string{"*"},
		}, "user:requestor@example.com")
		So(err, ShouldBeNil)
		So(q, ShouldNotBeNil)

		So(q.Requestor, ShouldEqual, "user:requestor@example.com")
		So(q.Delegator, ShouldEqual, "user:requestor@example.com")
		So(q.Audience.ToStrings(), ShouldResemble, []string{"group:A", "group:B", "user:requestor@example.com"})
	})

	Convey("bad 'delegated_identity'", t, func() {
		_, err := buildRulesQuery(ctx, &minter.MintDelegationTokenRequest{
			Audience: []string{"REQUESTOR"},
			Services: []string{"*"},
		}, "user:requestor@example.com")
		So(err, ShouldErrLike, `'delegated_identity' is required`)

		_, err = buildRulesQuery(ctx, &minter.MintDelegationTokenRequest{
			DelegatedIdentity: "junk",
			Audience:          []string{"REQUESTOR"},
			Services:          []string{"*"},
		}, "user:requestor@example.com")
		So(err, ShouldErrLike, `bad 'delegated_identity' - auth: bad identity string "junk"`)
	})

	Convey("bad 'audience'", t, func() {
		_, err := buildRulesQuery(ctx, &minter.MintDelegationTokenRequest{
			DelegatedIdentity: "REQUESTOR",
			Services:          []string{"*"},
		}, "user:requestor@example.com")
		So(err, ShouldErrLike, `'audience' is required`)

		_, err = buildRulesQuery(ctx, &minter.MintDelegationTokenRequest{
			DelegatedIdentity: "REQUESTOR",
			Audience:          []string{"REQUESTOR", "junk"},
			Services:          []string{"*"},
		}, "user:requestor@example.com")
		So(err, ShouldErrLike, `bad 'audience' - auth: bad identity string "junk"`)
	})

	Convey("bad 'services'", t, func() {
		_, err := buildRulesQuery(ctx, &minter.MintDelegationTokenRequest{
			DelegatedIdentity: "REQUESTOR",
			Audience:          []string{"REQUESTOR"},
		}, "user:requestor@example.com")
		So(err, ShouldErrLike, `'services' is required`)

		_, err = buildRulesQuery(ctx, &minter.MintDelegationTokenRequest{
			DelegatedIdentity: "REQUESTOR",
			Audience:          []string{"REQUESTOR"},
			Services:          []string{"junk"},
		}, "user:requestor@example.com")
		So(err, ShouldErrLike, `bad 'services' - auth: bad identity string "junk"`)

		_, err = buildRulesQuery(ctx, &minter.MintDelegationTokenRequest{
			DelegatedIdentity: "REQUESTOR",
			Audience:          []string{"REQUESTOR"},
			Services:          []string{"user:abc@example.com"},
		}, "user:requestor@example.com")
		So(err, ShouldErrLike, `bad 'services' - "user:abc@example.com" is not a service ID`)

		_, err = buildRulesQuery(ctx, &minter.MintDelegationTokenRequest{
			DelegatedIdentity: "REQUESTOR",
			Audience:          []string{"REQUESTOR"},
			Services:          []string{"group:abc"},
		}, "user:requestor@example.com")
		So(err, ShouldErrLike, `bad 'services' - can't specify groups`)
	})

	Convey("resolves https:// service refs", t, func() {
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
		So(err, ShouldBeNil)
		So(q, ShouldNotBeNil)

		So(q.Services.ToStrings(), ShouldResemble, []string{
			"service:A",
			"service:B",
			"service:C",
		})
	})

	Convey("handles errors when resolving https:// service refs", t, func() {
		_, err := buildRulesQuery(ctx, &minter.MintDelegationTokenRequest{
			DelegatedIdentity: "user:delegated@example.com",
			Audience:          []string{"*"},
			Services: []string{
				"https://A",
				"https://B",
				"https://crash",
			},
		}, "user:requestor@example.com")
		So(err, ShouldErrLike, `could not resolve "https://crash" to service ID - boom`)
	})
}

func TestMintDelegationToken(t *testing.T) {
	t.Parallel()

	ctx := testingContext()

	Convey("with mocked config and state", t, func() {
		cfg, err := loadConfig(`
			rules {
				name: "requstor for itself"
				requestor: "user:requestor@example.com"
				target_service: "*"
				allowed_to_impersonate: "REQUESTOR"
				allowed_audience: "REQUESTOR"
				max_validity_duration: 3600
			}
		`)
		So(err, ShouldBeNil)

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

		Convey("Happy path", func() {
			req := &minter.MintDelegationTokenRequest{
				DelegatedIdentity: "REQUESTOR",
				Audience:          []string{"REQUESTOR"},
				Services:          []string{"*"},
			}
			resp, err := rpc.MintDelegationToken(ctx, req)
			So(err, ShouldBeNil)
			So(resp.Token, ShouldEqual, "valid_token")
			So(resp.ServiceVersion, ShouldEqual, "unit-tests/mocked-ver")

			// LogToken called.
			So(loggedInfo, ShouldResemble, &MintedTokenInfo{
				Request:   req,
				Response:  resp,
				ConfigRev: cfg.ConfigRevision(),
				Rule: &admin.DelegationRule{
					Name:                 "requstor for itself",
					Requestor:            []string{"user:requestor@example.com"},
					AllowedToImpersonate: []string{"REQUESTOR"},
					AllowedAudience:      []string{"REQUESTOR"},
					TargetService:        []string{"*"},
					MaxValidityDuration:  3600,
				},
				PeerIP:    net.ParseIP("127.10.10.10"),
				RequestID: "gae-request-id",
				AuthDBRev: 1234,
			})
		})

		Convey("Using delegated identity for auth is forbidden", func() {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity:             "user:requestor@example.com",
				PeerIdentityOverride: "user:impersonator@example.com",
			})
			_, err := rpc.MintDelegationToken(ctx, &minter.MintDelegationTokenRequest{
				DelegatedIdentity: "REQUESTOR",
				Audience:          []string{"REQUESTOR"},
				Services:          []string{"*"},
			})
			So(err, ShouldBeRPCPermissionDenied, "delegation is forbidden for this API call")
		})

		Convey("Anonymous calls are forbidden", func() {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := rpc.MintDelegationToken(ctx, &minter.MintDelegationTokenRequest{
				DelegatedIdentity: "REQUESTOR",
				Audience:          []string{"REQUESTOR"},
				Services:          []string{"*"},
			})
			So(err, ShouldBeRPCUnauthenticated, "authentication required")
		})

		Convey("Unauthorized requestor", func() {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:unknown@example.com",
			})
			_, err := rpc.MintDelegationToken(ctx, &minter.MintDelegationTokenRequest{
				DelegatedIdentity: "REQUESTOR",
				Audience:          []string{"REQUESTOR"},
				Services:          []string{"*"},
			})
			So(err, ShouldBeRPCPermissionDenied, "not authorized")
		})

		Convey("Negative validity duration", func() {
			_, err := rpc.MintDelegationToken(ctx, &minter.MintDelegationTokenRequest{
				DelegatedIdentity: "REQUESTOR",
				Audience:          []string{"REQUESTOR"},
				Services:          []string{"*"},
				ValidityDuration:  -1,
			})
			So(err, ShouldBeRPCInvalidArgument, "bad request - invalid 'validity_duration' (-1)")
		})

		Convey("Malformed request", func() {
			_, err := rpc.MintDelegationToken(ctx, &minter.MintDelegationTokenRequest{
				DelegatedIdentity: "REQUESTOR",
				Audience:          []string{"junk"},
				Services:          []string{"*"},
			})
			So(err, ShouldBeRPCInvalidArgument, `bad request - bad 'audience' - auth: bad identity string "junk"`)
		})

		Convey("No matching rules", func() {
			_, err := rpc.MintDelegationToken(ctx, &minter.MintDelegationTokenRequest{
				DelegatedIdentity: "REQUESTOR",
				Audience:          []string{"user:someone-else@example.com"},
				Services:          []string{"*"},
			})
			So(err, ShouldBeRPCPermissionDenied, "forbidden - no matching delegation rules in the config")
		})

		Convey("Forbidden validity duration", func() {
			_, err := rpc.MintDelegationToken(ctx, &minter.MintDelegationTokenRequest{
				DelegatedIdentity: "REQUESTOR",
				Audience:          []string{"REQUESTOR"},
				Services:          []string{"*"},
				ValidityDuration:  3601,
			})
			So(err, ShouldBeRPCPermissionDenied, "forbidden - the requested validity duration (3601 sec) exceeds the maximum allowed one (3600 sec)")
		})

	})
}
