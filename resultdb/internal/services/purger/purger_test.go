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
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	durpb "github.com/golang/protobuf/ptypes/duration"

	"go.chromium.org/luci/common/clock"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

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

func insertInvocationWithTestResults(ctx context.Context, invID span.InvocationID, nTests, nPassingVariants, nFailedVariants int) span.InvocationID {
	now := clock.Now(ctx).UTC()

	// Insert an invocation,
	testutil.MustApply(ctx, testutil.InsertInvocation(invID, pb.Invocation_FINALIZED, map[string]interface{}{
		"ExpectedTestResultsExpirationTime": now.Add(-time.Minute),
		"CreateTime":                        now.Add(-time.Hour),
		"FinalizeTime":                      now.Add(-time.Hour),
	}))

	inserts := []*spanner.Mutation{}
	for i := 0; i < nTests; i++ {
		results := makeTestResultsWithVariants(string(invID), fmt.Sprintf("Test%d", i), nPassingVariants, nFailedVariants)
		inserts = append(inserts, testutil.InsertTestResults(results)...)
	}
	testutil.MustApply(ctx, testutil.CombineMutations(inserts)...)
	return invID
}

func countTestResults(ctx context.Context, invID span.InvocationID) int64 {
	st := spanner.NewStatement(`
		SELECT COUNT(*) FROM TestResults
		WHERE InvocationId = @invocationId`)
	st.Params["invocationId"] = span.ToSpanner(invID)
	var res int64
	So(span.QueryFirstRow(ctx, span.Client(ctx).Single(), st, &res), ShouldBeNil)
	return res
}

func TestPurgeExpiredResults(t *testing.T) {
	Convey(`TestPurgeExpiredResults`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		// Insert three invocations with their data.
		invocations := []span.InvocationID{
			insertInvocationWithTestResults(ctx, "inv-some-unexpected", 10, 9, 1),
			insertInvocationWithTestResults(ctx, "inv-no-unexpected", 10, 10, 0),
		}

		// Purge expired data.
		err := scheduleForPurgingOneShard(ctx, 0)
		So(err, ShouldBeNil)
		err = deleteTestResults(ctx)
		So(err, ShouldBeNil)

		// Count remaining results
		// 10 tests * 1 variant with unexpected results * 2 results per test variant
		So(countTestResults(ctx, invocations[0]), ShouldEqual, 20)

		// 10 tests * 0 variants with unexpected results * 2 results per test variant
		So(countTestResults(ctx, invocations[1]), ShouldEqual, 0)
	})
}
