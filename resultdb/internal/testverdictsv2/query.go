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
	"strconv"
	"strings"
	"text/template"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testresultsv2"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// Query represents a query for test verdicts.
type Query struct {
	// The root invocation.
	RootInvocationID rootinvocations.ID
	// The number of test verdicts to return per page.
	PageSize int
	// The soft limit on the number of bytes that should be returned by a Test Verdicts query.
	// Row size is estimated as proto.Size() + 1000 bytes (to allow for alternative encodings
	// which higher fixed overheads, e.g. protojson). If this is zero, no limit is applied.
	ResponseLimitBytes int
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
}

type Ordering int

const (
	// Verdicts should be sorted by (structured) test identifier.
	// This order is the best for RPC performance as it follows the natural table ordering.
	OrderingByID Ordering = iota
	// Verdicts should be sorted by UI priority.
	// This is means the order will be:
	// - Failed
	// - Execution Error
	// - Precluded
	// - Flaky
	// - Exonerated
	// - Passed and Skipped (treated equivalently).
	OrderingByUIPriority
)

// Fetch fetches a page of test verdicts.
func (q *Query) Fetch(ctx context.Context, pageToken string) ([]*pb.TestVerdict, string, error) {
	var results []*pb.TestVerdict
	var totalSize int
	nextPageToken, err := q.run(ctx, pageToken, func(tv *TestVerdictSummary) (bool, error) {
		p := tv.ToProto()
		if q.ResponseLimitBytes > 0 {
			// Estimate row size using formula documented on q.ResponseLimitBytes.
			size := proto.Size(p) + 1000
			if totalSize+size > q.ResponseLimitBytes {
				if len(results) == 0 {
					return false, errors.Fmt("a single verdict (%v bytes) was larger than the total response limit (%v bytes)", size, q.ResponseLimitBytes)
				}
				// We would exceed the hard limit. Stop iteration early.
				return false, iterator.Done
			}
			totalSize += size
		}
		results = append(results, p)
		return true, nil
	})
	if err != nil {
		return nil, "", err
	}
	return results, nextPageToken, nil
}

// run queries test verdicts for a given root invocation, calling the given row callback
// for each row.
//
// To stop iteration early, the callback should return iterator.Done. The value of `consumed`
// distinguishes stopping before the row and after the row for the purposes of advancing the
// page token.
//
// It is an error to return `false` for consumed and not return an error.
func (q *Query) run(ctx context.Context, pageToken string, rowCallback func(*TestVerdictSummary) (consumed bool, err error)) (string, error) {
	if q.PageSize <= 0 {
		return "", errors.New("page size must be positive")
	}
	if q.ResponseLimitBytes < 0 {
		return "", errors.New("response limit bytes must be non-negative")
	}

	st, err := q.buildQuery(pageToken)
	if err != nil {
		return "", err
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
		return "", err
	}
	// We had fewer verdicts than the page size and we didn't stop iteration early.
	if rowsSeen < q.PageSize && err == nil {
		// There are no more verdicts to query. The page token is empty to signal end of iteration.
		return "", nil
	}

	var nextPageToken string
	if lastSummary != nil {
		nextPageToken = q.makePageToken(lastSummary)
	} else {
		// If there are no more verdicts, the page token is empty to signal end of iteration.
		nextPageToken = ""
	}
	return nextPageToken, nil
}

func (q *Query) buildQuery(pageToken string) (spanner.Statement, error) {
	params := map[string]any{
		"shards":        q.RootInvocationID.AllShardIDs().ToSpanner(),
		"limit":         q.PageSize,
		"upgradeRealms": q.Access.Realms,
	}

	paginationClause := "TRUE"
	if pageToken != "" {
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
		"OrderingByUIPriority":           q.Order == OrderingByUIPriority,
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

func (q *Query) makePageToken(last *TestVerdictSummary) string {
	var parts []string
	if q.Order == OrderingByUIPriority {
		parts = append(parts, fmt.Sprintf("%d", last.UIPriority))
	}
	parts = append(parts, last.ID.ModuleName)
	parts = append(parts, last.ID.ModuleScheme)
	parts = append(parts, last.ID.ModuleVariantHash)
	parts = append(parts, last.ID.CoarseName)
	parts = append(parts, last.ID.FineName)
	parts = append(parts, last.ID.CaseName)
	return pagination.Token(parts...)
}

func (q *Query) whereAfterPageToken(token string, params map[string]any) (string, error) {
	parts, err := pagination.ParseToken(token)
	if err != nil {
		return "", errors.Fmt("invalid page token: %s", err)
	}
	const testIDKeyColumns = 6
	extraSortColumns := 0
	if q.Order == OrderingByUIPriority {
		extraSortColumns = 1
	}
	if len(parts) != (testIDKeyColumns + extraSortColumns) {
		return "", errors.Fmt("expected %v components, got %d", testIDKeyColumns+extraSortColumns, len(parts))
	}

	var builder strings.Builder
	var commonClause string
	if q.Order == OrderingByUIPriority {
		uiPriority, err := strconv.Atoi(parts[0])
		if err != nil {
			return "", errors.Fmt("invalid page token, got non-integer UIPriority: %v", parts[0])
		}
		// Once we have paged to the lowest UIPriority of zero, we can stop looking for lower values
		// in the WHERE clause.
		// This allows Spanner to push down the remaining pagination clause over ModuleName, ModuleScheme, ...
		// to the underlying TestResultsV2 table scan, which can provide for a much enhanced query performance.
		if uiPriority > 0 {
			builder.WriteString(`UIPriority < @afterUIPriority OR `)
		}
		params["afterUIPriority"] = uiPriority
		builder.WriteString(`(UIPriority = @afterUIPriority AND ModuleName > @afterModuleName)`)
		commonClause = "UIPriority = @afterUIPriority AND ModuleName = @afterModuleName"
	} else {
		builder.WriteString(`ModuleName > @afterModuleName`)
		commonClause = "ModuleName = @afterModuleName"
	}

	testIDParts := parts[extraSortColumns:]
	params["afterModuleName"] = testIDParts[0]
	params["afterModuleScheme"] = testIDParts[1]
	params["afterModuleVariantHash"] = testIDParts[2]
	params["afterCoarseName"] = testIDParts[3]
	params["afterFineName"] = testIDParts[4]
	params["afterCaseName"] = testIDParts[5]

	builder.WriteString(` OR (` + commonClause + ` AND ModuleScheme > @afterModuleScheme)`)
	builder.WriteString(` OR (` + commonClause + ` AND ModuleScheme = @afterModuleScheme AND ModuleVariantHash > @afterModuleVariantHash)`)
	builder.WriteString(` OR (` + commonClause + ` AND ModuleScheme = @afterModuleScheme AND ModuleVariantHash = @afterModuleVariantHash AND T1CoarseName > @afterCoarseName)`)
	builder.WriteString(` OR (` + commonClause + ` AND ModuleScheme = @afterModuleScheme AND ModuleVariantHash = @afterModuleVariantHash AND T1CoarseName = @afterCoarseName AND T2FineName > @afterFineName)`)
	builder.WriteString(` OR (` + commonClause + ` AND ModuleScheme = @afterModuleScheme AND ModuleVariantHash = @afterModuleVariantHash AND T1CoarseName = @afterCoarseName AND T2FineName = @afterFineName AND T3CaseName > @afterCaseName)`)
	return builder.String(), nil
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
			ANY_VALUE(TR.ModuleVariantMasked) AS ModuleVariantMasked,
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
		IsMasked,
		-- UI Priority Calculation
		(CASE
			-- Has blocking failures: priority 100
			WHEN (Status = {{.VerdictFailed}}) AND (NOT HasExonerations) THEN 100
			-- Has blocking test execution errors: priority 70
			WHEN (Status = {{.VerdictExecutionErrored}}) AND (NOT HasExonerations) THEN 70
			-- Has blocking precluded results: priority 70
			WHEN (Status = {{.VerdictPrecluded}}) AND (NOT HasExonerations) THEN 70
			-- Has non-exonerated flakes: priority 30
			WHEN (Status = {{.VerdictFlaky}}) AND (NOT HasExonerations) THEN 30
			-- Exonerated: priority 10
			WHEN HasExonerations AND (Status = {{.VerdictFailed}} OR Status = {{.VerdictExecutionErrored}} OR Status = {{.VerdictPrecluded}} OR Status = {{.VerdictFlaky}}) THEN 10
			-- Else: only passes or skips: priority 0
			ELSE 0
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
	IsMasked,
	UIPriority
FROM (
	{{template "Verdicts" .}}
)
WHERE {{.PaginationClause}} AND {{.FilterClause}}
ORDER BY {{if .OrderingByUIPriority}}
	UIPriority DESC, ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName
{{else}}
	ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName
{{end}}
LIMIT @limit
`))
