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
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/graph"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testresultsv2"
	"go.chromium.org/luci/resultdb/internal/testvariants"
	"go.chromium.org/luci/resultdb/internal/testverdictsv2"
	"go.chromium.org/luci/resultdb/internal/tracing"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

var (
	StatusV2EffectiveFieldPath = aip132.NewFieldPath("status_v2_effective")
)

// determineListAccessLevel determines the list access level the caller has for
// a set of invocations.
func determineListAccessLevel(ctx context.Context, in *pb.QueryTestVariantsRequest) (a testvariants.AccessLevel, err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/resultdb.determineListAccessLevel")
	defer func() { tracing.End(ts, err) }()

	// Perform request validation that is necessary for the permission check.
	var invIDs invocations.IDSet
	if in.Parent != "" {
		return testvariants.AccessLevelInvalid, errors.New("root invocations are not supported")
	} else if len(in.Invocations) > 0 {
		invIDs, err = invocations.ParseNames(in.Invocations)
		if err != nil {
			return testvariants.AccessLevelInvalid, appstatus.BadRequest(errors.Fmt("invocations: %w", err))
		}
	} else {
		return testvariants.AccessLevelInvalid, appstatus.BadRequest(errors.New("must specify either parent or invocations"))
	}
	realmMap := make(map[permissions.NamedResource]string)
	realms, err := invocations.ReadRealms(ctx, invIDs)
	if err != nil {
		return testvariants.AccessLevelInvalid, err
	}
	for id, realm := range realms {
		realmMap[id] = realm
	}

	// Check for unrestricted access
	hasUnrestricted, _, err := permissions.HasPermissionsInRealms(ctx, realmMap,
		rdbperms.PermListTestResults, rdbperms.PermListTestExonerations)
	if err != nil {
		return testvariants.AccessLevelInvalid, err // Internal error.
	}
	if hasUnrestricted {
		return testvariants.AccessLevelUnrestricted, nil
	}

	// Check for limited access
	hasLimited, desc, err := permissions.HasPermissionsInRealms(ctx, realmMap,
		rdbperms.PermListLimitedTestResults, rdbperms.PermListLimitedTestExonerations)
	if err != nil {
		return testvariants.AccessLevelInvalid, err // Internal error.
	}
	if hasLimited {
		return testvariants.AccessLevelLimited, nil
	}

	// Caller does not have access
	return testvariants.AccessLevelInvalid, appstatus.Error(codes.PermissionDenied, desc)
}

// QueryTestVariants implements pb.ResultDBServer.
func (s *resultDBServer) QueryTestVariants(ctx context.Context, in *pb.QueryTestVariantsRequest) (*pb.QueryTestVariantsResponse, error) {
	// Use one transaction for the entire RPC so that we work with a
	// consistent snapshot of the system state. This is important to
	// prevent subtle bugs and TOC-TOU vulnerabilities.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	if err := validateQueryTestVariantsRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}
	readMask, err := testvariants.QueryMask(in.GetReadMask())
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	if in.Parent != "" {
		return queryRootInvocationTestVariants(ctx, in, readMask)
	} else {
		return queryLegacyInvocationTestVariants(ctx, in, readMask)
	}
}

func queryRootInvocationTestVariants(ctx context.Context, in *pb.QueryTestVariantsRequest, readMask *mask.Mask) (*pb.QueryTestVariantsResponse, error) {
	rootInvID := rootinvocations.MustParseName(in.Parent)
	access, err := permissions.VerifyAllWorkUnitsAccess(ctx, rootInvID, permissions.ListVerdictsAccessModel, permissions.LimitedAccess)
	if err != nil {
		// Permission denied.
		return nil, err
	}
	pageToken, err := testverdictsv2.ParsePageToken(in.PageToken)
	if err != nil {
		return nil, appstatus.BadRequest(errors.New("page_token: invalid page token"))
	}
	rootInv, err := rootinvocations.Read(ctx, rootInvID)
	if err != nil {
		// Returns internal error or NotFound appstatus error.
		return nil, err
	}

	var effectiveStatusFilter []pb.VerdictEffectiveStatus
	if in.Predicate != nil {
		// Attempt a best-effort conversion of the status v1-based filters,
		// which are not supported by the new backend. The main client still using
		// these options is Gerrit and it should work OK with the adaptations here.
		switch in.Predicate.Status {
		case pb.TestVariantStatus_UNEXPECTED:
			// FAILED is approximately equivalent to the old UNEXPECTED,
			// except that SKIPPED + FAILED results yield a verdict status_v2 of
			// FAILED whereas in verdict status v1 it produced FLAKY.
			effectiveStatusFilter = []pb.VerdictEffectiveStatus{
				pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_FAILED,
			}
		case pb.TestVariantStatus_UNEXPECTED_MASK:
			// This is approximately equivalent to the old UNEXPECTED_MASK,
			// except that EXECUTION_ERRORED + PASSED would produce a v1
			// verdict of FLAKY whereas it produces a v2 verdict status of PASSED.
			effectiveStatusFilter = []pb.VerdictEffectiveStatus{
				pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_FAILED,
				pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_EXECUTION_ERRORED,
				pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_PRECLUDED,
				pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_FLAKY,
				pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_EXONERATED,
			}
		default:
			// This should not be hit, as validateQueryTestVariantsRequest validates
			// the allowed predicate values.
			return nil, errors.Fmt("attempt to filter to status %v; this code should be unreachable", in.Predicate.Status)
		}
	}

	q := testverdictsv2.Query{
		RootInvocationID: rootInvID,
		Order: testverdictsv2.Ordering{
			// By priority, then test ID.
			ByUIPriority:       true,
			ByStructuredTestID: true,
		},
		// The RPC describes the filter as a filter on the returned verdicts,
		// but as only test_id, variant, variant_hash are supported, it can
		// equally be implemented as a filter on the test results.
		ContainsTestResultFilter: in.Filter,
		Access:                   access,
		EffectiveStatusFilter:    effectiveStatusFilter,
		View:                     pb.TestVerdictView_TEST_VERDICT_VIEW_FULL,
	}
	opts := testverdictsv2.FetchOptions{
		PageSize:           pagination.AdjustPageSize(in.PageSize),
		ResponseLimitBytes: testvariants.DefaultResponseLimitBytes,
		TotalResultLimit:   testresultsv2.MaxTestResultsPageSize,
		VerdictResultLimit: testvariants.AdjustResultLimit(in.ResultLimit),
		VerdictSizeLimit:   testverdictsv2.StandardVerdictSizeLimit,
	}
	startTime := clock.Now(ctx)
	verdicts, nextPageToken, err := q.Fetch(ctx, pageToken, opts)
	if err != nil {
		return nil, err
	}
	duration := clock.Since(ctx, startTime)

	shardingInfo, err := rootinvocations.ReadTestShardingInformationFromShard(ctx, rootinvocations.ShardID{
		RootInvocationID: rootInvID,
		ShardIndex:       2, // Any shard will do.
	})
	if err != nil {
		return nil, err
	}
	queryTestVerdictsLatencyByShardingAlgorithm.Add(ctx, float64(int64(duration)/int64(time.Millisecond)), string(shardingInfo.Algorithm))

	sourcesID := graph.HashSources(rootInv.Sources).String()
	testVariants := make([]*pb.TestVariant, 0, len(verdicts))
	for _, verdict := range verdicts {
		tv := toTestVariant(verdict, sourcesID)
		// For compatibility with the old API, always keep certain fields in tact
		// regardless of whether they are specified in the mask.
		err := testvariants.TrimKeepingPageTokenFields(readMask, tv)
		if err != nil {
			return nil, err
		}
		testVariants = append(testVariants, tv)
	}
	// In root invocations, the source model is simplified and only
	// one set of sources is allowed.
	sources := map[string]*pb.Sources{}
	sources[sourcesID] = rootInv.Sources

	return &pb.QueryTestVariantsResponse{
		TestVariants:  testVariants,
		NextPageToken: nextPageToken.Serialize(),
		Sources:       sources,
	}, nil
}

func toTestVariant(verdict *pb.TestVerdict, sourceID string) *pb.TestVariant {
	tv := &pb.TestVariant{
		TestIdStructured: verdict.TestIdStructured,
		TestId:           verdict.TestId,
		Variant:          verdict.TestIdStructured.ModuleVariant,
		VariantHash:      verdict.TestIdStructured.ModuleVariantHash,
		// As the number of results returned may be limited, and the criteria
		// for allowing exoneration differs slightly between Status V1 and V2,
		// this is only an approximation. However, Status V1 support is not
		// a priority as it is a deprecated concept.
		Status:         variantStatusV1FromResults(verdict.Results, verdict.Exonerations),
		StatusV2:       verdict.Status,
		StatusOverride: verdict.StatusOverride,
		Results:        []*pb.TestResultBundle{},
		Exonerations:   verdict.Exonerations,
		TestMetadata:   verdict.TestMetadata,
		IsMasked:       verdict.IsMasked,
		SourcesId:      sourceID,
		Instruction:    nil, // Not supported via this RPC.
	}
	for _, result := range verdict.Results {
		tv.Results = append(tv.Results, &pb.TestResultBundle{
			Result: result,
		})
	}
	return tv
}

func variantStatusV1FromResults(results []*pb.TestResult, exonerations []*pb.TestExoneration) pb.TestVariantStatus {
	expectedCount := 0
	unexpectedCount := 0
	unexpectedSkipCount := 0
	for _, result := range results {
		if result.Expected {
			expectedCount++
		} else {
			unexpectedCount++
			if result.Status == pb.TestStatus_SKIP {
				unexpectedSkipCount++
			}
		}
	}
	if unexpectedCount == 0 {
		return pb.TestVariantStatus_EXPECTED
	} else { // unexpectedCount > 0
		if len(exonerations) > 0 {
			return pb.TestVariantStatus_EXONERATED
		}
		if expectedCount > 0 {
			return pb.TestVariantStatus_FLAKY
		}
		if unexpectedSkipCount == unexpectedCount {
			return pb.TestVariantStatus_UNEXPECTEDLY_SKIPPED
		}
		return pb.TestVariantStatus_UNEXPECTED
	}
}

func queryLegacyInvocationTestVariants(ctx context.Context, in *pb.QueryTestVariantsRequest, readMask *mask.Mask) (*pb.QueryTestVariantsResponse, error) {
	accessLevel, err := determineListAccessLevel(ctx, in)
	if err != nil {
		return nil, err
	}
	// Legacy invocation.
	filter, err := aip160.ParseFilter(in.Filter)
	if err != nil {
		// This should not happen, as validateQueryTestVariantsRequest enforces
		// that the filter is valid.
		return nil, errors.Fmt("filter: %w", err)
	}

	// validateQueryTestVariantsRequest enforces only one invocation can be in the request.
	requestedLegacyInvID := invocations.MustParseName(in.Invocations[0])

	// Get the transitive closure.
	invs, err := graph.Reachable(ctx, invocations.NewIDSet(requestedLegacyInvID))
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
		OrderBy:              testvariants.SortOrderStatusV2Effective,
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
	if in.Parent != "" {
		_, err := rootinvocations.ParseName(in.Parent)
		if err != nil {
			return errors.Fmt("parent: %w", err)
		}
		if len(in.Invocations) > 0 {
			return errors.New("invocations: must not be specified if parent is specified")
		}
	} else if len(in.Invocations) > 0 {
		_, err := invocations.ParseNames(in.Invocations)
		if err != nil {
			return errors.Fmt("invocations: %w", err)
		}
		if len(in.Invocations) > 1 {
			return errors.New("invocations: only one invocation is allowed")
		}
	} else {
		return errors.New("must specify either parent or invocations")
	}

	if in.Predicate != nil {
		// We want to deprecate this filter as it operates on the v1 status.
		// Clients should migrate to root invocations and QueryTestVerdicts.
		// If clients can't migrate to root invocations yet, we should add
		// support for a filter on the v2 verdict status.
		if in.Predicate.Status != pb.TestVariantStatus_UNEXPECTED && in.Predicate.Status != pb.TestVariantStatus_UNEXPECTED_MASK {
			return errors.Fmt("predicate: status: the filter %v is not supported", in.Predicate.Status)
		}
	}

	if err := testvariants.ValidateResultLimit(in.ResultLimit); err != nil {
		return errors.Fmt("result_limit: %w", err)
	}

	// validate paging
	if err := pagination.ValidatePageSize(in.PageSize); err != nil {
		return errors.Fmt("page_size: %w", err)
	}

	if in.Parent != "" {
		// For root invocations, the filter is implemented using the
		// testresultsv2 filter-generator. Ensure the predicate is valid
		// for that implementation.
		if err := testresultsv2.ValidateFilter(in.Filter); err != nil {
			return errors.Fmt("filter: %w", err)
		}
	} else {
		_, err := aip160.ParseFilter(in.Filter)
		if err != nil {
			return errors.Fmt("filter: %w", err)
		}
	}

	// We support sorting by status_v2_effective only.
	orderBy, err := aip132.ParseOrderBy(in.OrderBy)
	if err != nil {
		return errors.Fmt("order_by: %w", err)
	}
	if len(orderBy) > 1 {
		return errors.New("order_by: more than one order by field is not currently supported")
	}
	if len(orderBy) == 1 {
		orderByItem := orderBy[0]
		if !orderByItem.FieldPath.Equals(StatusV2EffectiveFieldPath) {
			return errors.Fmt("order_by: if set, order by field must be %q", StatusV2EffectiveFieldPath)
		}
		if orderByItem.Descending {
			return errors.New("order_by: descending order is not supported")
		}
	}
	return nil
}
