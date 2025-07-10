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

package resultdb

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func (s *resultDBServer) GetWorkUnit(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
	return nil, appstatus.Error(codes.Unimplemented, "not yet implemented")
}

// verifyGetWorkUnitPermissions verifies the user has access to the
// work unit specified in the request.
func verifyGetWorkUnitPermissions(ctx context.Context, req *pb.GetRootInvocationRequest) (permissions.AccessLevel, error) {
	rootInvocationID, workUnitID, err := pbutil.ParseWorkUnitName(req.Name)
	if err != nil {
		return permissions.NoAccess, appstatus.BadRequest(errors.Fmt("name: %w", err))
	}
	id := workunits.ID{
		RootInvocationID: rootinvocations.ID(rootInvocationID),
		WorkUnitID:       workUnitID,
	}
	// Check access on the root invocation.
	opts := permissions.QueryWorkUnitAccessOptions{
		Full:                 rdbperms.PermGetWorkUnit,
		Limited:              rdbperms.PermListLimitedWorkUnits,
		UpgradeLimitedToFull: rdbperms.PermGetWorkUnit,
	}
	accessLevel, err := permissions.QueryWorkUnitAccess(ctx, id, opts)
	if err != nil {
		// Root invocation does not exist or internal error querying permissions.
		return permissions.NoAccess, err
	}
	if accessLevel == permissions.NoAccess {
		return permissions.NoAccess, appstatus.Errorf(codes.PermissionDenied, `caller does not have permission %s (or %s) on root invocation %s`, rdbperms.PermGetWorkUnit, rdbperms.PermListLimitedWorkUnits, id.RootInvocationID.Name())
	}
	return accessLevel, nil
}

func toWorkUnitProto(in *workunits.WorkUnitRow, accessLevel permissions.AccessLevel, view pb.WorkUnitView) *pb.WorkUnit {
	// TODO: child work units, child invocations.
	result := &pb.WorkUnit{
		Name:             in.ID.Name(),
		WorkUnitId:       in.ID.WorkUnitID,
		State:            in.State,
		Realm:            in.Realm,
		CreateTime:       pbutil.MustTimestampProto(in.CreateTime),
		Creator:          in.CreatedBy,
		Deadline:         pbutil.MustTimestampProto(in.Deadline),
		ProducerResource: in.ProducerResource,
		Tags:             in.Tags,
		Properties:       in.Properties,
		Instructions:     in.Instructions,
	}
	if in.ID.WorkUnitID == "root" {
		result.Parent = in.ID.RootInvocationID.Name()
	} else {
		if !in.ParentWorkUnitID.Valid {
			panic(fmt.Sprintf("invariant violated: parent work unit ID not set on non-root work unit %q", in.ID.Name()))
		}
		result.Parent = workunits.ID{
			RootInvocationID: in.ID.RootInvocationID,
			WorkUnitID:       in.ParentWorkUnitID.StringVal,
		}.Name()
	}
	if view == pb.WorkUnitView_WORK_UNIT_VIEW_FULL {
		result.ExtendedProperties = in.ExtendedProperties
	}
	if in.FinalizeStartTime.Valid {
		result.FinalizeStartTime = pbutil.MustTimestampProto(in.FinalizeStartTime.Time)
	}
	if in.FinalizeTime.Valid {
		result.FinalizeTime = pbutil.MustTimestampProto(in.FinalizeTime.Time)
	}

	if accessLevel == permissions.LimitedAccess {
		// TODO: Add output only field on work unit to indicate fields have been masked.
		result.Properties = nil
		result.Tags = nil
		result.Instructions = nil
		result.ExtendedProperties = nil
	}
	return result
}
