// Copyright 2020 The LUCI Authors.
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
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/aip132"
	"go.chromium.org/luci/common/data/aip160"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/graph"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/testvariants"
	"go.chromium.org/luci/resultdb/internal/tracing"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

var (
	StatusFieldPath            = aip132.NewFieldPath("status")
	StatusV2EffectiveFieldPath = aip132.NewFieldPath("status_v2_effective")
)

// determineListAccessLevel determines the list access level the caller has for
// a set of invocations.
// There must not already be a transaction in the given context.
func determineListAccessLevel(ctx context.Context, ids invocations.IDSet) (a testvariants.AccessLevel, err error) {
	if len(ids) == 0 {
		// nothing to check, so the caller's access is unconfirmed
		return testvariants.AccessLevelInvalid, nil
	}

	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/resultdb.determineListAccessLevel")
	defer func() { tracing.End(ts, err) }()

	realms, err := invocations.ReadRealms(span.Single(ctx), ids)
	if err != nil {
		return testvariants.AccessLevelInvalid, err
	}
	// Check for unrestricted access
	hasUnrestricted, _, err := permissions.HasPermissionsInRealms(ctx, realms,
		rdbperms.PermListTestResults, rdbperms.PermListTestExonerations)
	if err != nil {
		return testvariants.AccessLevelInvalid, err
	}
	if hasUnrestricted {
		return testvariants.AccessLevelUnrestricted, nil
	}

	// Check for limited access
	hasLimited, desc, err := permissions.HasPermissionsInRealms(ctx, realms,
		rdbperms.PermListLimitedTestResults, rdbperms.PermListLimitedTestExonerations)
	if err != nil {
		return testvariants.AccessLevelInvalid, err
	}
	if hasLimited {
		return testvariants.AccessLevelLimited, nil
	}

	// Caller does not have access
	return testvariants.AccessLevelInvalid, appstatus.Error(codes.PermissionDenied, desc)
}

// QueryTestVariants implements pb.ResultDBServer.
func (s *resultDBServer) QueryTestVariants(ctx context.Context, in *pb.QueryTestVariantsRequest) (*pb.QueryTestVariantsResponse, error) {
	ids, err := invocations.ParseNames(in.Invocations)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}
	accessLevel, err := determineListAccessLevel(ctx, ids)
	if err != nil {
		return nil, err
	}

	if err := validateQueryTestVariantsRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}
	readMask, err := testvariants.QueryMask(in.GetReadMask())
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	filter, err := aip160.ParseFilter(in.Filter)
	if err != nil {
		return nil, appstatus.BadRequest(errors.Fmt("filter: %w", err))
	}

	orderByClause, err := aip132.ParseOrderBy(in.OrderBy)
	if err != nil {
		// This shouldn't happen, it should already be validated in
		// validateQueryTestVariantsRequest.
		return nil, err
	}

	verdictOrder := testvariants.SortOrderLegacyStatus
	if len(orderByClause) == 1 && orderByClause[0].FieldPath.Equals(StatusV2EffectiveFieldPath) {
		verdictOrder = testvariants.SortOrderStatusV2Effective
	}

	// Query is valid - increment the queryInvocationsCount metric
	queryInvocationsCount.Add(ctx, 1, "QueryTestVariants", len(in.Invocations))

	// Open a transaction.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	// Get the transitive closure.
	invs, err := graph.Reachable(ctx, ids)
	if err != nil {
		return nil, errors.Fmt("resolving reachable invocations: %w", err)
	}

	// Query test variants.
	q := testvariants.Query{
		ReachableInvocations: invs,
		Predicate:            in.Predicate,
		Filter:               filter,
		ResultLimit:          testvariants.AdjustResultLimit(in.ResultLimit),
		PageSize:             pagination.AdjustPageSize(in.PageSize),
		ResponseLimitBytes:   testvariants.DefaultResponseLimitBytes,
		PageToken:            in.PageToken,
		Mask:                 readMask,
		AccessLevel:          accessLevel,
		OrderBy:              verdictOrder,
	}

	var result testvariants.Page
	for len(result.TestVariants) == 0 {
		if result, err = q.Fetch(ctx); err != nil {
			return nil, errors.Fmt("fetching test variants: %w", err)
		}

		if result.NextPageToken == "" || outOfTime(ctx) {
			break
		}
		q.PageToken = result.NextPageToken
	}

	return &pb.QueryTestVariantsResponse{
		TestVariants:  result.TestVariants,
		NextPageToken: result.NextPageToken,
		Sources:       result.DistinctSources,
	}, nil
}

// outOfTime returns true if the context will expire in less than 500ms.
func outOfTime(ctx context.Context) bool {
	dl, ok := ctx.Deadline()
	return ok && clock.Until(ctx, dl) < 500*time.Millisecond
}

// validateQueryTestVariantsRequest returns a non-nil error if req is determined
// to be invalid.
func validateQueryTestVariantsRequest(in *pb.QueryTestVariantsRequest) error {
	if err := validateQueryRequest(in); err != nil {
		return err
	}

	if len(in.Invocations) > 1 {
		return errors.New("invocations: only one invocation is allowed")
	}
	if err := testvariants.ValidateResultLimit(in.ResultLimit); err != nil {
		return errors.Fmt("result_limit: %w", err)
	}

	// We support a limited subset of AIP-132 order by syntax, so as to specify
	// sorting by status or status_v2_effective only.
	orderBy, err := aip132.ParseOrderBy(in.OrderBy)
	if err != nil {
		return errors.Fmt("order_by: %w", err)
	}
	if len(orderBy) > 1 {
		return errors.New("order_by: more than one order by field is not currently supported")
	}
	if len(orderBy) == 1 {
		orderByItem := orderBy[0]
		if !orderByItem.FieldPath.Equals(StatusFieldPath) && !orderByItem.FieldPath.Equals(StatusV2EffectiveFieldPath) {
			return errors.Fmt("order_by: order by field must be one of %q or %q", StatusFieldPath, StatusV2EffectiveFieldPath)
		}
		if orderByItem.Descending {
			return errors.New("order_by: descending order is not supported")
		}
	}
	return nil
}
