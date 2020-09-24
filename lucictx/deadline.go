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

// AdjustDeadline returns a 'cleanup' channel and a new cancelable context based
// on the combination of the input context's deadline, as well as the Deadline
// information in LUCI_CONTEXT.
//
// This function allows you to reserve a portion of the deadline and/or
// grace_period with `reserve` and `reserveEmergency` respectively.
//
// reserve and reserveEmergency must not be less than 0 or this panics.
// If reserve < reserveEmergency, reserve is bumped up to match
//   reserveEmergency.
//
// First, if Deadline is missing from LUCI_CONTEXT, it is filled in with the
// default:
//
//   {deadline: infinity, grace_period: 30}
//
// Next, the earlier of the input ctx.Deadline and Deadline.deadline values is
// computed. `reserve` is subtracted from this and it becomes the new
// `adjustedDeadline`.
//
// Next, `reserveEmergency` is subtracted from Deadline.grace_period.
// This becomes the `adjustedGracePeriod`.
//
// The `cleanup` channel is set up with a goroutine which:
//   * On SIGTERM/os.Interrupt, continuously sends InterruptEvent until it
//     closes ctx.Done after adjustedDeadline, where it switches to ClosureEvent.
//   * On `adjustedDeadline-adjustedGracePeriod`, continuously sends
//     TimeoutEvent until ctx.Done, where it switches to ClosureEvent.
//   * On ctx.Done, continuously sends ClosureEvent.
//
// Finally, the returned context has its deadline set to `adjustedDeadline`, and
// LUCI_CONTEXT['deadline'] is populated with the adjusted deadline/gracePeriod.
//
// Note, if Deadline.grace_period is insufficient to cover reserveCleanup
// (including if reserveCleanup>DefaultGracePeriodSecs and no Deadline was in
// LUCI_CONTEXT at all), this function panics.
//
// Example:
//
//    func MainFunc(ctx context.Context) {
//      // deadline     = unix(t0+5:00)
//      // grace_period = 40
//      cleanup, cctx, cancel := lucictx.AdjustDeadline(ctx, time.Minute, 500*time.Millisecond)
//      defer cancel()
//      ScopedFunction(cctx, cleanup)
//
//      // Assuming ScopedFunction exits in a timely fashion on ctx.Done, you
//      // have at least 500ms to do stuff here. Alternately you could wait on
//      // `<-cleanup` and have at least 40s to do stuff, but ScopedFunction may
//      // not have finished yet.
//      //
//      // Note that if `<-cleanup` is TimeoutEvent, you could have a whole
//      // minute (i.e. we haven't been interrupted yet, just getting close to
//      // the deadline).
//    }
//
//    func ScopedFunction(ctx context.Context, cleanup <-chan struct{}) {
//      // deadline     = unix(t0+4:00)
//      // grace_period = 39.5
//      // Otherwise Deadline is still not in LUCI_CONTEXT.
//
//      go func() {
//        // cleanup is closed at SIGTERM or $deadline-$grace_period,
//        // whichever is first.
//        <-cleanup
//        // have grace_period to do something (say, send SIGTERM to a child)
//        // before ctx.Done().
//      }()
//    }
//
// NOTE: In the event that `ctx` is canceled, `cleanup` will not be closed.
func AdjustDeadline(ctx context.Context, reserve, reserveEmergency time.Duration) (cleanup <-chan DeadlineEvent, newCtx context.Context, cancel func()) {
	if reserve < 0 {
		panic(errors.Reason("reserve(%d) < 0", reserve).Err())
	}
	if reserveEmergency < 0 {
		panic(errors.Reason("reserveEmergency(%d) < 0", reserveEmergency).Err())
	}
	if reserveEmergency > reserve {
		reserve = reserveEmergency
	}

	d := GetDeadline(ctx)
	if d == nil {
		logging.Warningf(
			ctx, "AdjustDeadline without Deadline in LUCI_CONTEXT. "+
				"Assuming Deadline={grace_period: %d}", DefaultGracePeriod.Seconds())
		d = &Deadline{GracePeriod: DefaultGracePeriod.Seconds()}
	}

	// Adjust grace period.
	adjustedGrace := secsToDuration(d.GracePeriod) - reserveEmergency
	if adjustedGrace < 0 {
		panic(errors.Reason(
			"reserveEmergency(%f) > gracePeriod(%f)", reserveEmergency.Seconds(), d.GracePeriod).Err())
	}
	d.GracePeriod = adjustedGrace.Seconds()

	// note: 0 indicates an 'infinite' deadline.
	var ctxDeadline time.Time
	if d, ok := ctx.Deadline(); ok {
		ctxDeadline = d
	}
	var lucictxDeadline time.Time
	if d.Deadline != 0 {
		lucictxDeadline = unixFloatToTime(d.Deadline)
	}

	var adjustedDeadline time.Time

	var newCtxCancel func()

	switch {
	case ctxDeadline.IsZero() && !lucictxDeadline.IsZero():
		adjustedDeadline = lucictxDeadline
	case !ctxDeadline.IsZero() && lucictxDeadline.IsZero():
		adjustedDeadline = ctxDeadline
	case !ctxDeadline.IsZero() && !lucictxDeadline.IsZero():
		if ctxDeadline.Before(lucictxDeadline) {
			adjustedDeadline = ctxDeadline
		} else {
			adjustedDeadline = lucictxDeadline
		}
	}
	if !adjustedDeadline.IsZero() {
		adjustedDeadline = adjustedDeadline.Add(-reserve)
		newCtx, newCtxCancel = clock.WithDeadline(ctx, adjustedDeadline)
		d.Deadline = timeToUnixFloat(adjustedDeadline)
	} else {
		newCtx, newCtxCancel = context.WithCancel(ctx)
	}

	newCtx = SetDeadline(newCtx, d)

	cleanup = runMonitor(newCtx, newCtxCancel, adjustedGrace)

	cancel = func() {
		newCtxCancel()
		<-cleanup // ensures that signal handlers are cleaned up
	}

	return
}

func runMonitor(ctx context.Context, cancel func(), gracePeriod time.Duration) <-chan DeadlineEvent {
	cleanupCh := make(chan DeadlineEvent)
	// buffer 1 is essential; otherwise signals will be missed if our goroutine
	// isn't currently blocked on sigCh.
	sigCh := make(chan os.Signal, 1)
	signalNotify(sigCh, signals.Interrupts()...)

	var timeoutC <-chan clock.TimerResult
	if deadline, ok := ctx.Deadline(); ok && gracePeriod > 0 {
		sleepamt := clock.Until(ctx, deadline.Add(-gracePeriod))
		timeoutC = clock.After(ctx, sleepamt)
	}

	go func() {
		defer cancel()
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

		if evt == InterruptEvent && gracePeriod > 0 {
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
	return cleanupCh
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
//   {deadline: ctx.Deadline(), grace_period_secs: DefaultGracePeriod}
//
// If d.deadline == 0, adjusts it to ctx.Deadline().
//
// You probably want to use AdjustDeadline instead.
func SetDeadline(ctx context.Context, d *Deadline) context.Context {
	if d == nil {
		d = &Deadline{GracePeriod: DefaultGracePeriod.Seconds()}
	}
	if deadline, ok := ctx.Deadline(); ok && d.Deadline == 0 {
		d.Deadline = timeToUnixFloat(deadline)
	}
	return Set(ctx, "deadline", d)
}
