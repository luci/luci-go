// Copyright (c) 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package promise

import (
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"golang.org/x/net/context"
)

var (
	now = time.Unix(1420070400, 0)
)

func TestPromise(t *testing.T) {
	Convey(`An instrumented Promise instance`, t, func() {
		ctx, tc := testclock.UseTime(context.Background(), time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC))

		type operation struct {
			d interface{}
			e error
		}

		opC := make(chan operation)
		p := New(func() (interface{}, error) {
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
			tc.SetTimerCallback(func(_ clock.Timer) {
				close(readyC)
			})
			go func() {
				<-readyC
				tc.Add(1 * time.Second)
			}()

			ctx, _ = clock.WithTimeout(ctx, 1*time.Second)
			data, err := p.Get(ctx)
			So(data, ShouldBeNil)
			So(err, ShouldEqual, context.DeadlineExceeded)
		})

		Convey(`With data already added`, func() {
			e := errors.New("promise: fake test error")
			opC <- operation{"DATA", e}

			data, err := p.Get(ctx)
			So(data, ShouldEqual, "DATA")
			So(err, ShouldEqual, e)

			Convey(`Will return data immediately.`, func() {
				data, err := p.Peek()
				So(data, ShouldEqual, "DATA")
				So(err, ShouldEqual, e)
			})

			Convey(`Will return data instead of timing out.`, func() {
				// Wait until our Promise starts its timer. Then signal it.
				readyC := make(chan struct{})
				tc.SetTimerCallback(func(_ clock.Timer) {
					close(readyC)
				})
				go func() {
					<-readyC
					tc.Add(1 * time.Second)
				}()

				ctx, _ = clock.WithTimeout(ctx, 1*time.Second)
				data, err := p.Get(ctx)
				So(data, ShouldEqual, "DATA")
				So(err, ShouldEqual, e)
			})
		})
	})

	Convey(`A Promise instance with multiple consumers will block.`, t, func() {
		ctx, _ := testclock.UseTime(context.Background(), time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC))
		dataC := make(chan interface{})
		p := New(func() (interface{}, error) {
			return <-dataC, nil
		})

		finishedC := make(chan string)
		for i := uint(0); i < 100; i++ {
			go func() {
				data, _ := p.Get(ctx)
				finishedC <- data.(string)
			}()
			go func() {
				ctx, _ = clock.WithTimeout(ctx, 1*time.Hour)
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
