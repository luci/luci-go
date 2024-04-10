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

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
)

func TestCacheKey(t *testing.T) {
	t.Parallel()

	Convey("ToMapKey empty", t, func() {
		k := CacheKey{}
		So(k.ToMapKey(), ShouldEqual, "\x00")
	})

	Convey("ToMapKey works", t, func() {
		k := CacheKey{
			Key:    "a",
			Scopes: []string{"x", "y", "z"},
		}
		So(k.ToMapKey(), ShouldEqual, "a\x00x\x00y\x00z\x00")
	})

	Convey("EqualCacheKeys works", t, func() {
		So(equalCacheKeys(nil, nil), ShouldBeTrue)
		So(equalCacheKeys(&CacheKey{}, nil), ShouldBeFalse)
		So(equalCacheKeys(nil, &CacheKey{}), ShouldBeFalse)

		So(equalCacheKeys(
			&CacheKey{
				Key:    "k",
				Scopes: []string{"a", "b"},
			},
			&CacheKey{
				Key:    "k",
				Scopes: []string{"a", "b"},
			}),
			ShouldBeTrue)

		So(equalCacheKeys(
			&CacheKey{
				Key:    "k1",
				Scopes: []string{"a", "b"},
			},
			&CacheKey{
				Key:    "k2",
				Scopes: []string{"a", "b"},
			}),
			ShouldBeFalse)

		So(equalCacheKeys(
			&CacheKey{
				Key:    "k",
				Scopes: []string{"a1", "b"},
			},
			&CacheKey{
				Key:    "k",
				Scopes: []string{"a2", "b"},
			}),
			ShouldBeFalse)

		So(equalCacheKeys(
			&CacheKey{
				Key:    "k",
				Scopes: []string{"a"},
			},
			&CacheKey{
				Key:    "k",
				Scopes: []string{"a", "b"},
			}),
			ShouldBeFalse)
	})
}

func TestTokenHelpers(t *testing.T) {
	t.Parallel()

	ctx := mathrand.Set(context.Background(), rand.New(rand.NewSource(123)))
	ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeLocal)
	exp := testclock.TestRecentTimeLocal.Add(time.Hour)

	Convey("TokenExpiresIn works", t, func() {
		// Invalid tokens.
		So(TokenExpiresIn(ctx, nil, time.Minute), ShouldBeTrue)
		So(TokenExpiresIn(ctx, &Token{
			Token: oauth2.Token{
				AccessToken: "",
				Expiry:      exp,
			},
		}, time.Minute), ShouldBeTrue)

		// If expiry is not set, the token is non-expirable.
		So(TokenExpiresIn(ctx, &Token{
			Token: oauth2.Token{AccessToken: "abc"},
		}, 10*time.Hour), ShouldBeFalse)

		So(TokenExpiresIn(ctx, &Token{
			Token: oauth2.Token{
				AccessToken: "abc",
				Expiry:      exp,
			},
		}, time.Minute), ShouldBeFalse)

		tc.Add(59*time.Minute + 1*time.Second)

		So(TokenExpiresIn(ctx, &Token{
			Token: oauth2.Token{
				AccessToken: "abc",
				Expiry:      exp,
			},
		}, time.Minute), ShouldBeTrue)
	})

	Convey("TokenExpiresInRnd works", t, func() {
		// Invalid tokens.
		So(TokenExpiresInRnd(ctx, nil, time.Minute), ShouldBeTrue)
		So(TokenExpiresInRnd(ctx, &Token{
			Token: oauth2.Token{
				AccessToken: "",
				Expiry:      exp,
			},
		}, time.Minute), ShouldBeTrue)

		// If expiry is not set, the token is non-expirable.
		So(TokenExpiresInRnd(ctx, &Token{
			Token: oauth2.Token{AccessToken: "abc"},
		}, 10*time.Hour), ShouldBeFalse)

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
		//   * 0 samples until somewhere around 3 min 30 sec
		//     (which is tokenLifetime - requestedLifetime - expiryRandInterval).
		//   * increasingly more non zero samples in the following
		//     expiryRandInterval seconds (3 min 30 sec - 4 min 00 sec).
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

		// The first non-zero sample is at 3 min 30 sec (+1, by chance).
		So(firstNonZero, ShouldEqual, 3*60+30+1)
		// The first 100% sample is at 4 min (-1, by chance).
		So(firstFull, ShouldEqual, 4*60-1)
		// The in-between contains linearly increasing chance of early expiry, as
		// can totally be seen from this assertion.
		So(hist[firstNonZero:firstFull], ShouldResemble, []int{
			8, 6, 8, 23, 20, 27, 28, 29, 25, 36, 43, 41, 53, 49,
			49, 57, 59, 62, 69, 69, 76, 72, 82, 73, 85, 83, 93, 93,
		})
	})
}

func TestIsBadKeyError(t *testing.T) {
	Convey("Correctly sniffs out bad key error", t, func() {
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
		So(err, ShouldBeNil)
		_, err = cfg.TokenSource(context.Background()).Token()
		So(err, ShouldNotBeNil)
		So(isBadKeyError(err), ShouldBeTrue)
	})
}
