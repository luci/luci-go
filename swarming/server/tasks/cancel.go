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
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model"
)

const maxBatchCancellationRetries = 10

var taskUnknownTag = errtag.Make("unknown task", true)

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

	// LifecycleTasks is used to emit TQ tasks related to Swarming task lifecycle.
	LifecycleTasks LifecycleTasks
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
// Returns
// * a bool for whether the task has started the cancellation process as requested,
// * a bool for whether the task was running when being canceled,
// * an err for errors to cancel the task.
func (c *Cancellation) Run(ctx context.Context) (bool, bool, error) {
	if err := c.validate(); err != nil {
		return false, false, err
	}
	trKey, err := model.TaskIDToRequestKey(ctx, c.TaskID)
	if err != nil {
		return false, false, err
	}

	var stateChanged, canceled, wasRunning bool
	var toGet []any
	if c.TaskRequest == nil {
		c.TaskRequest = &model.TaskRequest{Key: trKey}
		toGet = append(toGet, c.TaskRequest)
	} else {
		if !c.TaskRequest.Key.Equal(trKey) {
			return false, false, errors.Reason("mismatched TaskID %s and TaskRequest with id %s", c.TaskID, model.RequestKeyToTaskID(c.TaskRequest.Key, model.AsRequest)).Err()
		}
	}

	c.TaskResultSummary = &model.TaskResultSummary{Key: model.TaskResultSummaryKey(ctx, trKey)}
	toGet = append(toGet, c.TaskResultSummary)

	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		if err := datastore.Get(ctx, toGet); err != nil {
			if err == datastore.ErrNoSuchEntity {
				return errors.Annotate(err, "missing TaskRequest or TaskResultSummary for task %s", c.TaskID).Tag(taskUnknownTag).Err()
			}
			return errors.Annotate(err, "datastore error fetching entities for task %s", c.TaskID).Err()
		}

		origState := c.TaskResultSummary.State
		wasRunning = origState == apipb.TaskState_RUNNING
		canceled, err = c.RunInTxn(ctx)
		if err != nil {
			return err
		}

		stateChanged = c.TaskResultSummary.State != origState
		return nil
	}, nil)

	if err != nil {
		return false, false, err
	}

	if stateChanged {
		onTaskStatusChangeSchedulerLatency(ctx, c.TaskResultSummary)
	}

	return canceled, wasRunning, nil
}

// RunInTxn updates entities of the task to cancel and enqueues the cloud tasks
// to cancel its children and send notifications.
//
// Mutates c.TaskResultSummary in place.
//
// Must run in a transaction.
//
// Returns
// * a bool for whether the task has started the cancellation process as requested,
// * an err for errors to cancel the task.
func (c *Cancellation) RunInTxn(ctx context.Context) (bool, error) {
	if datastore.CurrentTransaction(ctx) == nil {
		panic("cancel.RunInTxn must run in a transaction")
	}

	if err := c.validate(); err != nil {
		return false, err
	}
	if c.TaskRequest == nil || c.TaskResultSummary == nil {
		return false, errors.Reason("missing entities when cancelling %s", c.TaskID).Err()
	}

	tr := c.TaskRequest
	trs := c.TaskResultSummary
	wasRunning := trs.State == apipb.TaskState_RUNNING
	switch {
	case !trs.IsActive():
		// Finished tasks can't be canceled.
		return false, nil
	case wasRunning && !c.KillRunning:
		return false, nil
	case wasRunning && c.BotID != "" && c.BotID != trs.BotID.Get():
		logging.Debugf(ctx, "request to cancel task %s on bot %s, got bot %s instead", c.TaskID, c.BotID, trs.BotID.Get())
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
			return errors.Annotate(err, "failed to get the TaskToRun key for task %s", c.TaskID).Err()
		}
		toRun := &model.TaskToRun{Key: toRunKey}
		if err = datastore.Get(ctx, toRun); err != nil {
			if err == datastore.ErrNoSuchEntity {
				return errors.Annotate(err, "missing TaskToRun for task %s", c.TaskID).Tag(taskUnknownTag).Err()
			}
			return errors.Annotate(err, "datastore error fetching TaskToRun for task %s", c.TaskID).Err()
		}

		toRun.Consume("")
		toPut = append(toPut, toRun)
		if err = c.LifecycleTasks.enqueueRBECancel(ctx, tr, toRun); err != nil {
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
			if err == datastore.ErrNoSuchEntity {
				return errors.Annotate(err, "missing TaskRunResult for task %s", c.TaskID).Tag(taskUnknownTag).Err()
			}
			return errors.Annotate(err, "datastore error fetching TaskRunResult for task %s", c.TaskID).Err()
		}
		if trr.Killing {
			// The task has started cancelation. Skip.
			toPut = nil
			return nil
		}
		trr.Killing = true
		trr.Abandoned = datastore.NewIndexedNullable(now)
		trr.Modified = now
		toPut = append(toPut, trr)

		return c.LifecycleTasks.enqueueChildCancellation(ctx, c.TaskID)
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
		return false, errors.Annotate(putErr, "datastore error saving entities for canceling task %s", c.TaskID).Err()
	}

	if err := c.LifecycleTasks.sendOnTaskUpdate(ctx, tr, trs); err != nil {
		return false, errors.Annotate(err, "failed to enqueue pubsub notification cloud tasks for canceling task %s", c.TaskID).Err()
	}
	return true, nil
}

type childCancellation struct {
	parentID string

	// How many child tasks to fetch before sending them to a BatchCancelTask to
	// cancel.
	batchSize int

	lifecycleTasks LifecycleTasks
}

func (cc *childCancellation) validate() error {
	if cc.parentID == "" {
		return errors.New("parent_id is required")
	}
	if cc.batchSize == 0 {
		return errors.New("batch size is required")
	}
	return nil
}

// queryToCancel handles CancelChildrenTask.
// It queries the active children of a task then enqueue one or more BatchCancelTask
// tasks to cancel the active children.
func (cc *childCancellation) queryToCancel(ctx context.Context) error {
	if err := cc.validate(); err != nil {
		return err
	}
	return cc.getChildTaskResultSummaries(ctx, func(children []*model.TaskResultSummary, batchNum int) error {
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

		return cc.lifecycleTasks.EnqueueBatchCancel(ctx, toCancel, true, fmt.Sprintf("cancel children for %s batch %d", cc.parentID, batchNum), 0)
	})
}

func (cc *childCancellation) getChildTaskResultSummaries(ctx context.Context, sendToCancel func([]*model.TaskResultSummary, int) error) error {
	childReqKeys, err := cc.getChildTaskRequestKeys(ctx)
	if err != nil {
		return errors.Annotate(err, "failed to get child request keys for parent %s", cc.parentID).Err()
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
		size := min(cc.batchSize, len(toGet))
		batch, toGet = toGet[:size], toGet[size:]
		if err := datastore.Get(ctx, batch); err != nil {
			return errors.Annotate(err, "failed to get child result summary for parent %s", cc.parentID).Err()
		}
		if err := sendToCancel(batch, i); err != nil {
			return err
		}
		i++
	}

	return nil
}

func (cc *childCancellation) getChildTaskRequestKeys(ctx context.Context) ([]*datastore.Key, error) {
	parentReqKey, err := model.TaskIDToRequestKey(ctx, cc.parentID)
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

type batchCancellation struct {
	tasks       []string
	killRunning bool
	purpose     string
	retries     int32

	workers int

	lifecycleTasks LifecycleTasks
}

func (bc *batchCancellation) run(ctx context.Context) error {
	if len(bc.tasks) == 0 {
		return errors.New("no tasks specified for cancellation")
	}
	if bc.workers <= 0 {
		return errors.New("must specify a positive number of workers")
	}
	if bc.retries > 0 {
		logging.Infof(ctx, "Retry # %d for %s", bc.retries, bc.purpose)
	}

	merr := make(errors.MultiError, len(bc.tasks))
	eg, _ := errgroup.WithContext(ctx)
	eg.SetLimit(bc.workers)
	for i, t := range bc.tasks {
		i := i
		t := t
		eg.Go(func() error {
			c := &Cancellation{
				TaskID:         t,
				KillRunning:    bc.killRunning,
				LifecycleTasks: bc.lifecycleTasks,
			}
			_, wasRunning, err := c.Run(ctx)
			if err == nil {
				logging.Infof(ctx, "Task %s canceled: was running: %v", t, wasRunning)
			} else {
				merr[i] = err
				logging.Errorf(ctx, "Cancel %s failed: %s", t, err)
			}
			return nil
		})
	}

	// We use merr to catch the cancellation errors.
	_ = eg.Wait()

	// Enqueue a new task to retry the failed ones.
	toRetry := make([]string, 0, len(bc.tasks))
	for i, err := range merr {
		if err == nil {
			continue
		}
		if taskUnknownTag.In(err) {
			continue
		}
		toRetry = append(toRetry, bc.tasks[i])
	}

	if len(toRetry) == 0 {
		return nil
	}
	if bc.retries >= maxBatchCancellationRetries {
		logging.Errorf(ctx, "%s has retried %d times, give up", bc.purpose, bc.retries)
		return nil
	}
	return bc.lifecycleTasks.EnqueueBatchCancel(ctx, toRetry, bc.killRunning, bc.purpose, bc.retries+1)
}
