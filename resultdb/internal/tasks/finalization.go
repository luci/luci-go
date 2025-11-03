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
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/services/exportnotifier"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"

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

// FinalizeWorkUnitsTask describes how to route finalize work unit tasks.
//
// The handler is implemented in internal/services/finalizer.
var FinalizeWorkUnitsTask = tq.RegisterTaskClass(tq.TaskClass{
	ID:            "finalize-work-units",
	Prototype:     &taskspb.SweepWorkUnitsForFinalization{},
	Kind:          tq.Transactional,
	Queue:         "workunitfinalizer",
	RoutingPrefix: "/internal/tasks/finalizer", // for routing to "finalizer" service
})

// TestResultsPublisher describes how to route tasks to publish test results.
//
// The handler is implemented in internal/services/finalizer.
var TestResultsPublisher = tq.RegisterTaskClass(tq.TaskClass{
	ID:        "publish-test-results",
	Prototype: &taskspb.PublishTestResultsTask{},
	// Ensures the finalized work unit IDs are enqueued transactionally.
	Kind:          tq.Transactional,
	Queue:         "testresultspublisher",
	RoutingPrefix: "/internal/tasks/finalizer", // for routing to "finalizer" service
})

// StartInvocationFinalization enqueues a TryFinalizeInvocation task.
//
// The caller is responsible for ensuring that the invocation was
// previously active (except in case of new invocations being created
// in the FINALIZING state).
//
// TODO(nodir): this package is not a great place for this function, but there
// is no better package at the moment. Keep it here for now, but consider a
// new package as the code base grows.
func StartInvocationFinalization(ctx context.Context, id invocations.ID) {
	if id.IsRootInvocation() || id.IsWorkUnit() {
		panic("root invocation and work unit shouldn't use legacy finalizer")
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

// ScheduleWorkUnitsFinalization ensures there is a task being scheduled to sweep for work units to
// finalize for a given root invocation. If a task is already pending for the root invocation, this function does nothing.
// The task is scheduled with a delay of at most 1 minute.
//
// The caller must call this function within a Spanner read-write transaction.
// To ensure a work unit can be processed by the scheduled task,
// the caller must also set the `FinalizerCandidateTime` for that work unit in the same transaction.
func ScheduleWorkUnitsFinalization(ctx context.Context, rootInvocationID rootinvocations.ID) error {
	taskState, err := rootinvocations.ReadFinalizerTaskState(ctx, rootInvocationID)
	if err != nil {
		return err
	}
	if taskState.Pending {
		// A pending task exists.
		return nil
	}
	// No pending task, schedule a new task with delay and increment the sequence number.
	newSequence := taskState.Sequence + 1
	tq.MustAddTask(ctx, &tq.Task{
		Payload: &taskspb.SweepWorkUnitsForFinalization{
			RootInvocationId: string(rootInvocationID),
			SequenceNumber:   newSequence,
		},
		Title: fmt.Sprintf("%s-seq-%d", rootInvocationID, newSequence),
		ETA:   nextMinuteBoundaryWithOffset(ctx, rootInvocationID),
	})
	span.BufferWrite(ctx, rootinvocations.SetFinalizerPending(rootInvocationID, newSequence))
	return nil
}

// nextMinuteBoundaryWithOffset calculates the next available minute boundary
// plus a deterministic offset based on the root invocation ID.
// For example, if the offset is 15000 ms, and current time is 10:30:14, then the result is 10:30:15.
// This is used to calculate an ETA for a task, spreading the load by ensuring
// not all tasks start at the exact beginning of a minute.
func nextMinuteBoundaryWithOffset(ctx context.Context, id rootinvocations.ID) time.Time {
	// Offset is calculated based on the hash of root invocation id.
	hash := sha256.Sum256([]byte(id))
	hashInt := binary.BigEndian.Uint32(hash[:4])
	offsetMillis := hashInt % 60000
	offsetDuration := time.Duration(offsetMillis) * time.Millisecond

	now := clock.Now(ctx)
	thisMinuteWithOffset := now.Truncate(time.Minute).Add(offsetDuration)
	if now.Before(thisMinuteWithOffset) {
		return thisMinuteWithOffset
	}
	return thisMinuteWithOffset.Add(time.Minute)
}
