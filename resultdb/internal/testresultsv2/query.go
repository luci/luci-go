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

package testresultsv2

import (
	"context"
	"fmt"
	"text/template"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// MaxTestResultsPageSize is the maximum page test results page size.
// It is selected to avoid Spanner spilling bytes to disk, which usually
// appears to occur once the results exceed 16-32 MiB, which reduces
// performance.
//
// Most callers currently assume this exceeds the maximum page size for
// verdicts of 10,000 (plus one, for pagination purposes) so that verdict
// RPCs can return the stated number of verdicts in the ideal scenario of
// one result per verdict.
//
// If this number is ever reduced, callers may need to be updated to use
// multiple pages.
const MaxTestResultsPageSize = 10_001

// MaskedFailueReasonLength is the length to which failure reasons should be masked
// for users who only have resultdb.testResults.listLimited permission.
const MaskedFailureReasonLength = 140

// MaskedFailueReasonLength is the length to which skip reasons should be masked
// for users who only have resultdb.testResults.listLimited permission.
const MaskedSkipReasonLength = 140

// Query provides methods to query test results in a root invocation.
type Query struct {
	// The root invocation to query.
	RootInvocation rootinvocations.ID
	// The test prefix filter to apply.
	TestPrefixFilter *pb.TestIdentifierPrefix
	// The specific verdicts to retrieve. If this is set, both
	// RootInvocationID and TestPrefixFilter are ignored.
	//
	// Verdicts will be returned in the same order as this list.
	// Duplicates are allowed, and will result in the same results
	// being returned multiple times. Use TestResult.Ordinal to
	// identify which verdict the result is being returned for.
	//
	// At most 10,000 IDs can be nominated (see "Values in an IN operator"):
	// https://docs.cloud.google.com/spanner/quotas#query-limits
	VerdictIDs []VerdictID
	// The access the caller has to the root invocation.
	Access permissions.RootInvocationAccess
	// The sort order of the results.
	//
	// If VerdictIDs is set, this field is ignored, and Verdicts
	// will always be returned in the same order as VerdictIDs.
	Order Ordering
	// If set, returns only the following basic fields:
	// - ID
	// - ModuleVariant
	// - StatusV2
	// - IsMasked
	// This matches what is needed for the BASIC view of TestVerdict.
	BasicFieldsOnly bool
}

// PageToken represents a token that can be used to resume a query
// after a certain point.
type PageToken struct {
	// The primary key of the last test result.
	ID ID
	// A one-based index into q.VerdictIDs that indicates the last verdict returned.
	// Only set if Query.VerdictIDs != nil.
	// Used to keep position in case of a duplicated ID(s) in Query.VerdictIDs.
	RequestOrdinal int
}

// List returns an iterator over the test results in the root invocation,
// starting at the given pageToken. Results are listed in primary key order.
//
// To start from the beginning of the table, pass a pageToken of (ID{}).
// The returned iterator will iterate over all results that match the query.
func (q *Query) List(ctx context.Context, pageToken PageToken, opts spanutil.BufferingOptions) *spanutil.Iterator[*TestResultRow, PageToken] {
	pageSizeController := spanutil.NewPageSizeController(opts)

	queryFn := func(token PageToken) (*spanutil.PageIterator[*TestResultRow], error) {
		pageSize, err := pageSizeController.NextPageSize()
		if err != nil {
			return nil, fmt.Errorf("get next page size: %w", err)
		}
		st, err := q.buildQuery(token, pageSize)
		if err != nil {
			return nil, err
		}
		it := span.Query(ctx, st)
		var buf spanutil.Buffer
		var decoder Decoder
		decodeFn := func(row *spanner.Row) (*TestResultRow, error) {
			return q.decodeRow(row, &buf, &decoder)
		}
		return spanutil.NewPageIterator(it, decodeFn, pageSize), nil
	}

	return spanutil.NewIterator(queryFn, q.pageTokenFromResult, pageToken)
}

// buildQuery returns a spanner query that returns the next page of results,
// starting at pageToken.
func (q *Query) buildQuery(pageToken PageToken, pageSize int) (spanner.Statement, error) {
	if q.Access.Level == permissions.NoAccess {
		return spanner.Statement{}, errors.New("no access to root invocation")
	}
	params := map[string]any{
		"limit":         pageSize,
		"upgradeRealms": q.Access.Realms,
	}

	paginationClause := "TRUE"
	if pageToken != (PageToken{}) {
		paginationClause = q.whereAfterPageToken(pageToken, params)
	}

	usingVerdictIDs := false
	var shardParamNames []string

	if q.VerdictIDs != nil {
		verdicts, err := SpannerVerdictIDs(q.VerdictIDs)
		if err != nil {
			return spanner.Statement{}, errors.Fmt("verdict_ids: %w", err)
		}
		params["verdictIDs"] = verdicts
		usingVerdictIDs = true
	} else if q.Order == OrderingByTestID {
		// Optimization: use UNION ALL to query each shard individually.
		// This allows Spanner to use the natural table ordering in each shard
		// to answer the query.
		shardParamNames = make([]string, 0, rootinvocations.RootInvocationShardCount)
		for i := 0; i < rootinvocations.RootInvocationShardCount; i++ {
			shardID := rootinvocations.ShardID{RootInvocationID: q.RootInvocation, ShardIndex: i}
			parameterName := fmt.Sprintf("shard_%d", i)
			params[parameterName] = shardID.RowID()
			shardParamNames = append(shardParamNames, parameterName)
		}
	} else {
		params["rootInvocationShards"] = q.RootInvocation.AllShardIDs().ToSpanner()
	}

	wherePrefixClause := "TRUE"
	if q.TestPrefixFilter != nil {
		var err error
		wherePrefixClause, err = PrefixWhereClause(q.TestPrefixFilter, params)
		if err != nil {
			return spanner.Statement{}, errors.Fmt("test_prefix_filter: %w", err)
		}
	}

	tmplInput := map[string]any{
		"PaginationClause":  paginationClause,
		"HasVerdictIDs":     usingVerdictIDs,
		"ShardParamNames":   shardParamNames,
		"WherePrefixClause": wherePrefixClause,
		"FullAccess":        q.Access.Level == permissions.FullAccess,
		"OrderingByTestID":  q.Order == OrderingByTestID,
		"BasicFieldsOnly":   q.BasicFieldsOnly,
	}

	st, err := spanutil.GenerateStatement(testResultQueryTmpl, tmplInput)
	if err != nil {
		return spanner.Statement{}, err
	}

	st.Params = spanutil.ToSpannerMap(params)
	return st, nil
}

func (q *Query) whereAfterPageToken(token PageToken, params map[string]any) string {
	var columns []spanutil.PageTokenElement
	if q.VerdictIDs != nil {
		// Order by request index.
		columns = append(columns, spanutil.PageTokenElement{
			ColumnName: "RequestIndex",
			AfterValue: int64(token.RequestOrdinal - 1),
		})
	} else {
		// Ordering by primary key or test ID.

		if q.Order == OrderingByPrimaryKey {
			// If ordering by primary key, we need to order by RootInvocationShardID first.
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
	}
	columns = append(columns, []spanutil.PageTokenElement{
		{
			ColumnName: "WorkUnitID",
			AfterValue: token.ID.WorkUnitID,
		},
		{
			ColumnName: "ResultID",
			AfterValue: token.ID.ResultID,
		},
	}...)
	return spanutil.WhereAfterClause(columns, "after", params)
}

// pageTokenFromResult returns the page token for the page starting
// immediately after the given result.
func (q *Query) pageTokenFromResult(r *TestResultRow) PageToken {
	if q.VerdictIDs != nil {
		// We are retrieving nominated verdicts. Page based on the
		// RequestOrdinal (the index into VerdictIDs) and the WorkUnitID/ResultID.
		return PageToken{
			ID: ID{
				WorkUnitID: r.ID.WorkUnitID,
				ResultID:   r.ID.ResultID,
			},
			RequestOrdinal: r.RequestOrdinal,
		}
	} else {
		// Page based on primary key.
		return PageToken{ID: r.ID}
	}
}

var testResultQueryTmpl = template.Must(template.New("").Parse(`
-- We do not use WITH clauses below as Spanner query optimizer does not optimize
-- across WITH clause/CTE boundaries and this results in suboptimal query plans. Instead
-- we use templates to include the nested SQL statements.
{{define "MaskedTestResults"}}
		-- Masked test results.
		SELECT
			* EXCEPT (ModuleVariant, SummaryHTML, Tags, TestMetadataName, TestMetadataLocationRepo, TestMetadataLocationFileName, Properties),
			-- Provide masked versions of fields to support filtering on them in the query one level up.
		{{if .FullAccess}}
			-- Directly alias the columns if full access is granted. This can provide
			-- a performance boost as predicates can be pushed down to the storage layer.
			ModuleVariant AS ModuleVariantMasked,
			SummaryHTML AS SummaryHTMLMasked,
			Tags AS TagsMasked,
			TestMetadata as TestMetadataMasked,
			TestMetadataName AS TestMetadataNameMasked,
			TestMetadataLocationRepo AS TestMetadataLocationRepoMasked,
			TestMetadataLocationFileName AS TestMetadataLocationFileNameMasked,
			Properties AS PropertiesMasked,
			FALSE AS IsMasked,
		{{else}}
			-- User has limited acccess by default.
			-- The failure reason and skipped reason fields cannot be masked in SQL as they are serialized+compressed protos.
			IF(Realm IN UNNEST(@upgradeRealms), ModuleVariant, NULL) AS ModuleVariantMasked,
			IF(Realm IN UNNEST(@upgradeRealms), SummaryHTML, NULL) AS SummaryHTMLMasked,
			IF(Realm IN UNNEST(@upgradeRealms), Tags, NULL) AS TagsMasked,
			IF(Realm IN UNNEST(@upgradeRealms), TestMetadata, NULL) AS TestMetadataMasked,
			IF(Realm IN UNNEST(@upgradeRealms), TestMetadataName, NULL) AS TestMetadataNameMasked,
			IF(Realm IN UNNEST(@upgradeRealms), TestMetadataLocationRepo, NULL) AS TestMetadataLocationRepoMasked,
			IF(Realm IN UNNEST(@upgradeRealms), TestMetadataLocationFileName, NULL) AS TestMetadataLocationFileNameMasked,
			IF(Realm IN UNNEST(@upgradeRealms), Properties, NULL) AS PropertiesMasked,
			(Realm NOT IN UNNEST(@upgradeRealms)) AS IsMasked,
		{{end}}
	{{if .HasVerdictIDs}}
		FROM UNNEST(@verdictIDs) WITH OFFSET RequestIndex
		JOIN TestResultsV2 USING (RootInvocationShardId, ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName)
		WHERE ({{.WherePrefixClause}}) AND ({{.PaginationClause}})
	{{else if .OrderingByTestID}}
		-- Union all the shards. This 16-way union has some overheads compared to a single table scan, but it allows Spanner
		-- to use the natural table ordering in each shard to answer the query.
		--
		-- Benchmark on 1.3M test result invocation, with page size of 10,000:
		-- With UNION ALL optimisation: 477ms elapsed, 549ms CPU time.
		-- Without UNION ALL optimisation: 23.7s elapsed, 19.72s CPU time.
		FROM (
			{{range $i, $shardParamName := .ShardParamNames}}
				{{if $i}} UNION ALL {{end}}
				(
					SELECT * FROM TestResultsV2
					WHERE RootInvocationShardId = @{{$shardParamName}}
				)
			{{end}}
		)
		WHERE ({{.WherePrefixClause}}) AND ({{.PaginationClause}})
	{{else}}
		-- We will be sorting in primary key order.
		-- Benchmarked cost on 1.3M test result invocation, with page size of 10,000 test results: 99ms elapsed time, 111ms CPU time.
		FROM TestResultsV2
		WHERE RootInvocationShardId IN UNNEST(@rootInvocationShards) AND ({{.WherePrefixClause}}) AND ({{.PaginationClause}})
	{{end}}
{{end}}
SELECT
	RootInvocationShardId,
	ModuleName,
	ModuleScheme,
	ModuleVariantHash,
	T1CoarseName,
	T2FineName,
	T3CaseName,
	WorkUnitId,
	ResultId,
	ModuleVariantMasked,
	StatusV2,
	IsMasked,
{{if not .BasicFieldsOnly}}
	CreateTime,
	Realm,
	SummaryHTMLMasked,
	StartTime,
	RunDurationNanos,
	TagsMasked,
	TestMetadataMasked,
	TestMetadataNameMasked,
	TestMetadataLocationRepoMasked,
	TestMetadataLocationFileNameMasked,
	FailureReason,
	PropertiesMasked,
	SkipReason,
	SkippedReason,
	FrameworkExtensions,
{{end}}
	{{if .HasVerdictIDs}}RequestIndex,{{end}}
FROM (
	{{template "MaskedTestResults" .}}
) R
ORDER BY
{{if .HasVerdictIDs}}
	-- Sort by RequestIndex, then primary key.
	RequestIndex, RootInvocationShardId, ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName, WorkUnitId, ResultId
{{else if .OrderingByTestID}}
	-- Sort by test ID.
	ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName, WorkUnitId, ResultId
{{else}}
	-- Sort by primary key.
	RootInvocationShardId, ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName, WorkUnitId, ResultId
{{end}}
LIMIT @limit
`))

func (q *Query) decodeRow(spanRow *spanner.Row, b *spanutil.Buffer, decoder *Decoder) (*TestResultRow, error) {
	var summaryHTML, testMetadata, failureReason, properties, skippedReason, frameworkExtensions []byte
	var testMetadataName, testMetadataLocationRepo, testMetadataLocationFileName spanner.NullString
	var skipReason spanner.NullInt64
	var variant []string
	var statusV2 int64
	var requestIndex int64

	row := &TestResultRow{}
	dest := []any{
		&row.ID.RootInvocationShardID,
		&row.ID.ModuleName,
		&row.ID.ModuleScheme,
		&row.ID.ModuleVariantHash,
		&row.ID.CoarseName,
		&row.ID.FineName,
		&row.ID.CaseName,
		&row.ID.WorkUnitID,
		&row.ID.ResultID,
		&variant,
		&statusV2,
		&row.IsMasked,
	}
	if !q.BasicFieldsOnly {
		dest = append(dest,
			&row.CreateTime,
			&row.Realm,
			&summaryHTML,
			&row.StartTime,
			&row.RunDurationNanos,
			&row.Tags,
			&testMetadata,
			&testMetadataName,
			&testMetadataLocationRepo,
			&testMetadataLocationFileName,
			&failureReason,
			&properties,
			&skipReason,
			&skippedReason,
			&frameworkExtensions,
		)
	}
	if q.VerdictIDs != nil {
		dest = append(dest, &requestIndex)
	}
	err := b.FromSpanner(spanRow, dest...)
	if err != nil {
		return nil, errors.Fmt("unmarshal row: %w", err)
	}
	row.StatusV2 = pb.TestResult_Status(statusV2)
	row.SkipReason = DecodeSkipReason(skipReason)

	// For masked test verdicts, the variant is nil. This allows distinguishing
	// a masked variant from an empty variant.
	if variant != nil {
		row.ModuleVariant, err = pbutil.VariantFromStrings(variant)
		if err != nil {
			return nil, errors.Fmt("module variant: %w", err)
		}
	}

	if !q.BasicFieldsOnly {
		if row.SummaryHTML, err = decoder.DecompressText(summaryHTML); err != nil {
			return nil, errors.Fmt("decompress SummaryHTML: %w", err)
		}

		if row.TestMetadata, err = decoder.DecodeTestMetadata(testMetadata, testMetadataName, testMetadataLocationRepo, testMetadataLocationFileName); err != nil {
			return nil, errors.Fmt("decode TestMetadata: %w", err)
		}

		if row.FailureReason, err = decoder.DecodeFailureReason(failureReason); err != nil {
			return nil, errors.Fmt("decode FailureReason: %w", err)
		}

		if row.Properties, err = decoder.DecodeProperties(properties); err != nil {
			return nil, errors.Fmt("decode Properties: %w", err)
		}

		if row.SkippedReason, err = decoder.DecodeSkippedReason(skippedReason); err != nil {
			return nil, errors.Fmt("decode SkippedReason: %w", err)
		}

		if row.FrameworkExtensions, err = decoder.DecodeFrameworkExtensions(frameworkExtensions); err != nil {
			return nil, errors.Fmt("decode FrameworkExtensions: %w", err)
		}
	}

	if row.IsMasked {
		// Although it is the same on-the-wire, for testing purposes, prefer nil tags
		// over empty slice when the tags have been masked.
		row.Tags = nil

		// The following cannot be achieved inside the query because the failure reason and
		// skipped reason are stored in serialized protobufs.

		// Truncate FailureReason.
		if row.FailureReason != nil {
			row.FailureReason.PrimaryErrorMessage = pbutil.TruncateString(row.FailureReason.PrimaryErrorMessage, MaskedFailureReasonLength)
			for _, e := range row.FailureReason.Errors {
				e.Message = pbutil.TruncateString(e.Message, MaskedFailureReasonLength)
				e.Trace = ""
			}
		}
		// Truncate SkippedReason.
		if row.SkippedReason != nil {
			row.SkippedReason.ReasonMessage = pbutil.TruncateString(row.SkippedReason.ReasonMessage, MaskedSkipReasonLength)
			row.SkippedReason.Trace = ""
		}
	}
	if q.VerdictIDs != nil {
		// Convert from zero-based index to one-based index, so that we can detect
		// when RequestOrdinal is unset as opposed to referencing the first verdict ID.
		row.RequestOrdinal = int(requestIndex) + 1
	}
	return row, nil
}
