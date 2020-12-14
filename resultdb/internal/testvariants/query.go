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
	"context"

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

// testResultLimit is the limit of test results each test variant includes.
const testResultLimit = 10

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

func decompress(src, dst []byte) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}
	return spanutil.Decompress(src, dst)
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
	if q.summaryBuffer, err = decompress(r.SummaryHTML, q.summaryBuffer); err != nil {
		return nil, err
	}
	tr.SummaryHtml = string(q.summaryBuffer)

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

	st := spanner.Statement{SQL: testVariantsWithUnexpectedResultsSQL, Params: q.params}
	st.Params["limit"] = q.PageSize
	st.Params["testResultLimit"] = testResultLimit

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
			tv.Exonerations[i] = &pb.TestExoneration{}
			if expBytes, err = decompress(ex, expBytes); err != nil {
				return err
			}
			tv.Exonerations[i].ExplanationHtml = string(expBytes)
		}
		return f(tv)
	})
}

func (q *Query) queryTestResults(ctx context.Context, f func(*pb.TestResult) error) (err error) {
	ctx, ts := trace.StartSpan(ctx, "testvariants.Query.queryTestResults")
	ts.Attribute("cr.dev/invocations", len(q.InvocationIDs))
	defer func() { ts.End(err) }()
	st := spanner.Statement{SQL: allTestResultsSQL, Params: q.params}
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
	tvs = make([]*uipb.TestVariant, 0, q.PageSize)
	// Number of the total test results returned by the query.
	trLen := 0

	type tvId struct {
		TestId      string
		VariantHash string
	}
	// The last test variant we have completely processed.
	var lastProcessedTV tvId

	// The test variant we're processing right now.
	// It will be appended to tvs when all of its results are processed unless
	// it has unexpected results.
	var current *uipb.TestVariant
	var currentOnlyExpected bool
	err = q.queryTestResults(ctx, func(tr *pb.TestResult) error {
		trLen++
		if current != nil {
			if current.TestId == tr.TestId && current.VariantHash == tr.VariantHash {
				if len(current.Results) < testResultLimit {
					current.Results = append(current.Results, &uipb.TestResultBundle{
						Result: tr,
					})
				}
				currentOnlyExpected = currentOnlyExpected && tr.Expected
				return nil
			}

			// Different TestId or VariantHash from current, so all test results of
			// current have been processed.
			lastProcessedTV.TestId = current.TestId
			lastProcessedTV.VariantHash = current.VariantHash
			if currentOnlyExpected {
				tvs = append(tvs, current)
			}
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
		currentOnlyExpected = tr.Expected
		return nil
	})

	switch {
	case err != nil:
		tvs = nil
	case trLen < q.PageSize && currentOnlyExpected:
		// We have exhausted the test results, add current to tvs.
		tvs = append(tvs, current)
	case trLen == q.PageSize && !currentOnlyExpected:
		// Got page size of test results, need to return the next page token.
		// And current has unexpected results, skip it in the next page.
		nextPageToken = pagination.Token(uipb.TestVariantStatus_EXPECTED.String(), current.TestId, current.VariantHash)
	case trLen == q.PageSize:
		// In this page current only has expected results, but we're not sure if
		// we have exhausted its test results or not. Calculate the token using lastProcessedTV.
		nextPageToken = pagination.Token(uipb.TestVariantStatus_EXPECTED.String(), lastProcessedTV.TestId, lastProcessedTV.VariantHash)
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
		q.params["afterTvStatus"] = 0
		q.params["afterTestId"] = ""
		q.params["afterVariantHash"] = ""
	case len(parts) != 3:
		return nil, "", pagination.InvalidToken(errors.Reason("expected 3 components, got %q", parts).Err())
	default:
		status, ok := uipb.TestVariantStatus_value[parts[0]]
		if !ok {
			return nil, "", pagination.InvalidToken(errors.Reason("unrecognized test variant status: %q", parts[0]).Err())
		}
		expected = uipb.TestVariantStatus(status) == uipb.TestVariantStatus_EXPECTED
		q.params["afterTvStatus"] = int(status)
		q.params["afterTestId"] = parts[1]
		q.params["afterVariantHash"] = parts[2]
	}

	if expected {
		return q.fetchTestVariantsWithOnlyExpectedResults(ctx)
	}

	tvs = make([]*uipb.TestVariant, 0, q.PageSize)
	// Fetch test variants with unexpected results.
	err = q.queryTestVariantsWithUnexpectedResults(ctx, func(tv *uipb.TestVariant) error {
		tvs = append(tvs, tv)
		return nil
	})
	switch {
	case err != nil:
		tvs = nil
	case len(tvs) < q.PageSize:
		// If we got less than one page of test variants with unexpected results,
		// compute the nextPageToken for test variants with only expected results.
		nextPageToken = pagination.Token(uipb.TestVariantStatus_EXPECTED.String(), "", "")
	default:
		last := tvs[q.PageSize-1]
		nextPageToken = pagination.Token(last.Status.String(), last.TestId, last.VariantHash)
	}

	return
}

var testVariantsWithUnexpectedResultsSQL = `
	@{USE_ADDITIONAL_PARALLELISM=TRUE}
	WITH unexpectedTestVariants AS (
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
			ARRAY_AGG(STRUCT(
				InvocationId,
				ResultId,
				IsUnexpected,
				Status,
				StartTime,
				RunDurationUsec,
				SummaryHTML,
				Tags
			)) results,
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
			ARRAY(
				SELECT AS STRUCT *
				FROM UNNEST(tv.results)
				LIMIT @testResultLimit) results,
			exonerated.exonerationExplanations
		FROM test_variants tv
		LEFT JOIN exonerated USING(TestId, VariantHash)
		ORDER BY TvStatus, TestId, VariantHash
	)

	SELECT
		TestId,
		VariantHash,
		Variant,
		TvStatus,
		results,
		exonerationExplanations,
	FROM testVariantsWithUnexpectedResults
	WHERE (
		(TvStatus > @afterTvStatus) OR
		(TvStatus = @afterTvStatus AND TestId > @afterTestId) OR
		(TvStatus = @afterTvStatus AND TestId = @afterTestId AND VariantHash > @afterVariantHash)
	)
	ORDER BY TvStatus, TestId, VariantHash
	LIMIT @limit
`

var allTestResultsSQL = `
	@{USE_ADDITIONAL_PARALLELISM=TRUE}
	SELECT
		TestId,
		VariantHash,
		Variant Variant,
		InvocationId,
		ResultId,
		IsUnexpected,
		Status,
		StartTime,
		RunDurationUsec,
		SummaryHTML,
		Tags,
	FROM TestResults
	WHERE InvocationId in UNNEST(@invIDs)
	AND (
		(TestId > @afterTestId) OR
		(TestId = @afterTestId AND VariantHash > @afterVariantHash)
	)
	ORDER BY TestId, VariantHash
	LIMIT @limit
`
