// Copyright 2023 The LUCI Authors.
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

// Package testvariantbranch handles test variant branch of change point analysis.
package testvariantbranch

import (
	"sort"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	cpb "go.chromium.org/luci/analysis/internal/changepoints/proto"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

const (
	// Each test variant branch retains at most 100 finalized segments.
	maxFinalizedSegmentsToRetain = 100

	// We only retain finalized segments for the last 5 years.
	// For simplicity, we consider a year has 365 days.
	// For testibility, we calculate the 5 years from the last ingestion time
	// of the test variant branch (this means we may over-retain some segments).
	maxHoursToRetain = 5 * 365 * 24

	// StatisticsRetentionDays is the number of days to keep statistics about
	// evicted verdicts. See Statistics proto for more.
	//
	// This is a minimum period driven by functional and operational requirements,
	// our deletion logic will tend to keep retain data for longer (but this is
	// OK as it is not user data).
	StatisticsRetentionDays = 11
)

// Entry represents one row in the TestVariantBranch spanner table.
// See go/luci-test-variant-analysis-design for details.
type Entry struct {
	// IsNew is a boolean to denote if the TestVariantBranch is new or already
	// existed in Spanner.
	// It is used for reducing the number of mutations. For example, the Variant
	// field is only inserted once.
	IsNew       bool
	Project     string
	TestID      string
	VariantHash string
	Variant     *pb.Variant
	RefHash     []byte
	SourceRef   *pb.SourceRef
	InputBuffer *inputbuffer.Buffer
	// If this is true, it means we should trigger a write of FinalizingSegment
	// to Spanner.
	IsFinalizingSegmentDirty bool
	// The finalizing segment, if any.
	// The count for the finalizing segment should only include the verdicts
	// that are not in the input buffer anymore.
	FinalizingSegment *cpb.Segment
	// If this is true, it means we should trigger a write of FinalizedSegments
	// to Spanner.
	IsFinalizedSegmentsDirty bool
	// The finalized segments for the test variant branch.
	FinalizedSegments *cpb.Segments
	// If true, it means we should trigger a write of Statistics to Spanner.
	IsStatisticsDirty bool
	// Statistics about verdicts which have been evicted from the input buffer.
	Statistics *cpb.Statistics
}

// New creates a new empty test variant branch entry, with a preallocated input buffer.
func New() *Entry {
	tvb := &Entry{}
	tvb.InputBuffer = inputbuffer.New()
	return tvb
}

// Clear resets a test variant branch entry to an empty state, similar to
// after a call to New().
func (tvb *Entry) Clear() {
	tvb.IsNew = false
	tvb.Project = ""
	tvb.TestID = ""
	tvb.VariantHash = ""
	tvb.Variant = nil
	tvb.RefHash = nil
	tvb.SourceRef = nil
	tvb.InputBuffer.Clear()
	tvb.IsFinalizingSegmentDirty = false
	tvb.FinalizingSegment = nil
	tvb.IsFinalizedSegmentsDirty = false
	tvb.FinalizedSegments = nil
	tvb.IsStatisticsDirty = false
	tvb.Statistics = nil
}

// Copy makes a deep copy of a test variant branch entry.
func (tvb *Entry) Copy() *Entry {
	if tvb == nil {
		return nil
	}
	refHashCopy := make([]byte, len(tvb.RefHash))
	copy(refHashCopy, tvb.RefHash)

	return &Entry{
		IsNew:                    tvb.IsNew,
		Project:                  tvb.Project,
		TestID:                   tvb.TestID,
		VariantHash:              tvb.VariantHash,
		Variant:                  proto.Clone(tvb.Variant).(*pb.Variant),
		RefHash:                  refHashCopy,
		SourceRef:                proto.Clone(tvb.SourceRef).(*pb.SourceRef),
		InputBuffer:              tvb.InputBuffer.Copy(),
		IsFinalizingSegmentDirty: tvb.IsFinalizingSegmentDirty,
		FinalizingSegment:        proto.Clone(tvb.FinalizingSegment).(*cpb.Segment),
		IsFinalizedSegmentsDirty: tvb.IsFinalizedSegmentsDirty,
		FinalizedSegments:        proto.Clone(tvb.FinalizedSegments).(*cpb.Segments),
		IsStatisticsDirty:        tvb.IsStatisticsDirty,
		Statistics:               proto.Clone(tvb.Statistics).(*cpb.Statistics),
	}
}

// InsertToInputBuffer inserts data of a new test variant into the input
// buffer.
func (tvb *Entry) InsertToInputBuffer(pv inputbuffer.PositionVerdict) {
	tvb.InputBuffer.InsertVerdict(pv)
}

// InsertFinalizedSegment inserts a segment to the end of finalized segments.
func (tvb *Entry) InsertFinalizedSegment(segment *cpb.Segment) {
	if tvb.FinalizedSegments == nil {
		tvb.FinalizedSegments = &cpb.Segments{}
	}
	// Assert that segment is finalized.
	if segment.State != cpb.SegmentState_FINALIZED {
		panic("insert non-finalized segment to FinalizedSegments")
	}
	// Assert that inserted segment is later than existing segments.
	l := len(tvb.FinalizedSegments.Segments)
	if l > 0 && tvb.FinalizedSegments.Segments[l-1].EndPosition >= segment.StartPosition {
		panic("insert older segment to FinalizedSegments")
	}
	tvb.FinalizedSegments.Segments = append(tvb.FinalizedSegments.Segments, segment)
	tvb.IsFinalizedSegmentsDirty = true
}

// UpdateOutputBuffer updates the output buffer with the evicted segments from
// the input buffer.
// evictedSegments should contain only finalized segments, except for the
// last segment (if any), which must be a finalizing segment.
// evictedSegments is sorted in ascending order of commit position (oldest
// segment first).
func (tvb *Entry) UpdateOutputBuffer(evictedSegments []inputbuffer.EvictedSegment) {
	// Nothing to update.
	if len(evictedSegments) == 0 {
		return
	}
	verifyEvictedSegments(evictedSegments)
	// If there is a finalizing segment in the output buffer, this finalizing
	// segment should be "combined" with the first evicted segment.
	segmentIndex := 0
	if tvb.FinalizingSegment != nil {
		segmentIndex = 1
		combinedSegment := combineSegment(tvb.FinalizingSegment, evictedSegments[0].Segment)
		tvb.IsFinalizingSegmentDirty = true
		if combinedSegment.State == cpb.SegmentState_FINALIZING {
			// Replace the finalizing segment.
			tvb.FinalizingSegment = combinedSegment
		} else { // Finalized state.
			tvb.FinalizingSegment = nil
			tvb.InsertFinalizedSegment(combinedSegment)
		}
	}

	for ; segmentIndex < len(evictedSegments); segmentIndex++ {
		segment := evictedSegments[segmentIndex]
		if segment.Segment.State == cpb.SegmentState_FINALIZED {
			tvb.InsertFinalizedSegment(segment.Segment)
		} else { // Finalizing segment.
			tvb.FinalizingSegment = segment.Segment
			tvb.IsFinalizingSegmentDirty = true
		}
	}

	var evictedVerdicts []inputbuffer.PositionVerdict
	for _, segments := range evictedSegments {
		evictedVerdicts = append(evictedVerdicts, segments.Verdicts...)
	}
	tvb.Statistics = applyStatisticsRetention(insertVerdictsIntoStatistics(tvb.Statistics, evictedVerdicts))
	tvb.IsStatisticsDirty = true

	// Assert that finalizing segment is after finalized segments.
	tvb.verifyOutputBuffer()
}

func verifyEvictedSegments(evictedSegments []inputbuffer.EvictedSegment) {
	// Verify that evictedSegments contain all FINALIZED segment, except for
	// the last segment.
	for i, seg := range evictedSegments {
		if i != len(evictedSegments)-1 {
			if seg.Segment.State != cpb.SegmentState_FINALIZED {
				panic("evictedSegments should contains all finalized segments, except the last one")
			}
		} else {
			if seg.Segment.State != cpb.SegmentState_FINALIZING {
				panic("last segment of evicted segments should be finalizing")
			}
		}
	}
}

// verifyOutputBuffer verifies that the finalizing segment is older than any
// finalized segment.
// Panic if it is not the case.
func (tvb *Entry) verifyOutputBuffer() {
	finalizedSegments := tvb.FinalizedSegments.GetSegments()
	l := len(finalizedSegments)
	if tvb.FinalizingSegment == nil || l == 0 {
		return
	}
	if finalizedSegments[l-1].EndPosition >= tvb.FinalizingSegment.StartPosition {
		panic("finalizing segment should be older than finalized segments")
	}
}

// ApplyRetentionPolicyForFinalizedSegments applies retention policy
// to finalized segments.
// The following retention policy applies to finalized segments:
//   - At most 100 finalized segments can be stored.
//   - Finalized segments are retained for 5 years from when they closed.
//
// fromTime is the time when the 5 year period is calculated from.
//
// The retention policy to delete test variant branches without
// test results in 90 days will be enforced separately with a cron job.
func (tvb *Entry) ApplyRetentionPolicyForFinalizedSegments(fromTime time.Time) {
	finalizedSegments := tvb.FinalizedSegments.GetSegments()
	if len(finalizedSegments) == 0 {
		return
	}

	// We keep the finalized segments from this index.
	// Note that finalized segments are ordered by commit position (lowest first)
	// so theory (although it's rare), a later segment may have
	// smaller end hour than an earlier segment. Therefore, we may over-retain
	// some segments.
	startIndexToKeep := 0
	if len(finalizedSegments) > maxFinalizedSegmentsToRetain {
		startIndexToKeep = len(finalizedSegments) - maxFinalizedSegmentsToRetain
	}
	for i := startIndexToKeep; i < len(finalizedSegments); i++ {
		segment := finalizedSegments[i]
		if segment.EndHour.AsTime().Add(time.Hour * maxHoursToRetain).After(fromTime) {
			startIndexToKeep = i
			break
		}
	}

	if startIndexToKeep > 0 {
		tvb.IsFinalizedSegmentsDirty = true
		tvb.FinalizedSegments.Segments = finalizedSegments[startIndexToKeep:]
	}
}

// combineSegment combines the finalizing segment from the output buffer with
// another partial segment evicted from the input buffer.
func combineSegment(finalizingSegment, evictedSegment *cpb.Segment) *cpb.Segment {
	result := &cpb.Segment{
		State: evictedSegment.State,
		// Use the start position information provided by prior evictions.
		HasStartChangepoint:          finalizingSegment.HasStartChangepoint,
		StartPosition:                finalizingSegment.StartPosition,
		StartHour:                    finalizingSegment.StartHour,
		StartPositionLowerBound_99Th: finalizingSegment.StartPositionLowerBound_99Th,
		StartPositionUpperBound_99Th: finalizingSegment.StartPositionUpperBound_99Th,
		// Use end position information provided by later evictions.
		EndPosition: evictedSegment.EndPosition,
		EndHour:     evictedSegment.EndHour,
		// Combine counts.
		FinalizedCounts: AddCounts(finalizingSegment.FinalizedCounts, evictedSegment.FinalizedCounts),
	}
	result.MostRecentUnexpectedResultHour = finalizingSegment.MostRecentUnexpectedResultHour
	if result.MostRecentUnexpectedResultHour.GetSeconds() < evictedSegment.MostRecentUnexpectedResultHour.GetSeconds() {
		result.MostRecentUnexpectedResultHour = evictedSegment.MostRecentUnexpectedResultHour
	}
	return result
}

// AddCounts returns the sum of 2 statistics counts.
func AddCounts(count1 *cpb.Counts, count2 *cpb.Counts) *cpb.Counts {
	return &cpb.Counts{
		TotalResults:             count1.TotalResults + count2.TotalResults,
		UnexpectedResults:        count1.UnexpectedResults + count2.UnexpectedResults,
		ExpectedPassedResults:    count1.ExpectedPassedResults + count2.ExpectedPassedResults,
		ExpectedFailedResults:    count1.ExpectedFailedResults + count2.ExpectedFailedResults,
		ExpectedCrashedResults:   count1.ExpectedCrashedResults + count2.ExpectedCrashedResults,
		ExpectedAbortedResults:   count1.ExpectedAbortedResults + count2.ExpectedAbortedResults,
		UnexpectedPassedResults:  count1.UnexpectedPassedResults + count2.UnexpectedPassedResults,
		UnexpectedFailedResults:  count1.UnexpectedFailedResults + count2.UnexpectedFailedResults,
		UnexpectedCrashedResults: count1.UnexpectedCrashedResults + count2.UnexpectedCrashedResults,
		UnexpectedAbortedResults: count1.UnexpectedAbortedResults + count2.UnexpectedAbortedResults,
		TotalRuns:                count1.TotalRuns + count2.TotalRuns,
		UnexpectedUnretriedRuns:  count1.UnexpectedUnretriedRuns + count2.UnexpectedUnretriedRuns,
		UnexpectedAfterRetryRuns: count1.UnexpectedAfterRetryRuns + count2.UnexpectedAfterRetryRuns,
		FlakyRuns:                count1.FlakyRuns + count2.FlakyRuns,
		TotalVerdicts:            count1.TotalVerdicts + count2.TotalVerdicts,
		UnexpectedVerdicts:       count1.UnexpectedVerdicts + count2.UnexpectedVerdicts,
		FlakyVerdicts:            count1.FlakyVerdicts + count2.FlakyVerdicts,
	}
}

// insertVerdictsIntoStatistics updates the given statistics to include
// the given evicted verdicts. Retention policies are applied.
func insertVerdictsIntoStatistics(stats *cpb.Statistics, verdicts []inputbuffer.PositionVerdict) *cpb.Statistics {
	bucketByHour := make(map[int64]*cpb.Statistics_HourBucket)
	for _, bucket := range stats.GetHourlyBuckets() {
		// Copy hourly bucket to avoid mutating the passed statistics object.
		bucketByHour[bucket.Hour] = &cpb.Statistics_HourBucket{
			Hour:               bucket.Hour,
			UnexpectedVerdicts: bucket.UnexpectedVerdicts,
			FlakyVerdicts:      bucket.FlakyVerdicts,
			TotalVerdicts:      bucket.TotalVerdicts,
		}
	}

	for _, v := range verdicts {
		// Find or create hourly bucket.
		hour := v.Hour.Unix() / 3600
		bucket, ok := bucketByHour[hour]
		if !ok {
			bucket = &cpb.Statistics_HourBucket{Hour: hour}
			bucketByHour[hour] = bucket
		}

		// Add verdict to hourly bucket.
		bucket.TotalVerdicts++
		if !v.IsSimpleExpectedPass {
			verdictHasExpectedResults := false
			verdictHasUnexpectedResults := false
			for _, run := range v.Details.Runs {
				verdictHasExpectedResults = verdictHasExpectedResults || (run.Expected.Count() > 0)
				verdictHasUnexpectedResults = verdictHasUnexpectedResults || (run.Unexpected.Count() > 0)
			}
			if verdictHasUnexpectedResults && !verdictHasExpectedResults {
				bucket.UnexpectedVerdicts++
			}
			if verdictHasUnexpectedResults && verdictHasExpectedResults {
				bucket.FlakyVerdicts++
			}
		}
	}

	buckets := make([]*cpb.Statistics_HourBucket, 0, len(bucketByHour))
	for _, bucket := range bucketByHour {
		buckets = append(buckets, bucket)
	}

	// Sort in ascending order (oldest hour first).
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Hour < buckets[j].Hour
	})

	return &cpb.Statistics{
		HourlyBuckets: buckets,
	}
}

// applyStatisticsRetention applies the retention policies
// to statistics data.
func applyStatisticsRetention(stats *cpb.Statistics) *cpb.Statistics {
	buckets := stats.HourlyBuckets

	// Apply data deletion policies.
	if len(buckets) > 0 {
		lastHour := buckets[len(buckets)-1].Hour
		deleteBeforeIndex := -1
		for i, bucket := range buckets {
			// Retain buckets which are within the retention interval
			// of the most recent bucket hour. The most recent bucket
			// hour will always be less recent than time.Now(), so
			// this will tend to retain somewhat more data than necessary.
			//
			// We use this logic instead of one that depends on time.Now()
			// as it is simpler from a testability perspective than a
			// system time-dependant function.
			if bucket.Hour > lastHour-StatisticsRetentionDays*24 {
				break
			}
			deleteBeforeIndex = i
		}
		buckets = buckets[deleteBeforeIndex+1:]
	}
	return &cpb.Statistics{HourlyBuckets: buckets}
}

// MergedStatistics returns statistics about the verdicts ingested for
// given test variant branch. Statistics comprise data from both the
// input buffer and the output buffer.
func (tvb *Entry) MergedStatistics() *cpb.Statistics {
	verdicts := make([]inputbuffer.PositionVerdict, 0, inputbuffer.DefaultColdBufferCapacity+inputbuffer.DefaultHotBufferCapacity)
	verdicts = append(verdicts, tvb.InputBuffer.ColdBuffer.Verdicts...)
	verdicts = append(verdicts, tvb.InputBuffer.HotBuffer.Verdicts...)
	return insertVerdictsIntoStatistics(tvb.Statistics, verdicts)
}
