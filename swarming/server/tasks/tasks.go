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

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/swarming/server/cfg"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/notifications"
	"go.chromium.org/luci/swarming/server/rbe"
	"go.chromium.org/luci/swarming/server/tasks/taskspb"
)

// LifecycleTasks is used to emit TQ tasks related to Swarming task lifecycle.
type LifecycleTasks interface {
	EnqueueBatchCancel(ctx context.Context, batch []string, killRunning bool, purpose string, retries int32) error
	enqueueChildCancellation(ctx context.Context, taskID string) error
	enqueueRBECancel(ctx context.Context, tr *model.TaskRequest, ttr *model.TaskToRun) error
	enqueueRBENew(ctx context.Context, tr *model.TaskRequest, ttr *model.TaskToRun, cfg *cfg.Config) error
	sendOnTaskUpdate(ctx context.Context, tr *model.TaskRequest, trs *model.TaskResultSummary) error
}

type LifecycleTasksViaTQ struct {
	Dispatcher *tq.Dispatcher
}

// RegisterTQTasks regusters TQ tasks for task lifecycle management.
func (l *LifecycleTasksViaTQ) RegisterTQTasks() {
	l.Dispatcher.RegisterTaskClass(tq.TaskClass{
		ID:        "cancel-children-tasks-go",
		Kind:      tq.Transactional,
		Prototype: (*taskspb.CancelChildrenTask)(nil),
		Queue:     "cancel-children-tasks-go", // to replace "cancel-children-tasks" taskqueue in Py.
		Handler: func(ctx context.Context, payload proto.Message) error {
			t := payload.(*taskspb.CancelChildrenTask)
			cc := &childCancellation{
				parentID:       t.TaskId,
				batchSize:      300,
				lifecycleTasks: l,
			}
			return cc.queryToCancel(ctx)
		},
	})
	l.Dispatcher.RegisterTaskClass(tq.TaskClass{
		ID:        "cancel-tasks-go",
		Kind:      tq.NonTransactional,
		Prototype: (*taskspb.BatchCancelTask)(nil),
		Queue:     "cancel-tasks-go", // to replace "cancel-tasks" taskqueue in Py.
		Handler: func(ctx context.Context, payload proto.Message) error {
			t := payload.(*taskspb.BatchCancelTask)
			bc := &batchCancellation{
				tasks:          t.Tasks,
				killRunning:    t.KillRunning,
				workers:        64,
				retries:        t.Retries,
				purpose:        t.Purpose,
				lifecycleTasks: l,
			}
			return bc.run(ctx)
		},
	})
}

// EnqueueBatchCancel enqueues a tq task to cancel tasks in batch.
func (l *LifecycleTasksViaTQ) EnqueueBatchCancel(ctx context.Context, batch []string, killRunning bool, purpose string, retries int32) error {
	return tq.AddTask(ctx, &tq.Task{Payload: &taskspb.BatchCancelTask{Tasks: batch, KillRunning: killRunning, Retries: retries, Purpose: purpose}})
}

func (l *LifecycleTasksViaTQ) enqueueChildCancellation(ctx context.Context, taskID string) error {
	return tq.AddTask(ctx, &tq.Task{Payload: &taskspb.CancelChildrenTask{TaskId: taskID}})
}

func (l *LifecycleTasksViaTQ) enqueueRBECancel(ctx context.Context, tr *model.TaskRequest, ttr *model.TaskToRun) error {
	return rbe.EnqueueCancel(ctx, tr, ttr)
}

func (l *LifecycleTasksViaTQ) enqueueRBENew(ctx context.Context, tr *model.TaskRequest, ttr *model.TaskToRun, cfg *cfg.Config) error {
	return rbe.EnqueueNew(ctx, tr, ttr, cfg)
}

func (l *LifecycleTasksViaTQ) sendOnTaskUpdate(ctx context.Context, tr *model.TaskRequest, trs *model.TaskResultSummary) error {
	return notifications.SendOnTaskUpdate(ctx, tr, trs)
}

// TaskWriteOp is used to perform datastore writes on a task throughout its lifecycle.
type TaskWriteOp interface {
	// ClaimTxn calls op.ClaimTxn, but it can be mocked in tests.
	ClaimTxn(ctx context.Context, op *ClaimOp, bot *BotDetails) (*ClaimTxnOutcome, error)
	// FinishClaimOp calls op.Finish, but it can be mocked in tests.
	FinishClaimOp(ctx context.Context, op *ClaimOp, outcome *ClaimTxnOutcome)
}

type TaskWriteOpProd struct{}

func (t *TaskWriteOpProd) ClaimTxn(ctx context.Context, op *ClaimOp, bot *BotDetails) (*ClaimTxnOutcome, error) {
	return op.ClaimTxn(ctx, bot)
}

func (t *TaskWriteOpProd) FinishClaimOp(ctx context.Context, op *ClaimOp, outcome *ClaimTxnOutcome) {
	op.Finished(ctx, outcome)
}
