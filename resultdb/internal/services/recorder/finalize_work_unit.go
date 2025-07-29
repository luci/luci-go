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

// FinalizeWorkUnit implements pb.RecorderServer.
func (s *recorderServer) FinalizeWorkUnit(ctx context.Context, in *pb.FinalizeWorkUnitRequest) (*pb.WorkUnit, error) {
	if err := verifyFinalizeWorkUnitPermissions(ctx, in); err != nil {
		return nil, err
	}

	wuID := workunits.MustParseName(in.Name)

	var wuRow *workunits.WorkUnitRow
	commitTimestamp, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		var err error
		wuRow, err = workunits.Read(ctx, wuID, workunits.ExcludeExtendedProperties)
		if err != nil {
			return err
		}

		if wuRow.State != pb.WorkUnit_ACTIVE {
			// Finalization already started. Do not start finalization
			// again as doing so would overwrite the existing FinalizeStartTime
			// and create an unnecessary task.
			// This RPC should be idempotent so do not return an error.
			return nil
		}

		// Finalize as requested.
		span.BufferWrite(ctx, workunits.MarkFinalizing(wuID)...)
		wuRow.State = pb.WorkUnit_FINALIZING

		tasks.StartInvocationFinalization(ctx, wuID.LegacyInvocationID())
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !wuRow.FinalizeStartTime.Valid {
		// We set the work unit to finalizing.
		wuRow.FinalizeStartTime = spanner.NullTime{Valid: true, Time: commitTimestamp}
	}

	result := masking.WorkUnit(wuRow, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_BASIC)
	return result, nil
}

func verifyFinalizeWorkUnitPermissions(ctx context.Context, req *pb.FinalizeWorkUnitRequest) error {
	rootInvocationID, workUnitID, err := pbutil.ParseWorkUnitName(req.Name)
	if err != nil {
		return appstatus.BadRequest(errors.Fmt("name: %w", err))
	}

	token, err := extractUpdateToken(ctx)
	if err != nil {
		return err
	}
	wuID := workunits.ID{RootInvocationID: rootinvocations.ID(rootInvocationID), WorkUnitID: workUnitID}
	if err := validateWorkUnitUpdateToken(ctx, token, wuID); err != nil {
		return appstatus.Errorf(codes.PermissionDenied, "invalid update token")
	}
	return nil
}
