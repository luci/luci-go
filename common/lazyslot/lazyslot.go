// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package lazyslot implements a caching scheme for globally shared objects that
// take significant time to refresh. The defining property of the implementation
// is that only one goroutine (can be background one) will block when refreshing
// such object, while all others will use a slightly stale cached copy.
package lazyslot

import (
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
)

// Value is what's stored in a Slot. It is treated as immutable value.
type Value struct {
	// Value is whatever fetcher returned.
	Value interface{}
	// Expiration is time when this value expires and should be refetched.
	Expiration time.Time
}

// Fetcher knows how to load new value.
type Fetcher func(c context.Context, prev Value) (Value, error)

// Slot holds a cached Value and refreshes it when it expires. Only one
// goroutine will be busy refreshing, all others will see a slightly stale
// copy of the value during the refresh.
type Slot struct {
	Fetcher Fetcher       // used to actually load the value on demand
	Timeout time.Duration // how long to allow to fetch, 5 sec by default.
	Async   bool          // if true do fetches in background goroutine

	lock           sync.Mutex      // protects the guts below
	current        *Value          // currently known value or nil if not fetched
	currentFetcher context.Context // non nil if some goroutine is fetching now
}

// Peek returns currently cached value if there's one or zero Value{} if not.
// It doesn't try to fetch a value.
func (s *Slot) Peek() Value {
	if s.current == nil {
		return Value{}
	}
	return *s.current
}

// Get returns stored value if it is still fresh. It may return slightly stale
// copy if some other goroutine is fetching a new copy now. If there's no cached
// copy at all, blocks until it is retrieved (even if slot is configured with
// Async = true). Returns an error only when Fetcher returns an error.
func (s *Slot) Get(c context.Context) (Value, error) {
	now := clock.Now(c)

	// Set in the local function below, used it fetch is needed.
	var (
		ctx     context.Context
		fetchCb Fetcher
		prevVal Value
		async   bool
	)

	// If done is true, val and err are returned right away.
	done, val, err := func() (bool, Value, error) {
		s.lock.Lock()
		defer s.lock.Unlock()

		// Still fresh? Return right away.
		if s.current != nil && now.Before(s.current.Expiration) {
			return true, *s.current, nil
		}

		// Fetching the value for the first time ever? Do it under the lock because
		// there's nothing to return yet. All goroutines would have to wait for this
		// initial fetch to complete. They'll all block on s.lock.Lock() above.
		if s.current == nil {
			val, err := s.Fetcher(c, Value{})
			if err != nil {
				return true, Value{}, err
			}
			s.current = &val
			return true, val, nil
		}

		// We have a cached copy and it has expired. Maybe some other goroutine is
		// fetching it already? Returns the stale copy if so.
		if s.currentFetcher != nil {
			return true, *s.current, nil
		}

		// No one is fetching the value now, we should do it. Release the lock while
		// fetching to allow other goroutines to grab the stale copy.
		timeout := 5 * time.Second
		if s.Timeout != 0 {
			timeout = s.Timeout
		}
		s.currentFetcher, _ = context.WithTimeout(c, timeout)
		ctx = s.currentFetcher
		fetchCb = s.Fetcher
		prevVal = *s.current
		async = s.Async
		return false, Value{}, nil
	}()
	if done {
		return val, err
	}

	fetch := func() (val Value, err error) {
		defer func() {
			s.lock.Lock()
			defer s.lock.Unlock()
			s.currentFetcher = nil
			if err == nil && val.Value != nil {
				s.current = &val
			}
		}()
		return fetchCb(ctx, prevVal)
	}

	if async {
		go fetch()
		return prevVal, nil
	}
	return fetch()
}
