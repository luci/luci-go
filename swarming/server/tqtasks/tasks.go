// Copyright 2025 The LUCI Authors.
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

// Package tqtasks is a registry of TQ tasks used inside the Swarming server.
//
// Pulling them into a separate package simplifies testing interactions between
// different components of the server.
//
// Only includes tasks that are involved in communication between two or more
// different components (and that may need to be mocked in tests). Tasks that
// are used internally by a component are not included.
package tqtasks

import (
	"go.chromium.org/luci/server/tq"

	internalspb "go.chromium.org/luci/swarming/proto/internals"
	notificationspb "go.chromium.org/luci/swarming/server/notifications/taskspb"
	"go.chromium.org/luci/swarming/server/tasks/taskspb"

	// Enable datastore transactional tasks support.
	_ "go.chromium.org/luci/server/tq/txn/datastore"
)

// Tasks is a collection of registered TQ task classes.
type Tasks struct {
	TQ *tq.Dispatcher // the dispatcher they are registered in

	// Tasks implemented by notifications.PubSubNotifier.
	PubSubNotify      tq.TaskClassRef
	BuildbucketNotify tq.TaskClassRef

	// Tasks implemented by reservations.ReservationServer.
	EnqueueRBE tq.TaskClassRef
	CancelRBE  tq.TaskClassRef

	// Tasks implemented by tasks.Manager.
	CancelChildren tq.TaskClassRef
	BatchCancel    tq.TaskClassRef
	FinalizeTask   tq.TaskClassRef
}

// Register registers all task classes in the dispatcher.
//
// They have no handlers attached to them. Handlers are attached by the
// corresponding server subsystems (or by tests that mock them out).
func Register(disp *tq.Dispatcher) *Tasks {
	return &Tasks{
		TQ: disp,

		PubSubNotify: disp.RegisterTaskClass(tq.TaskClass{
			ID:        "pubsub-go",
			Kind:      tq.Transactional,
			Prototype: (*notificationspb.PubSubNotifyTask)(nil),
			Queue:     "pubsub-go",
		}),

		BuildbucketNotify: disp.RegisterTaskClass(tq.TaskClass{
			ID:        "buildbucket-notify-go",
			Kind:      tq.Transactional,
			Prototype: (*notificationspb.BuildbucketNotifyTask)(nil),
			Queue:     "buildbucket-notify-go",
		}),

		EnqueueRBE: disp.RegisterTaskClass(tq.TaskClass{
			ID:        "rbe-enqueue",
			Prototype: &internalspb.EnqueueRBETask{},
			Kind:      tq.Transactional,
			Queue:     "rbe-enqueue",
		}),

		CancelRBE: disp.RegisterTaskClass(tq.TaskClass{
			ID:        "rbe-cancel",
			Prototype: &internalspb.CancelRBETask{},
			Kind:      tq.Transactional,
			Queue:     "rbe-cancel",
		}),

		CancelChildren: disp.RegisterTaskClass(tq.TaskClass{
			ID:        "cancel-children-tasks-go",
			Kind:      tq.Transactional,
			Prototype: (*taskspb.CancelChildrenTask)(nil),
			Queue:     "cancel-children-tasks-go",
		}),

		BatchCancel: disp.RegisterTaskClass(tq.TaskClass{
			ID:        "cancel-tasks-go",
			Kind:      tq.NonTransactional,
			Prototype: (*taskspb.BatchCancelTask)(nil),
			Queue:     "cancel-tasks-go",
		}),

		FinalizeTask: disp.RegisterTaskClass(tq.TaskClass{
			ID:        "finalize-task",
			Kind:      tq.Transactional,
			Prototype: (*taskspb.FinalizeTask)(nil),
			Queue:     "finalize-task",
		}),
	}
}
