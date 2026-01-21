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

	"go.chromium.org/luci/resultdb/internal/permissions"
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

// FetchOptions specifies options for fetching a page of test verdicts.
type FetchOptions struct {
	// The maximum number of test verdicts to return per page.
	PageSize int
	// The limit on the number of bytes that should be returned in a page.
	// Row size is estimated as proto.Size() + protoJSONOverheadBytes bytes (to allow for
	// alternative encodings which higher fixed overheads than wire proto, e.g. protojson).
	// If this is zero, no limit is applied.
	ResponseLimitBytes int

	// The limit on the number of test results and test exonerations to return per verdict.
	// The limit is applied independently to test results and exonerations.
	VerdictResultLimit int
	// The size limit, in bytes, of each test verdict.
	VerdictSizeLimit int
}

// PageToken represents the token for a page of (detailed) verdicts.
type PageToken struct {
	// Either of the following will be specified; not both.

	// The verdict ID of the last returned verdict.
	ID testresultsv2.VerdictID
	// The one-based index into VerdictIDs that represents the last returned verdict.
	// Only set if QueryDetails.VerdictIDs is set.
	// Used to keep position in case of a duplicated ID(s) in QueryDetails.VerdictIDs.
	RequestOrdinal int
}

// protoJSONOverheadBytes is the assumed overhead of a protojson-encoding a TestVerdict proto,
// beyond its wire proto-encoded size.
const protoJSONOverheadBytes = 1000

// Fetch fetches a page of verdicts starting at the given page token.
func (q *QueryDetails) Fetch(ctx context.Context, pageToken PageToken, opts FetchOptions) ([]*pb.TestVerdict, PageToken, error) {
	if opts.PageSize <= 0 {
		return nil, PageToken{}, errors.New("page size must be positive")
	}
	if opts.VerdictResultLimit <= 0 {
		return nil, PageToken{}, errors.New("verdict result limit must be positive")
	}
	if opts.VerdictSizeLimit <= 0 {
		return nil, PageToken{}, errors.New("verdict size limit must be positive")
	}
	if opts.ResponseLimitBytes < 0 {
		return nil, PageToken{}, errors.New("if set, response limit bytes must be positive")
	}

	var results []*pb.TestVerdict
	var nextPageToken PageToken
	var totalSize int
	it := q.List(ctx, pageToken, opts.PageSize)
	err := it.Do(func(tv *TestVerdict) error {
		result := tv.ToProto(opts.VerdictResultLimit, opts.VerdictSizeLimit)

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
		nextPageToken = pageTokenFromVerdict(tv)
		if len(results) >= opts.PageSize {
			// We have met the page size target. Stop iteration.
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
func (q *QueryDetails) List(ctx context.Context, pageToken PageToken, bufferSize int) *Iterator {
	if bufferSize <= 0 {
		bufferSize = 1_000 // Set a sensible default size.
	}
	trQuery := testresultsv2.Query{
		RootInvocation:   q.RootInvocationID,
		TestPrefixFilter: q.TestPrefixFilter,
		VerdictIDs:       q.VerdictIDs,
		Access:           q.Access,
	}
	trPageToken := pageToken.toTestResultsPageToken()
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
	if trOpts.SecondPageSize < 1 {
		trOpts.SecondPageSize = 1
	}
	trIterator := trQuery.List(ctx, trPageToken, trOpts)

	teQuery := testexonerationsv2.Query{
		RootInvocation:   q.RootInvocationID,
		TestPrefixFilter: q.TestPrefixFilter,
		VerdictIDs:       q.VerdictIDs,
		Access:           q.Access,
	}
	tePageToken := pageToken.toTestExonerationsPageToken()
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
	if teOpts.FirstPageSize < 1 {
		teOpts.FirstPageSize = 1
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
	verdictToken := pageTokenFromResult(res)
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
			return nil, err
		}
		if pageTokenFromResult(r) != verdictToken {
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

		eToken := pageTokenFromExoneration(e)
		cmp := eToken.Compare(verdictToken)
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
// or until the callback function returns an error. It takes care of calling
// Stop.
func (i *Iterator) Do(f func(*TestVerdict) error) error {
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

// Compare returns -1 iff t < other, 0 iff t == other and 1 iff t > other
// in TestResultV2 / TestExonerationV2 table order.
func (t PageToken) Compare(other PageToken) int {
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

func pageTokenFromResult(result *testresultsv2.TestResultRow) PageToken {
	if result.RequestOrdinal > 0 {
		// Page token used when retrieiving nominated verdicts.
		return PageToken{RequestOrdinal: result.RequestOrdinal}
	} else {
		return PageToken{ID: result.ID.VerdictID()}
	}
}

func pageTokenFromExoneration(exoneration *testexonerationsv2.TestExonerationRow) PageToken {
	if exoneration.RequestOrdinal > 0 {
		// Page token used when retrieiving nominated verdicts.
		return PageToken{RequestOrdinal: exoneration.RequestOrdinal}
	} else {
		return PageToken{ID: exoneration.ID.VerdictID()}
	}
}

func pageTokenFromVerdict(verdict *TestVerdict) PageToken {
	if verdict.RequestOrdinal > 0 {
		// Page token used when retrieiving nominated verdicts.
		return PageToken{RequestOrdinal: verdict.RequestOrdinal}
	} else {
		return PageToken{ID: verdict.ID}
	}
}

// toTestResultsPageToken converts a test verdict page token to
// a test result page token.
func (t PageToken) toTestResultsPageToken() testresultsv2.PageToken {
	if t == (PageToken{}) {
		return testresultsv2.PageToken{}
	}
	var result testresultsv2.PageToken
	if t.RequestOrdinal > 0 {
		result.RequestOrdinal = t.RequestOrdinal
	} else {
		result.ID.RootInvocationShardID = t.ID.RootInvocationShardID
		result.ID.ModuleName = t.ID.ModuleName
		result.ID.ModuleScheme = t.ID.ModuleScheme
		result.ID.ModuleVariantHash = t.ID.ModuleVariantHash
		result.ID.CoarseName = t.ID.CoarseName
		result.ID.FineName = t.ID.FineName
		result.ID.CaseName = t.ID.CaseName
	}
	// We want to start *after* the last verdict.
	// U+10FFFF is the maximum unicode character and will sort after
	// all valid WorkUnits. It is not valid for work unit IDs itself.
	result.ID.WorkUnitID = "\U0010FFFF"
	result.ID.ResultID = ""
	return result
}

// toTestExonerationsPageToken converts a test verdict page token to
// a test exonerations page token.
func (t PageToken) toTestExonerationsPageToken() testexonerationsv2.PageToken {
	if t == (PageToken{}) {
		return testexonerationsv2.PageToken{}
	}
	var result testexonerationsv2.PageToken
	if t.RequestOrdinal > 0 {
		result.RequestOrdinal = t.RequestOrdinal
	} else {
		result.ID.RootInvocationShardID = t.ID.RootInvocationShardID
		result.ID.ModuleName = t.ID.ModuleName
		result.ID.ModuleScheme = t.ID.ModuleScheme
		result.ID.ModuleVariantHash = t.ID.ModuleVariantHash
		result.ID.CoarseName = t.ID.CoarseName
		result.ID.FineName = t.ID.FineName
		result.ID.CaseName = t.ID.CaseName
	}
	// We want to start *after* the last verdict.
	// U+10FFFF is the maximum unicode character and will sort after
	// all valid WorkUnits. It is not valid for work unit IDs itself.
	result.ID.WorkUnitID = "\U0010FFFF"
	result.ID.ExonerationID = ""
	return result
}
