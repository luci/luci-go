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
	"go.chromium.org/luci/common/validate"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testresultsv2"
	"go.chromium.org/luci/resultdb/internal/testverdictsv2"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// QueryTestVerdicts implements pb.ResultDBServer.
func (s *resultDBServer) QueryTestVerdicts(ctx context.Context, req *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
	// Use one transaction for the entire RPC so that we work with a
	// consistent snapshot of the system state.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	// AIP-211: Perform authorization checks before validating the request.
	access, err := verifyQueryTestVerdictsAccess(ctx, req)
	if err != nil {
		// Already returns appropriate appstatus-annotated error.
		return nil, err
	}

	if err := validateQueryTestVerdictsRequest(req); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	rootInvID := rootinvocations.MustParseName(req.Parent)
	pageSize := pagination.AdjustPageSize(req.PageSize)

	order, err := testverdictsv2.ParseOrderBy(req.OrderBy)
	if err != nil {
		// Internal error. This should have been caught in validateQueryTestVerdictsRequest already.
		return nil, err
	}

	q := &testverdictsv2.Query{
		RootInvocationID:         rootInvID,
		PageSize:                 pageSize,
		ResultLimit:              testverdictsv2.DefaultResultLimit(req.ResultLimit),
		Order:                    order,
		ContainsTestResultFilter: req.Predicate.GetContainsTestResultFilter(),
		TestPrefixFilter:         req.Predicate.GetTestPrefixFilter(),
		Access:                   access,
		Filter:                   req.Predicate.GetFilter(),
	}

	verdicts, nextPageToken, err := q.Fetch(ctx, req.PageToken)
	if err != nil {
		return nil, err
	}

	return &pb.QueryTestVerdictsResponse{
		TestVerdicts:  verdicts,
		NextPageToken: nextPageToken,
	}, nil
}

func verifyQueryTestVerdictsAccess(ctx context.Context, req *pb.QueryTestVerdictsRequest) (permissions.RootInvocationAccess, error) {
	rootInvID, err := rootinvocations.ParseName(req.Parent)
	if err != nil {
		return permissions.RootInvocationAccess{}, appstatus.Errorf(codes.InvalidArgument, "parent: %s", err)
	}
	result, err := permissions.VerifyAllWorkUnitsAccess(ctx, rootInvID, permissions.ListVerdictsAccessModel, permissions.LimitedAccess)
	if err != nil {
		return permissions.RootInvocationAccess{}, err
	}
	return result, nil
}

func validateQueryTestVerdictsRequest(req *pb.QueryTestVerdictsRequest) error {
	if err := pagination.ValidatePageSize(req.PageSize); err != nil {
		return errors.Fmt("page_size: %w", err)
	}

	if req.ResultLimit != 0 {
		if err := testverdictsv2.ValidateResultLimit(req.ResultLimit); err != nil {
			return errors.Fmt("result_limit: %w", err)
		}
	}

	if _, err := testverdictsv2.ParseOrderBy(req.OrderBy); err != nil {
		return errors.Fmt("order_by: %w", err)
	}

	if req.Predicate != nil {
		if err := validateQueryTestVerdictsPredicate(req.Predicate); err != nil {
			return errors.Fmt("predicate: %w", err)
		}
	}
	return nil
}

func validateQueryTestVerdictsPredicate(predicate *pb.TestVerdictPredicate) error {
	if predicate == nil {
		return validate.Unspecified()
	}

	if predicate.TestPrefixFilter != nil {
		if err := pbutil.ValidateTestIdentifierPrefixForQuery(predicate.TestPrefixFilter); err != nil {
			return errors.Fmt("test_prefix_filter: %w", err)
		}
	}

	if predicate.ContainsTestResultFilter != "" {
		if err := testresultsv2.ValidateFilter(predicate.ContainsTestResultFilter); err != nil {
			return errors.Fmt("contains_test_result_filter: %w", err)
		}
	}

	if predicate.Filter != "" {
		if err := testverdictsv2.ValidateFilter(predicate.Filter); err != nil {
			return errors.Fmt("filter: %w", err)
		}
	}
	return nil
}
