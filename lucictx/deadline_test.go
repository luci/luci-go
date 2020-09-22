// Copyright 2020 The LUCI Authors.
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

package lucictx

import (
	"context"
	"os"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/clock"
	. "go.chromium.org/luci/common/testing/assertions"

	"go.chromium.org/luci/common/clock/testclock"
)

func TestDeadline(t *testing.T) {
	// not parallel b/c this does stuff with signals.
	// t.Parallel()

	self, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}

	Convey(`AdjustDeadline`, t, func() {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestTimeUTC)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		Convey(`Empty context`, func() {
			cleanup, ac, accancel := AdjustDeadline(ctx, 5)
			defer accancel()

			deadline, ok := ac.Deadline()
			So(ok, ShouldBeFalse)
			So(deadline.IsZero(), ShouldBeTrue)

			// however, Interrupt/SIGTERM handler is still installed
			So(self.Signal(os.Interrupt), ShouldBeNil)

			// cleanup will happen, but context won't.
			<-cleanup
			So(ac, ShouldNotBeDone)

			// Advance the clock by 25s, and presto
			tc.Add(25 * time.Second)
			So(ac, ShouldBeDone)
		})

		Convey(`deadline context`, func() {
			ctx, cancel := clock.WithDeadline(ctx, clock.Now(ctx).Add(100*time.Second))
			defer cancel()

			_, ac, accancel := AdjustDeadline(ctx, 5)
			defer func() {
				accancel()
			}()

			deadline, ok := ac.Deadline()
			So(ok, ShouldBeTrue)
			So(deadline.IsZero(), ShouldBeFalse)
			got := GetDeadline(ac)

			So(got, ShouldResembleProto, &Deadline{
				Deadline:        testclock.TestTimeUTC.Truncate(time.Second).Add(95 * time.Second).Unix(),
				GracePeriodSecs: 25,
			})
		})

		Convey(`Deadline in LUCI_CONTEXT`, func() {
			luciDeadline := testclock.TestTimeUTC.Truncate(time.Second).Add(100 * time.Second)
			ctx = SetDeadline(ctx, &Deadline{
				Deadline:        luciDeadline.Unix(), // 100s into the future
				GracePeriodSecs: 40,
			})

			Convey(`no deadline in context`, func() {
				cleanup, ac, accancel := AdjustDeadline(ctx, 5)
				defer accancel()

				deadline, ok := ac.Deadline()
				So(ok, ShouldBeTrue)
				So(deadline, ShouldEqual, luciDeadline.Add(-5*time.Second))

				Convey(`natural expiration`, func() {
					tc.Add((95 - 35) * time.Second)
					<-cleanup // cleanup unblocks
					So(ac, ShouldNotBeDone)

					tc.Add(35 * time.Second)
					So(ac, ShouldBeDone)

					// We should have ended right around the deadline; there's some slop
					// in the clock package though, and this doesn't seem to be zero.
					So(tc.Now().Sub(deadline), ShouldBeBetween, -time.Millisecond, time.Millisecond)
				})

				Convey(`signal`, func() {
					So(self.Signal(os.Interrupt), ShouldBeNil)
					<-cleanup // cleanup unblocks on signal

					So(ac, ShouldNotBeDone)

					tc.Add(35 * time.Second)
					So(ac, ShouldBeDone)

					// Should have about 1m of time left before the deadline.
					So(tc.Now().Sub(deadline)+time.Minute, ShouldBeBetween,
						-time.Millisecond, time.Millisecond)
				})
			})

			Convey(`earlier deadline in context`, func() {
				ctx, cancel := clock.WithDeadline(ctx, luciDeadline.Add(-50*time.Second))
				defer cancel()

				cleanup, ac, accancel := AdjustDeadline(ctx, 5)
				defer accancel()

				deadline, ok := ac.Deadline()
				So(ok, ShouldBeTrue)
				So(deadline, ShouldEqual, luciDeadline.Add(-55*time.Second))

				Convey(`natural expiration`, func() {
					tc.Add(10 * time.Second)
					<-cleanup // cleanup unblocks
					So(ac, ShouldNotBeDone)

					tc.Add(35 * time.Second)
					So(ac, ShouldBeDone)

					// We should have ended right around the deadline; there's some slop
					// in the clock package though, and this doesn't seem to be zero.
					So(tc.Now().Sub(deadline), ShouldBeBetween, -time.Millisecond, time.Millisecond)
				})

				Convey(`signal`, func() {
					So(self.Signal(os.Interrupt), ShouldBeNil)
					<-cleanup // cleanup unblocks on signal

					So(ac, ShouldNotBeDone)

					tc.Add(35 * time.Second)
					So(ac, ShouldBeDone)

					// Should have about 10s of time left before the deadline.
					So(tc.Now().Sub(deadline)+(10*time.Second), ShouldBeBetween,
						-time.Millisecond, time.Millisecond)
				})
			})

		})
	})
}
