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
	EffectiveStatusFilter []pb.TestVerdictPredicate_VerdictEffectiveStatus
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
	if strings.TrimSpace(q.ContainsTestResultFilter) == "" && len(q.EffectiveStatusFilter) == 0 && isOrderSuitableForIteratorQuery {
		// Use the iterator query. It is fast and cheap.
		logging.Debugf(ctx, "Using iterator query.")
		return q.fetchUsingIteratorQuery(ctx, pageToken, opts)
	}

	// The query is a complex query. We need to use QuerySummaries
	// to identify the verdicts to return, and if the view is FULL,
	// we then need to fetch the details using a follow-up query.
	switch q.View {
	case pb.TestVerdictView_TEST_VERDICT_VIEW_BASIC:
		logging.Debugf(ctx, "Using summary query.")
		summaryQuery := q.toSummaryQuery()
		return summaryQuery.Fetch(ctx, pageToken, opts)
	case pb.TestVerdictView_TEST_VERDICT_VIEW_FULL:
		logging.Debugf(ctx, "Using summary then iterator query.")
		return q.fetchSummariesThenDetails(ctx, pageToken, opts)
	default:
		return nil, PageToken{}, errors.Fmt("unknown view %q", q.View)
	}
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
	var predicate func(*TestVerdict) bool
	order := testresultsv2.OrderingByPrimaryKey
	if q.Order.ByStructuredTestID {
		order = testresultsv2.OrderingByTestID
	}
	if q.Order.ByUIPriority {
		if pageToken.UIPriority != MaxUIPriority {
			return nil, PageToken{}, errors.New("page token must be in the region of the response where only passing & skipped verdicts are being returned")
		}
		predicate = func(tv *TestVerdict) bool {
			return tv.Status == pb.TestVerdict_PASSED || tv.Status == pb.TestVerdict_SKIPPED
		}
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
		Predicate:    predicate,
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
func (q *Query) fetchSummariesThenDetails(ctx context.Context, pageToken PageToken, opts FetchOptions) ([]*pb.TestVerdict, PageToken, error) {
	var ids []testresultsv2.VerdictID
	var summaries []*TestVerdictSummary

	startTime := clock.Now(ctx)
	// Fetch up to q.PageSize verdict summaries.
	_, err := q.toSummaryQuery().Run(ctx, pageToken, opts.PageSize, func(tv *TestVerdictSummary) (bool, error) {
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
