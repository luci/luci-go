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
	"errors"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLazySlot(t *testing.T) {
	Convey("Blocking mode works", t, func() {
		c, clk := newContext()

		lock := sync.Mutex{}
		counter := 0

		fetcher := func(c context.Context, prev interface{}) (interface{}, time.Time, error) {
			lock.Lock()
			defer lock.Unlock()
			counter++
			return counter, clk.Now().Add(time.Second), nil
		}

		s := Slot{}

		// Initial fetch.
		So(s.current, ShouldBeNil)
		v, _, err := s.Get(c, fetcher)
		So(err, ShouldBeNil)
		So(v.(int), ShouldEqual, 1)

		// Still fresh.
		v, _, err = s.Get(c, fetcher)
		So(err, ShouldBeNil)
		So(v.(int), ShouldEqual, 1)

		// Expires and refreshed.
		clk.Add(5 * time.Second)
		v, _, err = s.Get(c, fetcher)
		So(err, ShouldBeNil)
		So(v.(int), ShouldEqual, 2)
	})

	Convey("Returns stale copy while fetching", t, func(conv C) {
		c, clk := newContext()

		// Put initial value.
		s := Slot{}
		v, _, err := s.Get(c, func(c context.Context, prev interface{}) (interface{}, time.Time, error) {
			return 1, clk.Now().Add(time.Second), nil
		})
		So(err, ShouldBeNil)
		So(v.(int), ShouldEqual, 1)

		fetching := make(chan bool)
		resume := make(chan bool)
		fetcher := func(c context.Context, prev interface{}) (interface{}, time.Time, error) {
			fetching <- true
			<-resume
			return 2, clk.Now().Add(time.Second), nil
		}

		// Make it expire. Start blocking fetch of the new value.
		clk.Add(5 * time.Second)
		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, _, err := s.Get(c, fetcher)
			conv.So(err, ShouldBeNil)
			conv.So(v.(int), ShouldEqual, 2)
		}()

		// Wait until we hit the body of the fetcher callback.
		<-fetching

		// Concurrent Get() returns stale copy right away (does not deadlock).
		v, _, err = s.Get(c, fetcher)
		So(err, ShouldBeNil)
		So(v.(int), ShouldEqual, 1)

		// Wait until another goroutine finishes the fetch.
		resume <- true
		wg.Wait()

		// Returns new value now.
		v, _, err = s.Get(c, fetcher)
		So(err, ShouldBeNil)
		So(v.(int), ShouldEqual, 2)
	})

	Convey("Recovers from panic", t, func() {
		c, clk := newContext()

		// Initial value.
		s := Slot{}
		v, _, err := s.Get(c, func(c context.Context, prev interface{}) (interface{}, time.Time, error) {
			return 1, clk.Now().Add(time.Second), nil
		})
		So(err, ShouldBeNil)
		So(v.(int), ShouldEqual, 1)

		// Make it expire. Start panicing fetch.
		clk.Add(5 * time.Second)
		So(func() {
			s.Get(c, func(c context.Context, prev interface{}) (interface{}, time.Time, error) {
				panic("omg")
			})
		}, ShouldPanicWith, "omg")

		// Doesn't deadlock.
		v, _, err = s.Get(c, func(c context.Context, prev interface{}) (interface{}, time.Time, error) {
			return 2, clk.Now().Add(time.Second), nil
		})
		So(err, ShouldBeNil)
		So(v.(int), ShouldEqual, 2)
	})

	Convey("Checks for nil", t, func() {
		c, clk := newContext()
		s := Slot{}
		So(func() {
			s.Get(c, func(c context.Context, prev interface{}) (interface{}, time.Time, error) {
				return nil, clk.Now().Add(time.Second), nil
			})
		}, ShouldPanicWith, "fetcher returned nil value and no error, this is forbidden")
	})

	Convey("Retries failed refetch later", t, func() {
		c, clk := newContext()

		var errorToReturn error
		var valueToReturn int

		fetchCalls := 0
		fetcher := func(c context.Context, prev interface{}) (interface{}, time.Time, error) {
			fetchCalls++
			return valueToReturn, clk.Now().Add(time.Minute), errorToReturn
		}

		s := Slot{}

		// Initial fetch.
		valueToReturn = 1
		errorToReturn = nil
		val, _, err := s.Get(c, fetcher)
		So(val, ShouldResemble, 1)
		So(err, ShouldBeNil)
		So(fetchCalls, ShouldEqual, 1)

		// Cached copy is good after 30 sec.
		clk.Add(30 * time.Second)
		valueToReturn = 2
		errorToReturn = nil
		val, _, err = s.Get(c, fetcher)
		So(val, ShouldResemble, 1) // still cached copy
		So(err, ShouldBeNil)
		So(fetchCalls, ShouldEqual, 1)

		// After 31 the cache copy expires, we attempt to update it, but something
		// goes horribly wrong. Get(...) returns the old copy, with expiration time
		// bumped to now+5 sec.
		clk.Add(31 * time.Second)
		valueToReturn = 3
		errorToReturn = errors.New("omg")
		val, exp, err := s.Get(c, fetcher)
		So(val, ShouldResemble, 1) // still cached copy
		So(exp, ShouldResemble, clk.Now().Add(5*time.Second))
		So(err, ShouldBeNil)
		So(fetchCalls, ShouldEqual, 2) // attempted to fetch

		// 1 sec later still using old copy, because retry is scheduled for later.
		clk.Add(time.Second)
		valueToReturn = 4
		errorToReturn = nil
		val, _, err = s.Get(c, fetcher)
		So(val, ShouldResemble, 1) // still cached copy
		So(err, ShouldBeNil)
		So(fetchCalls, ShouldEqual, 2)

		// 5 sec later fetched is attempted, and it succeeds.
		clk.Add(5 * time.Second)
		valueToReturn = 5
		errorToReturn = nil
		val, _, err = s.Get(c, fetcher)
		So(val, ShouldResemble, 5) // new copy
		So(err, ShouldBeNil)
		So(fetchCalls, ShouldEqual, 3)
	})
}

func newContext() (context.Context, testclock.TestClock) {
	return testclock.UseTime(context.Background(), time.Unix(1442270520, 0))
}
