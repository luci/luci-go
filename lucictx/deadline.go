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

func (de DeadlineEvent) String() string {
	switch de {
	case ClosureEvent:
		return "ClosureEvent"
	case InterruptEvent:
		return "InterruptEvent"
	case TimeoutEvent:
		return "TimeoutEvent"
	default:
		panic(fmt.Sprintf("impossible DeadlineEvent %d", de))
	}
}

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
//   * On shutdown()/SIGTERM/os.Interrupt, continuously sends InterruptEvent.
//     After `adjustedGracePeriod` from the interrupt, newCtx will be canceled.
//   * On `adjustedSoftDeadline`, continuously sends TimeoutEvent.
//     The newCtx will meet its hard deadline after `adjustedGracePeriod`.
//   * On newCtx.Done, closes (so reads will see ClosureEvent).
//
// Finally, the newCtx has its deadline set to
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
//      cleanup, newCtx, shutdown := lucictx.AdjustDeadline(ctx, time.Minute, 500*time.Millisecond)
//      defer shutdown()
//      ScopedFunction(newCtx, cleanup)
//
//      // When ScopedFunction returns (assuming it returns immediately on
//      // newCtx.Done):
//      //   If there was a signal, you have 500ms left.
//      //   If newCtx timed out, you have 1m40s.
//      //
//      // If you had a goroutine waiting on <-cleanup, you would get the full
//      // grace_period to do something. If <-cleanup returned TimeoutEvent, you
//      // would also get the remainder of soft_deadline (i.e. 1 additional
//      // minute)
//    }
//
//    func ScopedFunction(newCtx context.Context, cleanup <-chan DeadlineEvent) {
//      // newCtx.Deadline  = unix(t0+4:39.5)
//      // soft_deadline = unix(t0+4:00)
//      // grace_period  = 39.5
//
//      go func() {
//        // cleanup is unblocked at SIGTERM, $soft_deadline or shutdown(),
//        // whichever is first.
//        <-cleanup
//        // have grace_period to do something (say, send SIGTERM to a child)
//        // before newCtx.Done().
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

	needSet := false

	// Adjust grace period.
	origGracePeriod := d.GracePeriodDuration()
	adjustedGrace := origGracePeriod - reserveCleanup
	if adjustedGrace < 0 {
		panic(errors.Reason(
			"reserveCleanup(%f) > gracePeriod(%f)", reserveCleanup.Seconds(), d.GracePeriod).Err())
	}
	if reserveCleanup > 0 {
		d.GracePeriod = adjustedGrace.Seconds()
		needSet = true
	}

	// find adjustedSoftDeadline
	var ctxSoftDeadline time.Time
	if d, ok := ctx.Deadline(); ok {
		ctxSoftDeadline = d.Add(-origGracePeriod)
	}
	var lucictxSoftDeadline time.Time
	if d.SoftDeadline != 0 {
		lucictxSoftDeadline = d.SoftDeadlineTime()
	}
	adjustedSoftDeadline := earlier(ctxSoftDeadline, lucictxSoftDeadline)

	var newCtxCancel func()

	// Set up hard deadline in context, set Deadline.soft_deadline
	if !adjustedSoftDeadline.IsZero() {
		adjustedSoftDeadline = adjustedSoftDeadline.Add(-reserve)
		d.SetSoftDeadline(adjustedSoftDeadline)
		needSet = true
		// we add adjustedGrace back because the ctx deadline is the HARD deadline;
		// it must occur `adjustedGrace` after the SoftDeadline.
		//
		// note that if adjustedSoftDeadline+adjustedGrace is in the past, this
		// returns newCtx as already Done.
		newCtx, newCtxCancel = clock.WithDeadline(ctx, adjustedSoftDeadline.Add(adjustedGrace))
	} else {
		// need cancel func here so that newCtx can hit hard closure after
		// a signal/shutdown, even though it won't have a deadline.
		newCtx, newCtxCancel = context.WithCancel(ctx)
	}

	if needSet {
		newCtx = SetDeadline(newCtx, d)
	}

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
		defer cancel()

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

		// Note we do this before signaling cleanupCh so that tests can force
		// `clock.After` to run before incrementing the test clock.
		if evt == InterruptEvent {
			timeoutC = clock.After(ctx, gracePeriod)
		} else {
			timeoutC = nil
		}

		if evt == ClosureEvent {
			close(cleanupCh)
		} else {
			go func() {
				for {
					cleanupCh <- evt
				}
			}()
		}

		select {
		case <-timeoutC:
		case <-ctx.Done():
		}
		// note `defer cancel()` at the top; at this point ctx has either timed out
		// from its internal deadline, or we're about to cancel it.
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
		d.SetSoftDeadline(deadline)
		d.SoftDeadline -= d.GracePeriod
	}
	return Set(ctx, "deadline", d)
}

// CheckDeadlines returns information about whether or not the soft/hard
// deadlines of `ctx` have been hit.
//
// This derives the soft deadline by:
//   * GetDeadline(ctx).SoftDeadline : if ctx/LUCI_CONTEXT have explicitly set deadline
//   * ctx.Deadline() - GracePeriod  : if ctx has a Go deadline set
//
// This derives the hard deadline by adding GracePeriod to the soft deadline.
//
// The booleans returned for a given context are sticky; Once past the soft
// deadline, `hitSoftDeadline` will always be true, and simiilarly for
// `hitHardDeadline`.
//
// See AdjustDeadline for details around deadlines and grace periods.
func CheckDeadlines(ctx context.Context) (hitSoftDeadline, hitHardDeadline bool) {
	dl := GetDeadline(ctx)
	if dl == nil {
		dl = GetDeadline(SetDeadline(ctx, nil))
	}
	if dl.SoftDeadline == 0 {
		return false, false
	}

	now := clock.Now(ctx)
	hitSoftDeadline = now.After(dl.SoftDeadlineTime())
	hitHardDeadline = now.After(dl.SoftDeadlineTime().Add(dl.GracePeriodDuration()))
	return
}

// SoftDeadlineTime returns the SoftDeadline as a time.Time.
//
// If SoftDeadline is 0 (or *Deadline is nil) this returns a Zero Time.
func (d *Deadline) SoftDeadlineTime() time.Time {
	if d.GetSoftDeadline() == 0 {
		return time.Time{}
	}

	int, frac := math.Modf(d.SoftDeadline)
	return time.Unix(int64(int), int64(frac*1e9)).UTC()
}

// SetSoftDeadline sets the SoftDeadline from a time.Time.
//
// If t.IsZero, this sets the SoftDeadline to 0 as well.
func (d *Deadline) SetSoftDeadline(t time.Time) {
	if t.IsZero() {
		d.SoftDeadline = 0
	} else {
		d.SoftDeadline = float64(t.Unix()) + (float64(t.Nanosecond()) / 1e9)
	}
}

// GracePeriodDuration returns the GracePeriod as a time.Duration.
//
// If d == nil, returns DefaultGracePeriod.
func (d *Deadline) GracePeriodDuration() time.Duration {
	if d == nil {
		return DefaultGracePeriod
	}
	return time.Duration(d.GracePeriod * float64(time.Second))
}
