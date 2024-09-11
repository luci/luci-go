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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/swarming/server/tasks/taskspb"
)

// RegisterTQTasks regusters Cloud tasks for task lifecycle management.
func RegisterTQTasks(disp *tq.Dispatcher) {
	disp.RegisterTaskClass(tq.TaskClass{
		ID:        "cancel-children-tasks-go",
		Kind:      tq.Transactional,
		Prototype: (*taskspb.CancelChildrenTask)(nil),
		Queue:     "cancel-children-tasks-go", // to replace "cancel-children-tasks" taskqueue in Py.
		Handler: func(ctx context.Context, payload proto.Message) error {
			t := payload.(*taskspb.CancelChildrenTask)
			return (&childCancellation{parentID: t.TaskId, batchSize: 300}).queryToCancel(ctx)
		},
	})
	disp.RegisterTaskClass(tq.TaskClass{
		ID:        "cancel-tasks-go",
		Kind:      tq.NonTransactional,
		Prototype: (*taskspb.BatchCancelTask)(nil),
		Queue:     "cancel-tasks-go", // to replace "cancel-tasks" taskqueue in Py.
		Handler: func(ctx context.Context, payload proto.Message) error {
			return errors.New("NotImplemented")
		},
	})
}
