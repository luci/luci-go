// Copyright 2026 The LUCI Authors.
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

package testverdictsv2

import (
	"context"
	"fmt"
	"strings"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testresultsv2"
	"go.chromium.org/luci/resultdb/internal/tracing"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// Query represents a query for test verdicts.
type Query struct {
	// The root invocation.
	RootInvocationID rootinvocations.ID
	// Whether to use UI sort order, i.e. by ui_priority first, instead of by test ID.
	// This incurs a performance penalty, as results are not returned in table order.
	Order Ordering
	// An AIP-160 filter expression for test results to filter by. Optional.
	// See filtering implementation in the `testresultsv2` package for details.
	// If this is set, a verdict will only be returned if this filter expression matches
	// at least one test result in the verdict.
	ContainsTestResultFilter string
	// The prefix of test identifiers to filter by. Optional.
	TestPrefixFilter *pb.TestIdentifierPrefix
	// The access the caller has to the root invocation.
	Access permissions.RootInvocationAccess
	// The filter on the effective status of test verdicts returned. Optional.
	EffectiveStatusFilter []pb.VerdictEffectiveStatus
	// The view to return.
	View pb.TestVerdictView
}

type FetchOptions struct {
	// The number of test verdicts to return per page.
	PageSize int
	// The hard limit on the number of bytes that should be returned by a Test Verdicts query.
	// Row size is estimated as proto.Size() + protoJSONOverheadBytes bytes (to allow for
	// alternative encodings which higher fixed overheads, e.g. protojson). If this is zero,
	// no limit is applied.
	// This should be set to a value equal or greater than VerdictSizeLimit.
	ResponseLimitBytes int
	// A soft limit on the the maximum number of results the underlying query should retrieve.
	// This is used to manage timeout risk.
	TotalResultLimit int
	// The limit on the number of test results and test exonerations to return per verdict.
	// The limit is applied independently to test results and exonerations.
	VerdictResultLimit int
	// The size limit, in bytes, of each test verdict.
	// This limit is only ever used to reduce the number of test results and exonerations
	// returned, and not any other properties.
	VerdictSizeLimit int
}

// Validate validates the fetch options.
func (opts FetchOptions) Validate() error {
	if opts.PageSize <= 0 {
		return errors.New("page size must be positive")
	}
	if opts.ResponseLimitBytes < 0 {
		return errors.New("if set, response limit bytes must be positive")
	}
	if opts.TotalResultLimit < 0 {
		return errors.New("if set, total result limit must be be positive")
	}
	if opts.VerdictResultLimit <= 0 {
		return errors.New("verdict result limit must be positive")
	}
	if opts.VerdictSizeLimit <= 0 {
		return errors.New("verdict size limit must be positive")
	}
	return nil
}

// Fetch fetches a page of test verdicts.
func (q *Query) Fetch(ctx context.Context, pageToken PageToken, opts FetchOptions) (verdicts []*pb.TestVerdict, nextPageToken PageToken, retErr error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/testverdictsv2.Query.Fetch")
	defer func() { tracing.End(ts, retErr) }()

	ctx = logging.SetFields(ctx, logging.Fields{
		"RootInvocationID": q.RootInvocationID,
		"PageSize":         opts.PageSize,
	})

	if err := opts.Validate(); err != nil {
		return nil, PageToken{}, errors.Fmt("opts: %w", err)
	}
	if q.Access.Level == permissions.NoAccess {
		return nil, PageToken{}, errors.New("no access to root invocation")
	}

	// The result order is suitable for an iterator query if:
	// - the query is not sorted by UI priority, or
	// - it is sorted by UI priority, but the UI priority is no
	//   longer a factor in the result order (we are in the region
	//   of the result set where we are paging over passing
	//   and skipped verdicts).
	isOrderSuitableForIteratorQuery := !q.Order.ByUIPriority || (q.Order.ByUIPriority && pageToken.UIPriority == MaxUIPriority)

	// The iterator query is suitable if:
	// - the query order is suitable,
	// - the query does not have a contains test result filter (such
	//   filters can be very selective and the iterator query does not
	//   perform well with selective queries that cannot be pushed down
	//   to Spanner)
	// - the query does not have a selective status filter (if it allows
	//   passed verdicts, we consider it to be not very selective).
	if strings.TrimSpace(q.ContainsTestResultFilter) == "" &&
		matchesStatus(q.EffectiveStatusFilter, pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_PASSED) &&
		isOrderSuitableForIteratorQuery {
		// Use the iterator query. It is fast and cheap.
		logging.Debugf(ctx, "Using iterator query.")
		return q.fetchUsingIteratorQuery(ctx, pageToken, opts)
	}

	// The query is a complex query. We need to use QuerySummaries
	// to identify the verdicts to return.
	summaryQuery := q.toSummaryQuery()

	// The summaries query is faster if it is only returning priority verdicts
	// (failed, execution errored, flaky, precluded, exonerated) as it can use an index.
	//
	// We should filter to priority verdicts only for this page of results if:
	// - results are ordered in priority order, AND
	// - we are in the region of the result set where we are paging over priority
	//   verdicts, AND
	// - the statuses filtered to (if any) include BOTH priority and non-priority
	//   (i.e. passed, skipped) verdicts. We need BOTH because:
	//   - If the user filtered only to non-priority verdicts, then it does not make sense
	//     to filter as we will produce an empty page for no reason.
	//   - If there are only priority verdicts, then the query will already use the index
	//     and again we do not need to do anything.
	filterToPriorityVerdictsOnly := q.Order.ByUIPriority && pageToken.UIPriority < MaxUIPriority &&
		filterHasBothPriorityAndNonPriorityVerdicts(q.EffectiveStatusFilter)
	if filterToPriorityVerdictsOnly {
		// Filter the current page to priority verdicts only.
		// This does not affect the result order, but might shrink the page size as
		// we are excluding passed and skipped verdicts.
		summaryQuery.EffectiveStatusFilter = intersectStatuses(q.EffectiveStatusFilter, PriorityStatuses)
	}

	// As QuerySummaries only returns summaries, if the view is FULL,
	// we then need to fetch the details using a follow-up query.
	switch q.View {
	case pb.TestVerdictView_TEST_VERDICT_VIEW_BASIC:
		logging.Debugf(ctx, "Using summary query.")
		verdicts, nextPageToken, retErr = summaryQuery.Fetch(ctx, pageToken, opts)
	case pb.TestVerdictView_TEST_VERDICT_VIEW_FULL:
		logging.Debugf(ctx, "Using summary then iterator query.")
		verdicts, nextPageToken, retErr = fetchSummariesThenDetails(ctx, summaryQuery, pageToken, opts)
	default:
		return nil, PageToken{}, errors.Fmt("unknown view %q", q.View)
	}

	if filterToPriorityVerdictsOnly && nextPageToken == (PageToken{}) {
		// We excluded the passed and skipped verdicts from the query, so we need to
		// intercept the empty page token and instead connect it to the first page of
		// non-priority verdicts.
		nextPageToken = PageToken{
			// For passed and skipped verdicts, which have the lowest priority (highest priority number).
			UIPriority: MaxUIPriority,
			// The empty verdict key appear before all actual verdicts.
			ID: testresultsv2.VerdictID{},
		}
	}

	return verdicts, nextPageToken, retErr
}

func (q *Query) toSummaryQuery() *QuerySummaries {
	return &QuerySummaries{
		RootInvocationID:         q.RootInvocationID,
		Order:                    q.Order,
		ContainsTestResultFilter: q.ContainsTestResultFilter,
		TestPrefixFilter:         q.TestPrefixFilter,
		Access:                   q.Access,
		EffectiveStatusFilter:    q.EffectiveStatusFilter,
	}
}

func (q *Query) fetchUsingIteratorQuery(ctx context.Context, pageToken PageToken, opts FetchOptions) ([]*pb.TestVerdict, PageToken, error) {
	order := testresultsv2.OrderingByPrimaryKey
	if q.Order.ByStructuredTestID {
		order = testresultsv2.OrderingByTestID
	}
	statusFilter := q.EffectiveStatusFilter
	if q.Order.ByUIPriority {
		if pageToken.UIPriority != MaxUIPriority {
			return nil, PageToken{}, errors.New("page token must be in the region of the response where only passing & skipped verdicts are being returned")
		}
		// Filter to skipped and passed verdicts only.
		statusFilter = intersectStatuses(statusFilter, NonPriorityStatuses)
	}

	// For each verdict summary, query the details.
	detailsQuery := IteratorQuery{
		RootInvocationID: q.RootInvocationID,
		Order:            order,
		TestPrefixFilter: q.TestPrefixFilter,
		Access:           q.Access,
		View:             q.View,
	}

	fetchOpts := IteratorFetchOptions{
		FetchOptions: opts,
		Predicate:    filterAsPredicate(statusFilter),
	}

	verdicts, nextPageToken, err := detailsQuery.Fetch(ctx, pageToken, fetchOpts)
	if err != nil {
		return nil, PageToken{}, err
	}

	if q.Order.ByUIPriority && nextPageToken != (PageToken{}) {
		// The page token returned by the query may be based on verdicts not included in the response.
		// This is a consequence of the iterator advancing the page token even for verdicts it may not
		// include in the response. To avoid this jumping us back into an earlier part of the query,
		// we need to pin the UI priority in the response token to MaxUIPriority.
		nextPageToken.UIPriority = MaxUIPriority
	}
	return verdicts, nextPageToken, nil
}

// fetchSummariesThenDetails fetches a page of test verdicts with view of TEST_VERDICT_VIEW_FULL.
func fetchSummariesThenDetails(ctx context.Context, q *QuerySummaries, pageToken PageToken, opts FetchOptions) ([]*pb.TestVerdict, PageToken, error) {
	var ids []testresultsv2.VerdictID
	var summaries []*TestVerdictSummary

	startTime := clock.Now(ctx)
	// Fetch up to q.PageSize verdict summaries.
	_, err := q.Run(ctx, pageToken, opts.PageSize, func(tv *TestVerdictSummary) (bool, error) {
		ids = append(ids, tv.ID)
		summaries = append(summaries, tv)
		return true, nil
	})
	if err != nil {
		return nil, PageToken{}, err
	}
	logging.Debugf(ctx, "Fetched summaries in %v.", clock.Since(ctx, startTime))

	// For each verdict summary, query the details.
	detailsQuery := IteratorQuery{
		VerdictIDs: ids,
		Access:     q.Access,
		View:       pb.TestVerdictView_TEST_VERDICT_VIEW_FULL,
	}

	// If querying all details for all the verdicts would retrieve too
	// many test results, cut down the size of this page. Cutting down
	// here is faster than stopping early in the results iterator, as it
	// makes the Spanner query simpler.
	if opts.TotalResultLimit != 0 {
		var totalResultCount int64
		for i, summary := range summaries {
			if i == 0 {
				// At minimum, we must always retrieve the first verdict, however
				// large it is, otherwise this query will never advance the page
				// token.
				totalResultCount += summary.ResultCount
				continue
			}
			// Check the verdict fits within the result limit.
			// The underlying iterator always needs on result extra to identify
			// the end of the verdict, so account for needing one test result extra.
			if (totalResultCount + summary.ResultCount + 1) <= int64(opts.TotalResultLimit) {
				// This verdict fits within the limit.
				totalResultCount += summary.ResultCount
				continue
			}
			// Cut the page here.
			// As we are cutting pages based on objective criteria, the RPC remains
			// deterministic.
			detailsQuery.VerdictIDs = detailsQuery.VerdictIDs[:i]
			logging.Debugf(ctx, "Cut fetched page size to %v (%v results).", len(detailsQuery.VerdictIDs), totalResultCount)
			break
		}
	}

	startTime = clock.Now(ctx)
	fetchOpts := IteratorFetchOptions{
		FetchOptions: opts,
		Predicate:    nil,
	}
	// We already applied a result limit above, don't apply it again.
	fetchOpts.TotalResultLimit = 0

	verdicts, _, err := detailsQuery.Fetch(ctx, PageToken{}, fetchOpts)
	if err != nil {
		// There was an error when retrieving the verdicts.
		return nil, PageToken{}, err
	}
	logging.Debugf(ctx, "Fetched details in %v.", clock.Since(ctx, startTime))

	// The first stage returned fewer verdicts than the page size,
	// and we retrieved all of them (didn't truncate the page size
	// due to test result or response size limits).
	if len(ids) < opts.PageSize && len(verdicts) == len(ids) {
		// There are no more verdicts to query. The page token is empty to signal end of iteration.
		return verdicts, PageToken{}, nil
	}

	// We either:
	// - Have a full page of verdicts, or
	// - The second stage didn't retrieve all verdicts (e.g. due to
	//   hitting response size limits.)
	// Continue paginating from the last result we are returning.
	var nextPageToken PageToken
	if len(verdicts) > 0 {
		lastSummary := summaries[len(verdicts)-1]
		nextPageToken = makePageTokenForSummary(lastSummary)
	} else {
		return nil, PageToken{}, errors.New("fetch is returning zero verdicts. This should never happen, as progress will never be made.")
	}
	return verdicts, nextPageToken, nil
}

// filterHasBothPriorityAndNonPriorityVerdicts returns true if the given filter contains both:
// - priority (FAILED, FLAKY, EXECUTION_ERRORED, PRECLUDED, EXONERATED) and
// - non-priority (PASSED, SKIPPED) verdicts.
func filterHasBothPriorityAndNonPriorityVerdicts(filter []pb.VerdictEffectiveStatus) bool {
	if len(filter) == 0 {
		// The empty filter is a special case, as it matches all verdicts.
		return true
	}
	hasPriorityVerdicts := false
	hasNonPriorityVerdicts := false
	for _, s := range filter {
		switch s {
		case pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_FAILED,
			pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_FLAKY,
			pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_EXECUTION_ERRORED,
			pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_PRECLUDED,
			pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_EXONERATED:
			hasPriorityVerdicts = true
		case pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_PASSED,
			pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_SKIPPED:
			hasNonPriorityVerdicts = true
		}
	}
	return hasPriorityVerdicts && hasNonPriorityVerdicts
}

// matchesStatus returns true if the given filter matches the given status.
func matchesStatus(filter []pb.VerdictEffectiveStatus, status pb.VerdictEffectiveStatus) bool {
	if len(filter) == 0 {
		// The empty filter matches all statuses.
		return true
	}
	for _, s := range filter {
		if s == status {
			return true
		}
	}
	return false
}

// intersectStatuses returns the intersection of the given filter and the priority verdict statuses.
func intersectStatuses(filter, statusList []pb.VerdictEffectiveStatus) []pb.VerdictEffectiveStatus {
	if len(filter) == 0 {
		// The empty filter matches all statuses. Return the RHS set.
		return statusList
	}
	// Compute the intersection of the LHS and RHS sets.
	var result []pb.VerdictEffectiveStatus
	for _, s := range filter {
		for _, status := range statusList {
			if s == status {
				result = append(result, s)
				break
			}
		}
	}
	return result
}

// filterAsPredicate returns a predicate function that returns true if the given verdict matches the filter.
func filterAsPredicate(filter []pb.VerdictEffectiveStatus) func(*TestVerdict) bool {
	return func(tv *TestVerdict) bool {
		var effectiveStatus pb.VerdictEffectiveStatus
		if tv.StatusOverride == pb.TestVerdict_EXONERATED {
			effectiveStatus = pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_EXONERATED
		} else {
			switch tv.Status {
			case pb.TestVerdict_FAILED:
				effectiveStatus = pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_FAILED
			case pb.TestVerdict_FLAKY:
				effectiveStatus = pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_FLAKY
			case pb.TestVerdict_EXECUTION_ERRORED:
				effectiveStatus = pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_EXECUTION_ERRORED
			case pb.TestVerdict_PRECLUDED:
				effectiveStatus = pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_PRECLUDED
			case pb.TestVerdict_SKIPPED:
				effectiveStatus = pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_SKIPPED
			case pb.TestVerdict_PASSED:
				effectiveStatus = pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_PASSED
			default:
				panic(fmt.Errorf("unknown status %v", tv.Status))
			}
		}
		return matchesStatus(filter, effectiveStatus)
	}
}
