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
	"text/template"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/trace"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/pagination"
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
	Predicate     *pb.TestVariantPredicate
	PageSize      int // must be positive
	// Consists of test variant status, test id and variant hash.
	PageToken string
	TestIDs   []string

	decompressBuf []byte                 // buffer for decompressing blobs
	params        map[string]interface{} // query parameters
}

// tvResult matches the result STRUCT of a test variant from the query.
type tvResult struct {
	InvocationID    string
	ResultID        string
	IsUnexpected    spanner.NullBool
	Status          int64
	StartTime       spanner.NullTime
	RunDurationUsec spanner.NullInt64
	SummaryHTML     []byte
	FailureReason   []byte
	Tags            []string
}

func (q *Query) decompressText(src []byte) (string, error) {
	if len(src) == 0 {
		return "", nil
	}
	var err error
	if q.decompressBuf, err = spanutil.Decompress(src, q.decompressBuf); err != nil {
		return "", err
	}
	return string(q.decompressBuf), nil
}

// decompressProto decompresses and unmarshals src to dest. It's a noop if src
// is empty.
func (q *Query) decompressProto(src []byte, dest proto.Message) error {
	if len(src) == 0 {
		return nil
	}
	var err error
	if q.decompressBuf, err = spanutil.Decompress(src, q.decompressBuf); err != nil {
		return err
	}
	return proto.Unmarshal(q.decompressBuf, dest)
}

func (q *Query) toTestResultProto(r *tvResult, testID string) (*pb.TestResult, error) {
	tr := &pb.TestResult{
		Name:     pbutil.TestResultName(string(invocations.IDFromRowID(r.InvocationID)), testID, r.ResultID),
		ResultId: r.ResultID,
		Status:   pb.TestStatus(r.Status),
	}
	if r.StartTime.Valid {
		tr.StartTime = pbutil.MustTimestampProto(r.StartTime.Time)
	}
	testresults.PopulateExpectedField(tr, r.IsUnexpected)
	testresults.PopulateDurationField(tr, r.RunDurationUsec)

	var err error
	if tr.SummaryHtml, err = q.decompressText(r.SummaryHTML); err != nil {
		return nil, err
	}

	if len(r.FailureReason) != 0 {
		// Don't initialize FailureReason when r.FailureReason is empty so
		// it won't produce {"failureReason": {}} when serialized to JSON.
		tr.FailureReason = &pb.FailureReason{}

		if err := q.decompressProto(r.FailureReason, tr.FailureReason); err != nil {
			return nil, err
		}
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

func (q *Query) queryTestVariantsWithUnexpectedResults(ctx context.Context, f func(*pb.TestVariant) error) (err error) {
	ctx, ts := trace.StartSpan(ctx, "testvariants.Query.run")
	ts.Attribute("cr.dev/invocations", len(q.InvocationIDs))
	defer func() { ts.End(err) }()

	if q.PageSize < 0 {
		panic("PageSize < 0")
	}

	st, err := spanutil.GenerateStatement(testVariantsWithUnexpectedResultsSQLTmpl, map[string]interface{}{
		"HasTestIds":   len(q.TestIDs) > 0,
		"StatusFilter": q.Predicate.GetStatus() != 0 && q.Predicate.GetStatus() != pb.TestVariantStatus_UNEXPECTED_MASK,
	})
	if err != nil {
		return
	}
	st.Params = q.params
	st.Params["limit"] = q.PageSize
	st.Params["testResultLimit"] = testResultLimit

	var b spanutil.Buffer
	return spanutil.Query(ctx, st, func(row *spanner.Row) error {
		tv := &pb.TestVariant{}
		var tvStatus int64
		var results []*tvResult
		var exoExplanationHtmls [][]byte
		var tmd spanutil.Compressed
		if err := b.FromSpanner(row, &tv.TestId, &tv.VariantHash, &tv.Variant, &tmd, &tvStatus, &results, &exoExplanationHtmls); err != nil {
			return err
		}

		tv.Status = pb.TestVariantStatus(tvStatus)
		if tv.Status == pb.TestVariantStatus_EXPECTED {
			panic("query of test variants with unexpected results returned a test variant with only expected results.")
		}

		if err := populateTestMetadata(tv, tmd); err != nil {
			return errors.Annotate(err, "error unmarshalling test_metadata for %s", tv.TestId).Err()
		}

		// Populate tv.Results
		tv.Results = make([]*pb.TestResultBundle, len(results))
		for i, r := range results {
			tr, err := q.toTestResultProto(r, tv.TestId)
			if err != nil {
				return err
			}
			tv.Results[i] = &pb.TestResultBundle{
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
			if tv.Exonerations[i].ExplanationHtml, err = q.decompressText(ex); err != nil {
				return err
			}
		}
		return f(tv)
	})
}

func (q *Query) queryTestResults(ctx context.Context, limit int, f func(*pb.TestResult, spanutil.Compressed) error) (err error) {
	ctx, ts := trace.StartSpan(ctx, "testvariants.Query.queryTestResults")
	ts.Attribute("cr.dev/invocations", len(q.InvocationIDs))
	defer func() { ts.End(err) }()
	st, err := spanutil.GenerateStatement(allTestResultsSQLTmpl, map[string]interface{}{
		"HasTestIds": len(q.TestIDs) > 0,
	})
	st.Params = q.params
	st.Params["limit"] = limit

	var b spanutil.Buffer
	var summaryHTML spanutil.Compressed
	return spanutil.Query(ctx, st, func(row *spanner.Row) error {
		var invID invocations.ID
		var maybeUnexpected spanner.NullBool
		var micros spanner.NullInt64
		var tmd spanutil.Compressed
		tr := &pb.TestResult{}
		err := b.FromSpanner(
			row, &tr.TestId, &tr.VariantHash, &tr.Variant, &tmd,
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

		return f(tr, tmd)
	})
}

func (q *Query) fetchTestVariantsWithOnlyExpectedResults(ctx context.Context) (tvs []*pb.TestVariant, nextPageToken string, err error) {
	tvs = make([]*pb.TestVariant, 0, q.PageSize)
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
	var current *pb.TestVariant
	var currentOnlyExpected bool
	// Query q.PageSize+1 test results for test variants with
	// only expected results, so that in the case of all test results are
	// expected in that page, we will return q.PageSize test variants instead of
	// q.PageSize-1.
	pageSize := q.PageSize + 1
	err = q.queryTestResults(ctx, pageSize, func(tr *pb.TestResult, tmd spanutil.Compressed) error {
		trLen++
		if current != nil {
			if current.TestId == tr.TestId && current.VariantHash == tr.VariantHash {
				if len(current.Results) < testResultLimit {
					current.Results = append(current.Results, &pb.TestResultBundle{
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
		current = &pb.TestVariant{
			TestId:      tr.TestId,
			VariantHash: tr.VariantHash,
			Variant:     tr.Variant,
			Status:      pb.TestVariantStatus_EXPECTED,
			Results: []*pb.TestResultBundle{
				{
					Result: tr,
				},
			},
		}
		currentOnlyExpected = tr.Expected
		if err := populateTestMetadata(current, tmd); err != nil {
			return errors.Annotate(err, "error unmarshalling test_metadata for %s", current.TestId).Err()
		}
		return nil
	})

	switch {
	case err != nil:
		tvs = nil
	case trLen < pageSize && currentOnlyExpected:
		// We have exhausted the test results, add current to tvs.
		tvs = append(tvs, current)
	case trLen == pageSize && !currentOnlyExpected:
		// Got page size of test results, need to return the next page token.
		// And current has unexpected results, skip it in the next page.
		nextPageToken = pagination.Token(pb.TestVariantStatus_EXPECTED.String(), current.TestId, current.VariantHash)
	case trLen == pageSize:
		// In this page current only has expected results, but we're not sure if
		// we have exhausted its test results or not. Calculate the token using lastProcessedTV.
		nextPageToken = pagination.Token(pb.TestVariantStatus_EXPECTED.String(), lastProcessedTV.TestId, lastProcessedTV.VariantHash)
	}

	return
}

// Fetch returns a page of test variants matching q.
// Returned test variants are ordered by test variant status, test ID and variant hash.
func (q *Query) Fetch(ctx context.Context) (tvs []*pb.TestVariant, nextPageToken string, err error) {
	if q.PageSize <= 0 {
		panic("PageSize <= 0")
	}

	status := int(q.Predicate.GetStatus())
	if q.Predicate.GetStatus() == pb.TestVariantStatus_UNEXPECTED_MASK {
		status = 0
	}

	q.params = map[string]interface{}{
		"invIDs":              q.InvocationIDs,
		"testIDs":             q.TestIDs,
		"skipStatus":          int(pb.TestStatus_SKIP),
		"unexpected":          int(pb.TestVariantStatus_UNEXPECTED),
		"unexpectedlySkipped": int(pb.TestVariantStatus_UNEXPECTEDLY_SKIPPED),
		"flaky":               int(pb.TestVariantStatus_FLAKY),
		"exonerated":          int(pb.TestVariantStatus_EXONERATED),
		"expected":            int(pb.TestVariantStatus_EXPECTED),
		"status":              status,
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
		status, ok := pb.TestVariantStatus_value[parts[0]]
		if !ok {
			return nil, "", pagination.InvalidToken(errors.Reason("unrecognized test variant status: %q", parts[0]).Err())
		}
		expected = pb.TestVariantStatus(status) == pb.TestVariantStatus_EXPECTED
		q.params["afterTvStatus"] = int(status)
		q.params["afterTestId"] = parts[1]
		q.params["afterVariantHash"] = parts[2]
	}

	if q.Predicate.GetStatus() == pb.TestVariantStatus_EXPECTED {
		expected = true
	}

	if expected {
		return q.fetchTestVariantsWithOnlyExpectedResults(ctx)
	}

	tvs = make([]*pb.TestVariant, 0, q.PageSize)
	// Fetch test variants with unexpected results.
	err = q.queryTestVariantsWithUnexpectedResults(ctx, func(tv *pb.TestVariant) error {
		tvs = append(tvs, tv)
		return nil
	})
	switch {
	case err != nil:
		tvs = nil
	case len(tvs) < q.PageSize && q.Predicate.GetStatus() != 0:
		// The query is for test variants with specific status, so the query reaches
		// to its last results already.
	case len(tvs) < q.PageSize:
		// If we got less than one page of test variants with unexpected results,
		// and the query is not for test variants with specific status,
		// compute the nextPageToken for test variants with only expected results.
		nextPageToken = pagination.Token(pb.TestVariantStatus_EXPECTED.String(), "", "")
	default:
		last := tvs[q.PageSize-1]
		nextPageToken = pagination.Token(last.Status.String(), last.TestId, last.VariantHash)
	}

	return
}

var testVariantsWithUnexpectedResultsSQLTmpl = template.Must(template.New("testVariantsWithUnexpectedResultsSQL").Parse(`
	@{USE_ADDITIONAL_PARALLELISM=TRUE}
	WITH unexpectedTestVariants AS (
		SELECT DISTINCT TestId, VariantHash
		FROM TestResults@{FORCE_INDEX=UnexpectedTestResults, spanner_emulator.disable_query_null_filtered_index_check=true}
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
			ANY_VALUE(TestMetadata) TestMetadata,
			COUNTIF(IsUnexpected) num_unexpected,
			COUNTIF(Status=@skipStatus) num_skipped,
			COUNT(TestId) num_total,
			ARRAY_AGG(STRUCT(
				InvocationId,
				ResultId,
				IsUnexpected,
				Status,
				StartTime,
				RunDurationUsec,
				SummaryHTML,
				FailureReason,
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
			tv.TestMetadata,
			CASE
				WHEN exonerated.TestId IS NOT NULL THEN @exonerated
				WHEN num_unexpected = 0 THEN @expected -- should never happen in this query
				WHEN num_skipped = num_unexpected AND num_skipped = num_total THEN @unexpectedlySkipped
				WHEN num_unexpected = num_total THEN @unexpected
				ELSE @flaky
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
		TestMetadata,
		TvStatus,
		results,
		exonerationExplanations,
	FROM testVariantsWithUnexpectedResults
	WHERE
	{{if .HasTestIds}}
		TestId in UNNEST(@testIDs) AND
	{{end}}
	{{if .StatusFilter}}
		(TvStatus = @status AND TestId > @afterTestId) OR
		(TvStatus = @status AND TestId = @afterTestId AND VariantHash > @afterVariantHash)
	{{else}}
		(TvStatus > @afterTvStatus) OR
		(TvStatus = @afterTvStatus AND TestId > @afterTestId) OR
		(TvStatus = @afterTvStatus AND TestId = @afterTestId AND VariantHash > @afterVariantHash)
	{{end}}
	ORDER BY TvStatus, TestId, VariantHash
	LIMIT @limit
`))

var allTestResultsSQLTmpl = template.Must(template.New("allTestResultsSQL").Parse(`
	@{USE_ADDITIONAL_PARALLELISM=TRUE}
	SELECT
		TestId,
		VariantHash,
		Variant,
		TestMetadata,
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
	{{if .HasTestIds}}
		AND TestId in UNNEST(@testIDs)
	{{end}}
	AND (
		(TestId > @afterTestId) OR
		(TestId = @afterTestId AND VariantHash > @afterVariantHash)
	)
	ORDER BY TestId, VariantHash
	LIMIT @limit
`))

func populateTestMetadata(tv *pb.TestVariant, tmd spanutil.Compressed) error {
	if len(tmd) == 0 {
		return nil
	}

	tv.TestMetadata = &pb.TestMetadata{}
	return proto.Unmarshal(tmd, tv.TestMetadata)
}
