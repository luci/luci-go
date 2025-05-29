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
	"math"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"

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
	// evicted source verdicts. See Statistics proto for more.
	//
	// This is a minimum period driven by 7 days of lookback required for
	// functional reasons, plus four days to allow for late data ingestion.
	// Our deletion logic will tend to keep retain data for longer (but this is
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

// InsertToInputBuffer attempts to insert data of a new test run into the input
// buffer. This method returns true if the test run could be inserted,
// false if it is too far out of order.
func (tvb *Entry) InsertToInputBuffer(r inputbuffer.Run) bool {
	if tvb.isTooFarOutOfOrder(r) {
		return false
	}
	tvb.InputBuffer.InsertRun(r)
	return true
}

func (tvb *Entry) isTooFarOutOfOrder(r inputbuffer.Run) bool {
	if len(tvb.FinalizedSegments.GetSegments()) == 0 && tvb.FinalizingSegment == nil {
		// There are no finalized or finalizing segments yet.
		// Everything is still in the input buffer, so we can
		// re-order things arbitrarily.
		return false
	}
	hotRuns := tvb.InputBuffer.HotBuffer.Runs
	coldRuns := tvb.InputBuffer.ColdBuffer.Runs
	minPos := int64(math.MaxInt64)
	if len(hotRuns) > 0 && minPos > hotRuns[0].CommitPosition {
		minPos = hotRuns[0].CommitPosition
	}
	if len(coldRuns) > 0 && minPos > coldRuns[0].CommitPosition {
		minPos = coldRuns[0].CommitPosition
	}
	// Do not accept runs which are earlier than the starting
	// commit position still in the input buffer.
	return r.CommitPosition < minPos
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
		combinedSegment := combineSegment(tvb.FinalizingSegment, evictedSegments[0])
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
		if segment.State == cpb.SegmentState_FINALIZED {
			tvb.InsertFinalizedSegment(toSegment(segment))
		} else {
			// Finalizing segment. This will always ever be the last evicted segment.
			tvb.FinalizingSegment = toSegment(segment)
			tvb.IsFinalizingSegmentDirty = true
		}
	}

	// evictedRuns is ordered oldest commit position first,
	// then oldest hour first.
	var evictedRuns []*inputbuffer.Run
	for _, segments := range evictedSegments {
		evictedRuns = append(evictedRuns, segments.Runs...)
	}
	tvb.Statistics = applyStatisticsRetention(insertVerdictsIntoStatistics(tvb.Statistics, evictedRuns))
	tvb.IsStatisticsDirty = true

	// Assert that finalizing segment is after finalized segments.
	tvb.verifyOutputBuffer()
}

func verifyEvictedSegments(evictedSegments []inputbuffer.EvictedSegment) {
	// Verify that evictedSegments contain all FINALIZED segment, except for
	// the last segment.
	for i, seg := range evictedSegments {
		if i != len(evictedSegments)-1 {
			if seg.State != cpb.SegmentState_FINALIZED {
				panic("evictedSegments should contains all finalized segments, except the last one")
			}
		} else {
			if seg.State != cpb.SegmentState_FINALIZING {
				panic("last segment of evicted segments should be finalizing")
			}
		}
		if err := inputbuffer.VerifyRunsOrdered(seg.Runs); err != nil {
			panic(errors.Fmt("segment %v: %w", i, err))
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
func combineSegment(finalizingSegment *cpb.Segment, evictedSegment inputbuffer.EvictedSegment) *cpb.Segment {
	if finalizingSegment.State != cpb.SegmentState_FINALIZING {
		panic("finalizing segment should be in FINALIZING state")
	}
	result := &cpb.Segment{
		State: evictedSegment.State,
		// Use the start position information provided by prior evictions.
		HasStartChangepoint:          finalizingSegment.HasStartChangepoint,
		StartPosition:                finalizingSegment.StartPosition,
		StartHour:                    finalizingSegment.StartHour,
		StartPositionLowerBound_99Th: finalizingSegment.StartPositionLowerBound_99Th,
		StartPositionUpperBound_99Th: finalizingSegment.StartPositionUpperBound_99Th,
		StartPositionDistribution:    finalizingSegment.StartPositionDistribution,
		// Update counts.
		FinalizedCounts: AddCounts(finalizingSegment.FinalizedCounts, evictedSegment.Runs),
	}
	var lastUnexpectedResultHour time.Time
	if finalizingSegment.MostRecentUnexpectedResultHour != nil {
		lastUnexpectedResultHour = finalizingSegment.MostRecentUnexpectedResultHour.AsTime()
	}
	if evictedSegment.MostRecentUnexpectedResultHour.After(lastUnexpectedResultHour) {
		lastUnexpectedResultHour = evictedSegment.MostRecentUnexpectedResultHour
	}
	result.MostRecentUnexpectedResultHour = toTimestampOrNil(lastUnexpectedResultHour)

	if evictedSegment.State == cpb.SegmentState_FINALIZED {
		// Copy properties only set on finalized segments.
		result.EndPosition = evictedSegment.EndPosition
		result.EndHour = timestamppb.New(evictedSegment.EndHour)

		result.FinalizedCounts = flattenCounts(result.FinalizedCounts)
	}
	return result
}

// toSegment converts an evicted segment to a new segment. This method
// should only be used when there is no need to merge with a previous
// finalizing segment.
func toSegment(evictedSegment inputbuffer.EvictedSegment) *cpb.Segment {
	var startLowerBound99th, startUpperBound99th int64
	if evictedSegment.HasStartChangepoint {
		startLowerBound99th, startUpperBound99th = evictedSegment.StartPositionDistribution.ConfidenceInterval(0.99)
	}

	result := &cpb.Segment{
		State:                          evictedSegment.State,
		HasStartChangepoint:            evictedSegment.HasStartChangepoint,
		StartPosition:                  evictedSegment.StartPosition,
		StartHour:                      timestamppb.New(evictedSegment.StartHour),
		StartPositionLowerBound_99Th:   startLowerBound99th,
		StartPositionUpperBound_99Th:   startUpperBound99th,
		StartPositionDistribution:      evictedSegment.StartPositionDistribution.Serialize(),
		MostRecentUnexpectedResultHour: toTimestampOrNil(evictedSegment.MostRecentUnexpectedResultHour),
		FinalizedCounts:                AddCounts(nil, evictedSegment.Runs),
	}
	if evictedSegment.State == cpb.SegmentState_FINALIZED {
		// Copy properties only set on finalized segments.
		result.EndPosition = evictedSegment.EndPosition
		result.EndHour = timestamppb.New(evictedSegment.EndHour)

		result.FinalizedCounts = flattenCounts(result.FinalizedCounts)
	}
	return result
}

func toTimestampOrNil(t time.Time) *timestamppb.Timestamp {
	if t == (time.Time{}) {
		return nil
	}
	return timestamppb.New(t)
}

// AddCounts updates counts with the given new runs.
func AddCounts(counts *cpb.Counts, runs []*inputbuffer.Run) *cpb.Counts {
	var result *cpb.Counts
	if counts == nil {
		result = &cpb.Counts{}
	} else {
		result = proto.Clone(counts).(*cpb.Counts)
	}

	verdictStream := NewRunStreamAggregator(result.PartialSourceVerdict)
	for _, run := range runs {
		// Result-level statistics.
		result.TotalResults += int64(run.Expected.Count() + run.Unexpected.Count())
		result.UnexpectedResults += int64(run.Unexpected.Count())
		result.ExpectedPassedResults += int64(run.Expected.PassCount)
		result.ExpectedFailedResults += int64(run.Expected.FailCount)
		result.ExpectedCrashedResults += int64(run.Expected.CrashCount)
		result.ExpectedAbortedResults += int64(run.Expected.AbortCount)
		result.UnexpectedPassedResults += int64(run.Unexpected.PassCount)
		result.UnexpectedFailedResults += int64(run.Unexpected.FailCount)
		result.UnexpectedCrashedResults += int64(run.Unexpected.CrashCount)
		result.UnexpectedAbortedResults += int64(run.Unexpected.AbortCount)

		// Run-level statistics.
		result.TotalRuns++
		// flaky run.
		isFlakyRun := run.Expected.Count() > 0 && run.Unexpected.Count() > 0
		if isFlakyRun {
			result.FlakyRuns++
		}
		// unexpected unretried run.
		isUnexpectedUnretried := run.Unexpected.Count() == 1 && run.Expected.Count() == 0
		if isUnexpectedUnretried {
			result.UnexpectedUnretriedRuns++
		}
		// unexpected after retries run.
		isUnexpectedAfterRetries := run.Unexpected.Count() > 1 && run.Expected.Count() == 0
		if isUnexpectedAfterRetries {
			result.UnexpectedAfterRetryRuns++
		}

		v, ok := verdictStream.Insert(run)
		if !ok {
			// No verdict yielded.
			continue
		}
		// Add verdict to hourly bucket.
		result.TotalSourceVerdicts++

		if v.UnexpectedResults > 0 {
			if v.ExpectedResults > 0 {
				result.FlakySourceVerdicts++
			} else {
				result.UnexpectedSourceVerdicts++
			}
		}
	}
	result.PartialSourceVerdict = verdictStream.SaveState()
	return result
}

// flattenCounts flattens the pending source verdict into the stored
// counts. This be called on the segment's counts when the segment
// is finalized and no more runs from the input buffer will be streamed
// into it, but not before.
//
// This does not change the semantics of the *cpb.Counts object, but
// is a slightly cleaner and more efficient representation.
func flattenCounts(counts *cpb.Counts) *cpb.Counts {
	result := proto.Clone(counts).(*cpb.Counts)

	if result.PartialSourceVerdict != nil {
		// Now that the segment is finalized, the partial source verdict
		// can be considered complete.
		pv := result.PartialSourceVerdict
		result.TotalSourceVerdicts++
		if pv.UnexpectedResults > 0 {
			if pv.ExpectedResults > 0 {
				result.FlakySourceVerdicts++
			} else {
				result.UnexpectedSourceVerdicts++
			}
		}
		result.PartialSourceVerdict = nil
	}
	return result
}

// insertVerdictsIntoStatistics updates the given statistics to include
// the given evicted runs. Retention policies are applied.
// Runs are sorted oldest commit position first, then oldest hour first.
func insertVerdictsIntoStatistics(stats *cpb.Statistics, runs []*inputbuffer.Run) *cpb.Statistics {
	bucketByHour := make(map[int64]*cpb.Statistics_HourBucket)
	for _, bucket := range stats.GetHourlyBuckets() {
		// Copy hourly bucket to avoid mutating the passed statistics object.
		bucketByHour[bucket.Hour] = &cpb.Statistics_HourBucket{
			Hour:                     bucket.Hour,
			UnexpectedSourceVerdicts: bucket.UnexpectedSourceVerdicts,
			FlakySourceVerdicts:      bucket.FlakySourceVerdicts,
			TotalSourceVerdicts:      bucket.TotalSourceVerdicts,
		}
	}

	verdictStream := NewRunStreamAggregator(stats.GetPartialSourceVerdict())

	for _, run := range runs {
		v, ok := verdictStream.Insert(run)
		if !ok {
			// No verdict yielded.
			continue
		}

		// Find or create hourly bucket.
		hour := v.LastHour.Unix() / 3600
		bucket, ok := bucketByHour[hour]
		if !ok {
			bucket = &cpb.Statistics_HourBucket{Hour: hour}
			bucketByHour[hour] = bucket
		}

		// Add verdict to hourly bucket.
		bucket.TotalSourceVerdicts++

		if v.UnexpectedResults > 0 {
			if v.ExpectedResults > 0 {
				bucket.FlakySourceVerdicts++
			} else {
				bucket.UnexpectedSourceVerdicts++
			}
		}
	}

	partialSourceVerdict := verdictStream.SaveState()

	buckets := make([]*cpb.Statistics_HourBucket, 0, len(bucketByHour))
	for _, bucket := range bucketByHour {
		buckets = append(buckets, bucket)
	}

	// Sort in ascending order (oldest hour first).
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Hour < buckets[j].Hour
	})

	return &cpb.Statistics{
		PartialSourceVerdict: partialSourceVerdict,
		HourlyBuckets:        buckets,
	}
}

// applyStatisticsRetention applies the retention policies
// to statistics data.
func applyStatisticsRetention(stats *cpb.Statistics) *cpb.Statistics {
	buckets := stats.HourlyBuckets

	var result []*cpb.Statistics_HourBucket

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
		result = append(result, buckets[deleteBeforeIndex+1:]...)
	}
	return &cpb.Statistics{
		HourlyBuckets:        result,
		PartialSourceVerdict: stats.PartialSourceVerdict,
	}
}

type HourlyStats struct {
	TotalSourceVerdicts      int64
	FlakySourceVerdicts      int64
	UnexpectedSourceVerdicts int64
}

// HourlyStatistics returns statistics about the verdicts ingested for
// given test variant branch. Statistics comprise data from both the
// input buffer and the output buffer.
//
// The results are in a map keyed by hour (unix seconds / 3600).
func (tvb *Entry) HourlyStatistics() map[int64]HourlyStats {
	var runs []*inputbuffer.Run
	tvb.InputBuffer.MergeBuffer(&runs)
	updatedStats := insertVerdictsIntoStatistics(tvb.Statistics, runs)

	result := make(map[int64]HourlyStats)
	for _, bucket := range updatedStats.HourlyBuckets {
		result[bucket.Hour] = HourlyStats{
			TotalSourceVerdicts:      bucket.TotalSourceVerdicts,
			FlakySourceVerdicts:      bucket.FlakySourceVerdicts,
			UnexpectedSourceVerdicts: bucket.UnexpectedSourceVerdicts,
		}
	}
	pendingSourceVerdict := updatedStats.PartialSourceVerdict
	if pendingSourceVerdict != nil {
		hour := int64(updatedStats.PartialSourceVerdict.LastHour.GetSeconds() / 3600)
		stats := result[hour]
		stats.TotalSourceVerdicts++
		if pendingSourceVerdict.UnexpectedResults > 0 {
			if pendingSourceVerdict.ExpectedResults > 0 {
				stats.FlakySourceVerdicts++
			} else {
				stats.UnexpectedSourceVerdicts++
			}
		}
		result[hour] = stats
	}
	return result
}
