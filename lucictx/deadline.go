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
	"math"
	"os"
	"os/signal"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/signals"
)

// signalNotify/signalStop are used to mock signal.Notify in tests.
var signalNotify = signal.Notify
var signalStop = signal.Stop

// DefaultGracePeriod is the value of Deadline.grace_period to assume
// if Deadline is entirely missing in LUCI_CONTEXT.
const DefaultGracePeriod = 30 * time.Second

func secsToDuration(secs float64) time.Duration {
	return time.Duration(secs * float64(time.Second))
}

func unixFloatToTime(t float64) time.Time {
	int, frac := math.Modf(t)
	return time.Unix(int64(int), int64(frac*1e9)).UTC()
}

func timeToUnixFloat(t time.Time) float64 {
	return float64(t.Unix()) + (float64(t.Nanosecond()) / 1e9)
}

// DeadlineEvent is the type pushed into the cleanup channel returned by
// AdjustDeadline.
type DeadlineEvent int

// The cleanup channel, when it unblocks, will have an infinite supply of one of
// the following event types.
const (
	// ClosureEvent occurs when the context returned by AdjustDeadline is Done.
	// This is the value you'll get from `cleanup` when it's closed.
	ClosureEvent DeadlineEvent = iota

	// InterruptEvent occurs when a SIGTERM/os.Interrupt was handled to unblock
	// the cleanup channel.
	InterruptEvent

	// TimeoutEvent occurs when the cleanup channel was unblocked due to
	// a timeout on deadline-gracePeriod.
	TimeoutEvent
)

// earlier returns the earlier of a and b, treating "zero" as "infinity".
func earlier(a, b time.Time) time.Time {
	if a.IsZero() {
		return b
	}
	if b.IsZero() {
		return a
	}
	if a.Before(b) {
		return a
	}
	return b
}

// AdjustDeadline returns a 'cleanup' channel and a new 'shutdown'-able context
// based on the combination of the input context's deadline, as well as the
// Deadline information in LUCI_CONTEXT.
//
// This function allows you to reserve a portion of the deadline and/or
// grace_period with `reserve` and `reserveCleanup` respectively.
//
// reserve and reserveCleanup must not be less than 0 or this panics.
//
// First, if Deadline is missing from LUCI_CONTEXT, it is filled in with the
// default:
//
//   {soft_deadline: infinity, grace_period: 30}
//
// We then calculate:
//
//   adjustedSoftDeadline = earlier(
//     ctx.Deadline() - gracePeriod,
//     Deadline.soft_deadline,
//   ) - reserve
//
// Next, `reserveCleanup` is subtracted from Deadline.grace_period.
// This becomes the `adjustedGracePeriod`.
//
// The `cleanup` channel is set up with a goroutine which:
//   * On shutdown()/SIGTERM/os.Interrupt, continuously sends InterruptEvent
//     until it closes ctx.Done after `adjustedGracePeriod`, where it switches
//     to ClosureEvent.
//   * On `adjustedSoftDeadline`, continuously sends TimeoutEvent until
//     ctx.Done, where it switches to ClosureEvent. The ctx will meet its hard
//     deadline after `adjustedGracePeriod`.
//   * On ctx.Done, continuously sends ClosureEvent.
//
// Finally, the returned context has its deadline set to
// `adjustedSoftDeadline+adjustedGracePeriod`, and LUCI_CONTEXT['deadline'] is
// populated with the adjusted soft_deadline and grace_period.
//
// The returned shutdown() function will begin the shutdown process (acts as if
// an os.Interrupt signal was triggered). After calling shutdown, you may block
// on ctx.Done() which will take up to adjustedGracePeriod to occur.
//
// Note, if Deadline.grace_period is insufficient to cover reserveCleanup
// (including if reserveCleanup>DefaultGracePeriodSecs and no Deadline was in
// LUCI_CONTEXT at all), this function panics.
//
// Example:
//
//    func MainFunc(ctx context.Context) {
//      // ctx.Deadline  = unix(t0+5:40)
//      // soft_deadline = unix(t0+5:00)
//      // grace_period  = 40
//      cleanup, cctx, shutdown := lucictx.AdjustDeadline(ctx, time.Minute, 500*time.Millisecond)
//      defer shutdown()
//      ScopedFunction(cctx, cleanup)
//
//      // Assuming ScopedFunction exits in a timely fashion on ctx.Done, you
//      // have at least 500ms to do stuff here. Alternately you could wait on
//      // `<-cleanup` and have 40s to do stuff, but ScopedFunction may not
//      // have finished yet.
//      //
//      // Note that if `<-cleanup` is TimeoutEvent, you could have a whole
//      // minute (i.e. we haven't been interrupted yet, just getting close to
//      // the deadline).
//    }
//
//    func ScopedFunction(ctx context.Context, cleanup <-chan struct{}) {
//      // ctx.Deadline  = unix(t0+4:39.5)
//      // soft_deadline = unix(t0+4:00)
//      // grace_period  = 39.5
//
//      go func() {
//        // cleanup is unblocked at SIGTERM, $soft_deadline or shutdown(),
//        // whichever is first.
//        <-cleanup
//        // have grace_period to do something (say, send SIGTERM to a child)
//        // before ctx.Done().
//      }()
//    }
//
// NOTE: In the event that `ctx` is canceled, everything immediately moves to
// the 'Kill' phase without providing any grace period.
func AdjustDeadline(ctx context.Context, reserve, reserveCleanup time.Duration) (cleanup <-chan DeadlineEvent, newCtx context.Context, shutdown func()) {
	if reserve < 0 {
		panic(errors.Reason("reserve(%d) < 0", reserve).Err())
	}
	if reserveCleanup < 0 {
		panic(errors.Reason("reserveCleanup(%d) < 0", reserveCleanup).Err())
	}

	d := GetDeadline(ctx)
	if d == nil {
		logging.Warningf(
			ctx, "AdjustDeadline without Deadline in LUCI_CONTEXT. "+
				"Assuming Deadline={grace_period: %d}", DefaultGracePeriod.Seconds())
		d = &Deadline{GracePeriod: DefaultGracePeriod.Seconds()}
	}

	// Adjust grace period.
	origGracePeriod := secsToDuration(d.GracePeriod)
	adjustedGrace := origGracePeriod - reserveCleanup
	if adjustedGrace < 0 {
		panic(errors.Reason(
			"reserveCleanup(%f) > gracePeriod(%f)", reserveCleanup.Seconds(), d.GracePeriod).Err())
	}
	d.GracePeriod = adjustedGrace.Seconds()

	// find adjustedSoftDeadline
	var ctxSoftDeadline time.Time
	if d, ok := ctx.Deadline(); ok {
		ctxSoftDeadline = d.Add(-origGracePeriod)
	}
	var lucictxSoftDeadline time.Time
	if d.SoftDeadline != 0 {
		lucictxSoftDeadline = unixFloatToTime(d.SoftDeadline)
	}
	adjustedSoftDeadline := earlier(ctxSoftDeadline, lucictxSoftDeadline)

	var newCtxCancel func()

	// Set up hard deadline in context, set Deadline.soft_deadline
	if !adjustedSoftDeadline.IsZero() {
		adjustedSoftDeadline = adjustedSoftDeadline.Add(-reserve)
		d.SoftDeadline = timeToUnixFloat(adjustedSoftDeadline)
		// we add adjustedGrace back because the ctx deadline is the HARD deadline;
		// it must occur `adjustedGrace` after the SoftDeadline.
		//
		// note that if adjustedSoftDeadline+adjustedGrace is in the past, this
		// returns newCtx as already Done.
		newCtx, newCtxCancel = clock.WithDeadline(ctx, adjustedSoftDeadline.Add(adjustedGrace))
	}

	newCtx = SetDeadline(newCtx, d)

	cleanup, shutdown = runDeadlineMonitor(newCtx, newCtxCancel, adjustedSoftDeadline, adjustedGrace)

	return
}

func runDeadlineMonitor(ctx context.Context, cancel func(), adjustedSoftDeadline time.Time, gracePeriod time.Duration) (<-chan DeadlineEvent, func()) {
	cleanupCh := make(chan DeadlineEvent)
	// buffer 1 is essential; otherwise signals will be missed if our goroutine
	// isn't currently blocked on sigCh. With a buffer of 1, sigCh is always ready
	// to send.
	sigCh := make(chan os.Signal, 1)
	signalNotify(sigCh, signals.Interrupts()...)

	var timeoutC <-chan clock.TimerResult
	if !adjustedSoftDeadline.IsZero() {
		timeoutC = clock.After(ctx, clock.Until(ctx, adjustedSoftDeadline))
	}

	go func() {
		if cancel != nil {
			defer cancel()
		}
		defer close(cleanupCh) // will switch to ClosureEvent

		evt := func() DeadlineEvent {
			defer signalStop(sigCh)

			select {
			case <-timeoutC:
				return TimeoutEvent
			case <-ctx.Done():
				return ClosureEvent
			case <-sigCh:
				return InterruptEvent
			}
		}()

		if evt == InterruptEvent {
			timeoutC = clock.After(ctx, gracePeriod)
		} else {
			timeoutC = nil
		}

		for {
			// bias towards context completion, otherwise cleanupCh<- and <-ctx.Done
			// are equally "ready" cases and golang can starve the ctx done-ness.
			select {
			case <-ctx.Done():
				return
			default:
			}

			select {
			case <-ctx.Done():
				return
			case <-timeoutC:
				return
			case cleanupCh <- evt:
			}
		}
	}()

	return cleanupCh, func() {
		// shutdown func just interrupts on sigCh; multiple calls will have no
		// effect since we only listen to sigCh exactly once.
		select {
		case sigCh <- os.Interrupt:
		default:
		}
	}
}

// GetDeadline retrieves the raw Deadline information from the context.
//
// You probably want to use AdjustDeadline instead.
func GetDeadline(ctx context.Context) *Deadline {
	t := Deadline{}
	ok, err := Lookup(ctx, "deadline", &t)
	if err != nil {
		panic(err)
	}
	if !ok {
		return nil
	}
	return &t
}

// SetDeadline sets the raw Deadline information in the context.
//
// If d is nil, sets a default deadline of:
//   {grace_period: DefaultGracePeriod}
//
// If d.deadline == 0, adjusts it to ctx.Deadline() - d.grace_period.
//
// You probably want to use AdjustDeadline instead.
func SetDeadline(ctx context.Context, d *Deadline) context.Context {
	if d == nil {
		d = &Deadline{GracePeriod: DefaultGracePeriod.Seconds()}
	}
	if deadline, ok := ctx.Deadline(); ok && d.SoftDeadline == 0 {
		d.SoftDeadline = timeToUnixFloat(deadline) - d.GracePeriod
	}
	return Set(ctx, "deadline", d)
}
