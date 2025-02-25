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

package bqexporter

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/analysis/internal/changepoints/analyzer"
	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	"go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestBQExporter(t *testing.T) {
	ftt.Run(`Export test variant branches`, t, func(t *ftt.Test) {
		ctx := context.Background()
		client := NewFakeClient()
		exporter := NewExporter(client)

		// Create row input data.
		variant := &pb.Variant{
			Def: map[string]string{
				"k": "v",
			},
		}

		sourceRef := &pb.SourceRef{
			System: &pb.SourceRef_Gitiles{
				Gitiles: &pb.GitilesRef{
					Host:    "host",
					Project: "proj",
					Ref:     "ref",
				},
			},
		}

		type RowInput struct {
			TestVariantBranch *testvariantbranch.Entry
			// Segments are sorted by commit position (most recent first).
			Segments []analyzer.Segment
		}

		row1 := &RowInput{
			TestVariantBranch: &testvariantbranch.Entry{
				Project:     "chromium",
				TestID:      "test_id_1",
				VariantHash: "variant_hash_1",
				RefHash:     []byte("refhash1"),
				Variant:     variant,
				SourceRef:   sourceRef,
			},
			Segments: []analyzer.Segment{
				{
					HasStartChangepoint:         false,
					StartPosition:               1,
					StartPositionLowerBound99Th: 1,
					StartPositionUpperBound99Th: 3,
					EndPosition:                 6,
					StartHour:                   time.Unix(3600, 0),
					EndHour:                     time.Unix(6*3600, 0),
					Counts: analyzer.Counts{
						TotalResults: 9,
					},
				},
			},
		}

		row2 := &RowInput{
			TestVariantBranch: &testvariantbranch.Entry{
				Project:     "chromium",
				TestID:      "test_id_2",
				VariantHash: "variant_hash_2",
				RefHash:     []byte("refhash2"),
				Variant:     variant,
				SourceRef:   sourceRef,
			},
			Segments: []analyzer.Segment{
				{
					HasStartChangepoint:         true,
					StartPosition:               41,
					StartPositionLowerBound99Th: 39,
					StartPositionUpperBound99Th: 43,
					EndPosition:                 40,
					StartHour:                   time.Unix(3600, 0),
					EndHour:                     time.Unix(6*3600, 0),
					Counts: analyzer.Counts{
						TotalSourceVerdicts: 6,
					},
				},
				{
					HasStartChangepoint:            true,
					StartPosition:                  21,
					StartPositionLowerBound99Th:    19,
					StartPositionUpperBound99Th:    23,
					EndPosition:                    40,
					StartHour:                      time.Unix(7000*3600, 0),
					EndHour:                        time.Unix(6*3600, 0),
					MostRecentUnexpectedResultHour: time.Unix(9000*3600, 0),
					Counts: analyzer.Counts{
						TotalSourceVerdicts:      9,
						UnexpectedSourceVerdicts: 4,
						FlakySourceVerdicts:      2,
						TotalRuns:                13,
						UnexpectedUnretriedRuns:  5,
						UnexpectedAfterRetryRuns: 5,
						FlakyRuns:                6,
						TotalResults:             21,
						UnexpectedResults:        15,
					},
				},
				{
					HasStartChangepoint:         true,
					StartPosition:               11,
					StartHour:                   time.Unix(7000*3600, 0),
					StartPositionLowerBound99Th: 9,
					StartPositionUpperBound99Th: 13,
					EndPosition:                 20,
					EndHour:                     time.Unix(8000*3600, 0),
					Counts: analyzer.Counts{
						TotalSourceVerdicts: 5,
					},
				},
				{
					HasStartChangepoint: false,
					StartPosition:       1,
					StartHour:           time.Unix(7000*3600, 0),
					EndPosition:         10,
					EndHour:             time.Unix(8000*3600, 0),
					// Less than 90 days ago
					MostRecentUnexpectedResultHour: time.Unix(9000*3600, 0),
					Counts: analyzer.Counts{
						TotalSourceVerdicts:      3,
						UnexpectedSourceVerdicts: 2,
						FlakySourceVerdicts:      1,
						TotalRuns:                7,
						UnexpectedUnretriedRuns:  6,
						UnexpectedAfterRetryRuns: 5,
						FlakyRuns:                4,
						TotalResults:             9,
						UnexpectedResults:        8,
					},
				},
			},
		}

		row3 := &RowInput{
			TestVariantBranch: &testvariantbranch.Entry{
				Project:     "chromium",
				TestID:      "test_id_3",
				VariantHash: "variant_hash_3",
				RefHash:     []byte("refhash3"),
				Variant:     variant,
				SourceRef:   sourceRef,
				InputBuffer: &inputbuffer.Buffer{},
			},
			Segments: []analyzer.Segment{
				{
					HasStartChangepoint:         true,
					StartPosition:               1,
					StartPositionLowerBound99Th: 1,
					StartPositionUpperBound99Th: 3,
					EndPosition:                 6,
					StartHour:                   time.Unix(3600, 0),
					EndHour:                     time.Unix(6*3600, 0),
					// More than 90 days ago.
					MostRecentUnexpectedResultHour: time.Unix(7000*3600, 0),
					Counts: analyzer.Counts{
						TotalResults:      9,
						UnexpectedResults: 4,
					},
				},
			},
		}

		var bqRows []PartialBigQueryRow
		for _, row := range []*RowInput{row1, row2, row3} {
			bqRow, err := ToPartialBigQueryRow(row.TestVariantBranch, row.Segments)
			assert.Loosely(t, err, should.BeNil)
			bqRows = append(bqRows, bqRow)
		}

		ris := RowInputs{
			Rows:            bqRows,
			CommitTimestamp: time.Unix(10000*3600, 0),
		}
		err := exporter.ExportTestVariantBranches(ctx, ris)
		assert.Loosely(t, err, should.BeNil)
		rows := client.Insertions
		assert.Loosely(t, len(rows), should.Equal(3))

		// Asserts the rows.
		assert.Loosely(t, rows[0], should.Match(&bqpb.TestVariantBranchRow{
			Project:     "chromium",
			TestId:      "test_id_1",
			VariantHash: "variant_hash_1",
			RefHash:     hex.EncodeToString([]byte("refhash1")),
			Variant:     "{\"k\":\"v\"}",
			Ref:         sourceRef,
			Version:     timestamppb.New(time.Unix(10000*3600, 0)),
			Segments: []*bqpb.Segment{
				{
					HasStartChangepoint:          false,
					StartPosition:                1,
					StartPositionLowerBound_99Th: 1,
					StartPositionUpperBound_99Th: 3,
					EndPosition:                  6,
					StartHour:                    timestamppb.New(time.Unix(3600, 0)),
					EndHour:                      timestamppb.New(time.Unix(6*3600, 0)),
					Counts: &bqpb.Segment_Counts{
						TotalResults: 9,
					},
				},
			},
		}))

		assert.Loosely(t, rows[1], should.Match(&bqpb.TestVariantBranchRow{
			Project:                    "chromium",
			TestId:                     "test_id_2",
			VariantHash:                "variant_hash_2",
			RefHash:                    hex.EncodeToString([]byte("refhash2")),
			Variant:                    "{\"k\":\"v\"}",
			Ref:                        sourceRef,
			Version:                    timestamppb.New(time.Unix(10000*3600, 0)),
			HasRecentUnexpectedResults: 1,
			Segments: []*bqpb.Segment{
				{
					HasStartChangepoint:          true,
					StartPosition:                41,
					StartPositionLowerBound_99Th: 39,
					StartPositionUpperBound_99Th: 43,
					EndPosition:                  40,
					StartHour:                    timestamppb.New(time.Unix(3600, 0)),
					EndHour:                      timestamppb.New(time.Unix(6*3600, 0)),
					Counts: &bqpb.Segment_Counts{
						TotalVerdicts: 6,
					},
				},
				{
					HasStartChangepoint:          true,
					StartPosition:                21,
					StartPositionLowerBound_99Th: 19,
					StartPositionUpperBound_99Th: 23,
					StartHour:                    timestamppb.New(time.Unix(7000*3600, 0)),
					EndPosition:                  40,
					EndHour:                      timestamppb.New(time.Unix(6*3600, 0)),
					Counts: &bqpb.Segment_Counts{
						TotalVerdicts:      9,
						UnexpectedVerdicts: 4,
						FlakyVerdicts:      2,

						TotalRuns:               13,
						UnexpectedUnretriedRuns: 5,

						UnexpectedAfterRetryRuns: 5,
						FlakyRuns:                6,
						TotalResults:             21,
						UnexpectedResults:        15,
					},
				},
				{
					HasStartChangepoint:          true,
					StartPosition:                11,
					StartHour:                    timestamppb.New(time.Unix(7000*3600, 0)),
					StartPositionLowerBound_99Th: 9,
					StartPositionUpperBound_99Th: 13,
					EndPosition:                  20,
					EndHour:                      timestamppb.New(time.Unix(8000*3600, 0)),
					Counts: &bqpb.Segment_Counts{
						TotalVerdicts: 5,
					},
				},
				{
					HasStartChangepoint: false,
					StartPosition:       1,
					StartHour:           timestamppb.New(time.Unix(7000*3600, 0)),
					EndPosition:         10,
					EndHour:             timestamppb.New(time.Unix(8000*3600, 0)),
					Counts: &bqpb.Segment_Counts{
						TotalVerdicts:            3,
						UnexpectedVerdicts:       2,
						FlakyVerdicts:            1,
						TotalRuns:                7,
						UnexpectedUnretriedRuns:  6,
						UnexpectedAfterRetryRuns: 5,
						FlakyRuns:                4,
						TotalResults:             9,
						UnexpectedResults:        8,
					},
				},
			},
		}))

		assert.Loosely(t, rows[2], should.Match(&bqpb.TestVariantBranchRow{
			Project:     "chromium",
			TestId:      "test_id_3",
			VariantHash: "variant_hash_3",
			RefHash:     hex.EncodeToString([]byte("refhash3")),
			Variant:     "{\"k\":\"v\"}",
			Ref:         sourceRef,
			Version:     timestamppb.New(time.Unix(10000*3600, 0)),
			Segments: []*bqpb.Segment{
				{
					HasStartChangepoint:          true,
					StartPosition:                1,
					StartPositionLowerBound_99Th: 1,
					StartPositionUpperBound_99Th: 3,
					EndPosition:                  6,
					StartHour:                    timestamppb.New(time.Unix(3600, 0)),
					EndHour:                      timestamppb.New(time.Unix(6*3600, 0)),
					Counts: &bqpb.Segment_Counts{
						TotalResults:      9,
						UnexpectedResults: 4,
					},
				},
			},
		}))
	})
}
