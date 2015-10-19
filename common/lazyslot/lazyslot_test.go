// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lazyslot

import (
	"sync"
	"testing"
	"time"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestLazySlot(t *testing.T) {
	Convey("Blocking mode works", t, func() {
		c := newContext()

		counter := 0
		s := Slot{
			Fetcher: func(c context.Context, prev Value) (Value, error) {
				counter++
				return Value{counter, clock.Now(c).Add(time.Second)}, nil
			},
		}

		// Initial fetch.
		So(s.Peek(), ShouldResemble, Value{})
		v, err := s.Get(c)
		So(err, ShouldBeNil)
		So(v.Value.(int), ShouldEqual, 1)
		So(s.Peek().Value.(int), ShouldEqual, 1)

		// Still fresh.
		v, err = s.Get(c)
		So(err, ShouldBeNil)
		So(v.Value.(int), ShouldEqual, 1)

		// Expires and refreshed.
		clock.Get(c).(testclock.TestClock).Add(5 * time.Second)
		v, err = s.Get(c)
		So(err, ShouldBeNil)
		So(v.Value.(int), ShouldEqual, 2)
	})

	Convey("Returns stale copy while fetching", t, func(conv C) {
		c := newContext()

		// Put initial value.
		s := Slot{
			Fetcher: func(c context.Context, prev Value) (Value, error) {
				return Value{1, clock.Now(c).Add(time.Second)}, nil
			},
		}
		v, err := s.Get(c)
		So(err, ShouldBeNil)
		So(v.Value.(int), ShouldEqual, 1)

		// Make it expire. Start blocking fetch of the new value.
		clock.Get(c).(testclock.TestClock).Add(5 * time.Second)
		fetching := make(chan bool)
		resume := make(chan bool)
		s.Fetcher = func(c context.Context, prev Value) (Value, error) {
			fetching <- true
			<-resume
			return Value{2, clock.Now(c).Add(time.Second)}, nil
		}
		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := s.Get(c)
			conv.So(err, ShouldBeNil)
			conv.So(v.Value.(int), ShouldEqual, 2)
		}()

		// Wait until we hit the body of the fetcher callback.
		<-fetching

		// Concurrent Get() returns stale copy right away (does not deadlock).
		v, err = s.Get(c)
		So(err, ShouldBeNil)
		So(v.Value.(int), ShouldEqual, 1)

		// Wait until another goroutine finishes the fetch.
		resume <- true
		wg.Wait()

		// Returns new value now.
		v, err = s.Get(c)
		So(err, ShouldBeNil)
		So(v.Value.(int), ShouldEqual, 2)
	})

	Convey("Recovers from panic", t, func(conv C) {
		c := newContext()

		// Initial value.
		s := Slot{
			Fetcher: func(c context.Context, prev Value) (Value, error) {
				return Value{1, clock.Now(c).Add(time.Second)}, nil
			},
		}
		v, err := s.Get(c)
		So(err, ShouldBeNil)
		So(v.Value.(int), ShouldEqual, 1)

		// Make it expire. Start panicing fetch.
		clock.Get(c).(testclock.TestClock).Add(5 * time.Second)
		s.Fetcher = func(c context.Context, prev Value) (Value, error) {
			panic("omg")
		}
		So(func() { s.Get(c) }, ShouldPanicWith, "omg")

		// Doesn't deadlock.
		s.Fetcher = func(c context.Context, prev Value) (Value, error) {
			return Value{2, clock.Now(c).Add(time.Second)}, nil
		}
		v, err = s.Get(c)
		So(err, ShouldBeNil)
		So(v.Value.(int), ShouldEqual, 2)
	})
}

func newContext() context.Context {
	return clock.Set(context.Background(), testclock.New(time.Unix(1442270520, 0)))
}
