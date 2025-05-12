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

package internal

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestCacheKey(t *testing.T) {
	t.Parallel()

	ftt.Run("ToMapKey empty", t, func(t *ftt.Test) {
		k := CacheKey{}
		assert.Loosely(t, k.ToMapKey(), should.Equal("\x00"))
	})

	ftt.Run("ToMapKey works", t, func(t *ftt.Test) {
		k := CacheKey{
			Key:    "a",
			Scopes: []string{"x", "y", "z"},
		}
		assert.Loosely(t, k.ToMapKey(), should.Equal("a\x00x\x00y\x00z\x00"))
	})

	ftt.Run("EqualCacheKeys works", t, func(t *ftt.Test) {
		assert.Loosely(t, equalCacheKeys(nil, nil), should.BeTrue)
		assert.Loosely(t, equalCacheKeys(&CacheKey{}, nil), should.BeFalse)
		assert.Loosely(t, equalCacheKeys(nil, &CacheKey{}), should.BeFalse)

		assert.Loosely(t, equalCacheKeys(
			&CacheKey{
				Key:    "k",
				Scopes: []string{"a", "b"},
			},
			&CacheKey{
				Key:    "k",
				Scopes: []string{"a", "b"},
			}),
			should.BeTrue)

		assert.Loosely(t, equalCacheKeys(
			&CacheKey{
				Key:    "k1",
				Scopes: []string{"a", "b"},
			},
			&CacheKey{
				Key:    "k2",
				Scopes: []string{"a", "b"},
			}),
			should.BeFalse)

		assert.Loosely(t, equalCacheKeys(
			&CacheKey{
				Key:    "k",
				Scopes: []string{"a1", "b"},
			},
			&CacheKey{
				Key:    "k",
				Scopes: []string{"a2", "b"},
			}),
			should.BeFalse)

		assert.Loosely(t, equalCacheKeys(
			&CacheKey{
				Key:    "k",
				Scopes: []string{"a"},
			},
			&CacheKey{
				Key:    "k",
				Scopes: []string{"a", "b"},
			}),
			should.BeFalse)
	})
}

func TestTokenHelpers(t *testing.T) {
	t.Parallel()

	ctx := mathrand.Set(context.Background(), rand.New(rand.NewSource(123)))
	ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeLocal)
	exp := testclock.TestRecentTimeLocal.Add(time.Hour)

	ftt.Run("TokenExpiresIn works", t, func(t *ftt.Test) {
		// Invalid tokens.
		assert.Loosely(t, TokenExpiresIn(ctx, nil, time.Minute), should.BeTrue)
		assert.Loosely(t, TokenExpiresIn(ctx, &Token{
			Token: oauth2.Token{
				AccessToken: "",
				Expiry:      exp,
			},
		}, time.Minute), should.BeTrue)

		// If expiry is not set, the token is non-expirable.
		assert.Loosely(t, TokenExpiresIn(ctx, &Token{
			Token: oauth2.Token{AccessToken: "abc"},
		}, 10*time.Hour), should.BeFalse)

		assert.Loosely(t, TokenExpiresIn(ctx, &Token{
			Token: oauth2.Token{
				AccessToken: "abc",
				Expiry:      exp,
			},
		}, time.Minute), should.BeFalse)

		tc.Add(59*time.Minute + 1*time.Second)

		assert.Loosely(t, TokenExpiresIn(ctx, &Token{
			Token: oauth2.Token{
				AccessToken: "abc",
				Expiry:      exp,
			},
		}, time.Minute), should.BeTrue)
	})

	ftt.Run("TokenExpiresInRnd works", t, func(t *ftt.Test) {
		// Invalid tokens.
		assert.Loosely(t, TokenExpiresInRnd(ctx, nil, time.Minute), should.BeTrue)
		assert.Loosely(t, TokenExpiresInRnd(ctx, &Token{
			Token: oauth2.Token{
				AccessToken: "",
				Expiry:      exp,
			},
		}, time.Minute), should.BeTrue)

		// If expiry is not set, the token is non-expirable.
		assert.Loosely(t, TokenExpiresInRnd(ctx, &Token{
			Token: oauth2.Token{AccessToken: "abc"},
		}, 10*time.Hour), should.BeFalse)

		// Generate a histogram of positive TokenExpiresInRnd responses per second,
		// for the duration of 10 min, assuming each TokenExpiresInRnd is called
		// 100 times per second.
		tokenLifetime := 5 * time.Minute
		requestedLifetime := time.Minute
		tok := &Token{
			Token: oauth2.Token{
				AccessToken: "abc",
				Expiry:      clock.Now(ctx).Add(tokenLifetime),
			},
		}
		hist := make([]int, 600)
		for s := 0; s < 600; s++ {
			for i := 0; i < 100; i++ {
				if TokenExpiresInRnd(ctx, tok, requestedLifetime) {
					hist[s]++
				}
				tc.Add(10 * time.Millisecond)
			}
		}

		// The histogram should have a shape:
		//   * 0 samples until somewhere around 3 min 55 sec
		//     (which is tokenLifetime - requestedLifetime - expiryRandInterval).
		//   * increasingly more non zero samples in the following
		//     expiryRandInterval seconds (3 min 55 sec - 4 min 00 sec).
		//   * 100% samples after tokenLifetime - requestedLifetime (> 4 min).
		firstNonZero := -1
		firstFull := -1
		for i, val := range hist {
			switch {
			case val != 0 && firstNonZero == -1:
				firstNonZero = i
			case val == 100 && firstFull == -1:
				firstFull = i
			}
		}

		// The first non-zero sample.
		assert.Loosely(t, time.Duration(firstNonZero)*time.Second,
			should.Equal(tokenLifetime-requestedLifetime-expiryRandInterval))
		// The first 100% sample.
		assert.Loosely(t, time.Duration(firstFull)*time.Second,
			should.Equal(tokenLifetime-requestedLifetime))
		// The in-between contains linearly increasing chance of early expiry, as
		// can totally be seen from this assertion.
		assert.Loosely(t, hist[firstNonZero:firstFull], should.Match([]int{
			6, 35, 55, 71, 93,
		}))
	})
}

func TestIsBadKeyError(t *testing.T) {
	ftt.Run("Correctly sniffs out bad key error", t, func(t *ftt.Test) {
		brokenKeyJSON := `{
			"type": "service_account",
			"project_id": "blah",
			"private_key_id": "zzzzzzzzzzzzzzzzzz",
			"private_key": "-----BEGIN PRIVATE KEY-----\nMIIEvgIBADANBgkqhBROKENq\nEpldRN3tNnLoLNy6\n-----END PRIVATE KEY-----\n",
			"client_email": "blah@blah.iam.gserviceaccount.com",
			"auth_uri": "https://accounts.google.com/o/oauth2/auth",
			"token_uri": "https://accounts.google.com/o/oauth2/token",
			"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs"
		}`
		cfg, err := google.JWTConfigFromJSON([]byte(brokenKeyJSON), "scope")
		assert.Loosely(t, err, should.BeNil)
		_, err = cfg.TokenSource(context.Background()).Token()
		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, isBadKeyError(err), should.BeTrue)
	})
}
