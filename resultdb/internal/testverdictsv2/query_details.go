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

	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testexonerationsv2"
	"go.chromium.org/luci/resultdb/internal/testresultsv2"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// QueryDetails represents a query for test verdicts that is based on
// simple iterators over test results and exoneration rows.
//
// It is more efficient than aggregating verdicts in Spanner but has
// fewer options. Crucially, it is performant enough to query detail
// fields like test results and exonerations.
type QueryDetails struct {
	// The root invocation.
	RootInvocationID rootinvocations.ID
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
	VerdictIDs []testresultsv2.VerdictID
}

// FetchOptions specifies options for fetching a page of test verdicts.
type FetchOptions struct {
	// The maximum number of test verdicts to return per page.
	PageSize int
	// The maximum number of test results or test exonerations to return per verdict
	// (limits are applied separately to test results and exonerations).
	ResultLimit int
	// The limit on the number of bytes that should be returned in a page.
	// Row size is estimated as proto.Size() + protoJSONOverheadBytes bytes (to allow for
	// alternative encodings which higher fixed overheads than wire proto, e.g. protojson).
	// If this is zero, no limit is applied.
	ResponseLimitBytes int
}

const protoJSONOverheadBytes = 1000

// Fetch fetches a page of verdicts starting at the given page token.
func (q *QueryDetails) Fetch(ctx context.Context, pageToken testresultsv2.VerdictID, opts FetchOptions) ([]*pb.TestVerdict, testresultsv2.VerdictID, error) {
	if opts.PageSize <= 0 {
		return nil, testresultsv2.VerdictID{}, errors.New("page size must be positive")
	}
	if opts.ResultLimit <= 0 {
		return nil, testresultsv2.VerdictID{}, errors.New("result limit must be positive")
	}
	if opts.ResponseLimitBytes < 0 {
		return nil, testresultsv2.VerdictID{}, errors.New("if set, response limit bytes must be positive")
	}

	var results []*pb.TestVerdict
	var nextPageToken testresultsv2.VerdictID
	var totalSize int
	it := q.List(ctx, pageToken, opts.PageSize)
	err := it.Do(func(tv *TestVerdict) error {
		result := tv.ToProto(opts.ResultLimit)

		if opts.ResponseLimitBytes != 0 {
			// Apply response size limiting by bytes.
			// Estimate size as per comment on FetchOptions.ResponseLimitBytes.
			resultSize := proto.Size(result) + protoJSONOverheadBytes
			if (totalSize + resultSize) > opts.ResponseLimitBytes {
				if len(results) == 0 {
					// This should not normally happen.
					return errors.Fmt("a single verdict (%v bytes) was larger than the total response limit (%v bytes)", resultSize, opts.ResponseLimitBytes)
				}
				// Stop iteration before appending the result.
				return iterator.Done
			}
			totalSize += resultSize
		}

		results = append(results, result)
		nextPageToken = tv.ID
		if len(results) >= opts.PageSize {
			// We have met the page size target. Stop iteration.
			return iterator.Done
		}
		return nil
	})
	if err != nil && err != iterator.Done {
		return nil, testresultsv2.VerdictID{}, err
	}
	// If we did not terminate early, the iterator has been exhausted.
	if err == nil {
		nextPageToken = testresultsv2.VerdictID{}
	}
	return results, nextPageToken, nil
}

// statusV2FromResults computes the verdict status (v2) based on
// the test results that are part of the verdict.
func statusV2FromResults(results []*testresultsv2.TestResultRow) pb.TestVerdict_Status {
	var (
		passedCount, failedCount, skippedCount, executionErroredCount, precludedCount int
	)
	for _, result := range results {
		switch result.StatusV2 {
		case pb.TestResult_PASSED:
			passedCount++
		case pb.TestResult_FAILED:
			failedCount++
		case pb.TestResult_SKIPPED:
			skippedCount++
		case pb.TestResult_EXECUTION_ERRORED:
			executionErroredCount++
		case pb.TestResult_PRECLUDED:
			precludedCount++
		}
	}
	switch {
	case passedCount > 0 && failedCount > 0:
		return pb.TestVerdict_FLAKY
	case passedCount > 0 && failedCount == 0:
		return pb.TestVerdict_PASSED
	case passedCount == 0 && failedCount > 0:
		return pb.TestVerdict_FAILED
	// If we fall through this far, there are no passing or failing results.
	case skippedCount > 0:
		return pb.TestVerdict_SKIPPED
	// If we fall through this far, there are no passing, failing or skipped results.
	case executionErroredCount > 0:
		return pb.TestVerdict_EXECUTION_ERRORED
	// If we fall through this far, there are only precluded results.
	default:
		return pb.TestVerdict_PRECLUDED
	}
}

// List returns an iterator that lists test verdicts starting from
// the given pageToken. bufferSize is an estimate of the number of
// test verdicts that will be retrieved using the List method.
//
// Using the iterator, rows are streamed from Spanner which prevents
// the need to keep the entire result set in memory.
func (q *QueryDetails) List(ctx context.Context, pageToken testresultsv2.VerdictID, bufferSize int) *Iterator {
	if bufferSize < 100 {
		bufferSize = 100
	}
	trQuery := testresultsv2.Query{
		RootInvocation:   q.RootInvocationID,
		TestPrefixFilter: q.TestPrefixFilter,
		VerdictIDs:     q.VerdictIDs,
	}
	trPageToken := testresultsv2.ID{
		RootInvocationShardID: pageToken.RootInvocationShardID,
		ModuleName:            pageToken.ModuleName,
		ModuleScheme:          pageToken.ModuleScheme,
		ModuleVariantHash:     pageToken.ModuleVariantHash,
		CoarseName:            pageToken.CoarseName,
		FineName:              pageToken.FineName,
		CaseName:              pageToken.CaseName,
		// We want to start *after* the last verdict.
		// U+10FFFF is the maximum unicode character and will sort after
		// all valid WorkUnits.
		WorkUnitID: "\U0010FFFF",
		ResultID:   "",
	}
	trOpts := spanutil.BufferingOptions{
		// System-wise, the average number of test results per verdict is about 1.1.
		// Here we request slightly more to reduce the chance we need a second page.
		FirstPageSize: bufferSize * (12 / 10),
		// If the first page isn't enough, we probably only need a small amount more.
		SecondPageSize: bufferSize / 2,
		// Thereafter, we are dealing with significant outliers or we might have
		// made a significant error in our estimate. Double the page size each time
		// to avoid perpetually underestimating.
		GrowthFactor: 2.0,
	}
	trIterator := trQuery.List(ctx, trPageToken, trOpts)

	teQuery := testexonerationsv2.Query{
		RootInvocation:   q.RootInvocationID,
		TestPrefixFilter: q.TestPrefixFilter,
		VerdictIDs:     q.VerdictIDs,
	}
	tePageToken := testexonerationsv2.ID{
		RootInvocationShardID: pageToken.RootInvocationShardID,
		ModuleName:            pageToken.ModuleName,
		ModuleScheme:          pageToken.ModuleScheme,
		ModuleVariantHash:     pageToken.ModuleVariantHash,
		CoarseName:            pageToken.CoarseName,
		FineName:              pageToken.FineName,
		CaseName:              pageToken.CaseName,
		// We want to start *after* the last verdict.
		// U+10FFFF is the maximum unicode character and will sort after
		// all valid WorkUnits.
		WorkUnitID:    "\U0010FFFF",
		ExonerationID: "",
	}
	teOpts := spanutil.BufferingOptions{
		// System-wise, about a half percent of verdicts are exonerated.
		FirstPageSize: bufferSize / 100,
		// If the first page isn't enough, maybe we hit a large block of exonerations.
		SecondPageSize: bufferSize,
		// Thereafter, we are dealing with significant outliers or we might have
		// made a significant error in our estimate. Double the page size each time
		// to avoid perpetually underestimating.
		GrowthFactor: 2.0,
	}
	teIterator := teQuery.List(ctx, tePageToken, teOpts)

	return &Iterator{
		testResults:      spanutil.NewPeekingIterator(trIterator),
		testExonerations: spanutil.NewPeekingIterator(teIterator),
	}
}

// Iterator iterates over test verdicts in a root invocation.
//
// Internally, it implements a merge join between test result and test exoneration
// iterators.
type Iterator struct {
	testResults      *spanutil.PeekingIterator[*testresultsv2.TestResultRow]
	testExonerations *spanutil.PeekingIterator[*testexonerationsv2.TestExonerationRow]
}

// Next returns the next test verdict.
// If there are no more test verdicts, iterator.Done is returned.
func (i *Iterator) Next() (*TestVerdict, error) {
	// Peek at the next test result.
	res, err := i.testResults.Peek()
	if err != nil {
		if err == iterator.Done {
			return nil, iterator.Done
		}
		return nil, err
	}

	// Identify the verdict we are about to construct.
	verdictID := res.ID.VerdictID()
	verdict := &TestVerdict{
		ID:           verdictID,
		TestMetadata: res.TestMetadata,
	}

	// Consume all test results for this verdict.
	for {
		r, err := i.testResults.Peek()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		if r.ID.VerdictID() != verdictID {
			// Test result is for a future verdict. Do not consume it.
			break
		}

		// Consume the result.
		verdict.Results = append(verdict.Results, r)
		if _, err := i.testResults.Next(); err != nil {
			return nil, err
		}
	}

	// Consume all test exonerations for this verdict.
	// Note: Exonerations are also ordered by verdict ID.
	// We skip exonerations that appear before the current verdict ID
	// (i.e. exonerations for verdicts that have no test results).
	for {
		e, err := i.testExonerations.Peek()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		eID := e.ID.VerdictID()
		cmp := eID.Compare(verdictID)
		if cmp < 0 {
			// Exoneration for a verdict we have already passed (never saw results for).
			// Skip it.
			if _, err := i.testExonerations.Next(); err != nil {
				return nil, err
			}
			continue
		}
		if cmp > 0 {
			// Exoneration is for a future verdict. Do not consume it.
			break
		}

		// Consume the exoneration.
		verdict.Exonerations = append(verdict.Exonerations, e)
		if _, err := i.testExonerations.Next(); err != nil {
			return nil, err
		}
	}
	return verdict, nil
}

// Do calls the given callback function for each test verdict in the iterator,
// or until the callback function returns an error.
func (i *Iterator) Do(f func(*TestVerdict) error) error {
	for {
		tv, err := i.Next()
		if err == iterator.Done {
			// Reached end of iterator.
			return nil
		}
		if err != nil {
			return err
		}
		if err := f(tv); err != nil {
			return err
		}
	}
}

// Stop terminates the iteration. It should be called after you finish using the iterator.
func (i *Iterator) Stop() {
	i.testResults.Stop()
	i.testExonerations.Stop()
}
