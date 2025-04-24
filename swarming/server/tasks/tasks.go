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

	"go.chromium.org/luci/swarming/server/resultdb"
	"go.chromium.org/luci/swarming/server/tasks/taskspb"
	"go.chromium.org/luci/swarming/server/tqtasks"
)

// Manager exposes various methods to change state of tasks.
//
// Other server subsystems (in particular related to Bots API) use Manager to
// change state of tasks. This allows all task-related interactions to be mocked
// out in tests (see MockedManager).
//
// Assumes the context has go.chromium.org/luci/gae/filter/txndefer filter
// installed. It is used to schedule best-effort post-transaction work (like
// reporting metrics).
type Manager interface {
	// CreateTask creates and stores all the entities for a new task.
	//
	// If a task ID collision happens, a special error `ErrAlreadyExists` will be
	// returned so the server could retry creating the entities.
	CreateTask(ctx context.Context, op *CreationOp) (*CreatedTask, error)

	// EnqueueBatchCancel enqueues a TQ task to cancel a batch of Swarming tasks.
	EnqueueBatchCancel(ctx context.Context, batch []string, killRunning bool, purpose string, retries int32) error

	// ClaimTxn runs the transactional logic to mark the task slice as claimed.
	ClaimTxn(ctx context.Context, op *ClaimOp) (*ClaimOpOutcome, error)
	// CancelTxn runs the transactional logic to cancel a single task.
	CancelTxn(ctx context.Context, op *CancelOp) (*CancelOpOutcome, error)
	// CompleteTxn runs the transactional logic to complete a single task.
	CompleteTxn(ctx context.Context, op *CompleteOp) (*CompleteTxnOutcome, error)
	// UpdateTxn runs the transactional logic to update a single task.
	//
	// It is used to perform updates like reporting stdout, or cost, etc.
	// The update is not suppose to complete a task. Use CompleteTxn for that.
	UpdateTxn(ctx context.Context, op *UpdateOp) (*UpdateTxnOutcome, error)
}

// managerImpl is the "production" implementation of Manager.
//
// It mutates the datastore for real. All operations are implemented in separate
// *.go files.
type managerImpl struct {
	disp                 *tq.Dispatcher
	serverProject        string
	serverVersion        string
	rdb                  resultdb.RecorderFactory
	allowAbandoningTasks bool

	testingPostCancelTxn func(reqID string) error // used in test to inject failures
}

// NewManager constructs a production implementation of Manager.
//
// It also registers its TQ tasks.
func NewManager(tasks *tqtasks.Tasks, serverProject, serverVersion string, rdb resultdb.RecorderFactory, allowAbandoningTasks bool) Manager {
	m := &managerImpl{
		disp:                 tasks.TQ,
		serverProject:        serverProject,
		serverVersion:        serverVersion,
		rdb:                  rdb,
		allowAbandoningTasks: allowAbandoningTasks,
	}
	tasks.CancelChildren.AttachHandler(func(ctx context.Context, payload proto.Message) error {
		t := payload.(*taskspb.CancelChildrenTask)
		return m.queryToCancel(ctx, 300, t.TaskId)
	})
	tasks.BatchCancel.AttachHandler(func(ctx context.Context, payload proto.Message) error {
		return m.runBatchCancellation(ctx, 64, payload.(*taskspb.BatchCancelTask))
	})
	tasks.FinalizeTask.AttachHandler(func(ctx context.Context, payload proto.Message) error {
		t := payload.(*taskspb.FinalizeTask)
		if err := m.finalizeResultDBInvocation(ctx, t.TaskId); err != nil {
			return err
		}
		return m.queryToCancel(ctx, 300, t.TaskId)
	})
	return m
}
