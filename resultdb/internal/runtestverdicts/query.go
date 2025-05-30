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

// Package runtestverdicts provides methods to query test verdicts within a test run
// (single invocation excluding included invocation).
package runtestverdicts

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
// that iteration is complete and no more test verdicts should be retrieved.
var complete = errors.New("callback does not want any more test verdicts")

// Query specifies the run test verdicts to fetch.
type Query struct {
	InvocationID       invocations.ID
	PageSize           int // must be non-negative
	PageToken          PageToken
	ResultLimit        int // must be non-negative
	ResponseLimitBytes int // maximum response size in bytes (soft limit)
}

// QueryResult is the result of a Query.
type QueryResult struct {
	// The test verdicts retrieved.
	//
	// Within each test verdict, results are ordered with unexpected results
	// first, then by result ID.
	RunTestVerdicts []*pb.RunTestVerdict
	// The continuation token. If this is empty, then there are no more
	// test verdicts to retrieve.
	NextPageToken PageToken
}

// Run queries the run's test verdicts. It behaves deterministically,
// returning the maximum number of test verdicts consistent with the
// requested PageSize and ResponseLimitBytes, in ascending (test, variantHash)
// order.
//
// Must be run in a transactional spanner context supporting multiple reads.
func (q *Query) Run(ctx context.Context) (result QueryResult, err error) {
	ctx, ts := tracing.Start(ctx, "runtestverdicts.Query.Run",
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

	var results []*pb.RunTestVerdict
	var responseSize int

	continuation := q.PageToken
	done := false

	// Assume 1.1 test results per test verdict.
	milliTestResultsPerTestVerdict := 1100
	for !done {
		opts := fetchOptions{
			targetTestVerdicts:             q.PageSize - len(results),
			milliTestResultsPerTestVerdict: milliTestResultsPerTestVerdict,
		}

		// Fetch the next batch of test verdicts.
		continuation, err = q.fetch(ctx, continuation, opts, func(tv *pb.RunTestVerdict) error {
			results = append(results, tv)

			// Check if we have reached our soft response size limit.
			// We check this after appending to ensure we always
			// return at least one test verdict on each Query.
			responseSize += estimateRunTestVerdictSize(tv)
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
			// Empty page token indicates we have read all test verdicts
			// in the invocation.
			done = true
		}

		// If we are not yet done, then our estimate of test results per test
		// verdict was too low to read everything in one fetch request.
		// Revise our estimate upwards.
		milliTestResultsPerTestVerdict *= 4
	}

	return QueryResult{
		RunTestVerdicts: results,
		NextPageToken:   continuation,
	}, nil
}

type fetchOptions struct {
	// The target number of test verdicts to retrieve.
	targetTestVerdicts int
	// The assumed number of test results per test verdict retrieved,
	// in thousandths of a test result per test verdict.
	// If this is smaller than the actual number, fewer test verdicts
	// will be returned. If this is larger than the actual number,
	// more test verdicts than targetTestVerdicts may be returned.
	//
	// 1000 is the minimum value, to indicate one test result per test
	// verdict.
	milliTestResultsPerTestVerdict int
}

// fetch executes a query to retrieve test verdicts starting at
// the given PageToken. The implementation will target the retrieval
// of a specified number of test verdicts using the options provided
// but is only guaranteed to return at least one in each call to fetch
// (assuming there are test verdicts remaining to be read at all).
//
// For each test verdict, the user-provided callback function
// is called. Return `complete` to stop iteration early.
//
// continuation returns a continuation token that may be used to
// coninue querying more test verdicts.
// If there are no more test verdicts to be read in the invocation,
// the continatuion token will be empty.
func (q *Query) fetch(ctx context.Context, start PageToken, opts fetchOptions, f func(tr *pb.RunTestVerdict) error) (continuation PageToken, err error) {
	if opts.targetTestVerdicts < 1 {
		panic("opts.targetTestVerdicts < 1")
	}
	if opts.milliTestResultsPerTestVerdict < 1000 {
		panic("opts.milliTestResultsPerTestVerdict < 1000")
	}

	// Bound the estimated number of test results per test verdict
	// to no more than q.ResultLimit as that is all we will read.
	milliTRPerTV := opts.milliTestResultsPerTestVerdict
	if milliTRPerTV > q.ResultLimit*1000 {
		milliTRPerTV = q.ResultLimit * 1000
	}

	// Even if the estimates are perfect, we need one more result
	// than (# test verdict) * (# avg test results/test verdict),
	// so that we can identify we have read all test results
	// for the last test verdict.
	limit := int(((int64(opts.targetTestVerdicts) * int64(milliTRPerTV)) / 1000) + 1)
	if limit < q.ResultLimit {
		// Ensure we query enough to be able to read one whole
		// test verdict in the worst case.
		limit = q.ResultLimit
	}

	stmt := spanner.NewStatement(querySQL)
	stmt.Params = spanutil.ToSpannerMap(map[string]any{
		"invID":            q.InvocationID,
		"afterTestId":      start.AfterTestID,
		"afterVariantHash": start.AfterVariantHash,
		"limit":            limit,
	})

	it := span.Query(ctx, stmt)

	// The continuation page token. Set based on the last test verdict
	// yielded.
	continuation = start

	yield := func(testVerdict *pb.RunTestVerdict) error {
		// Update page token.
		continuation.AfterTestID = testVerdict.TestId
		continuation.AfterVariantHash = testVerdict.VariantHash

		if len(testVerdict.Results) > q.ResultLimit {
			// Truncate results to set limit.
			testVerdict.Results = testVerdict.Results[:q.ResultLimit]
		}

		// Yield the current test verdict.
		if err := f(testVerdict); err != nil {
			if errors.Is(err, complete) {
				// Stop reading test verdicts.
				return complete
			}
			return err
		}
		return nil
	}

	var testVerdicts testVerdictsStream
	testResultCount := 0

	err = unmarshalRows(q.InvocationID, it, func(tr *pb.TestResult) error {
		testResultCount++

		toYield := testVerdicts.Push(tr)
		if toYield != nil {
			// N.B. yield may update continuation token.
			err := yield(toYield)
			if err != nil {
				if errors.Is(err, complete) {
					// Stop reading test results.
					return complete
				}
				return errors.Fmt("yield: %w", err)
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

	// This last test verdict will be either partial or complete
	// based on whether we have read all test results.
	lastTestVerdict := testVerdicts.Finish()

	if lastTestVerdict != nil && !terminatedEarly {
		// If we have a buffered test verdict and we did not terminate
		// iteration early, yield it if either:
		// - We read the last test result in the invocation, or
		// - The test verdict has at least q.ResultLimit results
		//   (avoids getting stuck on test verdicts which have
		//   more test results than our limit).
		if readAllResults || len(lastTestVerdict.Results) >= q.ResultLimit {
			// N.B. yield may update continuation token.
			err := yield(lastTestVerdict)
			if err != nil && !errors.Is(err, complete) {
				return PageToken{}, errors.Fmt("yield: %w", err)
			}
		}
	}

	if readAllResults {
		// Finished.
		return PageToken{}, nil
	}

	return continuation, nil
}

// testVerdictsStream converts a stream of test results (in
// test_id, variant_hash order) into a stream of test verdicts.
type testVerdictsStream struct {
	// The partially populated test verdict.
	partial *pb.RunTestVerdict
}

// Push adds a new test result read from the database.
// If a previous test verdict is now complete, it is returned.
func (e *testVerdictsStream) Push(tr *pb.TestResult) *pb.RunTestVerdict {
	var toYield *pb.RunTestVerdict
	if e.partial != nil && (e.partial.TestId != tr.TestId || e.partial.VariantHash != tr.VariantHash) {
		// Started a new test verdict. Yield the current one.
		toYield = e.partial
		e.partial = nil
	}
	if e.partial == nil {
		e.partial = &pb.RunTestVerdict{
			TestId:       tr.TestId,
			VariantHash:  tr.VariantHash,
			Variant:      tr.Variant,
			TestMetadata: tr.TestMetadata,
		}
	}
	// Clear test result fields lifted to the test verdict level.
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
// the test verdict returned is complete.
// If not all test results have been written, the test verdict
// returned may be partial.
// If no test results have been written, nil will be returned.
func (e *testVerdictsStream) Finish() *pb.RunTestVerdict {
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
	var skippedReason spanutil.Compressed
	var frameworkExtensions spanutil.Compressed
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
			&tr.StatusV2,
			&summaryHTML,
			&tr.StartTime,
			&durationMicros,
			&tr.Tags,
			&tmd,
			&fr,
			&properties,
			&skipReason,
			&skippedReason,
			&frameworkExtensions,
		)
		if err != nil {
			return errors.Fmt("unmarshal row: %w", err)
		}

		trName := pbutil.TestResultName(string(invID), tr.TestId, tr.ResultId)

		tr.Name = trName
		testresults.PopulateExpectedField(tr, isUnexpected)
		tr.SummaryHtml = string(summaryHTML)
		testresults.PopulateDurationField(tr, durationMicros)
		if err := testresults.PopulateTestMetadata(tr, tmd); err != nil {
			return errors.Fmt("unmarshal test metadata for %s: %w", trName, err)
		}
		if err := testresults.PopulateFailureReason(tr, fr); err != nil {
			return errors.Fmt("error unmarshalling failure_reason for %s: %w", trName, err)
		}
		if err := testresults.PopulateProperties(tr, properties); err != nil {
			return errors.Fmt("unmarshal properties for %s: %w", trName, err)
		}
		testresults.PopulateSkipReasonField(tr, skipReason)
		if err := testresults.PopulateSkippedReason(tr, skippedReason); err != nil {
			return errors.Fmt("unmarshal skipped reason for %s: %w", trName, err)
		}
		if err := testresults.PopulateFrameworkExtensions(tr, frameworkExtensions); err != nil {
			return errors.Fmt("unmarshal framework extensions for %s: %w", trName, err)
		}
		return f(tr)
	})
}

// estimateRunTestVerdictSize estimates the size of a run test verdict in
// a pRPC response (pRPC responses use JSON serialisation).
func estimateRunTestVerdictSize(tv *pb.RunTestVerdict) int {
	// Estimate the size of a JSON-serialised test verdict,
	// as the sum of the sizes of its fields, plus
	// an overhead (for JSON grammar and the field names).
	return 1000 + proto.Size(tv)
}

// This query design has been observed to be significantly more efficient for
// Spanner to execute than one in which we group test results to test verdicts
// and limit to a given number of test verdicts.
const querySQL = `
	SELECT
		TestId,
		VariantHash,
		Variant,
		ResultId,
		IsUnexpected,
		Status,
		StatusV2,
		SummaryHTML,
		StartTime,
		RunDurationUsec,
		Tags,
		TestMetadata,
		FailureReason,
		Properties,
		SkipReason,
		SkippedReason,
		FrameworkExtensions
	FROM TestResults
	WHERE InvocationId = @invID AND (
		(TestId > @afterTestId) OR
		(TestId = @afterTestId AND VariantHash > @afterVariantHash)
	)
	-- Within a test verdict, return the unexpected test results first as
	-- they are likely to be the most interesting.
	ORDER BY TestId, VariantHash, IsUnexpected DESC, ResultId
	LIMIT @limit
`
