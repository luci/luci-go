// Copyright 2020 The LUCI Authors.
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

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/services/exportnotifier"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	// Add support for Spanner transactions in TQ.
	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

// FinalizationTasks describes how to route finalization tasks.
//
// The handler is implemented in internal/services/finalizer.
var FinalizationTasks = tq.RegisterTaskClass(tq.TaskClass{
	ID:            "try-finalize-inv",
	Prototype:     &taskspb.TryFinalizeInvocation{},
	Kind:          tq.Transactional,
	Queue:         "finalizer",                 // use a dedicated queue
	RoutingPrefix: "/internal/tasks/finalizer", // for routing to "finalizer" service
})

// StartInvocationFinalization changes invocation state to FINALIZING
// if updateInv is set, and enqueues a TryFinalizeInvocation task.
//
// The caller is responsible for ensuring that the invocation was
// previously active (except in case of new invocations being created
// in the FINALIZING state).
//
// TODO(nodir): this package is not a great place for this function, but there
// is no better package at the moment. Keep it here for now, but consider a
// new package as the code base grows.
func StartInvocationFinalization(ctx context.Context, id invocations.ID, updateInv bool) {
	if updateInv {
		span.BufferWrite(ctx, spanutil.UpdateMap("Invocations", map[string]any{
			"InvocationId":      id,
			"State":             pb.Invocation_FINALIZING,
			"FinalizeStartTime": spanner.CommitTimestamp,
		}))
	}

	tq.MustAddTask(ctx, &tq.Task{
		Payload: &taskspb.TryFinalizeInvocation{InvocationId: string(id)},
		Title:   string(id),
	})

	// Inform export notifier of the invocation transitioning to
	// FINALIZING.
	exportnotifier.EnqueueTask(ctx, &taskspb.RunExportNotifications{
		InvocationId: string(id),
	})
}
