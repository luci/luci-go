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
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testaggregations"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

// QueryTestAggregations implements pb.ResultDBServer. Refer to
// the service definition in the proto file for details about what this RPC does.
func (s *resultDBServer) QueryTestAggregations(ctx context.Context, req *pb.QueryTestAggregationsRequest) (*pb.QueryTestAggregationsResponse, error) {
	// Use one transaction for the entire RPC so that we work with a
	// consistent snapshot of the system state.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	if err := verifyQueryTestAggregationsAccess(ctx, req); err != nil {
		// Already returns appropriate appstatus-annotated error.
		return nil, err
	}
	if err := validateQueryTestAggregationsRequest(req); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	rootInvID := rootinvocations.MustParseName(req.Parent)
	pageSize := pagination.AdjustPageSize(req.PageSize)

	order, err := testaggregations.ParseOrderBy(req.OrderBy)
	if err != nil {
		// Internal error. This should have been caught in validateQueryTestAggregationsRequest already.
		return nil, err
	}
	// Execute the query.
	q := &testaggregations.SingleLevelQuery{
		RootInvocationID: rootInvID,
		Level:            req.Predicate.AggregationLevel,
		TestPrefixFilter: req.Predicate.TestPrefixFilter,
		PageSize:         pageSize,
		Order:            order,
	}

	aggregations, nextPageToken, err := q.Fetch(ctx, req.PageToken)
	if err != nil {
		return nil, err
	}

	return &pb.QueryTestAggregationsResponse{
		Aggregations:  aggregations,
		NextPageToken: nextPageToken,
	}, nil
}

// verifyQueryTestAggregationsAccess validates the caller has permission
// to list work units, test results and test exonerations in the realm
// of the root invocation.
func verifyQueryTestAggregationsAccess(ctx context.Context, req *pb.QueryTestAggregationsRequest) error {
	rootInvID, err := rootinvocations.ParseName(req.Parent)
	if err != nil {
		return appstatus.Errorf(codes.InvalidArgument, "parent: %s", err)
	}

	realm, err := rootinvocations.ReadRealm(ctx, rootInvID)
	if err != nil {
		return err
	}

	// For now, always require full access to the root invocation.
	// TODO(b/467143483): support limited access with masking of the returned module variants.
	permissions := []realms.Permission{
		rdbperms.PermListWorkUnits,
		rdbperms.PermListTestResults,
		rdbperms.PermListTestExonerations,
	}
	for _, perm := range permissions {
		allowed, err := auth.HasPermission(ctx, perm, realm, nil)
		if err != nil {
			return err
		}
		if !allowed {
			return appstatus.Errorf(codes.PermissionDenied, "caller does not have permission %q in realm %q", perm, realm)
		}
	}
	return nil
}

// validateQueryTestAggregationsRequest
func validateQueryTestAggregationsRequest(req *pb.QueryTestAggregationsRequest) error {
	// Parent field is already validated when verify permissions.

	if err := pagination.ValidatePageSize(req.PageSize); err != nil {
		return errors.Fmt("page_size: %w", err)
	}

	// Validate OrderBy.
	orderBy, err := testaggregations.ParseOrderBy(req.OrderBy)
	if err != nil {
		return errors.Fmt("order_by: %w", err)
	}
	if err := validateQueryTestAggregationsPredicate(req.Predicate); err != nil {
		return errors.Fmt("predicate: %w", err)
	}
	if !orderBy.ByLevelFirst && (req.Predicate.GetAggregationLevel() == pb.AggregationLevel_AGGREGATION_LEVEL_UNSPECIFIED) {
		return errors.New("order_by: must order by id.level first if predicate.aggregation_level is unspecified")
	}
	return nil
}

func validateQueryTestAggregationsPredicate(predicate *pb.TestAggregationPredicate) error {
	// Validate Predicate.
	if predicate == nil {
		return validate.Unspecified()
	}
	// In future, we might allow the AggregationLevel filter to be unset when the backend supports it.
	if err := pbutil.ValidateAggregationLevel(predicate.AggregationLevel); err != nil {
		return errors.Fmt("aggregation_level: %w", err)
	}
	if predicate.AggregationLevel == pb.AggregationLevel_CASE {
		return errors.New("aggregation_level: CASE is not a valid aggregation level; to query test case verdicts, use the QueryTestVerdicts RPC")
	}
	if predicate.TestPrefixFilter != nil {
		if err := pbutil.ValidateTestIdentifierPrefixForQuery(predicate.TestPrefixFilter); err != nil {
			return errors.Fmt("test_prefix_filter: %w", err)
		}
		// A greater value means a finer aggregation. We expect predicate.AggregationLevel >= predicate.TestPrefixFilter.Level.
		if predicate.AggregationLevel < predicate.TestPrefixFilter.Level {
			return errors.Fmt("test_prefix_filter: level: must be equal to, or coarser than, the requested aggregation_level (%s)", predicate.AggregationLevel)
		}
	}
	return nil
}
