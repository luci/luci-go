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
	"text/template"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/pagination"
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
	// Shared decoding buffer to avoid a new memory allocation for each decompression.
	decoder testresultsv2.Decoder
}

// Fetch fetches a page of test verdicts.
func (q *Query) Fetch(ctx context.Context, pageToken string) ([]*pb.TestVerdict, string, error) {
	var results []*pb.TestVerdict
	nextPageToken, err := q.Run(ctx, pageToken, func(tv *pb.TestVerdict) error {
		results = append(results, tv)
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return results, nextPageToken, nil
}

// Run queries test verdicts for a given root invocation, calling the given row callback
// for each row. To stop iteration early, the callback should return iterator.Done.
func (q *Query) Run(ctx context.Context, pageToken string, rowCallback func(*pb.TestVerdict) error) (string, error) {
	if q.PageSize <= 0 {
		return "", errors.New("page size must be positive")
	}

	st, err := q.buildQuery(pageToken)
	if err != nil {
		return "", err
	}

	var lastTV *pb.TestVerdict
	var b spanutil.Buffer
	rowsSeen := 0

	err = span.Query(ctx, st).Do(func(row *spanner.Row) error {
		tv := &pb.TestVerdict{
			TestIdStructured: &pb.TestIdentifier{},
		}
		var variant []string
		var results []*tvTestResult
		var exonerations []*tvExoneration
		var testMetadata []byte
		var testMetadataName spanner.NullString
		var testMetadataLocationRepo spanner.NullString
		var testMetadataLocationFileName spanner.NullString

		err := b.FromSpanner(row,
			&tv.TestIdStructured.ModuleName,
			&tv.TestIdStructured.ModuleScheme,
			&tv.TestIdStructured.ModuleVariantHash,
			&tv.TestIdStructured.CoarseName,
			&tv.TestIdStructured.FineName,
			&tv.TestIdStructured.CaseName,
			&variant,
			&tv.Status,
			&tv.StatusOverride,
			&results,
			&exonerations,
			&testMetadata,
			&testMetadataName,
			&testMetadataLocationRepo,
			&testMetadataLocationFileName,
		)
		if err != nil {
			return err
		}

		// For masked test verdicts, the variant is nil. This allows distinguishing
		// a masked variant from an empty variant.
		if variant != nil {
			tv.TestIdStructured.ModuleVariant, err = pbutil.VariantFromStrings(variant)
			if err != nil {
				return errors.Fmt("module variant: %w", err)
			}
		}

		// Populate test ID (flat).
		tv.TestId = pbutil.EncodeTestID(pbutil.ExtractBaseTestIdentifier(tv.TestIdStructured))

		// Populate results.
		tv.Results, err = q.toResults(results, tv.TestId)
		if err != nil {
			return errors.Fmt("results: %w", err)
		}

		// Populate exonerations.
		tv.Exonerations, err = q.toExonerations(exonerations, tv.TestId)
		if err != nil {
			return errors.Fmt("exonerations: %w", err)
		}

		// Populate test metadata.
		// This is stored over a few fields so we need to reassemble it.
		tv.TestMetadata, err = q.decoder.DecodeTestMetadata(testMetadata, testMetadataName, testMetadataLocationRepo, testMetadataLocationFileName)
		if err != nil {
			return errors.Fmt("test metadata: %w", err)
		}

		lastTV = tv
		rowsSeen++
		return rowCallback(tv)
	})
	if err != nil && !errors.Is(err, iterator.Done) {
		return "", err
	}
	// We had fewer verdicts than the page size and we didn't stop iteration early.
	if rowsSeen < q.PageSize && err == nil {
		// There are no more verdicts to query.
		return "", nil
	}

	var nextPageToken string
	if lastTV != nil {
		nextPageToken = q.makePageToken(lastTV)
	} else {
		// If there are no more verdicts, the page token is empty to signal end of iteration.
		nextPageToken = ""
	}
	return nextPageToken, nil
}

func (q *Query) toExonerations(es []*tvExoneration, testID string) ([]*pb.TestExoneration, error) {
	results := make([]*pb.TestExoneration, 0, len(es))
	for _, e := range es {
		result, err := q.toExoneration(*e, testID)
		if err != nil {
			return nil, fmt.Errorf("exoneration %q/%q: %w", e.WorkUnitID, e.ExonerationID, err)
		}
		results = append(results, result)
	}
	return results, nil
}

func (q *Query) toExoneration(e tvExoneration, testID string) (*pb.TestExoneration, error) {
	result := &pb.TestExoneration{
		Name:          pbutil.TestExonerationName(string(q.RootInvocationID), e.WorkUnitID, testID, e.ExonerationID),
		ExonerationId: e.ExonerationID,
	}

	// TestIdStructured, TestId, Variant is elided on the child TestExoneration
	// as it is already set on the parent TestVerdict record.

	var err error
	result.ExplanationHtml, err = q.decoder.DecompressText(e.ExplanationHTML)
	if err != nil {
		return nil, fmt.Errorf("explanation html: %w", err)
	}
	result.Reason = pb.ExonerationReason(e.Reason)
	return result, nil
}

func (q *Query) toResults(rs []*tvTestResult, testID string) ([]*pb.TestResult, error) {
	results := make([]*pb.TestResult, 0, len(rs))
	for _, tr := range rs {
		result, err := q.toResult(*tr, testID)
		if err != nil {
			return nil, fmt.Errorf("result %q/%q: %w", tr.WorkUnitID, tr.ResultID, err)
		}
		results = append(results, result)
	}
	return results, nil
}

func (q *Query) toResult(r tvTestResult, testID string) (*pb.TestResult, error) {
	result := &pb.TestResult{
		Name:     pbutil.TestResultName(string(q.RootInvocationID), r.WorkUnitID, testID, r.ResultID),
		ResultId: r.ResultID,
		StatusV2: pb.TestResult_Status(r.StatusV2),
	}

	// TestIdStructured, TestId, Variant is elided on the child TestResult
	// as it is already set on the parent TestVerdict record.

	var err error
	result.SummaryHtml, err = q.decoder.DecompressText(r.SummaryHTML)
	if err != nil {
		return nil, errors.Fmt("summary html: %w", err)
	}

	if r.StartTime.Valid {
		result.StartTime = pbutil.MustTimestampProto(r.StartTime.Time)
	}
	result.Duration = testresultsv2.ToProtoDuration(r.RunDurationNanos)

	// Populate Tags.
	result.Tags = make([]*pb.StringPair, len(r.Tags))
	for i, p := range r.Tags {
		result.Tags[i] = pbutil.StringPairFromStringUnvalidated(p)
	}
	result.FailureReason, err = q.decoder.DecodeFailureReason(r.FailureReason)
	if err != nil {
		return nil, errors.Fmt("failure reason: %w", err)
	}
	result.Properties, err = q.decoder.DecodeProperties(r.Properties)
	if err != nil {
		return nil, errors.Fmt("properties: %w", err)
	}
	result.SkipReason = testresultsv2.DecodeSkipReason(r.SkipReason)
	result.SkippedReason, err = q.decoder.DecodeSkippedReason(r.SkippedReason)
	if err != nil {
		return nil, errors.Fmt("skipped reason: %w", err)
	}
	result.FrameworkExtensions, err = q.decoder.DecodeFrameworkExtensions(r.FrameworkExtensions)
	if err != nil {
		return nil, errors.Fmt("framework extensions: %w", err)
	}
	// Populate status v1 fields from status v2.
	result.Status, result.Expected = pbutil.TestStatusV1FromV2(result.StatusV2, result.FailureReason.GetKind(), result.FrameworkExtensions.GetWebTest())
	return result, nil
}

func (q *Query) buildQuery(pageToken string) (spanner.Statement, error) {
	params := map[string]any{
		"shards": q.RootInvocationID.AllShardIDs().ToSpanner(),
		"limit":  q.PageSize,
	}

	paginationClause := "TRUE"
	if pageToken != "" {
		var err error
		paginationClause, err = q.whereAfterPageToken(pageToken, params)
		if err != nil {
			return spanner.Statement{}, appstatus.Attachf(err, codes.InvalidArgument, "page_token: invalid page token")
		}
	}

	tmplInput := map[string]any{
		"PaginationClause":        paginationClause,
		"ResultPassed":            int64(pb.TestResult_PASSED),
		"ResultFailed":            int64(pb.TestResult_FAILED),
		"ResultSkipped":           int64(pb.TestResult_SKIPPED),
		"ResultExecutionErrored":  int64(pb.TestResult_EXECUTION_ERRORED),
		"ResultPrecluded":         int64(pb.TestResult_PRECLUDED),
		"VerdictPassed":           int64(pb.TestVerdict_PASSED),
		"VerdictFailed":           int64(pb.TestVerdict_FAILED),
		"VerdictSkipped":          int64(pb.TestVerdict_SKIPPED),
		"VerdictExecutionErrored": int64(pb.TestVerdict_EXECUTION_ERRORED),
		"VerdictPrecluded":        int64(pb.TestVerdict_PRECLUDED),
		"VerdictFlaky":            int64(pb.TestVerdict_FLAKY),
		"VerdictExonerated":       int64(pb.TestVerdict_EXONERATED),
		"VerdictNotOverridden":    int64(pb.TestVerdict_NOT_OVERRIDDEN),
	}

	st, err := spanutil.GenerateStatement(queryTmpl, tmplInput)
	if err != nil {
		return spanner.Statement{}, err
	}
	st.Params = params
	return st, nil
}

func (q *Query) makePageToken(last *pb.TestVerdict) string {
	var parts []string
	parts = append(parts, last.TestIdStructured.ModuleName)
	parts = append(parts, last.TestIdStructured.ModuleScheme)
	parts = append(parts, last.TestIdStructured.ModuleVariantHash)
	parts = append(parts, last.TestIdStructured.CoarseName)
	parts = append(parts, last.TestIdStructured.FineName)
	parts = append(parts, last.TestIdStructured.CaseName)
	return pagination.Token(parts...)
}

func (q *Query) whereAfterPageToken(token string, params map[string]any) (string, error) {
	parts, err := pagination.ParseToken(token)
	if err != nil {
		return "", errors.Fmt("invalid page token: %s", err)
	}
	const testIDKeyParts = 6
	if len(parts) != testIDKeyParts {
		return "", errors.Fmt("expected %v components, got %d", testIDKeyParts, len(parts))
	}

	var builder strings.Builder
	params["afterModuleName"] = parts[0]
	params["afterModuleScheme"] = parts[1]
	params["afterModuleVariantHash"] = parts[2]
	params["afterCoarseName"] = parts[3]
	params["afterFineName"] = parts[4]
	params["afterCaseName"] = parts[5]

	builder.WriteString(`ModuleName > @afterModuleName`)
	builder.WriteString(` OR (ModuleName = @afterModuleName AND ModuleScheme > @afterModuleScheme)`)
	builder.WriteString(` OR (ModuleName = @afterModuleName AND ModuleScheme = @afterModuleScheme AND ModuleVariantHash > @afterModuleVariantHash)`)
	builder.WriteString(` OR (ModuleName = @afterModuleName AND ModuleScheme = @afterModuleScheme AND ModuleVariantHash = @afterModuleVariantHash AND T1CoarseName > @afterCoarseName)`)
	builder.WriteString(` OR (ModuleName = @afterModuleName AND ModuleScheme = @afterModuleScheme AND ModuleVariantHash = @afterModuleVariantHash AND T1CoarseName = @afterCoarseName AND T2FineName > @afterFineName)`)
	builder.WriteString(` OR (ModuleName = @afterModuleName AND ModuleScheme = @afterModuleScheme AND ModuleVariantHash = @afterModuleVariantHash AND T1CoarseName = @afterCoarseName AND T2FineName = @afterFineName AND T3CaseName > @afterCaseName)`)
	return builder.String(), nil
}

// tvTestResult describes a nested test result record retrieved from Spanner.
type tvTestResult struct {
	WorkUnitID          string
	ResultID            string
	StatusV2            int64  // pb.TestResult_Status
	SummaryHTML         []byte // zstd-compressed string
	StartTime           spanner.NullTime
	RunDurationNanos    spanner.NullInt64
	Tags                []string          // key-vale pairs stored as ["key1:value1", "key2:value2"]
	FailureReason       []byte            // zstd-compressed luci.resultdb.v1.FailureReason
	Properties          []byte            // zstd-compressed google.protobuf.Struct
	SkipReason          spanner.NullInt64 // pb.SkipReason
	SkippedReason       []byte            // zstd-compressed SkippedReason
	FrameworkExtensions []byte            // zstd-compressed FrameworkExtensions
}

// tvTestResult describes a nested test exoneration record retrieved from Spanner.
type tvExoneration struct {
	WorkUnitID      string
	ExonerationID   string
	CreateTime      time.Time
	ExplanationHTML []byte // zstd-compressed string
	Reason          int64  // pb.ExonerationReason
}

var queryTmpl = template.Must(template.New("").Parse(`
-- We do not use WITH clauses below as Spanner query optimizer does not optimize
-- across WITH clause/CTE boundaries and this results in suboptimal query plans. Instead
-- we use templates to include the nested SQL statements.
{{define "TestExonerations"}}
			-- Test Exonerations.
			SELECT
				RootInvocationShardId, ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName,
				ARRAY_AGG(STRUCT(
					WorkUnitId,
					ExonerationId,
					CreateTime,
					ExplanationHTML,
					Reason
				)) AS Exonerations,
				TRUE AS HasExonerations
				FROM TestExonerationsV2
				WHERE RootInvocationShardId IN UNNEST(@shards)
			-- A given full test identifier will only appear in one shard, so we include RootInvocationShardId
			-- in the group by key to improve performance (this allows use of streaming aggregates).
			GROUP BY RootInvocationShardId, ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName
{{end}}
{{define "VerdictsByShard"}}
		-- Verdicts by shard.
		SELECT
			TR.RootInvocationShardId,
			TR.ModuleName, TR.ModuleScheme, TR.ModuleVariantHash, TR.T1CoarseName, TR.T2FineName, TR.T3CaseName,
			ANY_VALUE(TR.ModuleVariant) AS ModuleVariant,
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
			ARRAY_AGG(STRUCT(
				WorkUnitId,
				ResultId,
				StatusV2,
				SummaryHTML,
				StartTime,
				RunDurationNanos,
				Tags,
				FailureReason,
				Properties,
				SkipReason,
				SkippedReason,
				FrameworkExtensions
			)) AS Results,
			ANY_VALUE(E.Exonerations) AS Exonerations,
			COALESCE(ANY_VALUE(E.HasExonerations), FALSE) AS HasExonerations,
			ANY_VALUE(TR.TestMetadata) AS TestMetadata,
			ANY_VALUE(TR.TestMetadataName) AS TestMetadataName,
			ANY_VALUE(TR.TestMetadataLocationRepo) AS TestMetadataLocationRepo,
			ANY_VALUE(TR.TestMetadataLocationFileName) AS TestMetadataLocationFileName,
		FROM TestResultsV2 TR
		LEFT JOIN@{JOIN_METHOD=MERGE_JOIN} ({{template "TestExonerations" .}}
		) E USING (RootInvocationShardId, ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName)
		-- A given full test identifier will only appear in one shard, so we include RootInvocationShardId
		-- in the join key and group by key to improve performance.
		-- This allows use of a merge join and streaming aggregate.
		WHERE TR.RootInvocationShardId IN UNNEST(@shards)
		GROUP BY RootInvocationShardId, ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName
{{end}}
{{define "Verdicts"}}
	-- Verdicts.
	SELECT
		ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName,
		ModuleVariant,
		Status,
		(CASE
			WHEN HasExonerations AND (Status = {{.VerdictFailed}} OR Status = {{.VerdictExecutionErrored}} OR Status = {{.VerdictPrecluded}} OR Status = {{.VerdictFlaky}}) THEN {{.VerdictExonerated}}
			ELSE {{.VerdictNotOverridden}}
		END) AS StatusOverride,
		ARRAY(
			SELECT R
			FROM UNNEST(Results) R
			-- Order results by status first (putting failed first, then passed, skipped, execution errored, precluded).
			-- Use a secondary sort order to keep the query results deterministic.
			ORDER BY IF(StatusV2 = {{.ResultFailed}}, 0, StatusV2) ASC, WorkUnitId, ResultId
		) AS Results,
		-- To avoid confusion, exonerations are only returned if the status is one that can be exonerated.
		IF(Status = {{.VerdictFailed}} OR Status = {{.VerdictExecutionErrored}} OR Status = {{.VerdictPrecluded}} OR Status = {{.VerdictFlaky}},
			ARRAY(
				SELECT E
				FROM UNNEST(Exonerations) E
				-- Use a sort to keep the query results deterministic.
				ORDER BY WorkUnitId, ExonerationId
			),
			NULL) as Exonerations,
		TestMetadata,
		TestMetadataName,
		TestMetadataLocationRepo,
		TestMetadataLocationFileName
	FROM (
		{{template "VerdictsByShard" .}}
	) AS Verdicts
{{end}}
SELECT
	ModuleName,
	ModuleScheme,
	ModuleVariantHash,
	T1CoarseName,
	T2FineName,
	T3CaseName,
	ModuleVariant,
	Status,
	StatusOverride,
	Results,
	Exonerations,
	TestMetadata,
	TestMetadataName,
	TestMetadataLocationRepo,
	TestMetadataLocationFileName
FROM (
	{{template "Verdicts" .}}
)
WHERE {{.PaginationClause}}
ORDER BY ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName
LIMIT @limit
`))
