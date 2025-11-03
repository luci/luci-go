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

package finalizer

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/spanner"
	"go.opentelemetry.io/otel/attribute"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/tracing"
	"go.chromium.org/luci/resultdb/internal/workunits"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type sweepWorkUnitsForFinalizationOptions struct {
	// Overrides the default number of work units to finalize in a single
	// database transaction. If 0, the value of 2,000 is used.
	// For testing only.
	writeBatchSizeOverride int
	// Overrides the default limit for the number of candidates to read in one
	// round. If 0, the value of 10,000 is used.
	// For testing only.
	readLimitOverride int
	// Hostname of the luci.resultdb.v1.ResultDB service which can be
	// queried to fetch the details of root invocations being sent via pubsub.
	// E.g. "results.api.luci.app".
	resultDBHostname string
}

// sweepWorkUnitsForFinalization implements the batch-based finalizer for work units.
// It finalizes work units within a given root invocation.
//
// The finalization process is iterative. In each iteration, the function:
//  1. Finds a batch of work units that are ready for finalization. A work unit
//     is ready if it's in the FINALIZING state and all its children are FINALIZED.
//     This is done by starting with known candidates and walking up the work unit
//     tree.
//  2. Finalizes the ready work units by setting their state to FINALIZED, and
//     in the same transaction sets the `FinalizerCandidateTime` for the parents of the just-finalized
//     work units. (while 1. will attempt to identify parents that are eligible to be finalized at the
//     same time, due to batching size limits not all parents may be evaluated)
//  3. Resets the `FinalizerCandidateTime` for any initial candidates that were
//     not ready for finalization.
//
// This process repeats until no more finalization candidates are found.
//
// The follow invariants always holds:
// 1. Never enter a state that a finalizing work unit has empty finalizerCandidateTime, and all children are finalized.
// 2. Only transition a work unit to finalized when it has no active/finalizing children.
func sweepWorkUnitsForFinalization(ctx context.Context, rootInvID rootinvocations.ID, seq int64, opts sweepWorkUnitsForFinalizationOptions) (err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/finalizer.sweepWorkUnitsForFinalization")
	defer func() { tracing.End(ts, err) }()

	isStaleTask, err := checkAndResetFinalizerPending(ctx, rootInvID, seq)
	if err != nil {
		return errors.Fmt("check and reset finalizer pending state: %w", err)
	}
	if isStaleTask {
		logging.Infof(ctx, "exiting stale finalizer task for %q (task seq: %d)", rootInvID.Name(), seq)
		return nil
	}
	readOpts := findWorkUnitsReadyForFinalizationOptions{
		limitOverride: opts.readLimitOverride,
	}
	for {
		ineligibleCandidates, workUnitsToFinalize, moreToRead, err := findWorkUnitsReadyForFinalization(ctx, rootInvID, readOpts)
		if err != nil {
			return errors.Fmt("findWorkUnitsReadyForFinalization: %w", err)
		}
		logging.Infof(ctx, "finalizing %d work units, reseting %d not ready work units for %q", len(workUnitsToFinalize), len(ineligibleCandidates), rootInvID.Name())

		writeOpts := applyFinalizationUpdatesOptions{
			batchSizeOverride: opts.writeBatchSizeOverride,
			resultDBHostname:  opts.resultDBHostname,
		}
		err = applyFinalizationUpdates(ctx, rootInvID, workUnitsToFinalize, writeOpts)
		if err != nil {
			return errors.Fmt("apply  work unit finalization updates: %w", err)
		}
		err = markIneligibleCandidates(ctx, ineligibleCandidates)
		if err != nil {
			return errors.Fmt("reset finalizer candidate time for ineligible candidates: %w", err)
		}
		if !moreToRead {
			break
		}
	}
	return nil
}

// checkAndResetFinalizerPending checks if the finalizer task is stale.
// If the task is not stale, it resets the pending state for the finalizer.
func checkAndResetFinalizerPending(ctx context.Context, rootInvID rootinvocations.ID, seq int64) (isStaleTask bool, err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/finalizer.checkAndResetFinalizerPending")
	defer func() { tracing.End(ts, err) }()

	isStaleTask = false
	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		taskState, err := rootinvocations.ReadFinalizerTaskState(ctx, rootInvID)
		if err != nil {
			return err
		}
		// If the sequence number on the root invocation is different, it means a newer sweep
		// has been scheduled, so this task is stale and should exit.
		if taskState.Sequence != seq {
			isStaleTask = true
			return nil
		}
		// The task is current, so reset the pending flag. This allows a new
		// task to be scheduled if another candidate appears later.
		span.BufferWrite(ctx, rootinvocations.ResetFinalizerPending(rootInvID))
		return nil
	})
	if err != nil {
		return false, err
	}
	return isStaleTask, nil
}

type workUnitWithParent struct {
	ID workunits.ID
	// Parent is immuntable for work units, therefore we can cache it in memory.
	// For root work unit, parent is a empty workunits.ID struct.
	Parent workunits.ID
}

type findWorkUnitsReadyForFinalizationOptions struct {
	// limitOverride overrides the default limit for the number of work units to
	// process. If 0, a default value is used. For testing only.
	limitOverride int
}

// findWorkUnitsReadyForFinalization identifies work units that are ready to be
// finalized under a given root invocation.
//
// A work unit is considered ready for finalization if it is in the FINALIZING
// state and all of its direct children are in the FINALIZED state.
//
// This function operates in a single read-only transaction. It starts by
// querying for an initial set of finalization candidates. From this set, it
// finds those that are ready to be finalized and then iteratively walks up the
// work unit tree to find their parents that may now also be ready.
//
// It returns:
//   - ineligibleCandidates: A list of the initial candidates that were not ready for finalization in this batch. The size of this list is at most 10K.
//   - workUnitsToFinalize: A list of work units that are ready to be finalized, including those identified by walking up the hierarchy.
//     The list is ordered so that parents always appear after their children, and the size of this list is at most 10K.
//   - moreToRead: A boolean indicating if there might be more work units to process.
func findWorkUnitsReadyForFinalization(ctx context.Context, rootInvID rootinvocations.ID, opts findWorkUnitsReadyForFinalizationOptions) (ineligibleCandidates []workunits.FinalizerCandidate, workUnitsToFinalize []workUnitWithParent, moreToRead bool, err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/finalizer.findWorkUnitsReadyForFinalization")
	defer func() { tracing.End(ts, err) }()
	// It is safe to perform write in a separate transaction to this read, as any work units
	// that are eligible to be finalized now will still be eligible in future.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	// The limit caps the number of work units in `candidates` and `workUnitsToFinalize` lists.
	// This is necessary because subsequent queries use the IN operator, which has a limit of 10,000.
	// See https://cloud.google.com/spanner/quotas#query-limits.
	const maxLimit = 10_000
	limit := opts.limitOverride
	if opts.limitOverride < 0 || opts.limitOverride > maxLimit {
		return nil, nil, false, errors.New(fmt.Sprintf("limit must be between 0 and %d, got %d", maxLimit, opts.limitOverride))
	}
	if limit == 0 {
		limit = maxLimit
	}
	moreToRead = false
	// Find all work units under this root that are currently marked as candidates.
	candidates, err := workunits.QueryFinalizerCandidates(ctx, rootInvID, limit)
	if err != nil {
		return nil, nil, false, errors.Fmt("querying finalizer candidates: %w", err)
	}
	if len(candidates) == 0 {
		// No candidates, nothing to do.
		return nil, nil, false, nil
	}
	moreToRead = len(candidates) == limit

	// workUnitsToFinalize contains all work units that are ready to be finalized, along with their parent.
	workUnitsToFinalize = []workUnitWithParent{}
	candidateIDs := idSetFromCandidates(candidates)
	for len(workUnitsToFinalize) < limit {
		// From the current set of candidates, find which ones are ready to be finalized.
		// A work unit is ready if all its children are finalized. We can ignore
		// children that are part of the `workUnitsToFinalize` set as they are
		// about to be finalized.
		readyWorkUnitIDs, err := workunits.ReadyToFinalize(ctx, candidateIDs, toFinalizeReadyIDs(workUnitsToFinalize))
		if err != nil {
			return nil, nil, false, errors.Fmt("ready to finalize: %w", err)
		}
		if len(readyWorkUnitIDs) == 0 {
			break
		}
		readyWUs := readyWorkUnitIDs.SortedByID()
		// Cap readyWUs under limit.
		remainLimit := limit - len(workUnitsToFinalize)
		if len(readyWUs) > remainLimit {
			readyWUs = readyWUs[:remainLimit]
		}
		readyWUParents, err := workunits.ReadParents(ctx, readyWUs)
		if err != nil {
			return nil, nil, false, errors.Fmt("reading parents of work units to finalize: %w", err)
		}
		for i := range readyWUs {
			// Add the ready work units from this iteration to the list of work units to finalize.
			// A work unit can only be added once to the workUnitsToFinalize list.
			// This is because work unit becomes ready for finalization at the moment when its
			// last child that was not 'finalize-ready' becomes ready itself.  It's not possible
			// to reach this state twice.
			workUnitsToFinalize = append(workUnitsToFinalize, workUnitWithParent{
				ID:     readyWUs[i],
				Parent: readyWUParents[i],
			})
		}
		// The parents of the finalization ready work units become the candidates for the next iteration.
		candidateIDs = workunits.NewIDSet(readyWUParents...).NonEmptyIDs()
	}
	moreToRead = moreToRead || len(workUnitsToFinalize) == limit

	// Some candidates that were initially ineligible may have become eligible in
	// subsequent iterations as we realised their children are eligible to become
	// FINALIZED. Filter these out so that ineligibleCandidates and workUnitsToFinalize
	// are mutually exclusive sets.
	finalizeWorkUnitIDs := toFinalizeReadyIDs(workUnitsToFinalize)
	ineligibleCandidates = make([]workunits.FinalizerCandidate, 0, len(candidates))
	for i := range candidates {
		if !finalizeWorkUnitIDs.Has(candidates[i].ID) {
			ineligibleCandidates = append(ineligibleCandidates, candidates[i])
		}
	}
	return ineligibleCandidates, workUnitsToFinalize, moreToRead, nil
}

type applyFinalizationUpdatesOptions struct {
	batchSizeOverride int
	resultDBHostname  string
}

// applyFinalizationUpdates commits the finalization state for a given list of work units.
//
// The function relies on the caller to provide `workUnitsToFinalize` sorted with
// children appearing before parents. This ordering guarantees that a parent is only
// processed after its dependent children are finalized.
func applyFinalizationUpdates(ctx context.Context, rootInvID rootinvocations.ID, workUnitsToFinalize []workUnitWithParent, opts applyFinalizationUpdatesOptions) (err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/finalizer.applyFinalizationUpdates",
		attribute.Int("count", len(workUnitsToFinalize)))
	defer func() { tracing.End(ts, err) }()
	// Batching is necessary because spanner has a limit of 80,000 for the number of mutations per commit.
	// See https://cloud.google.com/spanner/quotas#limits-for.
	// Default batch size is 2000 work units per transaction, this allows 40 mutations per work unit.
	defaultBatchSize := 2000
	if opts.batchSizeOverride < 0 || opts.batchSizeOverride > defaultBatchSize {
		return errors.New(fmt.Sprintf("batchSize must be between 0 and %d, got %d", defaultBatchSize, opts.batchSizeOverride))
	}
	batchSize := opts.batchSizeOverride
	if batchSize == 0 {
		batchSize = defaultBatchSize
	}
	batches := batch(workUnitsToFinalize, batchSize)
	for _, finalizeBatch := range batches {
		_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			mutations := []*spanner.Mutation{}
			parents := toFinalizeReadyWorkUnitParents(finalizeBatch).NonEmptyIDs()
			for parent := range parents {
				// For each work unit being finalized, mark its parent (no matter the finalization state of the parent) as a candidate for the next iteration.
				// The blind write is to avoid a read-lock on the parent record.
				// While it would be more efficient to not mark parents which we will finalize in later write batches,
				// the task could fail between applying batches. This ensures those parents will always be picked up for finalization when the task is retried.
				mutations = append(mutations, workunits.SetFinalizerCandidateTime(parent))
			}

			// Re-read the state of work units just before finalizing them. This is a crucial
			// check to prevent a race condition where the state might have changed since the initial read.
			finalizeReadyIDs := toFinalizeReadyIDs(finalizeBatch).SortedByID()
			states, err := workunits.ReadFinalizationStates(ctx, finalizeReadyIDs)
			if err != nil {
				return err
			}

			var finalizedWUIDs []workunits.ID
			shouldFinalizeRootInvocation := false
			// Finalize work units only for work units that are still in the FINALIZING state.
			for i, id := range finalizeReadyIDs {
				if states[i] != pb.WorkUnit_FINALIZING {
					continue
				}
				mutations = append(mutations, workunits.MarkFinalized(id)...)
				finalizedWUIDs = append(finalizedWUIDs, id)
				if id.WorkUnitID == workunits.RootWorkUnitID {
					shouldFinalizeRootInvocation = true
				}
			}

			// Enqueue tasks to publish test results for the finalized work
			// units.
			if len(finalizedWUIDs) > 0 {
				wuIDs := make([]string, len(finalizedWUIDs))
				for i, wuID := range finalizedWUIDs {
					wuIDs[i] = wuID.WorkUnitID
				}
				tq.MustAddTask(ctx, &tq.Task{
					Payload: &taskspb.PublishTestResultsTask{
						RootInvocationId: string(rootInvID),
						WorkUnitIds:      wuIDs,
					},
					Title: fmt.Sprintf("%s-%d", rootInvID.Name(), time.Now().UnixNano()),
				})
			}

			if shouldFinalizeRootInvocation {
				mutations = append(mutations, rootinvocations.MarkFinalized(rootInvID)...)
				// Publish a finalized root invocation pubsub transactionally.
				if err := publishFinalizedRootInvocation(ctx, rootInvID, opts.resultDBHostname); err != nil {
					return errors.Fmt("publish finalized root invocation: %w", err)
				}
			}

			span.BufferWrite(ctx, mutations...)
			return nil
		})
		if err != nil {
			return errors.Fmt("commit updates for finalization ready work units: %w", err)
		}
	}
	return nil
}

// publishFinalizedRootInvocation publishes a pub/sub message for a finalized
func publishFinalizedRootInvocation(ctx context.Context, rootInvID rootinvocations.ID, rdbHostName string) error {
	// Enqueue a notification to pub/sub listeners that the root invocation
	// has been finalized.
	row, err := rootinvocations.Read(ctx, rootInvID)
	if err != nil {
		return errors.Fmt("failed to read finalized root notification info: %w", err)
	}

	// Note that this submits the notification transactionally,
	// i.e. conditionally on this transaction committing.
	notification := &pb.RootInvocationFinalizedNotification{
		RootInvocation: row.ToProto(),
		ResultdbHost:   rdbHostName,
	}
	tasks.NotifyRootInvocationFinalized(ctx, notification)
	return nil
}

// markIneligibleCandidates resets the finalizer candidate time for work units
// that were initially identified as candidates but were not ready for
// finalization in the current sweep.
func markIneligibleCandidates(ctx context.Context, ineligibleCandidates []workunits.FinalizerCandidate) (err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/finalizer.markIneligibleCandidates",
		attribute.Int("count", len(ineligibleCandidates)))
	defer func() { tracing.End(ts, err) }()

	// Reset the finalizer candidate time for ineligible candidates. It is safe to perform
	// this in a separate transaction, because this is a conditional update, it only resets
	// when the finalizerCandidateTime matches the original read.
	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		st := workunits.ResetFinalizerCandidateTime(ineligibleCandidates)
		_, err := span.Update(ctx, st)
		if err != nil {
			return err
		}
		return nil
	})
	return err
}

func batch(ids []workUnitWithParent, batchSize int) [][]workUnitWithParent {
	if len(ids) == 0 {
		return nil
	}
	var batches [][]workUnitWithParent
	for i := 0; i < len(ids); i += batchSize {
		end := min(len(ids), i+batchSize)
		batches = append(batches, ids[i:end])
	}
	return batches
}

// Extract all parents to a IDSet.
func toFinalizeReadyWorkUnitParents(ids []workUnitWithParent) workunits.IDSet {
	result := workunits.NewIDSet()
	for _, id := range ids {
		result.Add(id.Parent)
	}
	return result
}

// Extract all IDs to a IDSet.
func toFinalizeReadyIDs(ids []workUnitWithParent) workunits.IDSet {
	result := workunits.NewIDSet()
	for _, id := range ids {
		result.Add(id.ID)
	}
	return result
}

func idSetFromCandidates(candidates []workunits.FinalizerCandidate) workunits.IDSet {
	ids := workunits.NewIDSet()
	for _, candidate := range candidates {
		ids.Add(candidate.ID)
	}
	return ids
}
