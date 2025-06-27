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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
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
	for v := range nVariants {
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

func insertInvocation(ctx context.Context, t testing.TB, invID invocations.ID, nTests, nPassingVariants, nFailedVariants, nArtifactsPerResult int) invocations.ID {
	t.Helper()
	now := clock.Now(ctx).UTC()

	// Insert an invocation,
	testutil.MustApply(ctx, t, insert.Invocation(invID, pb.Invocation_FINALIZED, map[string]any{
		"ExpectedTestResultsExpirationTime": now.Add(-time.Minute),
		"CreateTime":                        now.Add(-time.Hour),
		"FinalizeTime":                      now.Add(-time.Hour),
	}))

	// Insert test results and artifacts.
	inserts := []*spanner.Mutation{}
	for i := range nTests {
		results := makeTestResultsWithVariants(string(invID), fmt.Sprintf("Test%d", i), nPassingVariants, nFailedVariants)
		inserts = append(inserts, insert.TestResultMessages(t, results)...)
		for _, res := range results {
			for j := range nArtifactsPerResult {
				inserts = append(inserts, spanutil.InsertMap("Artifacts", map[string]any{
					"InvocationId": invID,
					"ParentId":     artifacts.ParentID(res.TestId, res.ResultId),
					"ArtifactId":   strconv.Itoa(j),
				}))
			}
		}
	}
	testutil.MustApply(ctx, t, testutil.CombineMutations(inserts)...)
	return invID
}

func countRows(ctx context.Context, t testing.TB, invID invocations.ID) (testResults, artifacts int64) {
	st := spanner.NewStatement(`
		SELECT
			(SELECT COUNT(*) FROM TestResults WHERE InvocationId = @invocationId),
			(SELECT COUNT(*) FROM Artifacts WHERE InvocationId = @invocationId),
		`)
	st.Params["invocationId"] = spanutil.ToSpanner(invID)
	assert.Loosely(t, spanutil.QueryFirstRow(span.Single(ctx), st, &testResults, &artifacts), should.BeNil, truth.LineContext())
	return
}

func TestPurgeExpiredResults(t *testing.T) {
	ftt.Run(`TestPurgeExpiredResults`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		t.Run(`Some results are unexpected`, func(t *ftt.Test) {
			inv := insertInvocation(ctx, t, "inv-some-unexpected", 10, 9, 1, 0)

			err := purgeOneShard(ctx, 0)
			assert.Loosely(t, err, should.BeNil)

			// 10 tests * 1 variant with unexpected results * 2 results per test variant.
			testResults, _ := countRows(ctx, t, inv)
			assert.Loosely(t, testResults, should.Equal(20))
		})

		t.Run(`No unexpected results`, func(t *ftt.Test) {
			inv := insertInvocation(ctx, t, "inv-no-unexpected", 10, 10, 0, 0)

			err := purgeOneShard(ctx, 0)
			assert.Loosely(t, err, should.BeNil)

			// 10 tests * 0 variants with unexpected results * 2 results per test variant.
			testResults, _ := countRows(ctx, t, inv)
			assert.Loosely(t, testResults, should.BeZero)
		})

		t.Run(`Purge artifacts`, func(t *ftt.Test) {
			inv := insertInvocation(ctx, t, "inv", 10, 9, 1, 10)

			err := purgeOneShard(ctx, 0)
			assert.Loosely(t, err, should.BeNil)

			// 10 tests * 1 variant with unexpected results * 2 results per test variant * 10 artifacts..
			_, artifacts := countRows(ctx, t, inv)
			assert.Loosely(t, artifacts, should.Equal(200))
		})
	})
}
