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

var softDeadlineKey = "holds <-chan DeadlineEvent"

// TrackSoftDeadline returns a context containing a channel for the
// `SoftDeadlineDone` function in this package.
//
// The "soft" deadline is somewhat like context.WithDeadline, except that it
// participates in the LUCI_CONTEXT['deadline'] protocol as well. On hitting the
// soft deadline (or on an external interrupt signal), the SoftDeadlineDone()
// channel will produce a stream of DeadlineEvents. Once this happens, the
// program has LUCI_CONTEXT['deadline']['grace_period'] seconds until
// ctx.Done(). This is meant to give your program time to do cleanup actions
// with a non-canceled Context.
//
// The soft deadline expires based on the earlier of:
//   - LUCI_CONTEXT['deadline']['soft_deadline']
//   - ctx.Deadline() - LUCI_CONTEXT['deadline']['grace_period']
//
// If LUCI_CONTEXT['deadline'] is missing, it is assumed to be:
//
//	{soft_deadline: infinity, grace_period: 30}
//
// This function additionally allows you to reserve a portion of the
// grace_period with `reserveGracePeriod`. This will have the effect of
// adjusting LUCI_CONTEXT['deadline']['grace_period'] in the returned
// context, as well as canceling the returned context that much earlier.
//
// NOTE: If you want to reduce LUCI_CONTEXT['deadline']['soft_deadline'],
// you should do so by applying a Deadline/Timeout to the context prior
// to invoking this function.
//
// Panics if:
//   - reserveGracePeriod < 0.
//   - LUCI_CONTEXT['deadline']['grace_period'] (or its default 30s value)
//     is insufficient to cover reserveGracePeriod.
//
// Example:
//
//	func MainFunc(ctx context.Context) {
//	  // ctx.Deadline  = <unset>
//	  // soft_deadline = t0 + 5:00
//	  // grace_period  = 40
//
//	  newCtx, shutdown := lucictx.TrackSoftDeadline(ctx, 500*time.Millisecond)
//	  defer shutdown()
//	  ScopedFunction(newCtx)
//	}
//
//	func ScopedFunction(newCtx context.Context) {
//	  // hard deadline is (soft_deadline + grace_period - reserveGracePeriod)
//	  // newCtx.Deadline  = unix(t0+5:39.5)
//	  //
//	  // soft_deadline is unchanged
//	  // soft_deadline = t0 + 5:00
//	  //
//	  // grace_period is reduced by reserveGracePeriod
//	  // grace_period = 39.5
//
//	  go func() {
//	    // unblocked at SIGTERM, soft_deadline or shutdown(), whichever is first.
//	    <-lucictx.SoftDeadlineDone()
//	    // have 39.5s to do something (say, send SIGTERM to a child, or start
//	    // tearing down work in-process) before newCtx.Done().
//	  }()
//	}
//
// NOTE: In the event that `ctx` is canceled from outside, `newCtx` will also
// immediately cancel, and SoftDeadlineDone will also move to the ClosureEvent
// state.
func TrackSoftDeadline(ctx context.Context, reserveGracePeriod time.Duration) (newCtx context.Context, shutdown func()) {
	if reserveGracePeriod < 0 {
		panic(errors.Fmt("reserveGracePeriod(%d) < 0", reserveGracePeriod))
	}

	d := GetDeadline(ctx)
	if d == nil {
		logging.Warningf(
			ctx, "AdjustDeadline without Deadline in LUCI_CONTEXT. "+
				"Assuming Deadline={grace_period: %.2f}", DefaultGracePeriod.Seconds())
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
			panic(errors.Fmt("reserveGracePeriod(%s) > gracePeriod(%s)", reserveGracePeriod, d.GracePeriodDuration()))
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

	cleanup, shutdown := runDeadlineMonitor(newCtx, newCtxCancel, softDeadline, enforceSoftDeadline, adjustedGrace)
	newCtx = context.WithValue(newCtx, &softDeadlineKey, cleanup)

	return
}

// SoftDeadlineDone is the counterpart of TrackSoftDeadline, and returns
// a channel which unblocks when the soft deadline of ctx is met, or `ctx` has
// been shut down/interrupted.
//
// If ctx does not come from TrackSoftDeadline(), this returns a nil
// channel (i.e. blocks forever)
func SoftDeadlineDone(ctx context.Context) (ret <-chan DeadlineEvent) {
	ret, _ = ctx.Value(&softDeadlineKey).(<-chan DeadlineEvent)
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
			case <-ctx.Done():
				return ClosureEvent

			case <-timeoutC:
				// clock timer channels unblock on ctx.Done; It's a race whether the
				// previous select case or this one will activate, so check to see if
				// the context has Err set, and return accordingly.
				if ctx.Err() != nil {
					return ClosureEvent
				}
				return TimeoutEvent

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
//
//	{grace_period: DefaultGracePeriod}
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
