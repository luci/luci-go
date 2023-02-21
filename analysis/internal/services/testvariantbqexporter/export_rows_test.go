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

package testvariantbqexporter

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/clock"

	"go.chromium.org/luci/analysis/internal"
	"go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/internal/testutil/insert"
	"go.chromium.org/luci/analysis/pbutil"
	atvpb "go.chromium.org/luci/analysis/proto/analyzedtestvariant"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
	pb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type mockPassInserter struct {
	insertedMessages []*bq.Row
	mu               sync.Mutex
}

func (i *mockPassInserter) PutWithRetries(ctx context.Context, src []*bq.Row) error {
	i.mu.Lock()
	i.insertedMessages = append(i.insertedMessages, src...)
	i.mu.Unlock()
	return nil
}

type mockFailInserter struct {
}

func (i *mockFailInserter) PutWithRetries(ctx context.Context, src []*bq.Row) error {
	return fmt.Errorf("some error")
}

func TestQueryTestVariantsToExport(t *testing.T) {
	Convey(`queryTestVariantsToExport`, t, func() {
		ctx := testutil.IntegrationTestContext(t)
		realm := "chromium:ci"
		tID1 := "ninja://test1"
		tID2 := "ninja://test2"
		tID3 := "ninja://test3"
		tID4 := "ninja://test4"
		tID5 := "ninja://test5"
		tID6 := "ninja://test6"
		tID7 := "ninja://test7"
		variant := pbutil.Variant("builder", "Linux Tests")
		vh := "varianthash"
		tags := pbutil.StringPairs("k1", "v1")
		tmd := &pb.TestMetadata{
			Location: &pb.TestLocation{
				Repo:     "https://chromium.googlesource.com/chromium/src",
				FileName: "//a_test.go",
			},
		}
		tmdM, _ := proto.Marshal(tmd)
		now := clock.Now(ctx).Round(time.Microsecond)
		start := clock.Now(ctx).Add(-time.Hour)
		twoAndHalfHAgo := now.Add(-150 * time.Minute)
		oneAndHalfHAgo := now.Add(-90 * time.Minute)
		halfHAgo := now.Add(-30 * time.Minute)
		m46Ago := now.Add(-46 * time.Minute)
		ms := []*spanner.Mutation{
			insert.AnalyzedTestVariant(realm, tID1, vh, atvpb.Status_FLAKY, map[string]any{
				"Variant":          variant,
				"Tags":             tags,
				"TestMetadata":     span.Compressed(tmdM),
				"StatusUpdateTime": start.Add(-time.Hour),
			}),
			// New flaky test variant.
			insert.AnalyzedTestVariant(realm, tID2, vh, atvpb.Status_FLAKY, map[string]any{
				"Variant":          variant,
				"Tags":             tags,
				"TestMetadata":     span.Compressed(tmdM),
				"StatusUpdateTime": halfHAgo,
			}),
			// Flaky test with no verdicts in time range.
			insert.AnalyzedTestVariant(realm, tID3, vh, atvpb.Status_FLAKY, map[string]any{
				"Variant":          variant,
				"Tags":             tags,
				"TestMetadata":     span.Compressed(tmdM),
				"StatusUpdateTime": start.Add(-time.Hour),
			}),
			// Test variant with another status is not exported.
			insert.AnalyzedTestVariant(realm, tID4, vh, atvpb.Status_CONSISTENTLY_UNEXPECTED, map[string]any{
				"Variant":          variant,
				"Tags":             tags,
				"TestMetadata":     span.Compressed(tmdM),
				"StatusUpdateTime": start.Add(-time.Hour),
			}),
			// Test variant has multiple status updates.
			insert.AnalyzedTestVariant(realm, tID5, vh, atvpb.Status_FLAKY, map[string]any{
				"Variant":          variant,
				"Tags":             tags,
				"TestMetadata":     span.Compressed(tmdM),
				"StatusUpdateTime": halfHAgo,
				"PreviousStatuses": []atvpb.Status{
					atvpb.Status_CONSISTENTLY_EXPECTED,
					atvpb.Status_FLAKY},
				"PreviousStatusUpdateTimes": []time.Time{
					m46Ago,
					now.Add(-24 * time.Hour)},
			}),
			// Test variant with different variant.
			insert.AnalyzedTestVariant(realm, tID6, "c467ccce5a16dc72", atvpb.Status_CONSISTENTLY_EXPECTED, map[string]any{
				"Variant":          pbutil.Variant("a", "b"),
				"Tags":             tags,
				"TestMetadata":     span.Compressed(tmdM),
				"StatusUpdateTime": twoAndHalfHAgo,
			}),
			// Test variant with exonerated verdict.
			insert.AnalyzedTestVariant(realm, tID7, vh, atvpb.Status_FLAKY, map[string]any{
				"Variant":          variant,
				"Tags":             tags,
				"TestMetadata":     span.Compressed(tmdM),
				"StatusUpdateTime": start.Add(-time.Hour),
			}),
			insert.Verdict(realm, tID1, vh, "build-0", internal.VerdictStatus_EXPECTED, twoAndHalfHAgo, map[string]any{
				"IngestionTime":         oneAndHalfHAgo,
				"UnexpectedResultCount": 0,
				"TotalResultCount":      1,
				"Exonerated":            false,
			}),
			insert.Verdict(realm, tID1, vh, "build-1", internal.VerdictStatus_VERDICT_FLAKY, twoAndHalfHAgo, map[string]any{
				"IngestionTime":         halfHAgo,
				"UnexpectedResultCount": 1,
				"TotalResultCount":      2,
				"Exonerated":            false,
			}),
			insert.Verdict(realm, tID1, vh, "build-2", internal.VerdictStatus_EXPECTED, oneAndHalfHAgo, map[string]any{
				"IngestionTime":         halfHAgo,
				"UnexpectedResultCount": 0,
				"TotalResultCount":      1,
				"Exonerated":            false,
			}),
			insert.Verdict(realm, tID2, vh, "build-2", internal.VerdictStatus_VERDICT_FLAKY, oneAndHalfHAgo, map[string]any{
				"IngestionTime":         halfHAgo,
				"UnexpectedResultCount": 1,
				"TotalResultCount":      2,
				"Exonerated":            false,
			}),
			insert.Verdict(realm, tID5, vh, "build-1", internal.VerdictStatus_EXPECTED, twoAndHalfHAgo, map[string]any{
				"IngestionTime":         now.Add(-45 * time.Minute),
				"UnexpectedResultCount": 0,
				"TotalResultCount":      1,
				"Exonerated":            false,
			}),
			insert.Verdict(realm, tID5, vh, "build-2", internal.VerdictStatus_VERDICT_FLAKY, oneAndHalfHAgo, map[string]any{
				"IngestionTime":         halfHAgo,
				"UnexpectedResultCount": 1,
				"TotalResultCount":      2,
				"Exonerated":            false,
			}),
			insert.Verdict(realm, tID7, vh, "build-1", internal.VerdictStatus_VERDICT_FLAKY, oneAndHalfHAgo, map[string]any{
				"IngestionTime":         halfHAgo,
				"UnexpectedResultCount": 1,
				"TotalResultCount":      2,
				"Exonerated":            true,
			}),
		}
		testutil.MustApply(ctx, ms...)

		verdicts := map[string]map[string]*bqpb.Verdict{
			tID1: {
				"build-0": {
					Invocation: "build-0",
					Status:     "EXPECTED",
					CreateTime: timestamppb.New(twoAndHalfHAgo),
					Exonerated: false,
				},
				"build-1": {
					Invocation: "build-1",
					Status:     "VERDICT_FLAKY",
					CreateTime: timestamppb.New(twoAndHalfHAgo),
					Exonerated: false,
				},
				"build-2": {
					Invocation: "build-2",
					Status:     "EXPECTED",
					CreateTime: timestamppb.New(oneAndHalfHAgo),
					Exonerated: false,
				},
			},
			tID2: {
				"build-2": {
					Invocation: "build-2",
					Status:     "VERDICT_FLAKY",
					CreateTime: timestamppb.New(oneAndHalfHAgo),
					Exonerated: false,
				},
			},
			tID5: {
				"build-1": {
					Invocation: "build-1",
					Status:     "EXPECTED",
					CreateTime: timestamppb.New(twoAndHalfHAgo),
					Exonerated: false,
				},
				"build-2": {
					Invocation: "build-2",
					Status:     "VERDICT_FLAKY",
					CreateTime: timestamppb.New(oneAndHalfHAgo),
					Exonerated: false,
				},
			},
			tID7: {
				"build-1": {
					Invocation: "build-1",
					Status:     "VERDICT_FLAKY",
					CreateTime: timestamppb.New(oneAndHalfHAgo),
					Exonerated: true,
				},
			},
		}

		op := &Options{
			Realm:        realm,
			CloudProject: "cloud_project",
			Dataset:      "dataset",
			Table:        "table",
			TimeRange: &pb.TimeRange{
				Earliest: timestamppb.New(start),
				Latest:   timestamppb.New(now),
			},
		}
		br := CreateBQExporter(op)

		// To check when encountering an error, the test can run to the end
		// without hanging, or race detector does not detect anything.
		Convey(`insert fail`, func() {
			err := br.exportTestVariantRows(ctx, &mockFailInserter{})
			So(err, ShouldErrLike, "some error")
		})

		simplifyF := func(rows []*bqpb.TestVariantRow) []*bqpb.TestVariantRow {
			simpleRows := make([]*bqpb.TestVariantRow, len(rows))
			for i, r := range rows {
				simpleRows[i] = &bqpb.TestVariantRow{
					TestId:          r.TestId,
					Status:          r.Status,
					TimeRange:       r.TimeRange,
					FlakeStatistics: r.FlakeStatistics,
					TestMetadata:    r.TestMetadata,
					Verdicts:        r.Verdicts,
				}
			}
			return simpleRows
		}

		sortF := func(rows []*bqpb.TestVariantRow) {
			sort.Slice(rows, func(i, j int) bool {
				switch {
				case rows[i].TestId != rows[j].TestId:
					return rows[i].TestId < rows[j].TestId
				default:
					earliestI, _ := pbutil.AsTime(rows[i].TimeRange.Earliest)
					earliestJ, _ := pbutil.AsTime(rows[j].TimeRange.Earliest)
					return earliestI.Before(earliestJ)
				}
			})
		}

		test := func(predicate *atvpb.Predicate, expRows []*bqpb.TestVariantRow) {
			op.Predicate = predicate
			ins := &mockPassInserter{}
			err := br.exportTestVariantRows(ctx, ins)
			So(err, ShouldBeNil)

			rows := make([]*bqpb.TestVariantRow, len(ins.insertedMessages))
			for i, m := range ins.insertedMessages {
				rows[i] = m.Message.(*bqpb.TestVariantRow)
			}
			rows = simplifyF(rows)
			sortF(rows)
			sortF(expRows)
			So(rows, ShouldResembleProto, expRows)
		}

		Convey(`no predicate`, func() {
			expRows := []*bqpb.TestVariantRow{
				{
					TestId: tID1,
					TimeRange: &pb.TimeRange{
						Earliest: op.TimeRange.Earliest,
						Latest:   op.TimeRange.Latest,
					},
					TestMetadata: tmd,
					Status:       "FLAKY",
					FlakeStatistics: &atvpb.FlakeStatistics{
						FlakyVerdictRate:      0.5,
						FlakyVerdictCount:     1,
						TotalVerdictCount:     2,
						UnexpectedResultRate:  float32(1) / 3,
						UnexpectedResultCount: 1,
						TotalResultCount:      3,
					},
					Verdicts: []*bqpb.Verdict{
						verdicts[tID1]["build-2"],
						verdicts[tID1]["build-1"],
					},
				},
				{
					TestId: tID4,
					TimeRange: &pb.TimeRange{
						Earliest: op.TimeRange.Earliest,
						Latest:   op.TimeRange.Latest,
					},
					TestMetadata:    tmd,
					Status:          "CONSISTENTLY_UNEXPECTED",
					FlakeStatistics: zeroFlakyStatistics(),
				},
				{
					TestId: tID5,
					TimeRange: &pb.TimeRange{
						Earliest: timestamppb.New(halfHAgo),
						Latest:   op.TimeRange.Latest,
					},
					TestMetadata: tmd,
					Status:       "FLAKY",
					FlakeStatistics: &atvpb.FlakeStatistics{
						FlakyVerdictRate:      1.0,
						FlakyVerdictCount:     1,
						TotalVerdictCount:     1,
						UnexpectedResultRate:  0.5,
						UnexpectedResultCount: 1,
						TotalResultCount:      2,
					},
					Verdicts: []*bqpb.Verdict{
						verdicts[tID5]["build-2"],
					},
				},
				{
					TestId: tID5,
					TimeRange: &pb.TimeRange{
						Earliest: timestamppb.New(m46Ago),
						Latest:   timestamppb.New(halfHAgo),
					},
					TestMetadata: tmd,
					Status:       "CONSISTENTLY_EXPECTED",
					FlakeStatistics: &atvpb.FlakeStatistics{
						FlakyVerdictRate:      0.0,
						FlakyVerdictCount:     0,
						TotalVerdictCount:     1,
						UnexpectedResultRate:  0.0,
						UnexpectedResultCount: 0,
						TotalResultCount:      1,
					},
					Verdicts: []*bqpb.Verdict{
						verdicts[tID5]["build-1"],
					},
				},
				{
					TestId: tID5,
					TimeRange: &pb.TimeRange{
						Earliest: op.TimeRange.Earliest,
						Latest:   timestamppb.New(m46Ago),
					},
					TestMetadata:    tmd,
					Status:          "FLAKY",
					FlakeStatistics: zeroFlakyStatistics(),
				},
				{
					TestId: tID2,
					TimeRange: &pb.TimeRange{
						Earliest: timestamppb.New(halfHAgo),
						Latest:   op.TimeRange.Latest,
					},
					TestMetadata: tmd,
					Status:       "FLAKY",
					FlakeStatistics: &atvpb.FlakeStatistics{
						FlakyVerdictRate:      1.0,
						FlakyVerdictCount:     1,
						TotalVerdictCount:     1,
						UnexpectedResultRate:  0.5,
						UnexpectedResultCount: 1,
						TotalResultCount:      2,
					},
					Verdicts: []*bqpb.Verdict{
						verdicts[tID2]["build-2"],
					},
				},
				{
					TestId: tID6,
					TimeRange: &pb.TimeRange{
						Earliest: op.TimeRange.Earliest,
						Latest:   op.TimeRange.Latest,
					},
					TestMetadata:    tmd,
					Status:          "CONSISTENTLY_EXPECTED",
					FlakeStatistics: zeroFlakyStatistics(),
				},
				{
					TestId: tID3,
					TimeRange: &pb.TimeRange{
						Earliest: op.TimeRange.Earliest,
						Latest:   op.TimeRange.Latest,
					},
					TestMetadata:    tmd,
					Status:          "FLAKY",
					FlakeStatistics: zeroFlakyStatistics(),
				},
				{
					TestId: tID7,
					TimeRange: &pb.TimeRange{
						Earliest: op.TimeRange.Earliest,
						Latest:   op.TimeRange.Latest,
					},
					TestMetadata: tmd,
					Status:       "FLAKY",
					FlakeStatistics: &atvpb.FlakeStatistics{
						FlakyVerdictRate:      0,
						FlakyVerdictCount:     0,
						TotalVerdictCount:     1,
						UnexpectedResultRate:  0.5,
						UnexpectedResultCount: 1,
						TotalResultCount:      2,
					},
					Verdicts: []*bqpb.Verdict{
						verdicts[tID7]["build-1"],
					},
				},
			}
			test(nil, expRows)
		})

		Convey(`status predicate`, func() {
			predicate := &atvpb.Predicate{
				Status: atvpb.Status_FLAKY,
			}

			expRows := []*bqpb.TestVariantRow{
				{
					TestId: tID2,
					TimeRange: &pb.TimeRange{
						Earliest: timestamppb.New(halfHAgo),
						Latest:   op.TimeRange.Latest,
					},
					TestMetadata: tmd,
					Status:       "FLAKY",
					FlakeStatistics: &atvpb.FlakeStatistics{
						FlakyVerdictRate:      1.0,
						FlakyVerdictCount:     1,
						TotalVerdictCount:     1,
						UnexpectedResultRate:  0.5,
						UnexpectedResultCount: 1,
						TotalResultCount:      2,
					},
					Verdicts: []*bqpb.Verdict{
						verdicts[tID2]["build-2"],
					},
				},
				{
					TestId: tID1,
					TimeRange: &pb.TimeRange{
						Earliest: op.TimeRange.Earliest,
						Latest:   op.TimeRange.Latest,
					},
					TestMetadata: tmd,
					Status:       "FLAKY",
					FlakeStatistics: &atvpb.FlakeStatistics{
						FlakyVerdictRate:      0.5,
						FlakyVerdictCount:     1,
						TotalVerdictCount:     2,
						UnexpectedResultRate:  float32(1) / 3,
						UnexpectedResultCount: 1,
						TotalResultCount:      3,
					},
					Verdicts: []*bqpb.Verdict{
						verdicts[tID1]["build-2"],
						verdicts[tID1]["build-1"],
					},
				},
				{
					TestId: tID3,
					TimeRange: &pb.TimeRange{
						Earliest: op.TimeRange.Earliest,
						Latest:   op.TimeRange.Latest,
					},
					TestMetadata:    tmd,
					Status:          "FLAKY",
					FlakeStatistics: zeroFlakyStatistics(),
				},
				{
					TestId: tID5,
					TimeRange: &pb.TimeRange{
						Earliest: timestamppb.New(halfHAgo),
						Latest:   op.TimeRange.Latest,
					},
					TestMetadata: tmd,
					Status:       "FLAKY",
					FlakeStatistics: &atvpb.FlakeStatistics{
						FlakyVerdictRate:      1.0,
						FlakyVerdictCount:     1,
						TotalVerdictCount:     1,
						UnexpectedResultRate:  0.5,
						UnexpectedResultCount: 1,
						TotalResultCount:      2,
					},
					Verdicts: []*bqpb.Verdict{
						verdicts[tID5]["build-2"],
					},
				},
				{
					TestId: tID5,
					TimeRange: &pb.TimeRange{
						Earliest: op.TimeRange.Earliest,
						Latest:   timestamppb.New(m46Ago),
					},
					TestMetadata:    tmd,
					Status:          "FLAKY",
					FlakeStatistics: zeroFlakyStatistics(),
				},
				{
					TestId: tID7,
					TimeRange: &pb.TimeRange{
						Earliest: op.TimeRange.Earliest,
						Latest:   op.TimeRange.Latest,
					},
					TestMetadata: tmd,
					Status:       "FLAKY",
					FlakeStatistics: &atvpb.FlakeStatistics{
						FlakyVerdictRate:      0,
						FlakyVerdictCount:     0,
						TotalVerdictCount:     1,
						UnexpectedResultRate:  0.5,
						UnexpectedResultCount: 1,
						TotalResultCount:      2,
					},
					Verdicts: []*bqpb.Verdict{
						verdicts[tID7]["build-1"],
					},
				},
			}

			test(predicate, expRows)
		})

		Convey(`testIdRegexp`, func() {
			predicate := &atvpb.Predicate{
				TestIdRegexp: tID1,
			}
			expRows := []*bqpb.TestVariantRow{
				{
					TestId: tID1,
					TimeRange: &pb.TimeRange{
						Earliest: op.TimeRange.Earliest,
						Latest:   op.TimeRange.Latest,
					},
					TestMetadata: tmd,
					Status:       "FLAKY",
					FlakeStatistics: &atvpb.FlakeStatistics{
						FlakyVerdictRate:      0.5,
						FlakyVerdictCount:     1,
						TotalVerdictCount:     2,
						UnexpectedResultRate:  float32(1) / 3,
						UnexpectedResultCount: 1,
						TotalResultCount:      3,
					},
					Verdicts: []*bqpb.Verdict{
						verdicts[tID1]["build-2"],
						verdicts[tID1]["build-1"],
					},
				},
			}

			test(predicate, expRows)
		})

		variantExpRows := []*bqpb.TestVariantRow{
			{
				TestId: tID6,
				TimeRange: &pb.TimeRange{
					Earliest: op.TimeRange.Earliest,
					Latest:   op.TimeRange.Latest,
				},
				TestMetadata:    tmd,
				Status:          "CONSISTENTLY_EXPECTED",
				FlakeStatistics: zeroFlakyStatistics(),
			},
		}
		Convey(`variantEqual`, func() {
			predicate := &atvpb.Predicate{
				Variant: &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Equals{
						Equals: pbutil.Variant("a", "b"),
					},
				},
			}
			test(predicate, variantExpRows)
		})

		Convey(`variantHashEqual`, func() {
			predicate := &atvpb.Predicate{
				Variant: &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_HashEquals{
						HashEquals: pbutil.VariantHash(pbutil.Variant("a", "b")),
					},
				},
			}
			test(predicate, variantExpRows)
		})

		Convey(`variantContain`, func() {
			predicate := &atvpb.Predicate{
				Variant: &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: pbutil.Variant("a", "b"),
					},
				},
			}
			test(predicate, variantExpRows)
		})
	})
}
