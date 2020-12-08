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

package testvariants

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	uipb "go.chromium.org/luci/resultdb/internal/proto/ui"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestQueryTestVariants(t *testing.T) {
	Convey(`QueryTestVariants`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		q := &Query{
			InvocationIDs: invocations.NewIDSet("inv0", "inv1"),
			PageSize:      100,
		}

		fetch := func(q *Query) (tvs []*uipb.TestVariant, token string, err error) {
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			return q.Fetch(ctx)
		}

		mustFetch := func(q *Query) (tvs []*uipb.TestVariant, token string) {
			tvs, token, err := fetch(q)
			So(err, ShouldBeNil)
			return
		}

		getTVStrings := func(tvs []*uipb.TestVariant) []string {
			tvStrings := make([]string, len(tvs))
			for i, tv := range tvs {
				tvStrings[i] = fmt.Sprintf("%d/%s/%s", int32(tv.Status), tv.TestId, tv.VariantHash)
			}
			sort.Strings(tvStrings)
			return tvStrings
		}

		Convey(`Query`, func() {
			testutil.MustApply(ctx, insert.Invocation("inv0", pb.Invocation_ACTIVE, nil))
			testutil.MustApply(ctx, insert.Invocation("inv1", pb.Invocation_ACTIVE, nil))
			Convey(`unexpected`, func() {
				testutil.MustApply(ctx, testutil.CombineMutations(
					insert.TestResults("inv0", "T1", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL),
					insert.TestResults("inv0", "T2", nil, pb.TestStatus_PASS),
					insert.TestResults("inv0", "T5", nil, pb.TestStatus_FAIL),
					insert.TestResults("inv1", "T1", nil, pb.TestStatus_PASS),
					insert.TestResults("inv1", "T2", nil, pb.TestStatus_FAIL),
					insert.TestResults("inv1", "T3", nil, pb.TestStatus_PASS),
					insert.TestResults("inv1", "T5", pbutil.Variant("a", "b"), pb.TestStatus_FAIL, pb.TestStatus_PASS),

					insert.TestExonerations("inv0", "T1", nil, 1),
				)...)

				// Insert an additional TestResult for comparing TestVariant.Results.Result.
				startTime := timestamppb.New(testclock.TestRecentTimeUTC.Add(-2 * time.Minute))
				duration := &durationpb.Duration{Seconds: 0, Nanos: 234567000}
				strPairs := pbutil.StringPairs(
					"buildername", "blder",
					"test_suite", "foo_unittests",
					"test_id_prefix", "ninja://tests:tests/")

				testutil.MustApply(ctx,
					spanutil.InsertMap("TestResults", map[string]interface{}{
						"InvocationId":    invocations.ID("inv1"),
						"TestId":          "T4",
						"ResultId":        "0",
						"Variant":         pbutil.Variant("a", "b"),
						"VariantHash":     pbutil.VariantHash(pbutil.Variant("a", "b")),
						"CommitTimestamp": spanner.CommitTimestamp,
						"IsUnexpected":    true,
						"Status":          pb.TestStatus_FAIL,
						"RunDurationUsec": pbutil.MustDuration(duration).Microseconds(),
						"StartTime":       startTime,
						"SummaryHtml":     spanutil.Compressed("SummaryHtml"),
						"Tags":            pbutil.StringPairsToStrings(strPairs...),
					}),
				)

				Convey(`Works`, func() {
					tvs, _ := mustFetch(q)
					tvStrings := getTVStrings(tvs)
					So(tvStrings, ShouldResemble, []string{
						"1/T4/c467ccce5a16dc72",
						"1/T5/e3b0c44298fc1c14",
						"2/T2/e3b0c44298fc1c14",
						"2/T5/c467ccce5a16dc72",
						"3/T1/e3b0c44298fc1c14",
					})

					So(tvs[0].Results, ShouldResemble, []*uipb.TestResultBundle{
						&uipb.TestResultBundle{
							Result: &pb.TestResult{
								Name:        "invocations/inv1/tests/T4/results/0",
								ResultId:    "0",
								Expected:    false,
								Status:      pb.TestStatus_FAIL,
								StartTime:   startTime,
								Duration:    duration,
								SummaryHtml: "SummaryHtml",
								Tags:        strPairs,
							},
						},
					})
					So(tvs[4].Exonerations[0], ShouldResemble, &pb.TestExoneration{
						ExplanationHtml: "explanation 0",
					})
				})
			})
		})

	})
}
