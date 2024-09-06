// Copyright 2024 The LUCI Authors.
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

package tasks

import (
	"context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/notifications"
	"go.chromium.org/luci/swarming/server/rbe"
)

// Cancellation contains information to cancel a task.
type Cancellation struct {
	TaskID string

	// TaskRequest and TaskResultSummary are required in RunInTxn.
	TaskRequest       *model.TaskRequest
	TaskResultSummary *model.TaskResultSummary

	// Whether to kill the task if it has started running.
	KillRunning bool
	// ID of the bot the task should run on. Can only be specified if
	// KillRunning is true.
	BotID string

	// test functions for tq tasks. Should only be used in tests.
	testEnqueueRBECancel func(context.Context, *model.TaskRequest, *model.TaskToRun) error
	testSendOnTaskUpdate func(context.Context, *model.TaskRequest, *model.TaskResultSummary) error
}

func (c *Cancellation) validate() error {
	if c.TaskID == "" {
		return errors.New("no task id specified for cancellation")
	}

	if c.BotID != "" && !c.KillRunning {
		return errors.New("can only specify bot id in cancellation if can kill a running task")
	}

	return nil
}

// Run cancels a task if possible.
//
// Ensures that the associated TaskToRun is canceled (when PENDING) and
// updates the TaskResultSummary/TaskRunResult accordingly.
//
// For PENDING tasks, the TaskResultSummary's state is immediately changed.
// For RUNNING tasks, the TaskRunResult.Killing bit is immediately set, but
// their state (in TaskRunResult and TaskResultSummary) is not changed yet.
//
// Warning: ACL check must have been done before.
//
// Returns a bool for whether the task was running when being canceled, and
// an err for errors to cancel the task.
func (c *Cancellation) Run(ctx context.Context) (bool, error) {
	if err := c.validate(); err != nil {
		return false, err
	}
	trKey, err := model.TaskIDToRequestKey(ctx, c.TaskID)
	if err != nil {
		return false, err
	}

	stateChanged := false
	var wasRunning bool
	c.TaskRequest = &model.TaskRequest{Key: trKey}
	c.TaskResultSummary = &model.TaskResultSummary{Key: model.TaskResultSummaryKey(ctx, trKey)}
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		if err := datastore.Get(ctx, c.TaskRequest, c.TaskResultSummary); err != nil {
			return errors.Annotate(err, "datastore error fetching entities for task %s", c.TaskID).Err()
		}

		origState := c.TaskResultSummary.State
		wasRunning = origState == apipb.TaskState_RUNNING
		if err = c.RunInTxn(ctx); err != nil {
			return err
		}

		stateChanged = c.TaskResultSummary.State != origState
		return nil
	}, nil)

	if err != nil {
		return false, err
	}

	if stateChanged {
		// TODO(b/355013314): report to task_status_change_scheduler_latency metric if state changed.
	}

	return wasRunning, nil
}

// RunInTxn updates entities of the task to cancel and enqueues the cloud tasks
// to cancel its children and send notifications.
//
// Mutates c.TaskResultSummary in place.
//
// Must run in a transaction.
func (c *Cancellation) RunInTxn(ctx context.Context) error {
	if datastore.CurrentTransaction(ctx) == nil {
		panic("cancel.RunInTxn must run in a transaction")
	}

	if err := c.validate(); err != nil {
		return err
	}
	if c.TaskRequest == nil || c.TaskResultSummary == nil {
		return errors.Reason("missing entities when cancelling %s", c.TaskID).Err()
	}

	tr := c.TaskRequest
	trs := c.TaskResultSummary
	wasRunning := trs.State == apipb.TaskState_RUNNING
	switch {
	case !trs.IsActive():
		// Finished tasks can't be canceled.
		return nil
	case wasRunning && !c.KillRunning:
		return nil
	case wasRunning && c.BotID != "" && c.BotID != trs.BotID.Get():
		logging.Debugf(ctx, "request to cancel task %s on bot %s, got bot %s instead", c.TaskID, c.BotID, trs.BotID.Get())
		return nil
	}

	now := clock.Now(ctx).UTC()
	trs.Abandoned = datastore.NewIndexedNullable(now)
	trs.Modified = now
	toPut := []any{trs}

	cancelPending := func() error {
		// PENDING task can be canceled right away.
		trs.State = apipb.TaskState_CANCELED
		trs.Completed = datastore.NewIndexedNullable(now)

		// Update TaskToRun.
		toRunKey, err := model.TaskRequestToToRunKey(ctx, tr, int(trs.CurrentTaskSlice))
		if err != nil {
			return errors.Annotate(err, "failed to get the TaskToRun key for task %s", c.TaskID).Err()
		}
		toRun := &model.TaskToRun{Key: toRunKey}
		if err = datastore.Get(ctx, toRun); err != nil {
			return errors.Annotate(err, "datastore error fetching TaskToRun for task %s", c.TaskID).Err()
		}

		toRun.Consume("")
		toPut = append(toPut, toRun)
		if err = c.enqueueRBECancel(ctx, toRun); err != nil {
			return errors.Annotate(err, "failed to cancel RBE resevation for task %s", c.TaskID).Err()
		}
		return nil
	}

	cancelRunning := func() error {
		// Running task's state is not immediately changed, only the killing bit
		// is set.
		// The server will tell the bot to kill the task on the next bot report;
		// then when the bot reports the task has been terminated, set its state
		// to KILLED.
		trr := &model.TaskRunResult{Key: model.TaskRunResultKey(ctx, tr.Key)}
		if err := datastore.Get(ctx, trr); err != nil {
			return errors.Annotate(err, "datastore error fetching TaskRunResult for task %s", c.TaskID).Err()
		}
		trr.Killing = true
		trr.Abandoned = datastore.NewIndexedNullable(now)
		trr.Modified = now
		toPut = append(toPut, trr)

		// TODO(b/355013314): Cancel children tasks.
		return nil
	}

	if wasRunning {
		if err := cancelRunning(); err != nil {
			return err
		}
	} else {
		if err := cancelPending(); err != nil {
			return err
		}
	}

	if putErr := datastore.Put(ctx, toPut...); putErr != nil {
		return errors.Annotate(putErr, "datastore error saving entities for canceling task %s", c.TaskID).Err()
	}

	return errors.Annotate(c.sendOnTaskUpdate(ctx), "failed to enqueue pubsub notification cloud tasks for canceling task %s", c.TaskID).Err()
}

func (c *Cancellation) enqueueRBECancel(ctx context.Context, toRun *model.TaskToRun) error {
	if c.testEnqueueRBECancel != nil {
		return c.testEnqueueRBECancel(ctx, c.TaskRequest, toRun)
	}
	return rbe.EnqueueCancel(ctx, c.TaskRequest, toRun)
}

func (c *Cancellation) sendOnTaskUpdate(ctx context.Context) error {
	if c.testSendOnTaskUpdate != nil {
		return c.testSendOnTaskUpdate(ctx, c.TaskRequest, c.TaskResultSummary)
	}
	return notifications.SendOnTaskUpdate(ctx, c.TaskRequest, c.TaskResultSummary)
}
