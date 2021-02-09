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

// TrackDeadline returns a 'cleanup' channel and a new 'shutdown'-able context
// based on the combination of the input context's deadline and the
// Deadline information in LUCI_CONTEXT.
//
// This function additionally allows you to reserve a portion of the
// grace_period with `reserveGracePeriod`.
//
// First, if Deadline is missing from LUCI_CONTEXT, it is filled in with the
// default:
//
//   {soft_deadline: infinity, grace_period: 30}
//
// We then calculate:
//
//   softDeadline = earlier(
//     ctx.Deadline() - Deadline.grace_period,
//     Deadline.soft_deadline,
//   )
//   adjustedGracePeriod = Deadline.grace_period - reserveGracePeriod
//   adjustedHardDeadline = softDeadline + adjustedGracePeriod
//
// If softDeadline or adjustedGracePeriod differ from their counterparts
// in LUCI_CONTEXT, a new LUCI_CONTEXT['deadline'] is set in `newCtx` with
// the updated value(s).
//
// newCtx's deadline is set to adjustedHardDeadline.
//
// The `cleanup` channel is set up with a goroutine which:
//   * On shutdown()/SIGTERM/os.Interrupt, continuously sends InterruptEvent.
//     After `adjustedGracePeriod` from the interrupt, cancels newCtx.
//   * On `softDeadline`, continuously sends TimeoutEvent.
//     After `adjustedGracePeriod` from the interrupt, cancels newCtx.
//   * On newCtx.Done, closes (so reads will see ClosureEvent).
//
// The returned shutdown() function will begin the shutdown process (acts as
// if an os.Interrupt signal was triggered).
//
// NOTE: If you want to reduce `soft_deadline`, you should do so by applying a
// Deadline/Timeout to the context prior to invoking this function.
//
// Panics if:
//   * reserveGracePeriod < 0.
//   * Deadline.grace_period (or its default 30s value) is insufficient to
//     cover reserveGracePeriod.
//
// Example:
//
//    func MainFunc(ctx context.Context) {
//      // ctx.Deadline  = <unset>
//      // soft_deadline = unix(t0+5:00)
//      // grace_period  = 40
//      cleanup, newCtx, shutdown := lucictx.TrackDeadline(ctx, 500*time.Millisecond)
//      defer shutdown()
//      ScopedFunction(newCtx, cleanup)
//    }
//
//    func ScopedFunction(newCtx context.Context, cleanup <-chan DeadlineEvent) {
//      // newCtx.Deadline  = unix(t0+5:39.5)
//      // soft_deadline = unix(t0+5:00)
//      // grace_period  = 39.5
//
//      go func() {
//        // cleanup is unblocked at SIGTERM, soft_deadline or shutdown(),
//        // whichever is first.
//        <-cleanup
//        // have 39.5s to do something (say, send SIGTERM to a child, or start
//        // tearing down work in-process) before newCtx.Done().
//      }()
//    }
//
// NOTE: In the event that `ctx` is canceled, everything immediately moves to
// the 'Kill' phase without providing any grace period.
func TrackDeadline(ctx context.Context, reserveGracePeriod time.Duration) (cleanup <-chan DeadlineEvent, newCtx context.Context, shutdown func()) {
	if reserveGracePeriod < 0 {
		panic(errors.Reason("reserveGracePeriod(%d) < 0", reserveGracePeriod).Err())
	}

	d := GetDeadline(ctx)
	if d == nil {
		logging.Warningf(
			ctx, "AdjustDeadline without Deadline in LUCI_CONTEXT. "+
				"Assuming Deadline={grace_period: %d}", DefaultGracePeriod.Seconds())
		d = &Deadline{GracePeriod: DefaultGracePeriod.Seconds()}
	}

	needSet := false // set to true if we need to do a write to LUCI_CONTEXT

	// find current soft deadline
	var ctxSoftDeadline time.Time
	if ctxDl, ok := ctx.Deadline(); ok {
		// synthesize the current soft deadline by applying the current grace period
		// to the context's existing hard deadline.
		ctxSoftDeadline = ctxDl.Add(-d.GracePeriodDuration())
	}
	lucictxSoftDeadline := d.SoftDeadlineTime()
	softDeadline := earlier(ctxSoftDeadline, lucictxSoftDeadline)

	// adjust grace period.
	adjustedGrace := d.GracePeriodDuration()
	if reserveGracePeriod > 0 {
		adjustedGrace -= reserveGracePeriod
		if adjustedGrace < 0 {
			panic(errors.Reason(
				"reserveGracePeriod(%f) > gracePeriod(%f)", reserveGracePeriod.Seconds(), d.GracePeriod).Err())
		}
		d.GracePeriod = adjustedGrace.Seconds()
		needSet = true
	}

	var newCtxCancel func()
	var enforceSoftDeadline bool

	// set the new hard deadline, if any, on newCtx
	if softDeadline.IsZero() /* a.k.a. "infinity" */ {
		// need cancel func here so that newCtx can hit hard closure after
		// a signal/shutdown, even though it won't have a deadline.
		newCtx, newCtxCancel = context.WithCancel(ctx)
	} else {
		if !softDeadline.Equal(lucictxSoftDeadline) {
			d.SetSoftDeadline(softDeadline)
			needSet = true
			enforceSoftDeadline = true
		}
		newCtx, newCtxCancel = clock.WithDeadline(ctx, softDeadline.Add(adjustedGrace))
	}

	if needSet {
		newCtx = SetDeadline(newCtx, d)
	}

	cleanup, shutdown = runDeadlineMonitor(newCtx, newCtxCancel, softDeadline, enforceSoftDeadline, adjustedGrace)

	return
}

func runDeadlineMonitor(ctx context.Context, cancel func(), softDeadline time.Time, enforceSoftDeadline bool, gracePeriod time.Duration) (<-chan DeadlineEvent, func()) {
	cleanupCh := make(chan DeadlineEvent)
	// buffer 1 is essential; otherwise signals will be missed if our goroutine
	// isn't currently blocked on sigCh. With a buffer of 1, sigCh is always ready
	// to send.
	sigCh := make(chan os.Signal, 1)
	signalNotify(sigCh, signals.Interrupts()...)

	var timeoutC <-chan clock.TimerResult
	if enforceSoftDeadline {
		timeoutC = clock.After(ctx, clock.Until(ctx, softDeadline))
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
				if !softDeadline.IsZero() && clock.Now(ctx).After(softDeadline) {
					return TimeoutEvent
				}
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
