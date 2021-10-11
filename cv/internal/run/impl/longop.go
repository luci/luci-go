// Copyright 2021 The LUCI Authors.
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

package impl

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/longops"
)

// enqueueLongOp enqueues long operations task and updates the given Run's long
// operations state.
func (rp *runProcessor) enqueueLongOps(ctx context.Context, r *run.Run, opIDs ...string) error {
	for _, opID := range opIDs {
		err := rp.tqDispatcher.AddTask(ctx, &tq.Task{
			Title: fmt.Sprintf("%s/long-op/%s/%T", r.ID, opID, r.OngoingLongOps.GetOps()[opID].GetWork()),
			Payload: &eventpb.DoLongOpTask{
				RunId:       string(r.ID),
				OperationId: opID,
			},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (rm *RunManager) doLongOperation(ctx context.Context, task *eventpb.DoLongOpTask) error {
	notifyCompleted := func(res *eventpb.LongOpCompleted) error {
		if res.Status == eventpb.LongOpCompleted_LONG_OP_STATUS_UNSPECIFIED {
			panic(fmt.Errorf("LongOpCompleted.Status must be set"))
		}
		res.OperationId = task.GetOperationId()
		return rm.runNotifier.NotifyLongOpCompleted(ctx, common.RunID(task.GetRunId()), res)
	}

	r := &run.Run{ID: common.RunID(task.GetRunId())}
	switch err := datastore.Get(ctx, r); {
	case err == datastore.ErrNoSuchEntity:
		// Highly unexpected. Fail hard.
		return errors.Annotate(err, "Run %q not found", r.ID).Err()
	case err != nil:
		// Will retry.
		return errors.Annotate(err, "failed to load Run %q", r.ID).Tag(transient.Tag).Err()
	case r.OngoingLongOps.GetOps()[task.GetOperationId()] == nil:
		// Highly unexpected. Fail hard.
		return errors.Annotate(err, "Run %q has no outstanding long operation %q", r.ID, task.GetOperationId()).Err()
	}
	op := r.OngoingLongOps.GetOps()[task.GetOperationId()]

	now := clock.Now(ctx)
	d := op.GetDeadline().AsTime()
	if d.Before(now) {
		result := &eventpb.LongOpCompleted{Status: eventpb.LongOpCompleted_EXPIRED}
		if err := notifyCompleted(result); err != nil {
			logging.Errorf(ctx, "Failed to NotifyLongOpCompleted: %s", err)
		}
		return errors.Reason("DoLongRunOperationTask arrived too late (deadline: %s, now %s)", d, now).Err()
	}

	checker := &longOpCancellationChecker{}
	stop := checker.start(ctx, r, task.GetOperationId())
	defer stop()

	opBase := longops.Base{
		Run:               r,
		Op:                op,
		IsCancelRequested: checker.check,
	}

	dctx, cancel := clock.WithDeadline(ctx, d)
	defer cancel()
	f := rm.doLongOperationWithDeadline
	if rm.testDoLongOperationWithDeadline != nil {
		f = rm.testDoLongOperationWithDeadline
	}
	result, err := f(dctx, &opBase)

	switch {
	case err == nil:
		return notifyCompleted(result)
	case transient.Tag.In(err):
		// Just retry.
		return err
	case errors.Unwrap(err) == dctx.Err() && result == nil:
		// Failed due to hitting a deadline, so set an appropriate result by default.
		result = &eventpb.LongOpCompleted{Status: eventpb.LongOpCompleted_EXPIRED}
		fallthrough
	default:
		// On permanent failure, don't fail the task until the Run Manager is
		// notified.
		if result == nil {
			result = &eventpb.LongOpCompleted{Status: eventpb.LongOpCompleted_FAILED}
		}
		if nerr := notifyCompleted(result); nerr != nil {
			logging.Errorf(ctx, "Long op %T permanently failed with %s, but also failed to notify Run Manager", op.GetWork(), err)
			return nerr
		}
		return err
	}
}

func (rm *RunManager) doLongOperationWithDeadline(ctx context.Context, opBase *longops.Base) (*eventpb.LongOpCompleted, error) {
	var action interface {
		Do(context.Context) (*eventpb.LongOpCompleted, error)
	}

	switch w := opBase.Op.GetWork().(type) {
	case *run.OngoingLongOps_Op_PostStartMessage:
		action = &longops.PostStartMessageOp{
			Base:     opBase,
			GFactory: rm.gFactory,
		}
	default:
		logging.Errorf(ctx, "unknown LongOp work %T", w)
		// Fail task quickly for backwards compatibility in case of a rollback during
		// future deployment.
		return nil, errors.Reason("Skipping %T", opBase.Op.GetWork()).Tag(tq.Fatal).Err()
	}
	return action.Do(ctx)
}

// longOpCancellationChecker asynchroneously checks whether the given operation
// was requested to be cancelled by the Run Manager.
type longOpCancellationChecker struct {
	// Options.

	// interval controls how frequently the Datastore is checked. Defaults to 5s.
	interval time.Duration
	// testChan is used in tests to detect when background goroutine exists.
	testChan chan struct{}

	// Internal state.

	// state is atomically updated int which stores state of the cancelation
	// checker:
	//  * cancelationCheckerInitial
	//  * cancelationCheckerStarted
	//  * cancelationCheckerRequested
	state int32
}

const (
	cancelationCheckerInitial   = 0
	cancelationCheckerStarted   = 1
	cancelationCheckerRequested = 2
)

// check quickly and cheaply checks whether cancellation was requested.
//
// Does not block on anything.
func (l *longOpCancellationChecker) check() bool {
	return atomic.LoadInt32(&l.state) == cancelationCheckerRequested
}

// start spawns a goroutine checking if cancelation was requested.
//
// Returns a stop function, which should be called to free resources.
func (l *longOpCancellationChecker) start(ctx context.Context, initial *run.Run, opID string) (stop func()) {
	if !atomic.CompareAndSwapInt32(&l.state, cancelationCheckerInitial, cancelationCheckerStarted) {
		panic(fmt.Errorf("start called more than once"))
	}
	switch {
	case l.interval < 0:
		panic(fmt.Errorf("negative interval %s", l.interval))
	case l.interval == 0:
		l.interval = 5 * time.Second
	case l.interval < time.Second:
		// If lower frequency is desired in the future, use Redis directly (instead
		// of indirectly via dscache).
		panic(fmt.Errorf("too small interval %s -- don't hammer Datastore", l.interval))
	}

	ctx, stop = context.WithCancel(ctx)

	// Perform check on initial Run state immediately.
	// This is useful if TQ task performing long op is retried s.t. the request
	// for cancellation is detected immediately as opposed to during the next
	// reload of the Run.
	if !l.reevaluate(ctx, opID, initial, nil) {
		return stop
	}

	go func() {
		if l.testChan != nil {
			defer close(l.testChan)
		}
		l.background(ctx, opID, initial.ID)
	}()

	return stop
}

func (l *longOpCancellationChecker) background(ctx context.Context, opID string, runID common.RunID) {
	next := clock.Now(ctx)
	for {
		next = next.Add(l.interval)
		left := next.Sub(clock.Now(ctx))
		if left > 0 {
			<-clock.After(ctx, left)
		}
		r := run.Run{ID: runID}
		err := datastore.Get(ctx, &r)
		if !l.reevaluate(ctx, opID, &r, err) {
			break
		}
	}
}

// reevaluate updates state if necessary and returns whether the background
// checking should continue.
func (l *longOpCancellationChecker) reevaluate(ctx context.Context, opID string, r *run.Run, err error) bool {
	switch {
	case err == datastore.ErrNoSuchEntity:
		logging.Errorf(ctx, "Run was unexpectedly deleted")
		atomic.StoreInt32(&l.state, cancelationCheckerRequested)
		return false
	case err != nil && ctx.Err() != nil:
		// Context was cancelled or expired.
		return false
	case err != nil:
		logging.Warningf(ctx, "Failed to reload Run, will retry: %s", err)
		return true
	case r.OngoingLongOps == nil || r.OngoingLongOps.GetOps()[opID] == nil:
		logging.Warningf(ctx, "Reloaded Run no longer has this operation")
		atomic.StoreInt32(&l.state, cancelationCheckerRequested)
		return false
	case r.OngoingLongOps.GetOps()[opID].GetCancelRequested():
		logging.Warningf(ctx, "Cancelation request detected")
		atomic.StoreInt32(&l.state, cancelationCheckerRequested)
		return false
	default:
		return true
	}
}
