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

	"go.chromium.org/luci/resultdb/internal/masking"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func (s *resultDBServer) BatchGetWorkUnits(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error) {
	// Use one transaction for the entire RPC so that we work with a
	// consistent snapshot of the system state. This is important to
	// prevent subtle bugs and TOC-TOU vulnerabilities.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	ids, accessLevels, err := queryBatchGetWorkUnitAccess(ctx, in)
	if err != nil {
		return nil, err
	}

	anyHasFullAccess := false
	for i, level := range accessLevels {
		if level == permissions.NoAccess {
			return nil, appstatus.Errorf(codes.PermissionDenied, `caller does not have permission %s (or %s) on root invocation %q`,
				rdbperms.PermGetWorkUnit, rdbperms.PermListLimitedWorkUnits, ids[i].RootInvocationID.Name())
		}
		if level == permissions.FullAccess {
			anyHasFullAccess = true
		}
	}

	if err := validateBatchGetWorkUnitsRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	readMask := workunits.ExcludeExtendedProperties
	if in.View == pb.WorkUnitView_WORK_UNIT_VIEW_FULL && anyHasFullAccess {
		readMask = workunits.AllFields
	}

	// Read the work units.
	wus, err := workunits.ReadBatch(ctx, ids, readMask)
	if err != nil {
		return nil, err
	}

	protos := make([]*pb.WorkUnit, len(wus))
	for i, wu := range wus {
		protos[i] = masking.WorkUnit(wu, accessLevels[i], in.View)
	}

	return &pb.BatchGetWorkUnitsResponse{
		WorkUnits: protos,
	}, nil
}

func queryBatchGetWorkUnitAccess(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (ids []workunits.ID, accessLevels []permissions.AccessLevel, err error) {
	// While google.aip.dev/211 prescribes request validation should occur after
	// authorisation, these initial validation checks are important to the integrity
	// of authorisation.
	rootInvID, err := rootinvocations.ParseName(in.Parent)
	if err != nil {
		return nil, nil, appstatus.BadRequest(errors.Fmt("parent: %w", err))
	}

	if err := pbutil.ValidateBatchRequestCount(len(in.Names)); err != nil {
		return nil, nil, appstatus.BadRequest(errors.Fmt("names: %w", err))
	}

	ids = make([]workunits.ID, 0, len(in.Names))
	for i, name := range in.Names {
		wuID, err := workunits.ParseName(name)
		if err != nil {
			return nil, nil, appstatus.BadRequest(errors.Fmt("names[%d]: %w", i, err))
		}

		if wuID.RootInvocationID != rootInvID {
			return nil, nil, appstatus.BadRequest(errors.Fmt("names[%d]: does not match parent root invocation %q", i, rootinvocations.ID(rootInvID).Name()))
		}
		ids = append(ids, wuID)
	}

	// Check permissions.
	opts := permissions.QueryWorkUnitAccessOptions{
		Full:                 rdbperms.PermGetWorkUnit,
		Limited:              rdbperms.PermListLimitedWorkUnits,
		UpgradeLimitedToFull: rdbperms.PermGetWorkUnit,
	}
	accessLevels, err = permissions.QueryWorkUnitsAccess(ctx, ids, opts)
	if err != nil {
		// Returns NotFound appstatus error if one of the work units was not found,
		// or an internal error.
		return nil, nil, err
	}

	return ids, accessLevels, nil
}

func validateBatchGetWorkUnitsRequest(in *pb.BatchGetWorkUnitsRequest) error {
	// Names and Parent are already validated in queryBatchGetWorkUnitAccess.

	if err := pbutil.ValidateWorkUnitView(in.View); err != nil {
		return errors.Fmt("view: %w", err)
	}
	return nil
}
