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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/masking"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// BatchUpdateWorkUnits implements pb.RecorderServer.
func (s *recorderServer) BatchUpdateWorkUnits(ctx context.Context, in *pb.BatchUpdateWorkUnitsRequest) (*pb.BatchUpdateWorkUnitsResponse, error) {
	if err := verifyBatchUpdateWorkUnitsPermissions(ctx, in); err != nil {
		return nil, err
	}
	if err := validateBatchUpdateWorkUnitsRequest(ctx, in); err != nil {
		return nil, appstatus.BadRequest(err)
	}
	updatedRows, err := updateWorkUnits(ctx, in.Requests, in.RequestId)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Service(ctx)
	if err != nil {
		return nil, err
	}

	respWUs := make([]*pb.WorkUnit, len(updatedRows))
	for i, row := range updatedRows {
		// Use basic field which elides the extended_properties field to limit response size.
		respWUs[i] = masking.WorkUnit(row, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_BASIC, cfg)
	}
	return &pb.BatchUpdateWorkUnitsResponse{WorkUnits: respWUs}, nil
}

func updateWorkUnits(ctx context.Context, requests []*pb.UpdateWorkUnitRequest, requestID string) (updatedRows []*workunits.WorkUnitRow, err error) {
	wuIDs := make([]workunits.ID, len(requests))
	for i, r := range requests {
		wuIDs[i] = workunits.MustParseName(r.WorkUnit.Name)
	}
	updatedBy := string(auth.CurrentIdentity(ctx))

	updatedRows = make([]*workunits.WorkUnitRow, len(requests))
	commitTimestamp, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		// Reset variables in case the transaction gets retried.
		updatedRows = make([]*workunits.WorkUnitRow, len(requests))

		// ReadBatch returns an error if any of the work units are not found.
		originalWUs, err := workunits.ReadBatch(ctx, wuIDs, workunits.AllFields)
		if err != nil {
			return err
		}

		dedup, err := deduplicateUpdateWorkUnits(ctx, wuIDs, updatedBy, requestID)
		if err != nil {
			return err
		}
		if dedup {
			// This call should be deduplicated, so do not write to the database.
			// Return the work units as they were read at the start of this transaction.
			// This is a best-effort attempt to return the same response as the original
			// request. Note that if the work units were modified between the original
			// request and this one, this RPC will return the current state of the
			// work units, which will be different from the original response.
			copy(updatedRows, originalWUs)
			return nil
		}

		// Validate assumptions.
		// - All work units are active.
		// - Etag match.
		for i, originalWU := range originalWUs {
			if originalWU.FinalizationState != pb.WorkUnit_ACTIVE {
				return appstatus.Errorf(codes.FailedPrecondition, "requests[%d]: work unit %q is not active", i, originalWU.ID.Name())
			}
			reqWU := requests[i].WorkUnit
			if reqWU.Etag != "" {
				match, err := masking.IsWorkUnitETagMatch(originalWU, reqWU.Etag)
				if err != nil {
					// Impossible, etag has already been validated.
					return err
				}
				if !match {
					// Attach a codes.Aborted appstatus to a vanilla error to avoid
					// ReadWriteTransaction interpreting this case for a scenario
					// in which it should retry the transaction.
					err := errors.Fmt("etag mismatch")
					return appstatus.Attach(err, status.Newf(codes.Aborted, "requests[%d]: the work unit was modified since it was last read; the update was not applied", i))
				}
			}
		}

		// At most 3 mutations for each work unit - one for work unit row, one for legacy invocation row,
		// one for WorkUnitUpdateRequests row.
		ms := make([]*spanner.Mutation, 0, len(originalWUs)*3)
		hasWorkUnitFinalizing := false
		for i, originalWU := range originalWUs {
			updateMutations, updatedWURow, err := updateWorkUnitInternal(requests[i], originalWU, i)
			if err != nil {
				return err // BadRequest error, internal error.
			}
			updated := len(updateMutations) > 0
			if updated {
				ms = append(ms, updateMutations...)
				// Trigger finalization task, when the work unit is updated to a final state.
				if pbutil.IsFinalWorkUnitState(updatedWURow.State) {
					hasWorkUnitFinalizing = true
				}
			}
			updatedRows[i] = updatedWURow

			// Insert into WorkUnitUpdateRequests table.
			ms = append(ms, workunits.CreateWorkUnitUpdateRequest(wuIDs[i], updatedBy, requestID))
		}
		if hasWorkUnitFinalizing {
			// Transactionally schedule a work unit finalization task if any work unit transitions to FINALIZING.
			if err := tasks.ScheduleWorkUnitsFinalization(ctx, originalWUs[0].ID.RootInvocationID); err != nil {
				return err
			}
		}
		span.BufferWrite(ctx, ms...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, wu := range updatedRows {
		// Update all spanner.CommitTimestamp placeholders with the actual commit timestamp.
		if wu.LastUpdated == spanner.CommitTimestamp {
			wu.LastUpdated = commitTimestamp
		}
		if wu.FinalizeStartTime == (spanner.NullTime{Valid: true, Time: spanner.CommitTimestamp}) {
			wu.FinalizeStartTime = spanner.NullTime{Valid: true, Time: commitTimestamp}
		}
	}
	return updatedRows, nil
}

func deduplicateUpdateWorkUnits(ctx context.Context, ids []workunits.ID, updatedBy, requestID string) (shouldDedup bool, err error) {
	exists, err := workunits.CheckWorkUnitUpdateRequestsExist(ctx, ids, updatedBy, requestID)
	if err != nil {
		return false, err
	}
	var exampleExistWorkUnit workunits.ID
	existCount := 0
	for i, id := range ids {
		if exists[id] {
			if exampleExistWorkUnit == (workunits.ID{}) {
				exampleExistWorkUnit = ids[i]
			}
			existCount++
		}
	}
	if existCount == 0 {
		// Do not deduplicate, none of the id exists.
		return false, nil
	}
	if existCount != len(ids) {
		// some ids already exist, but some doesn't exist.
		// Could happen if someone sent two different but overlapping batch update
		// requests, but reused the request_id.
		return false, appstatus.Errorf(codes.FailedPrecondition, "request_id %q was used for some work units in the request (eg. %q), but not others", requestID, exampleExistWorkUnit.Name())
	}
	// All id exist, deduplicate this call.
	return true, nil
}

func validateBatchUpdateWorkUnitsRequest(ctx context.Context, in *pb.BatchUpdateWorkUnitsRequest) error {
	if in.RequestId == "" {
		// Request ID is required to ensure requests are treated idempotently
		// in case of inevitable retries.
		return errors.Fmt("request_id: unspecified (please provide a per-request UUID to ensure idempotence)")
	}
	if err := pbutil.ValidateRequestID(in.RequestId); err != nil {
		return errors.Fmt("request_id: %w", err)
	}
	wuIdx := make(map[workunits.ID]int, len(in.Requests))
	for i, r := range in.Requests {
		requireRequestID := false
		if err := validateUpdateWorkUnitRequest(ctx, r, requireRequestID); err != nil {
			return errors.Fmt("requests[%d]: %w", i, err)
		}
		if r.RequestId != "" && r.RequestId != in.RequestId {
			return errors.Fmt("requests[%d]: request_id: inconsistent with top-level request_id", i)
		}
		wuID := workunits.MustParseName(r.WorkUnit.Name)
		if dupIdx, ok := wuIdx[wuID]; ok {
			return errors.Fmt("requests[%d]: work_unit: name: duplicated work unit with requests[%d]", i, dupIdx)
		}
		wuIdx[wuID] = i
	}
	return nil
}

func verifyBatchUpdateWorkUnitsPermissions(ctx context.Context, in *pb.BatchUpdateWorkUnitsRequest) error {
	// Only perform minimal validation necessary to verify permissions. Full validation
	// will be performed in validateBatchUpdateWorkUnitsRequest.

	// For denial of service reasons, drop large requests early.
	if err := pbutil.ValidateBatchRequestCountAndSize(in.Requests); err != nil {
		return appstatus.BadRequest(errors.Fmt("requests: %w", err))
	}
	ids := make([]workunits.ID, len(in.Requests))
	for i, r := range in.Requests {
		if r.WorkUnit == nil {
			return appstatus.BadRequest(errors.Fmt("requests[%d]: work_unit: unspecified", i))
		}
		id, err := workunits.ParseName(r.WorkUnit.Name)
		if err != nil {
			return appstatus.BadRequest(errors.Fmt("requests[%d]: work_unit: name: %w", i, err))
		}
		ids[i] = id
	}
	token, err := extractUpdateToken(ctx)
	if err != nil {
		return err
	}
	// Ensure the requests are ones we could authorise with a single update
	// token.
	state, err := validateSameUpdateTokenState(ids, "work_unit: name")
	if err != nil {
		return appstatus.BadRequest(err)
	}
	if err := validateWorkUnitUpdateTokenForState(ctx, token, state); err != nil {
		return err // PermissionDenied appstatus error.
	}
	return nil
}
