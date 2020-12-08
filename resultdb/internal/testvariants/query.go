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

package testvariants

import (
	"bytes"
	"context"
	"text/template"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/trace"

	"go.chromium.org/luci/resultdb/internal/invocations"
	uipb "go.chromium.org/luci/resultdb/internal/proto/ui"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// Query specifies test variants to fetch.
type Query struct {
	InvocationIDs invocations.IDSet
	PageSize      int    // must be positive
	summaryBuffer []byte // Buffer for decompressing SummaryHTML
}

// tvResult matches the result STRUCT of a test variant from the query.
type tvResult struct {
	InvocationId    string
	ResultId        string
	IsUnexpected    spanner.NullBool
	Status          int64
	StartTime       spanner.NullTime
	RunDurationUsec spanner.NullInt64
	SummaryHTML     []byte
	Tags            []string
}

func (q *Query) decompressSummaryHTML(r *tvResult) (summaryHtml string, err error) {
	q.summaryBuffer, err = spanutil.Decompress(r.SummaryHTML, q.summaryBuffer)
	if err != nil {
		return
	}
	return string(q.summaryBuffer), nil
}

func (q *Query) toTestResultProto(r *tvResult, testId string) (*pb.TestResult, error) {
	tr := &pb.TestResult{
		Name:     pbutil.TestResultName(string(invocations.IDFromRowID(r.InvocationId)), testId, r.ResultId),
		ResultId: r.ResultId,
		Status:   pb.TestStatus(r.Status),
	}
	if r.StartTime.Valid {
		tr.StartTime = pbutil.MustTimestampProto(r.StartTime.Time)
	}
	testresults.PopulateExpectedField(tr, r.IsUnexpected)
	testresults.PopulateDurationField(tr, r.RunDurationUsec)

	// Decompress SummaryHtml.
	var err error
	if tr.SummaryHtml, err = q.decompressSummaryHTML(r); err != nil {
		return nil, err
	}

	// Populate Tags.
	tr.Tags = make([]*pb.StringPair, len(r.Tags))
	for i, p := range r.Tags {
		if tr.Tags[i], err = pbutil.StringPairFromString(p); err != nil {
			return nil, err
		}
	}

	return tr, nil
}

func (q *Query) run(ctx context.Context, f func(*uipb.TestVariant) error) (err error) {
	ctx, ts := trace.StartSpan(ctx, "testvariants.Query.run")
	ts.Attribute("cr.dev/invocations", len(q.InvocationIDs))
	defer func() { ts.End(err) }()

	if q.PageSize < 0 {
		panic("PageSize < 0")
	}

	var sql bytes.Buffer
	if err := queryTmpl.Execute(&sql, nil); err != nil {
		return err
	}
	st := spanner.NewStatement(sql.String())
	st.Params = map[string]interface{}{
		"invIDs": q.InvocationIDs,
		"limit":  q.PageSize,
	}

	var b spanutil.Buffer
	var expBytes []byte
	return spanutil.Query(ctx, st, func(row *spanner.Row) error {
		tv := &uipb.TestVariant{}
		var tvStatus int64
		var results []*tvResult
		var exoExplanationHtmls [][]byte
		if err := b.FromSpanner(row, &tv.TestId, &tv.VariantHash, &tv.Variant, &tvStatus, &results, &exoExplanationHtmls); err != nil {
			return err
		}

		tv.Status = uipb.TestVariantStatus(tvStatus)
		if tv.Status == uipb.TestVariantStatus_EXPECTED {
			panic("query of test variants with unexpected results returns a test variant with only expected results.")
		}

		// Populate tv.Results
		tv.Results = make([]*uipb.TestResultBundle, len(results))
		for i, r := range results {
			tr, err := q.toTestResultProto(r, tv.TestId)
			if err != nil {
				return err
			}
			tv.Results[i] = &uipb.TestResultBundle{
				Result: tr,
			}
		}

		// Populate tv.Exonerations
		tv.Exonerations = make([]*pb.TestExoneration, len(exoExplanationHtmls))
		for i, ex := range exoExplanationHtmls {
			if expBytes, err = spanutil.Decompress(ex, expBytes); err != nil {
				return err
			}
			tv.Exonerations[i] = &pb.TestExoneration{
				ExplanationHtml: string(expBytes),
			}
		}
		return f(tv)
	})
}

// Fetch returns a page of test variants matching q.
// Returned test variants are ordered by test variant status, test ID and variant hash.
func (q *Query) Fetch(ctx context.Context) (tvs []*uipb.TestVariant, nextPageToken string, err error) {
	if q.PageSize <= 0 {
		panic("PageSize <= 0")
	}

	// TODO(crbug.com/1112127): query expected results.
	// TODO(crbug.com/1112127): add paging.
	tvs = make([]*uipb.TestVariant, 0, q.PageSize)
	err = q.run(ctx, func(tv *uipb.TestVariant) error {
		tvs = append(tvs, tv)
		return nil
	})
	if err != nil {
		tvs = nil
	}
	return
}

// queryTmpl is a set of templates that generate the SQL statements to query test variants.
var queryTmpl = template.Must(template.New("testVariants").Parse(`
@{USE_ADDITIONAL_PARALLELISM=TRUE}
	{{template "testVariantsWithUnexpectedResultsTmpl" .}}
LIMIT @limit

{{define "testVariantsWithUnexpectedResultsTmpl"}}
	WITH variantsWithUnexpectedResults AS (
		SELECT DISTINCT TestId, VariantHash
		FROM TestResults@{FORCE_INDEX=UnexpectedTestResults}
		WHERE IsUnexpected AND InvocationId in UNNEST(@invIDs)
	),

	-- Get test variants and their results.
	-- Also count the number of unexpected results and total results for each test
	-- variant, which will be used to classify test variants.
	test_variants AS (
		SELECT
			TestId,
			VariantHash,
			ANY_VALUE(Variant) Variant,
			COUNTIF(IsUnexpected) num_unexpected,
			COUNT(TestId) num_total,
			{{template "testResults"}},
		FROM variantsWithUnexpectedResults vur
		JOIN@{FORCE_JOIN_ORDER=TRUE, JOIN_METHOD=HASH_JOIN} TestResults tr USING (TestId, VariantHash)
		WHERE InvocationId in UNNEST(@invIDs)
		GROUP BY TestId, VariantHash
	),

	exonerated AS (
		SELECT
			TestId,
			VariantHash,
			ARRAY_AGG(ExplanationHTML) exonerationExplanations
		FROM TestExonerations
		WHERE InvocationId IN UNNEST(@invIDs)
		GROUP BY TestId, VariantHash
	),

	testVariantsWithUnexpectedResults AS (
		SELECT
			tv.TestId,
			tv.VariantHash,
			tv.Variant,
			CASE
				WHEN exonerated.TestId IS NOT NULL THEN 3 -- "EXONERATED"
				WHEN num_unexpected = 0 THEN 16 -- "EXPECTED", but should never happen in this query
				WHEN num_unexpected = num_total THEN 1 -- "UNEXPECTED"
				ELSE 2 --"FLAKY"
			END TvStatus,
			tv.results,
			exonerated.exonerationExplanations
		FROM test_variants tv
		LEFT JOIN exonerated USING(TestId, VariantHash)
		ORDER BY TvStatus, TestId, VariantHash
	)

	{{template "columns"}}
	FROM testVariantsWithUnexpectedResults
	ORDER BY TvStatus, TestId, VariantHash
{{end}}

{{define "testResults"}}
  -- TODO(crbug.com/1112127): Limit the number of results for each test variant to prevent OOM if there are too many of them.
	ARRAY_AGG(
		STRUCT(
			InvocationId,
			ResultId,
			IsUnexpected,
			Status,
			StartTime,
			RunDurationUsec,
			SummaryHTML,
			Tags)
	) results
{{end}}

{{define "columns"}}
	SELECT
		TestId,
		VariantHash,
		Variant,
		TvStatus,
		results,
		exonerationExplanations,
{{end}}
`))
