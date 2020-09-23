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

// DefaultGracePeriodSecs is the value of Deadline.grace_period_secs to assume
// if Deadline is entirely missing in LUCI_CONTEXT.
const DefaultGracePeriodSecs = 30

func secsToDuration(secs float64) time.Duration {
	int, frac := math.Modf(secs)
	return (time.Duration(int) * time.Second) + (time.Duration(frac*1e9) * time.Nanosecond)
}

func unixFloatToTime(t float64) time.Time {
	int, frac := math.Modf(t)
	return time.Unix(int64(int), int64(frac*1e9)).UTC()
}

func timeToUnixFloat(t time.Time) float64 {
	return float64(t.Unix()) + (float64(t.Nanosecond()) / 1e9)
}

// AdjustDeadline returns a 'cleanup' channel and a new cancelable context based
// on the combination of the input context's deadline, as well as the Deadline
// information in LUCI_CONTEXT.
//
// First, if Deadline is missing from LUCI_CONTEXT, it is filled in with the
// default:
//
//   {deadline: infinity, grace_period_secs: 30}
//
// Next, the earlier of the input ctx.Deadline and Deadline.deadline values is
// computed. `reserveCleanup` is subtracted from this and it becomes the new
// `adjustedDeadline`.
//
// Next, `reserveCleanup` is subtracted from Deadline.grace_period_secs.
// This becomes the `adjustedGracePeriod`.
//
// The `cleanup` channel is set up with a goroutine which:
//   * Closes `cleanup` on either `adjustedDeadline-adjustedGracePeriod` timeout
//     or SIGTERM/os.Interrupt, whichever is earlier.
//   * If a signal was caught, Cancels the returned context
//     `adjustedGracePeriod` after this (otherwise allows it to reach
//     DeadlineExceeded).
//
// Finally, the returned context has its deadline set to `adjustedDeadline`, and
// LUCI_CONTEXT['deadline'] is populated with the adjusted deadline/gracePeriod.
//
// Note, if Deadline.grace_period_secs is insufficient to cover reserveCleanup
// (including if reserveCleanup>DefaultGracePeriodSecs and no Deadline was in
// LUCI_CONTEXT at all), this function panics.
//
// Example:
//
//    func MainFunc(ctx context.Context) {
//      // deadline          = unix(t0+5:00)
//      // grace_period_secs = 40
//      cleanup, cctx, cancel := lucictx.AdjustDeadline(ctx, 500*time.Millisecond)
//      defer cancel()
//      ScopedFunction(cctx, cleanup)
//
//      // Assuming ScopedFunction exits in a timely fashion on ctx.Done, you
//      // have ~5s to do stuff here. Alternately you could wait on `<-cleanup`
//      // and have ~30s to do stuff, but ScopedFunction may not have finished
//      // yet.
//    }
//
//    func ScopedFunction(ctx context.Context, cleanup <-chan struct{}) {
//      // deadline          = unix(t0+4:55)
//      // grace_period_secs = 35
//      // Otherwise Deadline is still not in LUCI_CONTEXT.
//
//      go func() {
//        // cleanup is closed at SIGTERM or $deadline-$grace_period_secs,
//        // whichever is first.
//        <-cleanup
//        // have grace_period_secs to do something (say, send SIGTERM to
//        // a child) before ctx.Done().
//      }()
//    }
//
// NOTE: In the event that `ctx` is canceled, `cleanup` will not be closed.
func AdjustDeadline(ctx context.Context, reserveCleanup time.Duration) (cleanup <-chan struct{}, newCtx context.Context, cancel func()) {
	if reserveCleanup < 0 {
		panic(errors.Reason("reserveCleanup(%d) < 0", reserveCleanup).Err())
	}

	d := GetDeadline(ctx)
	if d == nil {
		logging.Warningf(
			ctx, "AdjustDeadline without Deadline in LUCI_CONTEXT. "+
				"Assuming Deadline={grace_period_secs: %d}", DefaultGracePeriodSecs)
		d = &Deadline{GracePeriod: DefaultGracePeriodSecs}
	}

	// Adjust grace period.
	adjustedGrace := d.GracePeriod - reserveCleanup.Seconds()
	if adjustedGrace < 0 {
		panic(errors.Reason(
			"reserveCleanup(%f) > gracePeriod(%f)", reserveCleanup.Seconds(), d.GracePeriod).Err())
	}
	d.GracePeriod = adjustedGrace

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
		adjustedDeadline = adjustedDeadline.Add(-reserveCleanup)
		newCtx, newCtxCancel = clock.WithDeadline(ctx, adjustedDeadline)
		d.Deadline = timeToUnixFloat(adjustedDeadline)
	} else {
		newCtx, newCtxCancel = context.WithCancel(ctx)
	}

	newCtx = SetDeadline(newCtx, d)

	cleanup = runMonitor(newCtx, newCtxCancel, secsToDuration(adjustedGrace))

	cancel = func() {
		newCtxCancel()
		<-cleanup // ensures that signal handlers are cleaned up
	}

	return
}

func runMonitor(ctx context.Context, cancel func(), gracePeriod time.Duration) <-chan struct{} {
	cleanupCh := make(chan struct{})
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

		doSleep := func() bool {
			defer close(cleanupCh)
			defer signalStop(sigCh)

			select {
			case <-timeoutC:
			case <-ctx.Done():
			case <-sigCh:
				return true
			}
			return false
		}()

		if doSleep && gracePeriod > 0 {
			timeoutC = clock.After(ctx, gracePeriod)
		} else {
			timeoutC = nil
		}
		select {
		case <-timeoutC:
		case <-ctx.Done():
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
//   {deadline: ctx.Deadline(), grace_period_secs: DefaultGracePeriodSecs}
//
// If d.deadline == 0, adjusts it to ctx.Deadline().
//
// You probably want to use AdjustDeadline instead.
func SetDeadline(ctx context.Context, d *Deadline) context.Context {
	if d == nil {
		d = &Deadline{GracePeriod: DefaultGracePeriodSecs}
	}
	if deadline, ok := ctx.Deadline(); ok && d.Deadline == 0 {
		d.Deadline = timeToUnixFloat(deadline)
	}
	return Set(ctx, "deadline", d)
}
