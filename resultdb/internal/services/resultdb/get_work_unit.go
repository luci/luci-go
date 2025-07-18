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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/masking"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func (s *resultDBServer) GetWorkUnit(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
	// Use one transaction for the entire RPC so that we work with a
	// consistent snapshot of the system state. This is important to
	// prevent subtle bugs and TOC-TOU vulnerabilities.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	// Check permissions. As per google.aip.dev/211
	// this should happen before any other request validation.
	id, accessLevel, err := queryWorkUnitAccess(ctx, in)
	if err != nil {
		return nil, err
	}
	if accessLevel == permissions.NoAccess {
		return nil, appstatus.Errorf(codes.PermissionDenied, `caller does not have permission %s (or %s) on root invocation %q`,
			rdbperms.PermGetWorkUnit, rdbperms.PermListLimitedWorkUnits, id.RootInvocationID.Name())
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

	childWUs, err := workunits.ReadChildren(ctx, id)
	if err != nil {
		return nil, err
	}

	childInvs, err := invocations.ReadIncluded(ctx, id.LegacyInvocationID())
	if err != nil {
		return nil, err
	}

	inputs := masking.WorkUnitFields{
		Row:              wu,
		ChildWorkUnits:   childWUs,
		ChildInvocations: childInvs.SortByRowID(),
	}
	return masking.WorkUnit(inputs, accessLevel, in.View), nil
}

func queryWorkUnitAccess(ctx context.Context, in *pb.GetWorkUnitRequest) (id workunits.ID, accessLevel permissions.AccessLevel, err error) {
	rootInvocationID, workUnitID, err := pbutil.ParseWorkUnitName(in.Name)
	if err != nil {
		return workunits.ID{}, permissions.NoAccess, appstatus.BadRequest(errors.Fmt("name: %w", err))
	}
	id = workunits.ID{
		RootInvocationID: rootinvocations.ID(rootInvocationID),
		WorkUnitID:       workUnitID,
	}

	opts := permissions.QueryWorkUnitAccessOptions{
		Full:                 rdbperms.PermGetWorkUnit,
		Limited:              rdbperms.PermListLimitedWorkUnits,
		UpgradeLimitedToFull: rdbperms.PermGetWorkUnit,
	}
	accessLevel, err = permissions.QueryWorkUnitAccess(ctx, id, opts)
	if err != nil {
		return workunits.ID{}, permissions.NoAccess, err
	}
	return id, accessLevel, nil
}

func validateGetWorkUnitRequest(in *pb.GetWorkUnitRequest) error {
	// Name is already validated in GetWorkUnit.
	if err := pbutil.ValidateWorkUnitView(in.View); err != nil {
		return errors.Fmt("view: %w", err)
	}
	return nil
}
