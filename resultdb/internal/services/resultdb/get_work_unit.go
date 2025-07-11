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
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func (s *resultDBServer) GetWorkUnit(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
	rootInvocationID, workUnitID, err := pbutil.ParseWorkUnitName(in.Name)
	if err != nil {
		return nil, appstatus.BadRequest(errors.Fmt("name: %w", err))
	}
	id := workunits.ID{
		RootInvocationID: rootinvocations.ID(rootInvocationID),
		WorkUnitID:       workUnitID,
	}

	// Use one transaction for the entire RPC so that we work with a
	// consistent snapshot of the system state. This is important to
	// prevent subtle bugs and TOC-TOU vulnerabilities.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	// Check permissions. As per google.aip.dev/211
	// this should happen before any other request validation.
	opts := permissions.QueryWorkUnitAccessOptions{
		Full:                 rdbperms.PermGetWorkUnit,
		Limited:              rdbperms.PermListLimitedWorkUnits,
		UpgradeLimitedToFull: rdbperms.PermGetWorkUnit,
	}
	accessLevel, err := permissions.QueryWorkUnitAccess(ctx, id, opts)
	if err != nil {
		return nil, err
	}
	if accessLevel == permissions.NoAccess {
		return nil, appstatus.Errorf(codes.PermissionDenied, `caller does not have permission %s (or %s) on root invocation %s`, rdbperms.PermGetWorkUnit, rdbperms.PermListLimitedWorkUnits, id.RootInvocationID.Name())
	}

	if err := validateGetWorkUnitRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	readMask := workunits.ExcludeExtendedProperties
	if in.View == pb.WorkUnitView_WORK_UNIT_VIEW_FULL && accessLevel == permissions.FullAccess {
		readMask = workunits.AllFields
	}

	// Read the work unit.
	wu, err := workunits.Read(ctx, id, readMask)
	if err != nil {
		return nil, err
	}

	return toWorkUnitProto(wu, accessLevel, in.View), nil
}

func validateGetWorkUnitRequest(in *pb.GetWorkUnitRequest) error {
	// Name is already validated in GetWorkUnit.
	if err := pbutil.ValidateWorkUnitView(in.View); err != nil {
		return errors.Fmt("view: %w", err)
	}
	return nil
}

func toWorkUnitProto(in *workunits.WorkUnitRow, accessLevel permissions.AccessLevel, view pb.WorkUnitView) *pb.WorkUnit {
	// TODO: child work units, child invocations.
	result := &pb.WorkUnit{
		// Include metadata-only fields by default.
		Name:             in.ID.Name(),
		WorkUnitId:       in.ID.WorkUnitID,
		State:            in.State,
		Realm:            in.Realm,
		CreateTime:       pbutil.MustTimestampProto(in.CreateTime),
		Creator:          in.CreatedBy,
		Deadline:         pbutil.MustTimestampProto(in.Deadline),
		ProducerResource: in.ProducerResource,
		IsMasked:         true,
	}
	if accessLevel == permissions.FullAccess {
		result.Tags = in.Tags
		result.Properties = in.Properties
		result.Instructions = in.Instructions
		result.IsMasked = false

		if view == pb.WorkUnitView_WORK_UNIT_VIEW_FULL {
			result.ExtendedProperties = in.ExtendedProperties
		}
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
	if in.FinalizeStartTime.Valid {
		result.FinalizeStartTime = pbutil.MustTimestampProto(in.FinalizeStartTime.Time)
	}
	if in.FinalizeTime.Valid {
		result.FinalizeTime = pbutil.MustTimestampProto(in.FinalizeTime.Time)
	}
	return result
}
