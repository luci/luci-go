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

	"go.chromium.org/luci/analysis/internal/bqutil"
	"go.chromium.org/luci/analysis/internal/changepoints/analyzer"
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
		return errors.Fmt("insert rows: %w", err)
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
// segments is the logical segments on the TestVariantBranch.
// Segments are sorted by commit position (most recent first).
//
// To support re-use of the *testvariantbranch.Entry buffer, no reference
// to tvb or inputBufferSegments or their fields (except immutable strings)
// will be retained by this method or its result. (All data will
// be copied.)
func ToPartialBigQueryRow(tvb *testvariantbranch.Entry, segments []analyzer.Segment) (PartialBigQueryRow, error) {
	testIDStructured, err := bqutil.StructuredTestIdentifier(tvb.TestID, tvb.Variant)
	if err != nil {
		return PartialBigQueryRow{}, errors.Fmt("structured test identifier: %w", err)
	}

	row := &bqpb.TestVariantBranchRow{
		Project:          tvb.Project,
		TestIdStructured: testIDStructured,
		TestId:           tvb.TestID,
		VariantHash:      tvb.VariantHash,
		RefHash:          hex.EncodeToString(tvb.RefHash),
		Ref:              proto.Clone(tvb.SourceRef).(*analysispb.SourceRef),
	}

	// Variant.
	variant, err := pbutil.VariantToJSON(tvb.Variant)
	if err != nil {
		return PartialBigQueryRow{}, errors.Fmt("variant to json: %w", err)
	}
	row.Variant = variant

	// Segments.
	row.Segments = toBQSegments(segments)

	// The row is partial because HasRecentUnexpectedResults and Version
	// is not yet populated. Wrap it in another type to prevent export as-is.
	return PartialBigQueryRow{
		row:                            row,
		mostRecentUnexpectedResultHour: mostRecentUnexpectedResult(segments),
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

func toBQSegments(segments []analyzer.Segment) []*bqpb.Segment {
	result := make([]*bqpb.Segment, 0, len(segments))
	for _, s := range segments {
		result = append(result, toBQSegment(s))
	}
	return result
}

func toBQSegment(s analyzer.Segment) *bqpb.Segment {
	return &bqpb.Segment{
		HasStartChangepoint:          s.HasStartChangepoint,
		StartPosition:                s.StartPosition,
		StartHour:                    timestamppb.New(s.StartHour),
		StartPositionLowerBound_99Th: s.StartPositionLowerBound99Th,
		StartPositionUpperBound_99Th: s.StartPositionUpperBound99Th,
		EndPosition:                  s.EndPosition,
		EndHour:                      timestamppb.New(s.EndHour),
		Counts:                       toBQCounts(s.Counts),
	}
}

func toBQCounts(counts analyzer.Counts) *bqpb.Segment_Counts {
	return &bqpb.Segment_Counts{
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

		TotalRuns:                counts.TotalRuns,
		FlakyRuns:                counts.FlakyRuns,
		UnexpectedUnretriedRuns:  counts.UnexpectedUnretriedRuns,
		UnexpectedAfterRetryRuns: counts.UnexpectedAfterRetryRuns,

		TotalVerdicts:      counts.TotalSourceVerdicts,
		UnexpectedVerdicts: counts.UnexpectedSourceVerdicts,
		FlakyVerdicts:      counts.FlakySourceVerdicts,
	}
}

// mostRecentUnexpectedResult returns the most recent unexpected result,
// if any. If there is no recent unexpected result, return the zero time
// (time.Time{}).
func mostRecentUnexpectedResult(segments []analyzer.Segment) time.Time {
	var mostRecentUnexpected time.Time

	for _, segment := range segments {
		if segment.MostRecentUnexpectedResultHour.After(mostRecentUnexpected) {
			mostRecentUnexpected = segment.MostRecentUnexpectedResultHour
		}
	}

	return mostRecentUnexpected
}
