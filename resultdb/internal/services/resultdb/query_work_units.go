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

func validateQueryWorkUnitsRequest(req *pb.QueryWorkUnitsRequest) error {
	if err := pbutil.ValidateRootInvocationName(req.Parent); err != nil {
		return errors.Fmt("parent: %w", err)
	}
	if err := pbutil.ValidateWorkUnitView(req.View); err != nil {
		return errors.Fmt("view: %w", err)
	}
	if err := pbutil.ValidateWorkUnitPredicate(req.Predicate); err != nil {
		return errors.Fmt("predicate: %w", err)
	}

	// Validate that ancestors_of refers to the same root invocation as the request.
	// Since ValidateWorkUnitPredicate passed, req.Predicate is non-nil and AncestorsOf is non-empty.
	if req.Predicate.AncestorsOf != "" {
		rootInvID := rootinvocations.MustParseName(req.Parent)
		ancestorID, err := workunits.ParseName(req.Predicate.AncestorsOf)
		if err != nil {
			return errors.Fmt("predicate: ancestors_of: %w", err)
		}
		if ancestorID.RootInvocationID != rootInvID {
			return errors.Fmt("predicate: ancestors_of: work unit %q does not belong to the parent root invocation %q", req.Predicate.AncestorsOf, rootInvID.Name())
		}
	}

	return nil
}

func (s *resultDBServer) QueryWorkUnits(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error) {
	if err := validateQueryWorkUnitsRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	rootInvID := rootinvocations.MustParseName(in.Parent)

	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	// Verify the caller has at least limited permission to list work units
	// in the specified root invocation.
	if err := permissions.VerifyRootInvocation(ctx, rootInvID, rdbperms.PermListLimitedWorkUnits); err != nil {
		return nil, err
	}

	// We read all fields initially; they will be masked later based on permissions and view.
	q := workunits.Query{
		RootInvocationID: rootInvID,
		Predicate:        in.Predicate,
		Mask:             workunits.AllFields,
	}
	wus, err := q.Query(ctx)
	if err != nil {
		return nil, err
	}

	if len(wus) == 0 {
		return &pb.QueryWorkUnitsResponse{}, nil
	}

	ids := make([]workunits.ID, len(wus))
	for i, row := range wus {
		ids[i] = row.ID
	}
	accessLevels, err := permissions.QueryWorkUnitsAccess(ctx, ids, permissions.GetWorkUnitsAccessModel)
	if err != nil {
		return nil, err
	}

	// Apply masking based on access levels and the requested view.
	resWUs := make([]*pb.WorkUnit, 0, len(wus))
	for i, row := range wus {
		level := accessLevels[i]
		if level == permissions.NoAccess {
			return nil, errors.New("logic error: user had at least limited access to all work units at start of request but no longer has access")
		}
		resWUs = append(resWUs, masking.WorkUnit(row, level, in.View))
	}

	return &pb.QueryWorkUnitsResponse{
		WorkUnits: resWUs,
	}, nil
}
