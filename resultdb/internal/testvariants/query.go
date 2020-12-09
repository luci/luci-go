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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/trace"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/pagination"
	uipb "go.chromium.org/luci/resultdb/internal/proto/ui"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// Query specifies test variants to fetch.
type Query struct {
	InvocationIDs invocations.IDSet
	PageSize      int // must be positive
	// Consists of test variant status, test id and variant hash.
	PageToken     string
	summaryBuffer []byte                 // buffer for decompressing SummaryHTML
	unexpected    bool                   // if queried test variants have unexpected results
	params        map[string]interface{} // query parameters
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

func (q *Query) queryTestVariantsWithUnexpectedResults(ctx context.Context, f func(*uipb.TestVariant) error) (err error) {
	ctx, ts := trace.StartSpan(ctx, "testvariants.Query.run")
	ts.Attribute("cr.dev/invocations", len(q.InvocationIDs))
	defer func() { ts.End(err) }()

	if q.PageSize < 0 {
		panic("PageSize < 0")
	}

	var sql bytes.Buffer
	if err := queryTmpl.Execute(&sql, map[string]interface{}{"Unexpected": q.unexpected}); err != nil {
		return err
	}
	st := spanner.NewStatement(sql.String())
	st.Params = q.params

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
		if q.unexpected && tv.Status == uipb.TestVariantStatus_EXPECTED {
			panic("query of test variants with unexpected results returned a test variant with only expected results.")
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
		if len(exoExplanationHtmls) == 0 {
			return f(tv)
		}

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

type testVariant struct {
	TestID      string
	VariantHash string
}

// queryUnexpectedTestVariants returns test variants with unexpected
// results.
func (q *Query) queryUnexpectedTestVariants(ctx context.Context) (map[testVariant]struct{}, error) {
	var sql bytes.Buffer
	if err := queryTmpl.Execute(&sql, map[string]interface{}{"OnlyTestVariant": true}); err != nil {
		return nil, err
	}
	st := spanner.NewStatement(sql.String())
	st.Params = q.params

	testVariants := map[testVariant]struct{}{}
	err := spanutil.Query(ctx, st, func(row *spanner.Row) error {
		var tv testVariant
		if err := row.Columns(&tv.TestID, &tv.VariantHash); err != nil {
			return err
		}
		testVariants[tv] = struct{}{}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return testVariants, nil
}

func (q *Query) queryExpectedTestResults(ctx context.Context, f func(*pb.TestResult) error) (err error) {
	ctx, ts := trace.StartSpan(ctx, "testVariants.Query.queryExpectedTestResults")
	ts.Attribute("cr.dev/invocations", len(q.InvocationIDs))
	defer func() { ts.End(err) }()
	var sql bytes.Buffer
	if err := queryTmpl.Execute(&sql, nil); err != nil {
		return err
	}
	st := spanner.NewStatement(sql.String())
	st.Params = q.params
	// use a larger page size because:
	// * we need to remove the test variants with unexpected results
	// * If a test variant gets an expected result, it should not be retried for
	//   too many times, but sometime it does get retried.
	//   One example is in chromium try builds, successful tests are retried in
	//   (retry shards with patch) steps.
	st.Params["limit"] = 3 * q.PageSize

	var b spanutil.Buffer
	var summaryHTML spanutil.Compressed
	return spanutil.Query(ctx, st, func(row *spanner.Row) error {
		var invID invocations.ID
		var maybeUnexpected spanner.NullBool
		var micros spanner.NullInt64
		tr := &pb.TestResult{}
		if err := b.FromSpanner(
			row, &tr.TestId, &tr.VariantHash, &tr.Variant,
			&invID, &tr.ResultId, &maybeUnexpected, &tr.Status, &tr.StartTime,
			&micros, &summaryHTML, &tr.Tags,
		); err != nil {
			return err
		}
		tr.Name = pbutil.TestResultName(string(invID), tr.TestId, tr.ResultId)
		tr.SummaryHtml = string(summaryHTML)
		testresults.PopulateExpectedField(tr, maybeUnexpected)
		testresults.PopulateDurationField(tr, micros)

		return f(tr)
	})
}

func (q *Query) fetchTestVariantsWithOnlyExpectedResults(ctx context.Context) (tvs []*uipb.TestVariant, err error) {
	var unexpectedTVs map[testVariant]struct{}
	if unexpectedTVs, err = q.queryUnexpectedTestVariants(ctx); err != nil {
		return nil, err
	}

	tvs = make([]*uipb.TestVariant, 0, q.PageSize)
	i := -1 // index of the last test variant in tvs
	// TODO(crbug.com/1112127): Query next page if there are no enough expected results in this page.
	err = q.queryExpectedTestResults(ctx, func(tr *pb.TestResult) error {
		if len(unexpectedTVs) > 0 {
			if _, unexpected := unexpectedTVs[testVariant{TestID: tr.TestId, VariantHash: tr.VariantHash}]; unexpected {
				// This test variant has unexpected results, moving on.
				return nil
			}
		}

		if i >= 0 && tvs[i].TestId == tr.TestId && tvs[i].VariantHash == tr.VariantHash {
			tvs[i].Results = append(tvs[i].Results, &uipb.TestResultBundle{
				Result: tr,
			})
			return nil
		}

		// New test variant.
		if i == q.PageSize-1 {
			// Got one page of test variants.
			// TODO(crbug.com/1112127): Stop the query when get one page of test variants.
			return nil
		}

		tv := &uipb.TestVariant{
			TestId:      tr.TestId,
			VariantHash: tr.VariantHash,
			Variant:     tr.Variant,
			Status:      uipb.TestVariantStatus_EXPECTED,
			Results: []*uipb.TestResultBundle{
				{
					Result: tr,
				},
			},
		}

		i += 1
		tvs = append(tvs, tv)
		return nil
	})
	if err != nil {
		tvs = nil
	}
	return
}

// Fetch returns a page of test variants matching q.
// Returned test variants are ordered by test variant status, test ID and variant hash.
func (q *Query) Fetch(ctx context.Context) (tvs []*uipb.TestVariant, nextPageToken string, err error) {
	if q.PageSize <= 0 {
		panic("PageSize <= 0")
	}

	q.params = map[string]interface{}{
		"invIDs": q.InvocationIDs,
		"limit":  q.PageSize,
	}
	switch parts, err := pagination.ParseToken(q.PageToken); {
	case err != nil:
		return nil, "", err
	case len(parts) == 0:
		q.unexpected = true
		q.params["afterTvStatus"] = 0
		q.params["afterTestId"] = ""
		q.params["afterVariantHash"] = ""
	case len(parts) != 3:
		return nil, "", pagination.InvalidToken(errors.Reason("expected %d components, got %q", 3, parts).Err())
	default:
		status, ok := uipb.TestVariantStatus_value[parts[0]]
		if !ok {
			return nil, "", pagination.InvalidToken(errors.Reason("unrecognized test variant status: %q", parts[0]).Err())
		}
		q.unexpected = uipb.TestVariantStatus(status) != uipb.TestVariantStatus_EXPECTED
		q.params["afterTvStatus"] = int(status)
		q.params["afterTestId"] = parts[1]
		q.params["afterVariantHash"] = parts[2]
	}

	// TODO(crbug.com/1112127): populate next page token.
	tvs = make([]*uipb.TestVariant, 0, q.PageSize)

	// Fetch test variants with unexpected results.
	if q.unexpected {
		err = q.queryTestVariantsWithUnexpectedResults(ctx, func(tv *uipb.TestVariant) error {
			tvs = append(tvs, tv)
			return nil
		})
		if err != nil {
			tvs = nil
			return
		}
	} else {
		// Fetch test variants with only expected results.
		tvs, err = q.fetchTestVariantsWithOnlyExpectedResults(ctx)
		if err != nil {
			tvs = nil
			return
		}
	}

	// If we got pageSize results, then we haven't exhausted the collection and
	// need to return the next page token.
	if len(tvs) == q.PageSize {
		last := tvs[q.PageSize-1]
		nextPageToken = pagination.Token(last.Status.String(), last.TestId, last.VariantHash)
	}

	// If we got less than one page of test variants with unexpected results,
	// compute the nextPageToken for test variants with only expected results.
	if q.unexpected && len(tvs) < q.PageSize {
		nextPageToken = pagination.Token(uipb.TestVariantStatus_EXPECTED.String(), "", "")
	}

	return
}

// queryTmpl is a set of templates that generate the SQL statements to query test variants.
var queryTmpl = template.Must(template.New("testVariants").Parse(`
@{USE_ADDITIONAL_PARALLELISM=TRUE}
{{if .OnlyTestVariant}}
	{{template "unexpectedTestVariantsWithPageToken" .}}
{{else if .Unexpected}}
	{{template "testVariantsWithUnexpectedResults" .}}
{{else}}
	{{template "expectedResults" .}}
{{end}}
LIMIT @limit

{{define "testVariantsWithUnexpectedResults"}}
	WITH unexpectedTestVariants AS (
		-- We have to get the full list of unexpected test variants without applying
		-- page token because the test variants will be ordered by
    -- TvStatus, TestId, VariantHash. And in this subquery we do not know TvStatus
		-- of each test variants.
		{{template "unexpectedTestVariants"}}
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
			-- TODO(crbug.com/1112127): Limit the number of results for each test variant to prevent OOM if there are too many of them.
			ARRAY_AGG(STRUCT({{template "testResultsColumns"}})) results,
		FROM unexpectedTestVariants vur
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
	{{/* Apply the page token */}}
	WHERE (
		(TvStatus > @afterTvStatus) OR
		(TvStatus = @afterTvStatus AND TestId > @afterTestId) OR
		(TvStatus = @afterTvStatus AND TestId = @afterTestId AND VariantHash > @afterVariantHash)
	)
	ORDER BY TvStatus, TestId, VariantHash
{{end}}

{{define "expectedResults"}}
	SELECT
		TestId,
		VariantHash,
		Variant Variant,
		{{template "testResultsColumns"}},
	FROM TestResults
	WHERE InvocationId in UNNEST(@invIDs)
	AND IsUnexpected IS NULL
	{{/* Apply the page token */}}
	AND (
		(TestId > @afterTestId) OR
		(TestId = @afterTestId AND VariantHash > @afterVariantHash)
	)
	ORDER BY TestId, VariantHash
{{end}}

{{define "testResultsColumns"}}
	InvocationId,
	ResultId,
	IsUnexpected,
	Status,
	StartTime,
	RunDurationUsec,
	SummaryHTML,
	Tags
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

{{define "unexpectedTestVariants"}}
	SELECT DISTINCT TestId, VariantHash
		FROM TestResults@{FORCE_INDEX=UnexpectedTestResults}
		WHERE IsUnexpected AND InvocationId in UNNEST(@invIDs)
{{end}}

{{define "unexpectedTestVariantsWithPageToken"}}
	{{template "unexpectedTestVariants"}}
	AND (
		(TestId > @afterTestId) OR
		(TestId = @afterTestId AND VariantHash > @afterVariantHash)
	)
	ORDER BY TestId, VariantHash
{{end}}
`))
