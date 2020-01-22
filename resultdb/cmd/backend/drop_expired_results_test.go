// Copyright 2019 The LUCI Authors.
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

package main

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

func insertInvocationWithTestResults(ctx context.Context, invName string, nTests, nPassingVariants, nFailedVariants int) span.InvocationID {
	now := clock.Now(ctx)

	// Insert an invocation,
	invID := span.InvocationID(invName)
	extraArgs := map[string]interface{}{"ExpectedTestResultsExpirationTime": now.Add(-time.Minute)}
	testutil.MustApply(ctx, testutil.InsertInvocation(invID, pb.Invocation_FINALIZED, now.Add(-time.Hour), extraArgs))

	for i := 0; i < nTests; i++ {
		results := makeTestResultsWithVariants(string(invID), fmt.Sprintf("Test%d", i), nPassingVariants, nFailedVariants)
		inserts := testutil.InsertTestResults(results)
		testutil.MustApply(ctx, testutil.CombineMutations(inserts)...)
	}
	return invID
}

func deleteInvocation(ctx context.Context, invID span.InvocationID) error {
	st := spanner.NewStatement(`
		DELETE FROM Invocations
		WHERE InvocationId = @invocationId`)
	st.Params["invocationId"] = span.ToSpanner(invID)
	_, err := span.Client(ctx).PartitionedUpdate(ctx, st)
	return err
}

func countTestResults(ctx context.Context, invID span.InvocationID) (int64, error) {
	st := spanner.NewStatement(`
		SELECT COUNT(*) FROM TestResults
		WHERE InvocationId = @invocationId`)
	st.Params["invocationId"] = span.ToSpanner(invID)
	var res int64
	row, err := span.Client(ctx).Single().Query(ctx, st).Next()
	if err == nil {
		err = row.Columns(&res)
	}
	return res, err
}

func TestDropExpiredResults(t *testing.T) {
	Convey(`TestDropExpiredResults`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		// Insert three invocations with their data.
		invocations := []span.InvocationID{
			insertInvocationWithTestResults(ctx, "inv-some-unexpected", 10, 9, 1),
			insertInvocationWithTestResults(ctx, "inv-no-unexpected", 10, 10, 0),
			insertInvocationWithTestResults(ctx, "inv-too-many-unexpected", 100, 1, 99)}

		// Sample the table (insertInvocation hardcodes the shard to 0).
		expiredResultsInvocationIds, err := sampleExpiredResultsInvocations(ctx, clock.Now(ctx), 0, 100)
		So(err, ShouldBeNil)
		So(len(expiredResultsInvocationIds), ShouldEqual, 3)

		// Purge expired data.
		So(dispatchExpiredResultDeletionTasks(ctx, expiredResultsInvocationIds), ShouldBeNil)

		// Count remaining results
		n, err := countTestResults(ctx, invocations[0])
		So(err, ShouldBeNil)
		So(n, ShouldEqual, 20) // 10 tests * 1 variant with unexpected results * 2 results per test variant

		n, err = countTestResults(ctx, invocations[1])
		So(err, ShouldBeNil)
		So(n, ShouldEqual, 0) // 10 tests * 0 variants with unexpected results * 2 results per test variant

		n, err = countTestResults(ctx, invocations[2])
		So(err, ShouldBeNil)
		// Too many test variants with unexpected results (9900), so no deletions happen.
		So(n, ShouldEqual, 20000) // 100 tests * 100 variants * 2 results per test variants

		// Delete invocations, test results should be deleted by cascade.
		for _, i := range invocations {
			So(deleteInvocation(ctx, i), ShouldBeNil)
		}
	})
}
