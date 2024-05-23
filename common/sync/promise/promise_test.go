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

package promise

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPromise(t *testing.T) {
	t.Parallel()

	Convey(`An instrumented Promise instance`, t, func() {
		ctx, tc := testclock.UseTime(context.Background(), time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC))

		type operation struct {
			d any
			e error
		}

		opC := make(chan operation)
		p := New(ctx, func(context.Context) (any, error) {
			op := <-opC
			return op.d, op.e
		})

		Convey(`Has no data by default.`, func() {
			data, err := p.Peek()
			So(data, ShouldBeNil)
			So(err, ShouldEqual, ErrNoData)
		})

		Convey(`Will timeout with no data.`, func() {
			// Wait until our Promise starts its timer. Then signal it.
			readyC := make(chan struct{})
			tc.SetTimerCallback(func(_ time.Duration, _ clock.Timer) {
				close(readyC)
			})
			go func() {
				<-readyC
				tc.Add(1 * time.Second)
			}()

			ctx, cancel := clock.WithTimeout(ctx, 1*time.Second)
			defer cancel()
			data, err := p.Get(ctx)
			So(data, ShouldBeNil)
			So(err.Error(), ShouldEqual, context.DeadlineExceeded.Error())
		})

		Convey(`With data already added`, func() {
			e := errors.New("promise: fake test error")
			opC <- operation{"DATA", e}

			data, err := p.Get(ctx)
			So(data, ShouldEqual, "DATA")
			So(err, ShouldEqual, e)

			Convey(`Will return data immediately.`, func() {
				data, err = p.Peek()
				So(data, ShouldEqual, "DATA")
				So(err, ShouldEqual, e)
			})

			Convey(`Will return data instead of timing out.`, func() {
				ctx, cancelFunc := context.WithCancel(ctx)
				cancelFunc()

				data, err = p.Get(ctx)
				So(data, ShouldEqual, "DATA")
				So(err, ShouldEqual, e)
			})
		})
	})
}

func TestDeferredPromise(t *testing.T) {
	t.Parallel()

	Convey(`A deferred Promise instance`, t, func() {
		c := context.Background()

		Convey(`Will defer running until Get is called, and will panic the Get goroutine.`, func() {
			// Since our Get will cause the generator to be run in this goroutine,
			// calling Get with a generator that panics should cause a panic.
			p := NewDeferred(func(context.Context) (any, error) { panic("test panic") })
			So(func() { p.Get(c) }, ShouldPanic)
		})

		Convey(`Can output data.`, func() {
			p := NewDeferred(func(context.Context) (any, error) {
				return "hello", nil
			})
			v, err := p.Get(c)
			So(err, ShouldBeNil)
			So(v, ShouldEqual, "hello")
		})
	})
}

func TestPromiseSmoke(t *testing.T) {
	t.Parallel()

	Convey(`A Promise instance with multiple consumers will block.`, t, func() {
		ctx, _ := testclock.UseTime(context.Background(), time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC))
		dataC := make(chan any)
		p := New(ctx, func(context.Context) (any, error) {
			return <-dataC, nil
		})

		finishedC := make(chan string)
		for i := uint(0); i < 100; i++ {
			go func() {
				data, _ := p.Get(ctx)
				finishedC <- data.(string)
			}()
			go func() {
				ctx, cancel := clock.WithTimeout(ctx, 1*time.Hour)
				defer cancel()
				data, _ := p.Get(ctx)
				finishedC <- data.(string)
			}()
		}

		dataC <- "DATA"
		for i := uint(0); i < 200; i++ {
			So(<-finishedC, ShouldEqual, "DATA")
		}
	})
}
