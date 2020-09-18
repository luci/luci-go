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
	"os/signal"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/signals"
)

// AdjustDeadline does a couple things:
//   * Reads Deadline information from LUCI_CONTEXT
//   * Adjusts Deadline.deadline down by `atLeastCleanup`
//   * Adjusts Deadline.grace_period down by `atLeastCleanup`
//   * Sets the adjusted deadline in the returned context
//   * Sets the adjusted Deadline object in the returned context
//
// Installs a signal handler for SIGTERM/os.Interrupt to:
//   * close `cleanup` on signal
//   * cancel newCtx after the adjusted grace period.
//
// Example:
//
//    func MainFunc(ctx context.Context) {
//      // deadline     = t0+5:00
//      // grace_period = 30s
//      cctx, cleanup, cancel := lucictx.AdjustDeadline(ctx, 5*time.Second)
//      ScopedFunction(cctx, cleanup)
//
//      // Assuming ScopedFunction exits in a timely fashion on ctx.Done, you
//      // have ~5s to do stuff here. Alternately you could wait on `<-cleanup`
//      // and have ~30s to do stuff, but ScopedFunction may not have finished
//      // yet.
//    }
//
//    func ScopedFunction(ctx context.Context, cleanup <-chan struct{}) {
//      // deadline     = t0+4:55
//      // grace_period = 25s
//      go func() {
//        // cleanup is closed at SIGTERM or $deadline-$grace_period, whichever
//        // is first.
//        <-cleanup
//        // have 25 seconds to do something (say, send SIGTERM to a child)
//        // before ctx.Done().
//      }()
//    }
//
// NOTE: In the event that `ctx` is canceled, `cleanup` will not be closed.
func AdjustDeadline(ctx context.Context, atLeastCleanup time.Duration) (newCtx context.Context, cleanup <-chan struct{}, cancel func()) {
	if atLeastCleanup < 0 {
		panic(errors.Reason("atLeastCleanup < 0: %s", atLeastCleanup).Err())
	}

	t := Deadline{}
	_, err := Lookup(ctx, "deadline", &t)
	if err != nil {
		panic(err)
	}

	newCtx = ctx
	cancel = func() {}

	// first, figure out the new grace period
	curGrace := time.Duration(t.GracePeriodSecs) * time.Second
	newGrace := curGrace - atLeastCleanup
	if newGrace < 0 {
		newGrace = 0
	}
	t.GracePeriodSecs = int64(newGrace)

	curDeadline, _ := ctx.Deadline()
	var luciDeadline time.Time
	if t.Deadline != 0 {
		luciDeadline = time.Unix(t.Deadline, 0).UTC()
	}

	var adjustedDeadline time.Time

	switch {
	case curDeadline.IsZero() && !luciDeadline.IsZero():
		adjustedDeadline = luciDeadline
	case !curDeadline.IsZero() && luciDeadline.IsZero():
		adjustedDeadline = curDeadline
	case !curDeadline.IsZero() && !luciDeadline.IsZero():
		if curDeadline.Before(luciDeadline) {
			adjustedDeadline = curDeadline
		} else {
			adjustedDeadline = luciDeadline
		}
	}
	if !adjustedDeadline.IsZero() {
		adjustedDeadline = adjustedDeadline.Add(-atLeastCleanup)
		newCtx, cancel = context.WithDeadline(newCtx, adjustedDeadline)
		t.Deadline = adjustedDeadline.Unix()
	}

	newCtx = Set(newCtx, "deadline", &t)

	cleanup = runMonitor(newCtx, cancel, newGrace)

	return
}

func runMonitor(ctx context.Context, cancel func(), gracePeriod time.Duration) <-chan struct{} {
	cleanupCh := make(chan struct{})
	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, signals.Interrupts()...)
	cleanupTimer := clock.NewTimer(ctx)
	if deadline, ok := ctx.Deadline(); ok {
		cleanupTimer.Reset(clock.Until(ctx, deadline) - gracePeriod)
	}

	go func() {
		defer cancel()

		doSleep := false

		select {
		case <-ctx.Done():
		case <-cleanupTimer.GetC():
		case <-sigCh:
			doSleep = true
		}
		signal.Stop(sigCh)
		close(cleanupCh)
		if !cleanupTimer.Stop() {
			<-cleanupTimer.GetC()
		}

		// If we woke up due to a signal, we'll need to cancel the context after
		// gracePeriod. Otherwise the context's Deadline will do this for us.
		if doSleep {
			cleanupTimer.Reset(gracePeriod)
			select {
			case <-cleanupTimer.GetC():
			case <-ctx.Done():
			}
		}
	}()
	return cleanupCh
}
