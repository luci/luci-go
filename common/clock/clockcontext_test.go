// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package clock

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

// A test blocking operation.
func blockingOperation(ctx context.Context, releaseC chan struct{}) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()

	case <-releaseC:
		return true, nil
	}
}

func TestClockContext(t *testing.T) {
	t.Parallel()

	Convey(`A context with a deadline wrapping a cancellable parent`, t, func() {
		now := time.Now()
		cctx, pcf := context.WithCancel(context.Background())
		ctx, cf := WithTimeout(cctx, 10*time.Millisecond)
		releaseC := make(chan struct{})

		Convey(`Successfully reports its deadline.`, func() {
			deadline, ok := ctx.Deadline()
			So(ok, ShouldBeTrue)
			So(deadline.After(now), ShouldBeTrue)
		})

		Convey(`Will successfully time out.`, func() {
			released, err := blockingOperation(ctx, releaseC)
			So(released, ShouldBeFalse)
			So(err, ShouldEqual, context.DeadlineExceeded)
		})

		Convey(`Will successfully cancel with its cancel func.`, func() {
			go func() {
				cf()
			}()
			released, err := blockingOperation(ctx, releaseC)
			So(released, ShouldBeFalse)
			So(err, ShouldEqual, context.Canceled)
		})

		Convey(`Will successfully cancel if the parent is cancelled.`, func() {
			go func() {
				pcf()
			}()
			released, err := blockingOperation(ctx, releaseC)
			So(released, ShouldBeFalse)
			So(err, ShouldEqual, context.Canceled)
		})

		Convey(`Will release before the deadline.`, func() {
			go func() {
				close(releaseC)
			}()
			released, err := blockingOperation(ctx, releaseC)
			So(released, ShouldBeTrue)
			So(err, ShouldBeNil)
		})
	})

	Convey(`A context with a deadline wrapping a parent with a shorter deadline`, t, func() {
		cctx, _ := context.WithTimeout(context.Background(), 10*time.Millisecond)
		ctx, cf := WithTimeout(cctx, 1*time.Hour)
		releaseC := make(chan struct{})

		Convey(`Will successfully time out.`, func() {
			released, err := blockingOperation(ctx, releaseC)
			So(released, ShouldBeFalse)
			So(err, ShouldEqual, context.DeadlineExceeded)
		})

		Convey(`Will successfully cancel with its cancel func.`, func() {
			go func() {
				cf()
			}()
			released, err := blockingOperation(ctx, releaseC)
			So(released, ShouldBeFalse)
			So(err, ShouldEqual, context.Canceled)
		})
	})

	Convey(`A context with a deadline in the past`, t, func() {
		ctx, _ := WithDeadline(context.Background(), time.Unix(0, 0))
		releaseC := make(chan struct{})

		Convey(`Will time out immediately.`, func() {
			go func() {
				defer close(releaseC)
				time.Sleep(50 * time.Millisecond)
			}()
			released, err := blockingOperation(ctx, releaseC)
			So(released, ShouldBeFalse)
			So(err, ShouldEqual, context.DeadlineExceeded)
		})
	})
}
