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
	"text/template"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testresultsv2"
	"go.chromium.org/luci/resultdb/internal/tracing"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// MaxUIPriority is the maximum value of UIPriority field.
const MaxUIPriority = 100

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
	// An AIP-160 filter on the test verdicts returned. Optional.
	// See filter.go for details.
	Filter string
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
func (q *Query) Fetch(ctx context.Context, pageToken PageToken, opts FetchOptions) ([]*pb.TestVerdict, PageToken, error) {
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

	switch q.View {
	case pb.TestVerdictView_TEST_VERDICT_VIEW_BASIC:
		return q.fetchSimpleView(ctx, pageToken, opts)
	case pb.TestVerdictView_TEST_VERDICT_VIEW_FULL:
		return q.fetchDetailedView(ctx, pageToken, opts)
	default:
		return nil, PageToken{}, errors.Fmt("unknown view %q", q.View)
	}
}

// fetchSimpleView fetches a page of test verdicts with a view of TEST_VERDICT_VIEW_BASIC.
func (q *Query) fetchSimpleView(ctx context.Context, pageToken PageToken, opts FetchOptions) (verdicts []*pb.TestVerdict, nextPageToken PageToken, retErr error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/testverdictsv2.Query.fetchSimpleView")
	defer func() { tracing.End(ts, retErr) }()

	var totalSize int
	nextPageToken, err := q.run(ctx, pageToken, opts.PageSize, func(tv *TestVerdictSummary) (bool, error) {
		p := tv.ToProto()
		if opts.ResponseLimitBytes > 0 {
			// Estimate row size using formula documented on q.ResponseLimitBytes.
			size := proto.Size(p) + 1000
			if totalSize+size > opts.ResponseLimitBytes {
				if len(verdicts) == 0 {
					return false, errors.Fmt("a single verdict (%v bytes) was larger than the total response limit (%v bytes)", size, opts.ResponseLimitBytes)
				}
				// We would exceed the hard limit. Stop iteration early.
				return false, iterator.Done
			}
			totalSize += size
		}
		verdicts = append(verdicts, p)
		return true, nil
	})
	if err != nil {
		return nil, PageToken{}, err
	}
	return verdicts, nextPageToken, nil
}

// fetchDetailedView fetches a page of test verdicts with view of TEST_VERDICT_VIEW_FULL.
func (q *Query) fetchDetailedView(ctx context.Context, pageToken PageToken, opts FetchOptions) (verdicts []*pb.TestVerdict, nextPageToken PageToken, retErr error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/testverdictsv2.Query.fetchDetailedView")
	defer func() { tracing.End(ts, retErr) }()

	var ids []testresultsv2.VerdictID
	var summaries []*TestVerdictSummary

	startTime := clock.Now(ctx)
	// Fetch up to q.PageSize verdict summaries.
	_, err := q.run(ctx, pageToken, opts.PageSize, func(tv *TestVerdictSummary) (bool, error) {
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

	verdicts = make([]*pb.TestVerdict, 0, opts.PageSize)
	totalSize := 0

	startTime = clock.Now(ctx)
	it := detailsQuery.List(ctx, PageToken{}, opts.PageSize)
	err = it.Do(ctx, func(tv *TestVerdict) error {
		// Verdicts come in the order requested.
		verdict := tv.ToProto(opts.VerdictResultLimit, opts.VerdictSizeLimit)
		if opts.ResponseLimitBytes > 0 {
			// Estimate row size using formula documented on q.ResponseLimitBytes.
			size := proto.Size(verdict) + protoJSONOverheadBytes
			if totalSize+size > opts.ResponseLimitBytes {
				if len(verdicts) == 0 {
					return errors.Fmt("a single verdict (%v bytes) was larger than the total response limit (%v bytes)", size, opts.ResponseLimitBytes)
				}
				// We would exceed the hard limit. Stop iteration early.
				return iterator.Done
			}
			totalSize += size
		}
		verdicts = append(verdicts, verdict)
		return nil
	})
	if err != nil && !errors.Is(err, iterator.Done) {
		// There was an error when retrieving the verdicts.
		return nil, PageToken{}, err
	}
	logging.Debugf(ctx, "Fetched details in %v.", clock.Since(ctx, startTime))

	// The first stage returned fewer verdicts than the page size,
	// and we retrieved all of them (didn't stop iteration early).
	if len(ids) < opts.PageSize && len(verdicts) == len(ids) {
		// There are no more verdicts to query. The page token is empty to signal end of iteration.
		return verdicts, PageToken{}, nil
	}

	// We either:
	// - Have a full page of verdicts, or
	// - Stopped iteration early using iterator.Done
	// Continue paginating from the last result we are returning.
	if len(verdicts) > 0 {
		lastSummary := summaries[len(verdicts)-1]
		nextPageToken = makePageTokenForSummary(lastSummary)
	} else {
		return nil, PageToken{}, errors.New("fetch is returning zero verdicts. This should never happen, as progress will never be made.")
	}
	return verdicts, nextPageToken, nil
}

// run queries test verdicts for a given root invocation, calling the given row callback
// for each row.
//
// To stop iteration early, the callback should return iterator.Done. The value of `consumed`
// distinguishes stopping before the row and after the row for the purposes of advancing the
// page token.
//
// It is an error to return `false` for consumed and not return an error.
func (q *Query) run(ctx context.Context, pageToken PageToken, pageSize int, rowCallback func(*TestVerdictSummary) (consumed bool, err error)) (nextPageToken PageToken, retErr error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/testverdictsv2.Query.run")
	defer func() { tracing.End(ts, retErr) }()

	st, err := q.buildQuery(pageToken, pageSize)
	if err != nil {
		return PageToken{}, err
	}

	var lastSummary *TestVerdictSummary
	var b spanutil.Buffer
	rowsSeen := 0

	err = span.Query(ctx, st).Do(func(row *spanner.Row) error {
		tv := &TestVerdictSummary{}
		var variant []string

		err := b.FromSpanner(row,
			&tv.ID.RootInvocationShardID,
			&tv.ID.ModuleName,
			&tv.ID.ModuleScheme,
			&tv.ID.ModuleVariantHash,
			&tv.ID.CoarseName,
			&tv.ID.FineName,
			&tv.ID.CaseName,
			&variant,
			&tv.Status,
			&tv.StatusOverride,
			&tv.ResultCount,
			&tv.IsMasked,
			&tv.UIPriority,
		)
		if err != nil {
			return err
		}

		// For masked test verdicts, the variant is nil. This allows distinguishing
		// a masked variant from an empty variant.
		if variant != nil {
			tv.ModuleVariant, err = pbutil.VariantFromStrings(variant)
			if err != nil {
				return errors.Fmt("module variant: %w", err)
			}
		}

		consumed, err := rowCallback(tv)
		if consumed {
			lastSummary = tv
			rowsSeen++
		}
		if err != nil {
			// Stop iteration early if there was an error.
			return err
		}
		if err == nil && !consumed {
			return errors.New("callback did not consume row and did not return an error")
		}
		return nil
	})
	if err != nil && !errors.Is(err, iterator.Done) {
		// There was an error during iteration.
		return PageToken{}, err
	}
	// We had fewer verdicts than the page size and we didn't stop iteration early.
	if rowsSeen < pageSize && err == nil {
		// There are no more verdicts to query. The page token is empty to signal end of iteration.
		return PageToken{}, nil
	}

	// We either:
	// - Have a full page of verdicts
	// - Stopped iteration early using iterator.Done
	// Continue paginating.
	if lastSummary != nil {
		nextPageToken = makePageTokenForSummary(lastSummary)
	} else {
		// The very first callback returned (consumed = false, iterator.Done), so
		// the page token did not advance.
		nextPageToken = pageToken
	}
	return nextPageToken, nil
}

func (q *Query) buildQuery(pageToken PageToken, pageSize int) (spanner.Statement, error) {
	params := map[string]any{
		"shards":        q.RootInvocationID.AllShardIDs().ToSpanner(),
		"limit":         pageSize,
		"upgradeRealms": q.Access.Realms,
	}

	paginationClause := "TRUE"
	if pageToken != (PageToken{}) {
		var err error
		paginationClause, err = q.whereAfterPageToken(pageToken, params)
		if err != nil {
			return spanner.Statement{}, appstatus.Attachf(err, codes.InvalidArgument, "page_token: invalid page token")
		}
	}

	containsTestResultFilterClause := ""
	if q.ContainsTestResultFilter != "" {
		clause, additionalParams, err := testresultsv2.WhereClause(q.ContainsTestResultFilter, "TR", "ctrf_")
		if err != nil {
			return spanner.Statement{}, errors.Fmt("contains_test_result_filter: %w", err)
		}
		for _, p := range additionalParams {
			// All parameters should be prefixed by "ctrf_" so should not conflict with existing parameters.
			params[p.Name] = p.Value
		}
		containsTestResultFilterClause = "(" + clause + ")"
	}

	wherePrefixClause := "TRUE"
	if q.TestPrefixFilter != nil {
		clause, err := testresultsv2.PrefixWhereClause(q.TestPrefixFilter, params)
		if err != nil {
			return spanner.Statement{}, errors.Fmt("test_prefix_filter: %w", err)
		}
		wherePrefixClause = "(" + clause + ")"
	}

	filterClause := "TRUE"
	if q.Filter != "" {
		clause, additionalParams, err := whereClause(q.Filter, "", "f_")
		if err != nil {
			return spanner.Statement{}, errors.Fmt("filter: %w", err)
		}
		for _, p := range additionalParams {
			// All parameters should be prefixed by "f_" so should not conflict with existing parameters.
			params[p.Name] = p.Value
		}
		filterClause = "(" + clause + ")"
	}

	tmplInput := map[string]any{
		"PaginationClause":               paginationClause,
		"OrderingByUIPriority":           q.Order.ByUIPriority,
		"OrderingByStructuredTestID":     q.Order.ByStructuredTestID,
		"ResultPassed":                   int64(pb.TestResult_PASSED),
		"ResultFailed":                   int64(pb.TestResult_FAILED),
		"ResultSkipped":                  int64(pb.TestResult_SKIPPED),
		"ResultExecutionErrored":         int64(pb.TestResult_EXECUTION_ERRORED),
		"ResultPrecluded":                int64(pb.TestResult_PRECLUDED),
		"VerdictPassed":                  int64(pb.TestVerdict_PASSED),
		"VerdictFailed":                  int64(pb.TestVerdict_FAILED),
		"VerdictSkipped":                 int64(pb.TestVerdict_SKIPPED),
		"VerdictExecutionErrored":        int64(pb.TestVerdict_EXECUTION_ERRORED),
		"VerdictPrecluded":               int64(pb.TestVerdict_PRECLUDED),
		"VerdictFlaky":                   int64(pb.TestVerdict_FLAKY),
		"VerdictExonerated":              int64(pb.TestVerdict_EXONERATED),
		"VerdictNotOverridden":           int64(pb.TestVerdict_NOT_OVERRIDDEN),
		"ContainsTestResultFilterClause": containsTestResultFilterClause,
		"WherePrefixClause":              wherePrefixClause,
		"FilterClause":                   filterClause,
		"FullAccess":                     q.Access.Level == permissions.FullAccess,
	}

	st, err := spanutil.GenerateStatement(queryTmpl, tmplInput)
	if err != nil {
		return spanner.Statement{}, err
	}
	st.Params = params
	return st, nil
}

func (q *Query) whereAfterPageToken(token PageToken, params map[string]any) (string, error) {
	var columns []spanutil.PageTokenElement

	if q.Order.ByUIPriority {
		if token.UIPriority > MaxUIPriority {
			return "", errors.New("invalid page token or logic error: uiPriority > maxUIPriority")
		}
		columns = append(columns, spanutil.PageTokenElement{
			ColumnName: "UIPriority",
			AfterValue: token.UIPriority,
			// Once this column is at its limit, fix its value in the pagination clause.
			// This allows the corresponding `ORDER BY UIPriority` clause to be ignored by the
			// query planner, and the reversion to a more efficient query plan.
			AtLimit: token.UIPriority == MaxUIPriority,
		})
	}
	if !q.Order.ByStructuredTestID {
		// Order by primary key.
		columns = append(columns, spanutil.PageTokenElement{
			ColumnName: "RootInvocationShardID",
			AfterValue: token.ID.RootInvocationShardID.RowID(),
		})
	}
	columns = append(columns, []spanutil.PageTokenElement{
		{
			ColumnName: "ModuleName",
			AfterValue: token.ID.ModuleName,
		},
		{
			ColumnName: "ModuleScheme",
			AfterValue: token.ID.ModuleScheme,
		},
		{
			ColumnName: "ModuleVariantHash",
			AfterValue: token.ID.ModuleVariantHash,
		},
		{
			ColumnName: "T1CoarseName",
			AfterValue: token.ID.CoarseName,
		},
		{
			ColumnName: "T2FineName",
			AfterValue: token.ID.FineName,
		},
		{
			ColumnName: "T3CaseName",
			AfterValue: token.ID.CaseName,
		},
	}...)
	return spanutil.WhereAfterClause(columns, "after", params), nil
}

var queryTmpl = template.Must(template.New("").Parse(`
-- We do not use WITH clauses below as Spanner query optimizer does not optimize
-- across WITH clause/CTE boundaries and this results in suboptimal query plans. Instead
-- we use templates to include the nested SQL statements.
{{define "TestResults"}}
			-- Test results.
			SELECT
				* EXCEPT (ModuleVariant, SummaryHTML, Tags, TestMetadataName, TestMetadataLocationRepo, TestMetadataLocationFileName, Properties),
				-- Provide masked versions of fields to support filtering on them in the query one level up.
				{{if eq .FullAccess true}}
					-- Directly alias the columns if full access is granted. This can provide
					-- a performance boost as predicates can be pushed down to the storage layer.
					ModuleVariant AS ModuleVariantMasked,
					Tags AS TagsMasked,
					TestMetadata as TestMetadataMasked,
					TestMetadataName AS TestMetadataNameMasked,
					TestMetadataLocationRepo AS TestMetadataLocationRepoMasked,
					TestMetadataLocationFileName AS TestMetadataLocationFileNameMasked,
					TRUE AS HasAccess,
				{{else}}
					-- User has limited acccess by default.
					IF(Realm IN UNNEST(@upgradeRealms), ModuleVariant, NULL) AS ModuleVariantMasked,
					IF(Realm IN UNNEST(@upgradeRealms), Tags, NULL) AS TagsMasked,
					IF(Realm IN UNNEST(@upgradeRealms), TestMetadata, NULL) AS TestMetadataMasked,
					IF(Realm IN UNNEST(@upgradeRealms), TestMetadataName, NULL) AS TestMetadataNameMasked,
					IF(Realm IN UNNEST(@upgradeRealms), TestMetadataLocationRepo, NULL) AS TestMetadataLocationRepoMasked,
					IF(Realm IN UNNEST(@upgradeRealms), TestMetadataLocationFileName, NULL) AS TestMetadataLocationFileNameMasked,
					(Realm IN UNNEST(@upgradeRealms)) AS HasAccess,
				{{end}}
			FROM TestResultsV2
			WHERE RootInvocationShardId IN UNNEST(@shards) AND {{.WherePrefixClause}}
{{end}}
{{define "TestExonerations"}}
			-- Test Exonerations.
			SELECT
				RootInvocationShardId, ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName,
				TRUE AS HasExonerations
			FROM TestExonerationsV2
			WHERE RootInvocationShardId IN UNNEST(@shards) AND {{.WherePrefixClause}}
			-- A given full test identifier will only appear in one shard, so we include RootInvocationShardId
			-- in the group by key to improve performance (this allows use of streaming aggregates).
			GROUP BY RootInvocationShardId, ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName
{{end}}
{{define "VerdictsByShard"}}
		-- Verdicts by shard.
		SELECT
			TR.RootInvocationShardId,
			TR.ModuleName, TR.ModuleScheme, TR.ModuleVariantHash, TR.T1CoarseName, TR.T2FineName, TR.T3CaseName,
			-- Per Spanner documentation, ANY_VALUE should prefer non-null values, but
			-- this is not observed in Spanner emulator. Use HAVING MAX to ensure determinism.
			ANY_VALUE(TR.ModuleVariantMasked HAVING MAX TR.ModuleVariantMasked IS NOT NULL) AS ModuleVariantMasked,
			(CASE
				WHEN COUNTIF(TR.StatusV2 = {{.ResultPassed}}) = 0 AND COUNTIF(TR.StatusV2 = {{.ResultFailed}}) > 0 THEN {{.VerdictFailed}}
				WHEN COUNTIF(TR.StatusV2 = {{.ResultPassed}}) > 0 AND COUNTIF(TR.StatusV2 = {{.ResultFailed}}) > 0 THEN {{.VerdictFlaky}}
				WHEN COUNTIF(TR.StatusV2 = {{.ResultPassed}}) > 0 AND COUNTIF(TR.StatusV2 = {{.ResultFailed}}) = 0 THEN {{.VerdictPassed}}
				-- Verdicts can only be skipped if there are no passed or failed results.
				WHEN COUNTIF(TR.StatusV2 = {{.ResultSkipped}}) > 0 THEN {{.VerdictSkipped}}
				-- Verdicts can only be execution errored if there are no passed, failed or skipped results.
				WHEN COUNTIF(TR.StatusV2 = {{.ResultExecutionErrored}}) > 0 THEN {{.VerdictExecutionErrored}}
				-- Verdicts can only be precluded if there are no other result statuses.
				ELSE {{.VerdictPrecluded}}
			END) AS Status,
			COALESCE(ANY_VALUE(E.HasExonerations), FALSE) AS HasExonerations,
			COUNT(1) AS ResultCount,
			-- The test verdict is reported as masked if we do not have access to any of
			-- its results. In this case, the verdict's module_variant is unavailable
			-- and the test metadata (which should be the same on all results) is
			-- also unavailable.
			NOT LOGICAL_OR(TR.HasAccess) AS IsMasked,
		FROM ({{template "TestResults" .}}
		) TR
		LEFT JOIN@{JOIN_METHOD=MERGE_JOIN} ({{template "TestExonerations" .}}
		-- A given full test identifier will only appear in one shard, so we include RootInvocationShardId
		-- in the join key and group by key to improve performance.
		-- This allows use of a merge join and streaming aggregate.
		) E USING (RootInvocationShardId, ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName)
		GROUP BY RootInvocationShardId, ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName
		{{if ne .ContainsTestResultFilterClause ""}}
			HAVING LOGICAL_OR({{.ContainsTestResultFilterClause}})
		{{end}}
{{end}}
{{define "Verdicts"}}
	-- Verdicts.
	SELECT
		RootInvocationShardId,
		ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName,
		ModuleVariantMasked,
		Status,
		(CASE
			WHEN HasExonerations AND (Status = {{.VerdictFailed}} OR Status = {{.VerdictExecutionErrored}} OR Status = {{.VerdictPrecluded}} OR Status = {{.VerdictFlaky}}) THEN {{.VerdictExonerated}}
			ELSE {{.VerdictNotOverridden}}
		END) AS StatusOverride,
		ResultCount,
		IsMasked,
		-- UI Priority Calculation. Lower is higher priority.
		-- Keep in sync with go implementation in page_token.go.
		(CASE
			-- Has blocking failures: priority 0
			WHEN (Status = {{.VerdictFailed}}) AND (NOT HasExonerations) THEN 0
			-- Has blocking test execution errors: priority 30
			WHEN (Status = {{.VerdictExecutionErrored}}) AND (NOT HasExonerations) THEN 30
			-- Has blocking precluded results: priority 30
			WHEN (Status = {{.VerdictPrecluded}}) AND (NOT HasExonerations) THEN 30
			-- Has non-exonerated flakes: priority 70
			WHEN (Status = {{.VerdictFlaky}}) AND (NOT HasExonerations) THEN 70
			-- Exonerated: priority 90
			WHEN HasExonerations AND (Status = {{.VerdictFailed}} OR Status = {{.VerdictExecutionErrored}} OR Status = {{.VerdictPrecluded}} OR Status = {{.VerdictFlaky}}) THEN 90
			-- Else: only passes or skips: priority 100
			ELSE 100
		END) AS UIPriority
	FROM (
		{{template "VerdictsByShard" .}}
	) V
{{end}}
SELECT
	RootInvocationShardId,
	ModuleName,
	ModuleScheme,
	ModuleVariantHash,
	T1CoarseName,
	T2FineName,
	T3CaseName,
	ModuleVariantMasked,
	Status,
	StatusOverride,
	ResultCount,
	IsMasked,
	UIPriority
FROM (
	{{template "Verdicts" .}}
)
WHERE {{.PaginationClause}} AND {{.FilterClause}}
ORDER BY
{{/* Note: OrderingByUIPriority and OrderingByStructuredTestID may both be set or unset. */}}
{{if .OrderingByUIPriority}}
	UIPriority,
{{end}}
{{if not .OrderingByStructuredTestID}}
	-- Unless structured test ID order is explicitly requested, prefer ordering by
	-- primary key, as it is more performant.
	RootInvocationShardId,
{{end}}
	ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName
LIMIT @limit
`))
