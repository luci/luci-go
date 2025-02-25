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
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/signals"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

// shouldWaitForNotDone tests if the context's .Done() channel is still blocked.
func shouldWaitForNotDone(ctx context.Context, t testing.TB) {
	t.Helper()
	select {
	case <-ctx.Done():
		t.Fatal("Expected context NOT to be Done(), but it was.")

	case <-time.After(100 * time.Millisecond):
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

func clearSignalSet() {
	mockSigMu.Lock()
	defer mockSigMu.Unlock()
	mockSigSet = make(map[chan<- os.Signal]struct{})
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

	ftt.Run(`TrackSoftDeadline`, t, func(t *ftt.Test) {
		t0 := testclock.TestTimeUTC
		ctx, tc := testclock.UseTime(context.Background(), t0)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		defer clearSignalSet()

		// we explicitly remove the section to make these tests work correctly when
		// run in a context using LUCI_CONTEXT.
		ctx = Set(ctx, "deadline", nil)

		t.Run(`Empty context`, func(t *ftt.Test) {
			ac, shutdown := TrackSoftDeadline(ctx, 5*time.Second)
			defer shutdown()

			deadline, ok := ac.Deadline()
			assert.Loosely(t, ok, should.BeFalse)
			assert.Loosely(t, deadline.IsZero(), should.BeTrue)

			// however, Interrupt/SIGTERM handler is still installed
			mockGenerateInterrupt()

			// soft deadline will happen, but context.Done won't.
			assert.Loosely(t, <-SoftDeadlineDone(ac), should.Equal(InterruptEvent))
			shouldWaitForNotDone(ac, t)

			// Advance the clock by 25s, and presto
			tc.Add(25 * time.Second)
			<-ac.Done()
		})

		t.Run(`deadline context`, func(t *ftt.Test) {
			ctx, cancel := clock.WithDeadline(ctx, t0.Add(100*time.Second))
			defer cancel()

			ac, shutdown := TrackSoftDeadline(ctx, 5*time.Second)
			defer shutdown()

			hardDeadline, ok := ac.Deadline()
			assert.Loosely(t, ok, should.BeTrue)
			// hard deadline is still 95s because we the presumed grace period for the
			// context was 30s, but we reserved 5s for cleanup. Thus, this should end
			// 5s before the overall deadline,
			assert.Loosely(t, hardDeadline, should.Match(t0.Add(95*time.Second)))
			got := GetDeadline(ac)

			expect := &Deadline{GracePeriod: 25}
			// SoftDeadline is always GracePeriod earlier than the hard (context)
			// deadline.
			expect.SetSoftDeadline(t0.Add(70 * time.Second))
			assert.Loosely(t, got, should.Match(expect))
			shutdown()
			<-SoftDeadlineDone(ac) // force monitor to make timer before we increment the clock
			tc.Add(25 * time.Second)
			<-ac.Done()
		})

		t.Run(`deadline context reserve`, func(t *ftt.Test) {
			ctx, cancel := clock.WithDeadline(ctx, t0.Add(95*time.Second))
			defer cancel()

			ac, shutdown := TrackSoftDeadline(ctx, 0)
			defer shutdown()

			deadline, ok := ac.Deadline()
			assert.Loosely(t, ok, should.BeTrue)
			// hard deadline is 95s because we reserved 5s.
			assert.Loosely(t, deadline, should.Match(t0.Add(95*time.Second)))
			got := GetDeadline(ac)

			expect := &Deadline{GracePeriod: 30}
			// SoftDeadline is always GracePeriod earlier than the hard (context)
			// deadline.
			expect.SetSoftDeadline(t0.Add(65 * time.Second))
			assert.Loosely(t, got, should.Match(expect))
			shutdown()
			<-SoftDeadlineDone(ac) // force monitor to make timer before we increment the clock
			tc.Add(30 * time.Second)
			<-ac.Done()
		})

		t.Run(`Deadline in LUCI_CONTEXT`, func(t *ftt.Test) {
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

			t.Run(`no deadline in context`, func(t *ftt.Test) {
				ac, shutdown := TrackSoftDeadline(ctx, 5*time.Second)
				defer shutdown()

				softDeadline := GetDeadline(ac).SoftDeadlineTime()
				assert.Loosely(t, softDeadline, should.HappenWithin(time.Millisecond, externalSoftDeadline))

				hardDeadline, ok := ac.Deadline()
				assert.Loosely(t, ok, should.BeTrue)
				// hard deadline is soft deadline + adjusted grace period.
				// Cleanup reservation of 5s means that the adjusted grace period is
				// 35s.
				assert.Loosely(t, hardDeadline, should.HappenWithin(time.Millisecond, externalSoftDeadline.Add(35*time.Second)))

				t.Run(`natural expiration`, func(t *ftt.Test) {
					tc.Add(100 * time.Second)
					assert.Loosely(t, <-SoftDeadlineDone(ac), should.Equal(TimeoutEvent))
					shouldWaitForNotDone(ac, t)

					tc.Add(35 * time.Second)
					<-ac.Done()

					// We should have ended right around the deadline; there's some slop
					// in the clock package though, and this doesn't seem to be zero.
					assert.Loosely(t, tc.Now(), should.HappenWithin(time.Millisecond, hardDeadline))
				})

				t.Run(`signal`, func(t *ftt.Test) {
					mockGenerateInterrupt()
					assert.Loosely(t, <-SoftDeadlineDone(ac), should.Equal(InterruptEvent))

					shouldWaitForNotDone(ac, t)

					tc.Add(35 * time.Second)
					<-ac.Done()

					// should still have 65s before the soft deadline
					assert.Loosely(t, tc.Now(), should.HappenWithin(time.Millisecond, softDeadline.Add(-65*time.Second)))
				})

				t.Run(`cancel context`, func(t *ftt.Test) {
					cancel()
					assert.Loosely(t, <-SoftDeadlineDone(ac), should.Equal(ClosureEvent))
					<-ac.Done()
				})
			})

			t.Run(`earlier deadline in context`, func(t *ftt.Test) {
				ctx, cancel := clock.WithDeadline(ctx, externalSoftDeadline.Add(-50*time.Second))
				defer cancel()

				ac, shutdown := TrackSoftDeadline(ctx, 5*time.Second)
				defer shutdown()

				hardDeadline, ok := ac.Deadline()
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, hardDeadline, should.Match(externalSoftDeadline.Add(-55*time.Second)))

				t.Run(`natural expiration`, func(t *ftt.Test) {
					tc.Add(10 * time.Second)
					assert.Loosely(t, <-SoftDeadlineDone(ac), should.Equal(TimeoutEvent))
					shouldWaitForNotDone(ac, t)

					tc.Add(35 * time.Second)
					<-ac.Done()

					// We should have ended right around the deadline; there's some slop
					// in the clock package though, and this doesn't seem to be zero.
					assert.Loosely(t, tc.Now(), should.HappenWithin(time.Millisecond, hardDeadline))
				})

				t.Run(`signal`, func(t *ftt.Test) {
					mockGenerateInterrupt()
					assert.Loosely(t, <-SoftDeadlineDone(ac), should.Equal(InterruptEvent))

					shouldWaitForNotDone(ac, t)

					tc.Add(35 * time.Second)
					<-ac.Done()

					// Should have about 10s of time left before the deadline.
					assert.Loosely(t, tc.Now(), should.HappenWithin(time.Millisecond, hardDeadline.Add(-10*time.Second)))
				})
			})

		})
	})
}
