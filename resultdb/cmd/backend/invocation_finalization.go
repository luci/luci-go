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

package main

import (
	"context"
	"fmt"
	"sync"

	"cloud.google.com/go/spanner"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/resultdb/internal/metrics"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/tasks"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// tryFinalizeInvocation finalizes the invocation unless it directly includes
// an ACTIVE invocation.
// If the invocation is too early to finalize, logs the reason and returns nil.
// Idempotent.
func tryFinalizeInvocation(ctx context.Context, invID span.InvocationID) error {
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		switch should, err := shouldFinalize(ctx, txn, invID); {
		case err != nil:
			return err
		case !should:
			return nil
		default:
			return finalizeInvocation(ctx, txn, invID)
		}
	})
	return err
}

// These are used exclusively inside shouldFinalize.
var (
	errAlreadyFinalized = fmt.Errorf("the invocation is already finalized")
	notReadyToFinalize  = errors.BoolTag{Key: errors.NewTagKey("not ready to get finalized")}
)

// shouldFinalize returns true if the invocation should be finalized.
// An invocation is ready to be finalized if no ACTIVE invocation is reachable
// from it.
func shouldFinalize(ctx context.Context, txn span.Txn, invID span.InvocationID) (bool, error) {
	defer metrics.Trace(ctx, "shouldFinalize")()

	eg, ctx := errgroup.WithContext(ctx)
	defer eg.Wait()

	// Ensure the root invocation is in FINALIZING state.
	eg.Go(func() error {
		switch state, err := span.ReadInvocationState(ctx, txn, invID); {
		case err != nil:
			return err
		case state == pb.Invocation_FINALIZED:
			return errAlreadyFinalized
		case state != pb.Invocation_FINALIZING:
			return errors.Reason("expected %s to be FINALIZING, but it is %s", invID.Name(), state).Err()
		default:
			return nil
		}
	})

	// Walk the graph of invocations, starting from the root, along the inclusion
	// edges.
	// Stop walking as soon as we encounter an active invocation.
	seen := make(span.InvocationIDSet, 1)
	var mu sync.Mutex

	// Limit the number of concurrent queries.
	sem := semaphore.NewWeighted(64)

	var visit func(id span.InvocationID)
	visit = func(id span.InvocationID) {
		// Do not visit same node twice.
		mu.Lock()
		if seen.Has(id) {
			mu.Unlock()
			return
		}
		seen.Add(id)
		mu.Unlock()

		// Concurrently fetch inclusions without a lock.
		eg.Go(func() error {
			// Limit concurrent Spanner queries.
			if err := sem.Acquire(ctx, 1); err != nil {
				return err
			}
			defer sem.Release(1)

			// Ignore inclusions of FINALIZED invocations. An ACTIVE invocation is
			// certainly not reachable from those.
			st := spanner.NewStatement(`
				SELECT included.InvocationId, included.State
				FROM IncludedInvocations incl
				JOIN Invocations included on incl.IncludedInvocationId = included.InvocationId
				WHERE incl.InvocationId = @invID AND included.State != @finalized
			`)
			st.Params = span.ToSpannerMap(map[string]interface{}{
				"finalized": pb.Invocation_FINALIZED,
				"invID":     id,
			})
			var b span.Buffer
			return txn.Query(ctx, st).Do(func(row *spanner.Row) error {
				var includedID span.InvocationID
				var includedState pb.Invocation_State
				switch err := b.FromSpanner(row, &includedID, &includedState); {
				case err != nil:
					return err

				case includedState == pb.Invocation_ACTIVE:
					return errors.Reason("%s is still ACTIVE", includedID.Name()).Tag(notReadyToFinalize).Err()

				case includedState != pb.Invocation_FINALIZING:
					return errors.Reason("%s has unexpected state %s", includedID.Name(), includedState).Err()

				default:
					// The included invocation is FINALIZING and MAY include other
					// still-active invocations. We must go deeper.
					visit(includedID)
					return nil
				}
			})
		})
	}

	visit(invID)

	switch err := eg.Wait(); {
	case errors.Unwrap(err) == errAlreadyFinalized:
		// The invocation is already finalized.
		return false, nil

	case notReadyToFinalize.In(err):
		logging.Infof(ctx, "not ready to finalize: %s", err.Error())
		return false, nil

	default:
		return err == nil, err
	}
}

// finalizeInvocation unconditionally updates the invocation state to FINALIZED.
// Enqueues BigQuery export tasks.
// For each FINALIZING invocation that includes the given one, enqueues
// a finalization task.
func finalizeInvocation(ctx context.Context, txn *spanner.ReadWriteTransaction, invID span.InvocationID) error {
	// Enqueue tasks to try to finalize invocations that include ours.
	if err := insertNextFinalizatoinTasks(ctx, txn, invID); err != nil {
		return err
	}

	// TODO(crbug.com/1032434): Insert BigQuery export tasks

	// Update the invocation state.
	return txn.BufferWrite([]*spanner.Mutation{
		span.UpdateMap("Invocations", map[string]interface{}{
			"InvocationId": invID,
			"State":        pb.Invocation_FINALIZED,
			"FinalizeTime": spanner.CommitTimestamp,
		}),
	})
}

// insertNextFinalizatoinTasks, for each FINALIZING invocation that directly
// includes ours, schedules a task to try to finalize it.
func insertNextFinalizatoinTasks(ctx context.Context, txn *spanner.ReadWriteTransaction, invID span.InvocationID) error {
	// Note: its OK not to schedule a task for active invocations because
	// state transition ACTIVE->FINALIZING includes creating a finalization
	// task.
	// Note: Spanner currently does not support PENDING_COMMIT_TIMESTAMP()
	// in "INSERT INTO ... SELECT" queries.
	st := spanner.NewStatement(`
		INSERT INTO InvocationTasks (TaskType, TaskId, InvocationId, ProcessAfter)
		SELECT @taskType, FORMAT("%s/%s", @invID, including.InvocationId), including.InvocationId, CURRENT_TIMESTAMP()
		FROM IncludedInvocations@{FORCE_INDEX=ReversedIncludedInvocations} incl
		JOIN Invocations including ON incl.InvocationId = including.InvocationId
		WHERE IncludedInvocationId = @invID AND including.State = @finalizing
	`)
	st.Params = span.ToSpannerMap(map[string]interface{}{
		"taskType":   string(tasks.TryFinalizeInvocation),
		"invID":      invID,
		"finalizing": pb.Invocation_FINALIZING,
	})
	count, err := txn.Update(ctx, st)
	if err != nil {
		return errors.Annotate(err, "failed to insert further finalizing tasks").Err()
	}
	logging.Infof(ctx, "Inserted %d %s tasks", count, tasks.TryFinalizeInvocation)
	return nil
}
