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

	bbfacade "go.chromium.org/luci/cv/internal/buildbucket/facade"
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
			Title: fmt.Sprintf("%s/%s/%T", r.ID, opID, r.OngoingLongOps.GetOps()[opID].GetWork()),
			Payload: &eventpb.ManageRunLongOpTask{
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

func (rm *RunManager) doLongOperation(ctx context.Context, task *eventpb.ManageRunLongOpTask) error {
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
		return errors.Fmt("Run %q not found: %w", r.ID, err)
	case err != nil:
		// Will retry.
		return transient.Tag.Apply(errors.Fmt("failed to load Run %q: %w", r.ID, err))
	case r.OngoingLongOps.GetOps()[task.GetOperationId()] == nil:
		// Highly unexpected. Fail hard.
		return errors.Fmt("Run %q has no outstanding long operation %q", r.ID, task.GetOperationId())
	}
	op := r.OngoingLongOps.GetOps()[task.GetOperationId()]

	now := clock.Now(ctx)
	d := op.GetDeadline().AsTime()
	if d.Before(now) {
		result := &eventpb.LongOpCompleted{Status: eventpb.LongOpCompleted_EXPIRED}
		if err := notifyCompleted(result); err != nil {
			logging.Errorf(ctx, "Failed to NotifyLongOpCompleted: %s", err)
		}
		return errors.Fmt("DoLongRunOperationTask arrived too late (deadline: %s, now %s)", d, now)
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
	case transient.Tag.In(err):
		// Just retry.
		return err
	case result != nil:
		// honor the result status returned by the long op
	case errors.Unwrap(err) == dctx.Err():
		logging.Warningf(ctx, "Failed long op due to hitting a deadline, setting result as EXPIRED")
		result = &eventpb.LongOpCompleted{Status: eventpb.LongOpCompleted_EXPIRED}
	default:
		// permanent failure
		result = &eventpb.LongOpCompleted{Status: eventpb.LongOpCompleted_FAILED}
	}

	switch nerr := notifyCompleted(result); {
	case nerr == nil:
		return err
	case err == nil:
		// the op succeeded, but notify failed.
		logging.Errorf(ctx, "Long op succeeded but it failed to notify Run Manager: %s", nerr)
		return nerr
	default:
		// both op and notify failed.
		logging.Errorf(ctx, "Long op %T permanently failed with %s, but also failed to notify Run Manager due to %s", op.GetWork(), err, nerr)
		return err
	}
}

func (rm *RunManager) doLongOperationWithDeadline(ctx context.Context, opBase *longops.Base) (*eventpb.LongOpCompleted, error) {
	var op longops.Operation
	switch w := opBase.Op.GetWork().(type) {
	case *run.OngoingLongOps_Op_PostStartMessage:
		op = &longops.PostStartMessageOp{
			Base:     opBase,
			Env:      rm.env,
			GFactory: rm.gFactory,
		}
	case *run.OngoingLongOps_Op_PostGerritMessage_:
		op = &longops.PostGerritMessageOp{
			Base:     opBase,
			Env:      rm.env,
			GFactory: rm.gFactory,
		}
	case *run.OngoingLongOps_Op_ResetTriggers_:
		op = &longops.ResetTriggersOp{
			Base:        opBase,
			GFactory:    rm.gFactory,
			CLMutator:   rm.clMutator,
			Concurrency: 8,
		}
	case *run.OngoingLongOps_Op_ExecuteTryjobs:
		op = &longops.ExecuteTryjobsOp{
			Base:        opBase,
			Env:         rm.env,
			RunNotifier: rm.runNotifier,
			Backend: &bbfacade.Facade{
				ClientFactory: rm.bbFactory,
			},
		}
	case *run.OngoingLongOps_Op_ExecutePostAction:
		op = &longops.ExecutePostActionOp{
			Base:        opBase,
			GFactory:    rm.gFactory,
			RunNotifier: rm.runNotifier,
			QM:          rm.qm,
		}
	default:
		logging.Errorf(ctx, "unknown LongOp work %T", w)
		// Fail task quickly for backwards compatibility in case of a rollback during
		// future deployment.
		return nil, tq.Fatal.Apply(errors.Fmt("Skipping %T", opBase.Op.GetWork()))
	}
	return op.Do(ctx)
}

// longOpCancellationChecker asynchronously checks whether the given operation
// was requested to be cancelled by the Run Manager.
type longOpCancellationChecker struct {
	// Options.

	// interval controls how frequently the Datastore is checked. Defaults to 5s.
	interval time.Duration
	// testChan is used in tests to detect when background goroutine exists.
	testChan chan struct{}

	// Internal state.

	// state is atomically updated int which stores state of the cancellation
	//
	// state is atomically updated int which stores state of the cancelation
	// checker:
	//  * cancellationCheckerInitial
	//  * cancellationCheckerStarted
	//  * cancellationCheckerRequested
	state int32
}

const (
	cancellationCheckerInitial   = 0
	cancellationCheckerStarted   = 1
	cancellationCheckerRequested = 2
)

// check quickly and cheaply checks whether cancellation was requested.
//
// Does not block on anything.
func (l *longOpCancellationChecker) check() bool {
	return atomic.LoadInt32(&l.state) == cancellationCheckerRequested
}

// start spawns a goroutine checking if cancellation was requested.
//
// Returns a stop function, which should be called to free resources.
func (l *longOpCancellationChecker) start(ctx context.Context, initial *run.Run, opID string) (stop func()) {
	if !atomic.CompareAndSwapInt32(&l.state, cancellationCheckerInitial, cancellationCheckerStarted) {
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
			if res := <-clock.After(ctx, left); res.Err != nil {
				// Context got cancelled.
				break
			}
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
		atomic.StoreInt32(&l.state, cancellationCheckerRequested)
		return false
	case err != nil && ctx.Err() != nil:
		// Context was cancelled or expired.
		return false
	case err != nil:
		logging.Warningf(ctx, "Failed to reload Run, will retry: %s", err)
		return true
	case r.OngoingLongOps == nil || r.OngoingLongOps.GetOps()[opID] == nil:
		logging.Warningf(ctx, "Reloaded Run no longer has this operation")
		atomic.StoreInt32(&l.state, cancellationCheckerRequested)
		return false
	case r.OngoingLongOps.GetOps()[opID].GetCancelRequested():
		logging.Warningf(ctx, "Cancellation request detected")
		atomic.StoreInt32(&l.state, cancellationCheckerRequested)
		return false
	default:
		return true
	}
}
