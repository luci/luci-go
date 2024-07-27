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

package authtest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/integration/localauth"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/lucictx"
)

func TestFakeTokenGenerator(t *testing.T) {
	t.Parallel()

	ftt.Run("With fakes", t, func(t *ftt.Test) {
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)

		gen := FakeTokenGenerator{KeepRecord: true}

		srv := localauth.Server{
			TokenGenerators: map[string]localauth.TokenGenerator{
				"authtest": &gen,
			},
			DefaultAccountID: "authtest",
		}
		la, err := srv.Start(ctx)
		assert.Loosely(t, err, should.BeNil)
		t.Cleanup(func() { srv.Stop(ctx) })
		ctx = lucictx.SetLocalAuth(ctx, la)

		t.Run("Access tokens", func(t *ftt.Test) {
			for idx, scope := range []string{"A", "B"} {
				auth := auth.NewAuthenticator(ctx, auth.SilentLogin, auth.Options{
					Scopes: []string{scope, "zzz"},
				})

				email, err := auth.GetEmail()
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, email, should.Equal(DefaultFakeEmail))

				tok, err := auth.GetAccessToken(time.Minute)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, tok.AccessToken, should.Equal(fmt.Sprintf("fake_token_%d", idx)))

				// Expiry is rounded to integer number of seconds, since that's the
				// granularity of OAuth token expiration. Compare int unix timestamps to
				// account for that.
				assert.Loosely(t, tok.Expiry.Unix(), should.Equal(
					testclock.TestRecentTimeUTC.Add(DefaultFakeLifetime).Unix()))
			}

			assert.Loosely(t, gen.TokenScopes("fake_token_0"), should.Resemble([]string{"A", "zzz"}))
			assert.Loosely(t, gen.TokenScopes("fake_token_1"), should.Resemble([]string{"B", "zzz"}))
		})

		t.Run("ID tokens", func(t *ftt.Test) {
			for idx, aud := range []string{"A", "B"} {
				auth := auth.NewAuthenticator(ctx, auth.SilentLogin, auth.Options{
					UseIDTokens: true,
					Audience:    aud,
				})

				email, err := auth.GetEmail()
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, email, should.Equal(DefaultFakeEmail))

				tok, err := auth.GetAccessToken(time.Minute)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, tok.AccessToken, should.Equal(fmt.Sprintf("fake_token_%d", idx)))

				// Expiry is rounded to integer number of seconds, since that's the
				// granularity of OAuth token expiration. Compare int unix timestamps to
				// account for that.
				assert.Loosely(t, tok.Expiry.Unix(), should.Equal(
					testclock.TestRecentTimeUTC.Add(DefaultFakeLifetime).Unix()))
			}

			assert.Loosely(t, gen.TokenScopes("fake_token_0"), should.Resemble([]string{"audience:A"}))
			assert.Loosely(t, gen.TokenScopes("fake_token_1"), should.Resemble([]string{"audience:B"}))
		})
	})
}
