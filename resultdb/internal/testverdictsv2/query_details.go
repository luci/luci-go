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

	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testexonerationsv2"
	"go.chromium.org/luci/resultdb/internal/testresultsv2"
	"go.chromium.org/luci/resultdb/internal/tracing"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// IteratorQuery represents a query for test verdicts that is based on
// simple iterators over test results and exoneration rows.
//
// It is more efficient than aggregating verdicts in Spanner but has
// fewer options. Crucially, it is performant enough to query detail
// fields like test results and exonerations.
type IteratorQuery struct {
	// The root invocation.
	RootInvocationID rootinvocations.ID
	// The test prefix filter to apply.
	TestPrefixFilter *pb.TestIdentifierPrefix
	// The specific verdicts to filter to. If this is set, both
	// RootInvocationID and TestPrefixFilter are ignored.
	//
	// Verdicts will be returned in the same order as this list.
	// Duplicates are allowed, and will result in the same verdict
	// being returned multiple times. If some verdicts are missing,
	// placeholder `TestVerdicts`(s) will be returned. The
	// `RequestOrdinal` field on returned Verdicts will identify
	// which verdict (from this list) is being returned.
	//
	// At most 10,000 IDs can be nominated (see "Values in an IN operator"):
	// https://docs.cloud.google.com/spanner/quotas#query-limits
	VerdictIDs []testresultsv2.VerdictID
	// The access the caller has to the root invocation.
	Access permissions.RootInvocationAccess
}

// protoJSONOverheadBytes is the assumed overhead of a protojson-encoding a TestVerdict proto,
// beyond its wire proto-encoded size.
const protoJSONOverheadBytes = 1000

// IteratorFetchOptions represents options for fetching verdicts.
type IteratorFetchOptions struct {
	FetchOptions
	// The predicate to apply to each verdict.
	// If non-nil, it is called for each verdict. If it returns false, the verdict is
	// skipped, but it is still counted towards `FetchOptions.TotalResultLimit`.
	Predicate func(*TestVerdict) bool
}

// Fetch fetches a page of verdicts starting at the given page token.
func (q *IteratorQuery) Fetch(ctx context.Context, pageToken PageToken, opts IteratorFetchOptions) ([]*pb.TestVerdict, PageToken, error) {
	if err := opts.Validate(); err != nil {
		return nil, PageToken{}, err
	}

	var verdicts []*pb.TestVerdict
	var nextPageToken PageToken
	var totalSize int

	// The underlying iterator always needs to peek ahead one result, to cut
	// a verdict. So we start with a count of 1.
	totalResults := 1
	it := q.List(ctx, pageToken, opts.PageSize)
	err := it.Do(ctx, func(tv *TestVerdict) error {
		if opts.Predicate == nil || opts.Predicate(tv) {
			// Add the verdict to the page.
			verdict := tv.ToProto(opts.VerdictResultLimit, opts.VerdictSizeLimit)

			if opts.ResponseLimitBytes != 0 {
				// Apply response size limiting by bytes.
				// Estimate size as per comment on FetchOptions.ResponseLimitBytes.
				resultSize := proto.Size(verdict) + protoJSONOverheadBytes
				if (totalSize + resultSize) > opts.ResponseLimitBytes {
					if len(verdicts) == 0 {
						// This should not normally happen.
						return errors.Fmt("a single verdict (%v bytes) was larger than the total response limit (%v bytes)", resultSize, opts.ResponseLimitBytes)
					}
					// Stop iteration before appending the result.
					return iterator.Done
				}
				totalSize += resultSize
			}

			verdicts = append(verdicts, verdict)
		}
		// Advance the page token, even if the verdict was not used on the page.
		// This ensures that pagination will eventually progress through large regions
		// of verdicts that don't match the predicate, even if some pages are empty
		// because opts.TotalResultLimit was hit.
		nextPageToken = makePageToken(tv)

		if len(verdicts) >= opts.PageSize {
			// We have met the page size target. Stop iteration.
			return iterator.Done
		}
		totalResults += len(tv.Results)
		if opts.TotalResultLimit != 0 && totalResults >= opts.TotalResultLimit {
			// We have exceeded the total result limit, so end early.
			return iterator.Done
		}
		return nil
	})
	if err != nil && err != iterator.Done {
		return nil, PageToken{}, err
	}
	// If we did not terminate early, the iterator has been exhausted.
	if err == nil {
		nextPageToken = PageToken{}
	}
	return verdicts, nextPageToken, nil
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
func (q *IteratorQuery) List(ctx context.Context, pageToken PageToken, bufferSize int) *Iterator {
	if bufferSize <= 0 {
		bufferSize = 1_000 // Set a sensible default size.
	}
	trQuery := testresultsv2.Query{
		RootInvocation:   q.RootInvocationID,
		TestPrefixFilter: q.TestPrefixFilter,
		VerdictIDs:       q.VerdictIDs,
		Access:           q.Access,
		Order:            testresultsv2.OrderingByPrimaryKey,
	}
	trPageToken := pageToken.toTestResultsPageToken()
	trOpts := spanutil.BufferingOptions{
		// System-wide, the average number of test results per verdict is about 1.1.
		// Here we request slightly more to reduce the chance we need a second page.
		// Add one as our iterator needs one extra result to confirm it has finished
		// reading the verdict.
		FirstPageSize: bufferSize*(12/10) + 1,
		// If the first page isn't enough, get another similar amount.
		SecondPageSize: bufferSize,
		// Thereafter, we are dealing with significant outliers or we might have
		// made a significant error in our estimate. Double the page size each time
		// to avoid perpetually underestimating.
		GrowthFactor: 2.0,
		MaxPageSize:  testresultsv2.MaxTestResultsPageSize,
	}
	if q.VerdictIDs != nil {
		// If doing a query for nominated verdict IDs, clients typically try to page
		// through all verdicts they requested. There is no point starting with a small
		// page size to avoid reading more than necessary; we want to read everything
		// as fast as we can.
		trOpts.FirstPageSize = testresultsv2.MaxTestResultsPageSize
		trOpts.SecondPageSize = testresultsv2.MaxTestResultsPageSize
	}
	trIterator := trQuery.List(ctx, trPageToken, trOpts)

	teQuery := testexonerationsv2.Query{
		RootInvocation:   q.RootInvocationID,
		TestPrefixFilter: q.TestPrefixFilter,
		VerdictIDs:       q.VerdictIDs,
		Access:           q.Access,
		Order:            testresultsv2.OrderingByPrimaryKey,
	}
	tePageToken := pageToken.toTestExonerationsPageToken()
	teOpts := spanutil.BufferingOptions{
		// System-wide, about a half percent of verdicts are exonerated, here we
		// query 1%. Add one as our iterator always needs one more exoneration to
		// confirm it has finished reading the verdict.
		FirstPageSize: (bufferSize / 100) + 1,
		// If the first page isn't enough, maybe we hit a large block of exonerations.
		SecondPageSize: bufferSize,
		// Thereafter, we are dealing with significant outliers or we might have
		// made a significant error in our estimate. Double the page size each time
		// to avoid perpetually underestimating.
		GrowthFactor: 2.0,
		MaxPageSize:  testexonerationsv2.MaxTestExonerationsPageSize,
	}
	if q.VerdictIDs != nil {
		// If doing a query for nominated verdict IDs, clients typically try to page
		// through all verdicts they requested. There is no point starting with a small
		// page size to avoid reading more than necessary; we want to read everything
		// as fast as we can.
		teOpts.FirstPageSize = testexonerationsv2.MaxTestExonerationsPageSize
		teOpts.SecondPageSize = testexonerationsv2.MaxTestExonerationsPageSize
	}
	teIterator := teQuery.List(ctx, tePageToken, teOpts)

	return &Iterator{
		testResults:        spanutil.NewPeekingIterator(trIterator),
		testExonerations:   spanutil.NewPeekingIterator(teIterator),
		lastRequestOrdinal: pageToken.RequestOrdinal,
	}
}

// Iterator iterates over test verdicts in a root invocation.
//
// Internally, it implements a merge join between test result and test exoneration
// iterators.
type Iterator struct {
	testResults        *spanutil.PeekingIterator[*testresultsv2.TestResultRow]
	testExonerations   *spanutil.PeekingIterator[*testexonerationsv2.TestExonerationRow]
	lastRequestOrdinal int
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

	if res.RequestOrdinal != 0 && res.RequestOrdinal > (i.lastRequestOrdinal+1) {
		// We are querying specific verdicts via q.VerdictIDs and one or more
		// verdicts are missing. Return a placeholder verdict in place of
		// the missing verdict.
		i.lastRequestOrdinal++
		return &TestVerdict{RequestOrdinal: i.lastRequestOrdinal}, nil
	}

	// Identify the verdict we are about to construct.
	position := positionFromResult(res)
	verdict := &TestVerdict{
		ID:             res.ID.VerdictID(),
		RequestOrdinal: res.RequestOrdinal,
	}
	i.lastRequestOrdinal = res.RequestOrdinal

	// Consume all test results for this verdict.
	for {
		r, err := i.testResults.Peek()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("peek next test result: %w", err)
		}
		if positionFromResult(r) != position {
			// Test result is for a future verdict. Do not consume it.
			break
		}

		// Consume the result.
		verdict.Results = append(verdict.Results, r)
		if _, err := i.testResults.Next(); err != nil {
			return nil, fmt.Errorf("consume test result: %w", err)
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
			return nil, fmt.Errorf("peek next exoneration: %w", err)
		}

		ePosition := positionFromExoneration(e)
		cmp := ePosition.Compare(position)
		if cmp < 0 {
			// Exoneration for a verdict we have already passed (never saw results for).
			// Skip it.
			if _, err := i.testExonerations.Next(); err != nil {
				return nil, fmt.Errorf("consume test exoneration: %w", err)
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
			return nil, fmt.Errorf("consume test exoneration: %w", err)
		}
	}

	verdict.Status = statusV2FromResults(verdict.Results)
	isExonerable := (verdict.Status == pb.TestVerdict_FAILED ||
		verdict.Status == pb.TestVerdict_EXECUTION_ERRORED ||
		verdict.Status == pb.TestVerdict_PRECLUDED ||
		verdict.Status == pb.TestVerdict_FLAKY)
	if len(verdict.Exonerations) > 0 && isExonerable {
		verdict.StatusOverride = pb.TestVerdict_EXONERATED
	} else {
		verdict.StatusOverride = pb.TestVerdict_NOT_OVERRIDDEN
	}

	return verdict, nil
}

// Do calls the given callback function for each test verdict in the iterator,
// or until the callback function returns an error. It takes care of calling
// Stop.
func (i *Iterator) Do(ctx context.Context, f func(*TestVerdict) error) (err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/testverdictsv2.Iterator.Do")
	defer func() { tracing.End(ts, err) }()

	defer i.Stop()
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

// IteratorPosition represents the position of the verdict iterator.
type IteratorPosition struct {
	// Either of the following will be specified; not both.
	ID testresultsv2.VerdictID

	// The one-based index into VerdictIDs that represents the last returned verdict.
	// Only set if QueryDetails.VerdictIDs is set.
	// Used to keep position in case of a duplicated ID(s) in QueryDetails.VerdictIDs.
	RequestOrdinal int
}

// Compare returns -1 iff t < other, 0 iff t == other and 1 iff t > other
// in TestResultV2 / TestExonerationV2 table order.
func (t IteratorPosition) Compare(other IteratorPosition) int {
	if t.RequestOrdinal > 0 || other.RequestOrdinal > 0 {
		// Retrieiving nominated verdicts.
		if t.RequestOrdinal != other.RequestOrdinal {
			if t.RequestOrdinal < other.RequestOrdinal {
				return -1
			}
			return 1
		}
		return 0
	}
	// Retrieving verdicts in ID order.
	return t.ID.Compare(other.ID)
}

func positionFromResult(result *testresultsv2.TestResultRow) IteratorPosition {
	if result.RequestOrdinal > 0 {
		// Page token used when retrieiving nominated verdicts.
		return IteratorPosition{RequestOrdinal: result.RequestOrdinal}
	} else {
		return IteratorPosition{ID: result.ID.VerdictID()}
	}
}

func positionFromExoneration(exoneration *testexonerationsv2.TestExonerationRow) IteratorPosition {
	if exoneration.RequestOrdinal > 0 {
		// Page token used when retrieiving nominated verdicts.
		return IteratorPosition{RequestOrdinal: exoneration.RequestOrdinal}
	} else {
		return IteratorPosition{ID: exoneration.ID.VerdictID()}
	}
}
