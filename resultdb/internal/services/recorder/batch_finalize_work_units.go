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

		for _, wuRow := range wuRows {
			if wuRow.State != pb.WorkUnit_ACTIVE {
				// Finalization already started. Do not start finalization
				// again as doing so would overwrite the existing FinalizeStartTime
				// and create an unnecessary task.
				// This RPC should be idempotent so do not return an error.
				continue
			}

			// Finalize as requested.
			span.BufferWrite(ctx, workunits.MarkFinalizing(wuRow.ID)...)
			wuRow.State = pb.WorkUnit_FINALIZING

			tasks.StartInvocationFinalization(ctx, wuRow.ID.LegacyInvocationID())
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Populate FinalizeStartTime for newly finalized work units.
	for _, wuRow := range wuRows {
		if !wuRow.FinalizeStartTime.Valid {
			// We set the work unit to finalizing.
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
		return appstatus.Errorf(codes.PermissionDenied, "invalid update token")
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
