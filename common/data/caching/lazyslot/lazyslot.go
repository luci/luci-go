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

// Package lazyslot implements a caching scheme for globally shared objects that
// take significant time to refresh.
//
// The defining property of the implementation is that only one goroutine will
// block when refreshing such object, while all others will use a slightly stale
// cached copy.
package lazyslot

import (
	"context"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
)

// ExpiresImmediately can be returned by the fetcher callback to indicate that
// the item must be refreshed on the next access.
//
// This is sometimes useful in tests with "frozen" time to disable caching.
const ExpiresImmediately time.Duration = -1

// Fetcher knows how to load a new value or refresh the existing one.
//
// It receives the previously known value when refreshing it.
//
// If the returned expiration duration is zero, the returned value never
// expires. If the returned expiration duration is equal to ExpiresImmediately,
// then the very next Get(...) will trigger another refresh (this is sometimes
// useful in tests with "frozen" time to disable caching).
type Fetcher func(prev any) (updated any, exp time.Duration, err error)

// Slot holds a cached value and refreshes it when it expires.
//
// Only one goroutine will be busy refreshing, all others will see a slightly
// stale copy of the value during the refresh.
type Slot struct {
	RetryDelay time.Duration // how long to wait before fetching after a failure, 5 sec by default

	lock        sync.RWMutex // protects the guts below
	initialized bool         // true if fetched the initial value already
	current     any          // currently known value (may be nil)
	exp         time.Time    // when the currently known value expires or time.Time{} if never
	fetching    bool         // true if some goroutine is fetching the value now
}

// Get returns stored value if it is still fresh or refetches it if it's stale.
//
// It may return slightly stale copy if some other goroutine is fetching a new
// copy now. If there's no cached copy at all, blocks until it is retrieved.
//
// Returns an error only when there's no cached copy yet and Fetcher returns
// an error.
//
// If there's an expired cached copy, and Fetcher returns an error when trying
// to refresh it, logs the error and returns the existing cached copy (which is
// stale at this point). We assume callers prefer stale copy over a hard error.
//
// On refetch errors bumps expiration time of the cached copy to RetryDelay
// seconds from now, effectively scheduling a retry at some later time.
// RetryDelay is 5 sec by default.
//
// The passed context is used for logging and for getting time.
func (s *Slot) Get(ctx context.Context, fetcher Fetcher) (value any, err error) {
	now := clock.Now(ctx)

	// Fast path. Checks a cached value exists and it is still fresh or some
	// goroutine is already updating it (in that case we return a stale copy).
	ok := false
	s.lock.RLock()
	if s.initialized && (s.fetching || isFresh(s.exp, now)) {
		value = s.current
		ok = true
	}
	s.lock.RUnlock()
	if ok {
		return
	}

	// Slow path. Attempt to start the fetch if no one beat us to it.
	shouldFetch, value, err := s.initiateFetch(ctx, fetcher, now)
	if !shouldFetch {
		// Either someone did the fetch already, or the initial fetch failed. In
		// either case 'value' and 'err' are already set, so just return them.
		return
	}

	// 'value' here is currently known value that we are going to refresh. Need
	// to clear the variable to make sure 'defer' below sees nil on panic.
	prevValue := value
	value = nil

	// The current goroutine won the contest and now is responsible for refetching
	// the value. Do it, but be cautious to fix the state in case of a panic.
	var completed bool
	var exp time.Duration
	defer func() { s.finishFetch(completed, value, setExpiry(ctx, exp)) }()

	value, exp, err = fetcher(prevValue)
	completed = true // we didn't panic!

	// Log the error and return the previous value, bumping its expiration time by
	// retryDelay to trigger a retry at some later time.
	if err != nil {
		logging.WithError(err).Errorf(ctx, "lazyslot: failed to update instance of %T", prevValue)
		value = prevValue
		exp = s.retryDelay()
		err = nil
	}

	return
}

// initiateFetch modifies state of Slot to indicate that the current goroutine
// is going to do the fetch if no one is fetching it now.
//
// Returns:
//   - (true, known value, nil) if the current goroutine should refetch.
//   - (false, known value, nil) if the fetch is no longer necessary.
//   - (false, nil, err) if the initial fetch failed.
func (s *Slot) initiateFetch(ctx context.Context, fetcher Fetcher, now time.Time) (bool, any, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	// A cached value exists and it is still fresh? Return it right away. Someone
	// refetched it already.
	if s.initialized && isFresh(s.exp, now) {
		return false, s.current, nil
	}

	// Fetching the value for the first time ever? Do it under the lock because
	// there's nothing to return yet. All goroutines would have to wait for this
	// initial fetch to complete. They'll all block on s.lock.RLock() in Get(...).
	if !s.initialized {
		result, exp, err := fetcher(nil)
		if err != nil {
			return false, nil, err
		}
		s.initialized = true
		s.current = result
		s.exp = setExpiry(ctx, exp)
		return false, s.current, nil
	}

	// We have a cached copy but it has expired. Maybe some other goroutine is
	// fetching it already? Return the cached stale copy if so.
	if s.fetching {
		return false, s.current, nil
	}

	// No one is fetching the value now, we should do it. Make other goroutines
	// know we'll be fetching. Return the current value as well, to pass it to
	// the fetch callback.
	s.fetching = true
	return true, s.current, nil
}

// finishFetch switches the Slot back to "not fetching" state, remembering the
// fetched value.
//
// 'completed' is false if the fetch panicked.
func (s *Slot) finishFetch(completed bool, result any, exp time.Time) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.fetching = false
	if completed {
		s.current = result
		s.exp = exp
	}
}

func (s *Slot) retryDelay() time.Duration {
	if s.RetryDelay == 0 {
		return 5 * time.Second
	}
	return s.RetryDelay
}

func isFresh(exp, now time.Time) bool {
	return exp.IsZero() || now.Before(exp)
}

func setExpiry(ctx context.Context, exp time.Duration) time.Time {
	switch {
	case exp == 0:
		return time.Time{}
	case exp < 0: // including ExpiresImmediately
		return clock.Now(ctx) // this would make isFresh return false
	}
	return clock.Now(ctx).Add(exp)
}
