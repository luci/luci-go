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

package baselineupdater

import (
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/baselines"
	btv "go.chromium.org/luci/resultdb/internal/baselines/testvariants"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestShouldMarkSubmitted(t *testing.T) {
	ftt.Run(`ShouldMarkSubmitted`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		t.Run(`FinalizedInvocation`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_FINALIZED, map[string]any{
					"BaselineId": "try:linux-rel",
				}),
			)...)

			inv, err := invocations.Read(span.Single(ctx), "a")
			assert.Loosely(t, err, should.BeNil)

			err = shouldMarkSubmitted(inv)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`Not Finalized Invocation`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_ACTIVE, map[string]any{
					"BaselineId": "try:linux-rel",
				}),
			)...)

			inv, err := invocations.Read(span.Single(ctx), "a")
			assert.Loosely(t, err, should.BeNil)

			err = shouldMarkSubmitted(inv)
			assert.Loosely(t, err, should.ErrLike(`the invocation is not yet finalized`))
		})

		t.Run(`Non existent invocation`, func(t *ftt.Test) {
			_, err := invocations.Read(span.Single(ctx), "a")
			assert.Loosely(t, err, should.ErrLike(`invocations/a not found`))
		})
	})
}

func TestEnsureBaselineExists(t *testing.T) {
	ftt.Run(`EnsureBaselineExists`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		t.Run(`New Baseline`, func(t *ftt.Test) {
			inv := &pb.Invocation{
				Name:       "invocations/a",
				State:      pb.Invocation_FINALIZED,
				Realm:      "testproject:testrealm",
				BaselineId: "testrealm:linux-rel",
			}

			err := ensureBaselineExists(ctx, inv)
			assert.Loosely(t, err, should.BeNil)

			baseline, err := baselines.Read(span.Single(ctx), "testproject", "testrealm:linux-rel")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, baseline.Project, should.Equal("testproject"))
			assert.Loosely(t, baseline.BaselineID, should.Equal("testrealm:linux-rel"))
		})

		t.Run(`Existing baseline updated`, func(t *ftt.Test) {
			var twoHoursAgo = time.Now().UTC().Add(-time.Hour * 2)
			row := map[string]any{
				"Project":         "testproject",
				"BaselineId":      "try:linux-rel",
				"LastUpdatedTime": twoHoursAgo,
				"CreationTime":    twoHoursAgo,
			}

			testutil.MustApply(ctx, t,
				spanutil.InsertMap("Baselines", row),
			)

			baseline, err := baselines.Read(span.Single(ctx), "testproject", "try:linux-rel")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, baseline.LastUpdatedTime, should.Match(twoHoursAgo))

			inv := &pb.Invocation{
				Name:       "invocations/a",
				State:      pb.Invocation_FINALIZED,
				Realm:      "testproject:testrealm",
				BaselineId: "try:linux-rel",
			}

			// ensure baseline exists should update existing invocations'
			// last updated time to now.
			err = ensureBaselineExists(ctx, inv)
			assert.Loosely(t, err, should.BeNil)

			// re-read the baseline after ensuring it exists. the timestanp should
			// be different from the original timestamp it was written into.
			baseline, err = baselines.Read(span.Single(ctx), "testproject", "try:linux-rel")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, baseline.LastUpdatedTime, should.NotMatch(twoHoursAgo))
		})
	})
}

func TestTryMarkInvocationSubmitted(t *testing.T) {

	ftt.Run(`e2e`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		testutil.MustApply(ctx, t, testutil.CombineMutations(
			insert.InvocationWithInclusions("inv1", pb.Invocation_FINALIZED, map[string]any{"BaselineId": "try:linux-rel"}, "inv2"),
			insert.InvocationWithInclusions("inv2", pb.Invocation_FINALIZED, map[string]any{"BaselineId": "try:linux-rel"}),
		)...)

		// Split the test results between parent and included to ensure that the query
		// fetches results for all included invs.
		ms := make([]*spanner.Mutation, 0)
		for i := 1; i <= 100; i++ {
			ms = append(ms, insert.TestResults(t, "inv1", fmt.Sprintf("testId%d", i), pbutil.Variant("a", "b"), pb.TestStatus_PASS)...)
		}
		for i := 101; i <= 200; i++ {
			ms = append(ms, insert.TestResults(t, "inv2", fmt.Sprintf("testId%d", i), pbutil.Variant("a", "b"), pb.TestStatus_PASS)...)
		}

		testutil.MustApply(ctx, t, testutil.CombineMutations(ms)...)
		err := tryMarkInvocationSubmitted(ctx, invocations.ID("inv1"))
		assert.Loosely(t, err, should.BeNil)

		// fetch the 200 entry to ensure that test results from subinvocations
		// are being processed
		res, err := btv.Read(span.Single(ctx), "testproject", "try:linux-rel", "testId200", pbutil.VariantHash(pbutil.Variant("a", "b")))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, res.Project, should.Equal("testproject"))
		assert.Loosely(t, res.BaselineID, should.Equal("try:linux-rel"))
		assert.Loosely(t, res.TestID, should.Equal("testId200"))

		// find the baseline in the baselines table
		baseline, err := baselines.Read(span.Single(ctx), "testproject", "try:linux-rel")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, res.Project, should.Equal(baseline.Project))
		assert.Loosely(t, res.BaselineID, should.Equal(baseline.BaselineID))

		t.Run(`Missing baseline`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_FINALIZED, nil),
			)...)

			_, err := invocations.Read(span.Single(ctx), "a")
			assert.Loosely(t, err, should.BeNil)

			err = tryMarkInvocationSubmitted(ctx, invocations.ID("a"))
			// invocations without a baseline specified will terminate early with no error.
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`Mark existing baseline test variant submitted`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				[]*spanner.Mutation{insert.Invocation("inv3", pb.Invocation_FINALIZED, map[string]any{"BaselineId": "try:linux-rel"})},
				insert.TestResults(t, "inv3", "testId200", pbutil.Variant("a", "b"), pb.TestStatus_PASS),
			)...)

			err := tryMarkInvocationSubmitted(ctx, invocations.ID("inv3"))
			assert.Loosely(t, err, should.BeNil)

			exp := &btv.BaselineTestVariant{
				Project:     "testproject",
				BaselineID:  "try:linux-rel",
				TestID:      "testId200",
				VariantHash: pbutil.VariantHash(pbutil.Variant("a", "b")),
			}

			res, err := btv.Read(span.Single(ctx), exp.Project, exp.BaselineID, exp.TestID, exp.VariantHash)
			// zero out Timestamp
			res.LastUpdated = time.Time{}
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Match(exp))
		})

		t.Run(`Mark skipped test should be skipped`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				[]*spanner.Mutation{insert.Invocation("inv4", pb.Invocation_FINALIZED, map[string]any{"BaselineId": "try:linux-rel"})},
				insert.TestResults(t, "inv4", "testId202", pbutil.Variant("a", "b"), pb.TestStatus_SKIP),
			)...)

			err := tryMarkInvocationSubmitted(ctx, invocations.ID("inv4"))
			assert.Loosely(t, err, should.BeNil)

			exp := &btv.BaselineTestVariant{
				Project:     "testproject",
				BaselineID:  "try:linux-rel",
				TestID:      "testId202",
				VariantHash: pbutil.VariantHash(pbutil.Variant("a", "b")),
			}

			_, err = btv.Read(span.Single(ctx), exp.Project, exp.BaselineID, exp.TestID, exp.VariantHash)
			assert.Loosely(t, err, should.ErrLike(btv.NotFound))
		})
	})
}
