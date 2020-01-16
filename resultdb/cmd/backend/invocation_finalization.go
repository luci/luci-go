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

	"golang.org/x/sync/errgroup"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	internalpb "go.chromium.org/luci/resultdb/internal/proto"
	"go.chromium.org/luci/resultdb/internal/span"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// tryFinalizeInvocation finalizes the invocation if it does not include active
// invocations.
// If the invocation is too early to finalize, logs the reason and returns nil.
func tryFinalizeInvocation(ctx context.Context, invID span.InvocationID) error {
	// Do not rush opening a read-write transaction.
	// Start with a read-only transaction to do the check (inside shouldFinalize).
	// Its OK to use separate transactions because
	// - once a invocation is FINALIZING, it does not accept more inclusions
	// - an invocation never transitions from FINALIZING back to ACTIVE
	// This means once an invocaiton is ready to be finalized, it cannot go back
	// "not ready" state.
	switch proceed, err := shouldFinalize(ctx, invID); {
	case err != nil:
		return err
	case !proceed:
		return nil
	}

	// Mark it as finalized and schedule finalization tasks for all invocations
	// that include this one.
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		now := clock.Now(ctx)

		ms := []*spanner.Mutation{
			span.UpdateMap("Invocations", map[string]interface{}{
				"InvocationId": invID,
				"State":        pb.Invocation_FINALIZED,
			}),
		}

		// For each FINALIZING invocation I that directly includes ours, schedule
		// a task to try to finalize I.
		st := spanner.NewStatement(`
			SELECT incl.InvocationId
			FROM IncludedInvocations@{FORCE_INDEX=ReverseIncludedInvocations} incl
			JOIN Invocations including ON incl.InvocationId = including.InvocationId
			WHERE IncludedInvocationId = @includedInvId
			  AND including.State = @finalizing
		`)
		st.Params = span.ToSpannerMap(map[string]interface{}{
			"includedInvId": invID,
			"finalizing":    pb.Invocation_FINALIZING,
		})

		task := &internalpb.InvocationTask{
			Type: &internalpb.InvocationTask_TryFinalizeInvocation{},
		}

		var b span.Buffer
		err := span.Query(ctx, "reverse inclusions", txn, st, func(r *spanner.Row) error {
			var includingID span.InvocationID
			if err := b.FromSpanner(r, &includingID); err != nil {
				return err
			}

			taskKey := span.TaskKey{
				InvocationID: includingID,
				TaskID:       fmt.Sprintf("finalize:from_inv:%s", invID),
			}
			ms = append(ms, span.InsertInvocationTask(taskKey, task, now, false))
			return nil
		})
		if err != nil {
			return err
		}

		return txn.BufferWrite(ms)
	})
	return err
}

// shouldFinalize returns true if the invocation should be finalized.
// If the invocation is not in FINALIZING state,
func shouldFinalize(ctx context.Context, invID span.InvocationID) (bool, error) {
	eg, ctx := errgroup.WithContext(ctx)

	txn := span.Client(ctx).ReadOnlyTransaction()
	defer txn.Close()

	var includedRow *spanner.Row
	eg.Go(func() (err error) {
		st := spanner.NewStatement(`
			SELECT included.InvocationId
			FROM IncludedInvocations incl
			JOIN Invocations included ON incl.IncludedInvocationId = included.InvocationId
			WHERE incl.InvocationID = @invID AND included.State != @finalized
			LIMIT 1
		`)
		st.Params = span.ToSpannerMap(map[string]interface{}{
			"invID":     invID,
			"finalized": pb.Invocation_FINALIZED,
		})
		includedRow, err = span.QueryFirstRow(ctx, span.Client(ctx).Single(), st)
		return
	})

	var state pb.Invocation_State
	eg.Go(func() (err error) {
		state, err = span.ReadInvocationState(ctx, txn, invID)
		return
	})

	switch err := eg.Wait(); {
	case err != nil:
		return false, err

	case state == pb.Invocation_ACTIVE:
		// This fshould not have been created for this invocation.
		return false, errors.Reason("invocation %s is ACTIVE", invID.Name()).Tag(permanentInvocationTaskErrTag).Err()

	case state == pb.Invocation_FINALIZED:
		// Already finalized.
		return false, nil

	case includedRow == nil:
		// No non-finalized included invocations.
		// We can proceed to finalization of this one.
		return true, nil

	default:
		var b span.Buffer
		var includedID span.InvocationID
		if err := b.FromSpanner(includedRow, &includedID); err != nil {
			return false, err
		}

		logging.Infof(ctx, "%s still includes %s which is not finalized; skipping this task", invID.Name(), includedID)
		return false, nil
	}
}
