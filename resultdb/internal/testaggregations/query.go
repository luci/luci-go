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

package testaggregations

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"text/template"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testresultsv2"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// SingleLevelQuery represents a query for a single level of test aggregations.
type SingleLevelQuery struct {
	// The root invocation.
	RootInvocationID rootinvocations.ID
	// The level of test aggregation to query.
	Level pb.AggregationLevel
	// The prefix of test identifiers to filter by. Optional.
	TestPrefixFilter *pb.TestIdentifierPrefix
	// An AIP-160 filter expression for test results to filter by. Optional.
	// See pb.TestAggregationPredicate and filtering implementation in
	// `testresultsv2` package for details. If this is set, an aggregation
	// will only be returned if this filter expression matches at least one
	// test result in the aggregation.
	ContainsTestResultFilter string
	// The access the caller has to the root invocation.
	Access permissions.RootInvocationAccess
	// The number of test aggregations to return per page.
	PageSize int
	// Whether to use UI sort order, i.e. by ui_priority first, instead of by test ID.
	// This incurs a performance penalty, as results are not returned in table order.
	Order Ordering
	// A filter on the test aggregations. Optional.
	// See pb.TestAggregationPredicate.Filter for details.
	Filter string
}

// Fetch fetches a page of test aggregations.
func (q *SingleLevelQuery) Fetch(ctx context.Context, pageToken string) ([]*pb.TestAggregation, string, error) {
	var results []*pb.TestAggregation
	nextPageToken, err := q.Run(ctx, pageToken, func(ta *pb.TestAggregation) error {
		results = append(results, ta)
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return results, nextPageToken, nil
}

// Run queries test aggregations for a given root invocation, calling the given row callback
// for each row. To stop iteration early, the callback should return iterator.Done.
func (q *SingleLevelQuery) Run(ctx context.Context, pageToken string, rowCallback func(*pb.TestAggregation) error) (string, error) {
	// Build SQL query based on level and prefix.
	st, err := q.buildQuery(pageToken)
	if err != nil {
		return "", err
	}
	cfg, err := config.Service(ctx)
	if err != nil {
		return "", err
	}

	// If you need to dump the query for debugging:
	// fmt.Printf("Query: %s\n", st.SQL)

	var lastResult *pb.TestAggregation
	var lastUIPriority int64
	var b spanutil.Buffer
	var rowsSeen int

	query := span.Query(ctx, st)
	err = query.Do(func(row *spanner.Row) error {
		prefix := &pb.TestIdentifierPrefix{
			Level: q.Level,
			Id:    &pb.TestIdentifier{},
		}
		// Preserve the original variant slice, to distinguish nil (masked) from empty.
		// b.FromSpanner treats both the same, in part because legacy writes prior to
		// the introduction of the TestResultsV2 and WorkUnits data model might do either.
		var variant []string
		var uiPriority int64
		var moduleStatus int64
		var moduleStatusCounts moduleStatusCounts
		var verdictCounts verdictCounts
		var nextFinerLevel pb.AggregationLevel
		var columns []interface{}

		switch q.Level {
		case pb.AggregationLevel_INVOCATION:
			columns = columnsForModuleCounts(&moduleStatusCounts)
			columns = append(columns, columnsForVerdictCounts(&verdictCounts)...)
			err := b.FromSpanner(row, columns...)
			if err != nil {
				return err
			}
			nextFinerLevel = pb.AggregationLevel_MODULE
		case pb.AggregationLevel_MODULE:
			columns = []interface{}{
				&prefix.Id.ModuleName,
				&prefix.Id.ModuleScheme,
				&prefix.Id.ModuleVariantHash,
				&variant,
				&uiPriority,
				&moduleStatus,
			}
			columns = append(columns, columnsForVerdictCounts(&verdictCounts)...)
			err := b.FromSpanner(row, columns...)
			if err != nil {
				return err
			}
			nextFinerLevel = findNextFinerLevel(cfg, prefix.Id.ModuleScheme, q.Level)
		case pb.AggregationLevel_COARSE:
			columns = []interface{}{
				&prefix.Id.ModuleName,
				&prefix.Id.ModuleScheme,
				&prefix.Id.ModuleVariantHash,
				&prefix.Id.CoarseName,
				&variant,
				&uiPriority,
			}
			columns = append(columns, columnsForVerdictCounts(&verdictCounts)...)
			err := b.FromSpanner(row, columns...)
			if err != nil {
				return err
			}
			nextFinerLevel = findNextFinerLevel(cfg, prefix.Id.ModuleScheme, q.Level)
		case pb.AggregationLevel_FINE:
			columns = []interface{}{
				&prefix.Id.ModuleName,
				&prefix.Id.ModuleScheme,
				&prefix.Id.ModuleVariantHash,
				&prefix.Id.CoarseName,
				&prefix.Id.FineName,
				&variant,
				&uiPriority,
			}
			columns = append(columns, columnsForVerdictCounts(&verdictCounts)...)
			err := b.FromSpanner(row, columns...)
			if err != nil {
				return err
			}
			nextFinerLevel = pb.AggregationLevel_CASE
		default:
			return fmt.Errorf("unknown aggregation level: %v", q.Level)
		}

		if variant != nil {
			prefix.Id.ModuleVariant, err = pbutil.VariantFromStrings(variant)
			if err != nil {
				return err
			}
		}

		agg := &pb.TestAggregation{
			Id:             prefix,
			NextFinerLevel: nextFinerLevel,
			VerdictCounts: &pb.TestAggregation_VerdictCounts{
				// By status after overrides.
				Failed:           int32(verdictCounts.Failed),
				Flaky:            int32(verdictCounts.Flaky),
				Passed:           int32(verdictCounts.Passed),
				Skipped:          int32(verdictCounts.Skipped),
				ExecutionErrored: int32(verdictCounts.ExecutionErrored),
				Precluded:        int32(verdictCounts.Precluded),
				Exonerated:       int32(verdictCounts.FailedExonerated + verdictCounts.FlakyExonerated + verdictCounts.ExecutionErroredExonerated + verdictCounts.PrecludedExonerated),
				// By base status.
				FailedBase:           int32(verdictCounts.Failed + verdictCounts.FailedExonerated),
				FlakyBase:            int32(verdictCounts.Flaky + verdictCounts.FlakyExonerated),
				PassedBase:           int32(verdictCounts.Passed),
				SkippedBase:          int32(verdictCounts.Skipped),
				ExecutionErroredBase: int32(verdictCounts.ExecutionErrored + verdictCounts.ExecutionErroredExonerated),
				PrecludedBase:        int32(verdictCounts.Precluded + verdictCounts.PrecludedExonerated),
			},
		}
		if q.Level == pb.AggregationLevel_MODULE {
			agg.ModuleStatus = pb.TestAggregation_ModuleStatus(moduleStatus)
		}
		if q.Level == pb.AggregationLevel_INVOCATION {
			agg.ModuleStatusCounts = &pb.TestAggregation_ModuleStatusCounts{
				Failed:    int32(moduleStatusCounts.Failed),
				Running:   int32(moduleStatusCounts.Running),
				Pending:   int32(moduleStatusCounts.Pending),
				Cancelled: int32(moduleStatusCounts.Cancelled),
				Succeeded: int32(moduleStatusCounts.Succeeded),
				Skipped:   int32(moduleStatusCounts.Skipped),
			}
		}
		lastResult = agg
		lastUIPriority = uiPriority
		if err := rowCallback(agg); err != nil {
			return err
		}
		rowsSeen++
		return nil
	})
	if err != nil && !errors.Is(err, iterator.Done) {
		return "", err
	}
	// We had fewer rows than the page size and we didn't stop iteration early.
	if rowsSeen < q.PageSize && err == nil {
		// There are no more groups to query.
		return "", nil
	}

	var nextPageToken string
	if lastResult != nil {
		nextPageToken = q.makePageToken(lastResult, lastUIPriority)
	} else {
		// If there are no more groups, the page token is empty to signal end of iteration.
		nextPageToken = ""
	}
	return nextPageToken, nil
}

// findNextFinerLevel finds the next finer aggregation level that would be
// useful to query for the given aggregation level and scheme.
// Because not all schemes use all levels of the test hierarchy, it makes sense
// to skip some levels, especially when drilling-down into aggregations on the
// UI.
func findNextFinerLevel(cfg *config.CompiledServiceConfig, scheme string, currentLevel pb.AggregationLevel) pb.AggregationLevel {
	// Try to lookup the config for the scheme.
	// Note that this may be nil if the scheme is no longer configured;
	// in that case we only want to go to the next finer level.
	schemeConfig := cfg.Schemes[scheme]

	// Go to the next finer level if we can.
	nextLevel := currentLevel
	if currentLevel <= pb.AggregationLevel_FINE {
		nextLevel++
	}
	// If the scheme is configured, and the coarse-level is not used by the scheme,
	// advance past it.
	if nextLevel == pb.AggregationLevel_COARSE && (schemeConfig != nil && schemeConfig.Coarse == nil) {
		nextLevel = pb.AggregationLevel_FINE
	}
	// If the scheme is configured, and the fine-level is not used by the scheme,
	// advance past it.
	if nextLevel == pb.AggregationLevel_FINE && (schemeConfig != nil && schemeConfig.Fine == nil) {
		nextLevel = pb.AggregationLevel_CASE
	}
	return nextLevel
}

func (q *SingleLevelQuery) buildQuery(pageToken string) (spanner.Statement, error) {
	params := map[string]any{
		"shards":        q.RootInvocationID.AllShardIDs(),
		"pageSize":      q.PageSize,
		"upgradeRealms": q.Access.Realms,
	}

	wherePrefixClause := "TRUE"
	if q.TestPrefixFilter != nil {
		clause, err := q.prefixWhereClause(q.TestPrefixFilter, params)
		if err != nil {
			return spanner.Statement{}, errors.Fmt("test_prefix_filter: %w", err)
		}
		wherePrefixClause = "(" + clause + ")"
	}

	containsTestResultFilterClause := ""
	if q.ContainsTestResultFilter != "" {
		clause, additionalParams, err := testresultsv2.WhereClause(q.ContainsTestResultFilter, "TR", "ctrf_")
		if err != nil {
			return spanner.Statement{}, errors.Fmt("contains_test_result_filter: %w", err)
		}
		for _, p := range additionalParams {
			// All parameters should be prefixed by "ctrf" so should not conflict with existing parameters.
			params[p.Name] = p.Value
		}
		containsTestResultFilterClause = "(" + clause + ")"
	}

	paginationClause := "TRUE"
	if pageToken != "" {
		clause, err := whereAfterPageToken(q.Level, q.Order, pageToken, params)
		if err != nil {
			return spanner.Statement{}, appstatus.Attachf(err, codes.InvalidArgument, "page_token: invalid page token")
		}
		paginationClause = "(" + clause + ")"
	}

	filterClause := "TRUE"
	if q.Filter != "" {
		clause, additionalParams, err := whereClause(q.Filter, "", "f_")
		if err != nil {
			return spanner.Statement{}, errors.Fmt("filter: %w", err)
		}
		for _, p := range additionalParams {
			params[p.Name] = p.Value
		}
		filterClause = "(" + clause + ")"
	}

	templateParams := templateParameters{
		AggregateColumns:               groupingColumns(q.Level),
		OrderByColumns:                 orderByColumns(q.Order, q.Level),
		WherePrefixClause:              wherePrefixClause,
		ContainsTestResultFilterClause: containsTestResultFilterClause,
		PaginationClause:               paginationClause,
		FilterClause:                   filterClause,
		FullAccess:                     q.Access.Level == permissions.FullAccess,
		ResultStatuses: testResultStatusDefinitions{
			Passed:           int64(pb.TestResult_PASSED),
			Failed:           int64(pb.TestResult_FAILED),
			Skipped:          int64(pb.TestResult_SKIPPED),
			ExecutionErrored: int64(pb.TestResult_EXECUTION_ERRORED),
			Precluded:        int64(pb.TestResult_PRECLUDED),
		},
		VerdictStatuses: verdictStatusDefinitions{
			Flaky:            int64(pb.TestVerdict_FLAKY),
			Passed:           int64(pb.TestVerdict_PASSED),
			Failed:           int64(pb.TestVerdict_FAILED),
			Skipped:          int64(pb.TestVerdict_SKIPPED),
			ExecutionErrored: int64(pb.TestVerdict_EXECUTION_ERRORED),
			Precluded:        int64(pb.TestVerdict_PRECLUDED),
		},
		WorkUnitStatuses: workUnitStatusDefinitions{
			Pending:   int64(pb.WorkUnit_PENDING),
			Running:   int64(pb.WorkUnit_RUNNING),
			Succeeded: int64(pb.WorkUnit_SUCCEEDED),
			Skipped:   int64(pb.WorkUnit_SKIPPED),
			Failed:    int64(pb.WorkUnit_FAILED),
			Cancelled: int64(pb.WorkUnit_CANCELLED),
		},
		ModuleStatuses: moduleStatusDefinitions{
			Unspecified: int64(pb.TestAggregation_MODULE_STATUS_UNSPECIFIED),
			Pending:     int64(pb.TestAggregation_PENDING),
			Running:     int64(pb.TestAggregation_RUNNING),
			Succeeded:   int64(pb.TestAggregation_SUCCEEDED),
			Skipped:     int64(pb.TestAggregation_SKIPPED),
			Errored:     int64(pb.TestAggregation_ERRORED),
			Cancelled:   int64(pb.TestAggregation_CANCELLED),
		},
	}
	templateName := ""
	switch q.Level {
	case pb.AggregationLevel_INVOCATION:
		templateName = "invocation"
	case pb.AggregationLevel_MODULE:
		templateName = "modules"
	case pb.AggregationLevel_COARSE, pb.AggregationLevel_FINE:
		templateName = "coarseOrFine"
	default:
		return spanner.Statement{}, errors.Fmt("unsupported aggregation level: %v", q.Level)
	}

	stmt, err := genStatement(templateName, templateParams, params)
	if err != nil {
		return spanner.Statement{}, errors.Fmt("generate query SQL statement: %w", err)
	}
	return stmt, nil
}

// queryTmpl is a set of templates that generate the SQL statements used
// by Query type.
var queryTmpl = template.Must(template.New("").Parse(`
-- We do not use WITH clauses below as Spanner query optimizer does not optimize
-- across WITH clause/CTE boundaries and this results in suboptimal query plans. Instead
-- we use templates to include the nested SQL statements.
{{define "Exonerations"}}
				-- Exonerated test IDs.
				SELECT
					RootInvocationShardId, ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName
				FROM TestExonerationsV2
				WHERE (RootInvocationShardId IN UNNEST(@shards)) AND {{.WherePrefixClause}}
				-- A given full test identifier will only appear in one shard, so we include RootInvocationShardId
				-- in the group by to improve performance (this makes the GROUP BY columns a primary key prefix).
				GROUP BY RootInvocationShardId, ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName
{{end}}
{{define "TestResults"}}
				-- Test Results.
				SELECT
					* EXCEPT (ModuleVariant, Tags, TestMetadataName, TestMetadataLocationRepo, TestMetadataLocationFileName),
				-- Provide masked versions of fields to support filtering on them in the query one level up.
				{{if eq .FullAccess true}}
					-- Directly alias the columns if full access is granted. This can provide
					-- a performance boost as predicates can be pushed down to the storage layer.
					ModuleVariant AS ModuleVariantMasked,
					Tags AS TagsMasked,
					TestMetadataName AS TestMetadataNameMasked,
					TestMetadataLocationRepo AS TestMetadataLocationRepoMasked,
					TestMetadataLocationFileName AS TestMetadataLocationFileNameMasked,
				{{else}}
					-- User has limited acccess by default.
					IF(Realm IN UNNEST(@upgradeRealms), ModuleVariant, NULL) AS ModuleVariantMasked,
					IF(Realm IN UNNEST(@upgradeRealms), Tags, NULL) AS TagsMasked,
					IF(Realm IN UNNEST(@upgradeRealms), TestMetadataName, NULL) AS TestMetadataNameMasked,
					IF(Realm IN UNNEST(@upgradeRealms), TestMetadataLocationRepo, NULL) AS TestMetadataLocationRepoMasked,
					IF(Realm IN UNNEST(@upgradeRealms), TestMetadataLocationFileName, NULL) AS TestMetadataLocationFileNameMasked,
				{{end}}
				FROM TestResultsV2
				WHERE (RootInvocationShardId IN UNNEST(@shards)) AND {{.WherePrefixClause}}
{{end}}
{{define "Verdicts"}}
			-- Verdicts.
			SELECT
				RootInvocationShardId,
				ModuleName,	ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName,
				ANY_VALUE(ModuleVariantMasked) AS ModuleVariantMasked,
				(CASE
					WHEN COUNTIF(TR.StatusV2 = {{.ResultStatuses.Passed}}) = 0 AND COUNTIF(TR.StatusV2 = {{.ResultStatuses.Failed}}) > 0 THEN {{.VerdictStatuses.Failed}} -- Failed
					WHEN COUNTIF(TR.StatusV2 = {{.ResultStatuses.Passed}}) > 0 AND COUNTIF(TR.StatusV2 = {{.ResultStatuses.Failed}}) > 0 THEN {{.VerdictStatuses.Flaky}} -- Flaky
					WHEN COUNTIF(TR.StatusV2 = {{.ResultStatuses.Passed}}) > 0 AND COUNTIF(TR.StatusV2 = {{.ResultStatuses.Failed}}) = 0 THEN {{.VerdictStatuses.Passed}} -- Passed
					-- If we fall through to the cases below, we know that Count(Pass) = 0 AND Count(Fail) = 0.
					WHEN COUNTIF(TR.StatusV2 = {{.ResultStatuses.Skipped}}) > 0 THEN {{.VerdictStatuses.Skipped}} -- Skipped
					-- If we fall through to the cases below, we know that Count(Skip) = 0.
					WHEN COUNTIF(TR.StatusV2 = {{.ResultStatuses.ExecutionErrored}}) > 0 THEN {{.VerdictStatuses.ExecutionErrored}} -- Execution Errored
					-- If we fall through here, we know that Count(ExecutionErrored) = 0. Therefore the only
					-- results we have must be precluded results.
					ELSE {{.VerdictStatuses.Precluded}} -- Precluded
					END) AS VerdictStatus,
				ANY_VALUE(E.T3CaseName IS NOT NULL) AS IsExonerated,
			{{if ne .ContainsTestResultFilterClause ""}}
				LOGICAL_OR({{.ContainsTestResultFilterClause}}) AS ContainsTestResultMatchingFilter,
			{{end}}
			FROM ({{template "TestResults" .}}
			) TR
			LEFT JOIN@{JOIN_METHOD=MERGE_JOIN} ({{template "Exonerations" .}}
			-- A given full test identifier will only appear in one shard, so we include RootInvocationShardId
			-- in the join key to improve performance.
			) E USING (RootInvocationShardId, ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName)
			-- A given full test identifier will only appear in one shard, so we include RootInvocationShardId
			-- in the group by to improve performance (this makes the GROUP BY columns a primary key prefix).
			GROUP BY RootInvocationShardId, ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName
{{end}}
{{define "TestAggregationsByShard"}}
    -- Test aggregations, grouped by shard. This allows most aggregation to occur in parallel on each
		-- shard before before the results are combined via a distributed union.
		SELECT
			RootInvocationShardId,
		{{if ne .AggregateColumns ""}}
			{{.AggregateColumns}},
			ANY_VALUE(ModuleVariantMasked) AS ModuleVariantMasked,
		{{end}}
			COUNTIF(VerdictStatus = {{.VerdictStatuses.Passed}}) AS Passed,
			COUNTIF(VerdictStatus = {{.VerdictStatuses.Failed}} AND NOT IsExonerated) AS Failed,
			COUNTIF(VerdictStatus = {{.VerdictStatuses.Flaky}} AND NOT IsExonerated) AS Flaky,
			COUNTIF(VerdictStatus = {{.VerdictStatuses.Skipped}}) AS Skipped,
			COUNTIF(VerdictStatus = {{.VerdictStatuses.ExecutionErrored}} AND NOT IsExonerated) AS ExecutionErrored,
			COUNTIF(VerdictStatus = {{.VerdictStatuses.Precluded}} AND NOT IsExonerated) AS Precluded,
			COUNTIF(VerdictStatus = {{.VerdictStatuses.Failed}} AND IsExonerated) AS FailedExonerated,
			COUNTIF(VerdictStatus = {{.VerdictStatuses.Flaky}} AND IsExonerated) AS FlakyExonerated,
			COUNTIF(VerdictStatus = {{.VerdictStatuses.ExecutionErrored}} AND IsExonerated) AS ExecutionErroredExonerated,
			COUNTIF(VerdictStatus = {{.VerdictStatuses.Precluded}} AND IsExonerated) AS PrecludedExonerated,
		{{if ne .ContainsTestResultFilterClause ""}}
			LOGICAL_OR(ContainsTestResultMatchingFilter) AS ContainsTestResultMatchingFilter,
		{{end}}
			(CASE
				-- Has blocking failures: priority 100
				WHEN COUNTIF(VerdictStatus = {{.VerdictStatuses.Failed}} AND NOT IsExonerated) > 0 THEN 100
				-- Has blocking test execution errors: priority 70
				WHEN COUNTIF(VerdictStatus = {{.VerdictStatuses.ExecutionErrored}} AND NOT IsExonerated) > 0 THEN 70
				-- Has blocking precluded results (will typically overlap with module error): priority 70
				WHEN COUNTIF(VerdictStatus = {{.VerdictStatuses.Precluded}} AND NOT IsExonerated) > 0 THEN 70
				-- Has non-exonerated flakes: priority 30
				WHEN COUNTIF(VerdictStatus = {{.VerdictStatuses.Flaky}} AND NOT IsExonerated) > 0 THEN 30
				-- Has exonerations: priority 10
				WHEN COUNTIF(VerdictStatus = {{.VerdictStatuses.Failed}} AND IsExonerated) > 0 THEN 10
				-- Else: only passes or skips: priority 0
				ELSE 0
				END) AS UIPriority
		FROM ({{template "Verdicts" .}}
		)
		GROUP BY RootInvocationShardId{{if ne .AggregateColumns ""}}, {{.AggregateColumns}}{{end}}
{{end}}
{{define "TestAggregations"}}
	-- Test aggregations.
	SELECT
	{{if ne .AggregateColumns ""}}
		{{.AggregateColumns}},
		ANY_VALUE(ModuleVariantMasked) AS ModuleVariantMasked,
	{{end}}
		MAX(UIPriority) AS UIPriority,
		-- This field is used to ensure filters on module_status for FINE and COARSE levels are not rejected.
		-- It is always UNSPECIFIED for these levels.
		{{.ModuleStatuses.Unspecified}} AS ModuleStatus,
		SUM(Passed) AS TestsPassed,
		SUM(Failed) AS TestsFailed,
		SUM(Flaky) AS TestsFlaky,
		SUM(Skipped) AS TestsSkipped,
		SUM(ExecutionErrored) AS TestsExecutionErrored,
		SUM(Precluded) AS TestsPrecluded,
		SUM(FailedExonerated) AS TestsFailedExonerated,
		SUM(FlakyExonerated) AS TestsFlakyExonerated,
		SUM(ExecutionErroredExonerated) AS TestsExecutionErroredExonerated,
		SUM(PrecludedExonerated) AS TestsPrecludedExonerated,
		-- This field is used to support filtering on verdict_counts.exonerated.
		(SUM(FailedExonerated) + SUM(FlakyExonerated) + SUM(ExecutionErroredExonerated) + SUM(PrecludedExonerated)) AS TestsExonerated,
	{{if ne .ContainsTestResultFilterClause ""}}
		LOGICAL_OR(ContainsTestResultMatchingFilter) AS ContainsTestResultMatchingFilter,
	{{end}}
	FROM ({{template "TestAggregationsByShard" .}}
	)
	{{if ne .AggregateColumns ""}}GROUP BY {{.AggregateColumns}}{{end}}
{{end}}
{{define "ModuleShards"}}
			-- Module shards.
			SELECT
				ModuleName,
				ModuleScheme,
				ModuleVariantHash,
			{{if eq .FullAccess true}}
				ANY_VALUE(ModuleVariant) AS ModuleVariantMasked,
			{{else}}
				-- User has limited acccess by default.
				ANY_VALUE(IF(Realm IN UNNEST(@upgradeRealms), ModuleVariant, NULL)) AS ModuleVariantMasked,
			{{end}}
				ModuleShardKey,
				-- Aggregate from root work units to shards.
				-- SUCCEEDED > RUNNING > PENDING > SKIPPED > FAILED > CANCELLED.
				(CASE
					WHEN COUNTIF(State = {{.WorkUnitStatuses.Succeeded}}) > 0 THEN {{.ModuleStatuses.Succeeded}}
					WHEN COUNTIF(State = {{.WorkUnitStatuses.Running}}) > 0 THEN {{.ModuleStatuses.Running}}
					WHEN COUNTIF(State = {{.WorkUnitStatuses.Pending}}) > 0 THEN {{.ModuleStatuses.Pending}}
					WHEN COUNTIF(State = {{.WorkUnitStatuses.Skipped}}) > 0 THEN {{.ModuleStatuses.Skipped}}
					WHEN COUNTIF(State = {{.WorkUnitStatuses.Failed}}) > 0 THEN {{.ModuleStatuses.Errored}}
					ELSE {{.ModuleStatuses.Cancelled}}
					END) AS ShardStatus
			FROM WorkUnits
			WHERE RootInvocationShardId IN UNNEST(@shards)
				AND ModuleInheritanceStatus = 2 -- Root work unit.
				AND {{.WherePrefixClause}}
			GROUP BY ModuleName, ModuleScheme, ModuleVariantHash, ModuleShardKey
{{end}}
{{define "ModuleSummaries"}}
		-- Module summaries.
		SELECT
			ModuleName,
			ModuleScheme,
			ModuleVariantHash,
			ANY_VALUE(ModuleVariantMasked) AS ModuleVariantMasked,
			-- Aggregate from shards to modules.
			-- FAILED > RUNNING > PENDING > CANCELLED > SUCCEEDED > SKIPPED.
			(CASE
				WHEN COUNTIF(ShardStatus = {{.ModuleStatuses.Errored}}) > 0 THEN {{.ModuleStatuses.Errored}}
				WHEN COUNTIF(ShardStatus = {{.ModuleStatuses.Running}}) > 0 THEN {{.ModuleStatuses.Running}}
				WHEN COUNTIF(ShardStatus = {{.ModuleStatuses.Pending}}) > 0 THEN {{.ModuleStatuses.Pending}}
				WHEN COUNTIF(ShardStatus = {{.ModuleStatuses.Cancelled}}) > 0 THEN {{.ModuleStatuses.Cancelled}}
				WHEN COUNTIF(ShardStatus = {{.ModuleStatuses.Succeeded}}) > 0 THEN {{.ModuleStatuses.Succeeded}}
				ELSE {{.ModuleStatuses.Skipped}}
				END) AS ModuleStatus,
			(CASE
				-- Module failed: priority 70.
				WHEN COUNTIF(ShardStatus = {{.ModuleStatuses.Errored}}) > 0 THEN 70
				-- Module running: priority 0
				WHEN COUNTIF(ShardStatus = {{.ModuleStatuses.Running}}) > 0 THEN 0
				-- Module pending: priority 0
				WHEN COUNTIF(ShardStatus = {{.ModuleStatuses.Pending}}) > 0 THEN 0
				-- Module cancelled: priority 50
				WHEN COUNTIF(ShardStatus = {{.ModuleStatuses.Cancelled}}) > 0 THEN 50
				-- Module succeeded or skipped: priority 0
				ELSE 0
				END) AS UIPriority
		FROM ({{template "ModuleShards" .}}
		)
		GROUP BY ModuleName, ModuleScheme, ModuleVariantHash
{{end}}
{{define "ModulesWithResults"}}
	-- Modules with test result aggregations.
	SELECT
		ModuleName,
		ModuleScheme,
		ModuleVariantHash,
		-- For legacy modules, there may no work unit(s) defining the module.
		-- Take the variant from the test results table.
		COALESCE(M.ModuleVariantMasked, TA.ModuleVariantMasked) as ModuleVariantMasked,
		GREATEST(M.UIPriority, COALESCE(TA.UIPriority, 0)) AS UIPriority,
		-- For legacy modules, there may no work unit(s) defining the module status.
		-- Default the module status to UNSPECIFIED.
		COALESCE(M.ModuleStatus, {{.ModuleStatuses.Unspecified}}) AS ModuleStatus,
		COALESCE(TA.TestsPassed, 0) AS TestsPassed,
		COALESCE(TA.TestsFailed, 0) AS TestsFailed,
		COALESCE(TA.TestsFlaky, 0) AS TestsFlaky,
		COALESCE(TA.TestsSkipped, 0) AS TestsSkipped,
		COALESCE(TA.TestsExecutionErrored, 0) AS TestsExecutionErrored,
		COALESCE(TA.TestsPrecluded, 0) AS TestsPrecluded,
		COALESCE(TA.TestsFailedExonerated, 0) AS TestsFailedExonerated,
		COALESCE(TA.TestsFlakyExonerated, 0) AS TestsFlakyExonerated,
		COALESCE(TA.TestsExecutionErroredExonerated, 0) AS TestsExecutionErroredExonerated,
		COALESCE(TA.TestsPrecludedExonerated, 0) AS TestsPrecludedExonerated,
		-- This field is used to support filtering on verdict_counts.exonerated.
		COALESCE(TA.TestsExonerated, 0) AS TestsExonerated,
	{{if ne .ContainsTestResultFilterClause ""}}
		COALESCE(TA.ContainsTestResultMatchingFilter, FALSE) AS ContainsTestResultMatchingFilter,
	{{end}}
	FROM ({{template "ModuleSummaries" .}}
	) M
	-- This is an FULL OUTER JOIN instead of a LEFT JOIN to accomodate
	-- for test results uploaded to the module "legacy", which may not
	-- have an associated work unit with an identical variant.
	FULL OUTER JOIN ({{template "TestAggregations" .}}
	) TA USING (ModuleName, ModuleScheme, ModuleVariantHash)
{{end}}
{{define "ModuleAggregations"}}
	-- Module aggregations.
	SELECT
		COUNTIF(ModuleStatus = {{.ModuleStatuses.Errored}}) AS Failed,
		COUNTIF(ModuleStatus = {{.ModuleStatuses.Running}}) AS Running,
		COUNTIF(ModuleStatus = {{.ModuleStatuses.Pending}}) AS Pending,
		COUNTIF(ModuleStatus = {{.ModuleStatuses.Cancelled}}) AS Cancelled,
		COUNTIF(ModuleStatus = {{.ModuleStatuses.Succeeded}}) AS Succeeded,
		COUNTIF(ModuleStatus = {{.ModuleStatuses.Skipped}}) AS Skipped
	FROM ({{template "ModuleSummaries" .}}
	)
{{end}}
-- Final queries follow.
{{define "coarseOrFine"}}
SELECT
	{{.AggregateColumns}},
	ModuleVariantMasked,
	UIPriority,
	TestsPassed,
	TestsFailed,
	TestsFlaky,
	TestsSkipped,
	TestsExecutionErrored,
	TestsPrecluded,
	TestsFailedExonerated,
	TestsFlakyExonerated,
	TestsExecutionErroredExonerated,
	TestsPrecludedExonerated,
FROM ({{template "TestAggregations" .}}
)
WHERE {{.PaginationClause}} AND {{.FilterClause}}
{{if ne .ContainsTestResultFilterClause ""}} AND ContainsTestResultMatchingFilter{{end}}
ORDER BY {{.OrderByColumns}}
LIMIT @pageSize
{{end}}

{{define "modules"}}
SELECT
	ModuleName,
	ModuleScheme,
	ModuleVariantHash,
	ModuleVariantMasked,
	UIPriority,
	ModuleStatus,
	TestsPassed,
	TestsFailed,
	TestsFlaky,
	TestsSkipped,
	TestsExecutionErrored,
	TestsPrecluded,
	TestsFailedExonerated,
	TestsFlakyExonerated,
	TestsExecutionErroredExonerated,
	TestsPrecludedExonerated,
FROM ({{template "ModulesWithResults" .}}
)
WHERE {{.PaginationClause}} AND {{.FilterClause}}
{{if ne .ContainsTestResultFilterClause ""}} AND ContainsTestResultMatchingFilter{{end}}
ORDER BY {{.OrderByColumns}}
LIMIT @pageSize
{{end}}


{{define "invocation"}}
SELECT
	COALESCE(MA.Failed, 0) AS ModulesFailed,
	COALESCE(MA.Running, 0) AS ModulesRunning,
	COALESCE(MA.Pending, 0) AS ModulesPending,
	COALESCE(MA.Cancelled, 0) AS ModulesCancelled,
	COALESCE(MA.Succeeded, 0) AS ModulesSucceeded,
	COALESCE(MA.Skipped, 0) AS ModulesSkipped,
	COALESCE(TA.TestsPassed, 0) AS TestsPassed,
	COALESCE(TA.TestsFailed, 0) AS TestsFailed,
	COALESCE(TA.TestsFlaky, 0) AS TestsFlaky,
	COALESCE(TA.TestsSkipped, 0) AS TestsSkipped,
	COALESCE(TA.TestsExecutionErrored, 0) AS TestsExecutionErrored,
	COALESCE(TA.TestsPrecluded, 0) AS TestsPrecluded,
	COALESCE(TA.TestsFailedExonerated, 0) AS TestsFailedExonerated,
	COALESCE(TA.TestsFlakyExonerated, 0) AS TestsFlakyExonerated,
	COALESCE(TA.TestsExecutionErroredExonerated, 0) AS TestsExecutionErroredExonerated,
	COALESCE(TA.TestsPrecludedExonerated, 0) AS TestsPrecludedExonerated
-- Each table has exactly one row.
FROM ({{template "TestAggregations" .}}
) TA
CROSS JOIN@{JOIN_TYPE=HASH_JOIN} ({{template "ModuleAggregations" .}}
) MA
WHERE {{.FilterClause}}
{{if ne .ContainsTestResultFilterClause ""}}
 AND ContainsTestResultMatchingFilter
{{end}}
{{end}}
`))

// verdictCounts stores the counts of verdicts in a test aggregation, as returned by the Spanner query.
type verdictCounts struct {
	Passed                     int64
	Failed                     int64
	Flaky                      int64
	Skipped                    int64
	ExecutionErrored           int64
	Precluded                  int64
	FailedExonerated           int64
	FlakyExonerated            int64
	ExecutionErroredExonerated int64
	PrecludedExonerated        int64
}

// columnsForVerdictCounts facilitates reading a part of a Spanner row into a verdictCounts struct.
func columnsForVerdictCounts(result *verdictCounts) []interface{} {
	// The order of columns in this method must match the SQL above.
	return []interface{}{
		&result.Passed,
		&result.Failed,
		&result.Flaky,
		&result.Skipped,
		&result.ExecutionErrored,
		&result.Precluded,
		&result.FailedExonerated,
		&result.FlakyExonerated,
		&result.ExecutionErroredExonerated,
		&result.PrecludedExonerated,
	}
}

// moduleStatusCounts stores the counts of module statuses for an invocation-level aggregation,
// as returned by the Spanner query.
type moduleStatusCounts struct {
	Failed    int64
	Running   int64
	Pending   int64
	Cancelled int64
	Succeeded int64
	Skipped   int64
}

// columnsForModuleCounts facilitates reading a part of a Spanner row into a moduleStatusCounts struct.
func columnsForModuleCounts(result *moduleStatusCounts) []interface{} {
	// The order of columns in this method must match the SQL above.
	return []interface{}{
		&result.Failed,
		&result.Running,
		&result.Pending,
		&result.Cancelled,
		&result.Succeeded,
		&result.Skipped,
	}
}

type templateParameters struct {
	// The comma-separated list of key columns to aggregate by.
	AggregateColumns string
	// The comma-separated list of columns to order by.
	OrderByColumns string
	// The WHERE clause filtering to the test ID prefix we are interested in.
	// This is pushed down as far as possible in the query to maximise the
	// chance it will cut down the amount of data scanned.
	WherePrefixClause string
	// The clause defining the test results we are interested in. If set,
	// an aggeregation will only be returned if it contains a test result
	// that matches this filter. N.B. Setting this to TRUE will filter to
	// only module-aggregations that contain at least one test result.
	ContainsTestResultFilterClause string
	// The WHERE clause to apply to the final query for pagination purposes.
	PaginationClause string
	// The WHERE clause to apply to the final query for filtering purposes.
	FilterClause string
	// Whether the user has full access to the test results, exonerations and work units.
	// If this is false, it is assumed the user has at least limited (masked) access to all of them.
	FullAccess bool

	ResultStatuses   testResultStatusDefinitions
	VerdictStatuses  verdictStatusDefinitions
	WorkUnitStatuses workUnitStatusDefinitions
	ModuleStatuses   moduleStatusDefinitions
}

// testResultStatusDefinitions contains the values of the test result status v2 enum.
type testResultStatusDefinitions struct {
	Passed           int64
	Failed           int64
	Skipped          int64
	ExecutionErrored int64
	Precluded        int64
}

// verdictStatusDefinitions contains the values of the verdict status v2 enum.
type verdictStatusDefinitions struct {
	Flaky            int64
	Passed           int64
	Failed           int64
	Skipped          int64
	ExecutionErrored int64
	Precluded        int64
	Exonerated       int64
}

// workUnitStatusDefinitions contains the values of the work unit status enum.
type workUnitStatusDefinitions struct {
	Pending   int64
	Running   int64
	Succeeded int64
	Skipped   int64
	Failed    int64
	Cancelled int64
}

// moduleStatusDefinitions contains the values of the module status enum.
type moduleStatusDefinitions struct {
	Unspecified int64
	Pending     int64
	Running     int64
	Succeeded   int64
	Skipped     int64
	Errored     int64
	Cancelled   int64
}

func genStatement(templateName string, templateParams templateParameters, params map[string]any) (spanner.Statement, error) {
	var sql bytes.Buffer
	err := queryTmpl.ExecuteTemplate(&sql, templateName, templateParams)
	if err != nil {
		return spanner.Statement{}, err
	}
	return spanner.Statement{SQL: sql.String(), Params: spanutil.ToSpannerMap(params)}, nil
}

func (q *SingleLevelQuery) makePageToken(lastResult *pb.TestAggregation, lastUIPriority int64) string {
	var components []string
	lastPrefix := lastResult.Id
	if lastPrefix.Level == pb.AggregationLevel_INVOCATION {
		// This aggregation type only ever produces one row, so the page token is empty
		// to signal end of iteration.
		return ""
	}
	if q.Order.ByUIPriority {
		components = append(components, fmt.Sprintf("%d", lastUIPriority))
	}
	components = append(components, lastPrefix.Id.ModuleName)
	components = append(components, lastPrefix.Id.ModuleScheme)
	components = append(components, lastPrefix.Id.ModuleVariantHash)
	if lastPrefix.Level == pb.AggregationLevel_MODULE {
		return pagination.Token(components...)
	}
	components = append(components, lastPrefix.Id.CoarseName)
	if lastPrefix.Level == pb.AggregationLevel_COARSE {
		return pagination.Token(components...)
	}
	components = append(components, lastPrefix.Id.FineName)
	if lastPrefix.Level == pb.AggregationLevel_FINE {
		return pagination.Token(components...)
	}
	// We don't support aggregations finer than FINE in this method.
	// Use an RPC like QueryTestVerdicts instead.
	panic(fmt.Sprintf("logic error: invalid aggregation level %v", lastPrefix.Level))
}

func whereAfterPageToken(level pb.AggregationLevel, order Ordering, token string, params map[string]any) (string, error) {
	if token == "" {
		return "TRUE", nil
	}
	if level == pb.AggregationLevel_INVOCATION {
		// INVOCATION level only ever produces one row, so non-empty page tokens are not allowed.
		return "", errors.Fmt("invalid page token, it is never valid to have a page token for an invocation-level aggregation")
	}
	components, err := pagination.ParseToken(token)
	if err != nil {
		return "", errors.Fmt("invalid page token: %s", err)
	}
	var otherSortColumns int
	if order.ByUIPriority {
		otherSortColumns = 1
	}
	switch level {
	case pb.AggregationLevel_MODULE:
		if len(components) != (3 + otherSortColumns) {
			return "", errors.Fmt("invalid page token, got %v components, expected %v", len(components), 3+otherSortColumns)
		}
	case pb.AggregationLevel_COARSE:
		if len(components) != (4 + otherSortColumns) {
			return "", errors.Fmt("invalid page token, got %v components, expected %v", len(components), 4+otherSortColumns)
		}
	case pb.AggregationLevel_FINE:
		if len(components) != (5 + otherSortColumns) {
			return "", errors.Fmt("invalid page token, got %v components, expected %v", len(components), 5+otherSortColumns)
		}
	default:
		return "", errors.Fmt("invalid aggregation level %v", level)
	}
	var builder strings.Builder
	var commonClause string
	if order.ByUIPriority {
		uiPriority, err := strconv.Atoi(components[0])
		if err != nil {
			return "", errors.Fmt("invalid page token, got non-integer UIPriority: %v", components[0])
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

	builder.WriteString(` OR (` + commonClause + ` AND ModuleScheme > @afterModuleScheme)`)
	builder.WriteString(` OR (` + commonClause + ` AND ModuleScheme = @afterModuleScheme AND ModuleVariantHash > @afterModuleVariantHash)`)
	params["afterModuleName"] = components[otherSortColumns+0]
	params["afterModuleScheme"] = components[otherSortColumns+1]
	params["afterModuleVariantHash"] = components[otherSortColumns+2]
	if pb.AggregationLevel(level) == pb.AggregationLevel_MODULE {
		return builder.String(), nil
	}
	builder.WriteString(` OR (` + commonClause + ` AND ModuleScheme = @afterModuleScheme AND ModuleVariantHash = @afterModuleVariantHash AND T1CoarseName > @afterCoarseName)`)
	params["afterCoarseName"] = components[otherSortColumns+3]
	if pb.AggregationLevel(level) == pb.AggregationLevel_COARSE {
		return builder.String(), nil
	}
	builder.WriteString(` OR (` + commonClause + ` AND ModuleScheme = @afterModuleScheme AND ModuleVariantHash = @afterModuleVariantHash AND T1CoarseName = @afterCoarseName AND T2FineName > @afterFineName)`)
	params["afterFineName"] = components[otherSortColumns+4]
	return builder.String(), nil
}

func groupingColumns(level pb.AggregationLevel) string {
	switch level {
	case pb.AggregationLevel_INVOCATION:
		return ""
	case pb.AggregationLevel_MODULE:
		return "ModuleName, ModuleScheme, ModuleVariantHash"
	case pb.AggregationLevel_COARSE:
		return "ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName"
	case pb.AggregationLevel_FINE:
		return "ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName"
	default:
		// We don't support aggregating to the test case level in this method.
		// Use an RPC like QueryTestVerdicts instead.
		panic(fmt.Sprintf("logic error: invalid aggregation level %v", level))
	}
}

func orderByColumns(order Ordering, level pb.AggregationLevel) string {
	defaultOrder := groupingColumns(level)
	if order.ByUIPriority && level != pb.AggregationLevel_INVOCATION {
		return "UIPriority DESC, " + defaultOrder
	}
	return defaultOrder
}

// prefixWhereClause returns a WHERE clause that matches the given test ID prefix.
// The WHERE clause will be used to filter the TestResultsV2 and TestExonerationsV2 tables,
// and in the case of module-level aggregations or higher, the WorkUnits table.
func (q *SingleLevelQuery) prefixWhereClause(prefix *pb.TestIdentifierPrefix, params map[string]any) (predicate string, err error) {
	if prefix == nil || prefix.Level == pb.AggregationLevel_INVOCATION {
		return "TRUE", nil
	}
	// A higher level value means a finer aggregation.
	if prefix.Level > q.Level {
		return "", errors.Fmt("prefix filter: got %v, but must be equal to or coarser than the queried aggregation level %v", prefix.Level, q.Level)
	}

	var predicateBuilder strings.Builder
	predicateBuilder.WriteString("ModuleName = @prefixModuleName AND ModuleScheme = @prefixModuleScheme AND ModuleVariantHash = @prefixModuleVariantHash")
	var moduleVariantHash string
	if prefix.Id.ModuleVariant != nil {
		// Module variant was specified as a variant proto.
		moduleVariantHash = pbutil.VariantHash(prefix.Id.ModuleVariant)
	} else if prefix.Id.ModuleVariantHash != "" {
		// Module variant was specified as a hash.
		moduleVariantHash = prefix.Id.ModuleVariantHash
	} else {
		return "", errors.Fmt("prefix filter must specify Variant or VariantHash for a level of MODULE and below")
	}
	params["prefixModuleName"] = prefix.Id.ModuleName
	params["prefixModuleScheme"] = prefix.Id.ModuleScheme
	params["prefixModuleVariantHash"] = moduleVariantHash

	if prefix.Level == pb.AggregationLevel_MODULE {
		return predicateBuilder.String(), nil
	}

	predicateBuilder.WriteString(" AND T1CoarseName = @prefixCoarseName")
	params["prefixCoarseName"] = prefix.Id.CoarseName
	if prefix.Level == pb.AggregationLevel_COARSE {
		return predicateBuilder.String(), nil
	}

	predicateBuilder.WriteString(" AND T2FineName = @prefixFineName")
	params["prefixFineName"] = prefix.Id.FineName
	if prefix.Level == pb.AggregationLevel_FINE {
		return predicateBuilder.String(), nil
	}

	predicateBuilder.WriteString(" AND T3CaseName = @prefixCaseName")
	params["prefixCaseName"] = prefix.Id.CaseName
	return predicateBuilder.String(), nil
}
