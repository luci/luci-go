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

	. "github.com/smartystreets/goconvey/convey"
)

func TestCache(t *testing.T) {
	t.Parallel()

	Convey(`An locking LRU cache with size heuristic 3`, t, func() {
		ctx := context.Background()
		cache := New(3)

		Convey(`A Get() returns nil.`, func() {
			_, has := cache.Get(ctx, "test")
			So(has, ShouldBeFalse)
		})

		// Adds values to the cache sequentially, blocking on the values being
		// processed.
		addCacheValues := func(values ...string) {
			for _, v := range values {
				_, isPresent := cache.Peek(ctx, v)
				_, has := cache.Put(ctx, v, v+"v", 0)
				So(has, ShouldEqual, isPresent)
			}
		}

		get := func(key interface{}) (val interface{}) {
			val, _ = cache.Get(ctx, key)
			return
		}

		Convey(`With three values, {a, b, c}`, func() {
			addCacheValues("a", "b", "c")
			So(cache.Len(), ShouldEqual, 3)

			Convey(`Prune does nothing.`, func() {
				cache.Prune(ctx)
				So(cache.Len(), ShouldEqual, 3)
			})

			Convey(`Is empty after a reset.`, func() {
				cache.Reset()
				So(cache.Len(), ShouldEqual, 0)
			})

			Convey(`Can retrieve each of those values.`, func() {
				So(get("a"), ShouldEqual, "av")
				So(get("b"), ShouldEqual, "bv")
				So(get("c"), ShouldEqual, "cv")
			})

			Convey(`Get()ting "a", then adding "d" will cause "b" to be evicted.`, func() {
				So(get("a"), ShouldEqual, "av")
				addCacheValues("d")
				So(cache, shouldHaveValues, "a", "c", "d")
			})

			Convey(`Peek()ing "a", then adding "d" will cause "a" to be evicted.`, func() {
				v, has := cache.Peek(ctx, "a")
				So(has, ShouldBeTrue)
				So(v, ShouldEqual, "av")

				v, has = cache.Peek(ctx, "nonexist")
				So(has, ShouldBeFalse)
				So(v, ShouldBeNil)

				addCacheValues("d")
				So(cache, shouldHaveValues, "b", "c", "d")
			})
		})

		Convey(`When adding {a, b, c, d}, "a" will be evicted.`, func() {
			addCacheValues("a", "b", "c", "d")
			So(cache.Len(), ShouldEqual, 3)

			So(cache, shouldHaveValues, "b", "c", "d")

			Convey(`Requests for "a" will be nil.`, func() {
				So(get("a"), ShouldBeNil)
			})
		})

		Convey(`When adding {a, b, c, a, d}, "b" will be evicted.`, func() {
			addCacheValues("a", "b", "c", "a", "d")
			So(cache.Len(), ShouldEqual, 3)

			So(cache, shouldHaveValues, "a", "c", "d")

			Convey(`When removing "c", will contain {a, d}.`, func() {
				v, had := cache.Remove("c")
				So(had, ShouldBeTrue)
				So(v, ShouldEqual, "cv")
				So(cache, shouldHaveValues, "a", "d")

				Convey(`When adding {e, f}, "a" will be evicted.`, func() {
					addCacheValues("e", "f")
					So(cache, shouldHaveValues, "d", "e", "f")
				})
			})
		})

		Convey(`When removing a value that isn't there, returns nil.`, func() {
			v, has := cache.Remove("foo")
			So(has, ShouldBeFalse)
			So(v, ShouldBeNil)
		})
	})
}

func TestCacheWithExpiry(t *testing.T) {
	t.Parallel()

	Convey(`A cache of size 3 with a Clock`, t, func() {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestTimeUTC)
		cache := New(3)

		cache.Put(ctx, "a", "av", 1*time.Second)
		cache.Put(ctx, "b", "bv", 2*time.Second)
		cache.Put(ctx, "forever", "foreverv", 0)

		Convey(`When "a" is expired`, func() {
			tc.Add(time.Second)

			Convey(`Get doesn't yield "a", but yields "b".`, func() {
				_, has := cache.Get(ctx, "a")
				So(has, ShouldBeFalse)

				_, has = cache.Get(ctx, "b")
				So(has, ShouldBeTrue)
			})

			Convey(`Mutate treats "a" as missing.`, func() {
				v, ok := cache.Mutate(ctx, "a", func(it *Item) *Item {
					So(it, ShouldBeNil)
					return nil
				})
				So(ok, ShouldBeFalse)
				So(v, ShouldBeNil)
				So(cache.Len(), ShouldEqual, 2)

				_, has := cache.Get(ctx, "a")
				So(has, ShouldBeFalse)
			})

			Convey(`Mutate replaces "a" if a value is supplied.`, func() {
				v, ok := cache.Mutate(ctx, "a", func(it *Item) *Item {
					So(it, ShouldBeNil)
					return &Item{"av", 0}
				})
				So(ok, ShouldBeTrue)
				So(v, ShouldEqual, "av")
				So(cache, shouldHaveValues, "a", "b", "forever")

				v, has := cache.Get(ctx, "a")
				So(has, ShouldBeTrue)
				So(v, ShouldEqual, "av")
			})

			Convey(`Mutateing "b" yields the remaining time.`, func() {
				v, ok := cache.Mutate(ctx, "b", func(it *Item) *Item {
					So(it, ShouldResemble, &Item{"bv", 1 * time.Second})
					return it
				})
				So(ok, ShouldBeTrue)
				So(v, ShouldEqual, "bv")

				v, has := cache.Get(ctx, "b")
				So(has, ShouldBeTrue)
				So(v, ShouldEqual, "bv")

				tc.Add(time.Second)

				_, has = cache.Get(ctx, "b")
				So(has, ShouldBeFalse)
			})
		})

		Convey(`Prune prunes all expired entries.`, func() {
			tc.Add(1 * time.Hour)
			cache.Prune(ctx)
			So(cache, shouldHaveValues, "forever")
		})
	})
}

func TestUnboundedCache(t *testing.T) {
	t.Parallel()

	Convey(`An unbounded cache`, t, func() {
		ctx := context.Background()
		cache := New(0)

		Convey(`Grows indefinitely`, func() {
			for i := 0; i < 1000; i++ {
				cache.Put(ctx, i, "hey", 0)
			}
			So(cache.Len(), ShouldEqual, 1000)
		})

		Convey(`Grows indefinitely even if elements have an (ignored) expiry`, func() {
			for i := 0; i < 1000; i++ {
				cache.Put(ctx, i, "hey", time.Second)
			}
			So(cache.Len(), ShouldEqual, 1000)

			cache.Prune(ctx)
			So(cache.Len(), ShouldEqual, 1000)
		})
	})
}

func TestUnboundedCacheWithExpiry(t *testing.T) {
	t.Parallel()

	Convey(`An unbounded cache with a clock`, t, func() {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestTimeUTC)
		cache := New(0)

		Convey(`Grows indefinitely`, func() {
			for i := 0; i < 1000; i++ {
				cache.Put(ctx, i, "hey", 0)
			}
			So(cache.Len(), ShouldEqual, 1000)

			cache.Prune(ctx)
			So(cache.Len(), ShouldEqual, 1000)
		})

		Convey(`Grows indefinitely even if elements have an (ignored) expiry`, func() {
			for i := 1; i <= 1000; i++ {
				cache.Put(ctx, i, "hey", time.Duration(i)*time.Second)
			}
			So(cache.Len(), ShouldEqual, 1000)

			// Expire the first half of entries.
			tc.Add(500 * time.Second)

			Convey(`Get works`, func() {
				v, has := cache.Get(ctx, 1)
				So(has, ShouldBeFalse)
				So(v, ShouldBeNil)

				v, has = cache.Get(ctx, 500)
				So(has, ShouldBeFalse)
				So(v, ShouldBeNil)

				v, has = cache.Get(ctx, 501)
				So(has, ShouldBeTrue)
				So(v, ShouldEqual, "hey")
			})

			Convey(`Len works`, func() {
				// Without explicit pruning, Len includes expired elements.
				So(cache.Len(), ShouldEqual, 1000)

				// After pruning, Len is accurate again.
				cache.Prune(ctx)
				So(cache.Len(), ShouldEqual, 500)
			})
		})
	})
}

func TestGetOrCreate(t *testing.T) {
	t.Parallel()

	Convey(`An unbounded cache`, t, func() {
		ctx := context.Background()
		cache := New(0)

		Convey(`Can create a new value, and will synchronize around that creation`, func() {
			v, err := cache.GetOrCreate(ctx, "foo", func() (interface{}, time.Duration, error) {
				return "bar", 0, nil
			})
			So(err, ShouldBeNil)
			So(v, ShouldEqual, "bar")

			v, ok := cache.Get(ctx, "foo")
			So(ok, ShouldBeTrue)
			So(v, ShouldEqual, "bar")
		})

		Convey(`Will not retain a value if an error is returned.`, func() {
			errWat := errors.New("wat")
			v, err := cache.GetOrCreate(ctx, "foo", func() (interface{}, time.Duration, error) {
				return nil, 0, errWat
			})
			So(err, ShouldEqual, errWat)
			So(v, ShouldBeNil)

			_, ok := cache.Get(ctx, "foo")
			So(ok, ShouldBeFalse)
		})

		Convey(`Will call Maker in series, even with multiple callers, and lock individually.`, func(cc C) {
			const count = 16
			const contention = 16

			var wg sync.WaitGroup
			vals := make([]int, count)
			for i := 0; i < count; i++ {
				for j := 0; j < contention; j++ {
					i := i
					wg.Add(1)
					go func(cctx C) {
						defer wg.Done()
						v, err := cache.GetOrCreate(ctx, i, func() (interface{}, time.Duration, error) {
							val := vals[i]
							vals[i]++
							return val, 0, nil
						})
						cc.So(v, ShouldEqual, 0)
						cc.So(err, ShouldBeNil)
					}(cc)
				}
			}

			wg.Wait()
			for i := 0; i < count; i++ {
				v, ok := cache.Get(ctx, i)
				So(ok, ShouldBeTrue)
				So(v, ShouldEqual, 0)
				So(vals[i], ShouldEqual, 1)
			}
		})

		Convey(`Can retrieve values while a Maker is in-progress.`, func() {
			cache.Put(ctx, "foo", "bar", 0)

			// Value already exists, so retrieves current value.
			v, err := cache.GetOrCreate(ctx, "foo", func() (interface{}, time.Duration, error) {
				return "baz", 0, nil
			})
			So(err, ShouldBeNil)
			So(v, ShouldEqual, "bar")

			// Create a new value.
			changingC := make(chan struct{})
			waitC := make(chan struct{})
			doneC := make(chan struct{})

			var setV interface{}
			var setErr error
			go func() {
				setV, setErr = cache.Create(ctx, "foo", func() (interface{}, time.Duration, error) {
					close(changingC)
					<-waitC
					return "qux", 0, nil
				})

				close(doneC)
			}()

			// The goroutine's Create is in-progress, but the value is still present,
			// so we should be able to get the old value.
			<-changingC
			v, err = cache.GetOrCreate(ctx, "foo", func() (interface{}, time.Duration, error) {
				return "never", 0, nil
			})
			So(err, ShouldBeNil)
			So(v, ShouldEqual, "bar")

			// Our goroutine has finished setting. Validate its output.
			close(waitC)
			<-doneC

			So(setErr, ShouldBeNil)
			So(setV, ShouldEqual, "qux")

			// Run GetOrCreate. The value should be present, and should hold the new
			// value added by the goroutine.
			v, err = cache.GetOrCreate(ctx, "foo", func() (interface{}, time.Duration, error) {
				return "never", 0, nil
			})
			So(err, ShouldBeNil)
			So(v, ShouldEqual, "qux")
		})
	})
}

func shouldHaveValues(actual interface{}, expected ...interface{}) string {
	cache := actual.(*Cache)

	actualSnapshot := cache.snapshot()

	expectedSnapshot := snapshot{}
	for _, k := range expected {
		expectedSnapshot[k] = k.(string) + "v"
	}
	return ShouldResemble(actualSnapshot, expectedSnapshot)
}
