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
	"fmt"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/notifications"
	"go.chromium.org/luci/swarming/server/tasks/taskspb"
)

const maxBatchCancellationRetries = 10

var taskUnknownTag = errtag.Make("unknown task", true)

// CancelOp is an operation of canceling of a single task.
type CancelOp struct {
	// Request is the task being canceled.
	Request *model.TaskRequest
	// KillRunning it true to kill the task even if it has started running.
	KillRunning bool
	// BotID is the bot the task is expected to be running on right now.
	BotID string
}

// CancelOpOutcome is returned by CancelTxn.
type CancelOpOutcome struct {
	// Canceled is true if the task is now being canceled or was just canceled.
	Canceled bool
	// WasRunning is true if the task was already running when it was canceled.
	WasRunning bool
}

// taskID converts TaskRequest key to the task ID for logs.
func (op *CancelOp) taskID() string {
	return model.RequestKeyToTaskID(op.Request.Key, model.AsRequest)
}

// CancelTxn runs the transactional logic to cancel a single task.
//
// Ensures that the associated TaskToRun is canceled (when PENDING) and
// updates the TaskResultSummary/TaskRunResult accordingly.
//
// For PENDING tasks, the TaskResultSummary's state is immediately changed.
// For RUNNING tasks, the TaskRunResult.Killing bit is immediately set, but
// their state (in TaskRunResult and TaskResultSummary) is not changed yet.
//
// Warning: ACL check must have been done before.
func (m *managerImpl) CancelTxn(ctx context.Context, op *CancelOp) (*CancelOpOutcome, error) {
	trs := &model.TaskResultSummary{
		Key: model.TaskResultSummaryKey(ctx, op.Request.Key),
	}
	switch err := datastore.Get(ctx, trs); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, taskUnknownTag.Apply(errors.Fmt("missing TaskResultSummary for task %s: %w", op.taskID(), err))
	case err != nil:
		return nil, errors.Fmt("datastore error fetching TaskResultSummary for task %s: %w", op.taskID(), err)
	}

	origState := trs.State
	wasRunning := origState == apipb.TaskState_RUNNING

	canceled, err := m.runCancelTxn(ctx, op, trs)
	if err != nil {
		return nil, err
	}

	if trs.State != origState {
		txndefer.Defer(ctx, func(ctx context.Context) {
			onTaskStatusChangeSchedulerLatency(ctx, trs)
			if !trs.IsActive() {
				reportOnTaskCompleted(ctx, trs)
			}
		})
	}

	return &CancelOpOutcome{
		Canceled:   canceled,
		WasRunning: wasRunning,
	}, nil
}

// EnqueueBatchCancel enqueues a TQ task to cancel a batch of Swarming tasks.
func (m *managerImpl) EnqueueBatchCancel(ctx context.Context, batch []string, killRunning bool, purpose string, retries int32) error {
	return m.disp.AddTask(ctx, &tq.Task{
		Payload: &taskspb.BatchCancelTask{
			Tasks:       batch,
			KillRunning: killRunning,
			Retries:     retries,
			Purpose:     purpose,
		},
	})
}

// runCancelTxn updates entities of the task to cancel and enqueues the cloud
// tasks to cancel its children and send notifications.
//
// Mutates `trs` in place.
//
// Returns
// * a bool for whether the task has started the cancellation process as requested,
// * an err for errors to cancel the task.
func (m *managerImpl) runCancelTxn(ctx context.Context, op *CancelOp, trs *model.TaskResultSummary) (bool, error) {
	tr := op.Request
	wasRunning := trs.State == apipb.TaskState_RUNNING
	switch {
	case !trs.IsActive():
		// Finished tasks can't be canceled.
		return false, nil
	case wasRunning && !op.KillRunning:
		return false, nil
	case wasRunning && op.BotID != "" && op.BotID != trs.BotID.Get():
		logging.Debugf(ctx, "request to cancel task %s on bot %s, got bot %s instead", op.taskID(), op.BotID, trs.BotID.Get())
		return false, nil
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
			return errors.Fmt("failed to get the TaskToRun key for task %s: %w", op.taskID(), err)
		}
		toRun := &model.TaskToRun{Key: toRunKey}
		switch err = datastore.Get(ctx, toRun); {
		case errors.Is(err, datastore.ErrNoSuchEntity):
			return taskUnknownTag.Apply(errors.Fmt("missing TaskToRun for task %s: %w", op.taskID(), err))
		case err != nil:
			return errors.Fmt("datastore error fetching TaskToRun for task %s: %w", op.taskID(), err)
		}

		trs.ConsumeTaskToRun(toRun, "")
		toPut = append(toPut, toRun)

		// Finalize ResultDB invocation
		// The pending task should not have any children yet, so child task
		// cancellation in FinalizeTask is no-op.
		err = m.disp.AddTask(ctx, &tq.Task{
			Payload: &taskspb.FinalizeTask{
				TaskId: op.taskID(),
			},
		})
		if err != nil {
			return errors.Fmt("failed to enqueue finalization task for cancelling %s: %w", op.taskID(), err)
		}

		return EnqueueRBECancel(ctx, m.disp, tr, toRun)
	}

	cancelRunning := func() error {
		// Running task's state is not immediately changed, only the killing bit
		// is set.
		// The server will tell the bot to kill the task on the next bot report;
		// then when the bot reports the task has been terminated, set its state
		// to KILLED.
		trr := &model.TaskRunResult{Key: model.TaskRunResultKey(ctx, tr.Key)}
		switch err := datastore.Get(ctx, trr); {
		case errors.Is(err, datastore.ErrNoSuchEntity):
			return taskUnknownTag.Apply(errors.Fmt("missing TaskRunResult for task %s: %w", op.taskID(), err))
		case err != nil:
			return errors.Fmt("datastore error fetching TaskRunResult for task %s: %w", op.taskID(), err)
		}
		if trr.Killing {
			// The task has started cancellation. Skip.
			toPut = nil
			return nil
		}

		trr.Killing = true
		trr.Abandoned = datastore.NewIndexedNullable(now)
		trr.Modified = now
		toPut = append(toPut, trr)

		return m.disp.AddTask(ctx, &tq.Task{
			Payload: &taskspb.CancelChildrenTask{
				TaskId: op.taskID(),
			},
		})
	}

	if wasRunning {
		if err := cancelRunning(); err != nil {
			return false, err
		}
	} else {
		if err := cancelPending(); err != nil {
			return false, err
		}
	}

	if len(toPut) == 0 {
		return false, nil
	}
	if putErr := datastore.Put(ctx, toPut...); putErr != nil {
		return false, errors.Fmt("datastore error saving entities for canceling task %s: %w", op.taskID(), putErr)
	}

	if err := notifications.SendOnTaskUpdate(ctx, m.disp, tr, trs); err != nil {
		return false, errors.Fmt("failed to enqueue pubsub notification cloud tasks for canceling task %s: %w", op.taskID(), err)
	}

	if m.testingPostCancelTxn != nil {
		if err := m.testingPostCancelTxn(op.taskID()); err != nil {
			return false, err
		}
	}

	return true, nil
}

// queryToCancel handles CancelChildrenTask TQ task.
//
// It queries the active children of a task then enqueue one or more
// BatchCancelTask tasks to cancel the active children.
//
// batchSize is how many child tasks to fetch before sending them to
// a BatchCancelTask to cancel.
func (m *managerImpl) queryToCancel(ctx context.Context, batchSize int, taskID string) error {
	return getChildTaskResultSummaries(ctx, taskID, batchSize, func(children []*model.TaskResultSummary, batchNum int) error {
		toCancel := make([]string, 0, len(children))
		for _, child := range children {
			if !child.IsActive() {
				continue
			}
			toCancel = append(toCancel, model.RequestKeyToTaskID(child.TaskRequestKey(), model.AsRequest))
		}
		if len(toCancel) == 0 {
			return nil
		}
		return m.EnqueueBatchCancel(ctx, toCancel, true, fmt.Sprintf("cancel children for %s batch %d", taskID, batchNum), 0)
	})
}

func getChildTaskResultSummaries(ctx context.Context, parentID string, batchSize int, sendToCancel func([]*model.TaskResultSummary, int) error) error {
	childReqKeys, err := getChildTaskRequestKeys(ctx, parentID)
	if err != nil {
		return errors.Fmt("failed to get child request keys for parent %s: %w", parentID, err)
	}
	if len(childReqKeys) == 0 {
		return nil
	}

	toGet := make([]*model.TaskResultSummary, len(childReqKeys))
	for i, key := range childReqKeys {
		toGet[i] = &model.TaskResultSummary{
			Key: model.TaskResultSummaryKey(ctx, key),
		}
	}

	i := 0
	for len(toGet) != 0 {
		var batch []*model.TaskResultSummary
		size := min(batchSize, len(toGet))
		batch, toGet = toGet[:size], toGet[size:]
		if err := datastore.Get(ctx, batch); err != nil {
			return errors.Fmt("failed to get child result summary for parent %s: %w", parentID, err)
		}
		if err := sendToCancel(batch, i); err != nil {
			return err
		}
		i++
	}

	return nil
}

func getChildTaskRequestKeys(ctx context.Context, parentID string) ([]*datastore.Key, error) {
	parentReqKey, err := model.TaskIDToRequestKey(ctx, parentID)
	if err != nil {
		return nil, err
	}
	parentRunID := model.RequestKeyToTaskID(parentReqKey, model.AsRunResult)
	// TODO(b/355013314): We should put parent_task_id into TaskResultSummary and use a normal query.
	q := datastore.NewQuery("TaskRequest").Eq("parent_task_id", parentRunID).KeysOnly(true)
	var children []*datastore.Key
	err = datastore.GetAll(ctx, q, &children)
	return children, err
}

func (m *managerImpl) runBatchCancellation(ctx context.Context, workers int, bct *taskspb.BatchCancelTask) error {
	if len(bct.Tasks) == 0 {
		return errors.New("no tasks specified for cancellation")
	}
	if bct.Retries > 0 {
		logging.Infof(ctx, "Retry # %d for %s", bct.Retries, bct.Purpose)
	}

	merr := make(errors.MultiError, len(bct.Tasks))
	eg, _ := errgroup.WithContext(ctx)
	eg.SetLimit(workers)
	for i, t := range bct.Tasks {
		reqKey, err := model.TaskIDToRequestKey(ctx, t)
		if err != nil {
			merr[i] = taskUnknownTag.Apply(errors.Fmt("bad task ID %q, skipping the task: %w", t, err))
			continue
		}
		eg.Go(func() error {
			if err := m.cancelOneTask(ctx, reqKey, bct.KillRunning); err != nil {
				logging.Errorf(ctx, "Cancel %s failed: %s", t, err)
				merr[i] = errors.Fmt("task %s: %w", t, err)
			}
			return nil
		})
	}

	// We use merr to catch the cancellation errors.
	_ = eg.Wait()

	// Enqueue a new task to retry the failed ones.
	toRetry := make([]string, 0, len(bct.Tasks))
	for i, err := range merr {
		if err == nil {
			continue
		}
		if taskUnknownTag.In(err) {
			continue
		}
		toRetry = append(toRetry, bct.Tasks[i])
	}

	if len(toRetry) == 0 {
		return nil
	}
	if bct.Retries >= maxBatchCancellationRetries {
		logging.Errorf(ctx, "%s has retried %d times, give up", bct.Purpose, bct.Retries)
		return nil
	}
	return m.EnqueueBatchCancel(ctx, toRetry, bct.KillRunning, bct.Purpose, bct.Retries+1)
}

func (m *managerImpl) cancelOneTask(ctx context.Context, reqKey *datastore.Key, killRunning bool) error {
	taskReq, err := model.FetchTaskRequest(ctx, reqKey)
	switch {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return taskUnknownTag.Apply(errors.Fmt("missing TaskRequest: %w", err))
	case err != nil:
		return errors.Fmt("datastore error fetching TaskRequest: %w", err)
	}

	var outcome *CancelOpOutcome
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
		outcome, err = m.CancelTxn(ctx, &CancelOp{
			Request:     taskReq,
			KillRunning: killRunning,
		})
		return err
	}, nil)

	if err != nil {
		return err
	}

	logging.Infof(ctx, "Task %s canceled: was running: %v",
		model.RequestKeyToTaskID(reqKey, model.AsRequest),
		outcome.WasRunning,
	)
	return nil
}
