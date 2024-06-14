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

// Package stability implements the test stability analysis used by the
// QueryStability RPC.
package stability

import (
	"context"
	"sort"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server/span"

	spanutil "go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

const (
	// The maximum number of workers to run in parallel.
	// Given 100 is the maximum number of test variants queried at once,
	// it is desirable that maxWorkers * batchSize >= 100.
	maxWorkers = 10

	// The size of each batch (in test variants).
	batchSize = 10
)

// QueryStabilityOptions specifies options for QueryStability().
type QueryStabilityOptions struct {
	// Project is the LUCI Project to query.
	Project string
	// SubRealms are the project-scoped realms (of the form "ci",
	// NOT "chromium:ci") within the project to query.
	SubRealms []string
	// TestVariantPositions are the test variant positions to query.
	TestVariantPositions []*pb.QueryTestVariantStabilityRequest_TestVariantPosition
	// The test stability criteria to apply.
	Criteria *pb.TestStabilityCriteria
	// AsAtTime is latest parititon time to include in the results;
	// outside of testing contexts, this should be the current time.
	// QueryTestVariants returns data for the 14 day period leading
	// up to this time.
	AsAtTime time.Time
}

// run represents all executions of a test variant in a particular
// (lowest-level) ResultDB invocation.
type run struct {
	// Whether at least one non-skipped test result in the run was
	// expected.
	expected bool
}

// QueryStability queries the stability of nominated test variants.
// Used to inform exoneration decisions.
//
// Must be called in a Spanner transactional context. Context must
// support multiple reads (i.e. NOT spanner.Single()) as request may
// batched over multiple reads.
func QueryStability(ctx context.Context, opts QueryStabilityOptions) ([]*pb.TestVariantStabilityAnalysis, error) {
	batches := partitionQueryIntoBatches(opts.TestVariantPositions, batchSize)

	err := parallel.WorkPool(maxWorkers, func(c chan<- func() error) {
		for _, b := range batches {
			// Assign batch to a local variable to ensure its current
			// value is captured by function closures.
			batch := b
			c <- func() error {
				var err error
				batchOpts := opts
				batchOpts.TestVariantPositions = batch.input
				// queryStabilityShard ensures test variants appear
				// in the output in the same order as they appear in the
				// input.
				batch.output, err = queryStabilityShard(ctx, batchOpts)
				return err
			}
		}
	})
	if err != nil {
		return nil, err
	}

	// The order of test variants in the output should be the
	// same as the input. Perform the inverse to what we did
	// in batching.
	analysis := make([]*pb.TestVariantStabilityAnalysis, 0, len(opts.TestVariantPositions))
	for _, b := range batches {
		analysis = append(analysis, b.output...)
	}

	return analysis, nil
}

type queryStabilityBatch struct {
	input  []*pb.QueryTestVariantStabilityRequest_TestVariantPosition
	output []*pb.TestVariantStabilityAnalysis
}

// partitionQueryIntoBatches partitions a list of test variant positions
// into batches.
func partitionQueryIntoBatches(tvps []*pb.QueryTestVariantStabilityRequest_TestVariantPosition, batchSize int) []*queryStabilityBatch {
	var batches []*queryStabilityBatch
	batchInput := make([]*pb.QueryTestVariantStabilityRequest_TestVariantPosition, 0, batchSize)
	for _, tvp := range tvps {
		if len(batchInput) >= batchSize {
			batches = append(batches, &queryStabilityBatch{
				input: batchInput,
			})
			batchInput = make([]*pb.QueryTestVariantStabilityRequest_TestVariantPosition, 0, batchSize)
		}
		batchInput = append(batchInput, tvp)
	}
	if len(batchInput) > 0 {
		batches = append(batches, &queryStabilityBatch{
			input: batchInput,
		})
	}
	return batches
}

// queryStabilityShard reads test stability statistics for test variants.
// Must be called in a spanner transactional context.
func queryStabilityShard(ctx context.Context, opts QueryStabilityOptions) ([]*pb.TestVariantStabilityAnalysis, error) {
	type changelist struct {
		Host   string
		Change int64
	}
	type testVariantPosition struct {
		TestID              string
		VariantHash         string
		SourceRefHash       []byte
		QuerySourcePosition int64
		ExcludedChangelists []changelist
	}

	tvps := make([]testVariantPosition, 0, len(opts.TestVariantPositions))
	for _, ptv := range opts.TestVariantPositions {
		variantHash := ptv.VariantHash
		if variantHash == "" {
			variantHash = pbutil.VariantHash(ptv.Variant)
		}

		excludedCLs := make([]changelist, 0, len(ptv.Sources.Changelists))
		for _, cl := range ptv.Sources.Changelists {
			excludedCLs = append(excludedCLs, changelist{
				Host:   testresults.CompressHost(cl.Host),
				Change: cl.Change,
			})
		}

		tvps = append(tvps, testVariantPosition{
			TestID:              ptv.TestId,
			VariantHash:         variantHash,
			SourceRefHash:       pbutil.SourceRefHash(pbutil.SourceRefFromSources(ptv.Sources)),
			QuerySourcePosition: pbutil.SourcePosition(ptv.Sources),
			ExcludedChangelists: excludedCLs,
		})
	}

	stmt := spanner.NewStatement(testStabilityQuery)
	stmt.Params = map[string]any{
		"project":              opts.Project,
		"testVariantPositions": tvps,
		"subRealms":            opts.SubRealms,
		"asAtTime":             opts.AsAtTime,
		"skip":                 int64(pb.TestResultStatus_SKIP),
	}

	results := make([]*pb.TestVariantStabilityAnalysis, 0, len(tvps))

	index := 0
	var b spanutil.Buffer
	err := span.Query(ctx, stmt).Do(func(row *spanner.Row) error {
		var (
			testID, variantHash                               string
			verdictsBefore, verdictsOnOrAfter                 []*sourceVerdict
			sourcePositionBuckets                             []*sourcePositionBucket
			runFlakyVerdictsBefore, runFlakyVerdictsOnOrAfter []*sourceVerdict
		)

		err := b.FromSpanner(
			row,
			&testID,
			&variantHash,
			&verdictsBefore, &verdictsOnOrAfter,
			&sourcePositionBuckets,
			&runFlakyVerdictsBefore, &runFlakyVerdictsOnOrAfter,
		)
		if err != nil {
			return err
		}

		analysis := &pb.TestVariantStabilityAnalysis{}
		if testID != tvps[index].TestID || variantHash != tvps[index].VariantHash {
			// This should never happen, as the SQL statement is designed
			// to return results in the same order as test variants requested.
			panic("results in incorrect order")
		}
		sourcePosition := tvps[index].QuerySourcePosition

		analysis.TestId = testID
		analysis.Variant = opts.TestVariantPositions[index].Variant
		analysis.VariantHash = opts.TestVariantPositions[index].VariantHash
		analysis.FailureRate = applyFailureRateCriteria(verdictsBefore, verdictsOnOrAfter, sourcePosition, opts.Criteria.FailureRate)
		analysis.FlakeRate = applyFlakeRateCriteria(sourcePositionBuckets, sourcePosition, runFlakyVerdictsBefore, runFlakyVerdictsOnOrAfter, opts.Criteria.FlakeRate)
		results = append(results, analysis)
		index++
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// truncateSourceVerdicts truncates verdicts such that the
// total runs in the truncated slice is no greater than maxRuns.
//
// If the total number of runs in verdicts is already less
// than or equal to maxRuns, no truncation occurs.
//
// As individual source verdicts can represent more than one
// run, truncation may occur inside a source verdict (dropping
// some of its runs) to achieve the a total of maxRuns in
// the returned slice.
func truncateSourceVerdicts(verdicts []*sourceVerdict, maxRuns int) []*sourceVerdict {
	var runs int
	var result []*sourceVerdict
	for _, verdict := range verdicts {
		if runs >= maxRuns {
			break
		}

		remainingRuns := maxRuns - runs
		truncatedVerdict := truncateSourceVerdict(verdict, remainingRuns)
		runs += int(truncatedVerdict.ExpectedRuns + truncatedVerdict.UnexpectedRuns)
		result = append(result, truncatedVerdict)
	}
	return result
}

// truncateSourceVerdict truncates a verdict such that its
// total runs is no greater than maxRuns.
//
// If multiple runs are to be removed from a verdict, the dropped
// runs are balanced between the unexpected and expected runs
// proportionately.
func truncateSourceVerdict(verdict *sourceVerdict, maxRuns int) *sourceVerdict {
	// Copy the verdict as we may drop some of its runs.
	vc := *verdict

	excessRuns := (vc.ExpectedRuns + vc.UnexpectedRuns) - int64(maxRuns)
	if excessRuns > 0 {
		// Fairly share the runs to be removed between expected
		// and unexpected runs. Round towards removing more
		// expected runs than unexpected runs.
		unexpectedRunsToRemove := excessRuns * vc.UnexpectedRuns / (vc.ExpectedRuns + vc.UnexpectedRuns)
		expectedRunsToRemove := excessRuns - unexpectedRunsToRemove

		vc.UnexpectedRuns -= unexpectedRunsToRemove
		vc.ExpectedRuns -= expectedRunsToRemove
	}
	return &vc
}

// flattenSourceVerdictsToRuns transforms a list of source verdicts
// to a sequence of runs.
func flattenSourceVerdictsToRuns(verdicts []*sourceVerdict) []run {
	var result []run
	for _, verdict := range verdicts {
		result = append(result, flattenSourceVerdictToRuns(verdict)...)
	}
	return result
}

// flattenSourceVerdictToRuns transforms a source verdict
// into a sequence of runs. As the order of runs within a source verdict
// is not known, they are put in an arbitrary (fair) order.
func flattenSourceVerdictToRuns(verdict *sourceVerdict) []run {
	var result []run
	unexpectedRuns := verdict.UnexpectedRuns
	totalRuns := verdict.UnexpectedRuns + verdict.ExpectedRuns

	var unexpectedOutput int64
	var totalOutput int64
	for totalOutput < totalRuns {
		var expected bool

		remainingUnexpectedRuns := (unexpectedRuns - unexpectedOutput)
		remainingRuns := (totalRuns - totalOutput)

		// What percentage of remaining runs are unexpected?
		//
		// In case of only unexpected runs, this will be 100.
		// In case of only expected runs, this will be 0.
		// Otherwise, it should be somewhere in the middle.
		//
		// Invariant: 0 <= remainingUnexpectedPercent <= 100.
		remainingUnexpectedPercent := (remainingUnexpectedRuns * 100) / remainingRuns

		// If we output an expected run now, what percentage
		// of the runs output so far will be unexpected?
		//
		// Invariant: 0 <= unexpectedPercentIfOutputExpected < 100.
		unexpectedPercentIfOutputExpected := (unexpectedOutput * 100) / (totalOutput + 1)

		// Maintain fairness by alternating between expected
		// and unexpected runs to keep the proportion of
		// unexpected runs output so far and the proportion
		// of unexpected runs remaining about equal.
		//
		// In case of only expected runs remaining, remainingUnexpectedPercent
		// will equal zero so we will only output expected runs.
		//
		// In case of only unexpected runs remaining, remainingUnexpectedPercent
		// will equal 100. As unexpectedPercentIfOutputExpected is
		// always less than 100, we will only output unexpected runs.
		if unexpectedPercentIfOutputExpected >= remainingUnexpectedPercent {
			expected = true // output expected run.
		} else {
			expected = false // output unexpected run.
		}

		result = append(result, run{expected: expected})
		if !expected {
			unexpectedOutput++
		}
		totalOutput++
	}
	return result
}

func reverseVerdicts(verdicts []*sourceVerdict) []*sourceVerdict {
	reversed := make([]*sourceVerdict, 0, len(verdicts))
	for i := len(verdicts) - 1; i >= 0; i-- {
		reversed = append(reversed, verdicts[i])
	}
	return reversed
}

func reverseRuns(runs []run) []run {
	reversed := make([]run, 0, len(runs))
	for i := len(runs) - 1; i >= 0; i-- {
		reversed = append(reversed, runs[i])
	}
	return reversed
}

func splitOn(verdicts []*sourceVerdict, sourcePosition int64) (on, other []*sourceVerdict) {
	on = make([]*sourceVerdict, 0, len(verdicts))
	other = make([]*sourceVerdict, 0, len(verdicts))

	for _, v := range verdicts {
		if v.SourcePosition == sourcePosition {
			on = append(on, v)
		} else {
			other = append(other, v)
		}
	}
	return on, other
}

// filterSourceVerdicts filters source verdicts so that at most one
// test run is present for each verdict obtained in presubmit.
func filterSourceVerdictsForFailureRateCriteria(svs []*sourceVerdict) []*sourceVerdict {
	result := make([]*sourceVerdict, 0, len(svs))
	for _, sv := range svs {
		item := &sourceVerdict{}
		*item = *sv

		if item.ChangelistChange.Valid && (item.UnexpectedRuns+item.ExpectedRuns) > 1 {
			// For presubmit data, keep only one run, preferentially the unexpected run.
			if item.UnexpectedRuns >= 1 {
				item.UnexpectedRuns = 1
				item.ExpectedRuns = 0
			} else {
				item.UnexpectedRuns = 0
				item.ExpectedRuns = 1
			}
		}

		result = append(result, item)
	}
	return result
}

// applyFailureRateCriteria applies the failure rate criteria to a test variant
// at a given source position.
//
// beforeExamples is a list of (up to) 10 source verdicts with source position
// just prior to the queried source position. The list shall be ordered with
// the source verdict nearest the queried source position appearing first.
//
// onOrAfterExamples is a list of (up to) 10 source verdicts with source position
// equal to, or just after, the queried source position. The list shall be
// ordered with the source verdict nearest the queried source position
// appearing first.
//
// Both sets of examples should have had the following filtering applied:
//   - At most one source verdict per distinct CL (for source verdicts
//     testing CLs; no such restrictions apply to postsubmit data).
//   - Source verdicts must not be for the same CL as is being considered for
//     exoneration (if any).
//   - Source verdicts must not be for CLs authored by automation.
//
// criteria defines the failure rate thresholds to apply.
func applyFailureRateCriteria(beforeExamples, onOrAfterExamples []*sourceVerdict, sourcePosition int64, criteria *pb.TestStabilityCriteria_FailureRateCriteria) *pb.TestVariantStabilityAnalysis_FailureRate {
	// Limit source verdicts from presubmit to contributing at most 1 run each.
	// This is to avoid a single repeatedly retried bad CL from having an oversized
	// influence on the exoneration decision.
	beforeExamples = filterSourceVerdictsForFailureRateCriteria(beforeExamples)
	onOrAfterExamples = filterSourceVerdictsForFailureRateCriteria(onOrAfterExamples)

	onExamples, afterExamples := splitOn(onOrAfterExamples, sourcePosition)

	onExamples = truncateSourceVerdicts(onExamples, 10)
	onRuns := flattenSourceVerdictsToRuns(onExamples)

	// The window size is 10, and the window will always contain any runs on the queried
	// source position. Additional runs may come from source positions before or after.
	// Note: For the passed examples, the first example is the one nearest to the query
	// position, so truncating keeps only the examples closest to the queried source position.
	beforeExamples = truncateSourceVerdicts(beforeExamples, 10-len(onRuns))
	afterExamples = truncateSourceVerdicts(afterExamples, 10-len(onRuns))

	beforeRuns := flattenSourceVerdictsToRuns(beforeExamples)
	afterRuns := flattenSourceVerdictsToRuns(afterExamples)

	consecutive := consecutiveUnexpectedCount(reverseRuns(afterRuns), onRuns, beforeRuns)

	// Put runs in chronological order:
	// 0                 ....          len(runs)-1
	// <--- more recent  <query position>  less recent --->
	//
	// We need to reverse afterRuns as it is sorted with the first element
	// closest to the query position.
	runs := append(append(reverseRuns(afterRuns), onRuns...), beforeRuns...)
	maxFailuresInWindow := unexpectedRunsInWindow(runs, 10)

	// Also put source verdicts in chronological order.
	// 0                 ....          len(examples)-1
	// <--- more recent  <query position>  less recent --->
	examples := append(append(reverseVerdicts(afterExamples), onExamples...), beforeExamples...)

	return &pb.TestVariantStabilityAnalysis_FailureRate{
		IsMet: (consecutive >= int(criteria.ConsecutiveFailureThreshold) ||
			maxFailuresInWindow >= int(criteria.FailureThreshold)),
		UnexpectedTestRuns:            int32(maxFailuresInWindow),
		ConsecutiveUnexpectedTestRuns: int32(consecutive),
		RecentVerdicts:                toPBFailureRateRecentVerdict(examples),
	}
}

// unexpectedRunsInWindow considers all sliding windows of size
// windowSize over the slice runs, and returns the maximum number
// of unexpected test runs in any such window.
func unexpectedRunsInWindow(runs []run, windowSize int) int {
	// If the number of runs is less than the window size, consider
	// the runs that remain as a single window.
	if len(runs) < windowSize {
		windowSize = len(runs)
	}

	unexpectedCount := 0
	for i := 0; i < windowSize; i++ {
		if !runs[i].expected {
			unexpectedCount++
		}
	}
	// Now failureCount = COUNT_UNEXPECTED(runs[0:windowSize])

	maxFailuresInWindow := unexpectedCount
	for i := 1; i+windowSize-1 < len(runs); i++ {
		// Slide the window one position.
		if !runs[i-1].expected {
			unexpectedCount--
		}
		if !runs[i+windowSize-1].expected {
			unexpectedCount++
		}
		// Now failureCount = COUNT_UNEXPECTED(runs[i:windowSize+i])

		if unexpectedCount > maxFailuresInWindow {
			maxFailuresInWindow = unexpectedCount
		}
	}
	return maxFailuresInWindow
}

// consecutiveUnexpectedCount returns the number of consecutive unexpected runs
// present from the start or end of a series of runs, where those consecutive
// unexpected runs also include the query position.
//
// If the consecutive failures do not pass the query position, this method
// returns 0.
// If there are consecutive failures but none touch the start or end
// of the runs slice, this method also returns 0.
//
// Example:
//
//	[U U U] [U U] [U E U E E U U E] = afterRuns, onRuns, beforeRuns
//
// The method returns 6, because of there is a chain of 6 consecutive
// failures starting at the front of the runs slice, and that chain
// passes by the query position. It is also continues to one run
// in the 'beforeRuns' slice.
//
// The following conventions apply to arguments:
//   - the most recent runs (later source position) appear first
//     in all runs slices.
//   - onRuns represents runs exactly on the queried source position,
//   - afterRuns represents runs with a source position greater than
//     the queried source position
//   - beforeRuns represents runs with a source position less than
//     the queried source position
func consecutiveUnexpectedCount(afterRuns, onRuns, beforeRuns []run) int {
	for _, r := range onRuns {
		if r.expected {
			// There is an expected run on the queried source position.
			// The failures cannot be consecutive up to and including
			// the source position from either side.
			return 0
		}
	}

	// The number of consecutive runs in afterRuns, starting from
	// the side of the queried source position.
	afterRunsConsecutive := len(afterRuns)
	for i := len(afterRuns) - 1; i >= 0; i-- {
		if afterRuns[i].expected {
			// We encountered an expected run.
			afterRunsConsecutive = (len(afterRuns) - 1) - i
			break
		}
	}

	// The number of consecutive runs in beforeRuns, starting from
	// the side of the queried source position.
	beforeRunsConsecutive := len(beforeRuns)
	for i := 0; i < len(beforeRuns); i++ {
		if beforeRuns[i].expected {
			// We encountered an expected run.
			beforeRunsConsecutive = i
			break
		}
	}

	if len(afterRuns) == afterRunsConsecutive {
		// All runs after the source position are unexpected.
		// Additionally, we know all runs on the source position are unexpected.
		return len(afterRuns) + len(onRuns) + beforeRunsConsecutive
	}
	if len(beforeRuns) == beforeRunsConsecutive {
		// All runs before the source position are unexpected.
		// Additionally, we know all runs on the source position are unexpected.
		return len(beforeRuns) + len(onRuns) + afterRunsConsecutive
	}
	return 0
}

// applyFlakeRateCriteria applies the flake rate criteria to a test variant
// at a given source position.
//
// buckets should be in ascending order by source position.
func applyFlakeRateCriteria(buckets []*sourcePositionBucket, querySourcePosition int64, beforeExamples, onOrAfterExamples []*sourceVerdict, criteria *pb.TestStabilityCriteria_FlakeRateCriteria) *pb.TestVariantStabilityAnalysis_FlakeRate {
	ba := newBucketAnalyzer(buckets)
	partitionTime := ba.earliestPartitionTimeAtSourcePosition(querySourcePosition)

	// Query the soure position +/- 1 week.
	weekWindow := ba.bucketsForTimeRange(partitionTime.Add(-7*24*time.Hour), partitionTime.Add(7*24*time.Hour))
	runFlaky, total := countVerdicts(weekWindow)
	if total < int64(criteria.MinWindow) {
		// If the sample size is not large enough, revert to querying the full
		// 14 days of data. This exists to improve performance on infrequently
		// run tests at the cost of some recency.
		weekWindow = buckets
		runFlaky, total = countVerdicts(weekWindow)
	}

	// Query the source position +/- 1 weekday.
	dayWindow := ba.bucketsForTimeRange(jumpBack24WeekdayHours(partitionTime), jumpAhead24WeekdayHours(partitionTime))
	runFlaky1wd, _ := countVerdicts(dayWindow)

	// Examples arrive sorted such that those closest to the queried source
	// position are first.
	// Flip and combine them so that the most recent (latest source position)
	// are first.
	allExamples := append(reverseVerdicts(onOrAfterExamples), beforeExamples...)

	// Find examples from the window considered.
	var examples []*sourceVerdict
	var startPosition int64
	var endPosition int64
	if len(weekWindow) > 0 {
		startPosition = weekWindow[0].StartSourcePosition
		endPosition = weekWindow[len(weekWindow)-1].EndSourcePosition

		for _, e := range allExamples {
			if startPosition <= e.SourcePosition && e.SourcePosition <= endPosition {
				examples = append(examples, e)
			}
		}

		if len(examples) > 10 {
			examples = examples[:10]
		}
	}

	var startPosition1wd int64
	var endPosition1wd int64
	if len(dayWindow) > 0 {
		startPosition1wd = dayWindow[0].StartSourcePosition
		endPosition1wd = dayWindow[len(dayWindow)-1].EndSourcePosition
	}

	flakeRate := 0.0
	if total > 0 {
		flakeRate = float64(runFlaky) / float64(total)
	}

	return &pb.TestVariantStabilityAnalysis_FlakeRate{
		IsMet: runFlaky >= int64(criteria.FlakeThreshold) &&
			flakeRate >= criteria.FlakeRateThreshold &&
			runFlaky1wd >= int64(criteria.FlakeThreshold_1Wd),
		RunFlakyVerdicts:     int32(runFlaky),
		TotalVerdicts:        int32(total),
		FlakeExamples:        toPBFlakeRateVerdictExample(examples),
		StartPosition:        startPosition,
		EndPosition:          endPosition,
		RunFlakyVerdicts_1Wd: int32(runFlaky1wd),
		StartPosition_1Wd:    startPosition1wd,
		EndPosition_1Wd:      endPosition1wd,
	}
}

// sourcePositionBucket represents a range of source positions for
// a given test variant.
type sourcePositionBucket struct {
	BucketKey int64
	// Starting source position. Inclusive.
	StartSourcePosition int64
	// Ending source position. Inclusive.
	EndSourcePosition int64
	// The earliest partition time of a test result in the bucket.
	EarliestPartitionTime time.Time
	// The total number of source verdicts in the bucket.
	TotalVerdicts int64
	// The total number of run-flaky source verdicts in the bucket.
	RunFlakyVerdicts int64
}

// bucketAnalyzer conducts analysis about the number of flaky source verdicts
// and total source verdicts observed within a time window around a source
// position.
//
// It relies upon creating a conversion between source position and time
// to identify the time corresponding to a source position and the
// the source position range that corresponds to a time range.
type bucketAnalyzer struct {
	// buckets has statistics about source verdicts in ranges.
	// buckets must be in ascending order by source position,
	// i.e. oldest/smallest source position first.
	buckets []*sourcePositionBucket

	// earliestSourcePositionAvailability[i] represents the earliest partition time
	// observed for a test result with a source position at or after buckets[i].StartSourcePosition.
	//
	// Intuitively, it represents a best guess estimate about the time a source position
	// was first available in the repository. It is also consistent in the sense that
	// an earlier source position will never have a later time associated with it than
	// a later source position.
	earliestSourcePositionAvailability []time.Time
}

// newBucketAnalyzer initialises a new bucket analyzer.
// buckets must be in ascending order by source position,
// i.e. oldest/smallest source position first.
func newBucketAnalyzer(buckets []*sourcePositionBucket) bucketAnalyzer {
	earliestSourcePositionAvailability := make([]time.Time, len(buckets))
	earliestTime := time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)

	// Start at the more recent (larger) source position and work backwards
	// to the past.
	for i := len(buckets) - 1; i >= 0; i-- {
		b := buckets[i]
		if b.EndSourcePosition < b.StartSourcePosition {
			panic("end source position should be equal to or after start source position")
		}
		if i < len(buckets)-2 && !(buckets[i].EndSourcePosition < buckets[i+1].StartSourcePosition) {
			panic("end source position of bucket should be before start source position of next bucket")
		}
		// Regardless of the earliest time a source position in this bucket was observed,
		// if a bucket with a later source position had an earlier time, we should use that.
		// This is because the later source positions build upon earlier sources positions,
		// so the earlier source positions must have been available at that time too.
		if b.EarliestPartitionTime.Before(earliestTime) {
			earliestTime = b.EarliestPartitionTime
		}
		earliestSourcePositionAvailability[i] = earliestTime
	}

	return bucketAnalyzer{
		buckets:                            buckets,
		earliestSourcePositionAvailability: earliestSourcePositionAvailability,
	}
}

// earliestPartitionTimeAtSourcePosition converts a source position
// to a time, corresponding to the earliest partition time for a
// test result at or after that source position.
func (b *bucketAnalyzer) earliestPartitionTimeAtSourcePosition(querySourcePosition int64) time.Time {
	if len(b.buckets) == 0 {
		return time.Time{}
	}

	// Find the nearest bucket that includes, or is prior to, the queried source position.
	queryIndex := 0
	for i, b := range b.buckets {
		if b.StartSourcePosition > querySourcePosition {
			break
		}
		queryIndex = i
	}

	// The time approximately corresponding to the queried source position.
	return b.earliestSourcePositionAvailability[queryIndex]
}

// bucketsForTimeRange converts a partition time range to a slice
// of source position buckets. The time <--> source position conversion
// used is consistent with earliestPartitionTimeAtSourcePosition.
func (b *bucketAnalyzer) bucketsForTimeRange(start, end time.Time) []*sourcePositionBucket {
	if len(b.buckets) == 0 {
		return nil
	}

	startIndex := len(b.buckets)
	// Iterate from the oldest bucket to the newest.
	for i, time := range b.earliestSourcePositionAvailability {
		if !time.Before(start) { // time >= start
			startIndex = i
			break
		}
	}

	endIndex := 0
	// Iterate from the newest bucket to the oldest.
	for i := len(b.earliestSourcePositionAvailability) - 1; i >= 0; i-- {
		time := b.earliestSourcePositionAvailability[i]
		if !time.After(end) { // time <= end
			endIndex = i
			break
		}
	}

	return b.buckets[startIndex : endIndex+1]
}

func countVerdicts(buckets []*sourcePositionBucket) (runFlaky, total int64) {
	for _, b := range buckets {
		runFlaky += b.RunFlakyVerdicts
		total += b.TotalVerdicts
	}
	return runFlaky, total
}

// sourceVerdict is used to store an example source verdict returned by
// a Spanner query.
type sourceVerdict struct {
	SourcePosition int64
	// Verdicts considered by the analysis have at most one CL tested,
	// which is set below (if present).
	ChangelistHost      spanner.NullString
	ChangelistChange    spanner.NullInt64
	ChangelistPatchset  spanner.NullInt64
	ChangelistOwnerKind spanner.NullString
	RootInvocationIds   []string
	UnexpectedRuns      int64
	ExpectedRuns        int64
}

func toPBFailureRateRecentVerdict(verdicts []*sourceVerdict) []*pb.TestVariantStabilityAnalysis_FailureRate_RecentVerdict {
	results := make([]*pb.TestVariantStabilityAnalysis_FailureRate_RecentVerdict, 0, len(verdicts))
	for _, v := range verdicts {
		var changelists []*pb.Changelist
		if v.ChangelistHost.Valid {
			changelists = append(changelists, &pb.Changelist{
				Host:      testresults.DecompressHost(v.ChangelistHost.StringVal),
				Change:    v.ChangelistChange.Int64,
				Patchset:  int32(v.ChangelistPatchset.Int64),
				OwnerKind: testresults.OwnerKindFromDB(v.ChangelistOwnerKind.StringVal),
			})
		}

		results = append(results, &pb.TestVariantStabilityAnalysis_FailureRate_RecentVerdict{
			Position:       v.SourcePosition,
			Changelists:    changelists,
			Invocations:    sortStrings(v.RootInvocationIds),
			UnexpectedRuns: int32(v.UnexpectedRuns),
			TotalRuns:      int32(v.ExpectedRuns + v.UnexpectedRuns),
		})
	}
	return results
}

func sortStrings(ids []string) []string {
	idsCopy := make([]string, len(ids))
	copy(idsCopy, ids)
	sort.Strings(idsCopy)
	return idsCopy
}

func toPBFlakeRateVerdictExample(verdicts []*sourceVerdict) []*pb.TestVariantStabilityAnalysis_FlakeRate_VerdictExample {
	results := make([]*pb.TestVariantStabilityAnalysis_FlakeRate_VerdictExample, 0, len(verdicts))
	for _, v := range verdicts {
		var changelists []*pb.Changelist
		if v.ChangelistHost.Valid {
			changelists = append(changelists, &pb.Changelist{
				Host:      testresults.DecompressHost(v.ChangelistHost.StringVal),
				Change:    v.ChangelistChange.Int64,
				Patchset:  int32(v.ChangelistPatchset.Int64),
				OwnerKind: testresults.OwnerKindFromDB(v.ChangelistOwnerKind.StringVal),
			})
		}

		results = append(results, &pb.TestVariantStabilityAnalysis_FlakeRate_VerdictExample{
			Position:    v.SourcePosition,
			Changelists: changelists,
			Invocations: sortStrings(v.RootInvocationIds),
		})
	}
	return results
}

// jumpBack24WeekdayHours calculates the start time of an interval
// ending at the given endTime, such that the interval includes exactly
// 24 hours of weekday data (in UTC). Where there are multiple interval
// start times satisfying this criteria, the latest time is selected.
//
// For example, if the endTime is 08:21 on Tuesday, we would pick 08:21
// on Monday as the start time, as the period from start time to end time
// includes 24 hours of weekday (split over Monday and Tuesday).
//
// If the endTime is 08:21 on Monday, we would pick 08:21 on the previous
// Friday as the start time, as the period from start time to end time
// includes 24 hours of weekday (split over the Friday and Monday).
//
// If the endTime is midnight on the morning of Tuesday, any start time from
// midnight on Saturday morning to midnight on Monday morning would produce
// an interval that includes 24 hours of weekday data (i.e. the 24 hours of
// Monday). In this case, we pick midnight on Monday. This is the only case
// that is ambiguous.
//
// Rationale:
// Many projects see reduced testing activity on weekends, as fewer CLs are
// submitted. To avoid a dip in the sample size of statistics returned
// on these days (which stops exoneration), we effectively bunch weekend
// data together with Friday data in one period.
func jumpBack24WeekdayHours(endTime time.Time) time.Time {
	endTime = endTime.In(time.UTC)
	var startTime time.Time
	switch endTime.Weekday() {
	case time.Saturday:
		// Take us back to Saturday at 0:00.
		startTime = endTime.Truncate(24 * time.Hour)
		// Now take us back to Friday at 0:00.
		startTime = startTime.Add(-24 * time.Hour)
	case time.Sunday:
		// Take us back to Sunday at 0:00.
		startTime = endTime.Truncate(24 * time.Hour)
		// Now take us back to Friday at 0:00.
		startTime = startTime.Add(-2 * 24 * time.Hour)
	case time.Monday:
		// Take take us back to the same time
		// on the Friday.
		startTime = endTime.Add(-3 * 24 * time.Hour)
	default:
		// Take take us back to the same time
		// on the previous day (which will be a weekday).
		startTime = endTime.Add(-24 * time.Hour)
	}
	return startTime
}

// jumpAhead24WeekdayHours calculates the end time of an interval
// ending at the given startTime, such that the interval includes exactly
// 24 hours of weekday data (in UTC). Where there are multiple interval
// end times satisfying this criteria, the later time is selected.
//
// For example, if the startTime is 08:21 on Monday, we would pick 08:21
// on Tuesday as the end time, as the period from start time to end time
// includes 24 hours of weekday (split over Monday and Tuesday).
//
// If the startTime is 08:21 on Friday, we would pick 08:21 on the next
// Monday as the end time, as the period from start time to end time
// includes 24 hours of weekday (split over the Friday and Monday).
//
// If the endTime is midnight on the morning of Friday, any end time from
// midnight on Saturday morning to midnight on Monday morning would produce
// an interval that includes 24 hours of weekday data (i.e. the 24 hours of
// Monday). In this case, we pick midnight on Monday. This is the only case
// that is ambiguous.
func jumpAhead24WeekdayHours(startTime time.Time) time.Time {
	startTime = startTime.In(time.UTC)
	var endTime time.Time
	switch startTime.Weekday() {
	case time.Friday:
		// Take us forward to Monday at the same time.
		endTime = startTime.Add(3 * 24 * time.Hour)
	case time.Saturday:
		// Take us back to Saturday at 0:00.
		endTime = startTime.Truncate(24 * time.Hour)
		// Now take us forward to Tuesday at 0:00.
		endTime = endTime.Add(3 * 24 * time.Hour)
	case time.Sunday:
		// Take us back to Sunday at 0:00.
		endTime = startTime.Truncate(24 * time.Hour)
		// Now take us forward to Tuesday at 0:00.
		endTime = endTime.Add(2 * 24 * time.Hour)
	default:
		// Take us forward to the same time
		// on the next day (which will be a weekday).
		endTime = startTime.Add(24 * time.Hour)
	}
	return endTime
}

var testStabilityQuery = `
WITH test_variants_with_source_verdicts AS (
	SELECT
		Index,
		tv.TestId,
		tv.VariantHash,
		tv.QuerySourcePosition,
		ARRAY(
			-- Filter source verdicts to at most one per changelist under test.
			-- Don't filter source verdicts without any unsubmitted changelists
			-- under test (i.e. postsubmit data).
			SELECT
				ANY_VALUE(
					STRUCT(
						SourcePosition,
						ChangelistHost,
						ChangelistChange,
						ChangelistPatchset,
						ChangelistOwnerKind,
						RootInvocationIds,
						MaxPartitionTime,
						MinPartitionTime,
						UnexpectedRuns,
						ExpectedRuns
					)
					-- For any CL, prefer the verdict that is flaky.
					-- Then prefer the verdict that is closest to the queried
					-- source position.
					HAVING MIN ABS(SourcePosition - tv.QuerySourcePosition) + IF(UnexpectedRuns > 0 AND ExpectedRuns > 0, -1000 * 1000 * 1000, 0)
				) AS SourceVerdict,
			FROM (
				-- Flatten test runs to source verdicts.
				SELECT
					SourcePosition,
					ChangelistHost,
					ChangelistChange,
					ChangelistPatchset,
          ANY_VALUE(ChangelistOwnerKind) AS ChangelistOwnerKind,
					ANY_VALUE(HasDirtySources) AS HasDirtySources,
					ANY_VALUE(IF(HasDirtySources, RootInvocationId, NULL)) AS DirtySourcesUniqifier,
					ARRAY_AGG(DISTINCT RootInvocationId) as RootInvocationIds,
					MAX(PartitionTime) as MaxPartitionTime,
					MIN(PartitionTime) as MinPartitionTime,
					COUNTIF(IsUnexpectedRun) as UnexpectedRuns,
					COUNTIF(NOT IsUnexpectedRun) as ExpectedRuns,
				FROM (
					-- Flatten test results to test runs.
					SELECT
						RootInvocationId,
						InvocationId,
						ANY_VALUE(PartitionTime) as PartitionTime,
						LOGICAL_AND(COALESCE(IsUnexpected, FALSE)) AS IsUnexpectedRun,
						ANY_VALUE(SourcePosition) AS SourcePosition,
						ANY_VALUE(ChangelistHosts)[SAFE_OFFSET(0)] AS ChangelistHost,
						ANY_VALUE(ChangelistChanges)[SAFE_OFFSET(0)] AS ChangelistChange,
						ANY_VALUE(ChangelistPatchsets)[SAFE_OFFSET(0)] AS ChangelistPatchset,
						ANY_VALUE(ChangelistOwnerKinds)[SAFE_OFFSET(0)] AS ChangelistOwnerKind,
						ANY_VALUE(HasDirtySources) AS HasDirtySources
					FROM TestResultsBySourcePosition
					WHERE Project = @project
						AND TestId = tv.TestId
						AND VariantHash = tv.VariantHash
						AND SourceRefHash = tv.SourceRefHash
						AND SubRealm IN UNNEST(@subRealms)
						-- Limit to the last 14 days of data.
						AND PartitionTime >= TIMESTAMP_SUB(@asAtTime, INTERVAL 14 DAY)
						AND PartitionTime < @asAtTime
						-- Exclude skipped results.
						AND Status <> @skip
						AND (
							(
								-- Either there must be no CL tested by this result.
								ChangelistHosts IS NULL OR ARRAY_LENGTH(ChangelistHosts) = 0
							)
							OR (
								-- Or there must be exactly one CL tested.
								ARRAY_LENGTH(ChangelistHosts) = 1

								-- And that CL may not be authored by automation.
								-- Automatic uprev automation will happily upload out CL after CL
								-- with essentially the same change, that breaks the same test.
								-- This adds more noise than signal.
								AND ChangelistOwnerKinds[SAFE_OFFSET(0)] <> 'A'

								-- And that CL must not be one of changelists which we
								-- are considering exonerating as a result of this RPC.
								AND STRUCT(ChangelistHosts[SAFE_OFFSET(0)] as Host, ChangelistChanges[SAFE_OFFSET(0)] AS Change)
									NOT IN UNNEST(tv.ExcludedChangelists)
							)
						)
					GROUP BY RootInvocationId, InvocationId
				)
				GROUP BY
					-- Base source position tested
					SourcePosition,
					-- Patchset applied ontop of base sources (if any)
					ChangelistHost, ChangelistChange, ChangelistPatchset,
					-- If sources are marked dirty, then sources must be treated as unique
					-- per root invocation. (I.E. then source verdict == test verdict).
					IF(HasDirtySources, RootInvocationId, NULL)
			)
			-- Deduplicate to at most one per CL. For source verdicts not related
			-- to a CL, no deduplication shall occur.
			GROUP BY
				-- Changelist applied ontop of base sources (if any), excluding patchset number.
				ChangelistHost, ChangelistChange,
				-- If there is no CL under test, then the original source verdicts
				-- may be kept.
				IF(ChangelistHost IS NULL, SourcePosition, NULL),
				IF(ChangelistHost IS NULL, DirtySourcesUniqifier, NULL)
			ORDER BY SourceVerdict.SourcePosition DESC, SourceVerdict.MaxPartitionTime DESC
		) AS SourceVerdicts,
	FROM UNNEST(@testVariantPositions) tv WITH OFFSET Index
)

SELECT
	TestId,
	VariantHash,
	ARRAY(
		SELECT AS STRUCT
			SourcePosition,
			ChangelistHost,
			ChangelistChange,
			ChangelistPatchset,
			ChangelistOwnerKind,
			RootInvocationIds,
			UnexpectedRuns,
			ExpectedRuns
		FROM UNNEST(SourceVerdicts) v
		WHERE v.SourcePosition < QuerySourcePosition
		ORDER BY SourcePosition DESC, MaxPartitionTime DESC
		-- The actual criteria is for 10 runs, not 10 source verdicts,
		-- but this is hard to implement in SQL so we'll do post-filtering
		-- in the app.
		LIMIT 10
	) as FailureRateVerdictsBefore,
	ARRAY(
		SELECT AS STRUCT
			SourcePosition,
			ChangelistHost,
			ChangelistChange,
			ChangelistPatchset,
			ChangelistOwnerKind,
			RootInvocationIds,
			UnexpectedRuns,
			ExpectedRuns
		FROM UNNEST(SourceVerdicts) v
		WHERE v.SourcePosition >= QuerySourcePosition
		ORDER BY SourcePosition ASC, MaxPartitionTime DESC
		-- The actual criteria is for 10 runs, not 10 source verdicts,
		-- but this is hard to implement in SQL so we'll do post-filtering
		-- in the app.
		LIMIT 10
	) as FailureRateVerdictsOnOrAfter,
	ARRAY(
		-- The design calls for us to:
		-- 1. convert the query position to a partition time,
		-- 2. calculate a window +/- 7 days from that time,
		-- 3. convert that time window back to source position range
		-- 4. query that source position range and count the number of
		---   (and proportion of) flaky verdicts in the range.
		--
		-- As Spanner does not have analytic functions, steps 1 and 3 are
		-- hard to do in SQL.
		--
		-- Returning all verdicts to the backend to run the analysis there
		-- is also not viable: each test variant may have up to ~10,000
		-- source verdicts per two week period. At 100 bytes per verdict,
		-- this would imply a transfer of around 1 MB per test variant
		-- (or 100 MB in total for 100 test variants) to the backend.
		-- This is too much.
		--
		-- Therefore, we use an approximate implementation.
		-- We partition the source verdicts into 100 source position ranges,
		-- maintaining the earliest partition time for each. This allows
		-- steps 1-3 to be computed by AppEngine after the query returns.
		--
		-- Each bucket also maintains counts of flaky verdicts and total
		-- source verdicts. Because of this, there is no need to perform
		-- a follow-up query; once we determine the source position window
		-- to query, we simply count the verdicts in the buckets we
		-- determined to be part of that window.
		SELECT AS STRUCT
			CAST(FLOOR(Index * 100 / ARRAY_LENGTH(SourcePositions)) AS INT64) as BucketKey,
			MIN(SourcePosition) as StartSourcePosition,
			MAX(SourcePosition) as EndSourcePosition,
			MIN(EarliestPartitionTime) as EarliestPartitionTime,
			SUM(TotalVerdicts) as TotalVerdicts,
			SUM(RunFlakyVerdicts) as RunFlakyVerdicts,
		FROM (
			SELECT
				ARRAY(
					-- Group source verdicts by source position first,
					-- so that buckets contain all of a source position
					-- or none of it.
					SELECT AS STRUCT
						SourcePosition,
						MIN(MinPartitionTime) as EarliestPartitionTime,
						COUNT(1) as TotalVerdicts,
						COUNTIF(UnexpectedRuns > 0 AND ExpectedRuns > 0) as RunFlakyVerdicts,
					FROM UNNEST(SourceVerdicts) v
					GROUP BY SourcePosition
					ORDER BY SourcePosition
				) AS SourcePositions
		), UNNEST(SourcePositions) sp WITH OFFSET Index
		GROUP BY 1
		ORDER BY BucketKey
	) AS FlakeRateBuckets,
	-- We do not yet know exactly the range of source positions
	-- that we will end up using for the flake rate criteria.
	-- Get (up to) 10 examples of flake each side of the
	-- query position, so that regardless of where the
	-- window falls, we will be able to get 10 examples.
	ARRAY(
		SELECT AS STRUCT
			SourcePosition,
			ChangelistHost,
			ChangelistChange,
			ChangelistPatchset,
			ChangelistOwnerKind,
			RootInvocationIds,
			UnexpectedRuns,
			ExpectedRuns
		FROM UNNEST(SourceVerdicts) v WITH OFFSET Index
		WHERE UnexpectedRuns > 0 AND ExpectedRuns > 0
		  AND v.SourcePosition < QuerySourcePosition
		ORDER BY SourcePosition DESC, MaxPartitionTime DESC
		LIMIT 10
	) as FlakeExamplesBefore,
	ARRAY(
		SELECT AS STRUCT
			SourcePosition,
			ChangelistHost,
			ChangelistChange,
			ChangelistPatchset,
			ChangelistOwnerKind,
			RootInvocationIds,
			UnexpectedRuns,
			ExpectedRuns
		FROM UNNEST(SourceVerdicts) v WITH OFFSET Index
		WHERE UnexpectedRuns > 0 AND ExpectedRuns > 0
		  AND v.SourcePosition >= QuerySourcePosition
		ORDER BY SourcePosition ASC, MaxPartitionTime DESC
		LIMIT 10
	) as FlakeExamplesOnOrAfter,
FROM test_variants_with_source_verdicts
ORDER BY Index
`
