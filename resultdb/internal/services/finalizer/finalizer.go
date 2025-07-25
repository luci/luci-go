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

package finalizer

import (
	"context"
	"fmt"
	"sync"

	"cloud.google.com/go/spanner"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/services/artifactexporter"
	"go.chromium.org/luci/resultdb/internal/services/baselineupdater"
	"go.chromium.org/luci/resultdb/internal/services/bqexporter"
	"go.chromium.org/luci/resultdb/internal/services/testmetadataupdator"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/tracing"
	"go.chromium.org/luci/resultdb/internal/workunits"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type Options struct {
	// Hostname of the luci.resultdb.v1.ResultDB service which can be
	// queried to fetch the details of invocations being sent via pubsub.
	// E.g. "results.api.cr.dev".
	ResultDBHostname string
}

// InitServer initializes a finalizer server.
func InitServer(srv *server.Server, opts Options) {
	tasks.FinalizationTasks.AttachHandler(func(ctx context.Context, msg proto.Message) error {
		task := msg.(*taskspb.TryFinalizeInvocation)
		return tryFinalizeInvocation(ctx, invocations.ID(task.InvocationId), opts)
	})
}

// Invocation finalization is asynchronous. First, an invocation transitions
// from ACTIVE to FINALIZING state and transactionally an invocation task is
// enqueued to try to transition it from FINALIZING to FINALIZED.
// Then the task tries to finalize the invocation:
// 1. Check if the invocation is ready to be finalized.
// 2. Finalize the invocation.
//
// The invocation is ready to be finalized iff it is in FINALIZING state and it
// does not include, directly or indirectly, an active invocation.
// The latter involves a graph traversal.
// Given that a client cannot mutate inclusions of a FINALIZING/FINALIZED
// invocation, this means that once an invocation is ready to be finalized,
// it cannot become un-ready. This is why the check is done in a ready-only
// transaction with minimal contention.
// If the invocation is not ready to finalize, the task is dropped.
// This check is implemented in readyToFinalize() function.
//
// The second part is actual finalization. It is done in a separate read-write
// transaction. First the task checks again if the invocation is still
// FINALIZING. If so, the task changes state to FINALIZED, enqueues BQExport
// tasks and tasks to try to finalize invocations that directly include the
// current one (more about this below).
// The finalization is implemented in finalizeInvocation() function.
//
// If we have a chain of inclusions A includes B, B includes C, where A and B
// are FINALIZING and C is active, then A and B are waiting for C to be
// finalized.
// In this state, tasks attempting to finalize A or B will conclude that they
// are not ready.
// Once C is finalized, a task to try to finalize B is enqueued.
// B gets finalized and it enqueues a task to try to finalize A.
// More generally speaking, whenever a node transitions from FINALIZING to
// FINALIZED, we ping incoming edges. This may cause a chain of pings along
// the edges.
//
// More specifically, given edge (A, B), when finalizing B, A is pinged only if
// it is FINALIZING. It does not make sense to do it if A is FINALIZED for
// obvious reasons; and there is no need to do it if A is ACTIVE because
// a transition ACTIVE->FINALIZING is always accompanied with enqueuing a task
// to try to finalize it.

// tryFinalizeInvocation finalizes the invocation unless it directly or
// indirectly includes an ACTIVE invocation.
// If the invocation is too early to finalize, logs the reason and returns nil.
// Idempotent.
func tryFinalizeInvocation(ctx context.Context, invID invocations.ID, opts Options) error {
	// The check whether the invocation is ready to finalize involves traversing
	// the invocation graph and reading Invocations.State column. Doing so in a
	// RW transaction will cause contention. Fortunately, once an invocation
	// is ready to finalize, it cannot go back to being unready, so doing
	// check and finalization in separate transactions is fine.
	switch ready, err := readyToFinalize(ctx, invID); {
	case err != nil:
		return err

	case !ready:
		return nil

	default:
		logging.Infof(ctx, "decided to finalize %s...", invID.Name())
		return finalizeInvocation(ctx, invID, opts)
	}
}

var errAlreadyFinalized = fmt.Errorf("the invocation is already finalized")

// notReadyToFinalize means the invocation is not ready to finalize.
// It is used exclusively inside readyToFinalize.
var notReadyToFinalize = errtag.Make("not ready to get finalized", true)

// readyToFinalize returns true if the invocation should be finalized.
// An invocation is ready to be finalized if no ACTIVE invocation is reachable
// from it.
func readyToFinalize(ctx context.Context, invID invocations.ID) (ready bool, err error) {
	ctx, ts := tracing.Start(ctx, "resultdb.readyToFinalize")
	defer func() { tracing.End(ts, err) }()

	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	eg, ctx := errgroup.WithContext(ctx)
	defer eg.Wait()

	// Ensure the root invocation is in FINALIZING state.
	eg.Go(func() error {
		return ensureFinalizing(ctx, invID)
	})

	// Walk the graph of invocations, starting from the root, along the inclusion
	// edges.
	// Stop walking as soon as we encounter an active invocation.
	seen := make(invocations.IDSet, 1)
	var mu sync.Mutex

	// Limit the number of concurrent queries.
	sem := semaphore.NewWeighted(64)

	var visit func(id invocations.ID)
	visit = func(id invocations.ID) {
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
			st.Params = spanutil.ToSpannerMap(map[string]any{
				"finalized": pb.Invocation_FINALIZED,
				"invID":     id,
			})
			var b spanutil.Buffer
			return span.Query(ctx, st).Do(func(row *spanner.Row) error {
				var includedID invocations.ID
				var includedState pb.Invocation_State
				switch err := b.FromSpanner(row, &includedID, &includedState); {
				case err != nil:
					return err

				case includedState == pb.Invocation_ACTIVE:
					return notReadyToFinalize.Apply(errors.Fmt("%s is still ACTIVE", includedID.Name()))

				case includedState != pb.Invocation_FINALIZING:
					return errors.Fmt("%s has unexpected state %s", includedID.Name(), includedState)

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

func ensureFinalizing(ctx context.Context, invID invocations.ID) error {
	switch state, err := invocations.ReadState(ctx, invID); {
	case err != nil:
		return err
	case state == pb.Invocation_FINALIZED:
		return errAlreadyFinalized
	case state != pb.Invocation_FINALIZING:
		return errors.Fmt("expected %s to be FINALIZING, but it is %s", invID.Name(), state)
	default:
		return nil
	}
}

// finalizeInvocation updates the invocation state to FINALIZED.
// Enqueues BigQuery export tasks.
// For each FINALIZING invocation that includes the given one, enqueues
// a finalization task.
func finalizeInvocation(ctx context.Context, invID invocations.ID, opts Options) error {
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		// Check the state before proceeding, so that if the invocation already
		// finalized, we return errAlreadyFinalized.
		if err := ensureFinalizing(ctx, invID); err != nil {
			return err
		}

		parentInvs, err := parentsInFinalizingState(ctx, invID)
		if err != nil {
			return err
		}

		// Enqueue tasks to try to finalize invocations that include ours.
		// Note that MustAddTask in a Spanner transaction is essentially
		// a BufferWrite (no RPCs inside), it's fine to call it sequentially
		// and panic on errors.
		for _, id := range parentInvs {
			tq.MustAddTask(ctx, &tq.Task{
				Payload: &taskspb.TryFinalizeInvocation{InvocationId: string(id)},
				Title:   string(id),
			})
		}

		if !invID.IsRootInvocation() && !invID.IsWorkUnit() {
			// Enqueue a notification to pub/sub listeners that the invocation
			// has been finalized.
			inv, err := invocations.ReadFinalizedNotificationInfo(ctx, invID)
			if err != nil {
				return errors.Fmt("failed to read finalized notification info: %w", err)
			}

			// Note that this submits the notification transactionally,
			// i.e. conditionally on this transaction committing.
			notification := &pb.InvocationFinalizedNotification{
				Invocation:   invID.Name(),
				Realm:        inv.Realm,
				IsExportRoot: inv.IsExportRoot,
				ResultdbHost: opts.ResultDBHostname,
				CreateTime:   inv.CreateTime,
			}
			tasks.NotifyInvocationFinalized(ctx, notification)
		}

		// Enqueue update test metadata task transactionally.
		if err := testmetadataupdator.Schedule(ctx, invID); err != nil {
			return err
		}

		// Enqueue export artifact task transactionally.
		if err := artifactexporter.Schedule(ctx, invID); err != nil {
			return err
		}

		// Enqueue BigQuery exports transactionally.
		if err := bqexporter.Schedule(ctx, invID); err != nil {
			return err
		}

		// Work units do not have a submitted state.
		if !invID.IsWorkUnit() {
			// Enqueue baseline update task transactionally.
			submitted, err := invocations.ReadSubmitted(ctx, invID)
			if err != nil {
				return err
			}
			if submitted {
				baselineupdater.Schedule(ctx, string(invID))
			}
		}

		// Mark the invocation finalized.
		// If the invocation is a shadow record for a root invocation
		// or work unit, update via the source of truth.
		if invID.IsRootInvocation() {
			// Also updates the legacy invocation.
			span.BufferWrite(ctx, rootinvocations.MarkFinalized(rootinvocations.MustParseLegacyInvocationID(invID))...)
		} else if invID.IsWorkUnit() {
			// Also updates the legacy invocation.
			span.BufferWrite(ctx, workunits.MarkFinalized(workunits.MustParseLegacyInvocationID(invID))...)
		} else {
			span.BufferWrite(ctx, invocations.MarkFinalized(invID))
		}

		return nil
	})
	switch {
	case err == errAlreadyFinalized:
		return nil
	case err != nil:
		return err
	default:
		return nil
	}
}

// parentsInFinalizingState returns IDs of invocations in FINALIZING state that
// directly include ours.
func parentsInFinalizingState(ctx context.Context, invID invocations.ID) (ids []invocations.ID, err error) {
	st := spanner.NewStatement(`
		SELECT including.InvocationId
		FROM IncludedInvocations@{FORCE_INDEX=ReversedIncludedInvocations} incl
		JOIN Invocations including ON incl.InvocationId = including.InvocationId
		WHERE IncludedInvocationId = @invID AND including.State = @finalizing
	`)
	st.Params = spanutil.ToSpannerMap(map[string]any{
		"invID":      invID.RowID(),
		"finalizing": pb.Invocation_FINALIZING,
	})
	err = span.Query(ctx, st).Do(func(row *spanner.Row) error {
		var id invocations.ID
		if err := spanutil.FromSpanner(row, &id); err != nil {
			return err
		}
		ids = append(ids, id)
		return nil
	})
	return ids, err
}
