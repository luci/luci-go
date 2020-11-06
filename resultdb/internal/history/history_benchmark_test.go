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
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"
)

func insertBenchmarkData(ctx context.Context, nOses, nVariants, nBuilds, nSteps, nShards, nTests, nResults int) {
	needleSet := false
	start := testclock.TestRecentTimeUTC
	ms := make([]*spanner.Mutation, 0)
	for o := 0; o < nOses; o++ {
		for v := 0; v < nVariants; v++ {
			builder := fmt.Sprintf("OS%d_Variant%d", o, v)
			for build := 0; build < nBuilds; build++ {
				buildInvID := invocations.ID(fmt.Sprintf("%s-build%d", builder, build))
				buildTS := start.Add(time.Duration(o*v) * time.Minute) // + build hours + ov minutes
				ms = append(ms, insert.Invocation(buildInvID, pb.Invocation_ACTIVE, map[string]interface{}{"HistoryTime": buildTS, "Realm": "luci:benchmark"}))
				for step := 0; step < nSteps; step++ {
					stepInvID := invocations.ID(fmt.Sprintf("%s-build%d-step%d", builder, build, step))
					ms = append(ms, insert.Invocation(stepInvID, pb.Invocation_ACTIVE, nil))
					ms = append(ms, insert.Inclusion(buildInvID, stepInvID))
					for shard := 0; shard < nShards; shard++ {
						shardInvID := invocations.ID(fmt.Sprintf("%s-build%d-step%d-shard%d", builder, build, step, shard))
						ms = append(ms, insert.Invocation(shardInvID, pb.Invocation_ACTIVE, nil))
						ms = append(ms, insert.Inclusion(stepInvID, shardInvID))
						for test := 0; test < nTests; test++ {
							testID := fmt.Sprintf("step%d-shard%d-test%d", step, shard, test)
							if !needleSet {
								needleSet = true
								testID = "needle"
							}
							for result := 0; result < nResults; result++ {
								resultID := fmt.Sprint(result)
								ms = append(ms,
									spanutil.InsertMap("TestResults", map[string]interface{}{
										"InvocationId":    shardInvID,
										"TestId":          testID,
										"ResultId":        resultID,
										"Variant":         pbutil.Variant("os", fmt.Sprint(o), "variant", fmt.Sprint(v)),
										"VariantHash":     fmt.Sprintf("deadbeef%d%d", o, v),
										"CommitTimestamp": spanner.CommitTimestamp,
										"Status":          pb.TestStatus_PASS,
										"RunDurationUsec": 1534567,
									}))

							}
						}
					}
				}
			}
		}
	}
	maxSpannerBatch := 2000
	for len(ms) > maxSpannerBatch {
		span.Apply(ctx, ms[:maxSpannerBatch])
		ms = ms[maxSpannerBatch:]
	}
	span.Apply(ctx, ms)
}

func BenchmarkGetTestResultsHistory(b *testing.B) {

	past, _ := ptypes.TimestampProto(testclock.TestRecentTimeUTC)
	q := &Query{
		Request: &pb.GetTestResultHistoryRequest{
			Range: &pb.GetTestResultHistoryRequest_TimeRange{
				TimeRange: &pb.TimeRange{
					Earliest: past,
					Latest:   ptypes.TimestampNow(),
				},
			},
			Realm: "luci:benchmark",
		},
	}

	ctx := testutil.SpannerTestContext(b)
	insertBenchmarkData(ctx, 4, 4, 4, 4, 4, 4, 4) // 16k results
	b.ResetTimer()

	b.Run("AllBuildsAllTests", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			iCtx, cancel := span.ReadOnlyTransaction(ctx)
			q.Execute(iCtx)
			cancel()
		}
	})

	b.Run("SomeBuildsSomeTests", func(b *testing.B) {
		q.Request.TestIdRegexp = "step0-shard0.*"
		q.Request.VariantPredicate = &pb.VariantPredicate{
			Predicate: &pb.VariantPredicate_Contains{
				Contains: pbutil.Variant("os", "0", "variant", "0"),
			},
		}
		for i := 0; i < b.N; i++ {
			iCtx, cancel := span.ReadOnlyTransaction(ctx)
			q.Execute(iCtx)
			cancel()
		}
	})

	b.Run("NeedleInHaystack", func(b *testing.B) {
		q.Request.TestIdRegexp = "needle"
		q.Request.VariantPredicate = nil
		for i := 0; i < b.N; i++ {
			iCtx, cancel := span.ReadOnlyTransaction(ctx)
			q.Execute(iCtx)
			cancel()
		}
	})
}
