// Copyright 2020 The LUCI Authors.
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

package history

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/ptypes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

var useRedis = testutil.ConnectToRedis()

type benchmarkDataParams struct {
	nOSes, nVariants, nBuilds, nSteps, nShards, nTests, nResults int
}

func insertBenchmarkData(ctx context.Context, b *testing.B, p benchmarkDataParams) {
	start := testclock.TestRecentTimeUTC
	ms := make([]*spanner.Mutation, 0)
	for os := 0; os < p.nOSes; os++ {
		for v := 0; v < p.nVariants; v++ {
			builder := fmt.Sprintf("OS%d_Variant%d", os, v)
			for build := 0; build < p.nBuilds; build++ {
				buildInvID := invocations.ID(fmt.Sprintf("%s-build%d", builder, build))
				buildTS := start.Add(time.Duration(os*v) * time.Minute) // + build hours + ov minutes
				ms = append(ms, insert.Invocation(buildInvID, pb.Invocation_ACTIVE, map[string]interface{}{
					"HistoryTime": buildTS,
					"Realm":       "luci:benchmark",
				}))
				reachables := invocations.NewIDSet()
				for step := 0; step < p.nSteps; step++ {
					stepInvID := invocations.ID(fmt.Sprintf("%s-build%d-step%d", builder, build, step))
					ms = append(ms, insert.Invocation(stepInvID, pb.Invocation_ACTIVE, nil))
					ms = append(ms, insert.Inclusion(buildInvID, stepInvID))
					reachables.Add(stepInvID)
					for shard := 0; shard < p.nShards; shard++ {
						shardInvID := invocations.ID(fmt.Sprintf("%s-build%d-step%d-shard%d", builder, build, step, shard))
						ms = append(ms, insert.Invocation(shardInvID, pb.Invocation_ACTIVE, nil))
						ms = append(ms, insert.Inclusion(stepInvID, shardInvID))
						reachables.Add(shardInvID)
						for test := 0; test < p.nTests; test++ {
							testID := fmt.Sprintf("step%d-shard%d-test%d", step, shard, test)
							for result := 0; result < p.nResults; result++ {
								resultID := fmt.Sprint(result)
								ms = append(ms,
									spanutil.InsertMap("TestResults", map[string]interface{}{
										"InvocationId":    shardInvID,
										"TestId":          testID,
										"ResultId":        resultID,
										"Variant":         pbutil.Variant("os", fmt.Sprint(os), "variant", fmt.Sprint(v)),
										"VariantHash":     fmt.Sprintf("deadbeef%d%d", os, v),
										"CommitTimestamp": spanner.CommitTimestamp,
										"Status":          pb.TestStatus_PASS,
										"RunDurationUsec": 1534567,
									}))

							}
						}
					}
				}
				if useRedis {
					invocations.ReachCache(buildInvID).TryWrite(ctx, reachables)
				}

			}
		}
	}
	maxSpannerBatch := 2000
	for len(ms) > maxSpannerBatch {
		if _, err := span.Apply(ctx, ms[:maxSpannerBatch]); err != nil {
			b.Fatal(err)
		}
		ms = ms[maxSpannerBatch:]
	}

	if _, err := span.Apply(ctx, ms); err != nil {
		b.Fatal(err)
	}
}

func BenchmarkGetTestResultsHistory(b *testing.B) {
	runQueryBenchmark := func(ctx context.Context, b *testing.B, name string, q *Query, expectedResults int) {
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				iCtx, cancel := span.ReadOnlyTransaction(ctx)
				r, _, err := q.Execute(iCtx)
				if err != nil {
					b.Fatal(err)
				}
				if len(r) != expectedResults {
					b.Fatal(fmt.Errorf("Expected %d results but got %d", expectedResults, len(r)))
				}
				cancel()
			}
		})
	}

	past, _ := ptypes.TimestampProto(testclock.TestRecentTimeUTC)
	q := &Query{
		Request: &pb.GetTestResultHistoryRequest{
			Range: &pb.GetTestResultHistoryRequest_TimeRange{
				TimeRange: &pb.TimeRange{
					Earliest: past,
					Latest:   timestamppb.Now(),
				},
			},
			Realm: "luci:benchmark",
		},
		ignoreDeadline: true,
	}

	// 16 builders, each with 100 builds, where each build-level
	// invocation contains 20 sub-invocations (4 steps and 16 shards)
	// and 64 results (4 tests per shard, 1 result per test).
	p := benchmarkDataParams{4, 4, 100, 4, 4, 4, 1}
	ctx := testutil.SpannerTestContext(b)

	insertBenchmarkData(ctx, b, p)

	suffix := "-no-redis"
	if useRedis {
		suffix = "-local-redis"
	}

	q.Request.TestIdRegexp = "step0-shard0-test0"
	runQueryBenchmark(ctx, b, fmt.Sprintf("SpecificTest%s", suffix), q, 100)

	q.Request.VariantPredicate = &pb.VariantPredicate{
		Predicate: &pb.VariantPredicate_Contains{
			Contains: pbutil.Variant("os", "0", "variant", "0"),
		},
	}
	runQueryBenchmark(ctx, b, fmt.Sprintf("SpecificTestVariant%s", suffix), q, 100)
}
