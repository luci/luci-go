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

package gaemiddleware

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/caching"
)

func TestGlobalCache(t *testing.T) {
	t.Parallel()

	ftt.Parallel("Works", t, func(t *ftt.Test) {
		ctx := context.Background()
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = memory.Use(ctx)
		ctx = caching.WithGlobalCache(ctx, blobCacheProvider)

		cache := caching.GlobalCache(ctx, "namespace")

		// Cache miss.
		val, err := cache.Get(ctx, "key")
		assert.Loosely(t, err, should.Equal(caching.ErrCacheMiss))
		assert.Loosely(t, val, should.BeNil)
		assert.Loosely(t, cache.Set(ctx, "key_permanent", []byte("1"), 0), should.BeNil)
		assert.Loosely(t, cache.Set(ctx, "key_temp", []byte("2"), time.Minute), should.BeNil)

		// Cache hit.
		val, err = cache.Get(ctx, "key_permanent")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, val, should.Match([]byte("1")))

		val, err = cache.Get(ctx, "key_temp")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, val, should.Match([]byte("2")))

		// Expire one item.
		clock.Get(ctx).(testclock.TestClock).Add(2 * time.Minute)

		val, err = cache.Get(ctx, "key_permanent")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, val, should.Match([]byte("1")))

		// Expired!
		val, err = cache.Get(ctx, "key_temp")
		assert.Loosely(t, err, should.Equal(caching.ErrCacheMiss))
		assert.Loosely(t, val, should.BeNil)
	})
}
