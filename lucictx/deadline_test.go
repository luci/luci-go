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

// shouldWaitForDone tests if the context's .Done() channel is unblocked.
func shouldWaitForDone(actual interface{}, expected ...interface{}) string {
	if len(expected) > 0 {
		return fmt.Sprintf("shouldWaitForDone requires 0 values, got %d", len(expected))
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
		return ""
	case <-time.After(100 * time.Millisecond):
		return "Expected context to be Done(), but it wasn't."
	}
}

// shouldWaitForNotDone tests if the context's .Done() channel is still blocked.
func shouldWaitForNotDone(actual interface{}, expected ...interface{}) string {
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

	Convey(`AdjustDeadline`, t, func() {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestTimeUTC)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		defer assertEmptySignals()

		// we explicitly remove the section to make these tests work correctly when
		// run in a context using LUCI_CONTEXT.
		ctx = Set(ctx, "deadline", nil)

		Convey(`Empty context`, func() {
			cleanup, ac, accancel := AdjustDeadline(ctx, 0, 5*time.Second)
			defer accancel()

			deadline, ok := ac.Deadline()
			So(ok, ShouldBeFalse)
			So(deadline.IsZero(), ShouldBeTrue)

			// however, Interrupt/SIGTERM handler is still installed
			mockGenerateInterrupt()

			// cleanup will happen, but context won't.
			So(<-cleanup, ShouldEqual, InterruptEvent)
			So(ac, shouldWaitForNotDone)

			// Advance the clock by 25s, and presto
			tc.Add(25 * time.Second)
			So(ac, shouldWaitForDone)
		})

		Convey(`deadline context`, func() {
			ctx, cancel := clock.WithDeadline(ctx, clock.Now(ctx).Add(100*time.Second))
			defer cancel()

			_, ac, accancel := AdjustDeadline(ctx, 0, 5*time.Second)
			defer accancel()

			deadline, ok := ac.Deadline()
			So(ok, ShouldBeTrue)
			So(deadline.IsZero(), ShouldBeFalse)
			got := GetDeadline(ac)

			So(got, ShouldResembleProto, &Deadline{
				Deadline:    timeToUnixFloat(testclock.TestTimeUTC.Add(95 * time.Second)),
				GracePeriod: 25,
			})
		})

		Convey(`Deadline in LUCI_CONTEXT`, func() {
			luciDeadline := testclock.TestTimeUTC.Add(100 * time.Second)
			ctx = SetDeadline(ctx, &Deadline{
				Deadline:    timeToUnixFloat(luciDeadline), // 100s into the future
				GracePeriod: 40,
			})

			Convey(`no deadline in context`, func() {
				cleanup, ac, accancel := AdjustDeadline(ctx, 0, 5*time.Second)
				defer accancel()

				deadline, ok := ac.Deadline()
				So(ok, ShouldBeTrue)
				So(deadline, ShouldHappenWithin, time.Millisecond, luciDeadline.Add(-5*time.Second))

				Convey(`natural expiration`, func() {
					tc.Add((95 - 35) * time.Second)
					So(<-cleanup, ShouldEqual, TimeoutEvent) // cleanup unblocks
					So(ac, shouldWaitForNotDone)

					tc.Add(35 * time.Second)
					So(ac, shouldWaitForDone)

					// We should have ended right around the deadline; there's some slop
					// in the clock package though, and this doesn't seem to be zero.
					So(tc.Now(), ShouldHappenWithin, time.Millisecond, deadline)
				})

				Convey(`signal`, func() {
					mockGenerateInterrupt()
					So(<-cleanup, ShouldEqual, InterruptEvent) // cleanup unblocks on signal

					So(ac, shouldWaitForNotDone)

					tc.Add(35 * time.Second)
					So(ac, shouldWaitForDone)

					// Should have about 1m of time left before the deadline.
					So(tc.Now(), ShouldHappenWithin, time.Millisecond, deadline.Add(-time.Minute))
				})
			})

			Convey(`reduce deadline by more than grace_period`, func() {
				cleanup, ac, accancel := AdjustDeadline(ctx, 50*time.Second, time.Second)
				defer accancel()

				deadline, ok := ac.Deadline()
				So(ok, ShouldBeTrue)
				So(deadline, ShouldHappenWithin, time.Millisecond, luciDeadline.Add(-50*time.Second))

				Convey(`natural expiration`, func() {
					tc.Add(49 * time.Second)
					So(<-cleanup, ShouldEqual, TimeoutEvent) // cleanup unblocks
					So(ac, shouldWaitForNotDone)

					tc.Add(time.Second)
					So(ac, shouldWaitForDone)

					// We should have ended right around the deadline; there's some slop
					// in the clock package though, and this doesn't seem to be zero.
					So(tc.Now(), ShouldHappenWithin, time.Millisecond, deadline)
					// cleanup will close
					for range cleanup {
					}
				})

				Convey(`signal`, func() {
					mockGenerateInterrupt()
					So(<-cleanup, ShouldEqual, InterruptEvent) // cleanup unblocks on signal

					So(ac, shouldWaitForNotDone)

					tc.Add(39 * time.Second)
					So(ac, shouldWaitForDone)

					// Should have about 10s of time left before the deadline.
					So(tc.Now(), ShouldHappenWithin, time.Millisecond, deadline.Add(-11*time.Second))
					// cleanup will close
					for range cleanup {
					}
				})

			})

			Convey(`earlier deadline in context`, func() {
				ctx, cancel := clock.WithDeadline(ctx, luciDeadline.Add(-50*time.Second))
				defer cancel()

				cleanup, ac, accancel := AdjustDeadline(ctx, 0, 5*time.Second)
				defer accancel()

				deadline, ok := ac.Deadline()
				So(ok, ShouldBeTrue)
				So(deadline, ShouldEqual, luciDeadline.Add(-55*time.Second))

				Convey(`natural expiration`, func() {
					tc.Add(10 * time.Second)
					So(<-cleanup, ShouldEqual, TimeoutEvent) // cleanup unblocks
					So(ac, shouldWaitForNotDone)

					tc.Add(35 * time.Second)
					So(ac, shouldWaitForDone)

					// We should have ended right around the deadline; there's some slop
					// in the clock package though, and this doesn't seem to be zero.
					So(tc.Now(), ShouldHappenWithin, time.Millisecond, deadline)
				})

				Convey(`signal`, func() {
					mockGenerateInterrupt()
					So(<-cleanup, ShouldEqual, InterruptEvent) // cleanup unblocks on signal

					So(ac, shouldWaitForNotDone)

					tc.Add(35 * time.Second)
					So(ac, shouldWaitForDone)

					// Should have about 10s of time left before the deadline.
					So(tc.Now(), ShouldHappenWithin, time.Millisecond, deadline.Add(-10*time.Second))
					// cleanup will close
					for range cleanup {
					}
				})
			})

		})
	})
}
