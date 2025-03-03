// Copyright 2022 The LUCI Authors.
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

package testresults

import (
	"context"
	"text/template"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server/span"

	spanutil "go.chromium.org/luci/analysis/internal/span"
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

// QueryFailureRateOptions specifies options for QueryFailureRate().
type QueryFailureRateOptions struct {
	// Project is the LUCI Project to query.
	Project string
	// SubRealms are the realms (of the form "ci", NOT "chromium:ci")
	// within the project to query.
	SubRealms []string
	// TestVariants are the test variants to query.
	TestVariants []*pb.QueryTestVariantFailureRateRequest_TestVariant
	// AsAtTime is latest parititon time to include in the results;
	// outside of testing contexts, this should be the current time.
	// QueryTestVariants returns data for the 5 * 24 weekday hour
	// period leading up to this time.
	AsAtTime time.Time
}

// QueryFailureRate queries the failure rate of nominated test variants.
//
// Must be called in a Spanner transactional context. Context must
// support multiple reads (i.e. NOT spanner.Single()) as request may
// batched over multiple reads.
func QueryFailureRate(ctx context.Context, opts QueryFailureRateOptions) (*pb.QueryTestVariantFailureRateResponse, error) {
	batches := partitionIntoBatches(opts.TestVariants)

	intervals := defineIntervals(opts.AsAtTime)

	err := parallel.WorkPool(maxWorkers, func(c chan<- func() error) {
		for _, b := range batches {
			// Assign batch to a local variable to ensure its current
			// value is captured by function closures.
			batch := b
			c <- func() error {
				var err error
				batchOpts := opts
				batchOpts.TestVariants = batch.input
				// queryFailureRateShard ensures test variants appear
				// in the output in the same order as they appear in the
				// input.
				batch.output, err = queryFailureRateShard(ctx, batchOpts, intervals)
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
	analysis := make([]*pb.TestVariantFailureRateAnalysis, 0, len(opts.TestVariants))
	for _, b := range batches {
		analysis = append(analysis, b.output...)
	}

	response := &pb.QueryTestVariantFailureRateResponse{
		Intervals:    toPBIntervals(intervals),
		TestVariants: analysis,
	}
	return response, nil
}

type batch struct {
	input  []*pb.QueryTestVariantFailureRateRequest_TestVariant
	output []*pb.TestVariantFailureRateAnalysis
}

// partitionIntoBatches partitions a list of test variants into batches.
func partitionIntoBatches(tvs []*pb.QueryTestVariantFailureRateRequest_TestVariant) []*batch {
	var batches []*batch
	batchInput := make([]*pb.QueryTestVariantFailureRateRequest_TestVariant, 0, batchSize)
	for _, tv := range tvs {
		if len(batchInput) >= batchSize {
			batches = append(batches, &batch{
				input: batchInput,
			})
			batchInput = make([]*pb.QueryTestVariantFailureRateRequest_TestVariant, 0, batchSize)
		}
		batchInput = append(batchInput, tv)
	}
	if len(batchInput) > 0 {
		batches = append(batches, &batch{
			input: batchInput,
		})
	}
	return batches
}

// queryFailureRateShard reads failure rate statistics for test variants.
// Must be called in a spanner transactional context.
func queryFailureRateShard(ctx context.Context, opts QueryFailureRateOptions, intervals []interval) ([]*pb.TestVariantFailureRateAnalysis, error) {
	type testVariant struct {
		TestID      string
		VariantHash string
	}

	tvs := make([]testVariant, 0, len(opts.TestVariants))
	for _, ptv := range opts.TestVariants {
		variantHash := ptv.VariantHash
		if variantHash == "" {
			variantHash = pbutil.VariantHash(ptv.Variant)
		}

		tvs = append(tvs, testVariant{
			TestID:      ptv.TestId,
			VariantHash: variantHash,
		})
	}

	stmt, err := spanutil.GenerateStatement(failureRateQueryTmpl, failureRateQueryTmpl.Name(), nil)
	if err != nil {
		return nil, err
	}
	stmt.Params = map[string]any{
		"project":             opts.Project,
		"testVariants":        tvs,
		"subRealms":           opts.SubRealms,
		"afterPartitionTime":  queryStartTime(intervals),
		"beforePartitionTime": queryEndTime(intervals),
		"skip":                int64(pb.TestResultStatus_SKIP),
	}

	results := make([]*pb.TestVariantFailureRateAnalysis, 0, len(tvs))

	index := 0
	var b spanutil.Buffer
	err = span.Query(ctx, stmt).Do(func(row *spanner.Row) error {
		var testID, variantHash string
		var intervalStats []*intervalStats
		var runFlakyExamples []*verdictExample
		var recentVerdicts []*recentVerdict

		err := b.FromSpanner(
			row,
			&testID,
			&variantHash,
			&intervalStats,
			&runFlakyExamples,
			&recentVerdicts,
		)
		if err != nil {
			return err
		}

		analysis := &pb.TestVariantFailureRateAnalysis{}
		if testID != tvs[index].TestID || variantHash != tvs[index].VariantHash {
			// This should never happen, as the SQL statement is designed
			// to return results in the same order as test variants requested.
			panic("results in incorrect order")
		}

		analysis.TestId = testID
		analysis.Variant = opts.TestVariants[index].Variant
		analysis.VariantHash = opts.TestVariants[index].VariantHash
		analysis.IntervalStats = toPBIntervalStats(intervalStats, intervals)
		analysis.RunFlakyVerdictExamples = toPBVerdictExamples(runFlakyExamples)
		analysis.RecentVerdicts = toPBRecentVerdicts(recentVerdicts)
		results = append(results, analysis)
		index++
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
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
		startTime = startTime.Add(-1 * 24 * time.Hour)
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

// interval represents a time interval of data to be returned by
// QueryFailureRate.
type interval struct {
	// The interval start time (inclusive).
	startTime time.Time
	// The interval end time (exclusive).
	endTime time.Time
}

// defineIntervals defines the time intervals that should be returned
// by the QueryFailureRate query. This comprises five consecutive
// 24 weekday hour intervals ending at the given asAtTime.
//
// The first interval is the most recent interval, which ends at asAtTime
// and starts at such a time that means the interval includes 24
// hours of weekday data (as measured in UTC).
// The second interval ends at the start time of the first interval,
// and is generated in a similar fashion, and so on for the other
// three intervals.
// See jumpBack24WeekdayHours for how "24 weekday hours" is defined.
func defineIntervals(asAtTime time.Time) []interval {
	const intervalCount = 5
	result := make([]interval, 0, intervalCount)
	endTime := asAtTime.In(time.UTC)
	for i := 0; i < intervalCount; i++ {
		startTime := jumpBack24WeekdayHours(endTime)
		result = append(result, interval{
			startTime: startTime,
			endTime:   endTime,
		})
		endTime = startTime
	}
	return result
}

// queryStartTime returns the start of the partition time range that
// should be queried (inclusive). Verdicts with a partition time
// earlier than this time should not be included in the results.
func queryStartTime(intervals []interval) time.Time {
	return intervals[len(intervals)-1].startTime
}

// queryEndTime returns the end of the partition time range that should be
// queried (exclusive). Verdicts with partition times later than (or equal to)
// this time should not be included in the results.
func queryEndTime(intervals []interval) time.Time {
	return intervals[0].endTime
}

func toPBIntervals(intervals []interval) []*pb.QueryTestVariantFailureRateResponse_Interval {
	result := make([]*pb.QueryTestVariantFailureRateResponse_Interval, 0, len(intervals))
	for i, iv := range intervals {
		result = append(result, &pb.QueryTestVariantFailureRateResponse_Interval{
			IntervalAge: int32(i + 1),
			StartTime:   timestamppb.New(iv.startTime),
			EndTime:     timestamppb.New(iv.endTime),
		})
	}
	return result
}

// intervalStats represents time interval data returned by the
// QueryFailureRate query. Each interval represents 24 hours of data.
type intervalStats struct {
	// DaysSinceQueryStart is the time interval bucket represented by this
	// row.
	// Interval 0 represents the first 24 hours of the partition time range
	// queried, interval 1 is the second 24 hours, and so on.
	DaysSinceQueryStart int64
	// TotalRunExpectedVerdicts is the number of verdicts which had only
	// expected runs. An expected run is a run in which at least one test
	// result was expected (excluding skips).
	TotalRunExpectedVerdicts int64
	// TotalRunFlakyVerdicts is the number of verdicts which had both expected
	// and unexpected runs. An expected run is a run in which at least one
	// test result was expected (excluding skips). An unexpected run is a run
	// in which all test results are unexpected (excluding skips).
	TotalRunFlakyVerdicts int64
	// TotalRunExpectedVerdicts is the number of verdicts which had only
	// unexpected runs. An unexpected run is a run in which
	// al test results are unexpected (excluding skips).
	TotalRunUnexpectedVerdicts int64
}

func toPBIntervalStats(stats []*intervalStats, intervals []interval) []*pb.TestVariantFailureRateAnalysis_IntervalStats {
	queryStartTime := queryStartTime(intervals)
	queryEndTime := queryEndTime(intervals)

	results := make([]*pb.TestVariantFailureRateAnalysis_IntervalStats, 0, len(intervals))
	// Ensure every interval is included in the output, even if there are
	// no verdicts in it.
	for i := range intervals {
		results = append(results, &pb.TestVariantFailureRateAnalysis_IntervalStats{
			IntervalAge: int32(i + 1),
		})
	}
	for _, s := range stats {
		// Match the interval data returned by the SQL query to the intervals
		// returned by the RPC. The SQL query returns one interval per
		// rolling 24 hour period, whereas the RPC returns one interval per
		// rolling 24 weekday hour period (including any weekend included
		// in that range). In practice, this means weekend data needs to get
		// summed into the 24 weekday hour period that includes the Friday.

		// Calculate the time interval represented by the interval data
		// returned by the query.
		intervalStartTime := queryStartTime.Add(time.Duration(s.DaysSinceQueryStart) * 24 * time.Hour)
		intervalEndTime := intervalStartTime.Add(24 * time.Hour)
		if queryEndTime.Before(intervalEndTime) {
			intervalEndTime = queryEndTime
		}

		// Find the output interval that contains the query interval.
		intervalIndex := -1
		for i, iv := range intervals {
			if !intervalEndTime.After(iv.endTime) && !intervalStartTime.Before(iv.startTime) {
				intervalIndex = i
				break
			}
		}
		if intervalIndex == -1 {
			// This should never happen.
			panic("could not reconcile query intervals with output intervals")
		}

		results[intervalIndex].TotalRunExpectedVerdicts += int32(s.TotalRunExpectedVerdicts)
		results[intervalIndex].TotalRunFlakyVerdicts += int32(s.TotalRunFlakyVerdicts)
		results[intervalIndex].TotalRunUnexpectedVerdicts += int32(s.TotalRunUnexpectedVerdicts)
	}
	return results
}

// verdictExample is used to store an example verdict returned by
// a Spanner query.
type verdictExample struct {
	PartitionTime        time.Time
	IngestedInvocationId string
	ChangelistHosts      []string
	ChangelistChanges    []int64
	ChangelistPatchsets  []int64
	ChangelistOwnerKinds []string
}

func toPBVerdictExamples(ves []*verdictExample) []*pb.TestVariantFailureRateAnalysis_VerdictExample {
	results := make([]*pb.TestVariantFailureRateAnalysis_VerdictExample, 0, len(ves))
	for _, ve := range ves {
		cls := make([]*pb.Changelist, 0, len(ve.ChangelistHosts))
		// TODO(b/258734241): Expect ChangelistOwnerKinds will
		// have matching length in all cases from March 2023.
		if len(ve.ChangelistHosts) != len(ve.ChangelistChanges) ||
			len(ve.ChangelistChanges) != len(ve.ChangelistPatchsets) ||
			(ve.ChangelistOwnerKinds != nil && len(ve.ChangelistOwnerKinds) != len(ve.ChangelistHosts)) {
			panic("data consistency issue: length of changelist arrays do not match")
		}
		for i := range ve.ChangelistHosts {
			var ownerKind pb.ChangelistOwnerKind
			if ve.ChangelistOwnerKinds != nil {
				ownerKind = OwnerKindFromDB(ve.ChangelistOwnerKinds[i])
			}
			cls = append(cls, &pb.Changelist{
				Host:      DecompressHost(ve.ChangelistHosts[i]),
				Change:    ve.ChangelistChanges[i],
				Patchset:  int32(ve.ChangelistPatchsets[i]),
				OwnerKind: ownerKind,
			})
		}
		results = append(results, &pb.TestVariantFailureRateAnalysis_VerdictExample{
			PartitionTime:        timestamppb.New(ve.PartitionTime),
			IngestedInvocationId: ve.IngestedInvocationId,
			Changelists:          cls,
		})
	}
	return results
}

// recentVerdict represents one of the most recent verdicts for the test variant.
type recentVerdict struct {
	verdictExample
	HasUnexpectedRun bool
}

func toPBRecentVerdicts(verdicts []*recentVerdict) []*pb.TestVariantFailureRateAnalysis_RecentVerdict {
	results := make([]*pb.TestVariantFailureRateAnalysis_RecentVerdict, 0, len(verdicts))
	for _, v := range verdicts {
		cls := make([]*pb.Changelist, 0, len(v.ChangelistHosts))
		// TODO(b/258734241): Expect ChangelistOwnerKinds will
		// have matching length in all cases from March 2023.
		if len(v.ChangelistHosts) != len(v.ChangelistChanges) ||
			len(v.ChangelistChanges) != len(v.ChangelistPatchsets) ||
			(v.ChangelistOwnerKinds != nil && len(v.ChangelistOwnerKinds) != len(v.ChangelistHosts)) {
			panic("data consistency issue: length of changelist arrays do not match")
		}
		for i := range v.ChangelistHosts {
			var ownerKind pb.ChangelistOwnerKind
			if v.ChangelistOwnerKinds != nil {
				ownerKind = OwnerKindFromDB(v.ChangelistOwnerKinds[i])
			}
			cls = append(cls, &pb.Changelist{
				Host:      DecompressHost(v.ChangelistHosts[i]),
				Change:    v.ChangelistChanges[i],
				Patchset:  int32(v.ChangelistPatchsets[i]),
				OwnerKind: ownerKind,
			})
		}
		results = append(results, &pb.TestVariantFailureRateAnalysis_RecentVerdict{
			PartitionTime:        timestamppb.New(v.PartitionTime),
			IngestedInvocationId: v.IngestedInvocationId,
			Changelists:          cls,
			HasUnexpectedRuns:    v.HasUnexpectedRun,
		})
	}
	return results
}

var failureRateQueryTmpl = template.Must(template.New("").Parse(`
WITH test_variant_verdicts AS (
	SELECT
		Index,
		TestId,
		VariantHash,
		ARRAY(
			-- Filter verdicts to at most one per unsubmitted changelist under
			-- test. Don't filter verdicts without an unsubmitted changelist
			-- under test (i.e. CI data).
			SELECT
				ANY_VALUE(STRUCT(
				PartitionTime,
				IngestedInvocationId,
				HasUnexpectedRun,
				HasExpectedRun,
				ChangelistHosts,
				ChangelistChanges,
				ChangelistPatchsets,
				ChangelistOwnerKinds,
				AnyChangelistsByAutomation)
				-- Prefer the verdict that is flaky. If both (or neither) are flaky,
				-- pick the verdict with the highest partition time. If partition
				-- times are also the same, pick any.
				HAVING MAX IF(HasExpectedRun AND HasUnexpectedRun, TIMESTAMP_ADD(PartitionTime, INTERVAL 365 DAY), PartitionTime)) AS Verdict,
			FROM (
				-- Flatten test runs to test verdicts.
				SELECT
					PartitionTime,
					IngestedInvocationId,
					LOGICAL_OR(UnexpectedRun) AS HasUnexpectedRun,
					LOGICAL_OR(NOT UnexpectedRun) AS HasExpectedRun,
					ANY_VALUE(ChangelistHosts) AS ChangelistHosts,
					ANY_VALUE(ChangelistChanges) AS ChangelistChanges,
					ANY_VALUE(ChangelistPatchsets) AS ChangelistPatchsets,
					ANY_VALUE(ChangelistOwnerKinds) AS ChangelistOwnerKinds,
					ANY_VALUE(AnyChangelistsByAutomation) As AnyChangelistsByAutomation
				FROM (
					-- Flatten test results to test runs.
					SELECT
						PartitionTime,
						IngestedInvocationId,
						RunIndex,
						LOGICAL_AND(COALESCE(IsUnexpected, FALSE)) AS UnexpectedRun,
						ANY_VALUE(ChangelistHosts) AS ChangelistHosts,
						ANY_VALUE(ChangelistChanges) AS ChangelistChanges,
						ANY_VALUE(ChangelistPatchsets) AS ChangelistPatchsets,
						ANY_VALUE(ChangelistOwnerKinds) AS ChangelistOwnerKinds,
						'A' IN UNNEST(ANY_VALUE(ChangelistOwnerKinds)) AS AnyChangelistsByAutomation
					FROM TestResults
					WHERE Project = @project
						AND PartitionTime >= @afterPartitionTime
						AND PartitionTime < @beforePartitionTime
						AND TestId = tv.TestId And VariantHash = tv.VariantHash
						AND SubRealm IN UNNEST(@subRealms)
						-- Exclude skipped results.
						AND Status <> @skip
						-- Exclude test results testing multiple CLs, as
						-- we cannot ensure at most one verdict per CL for
						-- them.
						AND (ChangelistHosts IS NULL OR ARRAY_LENGTH(ChangelistHosts) <= 1)
					GROUP BY PartitionTime, IngestedInvocationId, RunIndex
				)
				GROUP BY PartitionTime, IngestedInvocationId
				ORDER BY PartitionTime DESC, IngestedInvocationId
			)
			-- Unique CL (if there is a CL under test).
			GROUP BY
				IF(ChangelistHosts IS NOT NULL AND ARRAY_LENGTH(ChangelistHosts) > 0, ChangelistHosts[OFFSET(0)], IngestedInvocationId),
				IF(ChangelistHosts IS NOT NULL AND ARRAY_LENGTH(ChangelistHosts) > 0, ChangelistChanges[OFFSET(0)], NULL)
			ORDER BY Verdict.PartitionTime DESC, Verdict.IngestedInvocationId
		) AS Verdicts,
	FROM UNNEST(@testVariants) tv WITH OFFSET Index
)

SELECT
	TestId,
	VariantHash,
	ARRAY(
		SELECT AS STRUCT
			TIMESTAMP_DIFF(v.PartitionTime, @afterPartitionTime, DAY) as DaysSinceQueryStart,
			COUNTIF(NOT v.HasUnexpectedRun AND v.HasExpectedRun) AS TotalRunExpectedVerdicts,
			COUNTIF(v.HasUnexpectedRun AND v.HasExpectedRun) AS TotalRunFlakyVerdicts,
			COUNTIF(v.HasUnexpectedRun AND NOT v.HasExpectedRun) AS TotalRunUnexpectedVerdicts
		FROM UNNEST(Verdicts) v
		WHERE
			-- Filter out CLs authored by automation.
			NOT v.AnyChangelistsByAutomation
		GROUP BY DaysSinceQueryStart
		ORDER BY DaysSinceQueryStart DESC
	) As IntervalStats,
	ARRAY(
		SELECT AS STRUCT
			v.PartitionTime,
			v.IngestedInvocationId,
			v.ChangelistHosts,
			v.ChangelistChanges,
			v.ChangelistPatchsets,
			v.ChangelistOwnerKinds,
		FROM UNNEST(Verdicts) v WITH OFFSET o
		WHERE v.HasUnexpectedRun AND v.HasExpectedRun AND
			-- Filter out CLs authored by automation.
			NOT v.AnyChangelistsByAutomation
		ORDER BY o -- Order by descending partition time.
		LIMIT 10
	) as RunFlakyExamples,
	ARRAY(
		SELECT AS STRUCT
			v.PartitionTime,
			v.IngestedInvocationId,
			v.ChangelistHosts,
			v.ChangelistChanges,
			v.ChangelistPatchsets,
			v.ChangelistOwnerKinds,
			v.HasUnexpectedRun
		FROM UNNEST(Verdicts) v WITH OFFSET o
		WHERE
			-- Filter out CLs authored by automation.
			NOT v.AnyChangelistsByAutomation
		ORDER BY o
		LIMIT 10
	) as RecentVerdicts,
FROM test_variant_verdicts
ORDER BY Index
`))
