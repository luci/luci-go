// Copyright 2015 The LUCI Authors.
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

package client

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/caching"
)

func TestGetAccessToken(t *testing.T) {
	ftt.Run("GetAccessToken works", t, func(t *ftt.Test) {
		ctx := testContext()

		// Getting initial token.
		ctx = mockAccessTokenRPC(ctx, t, []string{"A", "B"}, "access_token_1", testclock.TestRecentTimeUTC.Add(time.Hour))
		tok, err := GetAccessToken(ctx, []string{"B", "B", "A"})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tok, should.Resemble(&oauth2.Token{
			AccessToken: "access_token_1",
			TokenType:   "Bearer",
			Expiry:      testclock.TestRecentTimeUTC.Add(time.Hour).Add(-expirationMinLifetime),
		}))

		// Some time later same cached token is used.
		clock.Get(ctx).(testclock.TestClock).Add(30 * time.Minute)

		ctx = mockAccessTokenRPC(ctx, t, []string{"A", "B"}, "access_token_none", testclock.TestRecentTimeUTC.Add(time.Hour))
		tok, err = GetAccessToken(ctx, []string{"B", "B", "A"})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tok, should.Resemble(&oauth2.Token{
			AccessToken: "access_token_1",
			TokenType:   "Bearer",
			Expiry:      testclock.TestRecentTimeUTC.Add(time.Hour).Add(-expirationMinLifetime),
		}))

		// Closer to expiration, the token is updated, at some random invocation,
		// (depends on the seed, defines the loop limit in the test).
		clock.Get(ctx).(testclock.TestClock).Add(26 * time.Minute)
		for i := 0; ; i++ {
			ctx = mockAccessTokenRPC(ctx, t, []string{"A", "B"}, fmt.Sprintf("access_token_%d", i+2), testclock.TestRecentTimeUTC.Add(2*time.Hour))
			tok, err = GetAccessToken(ctx, []string{"B", "B", "A"})
			assert.Loosely(t, err, should.BeNil)
			if tok.AccessToken != "access_token_1" {
				break // got refreshed token!
			}
			assert.Loosely(t, i, should.BeLessThan(1000)) // the test is hanging, this means randomization doesn't work
		}
		assert.Loosely(t, tok, should.Resemble(&oauth2.Token{
			AccessToken: "access_token_3",
			TokenType:   "Bearer",
			Expiry:      testclock.TestRecentTimeUTC.Add(2 * time.Hour).Add(-expirationMinLifetime),
		}))

		// No randomization for token that are long expired.
		clock.Get(ctx).(testclock.TestClock).Add(2 * time.Hour)
		ctx = mockAccessTokenRPC(ctx, t, []string{"A", "B"}, "access_token_new", testclock.TestRecentTimeUTC.Add(5*time.Hour))
		tok, err = GetAccessToken(ctx, []string{"B", "B", "A"})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tok.AccessToken, should.Equal("access_token_new"))
	})
}

type mockedInfo struct {
	info.RawInterface

	t testing.TB

	scopes []string
	tok    string
	exp    time.Time
}

func (m *mockedInfo) AccessToken(scopes ...string) (string, time.Time, error) {
	assert.Loosely(m.t, scopes, should.Resemble(m.scopes))
	return m.tok, m.exp, nil
}

func testContext() context.Context {
	ctx := context.Background()
	ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
	ctx = memory.Use(ctx)
	ctx = mathrand.Set(ctx, rand.New(rand.NewSource(2)))
	ctx = caching.WithEmptyProcessCache(ctx)
	return ctx
}

func mockAccessTokenRPC(ctx context.Context, t testing.TB, scopes []string, tok string, exp time.Time) context.Context {
	return info.AddFilters(ctx, func(ci context.Context, i info.RawInterface) info.RawInterface {
		return &mockedInfo{i, t, scopes, tok, exp}
	})
}
