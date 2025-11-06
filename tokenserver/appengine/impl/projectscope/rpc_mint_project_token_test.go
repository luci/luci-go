// Copyright 2019 The LUCI Authors.
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

package projectscope

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/signing/signingtest"

	"go.chromium.org/luci/tokenserver/api/minter/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/projectidentity"
)

var (
	authorizedGroups = []string{projectActorsGroup}
	testingRequestID = trace.TraceID{1, 2, 3, 4, 5}
)

func testMintAccessToken(ctx context.Context, params auth.MintAccessTokenParams) (*auth.Token, error) {
	return &auth.Token{
		Token:  "",
		Expiry: time.Now().UTC(),
	}, nil
}

func testingContext(caller identity.Identity) context.Context {
	ctx := gaetesting.TestingContext()
	ctx = logging.SetLevel(ctx, logging.Debug)
	ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: testingRequestID,
	}))
	ctx, _ = testclock.UseTime(ctx, testclock.TestTimeUTC)
	return auth.WithState(ctx, &authtest.FakeState{
		Identity:       caller,
		IdentityGroups: authorizedGroups,
	})
}

func newTestMintProjectTokenRPC() *MintProjectTokenRPC {
	rpc := MintProjectTokenRPC{
		Signer:            signingtest.NewSigner(nil),
		MintAccessToken:   testMintAccessToken,
		ProjectIdentities: projectidentity.ProjectIdentities,
	}
	return &rpc
}

func TestMintProjectToken(t *testing.T) {
	t.Parallel()
	ctx := testingContext("service@example.com")

	ftt.Run("initialize rpc handler", t, func(t *ftt.Test) {
		rpc := newTestMintProjectTokenRPC()

		t.Run("validateRequest works", func(t *ftt.Test) {
			t.Run("empty fields", func(t *ftt.Test) {
				req := &minter.MintProjectTokenRequest{
					LuciProject:         "",
					OauthScope:          []string{},
					MinValidityDuration: 7200,
				}
				_, err := rpc.MintProjectToken(ctx, req)
				assert.Loosely(t, err, should.NotBeNil)
			})

			t.Run("empty project", func(t *ftt.Test) {
				req := &minter.MintProjectTokenRequest{
					LuciProject:         "",
					OauthScope:          []string{scopes.CloudPlatform},
					MinValidityDuration: 1800,
				}
				_, err := rpc.MintProjectToken(ctx, req)
				assert.Loosely(t, err, should.ErrLike(`luci_project is empty`))
			})

			t.Run("empty scopes", func(t *ftt.Test) {
				req := &minter.MintProjectTokenRequest{
					LuciProject:         "foo-project",
					OauthScope:          []string{},
					MinValidityDuration: 1800,
				}

				_, err := rpc.MintProjectToken(ctx, req)
				assert.Loosely(t, err, should.ErrLike(`oauth_scope is required`))
			})

			t.Run("returns nil for valid request", func(t *ftt.Test) {
				req := &minter.MintProjectTokenRequest{
					LuciProject:         "test-project",
					OauthScope:          []string{scopes.CloudPlatform},
					MinValidityDuration: 3600,
				}
				_, err := rpc.MintProjectToken(ctx, req)
				assert.Loosely(t, err, should.ErrLike("min_validity_duration must not exceed 1800"))
			})
		})

		t.Run("MintProjectToken does not return errors with valid input", func(t *ftt.Test) {
			identity := &projectidentity.ProjectIdentity{Project: "service-project", Email: "foo@bar.com"}
			assert.NoErr(t, rpc.ProjectIdentities(ctx).Update(ctx, identity))

			req := &minter.MintProjectTokenRequest{
				LuciProject: "service-project",
				OauthScope:  []string{scopes.CloudPlatform},
			}
			resp, err := rpc.MintProjectToken(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.NotBeNil)
		})
	})
}
