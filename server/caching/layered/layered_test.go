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

package layered

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/cachingtest"
)

var testingCache = RegisterCache(Parameters[[]byte]{
	GlobalNamespace: "namespace",
	Marshal: func(item []byte) ([]byte, error) {
		return item, nil
	},
	Unmarshal: func(blob []byte) ([]byte, error) {
		return blob, nil
	},
})

var testingCacheWithFallback = RegisterCache(Parameters[[]byte]{
	GlobalNamespace: "namespace",
	Marshal: func(item []byte) ([]byte, error) {
		return item, nil
	},
	Unmarshal: func(blob []byte) ([]byte, error) {
		return blob, nil
	},
	AllowNoProcessCacheFallback: true,
})

func TestCache(t *testing.T) {
	t.Parallel()

	ftt.Run("Without process cache", t, func(t *ftt.Test) {
		ctx := context.Background()

		_, err := testingCache.GetOrCreate(ctx, "item", func() ([]byte, time.Duration, error) {
			panic("should not be called")
		})
		assert.Loosely(t, err, should.Equal(caching.ErrNoProcessCache))

		calls := 0
		_, err = testingCacheWithFallback.GetOrCreate(ctx, "item", func() ([]byte, time.Duration, error) {
			calls += 1
			return nil, 0, nil
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, calls, should.Equal(1))
	})

	ftt.Run("With fake time", t, func(t *ftt.Test) {
		ctx := context.Background()
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(12345)))
		ctx, tc := testclock.UseTime(ctx, time.Date(2017, time.January, 1, 0, 0, 0, 0, time.UTC))
		ctx = caching.WithEmptyProcessCache(ctx)

		calls := 0
		value := []byte("value")
		anotherValue := []byte("anotherValue")

		getter := func() ([]byte, time.Duration, error) {
			calls++
			return value, time.Hour, nil
		}

		t.Run("Without global cache", func(t *ftt.Test) {
			item, err := testingCache.GetOrCreate(ctx, "item", getter)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, item, should.Match(value))
			assert.Loosely(t, calls, should.Equal(1))

			tc.Add(59 * time.Minute)

			item, err = testingCache.GetOrCreate(ctx, "item", getter)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, item, should.Match(value))
			assert.Loosely(t, calls, should.Equal(1)) // no new calls

			tc.Add(2 * time.Minute) // cached item expires

			item, err = testingCache.GetOrCreate(ctx, "item", getter)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, item, should.Match(value))
			assert.Loosely(t, calls, should.Equal(2)) // new call!
		})

		t.Run("With global cache", func(t *ftt.Test) {
			global := cachingtest.NewBlobCache()
			ctx = cachingtest.WithGlobalCache(ctx, map[string]caching.BlobCache{
				"namespace": global,
			})

			t.Run("Getting from the global cache", func(t *ftt.Test) {
				// The global cache is empty.
				assert.Loosely(t, global.LRU.Len(), should.BeZero)

				// Create an item.
				item, err := testingCache.GetOrCreate(ctx, "item", getter)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, item, should.Match(value))
				assert.Loosely(t, calls, should.Equal(1))

				// It is in the global cache now.
				assert.Loosely(t, global.LRU.Len(), should.Equal(1))

				// Clear the local cache.
				ctx = caching.WithEmptyProcessCache(ctx)

				// Grab the item again. Will be fetched from the global cache.
				item, err = testingCache.GetOrCreate(ctx, "item", getter)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, item, should.Match(value))
				assert.Loosely(t, calls, should.Equal(1)) // no new calls
			})

			t.Run("Broken global cache is ignored", func(t *ftt.Test) {
				global.Err = errors.New("broken")

				// Create an item.
				item, err := testingCache.GetOrCreate(ctx, "item", getter)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, item, should.Match(value))
				assert.Loosely(t, calls, should.Equal(1))

				// Clear the local cache.
				ctx = caching.WithEmptyProcessCache(ctx)

				// Grab the item again. Will be recreated again, since the global cache
				// is broken.
				item, err = testingCache.GetOrCreate(ctx, "item", getter)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, item, should.Match(value))
				assert.Loosely(t, calls, should.Equal(2)) // new call!
			})
		})

		t.Run("Never expiring item", func(t *ftt.Test) {
			item, err := testingCache.GetOrCreate(ctx, "item", func() ([]byte, time.Duration, error) {
				return value, 0, nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, item, should.Match(value))

			tc.Add(100 * time.Hour)

			item, err = testingCache.GetOrCreate(ctx, "item", func() ([]byte, time.Duration, error) {
				return nil, 0, errors.New("must not be called")
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, item, should.Match(value))
		})

		t.Run("WithMinTTL works", func(t *ftt.Test) {
			item, err := testingCache.GetOrCreate(ctx, "item", func() ([]byte, time.Duration, error) {
				return value, time.Hour, nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, item, should.Match(value))

			tc.Add(50 * time.Minute)

			// 9 min minTTL is still ok.
			item, err = testingCache.GetOrCreate(ctx, "item", func() ([]byte, time.Duration, error) {
				return nil, 0, errors.New("must not be called")
			}, WithMinTTL(9*time.Minute))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, item, should.Match(value))

			// But 10 min is not and the item is refreshed.
			item, err = testingCache.GetOrCreate(ctx, "item", func() ([]byte, time.Duration, error) {
				return anotherValue, time.Hour, nil
			}, WithMinTTL(10*time.Minute))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, item, should.Match(anotherValue))
		})

		t.Run("ErrCantSatisfyMinTTL", func(t *ftt.Test) {
			_, err := testingCache.GetOrCreate(ctx, "item", func() ([]byte, time.Duration, error) {
				return value, time.Minute, nil
			}, WithMinTTL(2*time.Minute))
			assert.Loosely(t, err, should.Equal(ErrCantSatisfyMinTTL))
		})

		oneRandomizedTrial := func(now, threshold time.Duration) (cacheHit bool) {
			// Reset state (except RNG).
			ctx := caching.WithEmptyProcessCache(ctx)
			ctx, tc := testclock.UseTime(ctx, time.Date(2017, time.January, 1, 0, 0, 0, 0, time.UTC))

			// Put the item in the cache.
			item, err := testingCache.GetOrCreate(ctx, "item", func() ([]byte, time.Duration, error) {
				return value, time.Hour, nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, item, should.Match(value))

			tc.Add(now)

			// Grab it (or trigger a refresh if randomly expired).
			item, err = testingCache.GetOrCreate(ctx, "item", func() ([]byte, time.Duration, error) {
				return anotherValue, time.Hour, nil
			}, WithRandomizedExpiration(threshold))
			assert.Loosely(t, err, should.BeNil)
			return bytes.Equal(item, value)
		}

		testCases := []struct {
			now, threshold  time.Duration
			expectedHitRate int
		}{
			{50 * time.Minute, 10 * time.Minute, 100}, // before threshold, no random expiration
			{51 * time.Minute, 10 * time.Minute, 90},  // slightly above => some expiration
			{59 * time.Minute, 10 * time.Minute, 11},  // almost at the threshold => heavy expiration
			{61 * time.Minute, 10 * time.Minute, 0},   // outside item expiration => always expired
		}

		for i := 0; i < len(testCases); i++ {
			now := testCases[i].now
			threshold := testCases[i].threshold
			expectedHitRate := testCases[i].expectedHitRate

			t.Run(fmt.Sprintf("WithRandomizedExpiration (now = %s)", now), func(t *ftt.Test) {
				cacheHits := 0
				for i := 0; i < 100; i++ {
					if oneRandomizedTrial(now, threshold) {
						cacheHits++
					}
				}
				assert.Loosely(t, cacheHits, should.Equal(expectedHitRate))
			})
		}
	})
}

func TestSerialization(t *testing.T) {
	t.Parallel()

	ftt.Run("With cache", t, func(t *ftt.Test) {
		now := time.Date(2017, time.January, 1, 0, 0, 0, 0, time.UTC)

		c := Cache[[]byte]{
			params: Parameters[[]byte]{
				Marshal: func(item []byte) ([]byte, error) {
					return item, nil
				},
				Unmarshal: func(blob []byte) ([]byte, error) {
					return blob, nil
				},
			},
		}

		t.Run("Happy path with deadline", func(t *ftt.Test) {
			originalItem := itemWithExp[[]byte]{[]byte("blah-blah"), now}

			blob, err := c.serializeItem(&originalItem)
			assert.Loosely(t, err, should.BeNil)

			item, err := c.deserializeItem(blob)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, item.exp.Equal(now), should.BeTrue)
			assert.Loosely(t, item.val, should.Match(originalItem.val))
		})

		t.Run("Happy path without deadline", func(t *ftt.Test) {
			originalItem := itemWithExp[[]byte]{[]byte("blah-blah"), time.Time{}}

			blob, err := c.serializeItem(&originalItem)
			assert.Loosely(t, err, should.BeNil)

			item, err := c.deserializeItem(blob)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, item.exp.IsZero(), should.BeTrue)
			assert.Loosely(t, item.val, should.Match(originalItem.val))
		})

		t.Run("Marshal error", func(t *ftt.Test) {
			fail := errors.New("failure")
			c.params.Marshal = func(item []byte) ([]byte, error) {
				return nil, fail
			}
			_, err := c.serializeItem(&itemWithExp[[]byte]{})
			assert.Loosely(t, err, should.Equal(fail))
		})

		t.Run("Small buffer in Unmarshal", func(t *ftt.Test) {
			_, err := c.deserializeItem([]byte{formatVersionByte, 0})
			assert.Loosely(t, err, should.ErrLike("buffer is too small"))
		})

		t.Run("Bad version in Unmarshal", func(t *ftt.Test) {
			_, err := c.deserializeItem([]byte{formatVersionByte + 1, 0, 0, 0, 0, 0, 0, 0, 0})
			assert.Loosely(t, err, should.ErrLike("bad format version"))
		})

		t.Run("Unmarshal error", func(t *ftt.Test) {
			fail := errors.New("failure")
			c.params.Unmarshal = func(blob []byte) ([]byte, error) {
				return nil, fail
			}

			blob, err := c.serializeItem(&itemWithExp[[]byte]{[]byte("blah-blah"), now})
			assert.Loosely(t, err, should.BeNil)

			_, err = c.deserializeItem(blob)
			assert.Loosely(t, err, should.Equal(fail))
		})
	})
}
