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
	"sync"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
)

// Fetcher knows how to load a new value (and its expiration time).
//
// If it returns no errors, it MUST return non-nil result or Slot.Get will
// panic. If the expiration duration is zero, the returned value never expires.
type Fetcher func(c context.Context, prev interface{}) (updated interface{}, exp time.Duration, err error)

// Slot holds a cached Value and refreshes it when it expires.
//
// Only one goroutine will be busy refreshing, all others will see a slightly
// stale copy of the value during the refresh.
type Slot struct {
	Timeout    time.Duration // how long to allow to fetch, 15 sec by default.
	RetryDelay time.Duration // how long to wait before fetching after a failure, 5 sec by default

	lock     sync.RWMutex // protects the guts below
	current  interface{}  // currently known value or nil if not fetched
	exp      time.Time    // when the currently known value expires or time.Time{} if never
	fetching bool         // true if some goroutine is fetching the value now
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
// Panics if Fetcher doesn't produce a non-nil value and doesn't return an
// error. It must either return an error or an non-nil value.
func (s *Slot) Get(c context.Context, fetcher Fetcher) (value interface{}, err error) {
	now := clock.Now(c)

	// Fast path. Checks a cached value exists and it is still fresh or some
	// goroutine is already updating it (in that case we return a stale copy).
	s.lock.RLock()
	if s.current != nil && (s.fetching || isFresh(s.exp, now)) {
		value = s.current
	}
	s.lock.RUnlock()
	if value != nil {
		return
	}

	// Slow path. Attempt to start the fetch if no one beat us to it.
	shouldFetch, value, err := s.initiateFetch(c, fetcher, now)
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
	var exp time.Duration
	defer func() { s.finishFetch(value, setExpiry(c, exp)) }()

	fc, done := clock.WithTimeout(c, s.timeout())
	defer done()

	switch value, exp, err = fetcher(fc, prevValue); {
	case err == nil && value == nil:
		panic("fetcher returned nil value and no error, this is forbidden")
	case err != nil:
		// Log the error and return the previous value, bumping its expiration
		// time by retryDelay to trigger a retry at some later time.
		logging.WithError(err).Errorf(fc, "lazyslot: failed to update instance of %T", prevValue)
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
//   * (true, known value, nil) if the current goroutine should refetch.
//   * (false, known value, nil) if the fetch no longer necessary.
//   * (false, nil, err) if the initial fetch failed.
func (s *Slot) initiateFetch(c context.Context, fetcher Fetcher, now time.Time) (bool, interface{}, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	// A cached value exists and it is still fresh? Return it right away. Someone
	// refetched it already.
	if s.current != nil && isFresh(s.exp, now) {
		return false, s.current, nil
	}

	// Fetching the value for the first time ever? Do it under the lock because
	// there's nothing to return yet. All goroutines would have to wait for this
	// initial fetch to complete. They'll all block on s.lock.RLock() in Get(...).
	if s.current == nil {
		ctx, done := clock.WithTimeout(c, s.timeout())
		defer done()
		switch result, exp, err := fetcher(ctx, nil); {
		case err != nil:
			return false, nil, err
		case result == nil:
			panic("fetcher returned nil value and no error, this is forbidden")
		default:
			s.current = result
			s.exp = setExpiry(c, exp)
			return false, s.current, nil
		}
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
func (s *Slot) finishFetch(result interface{}, exp time.Time) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.fetching = false
	if result != nil {
		s.current = result // result is not nil iff fetch succeeded and didn't panic
		s.exp = exp
	}
}

func (s *Slot) timeout() time.Duration {
	if s.Timeout == 0 {
		return 15 * time.Second
	}
	return s.Timeout
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

func setExpiry(c context.Context, exp time.Duration) time.Time {
	if exp == 0 {
		return time.Time{}
	}
	return clock.Now(c).Add(exp)
}
