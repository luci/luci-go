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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/workunits"
)

func sweepWorkUnitsForFinalization(ctx context.Context, rootInvID rootinvocations.ID, seq int64) error {
	isStaleTask, err := checkAndResetFinalizerPending(ctx, rootInvID, seq)
	if err != nil {
		return errors.Fmt("check and reset finalizer pending state: %w", err)
	}
	if isStaleTask {
		logging.Infof(ctx, "exiting stale finalizer task for %q (task seq: %d)", rootInvID.Name(), seq)
		return nil
	}
	for {
		ineligibleCandidates, workUnitsToFinalize, moreToRead, err := findWorkUnitsReadyForFinalization(ctx, rootInvID, findWorkUnitsReadyForFinalizationOptions{})
		if err != nil {
			return errors.Fmt("findWorkUnitsReadyForFinalization: %w", err)
		}
		logging.Infof(ctx, "finalizing %d work units, reset %d not ready work units for %q", len(workUnitsToFinalize), len(ineligibleCandidates), rootInvID.Name())
		if !moreToRead {
			break
		}
	}
	// TODO(beining): implement the write phase.
	return nil
}

// checkAndResetFinalizerPending checks if the finalizer task is stale.
// If the task is not stale, it resets the pending state for the finalizer.
func checkAndResetFinalizerPending(ctx context.Context, rootInvID rootinvocations.ID, seq int64) (isStaleTask bool, err error) {
	isStaleTask = false
	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		taskState, err := rootinvocations.ReadFinalizerTaskState(ctx, rootInvID)
		if err != nil {
			return err
		}
		if taskState.Sequence != seq {
			isStaleTask = true
			return nil
		}
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
	// limitOverwrite overwrites the default limit for the number of work units to
	// process. If 0, a default value is used. For testing only.
	limitOverwrite int
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
//   - ineligibleCandidates: A list of the initial candidates that were not ready for finalization in this batch.
//   - workUnitsToFinalize: A list of work units that are ready to be finalized, including those identified by walking up the hierarchy.
//     The list is ordered so that parents always appear after their children.
//   - moreToRead: A boolean indicating if there might be more work units to process.
func findWorkUnitsReadyForFinalization(ctx context.Context, rootInvID rootinvocations.ID, opts findWorkUnitsReadyForFinalizationOptions) (ineligibleCandidates []workunits.FinalizerCandidate, workUnitsToFinalize []workUnitWithParent, moreToRead bool, err error) {
	// It is safe to perform write in a separate transaction to this read, as any work units
	// that are eligible to be finalized now will still be eligible in future.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	// The limit caps the number of work units in `candidates` and `workUnitsToFinalize` lists.
	// This is necessary because subsequent queries use the IN operator, which has a limit of 10,000.
	// See https://cloud.google.com/spanner/quotas#query-limits.
	const maxLimit = 10000
	limit := opts.limitOverwrite
	if opts.limitOverwrite < 0 || opts.limitOverwrite > maxLimit {
		return nil, nil, false, errors.New(fmt.Sprintf("limit must be between 0 and %d, got %d", maxLimit, opts.limitOverwrite))
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
