// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package internal

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDiskTokenCache(t *testing.T) {
	t.Parallel()

	tmp, err := ioutil.TempDir("", "disk_token_cache")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)

	ctx := context.Background()
	ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeLocal)

	Convey("DiskTokenCache works", t, func() {
		// testCacheSemantics is in proc_cache_test.go.
		testCacheSemantics(ctx, &DiskTokenCache{
			Context:    ctx,
			SecretsDir: tmp,
		})
	})

	Convey("DiskTokenCache works (parallel)", t, func() {
		// testCacheInParallel is in proc_cache_test.go.
		//
		// Use real clock here to test real-world interaction when retrying disk
		// writes.
		ctx := context.Background()
		testCacheInParallel(ctx, &DiskTokenCache{
			Context:    ctx,
			SecretsDir: tmp,
		})
	})

	Convey("Cleans up old tokens", t, func() {
		cache := &DiskTokenCache{
			Context:    ctx,
			SecretsDir: tmp,
		}

		cache.PutToken(&CacheKey{Key: "a"}, &oauth2.Token{
			AccessToken: "abc",
			Expiry:      clock.Now(ctx),
		})
		cache.PutToken(&CacheKey{Key: "b"}, &oauth2.Token{
			AccessToken:  "abc",
			RefreshToken: "def",
			Expiry:       clock.Now(ctx),
		})

		// GCAccessTokenMaxAge later, "a" is gone while the cache is updated.
		tc.Add(GCAccessTokenMaxAge)
		unused := &oauth2.Token{
			AccessToken: "zzz",
			Expiry:      clock.Now(ctx).Add(365 * 24 * time.Hour),
		}
		cache.PutToken(&CacheKey{Key: "unused"}, unused)

		t, err := cache.GetToken(&CacheKey{Key: "a"})
		So(err, ShouldBeNil)
		So(t, ShouldBeNil)

		// "b" is still there.
		t, err = cache.GetToken(&CacheKey{Key: "b"})
		So(err, ShouldBeNil)
		So(t.RefreshToken, ShouldEqual, "def")

		// Some time later "b" is also removed.
		tc.Add(GCRefreshTokenMaxAge)
		cache.PutToken(&CacheKey{Key: "unused"}, unused)

		t, err = cache.GetToken(&CacheKey{Key: "b"})
		So(err, ShouldBeNil)
		So(t, ShouldBeNil)
	})
}
