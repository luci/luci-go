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
	"strings"
	"text/template"

	"cloud.google.com/go/spanner"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/common/trace"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/graph"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	// resultLimitMax is the maximum number of results can be included in a test
	// variant when querying test variants. The client may specify a lower limit.
	// It is required to prevent client-caused OOMs.
	resultLimitMax     = 100
	resultLimitDefault = 10

	// DefaultResponseLimitBytes is the default soft limit on the number of bytes
	// that should be returned by a Test Variants query.
	DefaultResponseLimitBytes = 20 * 1000 * 1000 // 20 MB
)

// AllFields is a field mask that selects all TestVariant fields.
var AllFields = mask.All(&pb.TestVariant{})

// QueryMask returns mask.Mask converted from field_mask.FieldMask.
// It returns a default mask with all fields if readMask is empty.
func QueryMask(readMask *field_mask.FieldMask) (*mask.Mask, error) {
	if len(readMask.GetPaths()) == 0 {
		return AllFields, nil
	}
	return mask.FromFieldMask(readMask, &pb.TestVariant{}, false, false)
}

// AdjustResultLimit takes the given requested resultLimit and adjusts as
// necessary.
func AdjustResultLimit(resultLimit int32) int {
	switch {
	case resultLimit >= resultLimitMax:
		return resultLimitMax
	case resultLimit > 0:
		return int(resultLimit)
	default:
		return resultLimitDefault
	}
}

// ValidateResultLimit returns a non-nil error if resultLimit is invalid.
// Returns nil if resultLimit is 0.
func ValidateResultLimit(resultLimit int32) error {
	if resultLimit < 0 {
		return errors.Reason("negative").Err()
	}
	return nil
}

type AccessLevel int

const (
	// The access level is invalid; either an error occurred when checking the
	// caller's access, or the caller does not have any kind of access to data in
	// the realms of the invocations being queried.
	AccessLevelInvalid AccessLevel = iota
	// Simplified Access Level 1. The caller has access to metadata only, such as
	// pass/fail status of tests, test IDs and sanitised failure reasons.
	AccessLevelSAL1
	// The caller has access to all data in the realms of the invocations being
	// queried.
	AccessLevelUnrestricted
)

// Query specifies test variants to fetch.
type Query struct {
	ReachableInvocations graph.ReachableInvocations
	Predicate            *pb.TestVariantPredicate
	// ResultLimit is a limit on the number of test results returned
	// per test variant. Must be postiive.
	ResultLimit int
	// PageSize is a limit on the number of test variants returned
	// per page. The actual number of test variants returned may be
	// less than this, or even zero. (It may be zero even if there are
	// more test variants in later pages). Must be positive.
	PageSize int
	// ResponseLimitBytes is the soft limit on the number of bytes returned
	// by a query. The soft limit is on JSON-serialised form of the
	// result. A page will be truncated once it exceeds this value.
	// Must be positive.
	ResponseLimitBytes int
	// Consists of test variant status, test id and variant hash.
	PageToken string
	Mask      *mask.Mask
	TestIDs   []string
	// The level of access the user has to test results and test exoneration data.
	// TODO: use this to apply SAL1 metadata masks to the response.
	AccessLevel AccessLevel

	decompressBuf []byte                 // buffer for decompressing blobs
	params        map[string]interface{} // query parameters
}

// trim is equivalent to q.Mask.Trim with the exception that "test_id",
// "variant_hash", and "status" are always kept intact. Those fields are needed
// to generate page tokens.
func (q *Query) trim(tv *pb.TestVariant) error {
	testID := tv.TestId
	vHash := tv.VariantHash
	status := tv.Status

	if err := q.Mask.Trim(tv); err != nil {
		return errors.Annotate(err, "error trimming fields for test variant with ID: %s, variant hash: %s", tv.TestId, tv.VariantHash).Err()
	}

	tv.TestId = testID
	tv.VariantHash = vHash
	tv.Status = status
	return nil
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
	Properties      []byte
}

// resultSelectColumns returns a list of columns needed to fetch `tvResult`s
// according to the fieldmask. `IsUnexpected` is always selected.
func (q *Query) resultSelectColumns() []string {
	columnSet := stringset.New(1)
	columnSet.Add("IsUnexpected")

	// Select extra columns depending on the mask.
	readMask := q.Mask
	if readMask.IsEmpty() {
		readMask = AllFields
	}
	selectIfIncluded := func(column string, fields ...string) {
		for _, field := range fields {
			switch inc, err := readMask.Includes(field); {
			case err != nil:
				panic(err)
			case inc != mask.Exclude:
				columnSet.Add(column)
				return
			}
		}
	}

	selectIfIncluded("InvocationId", "results.*.result.name")
	selectIfIncluded("ResultId", "results.*.result.name", "results.*.result.result_id")
	selectIfIncluded("Status", "results.*.result.status")
	selectIfIncluded("StartTime", "results.*.result.start_time")
	selectIfIncluded("RunDurationUsec", "results.*.result.duration")
	selectIfIncluded("SummaryHTML", "results.*.result.summary_html")
	selectIfIncluded("FailureReason", "results.*.result.failure_reason")
	selectIfIncluded("Tags", "results.*.result.tags")
	selectIfIncluded("Properties", "results.*.result.properties")

	return columnSet.ToSortedSlice()
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
		ResultId: r.ResultID,
		Status:   pb.TestStatus(r.Status),
	}
	if r.InvocationID != "" && testID != "" && r.ResultID != "" {
		tr.Name = pbutil.TestResultName(string(invocations.IDFromRowID(r.InvocationID)), testID, r.ResultID)
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
	if len(r.Properties) != 0 {
		// Don't initialize properties when r.Properties is empty so
		// it won't produce {"Properties": {}} when serialized to JSON.
		tr.Properties = &structpb.Struct{}

		if err := q.decompressProto(r.Properties, tr.Properties); err != nil {
			return nil, err
		}
	}

	// Populate Tags.
	tr.Tags = make([]*pb.StringPair, len(r.Tags))
	for i, p := range r.Tags {
		tr.Tags[i] = pbutil.StringPairFromStringUnvalidated(p)
	}

	return tr, nil
}

func (q *Query) queryTestVariantsWithUnexpectedResults(ctx context.Context, f func(*pb.TestVariant) error) (err error) {
	ctx, ts := trace.StartSpan(ctx, "testvariants.Query.run")
	ts.Attribute("cr.dev/invocations", len(q.ReachableInvocations))
	defer func() { ts.End(err) }()

	if q.PageSize < 0 {
		panic("PageSize < 0")
	}

	st, err := spanutil.GenerateStatement(testVariantsWithUnexpectedResultsSQLTmpl, map[string]interface{}{
		"ResultColumns": strings.Join(q.resultSelectColumns(), ", "),
		"HasTestIds":    len(q.TestIDs) > 0,
		"StatusFilter":  q.Predicate.GetStatus() != 0 && q.Predicate.GetStatus() != pb.TestVariantStatus_UNEXPECTED_MASK,
	})
	if err != nil {
		return
	}
	st.Params = q.params
	st.Params["limit"] = q.PageSize
	st.Params["testResultLimit"] = q.ResultLimit

	var b spanutil.Buffer
	return spanutil.Query(ctx, st, func(row *spanner.Row) error {
		tv := &pb.TestVariant{}
		var tvStatus int64
		var results []*tvResult
		var exonerationExplanationHTMLs [][]byte
		var exonerationReasons []int64
		var tmd spanutil.Compressed
		if err := b.FromSpanner(row, &tv.TestId, &tv.VariantHash, &tv.Variant, &tmd, &tvStatus, &results, &exonerationExplanationHTMLs, &exonerationReasons); err != nil {
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
		if len(exonerationReasons) == 0 {
			return f(tv)
		}

		tv.Exonerations = make([]*pb.TestExoneration, len(exonerationReasons))
		for i := range exonerationReasons {
			// Due to query design, length of exonerationExplanationHTMLs
			// should be identical to exonerationReasons.
			ex := exonerationExplanationHTMLs[i]
			e := &pb.TestExoneration{}
			if e.ExplanationHtml, err = q.decompressText(ex); err != nil {
				return err
			}
			e.Reason = pb.ExonerationReason(exonerationReasons[i])
			tv.Exonerations[i] = e
		}
		return f(tv)
	})
}

func (q *Query) fetchTestVariantsWithUnexpectedResults(ctx context.Context) (tvs []*pb.TestVariant, nextPageToken string, err error) {
	responseSize := 0
	tvs = make([]*pb.TestVariant, 0, q.PageSize)

	// Fetch test variants with unexpected results.
	err = q.queryTestVariantsWithUnexpectedResults(ctx, func(tv *pb.TestVariant) error {
		// Apply field mask.
		if err := q.trim(tv); err != nil {
			return err
		}

		tvs = append(tvs, tv)

		// Apply soft repsonse size limit.
		responseSize += estimateTestVariantSize(tv)
		if responseSize > q.ResponseLimitBytes {
			return responseLimitReachedErr
		}
		return nil
	})
	if err != nil && err != responseLimitReachedErr {
		return nil, "", err
	}

	if len(tvs) == q.PageSize || err == responseLimitReachedErr {
		// There could be more test variants to return.
		last := tvs[len(tvs)-1]
		nextPageToken = pagination.Token(last.Status.String(), last.TestId, last.VariantHash)
	} else {
		// We have finished reading all test variants with unexpected results.
		if q.Predicate.GetStatus() != 0 {
			// The query is for test variants with specific status, so the query reaches
			// to its last results already.
			nextPageToken = ""
		} else {
			// If we got less than one page of test variants with unexpected results,
			// and the query is not for test variants with specific status,
			// compute the nextPageToken for test variants with only expected results.
			nextPageToken = pagination.Token(pb.TestVariantStatus_EXPECTED.String(), "", "")
		}
	}
	return tvs, nextPageToken, nil
}

// queryTestResults returns a page of test results, calling f for each
// test result read. Test results are returned in test variant order.
// Within each test variant, unexpected results are returned first.
func (q *Query) queryTestResults(ctx context.Context, limit int, f func(testId, variantHash string, variant *pb.Variant, tmd spanutil.Compressed, tvr *tvResult) error) (err error) {
	ctx, ts := trace.StartSpan(ctx, "testvariants.Query.queryTestResults")
	ts.Attribute("cr.dev/invocations", len(q.ReachableInvocations))
	defer func() { ts.End(err) }()
	st, err := spanutil.GenerateStatement(allTestResultsSQLTmpl, map[string]interface{}{
		"ResultColumns": strings.Join(q.resultSelectColumns(), ", "),
		"HasTestIds":    len(q.TestIDs) > 0,
	})
	st.Params = q.params
	st.Params["limit"] = limit

	var b spanutil.Buffer
	return spanutil.Query(ctx, st, func(row *spanner.Row) error {
		var testID string
		var variantHash string
		variant := &pb.Variant{}
		var tmd spanutil.Compressed
		var results []*tvResult
		if err := b.FromSpanner(row, &testID, &variantHash, &variant, &tmd, &results); err != nil {
			return err
		}

		return f(testID, variantHash, variant, tmd, results[0])
	})
}

// queryTestVariants queries a page of test variants, calling f for each
// test variant read. Unless this method returns an error, it is guaranteed
// f will be called at least once, or finished will be set to true.
// finished indicates all test results from the invocation have been read.
//
// The number of test results provided per test variant is limited based
// on q.ResultLimit. Unexpected test results appear first in the
// list of test results provided on each test variant.
//
// The status of the test variant is not populated as exonerations are
// not read.
func (q *Query) queryTestVariants(ctx context.Context, f func(*pb.TestVariant) error) (finished bool, err error) {
	// Number of the total test results returned by the query.
	trCount := 0

	// The test variant we're processing right now.
	// It will be appended to tvs when all of its results are processed unless
	// it has unexpected results.
	var current *pb.TestVariant

	// Query q.PageSize+1 test results, so that in the case of all test results
	// correspond to one test variant, we will return q.PageSize test variants
	// instead of q.PageSize-1.
	pageSize := q.PageSize + 1

	// Ensure we always make forward progress, by having enough test results
	// to call the callback at least once.
	if q.ResultLimit > pageSize {
		pageSize = q.ResultLimit
	}

	// Number of test variants yielded so far. Used to ensure
	// we never yield more test variants than the page size.
	yieldCount := 0
	yield := func(tv *pb.TestVariant) error {
		yieldCount++
		// Never yield more test variants to the caller than the requested
		// query page size.
		if yieldCount <= q.PageSize {
			return f(tv)
		}
		return nil
	}

	err = q.queryTestResults(ctx, pageSize, func(testId, variantHash string, variant *pb.Variant, tmd spanutil.Compressed, tvr *tvResult) error {
		trCount++

		tr, err := q.toTestResultProto(tvr, testId)
		if err != nil {
			return err
		}

		var toYield *pb.TestVariant
		if current != nil {
			if current.TestId == testId && current.VariantHash == variantHash {
				// Another result within the same test variant.
				if len(current.Results) < q.ResultLimit {
					// Only append if within result limit.
					current.Results = append(current.Results, &pb.TestResultBundle{
						Result: tr,
					})
				}
				return nil
			}

			// Different TestId or VariantHash from current, so all test results of
			// current have been processed.
			toYield = current
		}

		// New test variant.
		current = &pb.TestVariant{
			TestId:      testId,
			VariantHash: variantHash,
			Variant:     variant,
			Results: []*pb.TestResultBundle{
				{
					Result: tr,
				},
			},
		}

		if err := populateTestMetadata(current, tmd); err != nil {
			return errors.Annotate(err, "error unmarshalling test_metadata for %s", current.TestId).Err()
		}
		if toYield != nil {
			return yield(toYield)
		}
		return nil
	})
	if err != nil {
		return false, err
	}

	if current != nil {
		// Yield current if:
		// - We finished reading all test results for the queried invocations, OR
		// - We have ResultLimit test results for the test variant.
		if len(current.Results) == q.ResultLimit || trCount < pageSize {
			err := yield(current)
			if err != nil {
				return false, err
			}
		}
	}

	// We have finished if we have run out of test results in the queried
	// invocations, and we managed to deliver all those test variants to
	// the caller.
	finished = trCount < pageSize && yieldCount <= q.PageSize
	return finished, nil
}

// responseLimitReachedErr is an error returned to avoid iterating over more
// results when the soft response size limit has been reached.
var responseLimitReachedErr = errors.New("terminating iteration early as response size limit reached")

func (q *Query) fetchTestVariantsWithOnlyExpectedResults(ctx context.Context) (tvs []*pb.TestVariant, nextPageToken string, err error) {
	responseSize := 0
	tvs = make([]*pb.TestVariant, 0, q.PageSize)

	// The last test variant we have completely processed.
	var lastProcessedTestID string
	var lastProcessedVariantHash string

	finished, err := q.queryTestVariants(ctx, func(tv *pb.TestVariant) error {
		lastProcessedTestID = tv.TestId
		lastProcessedVariantHash = tv.VariantHash

		// Within the test variant, unexpected results appear first.
		// So if the first result is expected, all are.
		isOnlyExpected := tv.Results[0].Result.Expected
		if isOnlyExpected {
			tv.Status = pb.TestVariantStatus_EXPECTED

			// Apply field mask.
			if err := q.trim(tv); err != nil {
				return err
			}

			tvs = append(tvs, tv)

			// Apply soft repsonse size limit.
			responseSize += estimateTestVariantSize(tv)
			if responseSize > q.ResponseLimitBytes {
				return responseLimitReachedErr
			}
		}
		return nil
	})
	if err != nil && err != responseLimitReachedErr {
		return nil, "", err
	}

	if finished {
		nextPageToken = ""
	} else {
		nextPageToken = pagination.Token(pb.TestVariantStatus_EXPECTED.String(), lastProcessedTestID, lastProcessedVariantHash)
	}

	return tvs, nextPageToken, nil
}

// Fetch returns a page of test variants matching q.
// Returned test variants are ordered by test variant status, test ID and variant hash.
func (q *Query) Fetch(ctx context.Context) (tvs []*pb.TestVariant, nextPageToken string, err error) {
	if q.PageSize <= 0 {
		panic("PageSize <= 0")
	}
	if q.ResultLimit <= 0 {
		panic("ResultLimit <= 0")
	}
	if q.ResponseLimitBytes <= 0 {
		panic("ResponseLimitBytes <= 0")
	}

	status := int(q.Predicate.GetStatus())
	if q.Predicate.GetStatus() == pb.TestVariantStatus_UNEXPECTED_MASK {
		status = 0
	}

	q.params = map[string]interface{}{
		"testResultInvIDs":      q.ReachableInvocations.WithTestResultsIDSet(),
		"testExonerationInvIDs": q.ReachableInvocations.WithExonerationsIDSet(),
		"testIDs":               q.TestIDs,
		"skipStatus":            int(pb.TestStatus_SKIP),
		"unexpected":            int(pb.TestVariantStatus_UNEXPECTED),
		"unexpectedlySkipped":   int(pb.TestVariantStatus_UNEXPECTEDLY_SKIPPED),
		"flaky":                 int(pb.TestVariantStatus_FLAKY),
		"exonerated":            int(pb.TestVariantStatus_EXONERATED),
		"expected":              int(pb.TestVariantStatus_EXPECTED),
		"status":                status,
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
	} else {
		return q.fetchTestVariantsWithUnexpectedResults(ctx)
	}
}

var testVariantsWithUnexpectedResultsSQLTmpl = template.Must(template.New("testVariantsWithUnexpectedResultsSQL").Parse(`
	@{USE_ADDITIONAL_PARALLELISM=TRUE}
	WITH unexpectedTestVariants AS (
		SELECT DISTINCT TestId, VariantHash
		FROM TestResults@{FORCE_INDEX=UnexpectedTestResults, spanner_emulator.disable_query_null_filtered_index_check=true}
		WHERE IsUnexpected AND InvocationId in UNNEST(@testResultInvIDs)
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
			ARRAY_AGG(STRUCT({{.ResultColumns}})) results,
		FROM unexpectedTestVariants vur
		JOIN@{FORCE_JOIN_ORDER=TRUE, JOIN_METHOD=HASH_JOIN} TestResults tr USING (TestId, VariantHash)
		WHERE InvocationId in UNNEST(@testResultInvIDs)
		GROUP BY TestId, VariantHash
	),

	exonerated AS (
		SELECT
			TestId,
			VariantHash,
			ARRAY_AGG(ExplanationHTML) ExonertionExplanationHTMLs,
			ARRAY_AGG(Reason) ExonerationReasons
		FROM TestExonerations
		WHERE InvocationId IN UNNEST(@testExonerationInvIDs)
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
			exonerated.ExonertionExplanationHTMLs,
			exonerated.ExonerationReasons
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
		ExonertionExplanationHTMLs,
		ExonerationReasons,
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

		-- Spanner doesn't support returning a struct as a column.
		-- https://cloud.google.com/spanner/docs/reference/standard-sql/query-syntax#using_structs_with_select
		-- Wrap it in an array instead.
		ARRAY(SELECT STRUCT({{.ResultColumns}})) AS results,
	FROM TestResults
	WHERE InvocationId in UNNEST(@testResultInvIDs)
	{{if .HasTestIds}}
		AND TestId in UNNEST(@testIDs)
	{{end}}
	AND (
		(TestId > @afterTestId) OR
		(TestId = @afterTestId AND VariantHash > @afterVariantHash)
	)
	-- Within a test variant, return the unexpected results first.
	ORDER BY TestId, VariantHash, IsUnexpected DESC
	LIMIT @limit
`))

func populateTestMetadata(tv *pb.TestVariant, tmd spanutil.Compressed) error {
	if len(tmd) == 0 {
		return nil
	}

	tv.TestMetadata = &pb.TestMetadata{}
	return proto.Unmarshal(tmd, tv.TestMetadata)
}

// estimateTestVariantSize estimates the size of a test variant in
// a pRPC response (pRPC responses use JSON serialisation).
func estimateTestVariantSize(tv *pb.TestVariant) int {
	// Estimate the size of a JSON-serialised test variant,
	// as the sum of the sizes of its fields, plus
	// an overhead (for JSON grammar and the field names).
	return 1000 + proto.Size(tv)
}
