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
	cfg, err := config.Service(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateBatchUpdateWorkUnitsRequest(ctx, in, cfg); err != nil {
		return nil, appstatus.BadRequest(err)
	}
	updatedRows, err := updateWorkUnits(ctx, in.Requests)
	if err != nil {
		return nil, err
	}
	respWUs := make([]*pb.WorkUnit, len(updatedRows))
	for i, row := range updatedRows {
		// Use basic field which elides the extended_properties field to limit response size.
		respWUs[i] = masking.WorkUnit(row, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_BASIC)
	}
	return &pb.BatchUpdateWorkUnitsResponse{WorkUnits: respWUs}, nil
}

func updateWorkUnits(ctx context.Context, requests []*pb.UpdateWorkUnitRequest) (updatedRows []*workunits.WorkUnitRow, err error) {
	wuIDs := make([]workunits.ID, len(requests))
	for i, r := range requests {
		wuIDs[i] = workunits.MustParseName(r.WorkUnit.Name)
	}
	updatedRows = make([]*workunits.WorkUnitRow, len(requests))
	updatedWUs := make([]bool, len(requests))
	shouldFinalizeWUs := make([]bool, len(requests))
	commitTimestamp, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		// Reset variables in case the transaction gets retried.
		updatedRows = make([]*workunits.WorkUnitRow, len(requests))
		updatedWUs = make([]bool, len(requests))
		shouldFinalizeWUs = make([]bool, len(requests))

		// ReadBatch returns an error if any of the work units are not found.
		originalWUs, err := workunits.ReadBatch(ctx, wuIDs, workunits.AllFields)
		if err != nil {
			return err
		}
		// Validate assumptions.
		// - All work units are active.
		// - Etag match.
		for i, originalWU := range originalWUs {
			if originalWU.State != pb.WorkUnit_ACTIVE {
				return appstatus.Errorf(codes.FailedPrecondition, "requests[%d]: work unit %q is not active", i, originalWU.ID.Name())
			}
			reqWU := requests[i].WorkUnit
			if reqWU.Etag != "" {
				match, err := masking.IsWorkUnitETagMatch(originalWU, reqWU.Etag)
				if err != nil {
					return appstatus.BadRequest(errors.Fmt("requests[%d]: work_unit: etag: %w", i, err))
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

		// At most 2 mutation - one for updating work unit, one for updating legacy invocation.
		ms := make([]*spanner.Mutation, 0, len(originalWUs)*2)
		for i, originalWU := range originalWUs {
			updateMutations, updatedWURow, err := updateWorkUnitInternal(requests[i], originalWU, i)
			if err != nil {
				return err // BadRequest error, internal error.
			}
			updated := len(updateMutations) > 0
			if updated {
				ms = append(ms, updateMutations...)
				// Trigger finalization task, when the work unit is updated to finalizing.
				if updatedWURow.State == pb.WorkUnit_FINALIZING {
					tasks.StartInvocationFinalization(ctx, wuIDs[i].LegacyInvocationID())
					shouldFinalizeWUs[i] = true
				}
			}
			updatedRows[i] = updatedWURow
			updatedWUs[i] = updated
		}
		span.BufferWrite(ctx, ms...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	for i, wu := range updatedRows {
		if updatedWUs[i] {
			wu.LastUpdated = commitTimestamp
		}
		if shouldFinalizeWUs[i] {
			wu.FinalizeStartTime = spanner.NullTime{Valid: true, Time: commitTimestamp}
		}
	}
	return updatedRows, nil
}

func validateBatchUpdateWorkUnitsRequest(ctx context.Context, in *pb.BatchUpdateWorkUnitsRequest, cfg *config.CompiledServiceConfig) error {
	wuIdx := make(map[workunits.ID]int, len(in.Requests))
	for i, r := range in.Requests {
		if err := validateUpdateWorkUnitRequest(ctx, r, cfg); err != nil {
			return errors.Fmt("requests[%d]: %w", i, err)
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
