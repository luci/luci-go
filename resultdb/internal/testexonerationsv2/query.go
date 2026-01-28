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

package testexonerationsv2

import (
	"context"
	"fmt"
	"text/template"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testresultsv2"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// MaxTestExonerationsPageSize is the maximum page test results page size.
// It is selected to avoid Spanner spilling bytes to disk, which usually
// appears to occur once the results exceed 16-32 MiB.
//
// Currently follows the test result page size, as exonerations are usually
// equal or smaller in size.
const MaxTestExonerationsPageSize = testresultsv2.MaxTestResultsPageSize

// Query provides methods to query test exonerations in a root invocation.
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
	VerdictIDs []testresultsv2.VerdictID
	// The access the caller has to the root invocation.
	Access permissions.RootInvocationAccess
	// The ordering to use when listing test exonerations.
	//
	// If VerdictIDs is set, this field is ignored, and Verdicts
	// will always be returned in the same order as VerdictIDs.
	Order testresultsv2.Ordering
}

// PageToken represents a token that can be used to resume a query
// after a certain point.
type PageToken struct {
	// The primary key of the last test exoneration.
	ID ID
	// A one-based index into q.VerdictIDs that indicates the last verdict returned.
	// Only set if Query.VerdictIDs != nil.
	// Used to keep position in case of a duplicated ID in Query.VerdictIDs.
	RequestOrdinal int
}

// List returns an iterator over the test exonerations in the root invocation,
// starting at the given pageToken. Results are listed in primary key order.
//
// To start from the beginning of the table, pass a pageToken of (ID{}).
// The returned iterator will iterate over all results that match the query.
func (q *Query) List(ctx context.Context, pageToken PageToken, opts spanutil.BufferingOptions) *spanutil.Iterator[*TestExonerationRow, PageToken] {
	pageSizeController := spanutil.NewPageSizeController(opts)

	queryFn := func(token PageToken) (*spanutil.PageIterator[*TestExonerationRow], error) {
		pageSize, err := pageSizeController.NextPageSize()
		if err != nil {
			return nil, fmt.Errorf("get next page size: %w", err)
		}
		st, err := q.buildQuery(token, pageSize)
		if err != nil {
			return nil, err
		}
		logging.Debugf(ctx, "Query test exonerations with page size %v.", pageSize)
		it := span.Query(ctx, st)
		var buf spanutil.Buffer
		decodeFn := func(row *spanner.Row) (*TestExonerationRow, error) {
			return q.decodeRow(row, &buf)
		}
		return spanutil.NewPageIterator(it, decodeFn, pageSize), nil
	}

	return spanutil.NewIterator(queryFn, q.pageTokenFromExoneration, pageToken)
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

	var usingVerdictIDs bool
	var shardParamNames []string
	if q.VerdictIDs != nil {
		verdicts, err := testresultsv2.SpannerVerdictIDs(q.VerdictIDs)
		if err != nil {
			return spanner.Statement{}, errors.Fmt("verdict_ids: %w", err)
		}
		params["verdictIDs"] = verdicts
		usingVerdictIDs = true
	} else if q.Order == testresultsv2.OrderingByTestID {
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
		wherePrefixClause, err = testresultsv2.PrefixWhereClause(q.TestPrefixFilter, params)
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
		"OrderingByTestID":  q.Order == testresultsv2.OrderingByTestID,
	}

	st, err := spanutil.GenerateStatement(testExonerationQueryTmpl, tmplInput)
	if err != nil {
		return spanner.Statement{}, err
	}

	st.Params = params
	return st, nil
}

func (q *Query) whereAfterPageToken(token PageToken, params map[string]any) string {
	var columns []spanutil.PageTokenElement
	if q.VerdictIDs != nil {
		columns = append(columns, spanutil.PageTokenElement{
			ColumnName: "RequestIndex",
			AfterValue: int64(token.RequestOrdinal - 1),
		})
	} else {
		if q.Order == testresultsv2.OrderingByPrimaryKey {
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
		}, {
			ColumnName: "ExonerationID",
			AfterValue: token.ID.ExonerationID,
		},
	}...)
	return spanutil.WhereAfterClause(columns, "after", params)
}

// pageTokenFromExoneration returns the page token for the page starting
// immediately after the given test exoneration.
func (q *Query) pageTokenFromExoneration(r *TestExonerationRow) PageToken {
	if q.VerdictIDs != nil {
		// We are retrieving nominated verdicts. Page based on the
		// RequestOrdinal (the index into VerdictIDs) and the WorkUnitID/ExonerationID.
		return PageToken{
			ID: ID{
				WorkUnitID:    r.ID.WorkUnitID,
				ExonerationID: r.ID.ExonerationID,
			},
			RequestOrdinal: r.RequestOrdinal,
		}
	} else {
		// Page based on primary key.
		return PageToken{ID: r.ID}
	}
}

var testExonerationQueryTmpl = template.Must(template.New("").Parse(`
-- We do not use WITH clauses below as Spanner query optimizer does not optimize
-- across WITH clause/CTE boundaries and this results in suboptimal query plans. Instead
-- we use templates to include the nested SQL statements.
{{define "MaskedTestExonerations"}}
		-- Masked test exonerations.
		SELECT
			* EXCEPT (ModuleVariant),
		{{if .FullAccess}}
			ModuleVariant AS ModuleVariantMasked,
			FALSE AS IsMasked,
		{{else}}
			IF(Realm IN UNNEST(@upgradeRealms), ModuleVariant, NULL) AS ModuleVariantMasked,
			(Realm NOT IN UNNEST(@upgradeRealms)) AS IsMasked,
		{{end}}
	{{if .HasVerdictIDs}}
		FROM UNNEST(@verdictIDs) WITH OFFSET RequestIndex
		JOIN TestExonerationsV2 USING (RootInvocationShardId, ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName)
		WHERE {{.WherePrefixClause}} AND {{.PaginationClause}}
	{{else if .OrderingByTestID}}
		-- We will be sorting in test ID order. This UNION ALL construction allows Spanner to use the natural table ordering in each shard.
		FROM (
			{{range $i, $shardParamName := .ShardParamNames}}
				{{if $i}} UNION ALL {{end}}
				(
					SELECT * FROM TestExonerationsV2
					WHERE RootInvocationShardId = @{{$shardParamName}}
				)
			{{end}}
		)
		WHERE {{.WherePrefixClause}} AND {{.PaginationClause}}
	{{else}}
		-- We will be sorting in primary key order.
		FROM TestExonerationsV2
		WHERE RootInvocationShardId IN UNNEST(@rootInvocationShards) AND {{.WherePrefixClause}} AND {{.PaginationClause}}
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
	ExonerationId,
	ModuleVariantMasked,
	CreateTime,
	Realm,
	ExplanationHTML,
	Reason,
	IsMasked,
	{{if eq .HasVerdictIDs true}}RequestIndex,{{end}}
FROM (
	{{template "MaskedTestExonerations" .}}
) E
ORDER BY
{{if .HasVerdictIDs}}
	-- Sort by RequestIndex, then primary key.
	RequestIndex, RootInvocationShardId, ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName, WorkUnitId, ExonerationId
{{else if .OrderingByTestID}}
	-- Sort by test ID.
	ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName, WorkUnitId, ExonerationId
{{else}}
	-- Sort by primary key.
	RootInvocationShardId, ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName, WorkUnitId, ExonerationId
{{end}}
LIMIT @limit
`))

func (q *Query) decodeRow(spanRow *spanner.Row, b *spanutil.Buffer) (*TestExonerationRow, error) {
	row := &TestExonerationRow{}
	var explanationHTML spanutil.Compressed
	var moduleVariant []string
	var requestIndex int64
	dest := []any{
		&row.ID.RootInvocationShardID,
		&row.ID.ModuleName,
		&row.ID.ModuleScheme,
		&row.ID.ModuleVariantHash,
		&row.ID.CoarseName,
		&row.ID.FineName,
		&row.ID.CaseName,
		&row.ID.WorkUnitID,
		&row.ID.ExonerationID,
		&moduleVariant,
		&row.CreateTime,
		&row.Realm,
		&explanationHTML,
		&row.Reason,
		&row.IsMasked,
	}
	if q.VerdictIDs != nil {
		dest = append(dest, &requestIndex)
	}

	err := b.FromSpanner(spanRow, dest...)
	if err != nil {
		return nil, errors.Fmt("unmarshal row: %w", err)
	}

	// For masked test verdicts, the variant is nil. This allows distinguishing
	// a masked variant from an empty variant.
	if moduleVariant != nil {
		row.ModuleVariant, err = pbutil.VariantFromStrings(moduleVariant)
		if err != nil {
			return nil, errors.Fmt("module variant: %w", err)
		}
	}

	row.ExplanationHTML = string(explanationHTML)

	if q.VerdictIDs != nil {
		// Convert from zero-based index to one-based index, so that we can detect
		// when RequestOrdinal is unset as opposed to referencing the first verdict ID.
		row.RequestOrdinal = int(requestIndex) + 1
	}
	return row, nil
}
