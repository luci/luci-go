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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
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
		switch {
		case res == nil:
			res = &eventpb.LongOpCompleted{OperationId: task.GetOperationId()}
		default:
			res.OperationId = task.GetOperationId()
		}
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
		if err := notifyCompleted(nil); err != nil {
			logging.Errorf(ctx, "Failed to NotifyLongOpCompleted: %s", err)
		}
		return errors.Reason("DoLongRunOperationTask arrived too late (deadline: %s, now %s)", d, now).Err()
	}

	dctx, cancel := clock.WithDeadline(ctx, d)
	defer cancel()

	f := rm.doLongOperationWithDeadline
	if rm.testDoLongOperationWithDeadline != nil {
		f = rm.testDoLongOperationWithDeadline
	}
	result, err := f(dctx, r, op)

	switch {
	case err == nil:
		return notifyCompleted(result)
	case transient.Tag.In(err):
		// Just retry.
		return err
	default:
		// On permanent failure, don't fail task until the Run Manager is notified.
		if nerr := notifyCompleted(result); nerr != nil {
			logging.Errorf(ctx, "Long op %T permanently failed with %s, but also failed to notify Run Manager", op.GetWork(), err)
			return nerr
		}
		return err
	}
}

func (rm *RunManager) doLongOperationWithDeadline(ctx context.Context, r *run.Run, op *run.OngoingLongOps_Op) (*eventpb.LongOpCompleted, error) {
	switch w := op.GetWork().(type) {
	case *run.OngoingLongOps_Op_PostStartMessage:
		// TODO(tandrii): implement.
		logging.Errorf(ctx, "TODO(tandrii): implement LongOp.PostStartMessage(%t)", w.PostStartMessage)
	default:
		logging.Errorf(ctx, "unknown LongOp work %T", w)
	}
	// Fail task quickly for backwards compatibility in case of a rollback during
	// future deployment.
	return nil, errors.Reason("Skipping %T", op.GetWork()).Tag(tq.Fatal).Err()
}
