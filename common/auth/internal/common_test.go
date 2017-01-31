// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package internal

import (
	"math/rand"
	"testing"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/data/rand/mathrand"
	. "github.com/smartystreets/goconvey/convey"
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
		So(EqualCacheKeys(nil, nil), ShouldBeTrue)
		So(EqualCacheKeys(&CacheKey{}, nil), ShouldBeFalse)
		So(EqualCacheKeys(nil, &CacheKey{}), ShouldBeFalse)

		So(EqualCacheKeys(
			&CacheKey{
				Key:    "k",
				Scopes: []string{"a", "b"},
			},
			&CacheKey{
				Key:    "k",
				Scopes: []string{"a", "b"},
			}),
			ShouldBeTrue)

		So(EqualCacheKeys(
			&CacheKey{
				Key:    "k1",
				Scopes: []string{"a", "b"},
			},
			&CacheKey{
				Key:    "k2",
				Scopes: []string{"a", "b"},
			}),
			ShouldBeFalse)

		So(EqualCacheKeys(
			&CacheKey{
				Key:    "k",
				Scopes: []string{"a1", "b"},
			},
			&CacheKey{
				Key:    "k",
				Scopes: []string{"a2", "b"},
			}),
			ShouldBeFalse)

		So(EqualCacheKeys(
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
		So(TokenExpiresIn(ctx, &oauth2.Token{
			AccessToken: "",
			Expiry:      exp,
		}, time.Minute), ShouldBeTrue)

		// If expiry is not set, the token is non-expirable.
		So(TokenExpiresIn(ctx, &oauth2.Token{AccessToken: "abc"}, 10*time.Hour), ShouldBeFalse)

		So(TokenExpiresIn(ctx, &oauth2.Token{
			AccessToken: "abc",
			Expiry:      exp,
		}, time.Minute), ShouldBeFalse)

		tc.Add(59*time.Minute + 1*time.Second)

		So(TokenExpiresIn(ctx, &oauth2.Token{
			AccessToken: "abc",
			Expiry:      exp,
		}, time.Minute), ShouldBeTrue)
	})

	Convey("TokenExpiresInRnd works", t, func() {
		// Invalid tokens.
		So(TokenExpiresInRnd(ctx, nil, time.Minute), ShouldBeTrue)
		So(TokenExpiresInRnd(ctx, &oauth2.Token{
			AccessToken: "",
			Expiry:      exp,
		}, time.Minute), ShouldBeTrue)

		// If expiry is not set, the token is non-expirable.
		So(TokenExpiresInRnd(ctx, &oauth2.Token{AccessToken: "abc"}, 10*time.Hour), ShouldBeFalse)

		// Generate a histogram of positive TokenExpiresInRnd responses per second,
		// for the duration of 10 min, assuming each TokenExpiresInRnd is called
		// 100 times per second.
		tokenLifetime := 5 * time.Minute
		requestedLifetime := time.Minute
		tok := &oauth2.Token{
			AccessToken: "abc",
			Expiry:      clock.Now(ctx).Add(tokenLifetime),
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
