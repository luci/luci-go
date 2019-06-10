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

package lazyslot

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLazySlot(t *testing.T) {
	Convey("Blocking mode works", t, func() {
		c, clk := newContext()

		lock := sync.Mutex{}
		counter := 0

		fetcher := func(prev interface{}) (interface{}, time.Duration, error) {
			lock.Lock()
			defer lock.Unlock()
			counter++
			return counter, time.Second, nil
		}

		s := Slot{}

		// Initial fetch.
		So(s.current, ShouldBeNil)
		v, err := s.Get(c, fetcher)
		So(err, ShouldBeNil)
		So(v.(int), ShouldEqual, 1)

		// Still fresh.
		v, err = s.Get(c, fetcher)
		So(err, ShouldBeNil)
		So(v.(int), ShouldEqual, 1)

		// Expires and refreshed.
		clk.Add(5 * time.Second)
		v, err = s.Get(c, fetcher)
		So(err, ShouldBeNil)
		So(v.(int), ShouldEqual, 2)
	})

	Convey("Initial failed fetch causes errors", t, func() {
		c, _ := newContext()

		s := Slot{}

		// Initial failed fetch.
		failErr := errors.New("fail")
		_, err := s.Get(c, func(prev interface{}) (interface{}, time.Duration, error) {
			return nil, 0, failErr
		})
		So(err, ShouldEqual, failErr)

		// Subsequence successful fetch.
		val, err := s.Get(c, func(prev interface{}) (interface{}, time.Duration, error) {
			return 1, 0, nil
		})
		So(err, ShouldBeNil)
		So(val, ShouldResemble, 1)
	})

	Convey("Returns stale copy while fetching", t, func(conv C) {
		c, clk := newContext()

		// Put initial value.
		s := Slot{}
		v, err := s.Get(c, func(prev interface{}) (interface{}, time.Duration, error) {
			return 1, time.Second, nil
		})
		So(err, ShouldBeNil)
		So(v.(int), ShouldEqual, 1)

		fetching := make(chan bool)
		resume := make(chan bool)
		fetcher := func(prev interface{}) (interface{}, time.Duration, error) {
			fetching <- true
			<-resume
			return 2, time.Second, nil
		}

		// Make it expire. Start blocking fetch of the new value.
		clk.Add(5 * time.Second)
		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := s.Get(c, fetcher)
			conv.So(err, ShouldBeNil)
			conv.So(v.(int), ShouldEqual, 2)
		}()

		// Wait until we hit the body of the fetcher callback.
		<-fetching

		// Concurrent Get() returns stale copy right away (does not deadlock).
		v, err = s.Get(c, fetcher)
		So(err, ShouldBeNil)
		So(v.(int), ShouldEqual, 1)

		// Wait until another goroutine finishes the fetch.
		resume <- true
		wg.Wait()

		// Returns new value now.
		v, err = s.Get(c, fetcher)
		So(err, ShouldBeNil)
		So(v.(int), ShouldEqual, 2)
	})

	Convey("Recovers from panic", t, func() {
		c, clk := newContext()

		// Initial value.
		s := Slot{}
		v, err := s.Get(c, func(prev interface{}) (interface{}, time.Duration, error) {
			return 1, time.Second, nil
		})
		So(err, ShouldBeNil)
		So(v.(int), ShouldEqual, 1)

		// Make it expire. Start panicking fetch.
		clk.Add(5 * time.Second)
		So(func() {
			s.Get(c, func(prev interface{}) (interface{}, time.Duration, error) {
				panic("omg")
			})
		}, ShouldPanicWith, "omg")

		// Doesn't deadlock.
		v, err = s.Get(c, func(prev interface{}) (interface{}, time.Duration, error) {
			return 2, time.Second, nil
		})
		So(err, ShouldBeNil)
		So(v.(int), ShouldEqual, 2)
	})

	Convey("Nil value is allowed", t, func() {
		c, clk := newContext()
		s := Slot{}

		// Initial nil fetch.
		val, err := s.Get(c, func(prev interface{}) (interface{}, time.Duration, error) {
			return nil, time.Second, nil
		})
		So(err, ShouldBeNil)
		So(val, ShouldBeNil)

		// Some time later this nil expires and we fetch something else.
		clk.Add(2 * time.Second)
		val, err = s.Get(c, func(prev interface{}) (interface{}, time.Duration, error) {
			So(prev, ShouldBeNil)
			return 1, time.Second, nil
		})
		So(err, ShouldBeNil)
		So(val, ShouldResemble, 1)
	})

	Convey("Zero expiration means never expires", t, func() {
		c, clk := newContext()
		s := Slot{}

		// Initial fetch.
		val, err := s.Get(c, func(prev interface{}) (interface{}, time.Duration, error) {
			return 1, 0, nil
		})
		So(err, ShouldBeNil)
		So(val, ShouldResemble, 1)

		// Many years later still cached.
		clk.Add(200000 * time.Hour)
		val, err = s.Get(c, func(prev interface{}) (interface{}, time.Duration, error) {
			return 2, time.Second, nil
		})
		So(err, ShouldBeNil)
		So(val, ShouldResemble, 1)
	})

	Convey("ExpiresImmediately means expires at the same instant", t, func() {
		c, _ := newContext()
		s := Slot{}

		// Initial fetch.
		val, err := s.Get(c, func(prev interface{}) (interface{}, time.Duration, error) {
			return 1, ExpiresImmediately, nil
		})
		So(err, ShouldBeNil)
		So(val, ShouldResemble, 1)

		// No time moved, but refetch still happened.
		val, err = s.Get(c, func(prev interface{}) (interface{}, time.Duration, error) {
			return 2, time.Second, nil
		})
		So(err, ShouldBeNil)
		So(val, ShouldResemble, 2)
	})

	Convey("Retries failed refetch later", t, func() {
		c, clk := newContext()

		var errorToReturn error
		var valueToReturn int

		fetchCalls := 0
		fetcher := func(prev interface{}) (interface{}, time.Duration, error) {
			fetchCalls++
			return valueToReturn, time.Minute, errorToReturn
		}

		s := Slot{}

		// Initial fetch.
		valueToReturn = 1
		errorToReturn = nil
		val, err := s.Get(c, fetcher)
		So(val, ShouldResemble, 1)
		So(err, ShouldBeNil)
		So(fetchCalls, ShouldEqual, 1)

		// Cached copy is good after 30 sec.
		clk.Add(30 * time.Second)
		valueToReturn = 2
		errorToReturn = nil
		val, err = s.Get(c, fetcher)
		So(val, ShouldResemble, 1) // still cached copy
		So(err, ShouldBeNil)
		So(fetchCalls, ShouldEqual, 1)

		// After 31 the cache copy expires, we attempt to update it, but something
		// goes horribly wrong. Get(...) returns the old copy.
		clk.Add(31 * time.Second)
		valueToReturn = 3
		errorToReturn = errors.New("omg")
		val, err = s.Get(c, fetcher)
		So(val, ShouldResemble, 1) // still cached copy
		So(err, ShouldBeNil)
		So(fetchCalls, ShouldEqual, 2) // attempted to fetch

		// 1 sec later still using old copy, because retry is scheduled for later.
		clk.Add(time.Second)
		valueToReturn = 4
		errorToReturn = nil
		val, err = s.Get(c, fetcher)
		So(val, ShouldResemble, 1) // still cached copy
		So(err, ShouldBeNil)
		So(fetchCalls, ShouldEqual, 2)

		// 5 sec later fetched is attempted, and it succeeds.
		clk.Add(5 * time.Second)
		valueToReturn = 5
		errorToReturn = nil
		val, err = s.Get(c, fetcher)
		So(val, ShouldResemble, 5) // new copy
		So(err, ShouldBeNil)
		So(fetchCalls, ShouldEqual, 3)
	})
}

func newContext() (context.Context, testclock.TestClock) {
	return testclock.UseTime(context.Background(), time.Unix(1442270520, 0))
}
