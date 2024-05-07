// Copyright 2024 The LUCI Authors.
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

// Package runtestvariants provides methods to query test variants within a test run
// (single invocation excluding included invocation).
package runtestvariants

import (
	"context"

	"cloud.google.com/go/spanner"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/internal/tracing"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// complete is returned by the callback provided to fetch(...) to indicate
// that iteration is complete and no more test variants should be retrieved.
var complete = errors.New("callback does not want any more test variants")

// Query specifies the run test variants to fetch.
type Query struct {
	InvocationID       invocations.ID
	PageSize           int // must be non-negative
	PageToken          PageToken
	ResultLimit        int // must be non-negative
	ResponseLimitBytes int // maximum response size in bytes (soft limit)
}

// QueryResult is the result of a Query.
type QueryResult struct {
	// The test variants retrieved.
	//
	// Within each test variant, results are ordered with unexpected results
	// first, then by result ID.
	//
	// The test variants of a run do not have a concept of status,
	// exonerations or sources. As such those fields are not populated.
	TestVariants []*pb.TestVariant
	// The continuation token. If this is empty, then there are no more
	// test variants to retrieve.
	NextPageToken PageToken
}

// Run queries the run's test variants. It behaves deterministically,
// returning the maximum number of test variants consistent with the
// requested PageSize and ResponseLimitBytes, in ascending (test, variantHash)
// order.
//
// Must be run in a transactional spanner context supporting multiple reads.
func (q *Query) Run(ctx context.Context) (result QueryResult, err error) {
	ctx, ts := tracing.Start(ctx, "runtestvariants.Query.run",
		attribute.String("cr.dev.invocation", string(q.InvocationID)),
	)
	defer func() { tracing.End(ts, err) }()

	if q.PageSize < 0 {
		panic("PageSize < 0")
	}
	if q.ResultLimit < 0 {
		panic("ResultLimit < 0")
	}
	if q.ResponseLimitBytes < 0 {
		panic("ResponseLimitBytes < 0")
	}

	var results []*pb.TestVariant
	var responseSize int

	continuation := q.PageToken
	done := false

	// Assume 1.1 test results per test variant.
	milliTestResultsPerTestVariant := 1100
	for !done {
		opts := fetchOptions{
			targetTestVariants:             q.PageSize - len(results),
			milliTestResultsPerTestVariant: milliTestResultsPerTestVariant,
		}

		// Fetch the next batch of test variants.
		continuation, err = q.fetch(ctx, continuation, opts, func(tv *pb.TestVariant) error {
			results = append(results, tv)

			// Check if we have reached our soft response size limit.
			// We check this after appending to ensure we always
			// return at least one test variant on each Query.
			responseSize += estimateTestVariantSize(tv)
			if responseSize >= q.ResponseLimitBytes {
				done = true
				return complete
			}

			// Check if we have reached our target page size.
			if len(results) >= q.PageSize {
				done = true
				return complete
			}
			// Continue iterating.
			return nil
		})
		if err != nil {
			return QueryResult{}, err
		}
		if (continuation == PageToken{}) {
			// Empty page token indicates we have read all test variants
			// in the invocation.
			done = true
		}

		// If we are not yet done, then our estimate of test results per test
		// variant was too low to read everything in one fetch request.
		// Revise our estimate upwards.
		milliTestResultsPerTestVariant *= 4
	}

	return QueryResult{
		TestVariants:  results,
		NextPageToken: continuation,
	}, nil
}

type fetchOptions struct {
	// The target number of test variants to retrieve.
	targetTestVariants int
	// The assumed number of test results per test variant retrieved,
	// in thousandths of a test result per test variant.
	// If this is smaller than the actual number, fewer test variants
	// will be returned. If this is larger than the actual number,
	// more test variants than targetTestVariants may be returned.
	//
	// 1000 is the minimum value, to indicate one test result per test
	// variant.
	milliTestResultsPerTestVariant int
}

// fetch executes a query to retrieve test variants starting at
// the given PageToken. The implementation will target the retrieval
// of a specified number of test variants using the options provided
// but is only guaranteed to return at least one in each call to fetch
// (assuming there are test variants remaining to be read at all).
//
// For each test variant, the user-provided callback function
// is called. Return `complete` to stop iteration early.
//
// continuation returns a continuation token that may be used to
// coninue querying more test variants.
// If there are no more test variants to be read in the invocation,
// the continatuion token will be empty.
func (q *Query) fetch(ctx context.Context, start PageToken, opts fetchOptions, f func(tr *pb.TestVariant) error) (continuation PageToken, err error) {
	if opts.targetTestVariants < 1 {
		panic("opts.targetTestVariants < 1")
	}
	if opts.milliTestResultsPerTestVariant < 1000 {
		panic("opts.milliTestResultsPerTestVariant < 1000")
	}

	// Bound the estimated number of test results per test variant
	// to no more than q.ResultLimit as that is all we will read.
	milliTRPerTV := opts.milliTestResultsPerTestVariant
	if milliTRPerTV > q.ResultLimit*1000 {
		milliTRPerTV = q.ResultLimit * 1000
	}

	// Even if the estimates are perfect, we need one more result
	// than (# test variants) * (# avg test results/test variant),
	// so that we can identify we have read all test results
	// for the last test variant.
	limit := int(((int64(opts.targetTestVariants) * int64(milliTRPerTV)) / 1000) + 1)
	if limit < q.ResultLimit {
		// Ensure we query enough to be able to read one whole
		// test variant in the worst case.
		limit = q.ResultLimit
	}

	stmt := spanner.NewStatement(querySQL)
	stmt.Params = spanutil.ToSpannerMap(map[string]interface{}{
		"invID":            q.InvocationID,
		"afterTestId":      start.AfterTestID,
		"afterVariantHash": start.AfterVariantHash,
		"limit":            limit,
	})

	it := span.Query(ctx, stmt)

	// The continuation page token. Set based on the last test variant
	// yielded.
	continuation = start

	yield := func(testVariant *pb.TestVariant) error {
		// Update page token.
		continuation.AfterTestID = testVariant.TestId
		continuation.AfterVariantHash = testVariant.VariantHash

		if len(testVariant.Results) > q.ResultLimit {
			// Truncate results to set limit.
			testVariant.Results = testVariant.Results[:q.ResultLimit]
		}

		// Yield the current test variant.
		if err := f(testVariant); err != nil {
			if errors.Is(err, complete) {
				// Stop reading test variants.
				return complete
			}
			return err
		}
		return nil
	}

	var testVariants testVariantsStream
	testResultCount := 0

	err = unmarshalRows(q.InvocationID, it, func(tr *pb.TestResult) error {
		testResultCount++

		toYield := testVariants.Push(tr)
		if toYield != nil {
			// N.B. yield may update continuation token.
			err := yield(toYield)
			if err != nil {
				if errors.Is(err, complete) {
					// Stop reading test results.
					return complete
				}
				return errors.Annotate(err, "yield").Err()
			}
		}
		return nil
	})
	terminatedEarly := false
	if err != nil {
		if errors.Is(err, complete) {
			terminatedEarly = true
		} else {
			return PageToken{}, err
		}
	}

	// Whether we have read the last test result in the invocation.
	readAllResults := testResultCount < limit && !terminatedEarly

	// This last test variant will be either partial or complete
	// based on whether we have read all test results.
	lastTestVariant := testVariants.Finish()

	if lastTestVariant != nil && !terminatedEarly {
		// If we have a buffered test variant and we did not terminate
		// iteration early, yield it if either:
		// - We read the last test result in the invocation, or
		// - The test variant has at least q.ResultLimit results
		//   (avoids getting stuck on test variants which have
		//   more test results than our limit).
		if readAllResults || len(lastTestVariant.Results) >= q.ResultLimit {
			// N.B. yield may update continuation token.
			err := yield(lastTestVariant)
			if err != nil && !errors.Is(err, complete) {
				return PageToken{}, errors.Annotate(err, "yield").Err()
			}
		}
	}

	if readAllResults {
		// Finished.
		return PageToken{}, nil
	}

	return continuation, nil
}

// testVariantsStream converts a stream of test results (in
// test_id, variant_hash order) into a stream of test variants.
type testVariantsStream struct {
	// The partially populated test variant.
	partial *pb.TestVariant
}

// Push adds a new test result read from the database.
// If a previous test variant is now complete, it is returned.
func (e *testVariantsStream) Push(tr *pb.TestResult) *pb.TestVariant {
	var toYield *pb.TestVariant
	if e.partial != nil && (e.partial.TestId != tr.TestId || e.partial.VariantHash != tr.VariantHash) {
		// Started a new test variant. Yield the current one.
		toYield = e.partial
		e.partial = nil
	}
	if e.partial == nil {
		e.partial = &pb.TestVariant{
			TestId:       tr.TestId,
			VariantHash:  tr.VariantHash,
			Variant:      tr.Variant,
			TestMetadata: tr.TestMetadata,
		}
	}
	// Clear test result fields lifted to the test variant level.
	tr.TestId = ""
	tr.VariantHash = ""
	tr.Variant = nil
	tr.TestMetadata = nil

	e.partial.Results = append(e.partial.Results, &pb.TestResultBundle{
		Result: tr,
	})

	return toYield
}

// Finish notifies that all test results have been written
// to the stream.
//
// If the last test result in the invocation has been written,
// the test variant returned is complete.
// If not all test results have been written, the test variant
// returned may be partial.
// If no test variants were written, nil will be returned.
func (e *testVariantsStream) Finish() *pb.TestVariant {
	toYield := e.partial
	e.partial = nil
	return toYield
}

// unmarshalRows reads test results from the iterator.
// Inside the callback function, return `complete` to stop iterating
// early without error.
func unmarshalRows(invID invocations.ID, it *spanner.RowIterator, f func(tr *pb.TestResult) error) error {
	// Build a parser function.
	var b spanutil.Buffer
	var summaryHTML spanutil.Compressed
	var tmd spanutil.Compressed
	var fr spanutil.Compressed
	var properties spanutil.Compressed
	return it.Do(func(r *spanner.Row) error {
		tr := &pb.TestResult{}
		var isUnexpected spanner.NullBool
		var durationMicros spanner.NullInt64
		var skipReason spanner.NullInt64

		err := b.FromSpanner(r,
			&tr.TestId,
			&tr.VariantHash,
			&tr.Variant,
			&tr.ResultId,
			&isUnexpected,
			&tr.Status,
			&summaryHTML,
			&tr.StartTime,
			&durationMicros,
			&tr.Tags,
			&tmd,
			&fr,
			&properties,
			&skipReason,
		)
		if err != nil {
			return errors.Annotate(err, "unmarshal row").Err()
		}

		trName := pbutil.TestResultName(string(invID), tr.TestId, tr.ResultId)

		tr.Name = trName
		testresults.PopulateExpectedField(tr, isUnexpected)
		tr.SummaryHtml = string(summaryHTML)
		testresults.PopulateDurationField(tr, durationMicros)
		if err := testresults.PopulateTestMetadata(tr, tmd); err != nil {
			return errors.Annotate(err, "unmarshal test metadata for %s", trName).Err()
		}
		if err := testresults.PopulateFailureReason(tr, fr); err != nil {
			return errors.Annotate(err, "error unmarshalling failure_reason for %s", trName).Err()
		}
		if err := testresults.PopulateProperties(tr, properties); err != nil {
			return errors.Annotate(err, "failed to unmarshal properties").Err()
		}
		testresults.PopulateSkipReasonField(tr, skipReason)

		return f(tr)
	})
}

// estimateTestVariantSize estimates the size of a test variant in
// a pRPC response (pRPC responses use JSON serialisation).
func estimateTestVariantSize(tv *pb.TestVariant) int {
	// Estimate the size of a JSON-serialised test variant,
	// as the sum of the sizes of its fields, plus
	// an overhead (for JSON grammar and the field names).
	return 1000 + proto.Size(tv)
}

// This query design has been observed to be significantly more efficient for
// Spanner to execute than one in which we group test results to test variants
// and return a given number of test variants.
const querySQL = `
	SELECT
		TestId,
		VariantHash,
		Variant,
		ResultId,
		IsUnexpected,
		Status,
		SummaryHTML,
		StartTime,
		RunDurationUsec,
		Tags,
		TestMetadata,
		FailureReason,
		Properties,
		SkipReason
	FROM TestResults
	WHERE InvocationId = @invID AND (
		(TestId > @afterTestId) OR
		(TestId = @afterTestId AND VariantHash > @afterVariantHash)
	)
	-- Within a test variant, return the unexpected test results first as
	-- they are likely to be the most interesting.
	ORDER BY TestId, VariantHash, IsUnexpected DESC, ResultId
	LIMIT @limit
`
