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

package changepoints

import (
	"testing"
	"time"

	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	cpb "go.chromium.org/luci/analysis/internal/changepoints/proto"
	tu "go.chromium.org/luci/analysis/internal/changepoints/testutil"
	"go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
	clusteringpb "go.chromium.org/luci/analysis/internal/clustering/proto"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestQueryForClustering(t *testing.T) {
	Convey(`With analysis available`, t, func() {
		ctx := newContext(t)
		sourcesMap := tu.SampleSourcesMap(10)

		ref := pbutil.SourceRefFromSources(sourcesMap["sources_id"])
		tvb := &testvariantbranch.Entry{
			IsNew:       true,
			Project:     "chromium",
			TestID:      "test_1",
			VariantHash: "hash_1",
			SourceRef:   ref,
			RefHash:     pbutil.SourceRefHash(ref),
			Variant: &pb.Variant{
				Def: map[string]string{
					"k": "v",
				},
			},
			InputBuffer: &inputbuffer.Buffer{
				HotBuffer: inputbuffer.History{
					Verdicts: []inputbuffer.PositionVerdict{
						unexpectedVerdictAtHour(99),
						unexpectedVerdictAtHour(100),
					},
				},
				ColdBuffer: inputbuffer.History{
					Verdicts: []inputbuffer.PositionVerdict{
						flakyVerdictAtHour(122),
						unexpectedVerdictAtHour(123),
						unexpectedVerdictAtHour(124),
					},
				},
			},
			Statistics: &cpb.Statistics{
				HourlyBuckets: []*cpb.Statistics_HourBucket{
					{
						Hour:          99,
						FlakyVerdicts: 1,
						TotalVerdicts: 1,
					},
					{
						Hour:          100,
						FlakyVerdicts: 2,
						TotalVerdicts: 3,
					},
					{
						Hour:          123,
						FlakyVerdicts: 4,
						TotalVerdicts: 5,
					},
					{
						Hour:          124,
						FlakyVerdicts: 8,
						TotalVerdicts: 8,
					},
				},
			},
		}
		var hs inputbuffer.HistorySerializer
		mutation, err := tvb.ToMutation(&hs)
		So(err, ShouldBeNil)
		testutil.MustApply(ctx, mutation)

		tvs := []*rdbpb.TestVariant{
			{
				TestId:      "test_1",
				VariantHash: "hash_1",
				SourcesId:   "sources_id",
			},
		}
		partitionTime := time.Unix(123*3600+1000, 0)

		expectedResult := &clusteringpb.TestVariantBranch{
			FlakyVerdicts_24H:      7,  // 2 + 4 (from stored statistics) + 1 (cold buffer).
			UnexpectedVerdicts_24H: 2,  // 1 (hot buffer) + 1 (cold buffer).
			TotalVerdicts_24H:      11, // 3 + 5 (from stored statistics) + 2 (cold buffer) + 1 (hot buffer).
		}

		Convey(`Query single test verdict`, func() {
			result, err := QueryStatsForClustering(ctx, tvs, "chromium", partitionTime, sourcesMap)
			So(err, ShouldBeNil)

			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldResembleProto, expectedResult)
		})
		Convey(`Query multiple test verdicts, some without sources`, func() {
			tvs = []*rdbpb.TestVariant{
				{
					TestId:      "test_0",
					VariantHash: "hash_0",
				},
				{
					TestId:      "test_1",
					VariantHash: "hash_1",
					SourcesId:   "sources_id",
				},
				{
					TestId:      "test_2",
					VariantHash: "hash_2",
				},
			}

			result, err := QueryStatsForClustering(ctx, tvs, "chromium", partitionTime, sourcesMap)
			So(err, ShouldBeNil)

			So(result, ShouldHaveLength, 3)
			So(result[0], ShouldBeNil)
			So(result[1], ShouldResembleProto, expectedResult)
			So(result[2], ShouldBeNil)
		})
		Convey(`Query multiple test verdicts, some without stored analysis available`, func() {
			tvs := testVariants(3)
			result, err := QueryStatsForClustering(ctx, tvs, "chromium", partitionTime, sourcesMap)
			So(err, ShouldBeNil)
			So(result, ShouldHaveLength, 3)
			for i, item := range result {
				if i == 1 {
					So(item, ShouldResembleProto, expectedResult)
				} else {
					// If we had sources and therefore could lookup analysis,
					// but found no previous verdicts ingested, zero counts
					// should be returned.
					So(item, ShouldResembleProto, &clusteringpb.TestVariantBranch{})
				}
			}
		})
		Convey(`Query large number of test verdicts`, func() {
			// Up to 10,000 test variant branches may be queried at once.
			tvs := testVariants(10000)
			result, err := QueryStatsForClustering(ctx, tvs, "chromium", partitionTime, sourcesMap)
			So(err, ShouldBeNil)
			So(result, ShouldHaveLength, 10000)
			for i, item := range result {
				if i == 1 {
					So(item, ShouldResembleProto, expectedResult)
				} else {
					So(item, ShouldResembleProto, &clusteringpb.TestVariantBranch{})
				}
			}
		})
	})
	Convey(`No verdicts have sources`, t, func() {
		ctx := newContext(t)
		tvs := []*rdbpb.TestVariant{
			{
				TestId:      "test_0",
				VariantHash: "hash_0",
			},
		}

		result, err := QueryStatsForClustering(ctx, tvs, "chromium", time.Unix(10*3600, 0), nil)
		So(err, ShouldBeNil)
		So(result, ShouldHaveLength, 1)
		So(result[0], ShouldBeNil)
	})
	Convey(`Empty request`, t, func() {
		ctx := newContext(t)
		tvs := []*rdbpb.TestVariant{}
		result, err := QueryStatsForClustering(ctx, tvs, "chromium", time.Unix(10*3600, 0), nil)
		So(err, ShouldBeNil)
		So(result, ShouldHaveLength, 0)
	})
}

func unexpectedVerdictAtHour(hour int) inputbuffer.PositionVerdict {
	return inputbuffer.PositionVerdict{
		CommitPosition: 1,
		Hour:           time.Unix(int64(hour)*3600, 0),
		Details: inputbuffer.VerdictDetails{
			Runs: []inputbuffer.Run{
				{
					Unexpected: inputbuffer.ResultCounts{
						FailCount: 1,
					},
				},
			},
		},
	}
}

func flakyVerdictAtHour(hour int) inputbuffer.PositionVerdict {
	return inputbuffer.PositionVerdict{
		CommitPosition: 1,
		Hour:           time.Unix(int64(hour)*3600, 0),
		Details: inputbuffer.VerdictDetails{
			Runs: []inputbuffer.Run{
				{
					Unexpected: inputbuffer.ResultCounts{
						FailCount: 1,
					},
					Expected: inputbuffer.ResultCounts{
						PassCount: 1,
					},
				},
			},
		},
	}
}
