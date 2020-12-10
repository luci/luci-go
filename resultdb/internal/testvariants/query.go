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
	"fmt"
	"sync"
	"text/template"

	"cloud.google.com/go/spanner"
	"golang.org/x/sync/errgroup"

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

	st := q.genStatement("testVariantsWithUnexpectedResults")
	st.Params["limit"] = q.PageSize

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
func (q *Query) queryUnexpectedTestVariants(ctx context.Context) (testVariants map[testVariant]struct{}, err error) {
	ctx, ts := trace.StartSpan(ctx, "testvariants.Query.queryUnexpectedTestVariants")
	ts.Attribute("cr.dev/invocations", len(q.InvocationIDs))
	defer func() { ts.End(err) }()

	testVariants = map[testVariant]struct{}{}
	var mu sync.Mutex
	st := q.genStatement("unexpectedTestVariantsQuery")

	eg, ctx := errgroup.WithContext(ctx)
	for _, st := range invocations.ShardStatement(st, "invIDs") {
		st := st
		eg.Go(func() error {
			return spanutil.Query(ctx, st, func(row *spanner.Row) error {
				var tv testVariant
				if err := row.Columns(&tv.TestID, &tv.VariantHash); err != nil {
					return err
				}

				mu.Lock()
				testVariants[tv] = struct{}{}
				mu.Unlock()
				return nil
			})
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return
}

func (q *Query) queryExpectedTestResults(ctx context.Context, f func(*pb.TestResult) error) (err error) {
	ctx, ts := trace.StartSpan(ctx, "testvariants.Query.queryExpectedTestResults")
	ts.Attribute("cr.dev/invocations", len(q.InvocationIDs))
	defer func() { ts.End(err) }()
	st := q.genStatement("expectedResults")
	// TODO(crbug.com/1157349): Tune the page size.
	st.Params["limit"] = q.PageSize

	var b spanutil.Buffer
	var summaryHTML spanutil.Compressed
	return spanutil.Query(ctx, st, func(row *spanner.Row) error {
		var invID invocations.ID
		var maybeUnexpected spanner.NullBool
		var micros spanner.NullInt64
		tr := &pb.TestResult{}
		err := b.FromSpanner(
			row, &tr.TestId, &tr.VariantHash, &tr.Variant,
			&invID, &tr.ResultId, &maybeUnexpected, &tr.Status, &tr.StartTime,
			&micros, &summaryHTML, &tr.Tags,
		)

		if err != nil {
			return err
		}
		tr.Name = pbutil.TestResultName(string(invID), tr.TestId, tr.ResultId)
		tr.SummaryHtml = string(summaryHTML)
		testresults.PopulateExpectedField(tr, maybeUnexpected)
		testresults.PopulateDurationField(tr, micros)

		return f(tr)
	})
}

func (q *Query) fetchTestVariantsWithOnlyExpectedResults(ctx context.Context) (tvs []*uipb.TestVariant, nextPageToken string, err error) {
	// TODO(crbug.com/1112127): run the query concurrently, then in callback wait for this to finish.
	unexpectedTVs, err := q.queryUnexpectedTestVariants(ctx)
	if err != nil {
		return nil, "", err
	}

	tvs = make([]*uipb.TestVariant, 0, q.PageSize)
	// Number of the total test results returned by the query.
	trLen := 0
	// The test variant we're processing right now.
	// It will be appended to tvs when all of its results are added.
	var current *uipb.TestVariant
	err = q.queryExpectedTestResults(ctx, func(tr *pb.TestResult) error {
		trLen += 1
		if _, unexpected := unexpectedTVs[testVariant{TestID: tr.TestId, VariantHash: tr.VariantHash}]; unexpected {
			// This test variant has unexpected results, moving on.
			return nil
		}

		if current != nil {
			if current.TestId == tr.TestId && current.VariantHash == tr.VariantHash {
				current.Results = append(current.Results, &uipb.TestResultBundle{
					Result: tr,
				})
				return nil
			}

			// All test results of current have been processed.
			tvs = append(tvs, current)
		}

		// New test variant.
		current = &uipb.TestVariant{
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
		return nil
	})
	if err != nil {
		tvs = nil
		return
	}

	if trLen < q.PageSize {
		// We have exhausted the test results, add current to tvs.
		tvs = append(tvs, current)
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
	}
	var expected bool
	switch parts, err := pagination.ParseToken(q.PageToken); {
	case err != nil:
		return nil, "", err
	case len(parts) == 0:
		expected = false
	case len(parts) != 3:
		return nil, "", pagination.InvalidToken(errors.Reason("expected 3 components, got %q", parts).Err())
	default:
		status, ok := uipb.TestVariantStatus_value[parts[0]]
		if !ok {
			return nil, "", pagination.InvalidToken(errors.Reason("unrecognized test variant status: %q", parts[0]).Err())
		}
		expected = uipb.TestVariantStatus(status) == uipb.TestVariantStatus_EXPECTED
	}

	if expected {
		return q.fetchTestVariantsWithOnlyExpectedResults(ctx)
	}

	// TODO(crbug.com/1112127): populate next page token.
	tvs = make([]*uipb.TestVariant, 0, q.PageSize)
	// Fetch test variants with unexpected results.
	err = q.queryTestVariantsWithUnexpectedResults(ctx, func(tv *uipb.TestVariant) error {
		tvs = append(tvs, tv)
		return nil
	})
	if err != nil {
		tvs = nil
	}
	return
}

func (q *Query) genStatement(templateName string) spanner.Statement {
	var sql bytes.Buffer
	err := queryTmpl.ExecuteTemplate(&sql, templateName, nil)
	if err != nil {
		panic(fmt.Sprintf("failed to generate a SQL statement: %s", err))
	}
	return spanner.Statement{SQL: sql.String(), Params: q.params}
}

// queryTmpl is a set of templates that generate the SQL statements to query test variants.
var queryTmpl = template.Must(template.New("testVariants").Parse(`
{{define "testVariantsWithUnexpectedResults"}}
	@{USE_ADDITIONAL_PARALLELISM=TRUE}
	WITH unexpectedTestVariants AS (
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
	ORDER BY TvStatus, TestId, VariantHash
	LIMIT @limit
{{end}}

{{define "expectedResults"}}
	@{USE_ADDITIONAL_PARALLELISM=TRUE}
	SELECT
		TestId,
		VariantHash,
		Variant Variant,
		{{template "testResultsColumns"}},
	FROM TestResults
	WHERE InvocationId in UNNEST(@invIDs)
	AND IsUnexpected IS NULL
	ORDER BY TestId, VariantHash
	LIMIT @limit
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

{{define "unexpectedTestVariantsQuery"}}
	@{USE_ADDITIONAL_PARALLELISM=TRUE}
	{{template "unexpectedTestVariants"}}
{{end}}
`))
