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

	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	cpb "go.chromium.org/luci/analysis/internal/changepoints/proto"
	"go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
	"go.chromium.org/luci/analysis/pbutil"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
	"go.chromium.org/luci/common/errors"
	"google.golang.org/protobuf/types/known/timestamppb"
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

// RowInput represents the data for a TestVariantBranchRow.
// It consists of TestVariantBranch and segments of the input buffer.
// Another alternative here is to run the changepoint analysis again for the
// input buffer to get the segments, but it may be expensive, so we
// made a decision to reuse the input segments from the existing analysis.
type RowInput struct {
	TestVariantBranch *testvariantbranch.Entry
	// InputBufferSegments is the remaining segments in the input buffer of
	// TestVariantBranch, after changepoint analysis and eviction process.
	// Segments are sorted by commit position (lowest/oldest first).
	InputBufferSegments []*inputbuffer.Segment
}

// RowInputs is the contains the rows to be exported to BigQuery, together with
// the Spanner commit timestamp.
type RowInputs struct {
	Rows            []*RowInput
	CommitTimestamp time.Time
}

// ExportTestVariantBranches exports test variant branches to BigQuery.
func (e *Exporter) ExportTestVariantBranches(ctx context.Context, rowInputs *RowInputs) error {
	rows := make([]*bqpb.TestVariantBranchRow, len(rowInputs.Rows))
	for i, ri := range rowInputs.Rows {
		row, err := ToBigQueryRow(ctx, ri, rowInputs.CommitTimestamp)
		if err != nil {
			return errors.Annotate(err, "to bigquery row").Err()
		}
		rows[i] = row
	}
	err := e.client.Insert(ctx, rows)
	if err != nil {
		return errors.Annotate(err, "insert rows").Err()
	}
	return nil
}

// ToBigQueryRow converts a RowInput to a BigQuery TestVariantBranchRow.
// commitTimestamp is the latest spanner commit timestamp of the
// TestVariantBranch of the RowInput.
func ToBigQueryRow(ctx context.Context, ri *RowInput, commitTimestamp time.Time) (*bqpb.TestVariantBranchRow, error) {
	tvb := ri.TestVariantBranch
	row := &bqpb.TestVariantBranchRow{
		Project:     tvb.Project,
		TestId:      tvb.TestID,
		VariantHash: tvb.VariantHash,
		RefHash:     hex.EncodeToString(tvb.RefHash),
		Ref:         tvb.SourceRef,
	}

	// Variant.
	variant, err := pbutil.VariantToJSON(tvb.Variant)
	if err != nil {
		return nil, errors.Annotate(err, "variant to json").Err()
	}
	row.Variant = variant

	// Has recent unexpected result.
	if hasRecentUnexpectedResult(ctx, ri, commitTimestamp) {
		row.HasRecentUnexpectedResults = 1
	}

	// Segments.
	row.Segments = toSegments(ri)

	row.Version = timestamppb.New(commitTimestamp)
	return row, nil
}

// toSegments returns the segments for row input.
// The segments returned will be sorted, with the most recent segment
// comes first.
func toSegments(ri *RowInput) []*bqpb.Segment {
	results := []*bqpb.Segment{}
	tvb := ri.TestVariantBranch
	inputSegments := ri.InputBufferSegments

	// The index where the active segments starts.
	// If there is a finalizing segment, then the we need to first combine it will
	// the first segment from the input buffer.
	activeStartIndex := 0
	if tvb.FinalizingSegment != nil {
		activeStartIndex = 1
	}

	// Add the active segments.
	for i := len(inputSegments) - 1; i >= activeStartIndex; i-- {
		inputSegment := inputSegments[i]
		bqSegment := inputSegmentToBQSegment(inputSegment)
		results = append(results, bqSegment)
	}

	// Add the finalizing segment.
	if tvb.FinalizingSegment != nil {
		bqSegment := combineSegment(tvb.FinalizingSegment, inputSegments[0])
		results = append(results, bqSegment)
	}

	// Add the finalizing segments.
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
		StartHour:                    finalizingSegment.StartHour,
		StartPositionLowerBound_99Th: finalizingSegment.StartPositionLowerBound_99Th,
		StartPositionUpperBound_99Th: finalizingSegment.StartPositionUpperBound_99Th,
		EndPosition:                  inputSegment.EndPosition,
		EndHour:                      inputSegment.EndHour,
		Counts:                       countsToBQCounts(testvariantbranch.AddCounts(finalizingSegment.FinalizedCounts, inputSegment.Counts)),
	}
}

func inputSegmentToBQSegment(segment *inputbuffer.Segment) *bqpb.Segment {
	return &bqpb.Segment{
		HasStartChangepoint:          segment.HasStartChangepoint,
		StartPosition:                segment.StartPosition,
		StartPositionLowerBound_99Th: segment.StartPositionLowerBound99Th,
		StartPositionUpperBound_99Th: segment.StartPositionUpperBound99Th,
		StartHour:                    segment.StartHour,
		EndPosition:                  segment.EndPosition,
		EndHour:                      segment.EndHour,
		Counts:                       countsToBQCounts(segment.Counts),
	}
}

func segmentToBQSegment(segment *cpb.Segment) *bqpb.Segment {
	return &bqpb.Segment{
		HasStartChangepoint:          segment.HasStartChangepoint,
		StartPosition:                segment.StartPosition,
		StartPositionLowerBound_99Th: segment.StartPositionLowerBound_99Th,
		StartPositionUpperBound_99Th: segment.StartPositionUpperBound_99Th,
		StartHour:                    segment.StartHour,
		EndPosition:                  segment.EndPosition,
		EndHour:                      segment.EndHour,
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
	}
}

// hasRecentUnexpectedResult returns true if ri has any unexpected result
// in the last 90 days.
// It is used for partitioning.
func hasRecentUnexpectedResult(ctx context.Context, ri *RowInput, commitTimestamp time.Time) bool {
	// Check input segments.
	for _, inputSegment := range ri.InputBufferSegments {
		if inputSegmentHasRecentUnexpectedResult(ctx, inputSegment, commitTimestamp) {
			return true
		}
	}

	tvb := ri.TestVariantBranch
	// Check finalizing segment.
	if segmentHasRecentUnexpectedResult(ctx, tvb.FinalizingSegment, commitTimestamp) {
		return true
	}

	// Check finalized segments.
	if tvb.FinalizedSegments != nil {
		for _, segment := range tvb.FinalizedSegments.Segments {
			if segmentHasRecentUnexpectedResult(ctx, segment, commitTimestamp) {
				return true
			}
		}
	}

	return false
}

func segmentHasRecentUnexpectedResult(ctx context.Context, segment *cpb.Segment, commitTimestamp time.Time) bool {
	if segment == nil {
		return false
	}
	if segment.MostRecentUnexpectedResultHour == nil {
		return false
	}
	unexpectedTime := segment.MostRecentUnexpectedResultHour.AsTime()
	return commitTimestamp.Sub(unexpectedTime).Hours() <= recentUnexpectedResultThresholdHours
}

func inputSegmentHasRecentUnexpectedResult(ctx context.Context, inputSegment *inputbuffer.Segment, commitTimestamp time.Time) bool {
	if inputSegment == nil {
		return false
	}
	if inputSegment.MostRecentUnexpectedResultHourAllVerdicts == nil {
		return false
	}
	unexpectedTime := inputSegment.MostRecentUnexpectedResultHourAllVerdicts.AsTime()
	return commitTimestamp.Sub(unexpectedTime).Hours() <= recentUnexpectedResultThresholdHours
}
