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

package recorder

import (
	"context"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/masking"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// BatchFinalizeWorkUnits implements pb.RecorderServer.
func (s *recorderServer) BatchFinalizeWorkUnits(ctx context.Context, in *pb.BatchFinalizeWorkUnitsRequest) (*pb.BatchFinalizeWorkUnitsResponse, error) {
	if err := verifyBatchFinalizeWorkUnitsPermissions(ctx, in); err != nil {
		return nil, err
	}
	if err := validateBatchFinalizeWorkUnitsRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	wuIDs := mustParseWorkUnitIDsFromFinalizeRequests(in.Requests)

	// Now do the transaction.
	var wuRows []*workunits.WorkUnitRow
	commitTimestamp, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		var err error
		// ReadBatch returns an error if any of the work units are not found.
		wuRows, err = workunits.ReadBatch(ctx, wuIDs, workunits.ExcludeExtendedProperties)
		if err != nil {
			return err
		}

		hasWorkUnitFinalizing := false
		for i, wuRow := range wuRows {
			if wuRow.FinalizationState != pb.WorkUnit_ACTIVE {
				// Finalization already started. Do not start finalization
				// again as doing so would overwrite the existing FinalizeStartTime
				// and create an unnecessary task.
				// This RPC should be idempotent so do not return an error.
				continue
			}

			// Finalize as requested.
			req := in.Requests[i]
			state := req.State
			summaryMarkdown := req.SummaryMarkdown
			if state == pb.WorkUnit_STATE_UNSPECIFIED {
				// We need to transition to some final state to start the process
				// of finalizing the work unit.
				// TODO(meiring): Remove this behaviour once state field becomes mandatory.
				state = pb.WorkUnit_FAILED
				summaryMarkdown = "Client did not report a final state in its FinalizeWorkUnit request."
			}

			mb := workunits.NewMutationBuilder(wuRow.ID)
			// As the state is a terminal state, this will also transition the
			// work unit to FINALIZING.
			mb.UpdateState(state)
			mb.UpdateSummaryMarkdown(pbutil.TruncateSummaryMarkdown(summaryMarkdown))
			span.BufferWrite(ctx, mb.Build()...)

			wuRow.State = state
			wuRow.SummaryMarkdown = summaryMarkdown
			wuRow.FinalizationState = pb.WorkUnit_FINALIZING
			// Set a placeholder timestamps, after the transaction commits we can
			// replace `spanner.CommitTimestamp` with the actual timestamp.
			wuRow.FinalizeStartTime = spanner.NullTime{Valid: true, Time: spanner.CommitTimestamp}
			wuRow.LastUpdated = spanner.CommitTimestamp

			hasWorkUnitFinalizing = true
		}
		if hasWorkUnitFinalizing {
			// Transactionally schedule a work unit finalization task if any work unit transitions to finalizing state.
			if err := tasks.ScheduleWorkUnitsFinalization(ctx, wuRows[0].ID.RootInvocationID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Populate FinalizeStartTime for newly finalized work units.
	for _, wuRow := range wuRows {
		// Check for the placeholder timestamp.
		if wuRow.LastUpdated == spanner.CommitTimestamp {
			// We set the work unit to finalizing.
			wuRow.LastUpdated = commitTimestamp
			wuRow.FinalizeStartTime = spanner.NullTime{Valid: true, Time: commitTimestamp}
		}
	}

	// Prepare response.
	response := &pb.BatchFinalizeWorkUnitsResponse{
		WorkUnits: make([]*pb.WorkUnit, len(wuRows)),
	}
	for i, wuRow := range wuRows {
		// The user has permission to update, so they have full access to view.
		response.WorkUnits[i] = masking.WorkUnit(wuRow, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_BASIC)
	}

	return response, nil
}

func verifyBatchFinalizeWorkUnitsPermissions(ctx context.Context, req *pb.BatchFinalizeWorkUnitsRequest) error {
	// This errors on the case where there are no requests or too many.
	if err := pbutil.ValidateBatchRequestCount(len(req.Requests)); err != nil {
		return appstatus.BadRequest(errors.Fmt("requests: %w", err))
	}

	var rootInvocationID rootinvocations.ID
	var state string

	seenIDs := workunits.NewIDSet()
	for i, r := range req.Requests {
		wuID, err := workunits.ParseName(r.Name)
		if err != nil {
			return appstatus.BadRequest(errors.Fmt("requests[%d]: name: %w", i, err))
		}

		if seenIDs.Has(wuID) {
			return appstatus.BadRequest(errors.Fmt("requests[%d]: name: %q appears more than once in the request", i, r.Name))
		}
		seenIDs.Add(wuID)

		// Check all root invocations are the same. This can generate more helpful errors
		// than checking the token states are equal directly.
		if i == 0 {
			rootInvocationID = wuID.RootInvocationID
		} else if rootInvocationID != wuID.RootInvocationID {
			return appstatus.BadRequest(errors.Fmt("requests[%d]: name: all work units in a batch must belong to the same root invocation; got %q, want %q", i, wuID.RootInvocationID.Name(), rootInvocationID.Name()))
		}

		s := workUnitUpdateTokenState(wuID)
		if i == 0 {
			state = s
		} else if state != s {
			return appstatus.BadRequest(errors.Fmt("requests[%d]: name: work unit %q requires a different update token to request[0]'s %q, but this RPC only accepts one update token", i, wuID.Name(), req.Requests[0].Name))
		}
	}

	// Now check the token.
	token, err := extractUpdateToken(ctx)
	if err != nil {
		return err
	}
	if err := validateWorkUnitUpdateTokenForState(ctx, token, state); err != nil {
		return err // PermissionDenied appstatus error.
	}
	return nil
}

func validateBatchFinalizeWorkUnitsRequest(req *pb.BatchFinalizeWorkUnitsRequest) error {
	for i, r := range req.Requests {
		if err := validateFinalizeWorkUnitRequest(r); err != nil {
			return errors.Fmt("requests[%d]: %w", i, err)
		}
	}
	return nil
}

func validateFinalizeWorkUnitRequest(req *pb.FinalizeWorkUnitRequest) error {
	// TODO(meiring): Make this a required field.
	if req.State != pb.WorkUnit_STATE_UNSPECIFIED {
		if err := pbutil.ValidateWorkUnitState(req.State); err != nil {
			return errors.Fmt("state: %w", err)
		}
		if !pbutil.IsFinalWorkUnitState(req.State) {
			return errors.New("state: must be a terminal state")
		}
	}

	// We do not enforce length limits via the FinalizeWorkUnit RPC.
	// While clients should truncate on their side to avoid request size errors
	// (especially on BatchFinalize RPCs), we handle truncation silently here as
	// a fallback. It is foreseeable that clients have implementation bugs
	// and we'd rather have the error to show users than reject it outright.
	const enforceLength = false
	if err := pbutil.ValidateSummaryMarkdown(req.SummaryMarkdown, enforceLength); err != nil {
		return errors.Fmt("summary_markdown: %w", err)
	}
	return nil
}

func mustParseWorkUnitIDsFromFinalizeRequests(reqs []*pb.FinalizeWorkUnitRequest) []workunits.ID {
	ids := make([]workunits.ID, len(reqs))
	for i, r := range reqs {
		ids[i] = workunits.MustParseName(r.Name)
	}
	return ids
}
