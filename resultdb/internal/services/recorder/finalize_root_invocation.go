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

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/resultdb/internal/workunits"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// FinalizeRootInvocation implements pb.RecorderServer.
func (s *recorderServer) FinalizeRootInvocation(ctx context.Context, in *pb.FinalizeRootInvocationRequest) (*pb.RootInvocation, error) {
	if err := verifyFinalizeRootInvocationPermissions(ctx, in); err != nil {
		return nil, err
	}
	if err := validateFinalizeRootInvocationRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	rootInvocationID := rootinvocations.MustParseName(in.Name)
	rootWorkUnitID := workunits.ID{
		RootInvocationID: rootInvocationID,
		WorkUnitID:       workunits.RootWorkUnitID,
	}

	var invRow *rootinvocations.RootInvocationRow
	commitTimestamp, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		var err error
		invRow, err = rootinvocations.Read(ctx, rootInvocationID)
		if err != nil {
			return err
		}
		if invRow.State == pb.RootInvocation_ACTIVE {
			// Finalize as requested.
			span.BufferWrite(ctx, rootinvocations.MarkFinalizing(rootInvocationID)...)
			invRow.State = pb.RootInvocation_FINALIZING

			tasks.StartInvocationFinalization(ctx, rootInvocationID.LegacyInvocationID())
		} else {
			// Finalization already started. Do not start finalization
			// again as doing so would overwrite the existing FinalizeStartTime
			// and create an unnecessary task.
			// This RPC should be idempotent so do not return an error.
		}

		if in.FinalizationScope == pb.FinalizeRootInvocationRequest_INCLUDE_ROOT_WORK_UNIT {
			wuState, err := workunits.ReadState(ctx, rootWorkUnitID)
			if err != nil {
				return err
			}
			if wuState == pb.WorkUnit_ACTIVE {
				span.BufferWrite(ctx, workunits.MarkFinalizing(rootWorkUnitID)...)
				tasks.StartInvocationFinalization(ctx, rootWorkUnitID.LegacyInvocationID())
			} else {
				// Finalization already started. Do not start finalization again
				// for the same reasons as for root invocations.
				// This RPC should be idempotent so do not return an error.
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !invRow.FinalizeStartTime.Valid {
		// We set the root invocation to finalizing.
		invRow.FinalizeStartTime = spanner.NullTime{Valid: true, Time: commitTimestamp}
	}

	return invRow.ToProto(), nil
}

func verifyFinalizeRootInvocationPermissions(ctx context.Context, req *pb.FinalizeRootInvocationRequest) error {
	rootInvocationID, err := rootinvocations.ParseName(req.Name)
	if err != nil {
		return appstatus.BadRequest(errors.Fmt("name: %w", err))
	}

	token, err := extractUpdateToken(ctx)
	if err != nil {
		return err
	}

	// Root invocation expects the update token of the root work unit.
	rootWorkUnitID := workunits.ID{
		RootInvocationID: rootInvocationID,
		WorkUnitID:       workunits.RootWorkUnitID,
	}
	if err := validateWorkUnitUpdateToken(ctx, token, rootWorkUnitID); err != nil {
		return appstatus.Errorf(codes.PermissionDenied, "invalid update token")
	}
	return nil
}

func validateFinalizeRootInvocationRequest(req *pb.FinalizeRootInvocationRequest) error {
	// Name is already verified in verifyFinalizeRootInvocationPermissions.

	if req.FinalizationScope == pb.FinalizeRootInvocationRequest_FINALIZATION_SCOPE_UNSPECIFIED {
		return errors.New("finalization_scope: unspecified")
	}
	if req.FinalizationScope != pb.FinalizeRootInvocationRequest_EXCLUDE_ROOT_WORK_UNIT &&
		req.FinalizationScope != pb.FinalizeRootInvocationRequest_INCLUDE_ROOT_WORK_UNIT {
		return errors.Fmt("finalization_scope: invalid value (%v)", int32(req.FinalizationScope))
	}
	return nil
}
