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

package purger

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	durpb "google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
)

// makeTestResultsWithVariants creates test results with a number of passing/failing variants.
//
// There'll be two rows for each variant. If the variant is a passing variant, both results
// will have a passing status, otherwise the first will be passing, and the second faled.
func makeTestResultsWithVariants(invID, testID string, nPassingVariants, nFailedVariants int) []*pb.TestResult {
	nVariants := nPassingVariants + nFailedVariants
	// For every variant we'll generate two rows.
	statuses := []pb.TestStatus{pb.TestStatus_PASS, pb.TestStatus_PASS}
	trs := make([]*pb.TestResult, nVariants*len(statuses))
	for v := 0; v < nVariants; v++ {
		// We'll generate all passing variants first, so we only have to change the
		// list of statuses once.
		if v == nPassingVariants {
			statuses[len(statuses)-1] = pb.TestStatus_FAIL
		}
		for s, status := range statuses {
			rIndex := v*len(statuses) + s
			resultID := fmt.Sprintf("%d", rIndex)
			trs[rIndex] = &pb.TestResult{
				Name:     pbutil.TestResultName(invID, testID, resultID),
				TestId:   testID,
				ResultId: resultID,
				Variant:  pbutil.Variant("k1", "v1", "k2", fmt.Sprintf("v%d", v)),
				Expected: status == pb.TestStatus_PASS,
				Status:   status,
				Duration: &durpb.Duration{Seconds: int64(rIndex), Nanos: 234567000},
			}
		}
	}
	return trs
}

func insertInvocation(ctx context.Context, invID invocations.ID, nTests, nPassingVariants, nFailedVariants, nArtifactsPerResult int) invocations.ID {
	now := clock.Now(ctx).UTC()

	// Insert an invocation,
	testutil.MustApply(ctx, insert.Invocation(invID, pb.Invocation_FINALIZED, map[string]any{
		"ExpectedTestResultsExpirationTime": now.Add(-time.Minute),
		"CreateTime":                        now.Add(-time.Hour),
		"FinalizeTime":                      now.Add(-time.Hour),
	}))

	// Insert test results and artifacts.
	inserts := []*spanner.Mutation{}
	for i := 0; i < nTests; i++ {
		results := makeTestResultsWithVariants(string(invID), fmt.Sprintf("Test%d", i), nPassingVariants, nFailedVariants)
		inserts = append(inserts, insert.TestResultMessages(results)...)
		for _, res := range results {
			for j := 0; j < nArtifactsPerResult; j++ {
				inserts = append(inserts, spanutil.InsertMap("Artifacts", map[string]any{
					"InvocationId": invID,
					"ParentId":     artifacts.ParentID(res.TestId, res.ResultId),
					"ArtifactId":   strconv.Itoa(j),
				}))
			}
		}
	}
	testutil.MustApply(ctx, testutil.CombineMutations(inserts)...)
	return invID
}

func countRows(ctx context.Context, invID invocations.ID) (testResults, artifacts int64) {
	st := spanner.NewStatement(`
		SELECT
			(SELECT COUNT(*) FROM TestResults WHERE InvocationId = @invocationId),
			(SELECT COUNT(*) FROM Artifacts WHERE InvocationId = @invocationId),
		`)
	st.Params["invocationId"] = spanutil.ToSpanner(invID)
	So(spanutil.QueryFirstRow(span.Single(ctx), st, &testResults, &artifacts), ShouldBeNil)
	return
}

func TestPurgeExpiredResults(t *testing.T) {
	Convey(`TestPurgeExpiredResults`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		Convey(`Some results are unexpected`, func() {
			inv := insertInvocation(ctx, "inv-some-unexpected", 10, 9, 1, 0)

			err := purgeOneShard(ctx, 0)
			So(err, ShouldBeNil)

			// 10 tests * 1 variant with unexpected results * 2 results per test variant.
			testResults, _ := countRows(ctx, inv)
			So(testResults, ShouldEqual, 20)
		})

		Convey(`No unexpected results`, func() {
			inv := insertInvocation(ctx, "inv-no-unexpected", 10, 10, 0, 0)

			err := purgeOneShard(ctx, 0)
			So(err, ShouldBeNil)

			// 10 tests * 0 variants with unexpected results * 2 results per test variant.
			testResults, _ := countRows(ctx, inv)
			So(testResults, ShouldEqual, 0)
		})

		Convey(`Purge artifacts`, func() {
			inv := insertInvocation(ctx, "inv", 10, 9, 1, 10)

			err := purgeOneShard(ctx, 0)
			So(err, ShouldBeNil)

			// 10 tests * 1 variant with unexpected results * 2 results per test variant * 10 artifacts..
			_, artifacts := countRows(ctx, inv)
			So(artifacts, ShouldEqual, 200)
		})
	})
}
