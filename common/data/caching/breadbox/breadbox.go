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

// Package breadbox implements a caching scheme for globally shared objects that
// take significant time to refresh.
//
// It is similar to lazyslot, but it has two distinct ways to retrieve from the
// cache:
//   - Get: accept staleness and have at most 1 goroutine block while
//     refreshing (all other goroutines using this method will receive a
//     slightly stale copy);
//   - GetFresh: refuse staleness and forcibly refresh the cache.
package breadbox

import (
	"context"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
)

const (
	// The default delay between retries, if fetching fails and staleness is
	// acceptable.
	DefaultRetryDelay = 5 * time.Second
)

type Refresher func(ctx context.Context, prev any) (updated any, err error)

type Breadbox struct {
	RetryDelay time.Duration

	dataLock      sync.RWMutex
	initialized   bool
	current       any
	lastAttempted time.Time
	failed        bool
	fetching      bool

	fetchLock sync.Mutex
}

// Get returns stored value if it is still fresh or refreshes it if it's stale.
//
// May return a stale copy if some other goroutine is fetching a new copy now.
// If there's no cached copy at all, blocks until it is retrieved.
//
// Returns an error only when there's no cached copy yet and Refresher returns
// an error.
//
// If there's an existing cached copy, and Refresher returns an error when
// trying to refresh it, logs the error and returns the existing cached copy
// (which is stale at this point). We assume callers prefer stale copy over a
// hard error.
//
// On refresh errors, delays the next retry by at least RetryDelay.
// RetryDelay is 5 sec by default.
//
// The passed context is used for logging and for getting time.
func (b *Breadbox) Get(ctx context.Context, maxStaleness time.Duration,
	refresher Refresher) (value any, err error) {
	now := clock.Now(ctx)

	// Fast path. Checks a cached value exists and it is still fresh or some
	// goroutine is already updating it (in that case we return a stale copy).
	ok := false
	b.dataLock.RLock()
	if b.initialized && (b.fetching || b.isFresh(b.lastAttempted, now, maxStaleness, b.failed)) {
		value = b.current
		ok = true
	}
	b.dataLock.RUnlock()
	if ok {
		return
	}

	// Slow path. Attempt to start the fetch if no one beat us to it.
	shouldRefresh, value, err := b.initiateFetch(ctx, refresher, now, maxStaleness)
	if !shouldRefresh {
		// Either someone did the refresh already, or the initial fetch failed.
		// In either case 'value' and 'err' are already set, so just return
		// them.
		return
	}

	// 'value' here is currently known value that we are going to refresh. Need
	// to clear the variable to make sure 'defer' below sees nil on panic.
	prevValue := value
	value = nil

	// The current goroutine won the contest and now is responsible for
	// refetching the value. Do it, but be cautious to fix the state in case of
	// a panic.
	completed := false
	failed := true
	var lastAttempted time.Time

	// Guard with fetchLock to prevent concurrent expensive fetching.
	b.fetchLock.Lock()
	defer func() {
		b.finishFetch(completed, failed, value, lastAttempted)
		b.fetchLock.Unlock()
	}()

	value, err = refresher(ctx, prevValue)
	lastAttempted = clock.Now(ctx)
	completed = true
	failed = err != nil

	// Log the error and return the previous value.
	if err != nil {
		logging.WithError(err).Errorf(
			ctx, "breadbox cache: failed to update instance of %T", prevValue)
		value = prevValue
		err = nil
	}

	return
}

// GetFresh forcibly refreshes the cache by refetching to get the latest value.
//
// Returns an error only when there's no cached copy yet and Refresher returns
// an error.
//
// If there's an existing cached copy, and Refresher returns an error when
// trying to refresh it, logs the error and returns the existing cached copy
// (which is stale at this point). We assume callers prefer stale copy over a
// hard error.
//
// The passed context is used for logging and for getting time.
func (b *Breadbox) GetFresh(
	ctx context.Context, refresher Refresher) (value any, err error) {
	// Guard with fetchLock to prevent concurrent expensive fetching; dataLock
	// shouldn't be locked for long, as that would degrade the performance of
	// Get(...).
	b.fetchLock.Lock()
	defer b.fetchLock.Unlock()

	// Retrieve the current cached value that will be refreshed.
	b.dataLock.Lock()
	// Make other goroutines which called Get(...) know we'll be fetching.
	b.fetching = true

	// Fetching the value for the first time ever? Do it under the dataLock
	// because there's nothing to return yet. All goroutines would have to wait
	// for this initial fetch to complete. They'll all block on
	// b.dataLock.RLock() in Get(...), or b.dataLock.Lock in GetFresh(...).
	if !b.initialized {
		defer func() {
			b.fetching = false
			b.dataLock.Unlock()
		}()

		value, err = refresher(ctx, nil)
		if err != nil {
			return nil, err
		}
		b.initialized = true
		b.current = value
		b.lastAttempted = clock.Now(ctx)
		return
	}
	prevValue := b.current
	b.dataLock.Unlock()

	completed := false
	failed := true
	var lastAttempted time.Time

	defer func() {
		b.finishFetch(completed, failed, value, lastAttempted)
	}()

	value, err = refresher(ctx, prevValue)
	lastAttempted = clock.Now(ctx)
	completed = true
	failed = err != nil

	// Log the error and return the previous value.
	if err != nil {
		logging.WithError(err).Errorf(
			ctx, "breadbox cache: failed to forcibly update instance of %T", prevValue)
		value = prevValue
		err = nil
	}

	return
}

// initiateFetch modifies state of Breadbox to indicate that the current
// goroutine is going to do the fetch if no one is fetching it now.
//
// Returns:
//   - (true, known value, nil) if the current goroutine should refetch.
//   - (false, known value, nil) if the fetch is no longer necessary.
//   - (false, nil, err) if the initial fetch failed.
func (b *Breadbox) initiateFetch(ctx context.Context, refresher Refresher, now time.Time, maxStaleness time.Duration) (bool, any, error) {
	b.dataLock.Lock()
	defer b.dataLock.Unlock()

	// A cached value exists and it is still fresh? Return it right away.
	// Another goroutine refetched it already.
	if b.initialized && b.isFresh(b.lastAttempted, now, maxStaleness, b.failed) {
		return false, b.current, nil
	}

	// Fetching the value for the first time ever? Do it under the dataLock
	// because there's nothing to return yet. All goroutines would have to wait
	// for this initial fetch to complete. They'll all block on
	// b.dataLock.RLock() in Get(...) or b.dataLock.Lock in GetFresh(...).
	if !b.initialized {
		result, err := refresher(ctx, nil)
		if err != nil {
			return false, nil, err
		}
		b.lastAttempted = clock.Now(ctx)
		b.initialized = true
		b.current = result
		return false, b.current, nil
	}

	// We have a cached copy but it has expired. Maybe some other goroutine is
	// fetching it already? Return the cached stale copy if so.
	if b.fetching {
		return false, b.current, nil
	}

	// No one is fetching the value now - we should do it. Make other goroutines
	// know we'll be fetching. Return the current value as well, to pass it to the
	// fetch callback.
	b.fetching = true
	return true, b.current, nil
}

// finishFetch switches the Breadbox back to "not fetching" state, remembering
// the fetched value.
//
// 'completed' is false if the fetch panicked.
// 'failed' is true if the fetch did not panic but returned an error.
func (b *Breadbox) finishFetch(completed, failed bool, result any, lastAttempted time.Time) {
	b.dataLock.Lock()
	defer b.dataLock.Unlock()
	b.fetching = false
	if completed {
		b.current = result
		b.lastAttempted = lastAttempted
		b.failed = failed
	}
}

func (b *Breadbox) retryDelay() time.Duration {
	if b.RetryDelay == 0 {
		return DefaultRetryDelay
	}
	return b.RetryDelay
}

func (b *Breadbox) isFresh(lastAttempted, now time.Time, maxStaleness time.Duration, failed bool) bool {
	if failed {
		return now.Before(lastAttempted.Add(b.retryDelay()))
	}
	return now.Before(lastAttempted.Add(maxStaleness))
}
