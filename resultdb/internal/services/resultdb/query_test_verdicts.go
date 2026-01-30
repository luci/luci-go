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

// queryTestVerdictsResponseLimitBytes is the soft limit on the number of bytes
// that should be returned by a Test Verdicts query.
const queryTestVerdictsResponseLimitBytes = 20 * 1000 * 1000 // 20 MB

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
		return nil, errors.Fmt("order_by: %w", err)
	}
	pageToken, err := testverdictsv2.ParsePageToken(req.PageToken)
	if err != nil {
		// Internal error. This should have been caught in validateQueryTestVerdictsRequest already.
		return nil, errors.Fmt("page_token: %w", err)
	}

	view := req.View
	if view == pb.TestVerdictView_TEST_VERDICT_VIEW_UNSPECIFIED {
		// Default to basic view if the user has not specified anything.
		view = pb.TestVerdictView_TEST_VERDICT_VIEW_BASIC
	}

	q := &testverdictsv2.Query{
		RootInvocationID:         rootInvID,
		Order:                    order,
		ContainsTestResultFilter: req.Predicate.GetContainsTestResultFilter(),
		TestPrefixFilter:         req.Predicate.GetTestPrefixFilter(),
		Access:                   access,
		EffectiveStatusFilter:    req.Predicate.GetEffectiveVerdictStatus(),
		View:                     view,
	}

	fetchOpts := testverdictsv2.FetchOptions{
		PageSize:           pageSize,
		ResponseLimitBytes: queryTestVerdictsResponseLimitBytes,
		TotalResultLimit:   testresultsv2.MaxTestResultsPageSize,
		VerdictResultLimit: testverdictsv2.StandardVerdictResultLimit,
		VerdictSizeLimit:   testverdictsv2.StandardVerdictSizeLimit,
	}

	verdicts, nextPageToken, err := q.Fetch(ctx, pageToken, fetchOpts)
	if err != nil {
		return nil, err
	}

	return &pb.QueryTestVerdictsResponse{
		TestVerdicts:  verdicts,
		NextPageToken: nextPageToken.Serialize(),
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
	// req.Parent is already validated by verifyQueryTestVerdictsAccess.

	if req.Predicate != nil {
		if err := validateQueryTestVerdictsPredicate(req.Predicate); err != nil {
			return errors.Fmt("predicate: %w", err)
		}
	}

	if _, err := testverdictsv2.ParseOrderBy(req.OrderBy); err != nil {
		return errors.Fmt("order_by: %w", err)
	}

	// Per https://google.aip.dev/157, view must be an optional field.
	if req.View != pb.TestVerdictView_TEST_VERDICT_VIEW_UNSPECIFIED {
		if err := pbutil.ValidateTestVerdictView(req.View); err != nil {
			return errors.Fmt("view: %w", err)
		}
	}

	if err := pagination.ValidatePageSize(req.PageSize); err != nil {
		return errors.Fmt("page_size: %w", err)
	}

	if _, err := testverdictsv2.ParsePageToken(req.PageToken); err != nil {
		return errors.Fmt("page_token: invalid page token: %w", err)
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

	if len(predicate.EffectiveVerdictStatus) > 0 {
		if err := testverdictsv2.ValidateFilter(predicate.EffectiveVerdictStatus); err != nil {
			return errors.Fmt("effective_verdict_status: %w", err)
		}
	}
	return nil
}
