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

package lru

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestCache(t *testing.T) {
	t.Parallel()

	ftt.Run(`An locking LRU cache with size heuristic 3`, t, func(t *ftt.Test) {
		ctx := context.Background()
		cache := New[string, string](3)

		t.Run(`A Get() returns "no item".`, func(t *ftt.Test) {
			_, has := cache.Get(ctx, "test")
			assert.Loosely(t, has, should.BeFalse)
		})

		// Adds values to the cache sequentially, blocking on the values being
		// processed.
		addCacheValues := func(values ...string) {
			for _, v := range values {
				_, isPresent := cache.Peek(ctx, v)
				_, has := cache.Put(ctx, v, v+"v", 0)
				assert.Loosely(t, has, should.Equal(isPresent))
			}
		}

		get := func(key string) (val string) {
			val, _ = cache.Get(ctx, key)
			return
		}

		t.Run(`With three values, {a, b, c}`, func(t *ftt.Test) {
			addCacheValues("a", "b", "c")
			assert.Loosely(t, cache.Len(), should.Equal(3))

			t.Run(`Prune does nothing.`, func(t *ftt.Test) {
				cache.Prune(ctx)
				assert.Loosely(t, cache.Len(), should.Equal(3))
			})

			t.Run(`Is empty after a reset.`, func(t *ftt.Test) {
				cache.Reset()
				assert.Loosely(t, cache.Len(), should.BeZero)
			})

			t.Run(`Can retrieve each of those values.`, func(t *ftt.Test) {
				assert.Loosely(t, get("a"), should.Equal("av"))
				assert.Loosely(t, get("b"), should.Equal("bv"))
				assert.Loosely(t, get("c"), should.Equal("cv"))
			})

			t.Run(`Get()ting "a", then adding "d" will cause "b" to be evicted.`, func(t *ftt.Test) {
				assert.Loosely(t, get("a"), should.Equal("av"))
				addCacheValues("d")
				assert.That(t, cache, shouldHaveValues("a", "c", "d"))
			})

			t.Run(`Peek()ing "a", then adding "d" will cause "a" to be evicted.`, func(t *ftt.Test) {
				v, has := cache.Peek(ctx, "a")
				assert.Loosely(t, has, should.BeTrue)
				assert.Loosely(t, v, should.Equal("av"))

				v, has = cache.Peek(ctx, "nonexist")
				assert.Loosely(t, has, should.BeFalse)
				assert.Loosely(t, v, should.BeEmpty)

				addCacheValues("d")
				assert.That(t, cache, shouldHaveValues("b", "c", "d"))
			})
		})

		t.Run(`When adding {a, b, c, d}, "a" will be evicted.`, func(t *ftt.Test) {
			addCacheValues("a", "b", "c", "d")
			assert.Loosely(t, cache.Len(), should.Equal(3))

			assert.That(t, cache, shouldHaveValues("b", "c", "d"))

			t.Run(`Requests for "a" will be empty.`, func(t *ftt.Test) {
				assert.Loosely(t, get("a"), should.BeEmpty)
			})
		})

		t.Run(`When adding {a, b, c, a, d}, "b" will be evicted.`, func(t *ftt.Test) {
			addCacheValues("a", "b", "c", "a", "d")
			assert.Loosely(t, cache.Len(), should.Equal(3))

			assert.That(t, cache, shouldHaveValues("a", "c", "d"))

			t.Run(`When removing "c", will contain {a, d}.`, func(t *ftt.Test) {
				v, had := cache.Remove("c")
				assert.Loosely(t, had, should.BeTrue)
				assert.Loosely(t, v, should.Equal("cv"))
				assert.That(t, cache, shouldHaveValues("a", "d"))

				t.Run(`When adding {e, f}, "a" will be evicted.`, func(t *ftt.Test) {
					addCacheValues("e", "f")
					assert.That(t, cache, shouldHaveValues("d", "e", "f"))
				})
			})
		})

		t.Run(`When removing a value that isn't there, returns "no item".`, func(t *ftt.Test) {
			v, has := cache.Remove("foo")
			assert.Loosely(t, has, should.BeFalse)
			assert.Loosely(t, v, should.BeEmpty)
		})
	})
}

func TestCacheWithExpiry(t *testing.T) {
	t.Parallel()

	ftt.Run(`A cache of size 3 with a Clock`, t, func(t *ftt.Test) {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestTimeUTC)
		cache := New[string, string](3)

		cache.Put(ctx, "a", "av", 1*time.Second)
		cache.Put(ctx, "b", "bv", 2*time.Second)
		cache.Put(ctx, "forever", "foreverv", 0)

		t.Run(`When "a" is expired`, func(t *ftt.Test) {
			tc.Add(time.Second)

			t.Run(`Get doesn't yield "a", but yields "b".`, func(t *ftt.Test) {
				_, has := cache.Get(ctx, "a")
				assert.Loosely(t, has, should.BeFalse)

				_, has = cache.Get(ctx, "b")
				assert.Loosely(t, has, should.BeTrue)
			})

			t.Run(`Mutate treats "a" as missing.`, func(t *ftt.Test) {
				v, ok := cache.Mutate(ctx, "a", func(it *Item[string]) *Item[string] {
					assert.Loosely(t, it, should.BeNil)
					return nil
				})
				assert.Loosely(t, ok, should.BeFalse)
				assert.Loosely(t, v, should.BeEmpty)
				assert.Loosely(t, cache.Len(), should.Equal(2))

				_, has := cache.Get(ctx, "a")
				assert.Loosely(t, has, should.BeFalse)
			})

			t.Run(`Mutate replaces "a" if a value is supplied.`, func(t *ftt.Test) {
				v, ok := cache.Mutate(ctx, "a", func(it *Item[string]) *Item[string] {
					assert.Loosely(t, it, should.BeNil)
					return &Item[string]{"av", 0}
				})
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, v, should.Equal("av"))
				assert.Loosely(t, cache, shouldHaveValues("a", "b", "forever"))

				v, has := cache.Get(ctx, "a")
				assert.Loosely(t, has, should.BeTrue)
				assert.Loosely(t, v, should.Equal("av"))
			})

			t.Run(`Mutateing "b" yields the remaining time.`, func(t *ftt.Test) {
				v, ok := cache.Mutate(ctx, "b", func(it *Item[string]) *Item[string] {
					assert.Loosely(t, it, should.Resemble(&Item[string]{"bv", 1 * time.Second}))
					return it
				})
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, v, should.Equal("bv"))

				v, has := cache.Get(ctx, "b")
				assert.Loosely(t, has, should.BeTrue)
				assert.Loosely(t, v, should.Equal("bv"))

				tc.Add(time.Second)

				_, has = cache.Get(ctx, "b")
				assert.Loosely(t, has, should.BeFalse)
			})
		})

		t.Run(`Prune prunes all expired entries.`, func(t *ftt.Test) {
			tc.Add(1 * time.Hour)
			cache.Prune(ctx)
			assert.Loosely(t, cache, shouldHaveValues("forever"))
		})
	})
}

func TestUnboundedCache(t *testing.T) {
	t.Parallel()

	ftt.Run(`An unbounded cache`, t, func(t *ftt.Test) {
		ctx := context.Background()
		cache := New[int, string](0)

		t.Run(`Grows indefinitely`, func(t *ftt.Test) {
			for i := 0; i < 1000; i++ {
				cache.Put(ctx, i, "hey", 0)
			}
			assert.Loosely(t, cache.Len(), should.Equal(1000))
		})

		t.Run(`Grows indefinitely even if elements have an (ignored) expiry`, func(t *ftt.Test) {
			for i := 0; i < 1000; i++ {
				cache.Put(ctx, i, "hey", time.Second)
			}
			assert.Loosely(t, cache.Len(), should.Equal(1000))

			cache.Prune(ctx)
			assert.Loosely(t, cache.Len(), should.Equal(1000))
		})
	})
}

func TestUnboundedCacheWithExpiry(t *testing.T) {
	t.Parallel()

	ftt.Run(`An unbounded cache with a clock`, t, func(t *ftt.Test) {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestTimeUTC)
		cache := New[int, string](0)

		t.Run(`Grows indefinitely`, func(t *ftt.Test) {
			for i := 0; i < 1000; i++ {
				cache.Put(ctx, i, "hey", 0)
			}
			assert.Loosely(t, cache.Len(), should.Equal(1000))

			cache.Prune(ctx)
			assert.Loosely(t, cache.Len(), should.Equal(1000))
		})

		t.Run(`Grows indefinitely even if elements have an (ignored) expiry`, func(t *ftt.Test) {
			for i := 1; i <= 1000; i++ {
				cache.Put(ctx, i, "hey", time.Duration(i)*time.Second)
			}
			assert.Loosely(t, cache.Len(), should.Equal(1000))

			// Expire the first half of entries.
			tc.Add(500 * time.Second)

			t.Run(`Get works`, func(t *ftt.Test) {
				v, has := cache.Get(ctx, 1)
				assert.Loosely(t, has, should.BeFalse)
				assert.Loosely(t, v, should.BeEmpty)

				v, has = cache.Get(ctx, 500)
				assert.Loosely(t, has, should.BeFalse)
				assert.Loosely(t, v, should.BeEmpty)

				v, has = cache.Get(ctx, 501)
				assert.Loosely(t, has, should.BeTrue)
				assert.Loosely(t, v, should.Equal("hey"))
			})

			t.Run(`Len works`, func(t *ftt.Test) {
				// Without explicit pruning, Len includes expired elements.
				assert.Loosely(t, cache.Len(), should.Equal(1000))

				// After pruning, Len is accurate again.
				cache.Prune(ctx)
				assert.Loosely(t, cache.Len(), should.Equal(500))
			})
		})
	})
}

func TestGetOrCreate(t *testing.T) {
	t.Parallel()

	ftt.Run(`An unbounded cache`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`Can create a new value, and will synchronize around that creation`, func(t *ftt.Test) {
			cache := New[string, string](0)

			v, err := cache.GetOrCreate(ctx, "foo", func() (string, time.Duration, error) {
				return "bar", 0, nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v, should.Equal("bar"))

			v, ok := cache.Get(ctx, "foo")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, v, should.Equal("bar"))
		})

		t.Run(`Will not retain a value if an error is returned.`, func(t *ftt.Test) {
			cache := New[string, string](0)

			errWat := errors.New("wat")
			v, err := cache.GetOrCreate(ctx, "foo", func() (string, time.Duration, error) {
				return "", 0, errWat
			})
			assert.Loosely(t, err, should.Equal(errWat))
			assert.Loosely(t, v, should.BeEmpty)

			_, ok := cache.Get(ctx, "foo")
			assert.Loosely(t, ok, should.BeFalse)
		})

		t.Run(`Will call Maker in series, even with multiple callers, and lock individually.`, func(t *ftt.Test) {
			const count = 16
			const contention = 16

			cache := New[int, int](0)

			var wg sync.WaitGroup
			vals := make([]int, count)
			for i := 0; i < count; i++ {
				for j := 0; j < contention; j++ {
					i := i
					wg.Add(1)
					go func(t testing.TB) {
						defer wg.Done()
						v, err := cache.GetOrCreate(ctx, i, func() (int, time.Duration, error) {
							val := vals[i]
							vals[i]++
							return val, 0, nil
						})
						assert.Loosely(t, v, should.BeZero)
						assert.Loosely(t, err, should.BeNil)
					}(t)
				}
			}

			wg.Wait()
			for i := 0; i < count; i++ {
				v, ok := cache.Get(ctx, i)
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, v, should.BeZero)
				assert.Loosely(t, vals[i], should.Equal(1))
			}
		})

		t.Run(`Can retrieve values while a Maker is in-progress.`, func(t *ftt.Test) {
			cache := New[string, string](0)
			cache.Put(ctx, "foo", "bar", 0)

			// Value already exists, so retrieves current value.
			v, err := cache.GetOrCreate(ctx, "foo", func() (string, time.Duration, error) {
				return "baz", 0, nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v, should.Equal("bar"))

			// Create a new value.
			changingC := make(chan struct{})
			waitC := make(chan struct{})
			doneC := make(chan struct{})

			var setV any
			var setErr error
			go func() {
				setV, setErr = cache.Create(ctx, "foo", func() (string, time.Duration, error) {
					close(changingC)
					<-waitC
					return "qux", 0, nil
				})

				close(doneC)
			}()

			// The goroutine's Create is in-progress, but the value is still present,
			// so we should be able to get the old value.
			<-changingC
			v, err = cache.GetOrCreate(ctx, "foo", func() (string, time.Duration, error) {
				return "never", 0, nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v, should.Equal("bar"))

			// Our goroutine has finished setting. Validate its output.
			close(waitC)
			<-doneC

			assert.Loosely(t, setErr, should.BeNil)
			assert.Loosely(t, setV, should.Equal("qux"))

			// Run GetOrCreate. The value should be present, and should hold the new
			// value added by the goroutine.
			v, err = cache.GetOrCreate(ctx, "foo", func() (string, time.Duration, error) {
				return "never", 0, nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v, should.Equal("qux"))
		})
	})
}

func shouldHaveValues(expected ...string) comparison.Func[*Cache[string, string]] {
	expectedSnapshot := map[string]string{}
	for _, k := range expected {
		expectedSnapshot[k] = k + "v"
	}

	return func(actual *Cache[string, string]) *failure.Summary {
		actualSnapshot := map[string]string{}
		for k, e := range actual.cache {
			actualSnapshot[k] = e.v
		}
		return should.Match(expectedSnapshot)(actualSnapshot)
	}
}
