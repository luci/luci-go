// Copyright 2025 The LUCI Authors.
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

package breadbox

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestBreadbox(t *testing.T) {
	t.Parallel()

	maxStaleness := 5 * time.Minute
	testTime := time.Date(2023, 2, 1, 0, 0, 0, 0, time.UTC)

	ftt.Run("Breadbox.Get works", t, func(t *ftt.Test) {
		ctx := context.Background()
		ctx, tc := testclock.UseTime(ctx, testTime)

		t.Run("Blocking mode works", func(t *ftt.Test) {
			lock := sync.Mutex{}
			counter := 0

			refresher := func(_ context.Context, prev any) (any, error) {
				lock.Lock()
				defer lock.Unlock()
				counter++
				return counter, nil
			}

			box := &Breadbox{}

			// Initial fetch.
			assert.Loosely(t, box.current, should.BeNil)
			v, err := box.Get(ctx, maxStaleness, refresher)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v.(int), should.Equal(1))

			// Still fresh enough.
			tc.Add(maxStaleness - 1)
			v, err = box.Get(ctx, maxStaleness, refresher)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v.(int), should.Equal(1))

			// Refreshed when cache is as stale as maxStaleness.
			tc.Add(1)
			v, err = box.Get(ctx, maxStaleness, refresher)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v.(int), should.Equal(2))
		})

		t.Run("Initial failed fetch returns an error", func(t *ftt.Test) {
			box := &Breadbox{}

			// Initial failed fetch.
			failErr := errors.New("fail")
			_, err := box.Get(ctx, maxStaleness, func(rCtx context.Context, prev any) (any, error) {
				return nil, failErr
			})
			assert.Loosely(t, err, should.Equal(failErr))

			// Subsequent successful fetch.
			val, err := box.Get(ctx, maxStaleness, func(rCtx context.Context, prev any) (any, error) {
				return 1, nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, val, should.Equal(1))
		})

		t.Run("Returns stale copy while fetching", func(t *ftt.Test) {
			// Put initial value.
			box := &Breadbox{}
			v, err := box.Get(ctx, maxStaleness, func(_ context.Context, prev any) (any, error) {
				return 1, nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v.(int), should.Equal(1))

			fetching := make(chan bool)
			resume := make(chan bool)
			fetcher := func(_ context.Context, prev any) (any, error) {
				fetching <- true
				<-resume
				return 2, nil
			}

			// Make it expire. Start blocking fetch of the new value.
			tc.Add(maxStaleness)
			wg := sync.WaitGroup{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				v, err := box.Get(ctx, maxStaleness, fetcher)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, v.(int), should.Equal(2))
			}()

			// Wait until we hit the body of the fetcher callback.
			<-fetching

			// Concurrent Get() returns stale copy right away (does not
			// deadlock).
			v, err = box.Get(ctx, maxStaleness, fetcher)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v.(int), should.Equal(1))

			// Wait until another goroutine finishes the fetch.
			resume <- true
			wg.Wait()

			// Returns new value now.
			v, err = box.Get(ctx, maxStaleness, fetcher)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v.(int), should.Equal(2))
		})

		t.Run("Recovers from panic", func(t *ftt.Test) {
			// Initial value.
			box := &Breadbox{}
			v, err := box.Get(ctx, maxStaleness, func(_ context.Context, prev any) (any, error) {
				return 1, nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v.(int), should.Equal(1))

			// Make it expire. Start panicking fetch.
			tc.Add(maxStaleness)
			assert.Loosely(t, func() {
				box.Get(ctx, maxStaleness, func(_ context.Context, prev any) (any, error) {
					panic("oh dear")
				})
			}, should.PanicLikeString("oh dear"))

			// Doesn't deadlock.
			v, err = box.Get(ctx, maxStaleness, func(_ context.Context, prev any) (any, error) {
				return 2, nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v.(int), should.Equal(2))
		})

		t.Run("Nil value is allowed", func(t *ftt.Test) {
			// Initial nil value.
			box := &Breadbox{}
			v, err := box.Get(ctx, maxStaleness, func(_ context.Context, prev any) (any, error) {
				return nil, nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v, should.BeNil)

			// Some time later this nil expires and we fetch something else.
			tc.Add(maxStaleness)
			v, err = box.Get(ctx, maxStaleness, func(_ context.Context, prev any) (any, error) {
				assert.Loosely(t, prev, should.BeNil)
				return 1, nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v.(int), should.Equal(1))
		})

		t.Run("Retries failed refetch later", func(t *ftt.Test) {
			var errorToReturn error
			var valueToReturn int

			fetchCalls := 0
			fetcher := func(_ context.Context, prev any) (any, error) {
				fetchCalls++
				return valueToReturn, errorToReturn
			}

			box := &Breadbox{}
			// Set the max staleness to a minute.
			maxStaleness = time.Minute

			// Initial fetch.
			valueToReturn = 1
			errorToReturn = nil
			v, err := box.Get(ctx, maxStaleness, fetcher)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v.(int), should.Equal(1))
			assert.Loosely(t, fetchCalls, should.Equal(1))

			// Cached copy is good after 30 sec.
			tc.Add(30 * time.Second)
			valueToReturn = 2
			errorToReturn = nil
			v, err = box.Get(ctx, maxStaleness, fetcher)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v.(int), should.Equal(1)) // still cached copy
			assert.Loosely(t, fetchCalls, should.Equal(1))

			// After 30 more seconds, the cache copy expires, we attempt to
			// update it, but something goes horribly wrong. Get(...) returns
			// the old copy.
			tc.Add(30 * time.Second)
			valueToReturn = 3
			errorToReturn = errors.New("oh dear")
			v, err = box.Get(ctx, maxStaleness, fetcher)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v.(int), should.Equal(1))    // still cached copy
			assert.Loosely(t, fetchCalls, should.Equal(2)) // fetch attempted

			// 1 sec later still using old copy, because retry is scheduled for
			// later.
			tc.Add(time.Second)
			valueToReturn = 4
			errorToReturn = nil
			v, err = box.Get(ctx, maxStaleness, fetcher)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v.(int), should.Equal(1)) // still cached copy
			assert.Loosely(t, fetchCalls, should.Equal(2))

			// 5 seconds later, fetch is attempted and it succeeds.
			tc.Add(5 * time.Second)
			valueToReturn = 5
			errorToReturn = nil
			v, err = box.Get(ctx, maxStaleness, fetcher)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v.(int), should.Equal(5)) // new copy
			assert.Loosely(t, fetchCalls, should.Equal(3))
		})
	})

	ftt.Run("Breadbox.GetFresh works", t, func(t *ftt.Test) {
		ctx := context.Background()
		ctx, tc := testclock.UseTime(ctx, testTime)

		t.Run("Refresher always called", func(t *ftt.Test) {
			lock := sync.Mutex{}
			counter := 0

			refresher := func(_ context.Context, prev any) (any, error) {
				lock.Lock()
				defer lock.Unlock()
				counter++
				return counter, nil
			}

			box := &Breadbox{}

			// Initial fetch.
			assert.Loosely(t, box.current, should.BeNil)
			v, err := box.GetFresh(ctx, refresher)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v.(int), should.Equal(1))

			// Refreshed, even when no time has passed.
			v, err = box.GetFresh(ctx, refresher)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v.(int), should.Equal(2))
		})

		t.Run("Initial failed fetch returns an error", func(t *ftt.Test) {
			box := &Breadbox{}

			// Initial failed fetch.
			failErr := errors.New("fail")
			_, err := box.GetFresh(ctx, func(_ context.Context, prev any) (any, error) {
				return nil, failErr
			})
			assert.Loosely(t, err, should.Equal(failErr))

			// Subsequent successful fetch.
			val, err := box.GetFresh(ctx, func(_ context.Context, prev any) (any, error) {
				return 1, nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, val, should.Equal(1))
		})

		t.Run("Get not blocked while GetFresh runs", func(t *ftt.Test) {
			// Put initial value.
			box := &Breadbox{}
			v, err := box.GetFresh(ctx, func(_ context.Context, prev any) (any, error) {
				return 1, nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v.(int), should.Equal(1))

			fetching := make(chan bool)
			resume := make(chan bool)
			fetcher := func(_ context.Context, prev any) (any, error) {
				fetching <- true
				<-resume
				return 2, nil
			}

			// Start blocking fetch of the new value.
			wg := sync.WaitGroup{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				v, err := box.GetFresh(ctx, fetcher)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, v.(int), should.Equal(2))
			}()

			// Wait until we hit the body of the fetcher callback.
			<-fetching

			// Concurrent Get() returns existing copy right away (does not deadlock).
			v, err = box.Get(ctx, maxStaleness, fetcher)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v.(int), should.Equal(1))

			// Make the cached value stale.
			tc.Add(2 * maxStaleness)
			// Concurrent Get() returns existing copy right away (does not deadlock),
			// despite being too stale.
			v, err = box.Get(ctx, maxStaleness, fetcher)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v.(int), should.Equal(1))

			// Wait until GetFresh goroutine finishes the fetch.
			resume <- true
			wg.Wait()

			// Returns new value now.
			v, err = box.Get(ctx, maxStaleness, fetcher)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v.(int), should.Equal(2))
		})

		t.Run("Recovers from panic", func(t *ftt.Test) {
			// Initial value.
			box := &Breadbox{}
			v, err := box.GetFresh(ctx, func(_ context.Context, prev any) (any, error) {
				return 1, nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v.(int), should.Equal(1))

			// Start panicking fetch.
			assert.Loosely(t, func() {
				box.GetFresh(ctx, func(_ context.Context, prev any) (any, error) {
					panic("oh dear")
				})
			}, should.PanicLikeString("oh dear"))

			// Doesn't deadlock.
			v, err = box.GetFresh(ctx, func(_ context.Context, prev any) (any, error) {
				return 2, nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v.(int), should.Equal(2))
		})

		t.Run("Nil value is allowed", func(t *ftt.Test) {
			// Initial nil value.
			box := &Breadbox{}
			v, err := box.GetFresh(ctx, func(_ context.Context, prev any) (any, error) {
				return nil, nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v, should.BeNil)

			// We fetch something else.
			v, err = box.GetFresh(ctx, func(_ context.Context, prev any) (any, error) {
				assert.Loosely(t, prev, should.BeNil)
				return 1, nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v.(int), should.Equal(1))
		})
	})
}
