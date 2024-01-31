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

// Package bqexporter handles the export of test variant analysis results
// to BigQuery.
package bqexporter

import (
	"context"
	"encoding/hex"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	cpb "go.chromium.org/luci/analysis/internal/changepoints/proto"
	"go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
	"go.chromium.org/luci/analysis/pbutil"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
	analysispb "go.chromium.org/luci/analysis/proto/v1"
)

// If an unexpected test result is within 90 days, it is consider
// recently unexpected.
const recentUnexpectedResultThresholdHours = 90 * 24

// InsertClient defines an interface for inserting TestVariantBranchRow into BigQuery.
type InsertClient interface {
	// Insert inserts the given rows into BigQuery.
	Insert(ctx context.Context, rows []*bqpb.TestVariantBranchRow) error
}

// Exporter provides methods to export test variant branches to BigQuery.
type Exporter struct {
	client InsertClient
}

// NewExporter instantiates a new Exporter. The given client is used
// to insert rows into BigQuery.
func NewExporter(client InsertClient) *Exporter {
	return &Exporter{client: client}
}

// RowInputs is the contains the rows to be exported to BigQuery, together with
// the Spanner commit timestamp.
type RowInputs struct {
	Rows            []PartialBigQueryRow
	CommitTimestamp time.Time
}

// ExportTestVariantBranches exports test variant branches to BigQuery.
func (e *Exporter) ExportTestVariantBranches(ctx context.Context, rowInputs RowInputs) error {
	bqRows := make([]*bqpb.TestVariantBranchRow, len(rowInputs.Rows))
	for i, r := range rowInputs.Rows {
		bqRows[i] = r.Complete(rowInputs.CommitTimestamp)
	}
	err := e.client.Insert(ctx, bqRows)
	if err != nil {
		return errors.Annotate(err, "insert rows").Err()
	}
	return nil
}

// PartialBigQueryRow represents a partially constructed BigQuery
// export row. Call Complete(...) on the row to finish its construction.
type PartialBigQueryRow struct {
	// Field is private to avoid callers mistakenly exporting partially
	// populated rows.
	row                            *bqpb.TestVariantBranchRow
	mostRecentUnexpectedResultHour time.Time
}

// ToPartialBigQueryRow starts building a BigQuery TestVariantBranchRow.
// All fields except those dependent on the commit timestamp, (i.e.
// Version and HasRecentUnexpectedResults) are populated.
//
// inputBufferSegments is the remaining segments in the input buffer of
// TestVariantBranch, after changepoint analysis and eviction process.
// Segments are sorted by commit position (lowest/oldest first).
//
// To support re-use of the *testvariantbranch.Entry buffer, no reference
// to tvb or inputBufferSegments or their fields (except immutable strings)
// will be retained by this method or its result. (All data will
// be copied.)
func ToPartialBigQueryRow(tvb *testvariantbranch.Entry, inputBufferSegments []*inputbuffer.Segment) (PartialBigQueryRow, error) {
	row := &bqpb.TestVariantBranchRow{
		Project:     tvb.Project,
		TestId:      tvb.TestID,
		VariantHash: tvb.VariantHash,
		RefHash:     hex.EncodeToString(tvb.RefHash),
		Ref:         proto.Clone(tvb.SourceRef).(*analysispb.SourceRef),
	}

	// Variant.
	variant, err := pbutil.VariantToJSON(tvb.Variant)
	if err != nil {
		return PartialBigQueryRow{}, errors.Annotate(err, "variant to json").Err()
	}
	row.Variant = variant

	// Segments.
	row.Segments = toSegments(tvb, inputBufferSegments)

	// The row is partial because HasRecentUnexpectedResults and Version
	// is not yet populated. Wrap it in another type to prevent export as-is.
	return PartialBigQueryRow{
		row:                            row,
		mostRecentUnexpectedResultHour: mostRecentUnexpectedResult(tvb, inputBufferSegments),
	}, nil
}

// Complete finishes creating the BigQuery export row, returning it.
func (r PartialBigQueryRow) Complete(commitTimestamp time.Time) *bqpb.TestVariantBranchRow {
	row := r.row

	// Has recent unexpected result.
	if commitTimestamp.Sub(r.mostRecentUnexpectedResultHour).Hours() <= recentUnexpectedResultThresholdHours {
		row.HasRecentUnexpectedResults = 1
	}

	row.Version = timestamppb.New(commitTimestamp)
	return row
}

// toSegments returns the segments for row input.
// The segments returned will be sorted, with the most recent segment
// comes first.
func toSegments(tvb *testvariantbranch.Entry, inputBufferSegments []*inputbuffer.Segment) []*bqpb.Segment {
	results := []*bqpb.Segment{}

	// The index where the active segments starts.
	// If there is a finalizing segment, then the we need to first combine it will
	// the first segment from the input buffer.
	activeStartIndex := 0
	if tvb.FinalizingSegment != nil {
		activeStartIndex = 1
	}

	// Add the active segments.
	for i := len(inputBufferSegments) - 1; i >= activeStartIndex; i-- {
		inputSegment := inputBufferSegments[i]
		bqSegment := inputSegmentToBQSegment(inputSegment)
		results = append(results, bqSegment)
	}

	// Add the finalizing segment.
	if tvb.FinalizingSegment != nil {
		bqSegment := combineSegment(tvb.FinalizingSegment, inputBufferSegments[0])
		results = append(results, bqSegment)
	}

	// Add the finalized segments.
	if tvb.FinalizedSegments != nil {
		// More recent segments are on the back.
		for i := len(tvb.FinalizedSegments.Segments) - 1; i >= 0; i-- {
			segment := tvb.FinalizedSegments.Segments[i]
			bqSegment := segmentToBQSegment(segment)
			results = append(results, bqSegment)
		}
	}

	return results
}

// combineSegment constructs a finalizing segment from its finalized part in
// the output buffer and its unfinalized part in the input buffer.
func combineSegment(finalizingSegment *cpb.Segment, inputSegment *inputbuffer.Segment) *bqpb.Segment {
	return &bqpb.Segment{
		HasStartChangepoint:          finalizingSegment.HasStartChangepoint,
		StartPosition:                finalizingSegment.StartPosition,
		StartHour:                    timestamppb.New(finalizingSegment.StartHour.AsTime()),
		StartPositionLowerBound_99Th: finalizingSegment.StartPositionLowerBound_99Th,
		StartPositionUpperBound_99Th: finalizingSegment.StartPositionUpperBound_99Th,
		EndPosition:                  inputSegment.EndPosition,
		EndHour:                      timestamppb.New(inputSegment.EndHour.AsTime()),
		Counts:                       countsToBQCounts(testvariantbranch.AddCounts(finalizingSegment.FinalizedCounts, inputSegment.Counts)),
	}
}

func inputSegmentToBQSegment(segment *inputbuffer.Segment) *bqpb.Segment {
	return &bqpb.Segment{
		HasStartChangepoint:          segment.HasStartChangepoint,
		StartPosition:                segment.StartPosition,
		StartPositionLowerBound_99Th: segment.StartPositionLowerBound99Th,
		StartPositionUpperBound_99Th: segment.StartPositionUpperBound99Th,
		StartHour:                    timestamppb.New(segment.StartHour.AsTime()),
		EndPosition:                  segment.EndPosition,
		EndHour:                      timestamppb.New(segment.EndHour.AsTime()),
		Counts:                       countsToBQCounts(segment.Counts),
	}
}

func segmentToBQSegment(segment *cpb.Segment) *bqpb.Segment {
	return &bqpb.Segment{
		HasStartChangepoint:          segment.HasStartChangepoint,
		StartPosition:                segment.StartPosition,
		StartPositionLowerBound_99Th: segment.StartPositionLowerBound_99Th,
		StartPositionUpperBound_99Th: segment.StartPositionUpperBound_99Th,
		StartHour:                    timestamppb.New(segment.StartHour.AsTime()),
		EndPosition:                  segment.EndPosition,
		EndHour:                      timestamppb.New(segment.EndHour.AsTime()),
		Counts:                       countsToBQCounts(segment.FinalizedCounts),
	}
}

func countsToBQCounts(counts *cpb.Counts) *bqpb.Segment_Counts {
	return &bqpb.Segment_Counts{
		TotalVerdicts:            counts.TotalVerdicts,
		UnexpectedVerdicts:       counts.UnexpectedVerdicts,
		FlakyVerdicts:            counts.FlakyVerdicts,
		TotalRuns:                counts.TotalRuns,
		FlakyRuns:                counts.FlakyRuns,
		UnexpectedUnretriedRuns:  counts.UnexpectedUnretriedRuns,
		UnexpectedAfterRetryRuns: counts.UnexpectedAfterRetryRuns,
		TotalResults:             counts.TotalResults,
		UnexpectedResults:        counts.UnexpectedResults,
		ExpectedPassedResults:    counts.ExpectedPassedResults,
		ExpectedFailedResults:    counts.ExpectedFailedResults,
		ExpectedCrashedResults:   counts.ExpectedCrashedResults,
		ExpectedAbortedResults:   counts.ExpectedAbortedResults,
		UnexpectedPassedResults:  counts.UnexpectedPassedResults,
		UnexpectedFailedResults:  counts.UnexpectedFailedResults,
		UnexpectedCrashedResults: counts.UnexpectedCrashedResults,
		UnexpectedAbortedResults: counts.UnexpectedAbortedResults,
	}
}

// mostRecentUnexpectedResult returns the most recent unexpected result,
// if any. If there is no recent unexpected result, return the zero time
// (time.Time{}).
func mostRecentUnexpectedResult(tvb *testvariantbranch.Entry, inputBufferSegments []*inputbuffer.Segment) time.Time {
	var mostRecentUnexpected time.Time

	// Check input segments.
	for _, segment := range inputBufferSegments {
		if segment.MostRecentUnexpectedResultHourAllVerdicts != nil {
			time := segment.MostRecentUnexpectedResultHourAllVerdicts.AsTime()
			if time.After(mostRecentUnexpected) {
				mostRecentUnexpected = time
			}
		}

	}

	// Check finalizing segment.
	if tvb.FinalizingSegment != nil && tvb.FinalizingSegment.MostRecentUnexpectedResultHour != nil {
		time := tvb.FinalizingSegment.MostRecentUnexpectedResultHour.AsTime()
		if time.After(mostRecentUnexpected) {
			mostRecentUnexpected = time
		}
	}

	// Check finalized segments.
	if tvb.FinalizedSegments != nil {
		for _, segment := range tvb.FinalizedSegments.Segments {
			if segment.MostRecentUnexpectedResultHour != nil {
				time := segment.MostRecentUnexpectedResultHour.AsTime()
				if time.After(mostRecentUnexpected) {
					mostRecentUnexpected = time
				}
			}
		}
	}

	return mostRecentUnexpected
}
