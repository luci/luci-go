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
	// The specific verdicts to filter to. If this is set, both
	// RootInvocationID and TestPrefixFilter are ignored.
	//
	// This list is treated as a set; the verdicts will not necessarily
	// be returned in this order.
	//
	// At most 10,000 IDs can be nominated (see "Values in an IN operator"):
	// https://docs.cloud.google.com/spanner/quotas#query-limits
	VerdictIDs []VerdictID
	// The access the caller has to the root invocation.
	Access permissions.RootInvocationAccess
}

// List returns an iterator over the test results in the root invocation,
// starting at the given pageToken. Results are listed in primary key order.
//
// To start from the beginning of the table, pass a pageToken of (ID{}).
// The returned iterator will iterate over all results that match the query.
func (q *Query) List(ctx context.Context, pageToken ID, opts spanutil.BufferingOptions) *spanutil.Iterator[*TestResultRow, ID] {
	pageSizeController := spanutil.NewPageSizeController(opts)

	queryFn := func(token ID) (*spanutil.PageIterator[*TestResultRow], error) {
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
			return decodeRow(row, &buf, &decoder)
		}
		return spanutil.NewPageIterator(it, decodeFn, pageSize), nil
	}
	idAccessor := func(r *TestResultRow) ID { return r.ID }

	return spanutil.NewIterator(queryFn, idAccessor, pageToken)
}

// buildQuery returns a spanner query that returns the next page of results,
// starting at pageToken.
func (q *Query) buildQuery(pageToken ID, pageSize int) (spanner.Statement, error) {
	if q.Access.Level == permissions.NoAccess {
		return spanner.Statement{}, errors.New("no access to root invocation")
	}
	params := map[string]any{
		"limit":         pageSize,
		"upgradeRealms": q.Access.Realms,
	}

	paginationClause := "TRUE"
	if pageToken != (ID{}) {
		paginationClause = q.whereAfterPageToken(pageToken, params)
	}

	var whereClause string
	if len(q.VerdictIDs) > 0 {
		clause, err := NominatedVerdictsClause(q.VerdictIDs, params)
		if err != nil {
			return spanner.Statement{}, errors.Fmt("verdict_ids: %w", err)
		}
		whereClause = "(" + clause + ")"
	} else {
		whereClause = "RootInvocationShardId IN UNNEST(@rootInvocationShards)"
		params["rootInvocationShards"] = q.RootInvocation.AllShardIDs().ToSpanner()
		if q.TestPrefixFilter != nil {
			clause, err := PrefixWhereClause(q.TestPrefixFilter, params)
			if err != nil {
				return spanner.Statement{}, errors.Fmt("test_prefix_filter: %w", err)
			}
			whereClause += " AND (" + clause + ")"
		}
	}

	tmplInput := map[string]any{
		"PaginationClause": paginationClause,
		"WhereClause":      whereClause,
		"FullAccess":       q.Access.Level == permissions.FullAccess,
	}

	st, err := spanutil.GenerateStatement(testResultQueryTmpl, tmplInput)
	if err != nil {
		return spanner.Statement{}, err
	}

	st.Params = spanutil.ToSpannerMap(params)
	return st, nil
}

func (q *Query) whereAfterPageToken(token ID, params map[string]any) string {
	columns := []spanutil.PageTokenElement{
		{
			ColumnName: "RootInvocationShardID",
			AfterValue: token.RootInvocationShardID.RowID(),
		},
		{
			ColumnName: "ModuleName",
			AfterValue: token.ModuleName,
		},
		{
			ColumnName: "ModuleScheme",
			AfterValue: token.ModuleScheme,
		},
		{
			ColumnName: "ModuleVariantHash",
			AfterValue: token.ModuleVariantHash,
		},
		{
			ColumnName: "T1CoarseName",
			AfterValue: token.CoarseName,
		},
		{
			ColumnName: "T2FineName",
			AfterValue: token.FineName,
		},
		{
			ColumnName: "T3CaseName",
			AfterValue: token.CaseName,
		},
		{
			ColumnName: "WorkUnitID",
			AfterValue: token.WorkUnitID,
		},
		{
			ColumnName: "ResultID",
			AfterValue: token.ResultID,
		},
	}
	return spanutil.WhereAfterClause(columns, "after", params)
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
		{{if eq .FullAccess true}}
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
	FROM TestResultsV2
	WHERE {{.WhereClause}}
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
	CreateTime,
	Realm,
	StatusV2,
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
	IsMasked,
FROM (
	{{template "MaskedTestResults" .}}
) R
WHERE {{.PaginationClause}}
ORDER BY RootInvocationShardId, ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName, WorkUnitId, ResultId
LIMIT @limit
`))

func decodeRow(spanRow *spanner.Row, b *spanutil.Buffer, decoder *Decoder) (*TestResultRow, error) {
	var summaryHTML, testMetadata, failureReason, properties, skippedReason, frameworkExtensions []byte
	var testMetadataName, testMetadataLocationRepo, testMetadataLocationFileName spanner.NullString
	var skipReason spanner.NullInt64
	var variant []string
	var statusV2 int64

	row := &TestResultRow{}
	err := b.FromSpanner(spanRow,
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
		&row.CreateTime,
		&row.Realm,
		&statusV2,
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
		&row.IsMasked,
	)
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
	return row, nil
}
