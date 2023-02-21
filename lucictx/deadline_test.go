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
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/signals"
	. "go.chromium.org/luci/common/testing/assertions"
)

// shouldWaitForNotDone tests if the context's .Done() channel is still blocked.
func shouldWaitForNotDone(actual any, expected ...any) string {
	if len(expected) > 0 {
		return fmt.Sprintf("shouldWaitForNotDone requires 0 values, got %d", len(expected))
	}

	if actual == nil {
		return ShouldNotBeNil(actual)
	}

	ctx, ok := actual.(context.Context)
	if !ok {
		return ShouldHaveSameTypeAs(actual, context.Context(nil))
	}

	if ctx == nil {
		return ShouldNotBeNil(actual)
	}

	select {
	case <-ctx.Done():
		return "Expected context NOT to be Done(), but it was."
	case <-time.After(100 * time.Millisecond):
		return ""
	}
}

var mockSigMu = sync.Mutex{}
var mockSigSet = make(map[chan<- os.Signal]struct{})

func mockGenerateInterrupt() {
	mockSigMu.Lock()
	defer mockSigMu.Unlock()

	if len(mockSigSet) == 0 {
		panic(errors.New(
			"mockGenerateInterrupt but no handlers registered; Would have terminated program"))
	}

	for ch := range mockSigSet {
		select {
		case ch <- os.Interrupt:
		default:
		}
	}
}

func assertEmptySignals() {
	mockSigMu.Lock()
	defer mockSigMu.Unlock()
	So(mockSigSet, ShouldBeEmpty)
}

func init() {
	interrupts := signals.Interrupts()
	checkSig := func(sig os.Signal) {
		for _, okSig := range interrupts {
			if sig == okSig {
				return
			}
		}
		panic(errors.Reason("unsupported mock signal: %s", sig).Err())
	}

	signalNotify = func(ch chan<- os.Signal, sigs ...os.Signal) {
		for _, sig := range sigs {
			checkSig(sig)
		}
		mockSigMu.Lock()
		mockSigSet[ch] = struct{}{}
		mockSigMu.Unlock()
	}

	signalStop = func(ch chan<- os.Signal) {
		mockSigMu.Lock()
		delete(mockSigSet, ch)
		mockSigMu.Unlock()
	}
}

func TestDeadline(t *testing.T) {
	// not Parallel because this uses the global mock signalNotify.
	// t.Parallel()

	Convey(`TrackSoftDeadline`, t, func() {
		t0 := testclock.TestTimeUTC
		ctx, tc := testclock.UseTime(context.Background(), t0)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		defer assertEmptySignals()

		// we explicitly remove the section to make these tests work correctly when
		// run in a context using LUCI_CONTEXT.
		ctx = Set(ctx, "deadline", nil)

		Convey(`Empty context`, func() {
			ac, shutdown := TrackSoftDeadline(ctx, 5*time.Second)
			defer shutdown()

			deadline, ok := ac.Deadline()
			So(ok, ShouldBeFalse)
			So(deadline.IsZero(), ShouldBeTrue)

			// however, Interrupt/SIGTERM handler is still installed
			mockGenerateInterrupt()

			// soft deadline will happen, but context.Done won't.
			So(<-SoftDeadlineDone(ac), ShouldEqual, InterruptEvent)
			So(ac, shouldWaitForNotDone)

			// Advance the clock by 25s, and presto
			tc.Add(25 * time.Second)
			<-ac.Done()
		})

		Convey(`deadline context`, func() {
			ctx, cancel := clock.WithDeadline(ctx, t0.Add(100*time.Second))
			defer cancel()

			ac, shutdown := TrackSoftDeadline(ctx, 5*time.Second)
			defer shutdown()

			hardDeadline, ok := ac.Deadline()
			So(ok, ShouldBeTrue)
			// hard deadline is still 95s because we the presumed grace period for the
			// context was 30s, but we reserved 5s for cleanup. Thus, this should end
			// 5s before the overall deadline,
			So(hardDeadline, ShouldEqual, t0.Add(95*time.Second))
			got := GetDeadline(ac)

			expect := &Deadline{GracePeriod: 25}
			// SoftDeadline is always GracePeriod earlier than the hard (context)
			// deadline.
			expect.SetSoftDeadline(t0.Add(70 * time.Second))
			So(got, ShouldResembleProto, expect)
			shutdown()
			<-SoftDeadlineDone(ac) // force monitor to make timer before we increment the clock
			tc.Add(25 * time.Second)
			<-ac.Done()
		})

		Convey(`deadline context reserve`, func() {
			ctx, cancel := clock.WithDeadline(ctx, t0.Add(95*time.Second))
			defer cancel()

			ac, shutdown := TrackSoftDeadline(ctx, 0)
			defer shutdown()

			deadline, ok := ac.Deadline()
			So(ok, ShouldBeTrue)
			// hard deadline is 95s because we reserved 5s.
			So(deadline, ShouldEqual, t0.Add(95*time.Second))
			got := GetDeadline(ac)

			expect := &Deadline{GracePeriod: 30}
			// SoftDeadline is always GracePeriod earlier than the hard (context)
			// deadline.
			expect.SetSoftDeadline(t0.Add(65 * time.Second))
			So(got, ShouldResembleProto, expect)
			shutdown()
			<-SoftDeadlineDone(ac) // force monitor to make timer before we increment the clock
			tc.Add(30 * time.Second)
			<-ac.Done()
		})

		Convey(`Deadline in LUCI_CONTEXT`, func() {
			externalSoftDeadline := t0.Add(100 * time.Second)

			// Note, LUCI_CONTEXT asserts that non-zero SoftDeadlines must be enforced
			// by 'an external process', so we mock that with the goroutine here.
			//
			// Must do clock.After outside goroutine to force this time calculation to
			// happen before we start manipulating `tc`.
			externalTimeout := clock.After(ctx, 100*time.Second)
			go func() {
				if (<-externalTimeout).Err == nil {
					mockGenerateInterrupt()
				}
			}()

			dl := &Deadline{GracePeriod: 40}
			dl.SetSoftDeadline(externalSoftDeadline) // 100s into the future

			ctx := SetDeadline(ctx, dl)

			Convey(`no deadline in context`, func() {
				ac, shutdown := TrackSoftDeadline(ctx, 5*time.Second)
				defer shutdown()

				softDeadline := GetDeadline(ac).SoftDeadlineTime()
				So(softDeadline, ShouldHappenWithin, time.Millisecond, externalSoftDeadline)

				hardDeadline, ok := ac.Deadline()
				So(ok, ShouldBeTrue)
				// hard deadline is soft deadline + adjusted grace period.
				// Cleanup reservation of 5s means that the adjusted grace period is
				// 35s.
				So(hardDeadline, ShouldHappenWithin, time.Millisecond, externalSoftDeadline.Add(35*time.Second))

				Convey(`natural expiration`, func() {
					tc.Add(100 * time.Second)
					So(<-SoftDeadlineDone(ac), ShouldEqual, TimeoutEvent)
					So(ac, shouldWaitForNotDone)

					tc.Add(35 * time.Second)
					<-ac.Done()

					// We should have ended right around the deadline; there's some slop
					// in the clock package though, and this doesn't seem to be zero.
					So(tc.Now(), ShouldHappenWithin, time.Millisecond, hardDeadline)
				})

				Convey(`signal`, func() {
					mockGenerateInterrupt()
					So(<-SoftDeadlineDone(ac), ShouldEqual, InterruptEvent)

					So(ac, shouldWaitForNotDone)

					tc.Add(35 * time.Second)
					<-ac.Done()

					// should still have 65s before the soft deadline
					So(tc.Now(), ShouldHappenWithin, time.Millisecond, softDeadline.Add(-65*time.Second))
				})

				Convey(`cancel context`, func() {
					cancel()
					So(<-SoftDeadlineDone(ac), ShouldEqual, ClosureEvent)
					<-ac.Done()
				})
			})

			Convey(`earlier deadline in context`, func() {
				ctx, cancel := clock.WithDeadline(ctx, externalSoftDeadline.Add(-50*time.Second))
				defer cancel()

				ac, shutdown := TrackSoftDeadline(ctx, 5*time.Second)
				defer shutdown()

				hardDeadline, ok := ac.Deadline()
				So(ok, ShouldBeTrue)
				So(hardDeadline, ShouldEqual, externalSoftDeadline.Add(-55*time.Second))

				Convey(`natural expiration`, func() {
					tc.Add(10 * time.Second)
					So(<-SoftDeadlineDone(ac), ShouldEqual, TimeoutEvent)
					So(ac, shouldWaitForNotDone)

					tc.Add(35 * time.Second)
					<-ac.Done()

					// We should have ended right around the deadline; there's some slop
					// in the clock package though, and this doesn't seem to be zero.
					So(tc.Now(), ShouldHappenWithin, time.Millisecond, hardDeadline)
				})

				Convey(`signal`, func() {
					mockGenerateInterrupt()
					So(<-SoftDeadlineDone(ac), ShouldEqual, InterruptEvent)

					So(ac, shouldWaitForNotDone)

					tc.Add(35 * time.Second)
					<-ac.Done()

					// Should have about 10s of time left before the deadline.
					So(tc.Now(), ShouldHappenWithin, time.Millisecond, hardDeadline.Add(-10*time.Second))
				})
			})

		})
	})
}
