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

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	cpb "go.chromium.org/luci/analysis/internal/changepoints/proto"
	tu "go.chromium.org/luci/analysis/internal/changepoints/testutil"
	"go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
	clusteringpb "go.chromium.org/luci/analysis/internal/clustering/proto"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestQueryForClustering(t *testing.T) {
	ftt.Run(`With analysis available`, t, func(t *ftt.Test) {
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
			// Final source verdicts:
			// - At hour 125: 1 flaky verdict (from commit position 10,
			//   merging unexpected run in the input buffer with the passing
			//   partial source verdict in the output buffer)
			// - At hour 124: 1 unexpected verdict (input buffer), plus
			//   8 flaky verdicts (output buffer).
			// - At hour 123: 1 unexpected verdict (input buffer), plus
			//   4 flaky verdicts (output buffer).
			// - At hour 122: 1 flaky verdict (input buffer).
			// - At hour 100: 2 flaky verdicts (output buffer), 1 unexpected
			//   verdict (output buffer), 1 unexpected verdict (input buffer).
			// - At hour 99: 1 flaky verdict (output buffer).
			InputBuffer: &inputbuffer.Buffer{
				HotBuffer: inputbuffer.History{
					Runs: []inputbuffer.Run{
						unexpectedRunAtPositionAndHour(10, 125),
						unexpectedRunAtPositionAndHour(12, 100),
					},
				},
				ColdBuffer: inputbuffer.History{
					Runs: []inputbuffer.Run{
						flakyRunAtPositionAndHour(13, 122),
						unexpectedRunAtPositionAndHour(14, 123),
						unexpectedRunAtPositionAndHour(15, 124),
					},
				},
			},
			Statistics: &cpb.Statistics{
				PartialSourceVerdict: &cpb.PartialSourceVerdict{
					// Should merge with unexpectedRunAtPositionAndHour(1, 99) to produce
					// a flaky verdict at hour 125.
					CommitPosition:  10,
					LastHour:        timestamppb.New(time.Unix(int64(99)*3600, 0)),
					ExpectedResults: 1,
				},
				HourlyBuckets: []*cpb.Statistics_HourBucket{
					{
						Hour:                99,
						FlakySourceVerdicts: 1,
						TotalSourceVerdicts: 1,
					},
					{
						Hour:                100,
						FlakySourceVerdicts: 2,
						TotalSourceVerdicts: 3,
					},
					{
						Hour:                123,
						FlakySourceVerdicts: 4,
						TotalSourceVerdicts: 5,
					},
					{
						Hour:                124,
						FlakySourceVerdicts: 8,
						TotalSourceVerdicts: 8,
					},
				},
			},
		}
		var hs inputbuffer.HistorySerializer
		mutation, err := tvb.ToMutation(&hs)
		assert.Loosely(t, err, should.BeNil)
		testutil.MustApply(ctx, t, mutation)

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

		t.Run(`Query single test verdict`, func(t *ftt.Test) {
			result, err := QueryStatsForClustering(ctx, tvs, "chromium", partitionTime, sourcesMap)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, result, should.HaveLength(1))
			assert.Loosely(t, result[0], should.Match(expectedResult))
		})
		t.Run(`Query multiple test verdicts, some without sources`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, result, should.HaveLength(3))
			assert.Loosely(t, result[0], should.BeNil)
			assert.Loosely(t, result[1], should.Match(expectedResult))
			assert.Loosely(t, result[2], should.BeNil)
		})
		t.Run(`Query multiple test verdicts, some without stored analysis available`, func(t *ftt.Test) {
			tvs := testVariants(3)
			result, err := QueryStatsForClustering(ctx, tvs, "chromium", partitionTime, sourcesMap)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.HaveLength(3))
			for i, item := range result {
				if i == 1 {
					assert.Loosely(t, item, should.Match(expectedResult))
				} else {
					// If we had sources and therefore could lookup analysis,
					// but found no previous verdicts ingested, zero counts
					// should be returned.
					assert.Loosely(t, item, should.Match(&clusteringpb.TestVariantBranch{}))
				}
			}
		})
		t.Run(`Query large number of test verdicts`, func(t *ftt.Test) {
			// Up to 10,000 test variant branches may be queried at once.
			tvs := testVariants(10000)
			result, err := QueryStatsForClustering(ctx, tvs, "chromium", partitionTime, sourcesMap)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.HaveLength(10000))
			for i, item := range result {
				if i == 1 {
					assert.Loosely(t, item, should.Match(expectedResult))
				} else {
					assert.Loosely(t, item, should.Match(&clusteringpb.TestVariantBranch{}))
				}
			}
		})
	})
	ftt.Run(`No verdicts have sources`, t, func(t *ftt.Test) {
		ctx := newContext(t)
		tvs := []*rdbpb.TestVariant{
			{
				TestId:      "test_0",
				VariantHash: "hash_0",
			},
		}

		result, err := QueryStatsForClustering(ctx, tvs, "chromium", time.Unix(10*3600, 0), nil)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, result, should.HaveLength(1))
		assert.Loosely(t, result[0], should.BeNil)
	})
	ftt.Run(`Empty request`, t, func(t *ftt.Test) {
		ctx := newContext(t)
		tvs := []*rdbpb.TestVariant{}
		result, err := QueryStatsForClustering(ctx, tvs, "chromium", time.Unix(10*3600, 0), nil)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, result, should.HaveLength(0))
	})
}

func unexpectedRunAtPositionAndHour(position, hour int) inputbuffer.Run {
	return inputbuffer.Run{
		CommitPosition: int64(position),
		Hour:           time.Unix(int64(hour)*3600, 0),
		Unexpected: inputbuffer.ResultCounts{
			FailCount: 1,
		},
	}
}

func flakyRunAtPositionAndHour(position, hour int) inputbuffer.Run {
	return inputbuffer.Run{
		CommitPosition: int64(position),
		Hour:           time.Unix(int64(hour)*3600, 0),
		Unexpected: inputbuffer.ResultCounts{
			FailCount: 1,
		},
		Expected: inputbuffer.ResultCounts{
			PassCount: 1,
		},
	}
}
