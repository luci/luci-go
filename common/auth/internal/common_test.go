// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package internal

import (
	"testing"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"github.com/luci/luci-go/common/clock/testclock"
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

	ctx := context.Background()
	ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeLocal)
	exp := testclock.TestRecentTimeLocal.Add(time.Hour)

	Convey("TokenExpiresIn works", t, func() {
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
}
