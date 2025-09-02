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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/masking"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// BatchGetWorkUnits implements pb.RecorderServer.
//
// N.B. Unlike the API with the same name in ResultDB service, this API
// uses update tokens to authorise reads. It is intended for use by
// recorders only, specifically facilitating work unit updates using
// optimistic locking (using aip.dev/154 etags).
func (s *recorderServer) BatchGetWorkUnits(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) (*pb.BatchGetWorkUnitsResponse, error) {
	ids, err := verifyBatchGetWorkUnitAccess(ctx, in)
	if err != nil {
		return nil, err // PermissionDenied or InvalidArgument error.
	}

	if err := validateBatchGetWorkUnitsRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	readMask := workunits.ExcludeExtendedProperties
	if in.View == pb.WorkUnitView_WORK_UNIT_VIEW_FULL {
		readMask = workunits.AllFields
	}

	// Use one transaction for the entire RPC so that we work with a
	// consistent snapshot of the system state. This is important to
	// prevent subtle bugs and TOC-TOU vulnerabilities.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	// Read the work units.
	wus, err := workunits.ReadBatch(ctx, ids, readMask)
	if err != nil {
		return nil, err
	}

	protos := make([]*pb.WorkUnit, len(wus))
	for i, wu := range wus {
		protos[i] = masking.WorkUnit(wu, permissions.FullAccess, in.View)
	}

	return &pb.BatchGetWorkUnitsResponse{
		WorkUnits: protos,
	}, nil
}

func verifyBatchGetWorkUnitAccess(ctx context.Context, in *pb.BatchGetWorkUnitsRequest) ([]workunits.ID, error) {
	// While google.aip.dev/211 prescribes request validation should occur after
	// authorisation, these initial validation checks are important to the integrity
	// of authorisation.
	rootInvID, err := rootinvocations.ParseName(in.Parent)
	if err != nil {
		return nil, appstatus.BadRequest(errors.Fmt("parent: %w", err))
	}

	if err := pbutil.ValidateBatchRequestCount(len(in.Names)); err != nil {
		return nil, appstatus.BadRequest(errors.Fmt("names: %w", err))
	}

	results := make([]workunits.ID, 0, len(in.Names))
	var state string
	for i, name := range in.Names {
		wuID, err := workunits.ParseName(name)
		if err != nil {
			return nil, appstatus.BadRequest(errors.Fmt("names[%d]: %w", i, err))
		}

		if wuID.RootInvocationID != rootInvID {
			return nil, appstatus.BadRequest(errors.Fmt("names[%d]: does not match parent root invocation %q", i, rootinvocations.ID(rootInvID).Name()))
		}

		s := workUnitUpdateTokenState(wuID)
		if i == 0 {
			state = s
		} else if state != s {
			return nil, appstatus.BadRequest(errors.Fmt("names[%d]: work unit %q requires a different update token to names[0]'s %q, but this RPC only accepts one update token", i, wuID.Name(), in.Names[0]))
		}

		results = append(results, wuID)
	}

	// Now check the token.
	token, err := extractUpdateToken(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateWorkUnitUpdateTokenForState(ctx, token, state); err != nil {
		return nil, err // PermissionDenied appstatus error.
	}
	return results, nil
}

func validateBatchGetWorkUnitsRequest(in *pb.BatchGetWorkUnitsRequest) error {
	// Names and Parent are already validated in verifyBatchGetWorkUnitAccess.

	if err := pbutil.ValidateWorkUnitView(in.View); err != nil {
		return errors.Fmt("view: %w", err)
	}
	return nil
}
