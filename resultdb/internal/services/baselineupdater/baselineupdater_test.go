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
	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"

	"go.chromium.org/luci/resultdb/internal/baselines"
	btv "go.chromium.org/luci/resultdb/internal/baselines/testvariants"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"
)

func TestShouldMarkSubmitted(t *testing.T) {
	Convey(`ShouldMarkSubmitted`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		Convey(`FinalizedInvocation`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_FINALIZED, map[string]any{
					"BaselineId": "try:linux-rel",
				}),
			)...)

			inv, err := invocations.Read(span.Single(ctx), "a")
			So(err, ShouldBeNil)

			err = shouldMarkSubmitted(inv)
			So(err, ShouldBeNil)
		})

		Convey(`Not Finalized Invocation`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_ACTIVE, map[string]any{
					"BaselineId": "try:linux-rel",
				}),
			)...)

			inv, err := invocations.Read(span.Single(ctx), "a")
			So(err, ShouldBeNil)

			err = shouldMarkSubmitted(inv)
			So(err, ShouldErrLike, `the invocation is not yet finalized`)
		})

		Convey(`Non existent invocation`, func() {
			_, err := invocations.Read(span.Single(ctx), "a")
			So(err, ShouldErrLike, `invocations/a not found`)
		})
	})
}

func TestEnsureBaselineExists(t *testing.T) {
	Convey(`EnsureBaselineExists`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		Convey(`New Baseline`, func() {
			inv := &pb.Invocation{
				Name:       "invocations/a",
				State:      pb.Invocation_FINALIZED,
				Realm:      "testproject:testrealm",
				BaselineId: "testrealm:linux-rel",
			}

			err := ensureBaselineExists(ctx, inv)
			So(err, ShouldBeNil)

			baseline, err := baselines.Read(span.Single(ctx), "testproject", "testrealm:linux-rel")
			So(err, ShouldBeNil)
			So(baseline.Project, ShouldEqual, "testproject")
			So(baseline.BaselineID, ShouldEqual, "testrealm:linux-rel")
		})

		Convey(`Existing baseline updated`, func() {
			var twoHoursAgo = time.Now().UTC().Add(-time.Hour * 2)
			row := map[string]any{
				"Project":         "testproject",
				"BaselineId":      "try:linux-rel",
				"LastUpdatedTime": twoHoursAgo,
				"CreationTime":    twoHoursAgo,
			}

			testutil.MustApply(ctx,
				spanutil.InsertMap("Baselines", row),
			)

			baseline, err := baselines.Read(span.Single(ctx), "testproject", "try:linux-rel")
			So(err, ShouldBeNil)
			So(baseline.LastUpdatedTime, ShouldEqual, twoHoursAgo)

			inv := &pb.Invocation{
				Name:       "invocations/a",
				State:      pb.Invocation_FINALIZED,
				Realm:      "testproject:testrealm",
				BaselineId: "try:linux-rel",
			}

			// ensure baseline exists should update existing invocations'
			// last updated time to now.
			err = ensureBaselineExists(ctx, inv)
			So(err, ShouldBeNil)

			// re-read the baseline after ensuring it exists. the timestanp should
			// be different from the original timestamp it was written into.
			baseline, err = baselines.Read(span.Single(ctx), "testproject", "try:linux-rel")
			So(err, ShouldBeNil)
			So(baseline.LastUpdatedTime, ShouldNotEqual, twoHoursAgo)
		})
	})
}

func TestTryMarkInvocationSubmitted(t *testing.T) {

	Convey(`e2e`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		testutil.MustApply(ctx, testutil.CombineMutations(
			insert.InvocationWithInclusions("inv1", pb.Invocation_FINALIZED, map[string]any{"BaselineId": "try:linux-rel"}, "inv2"),
			insert.InvocationWithInclusions("inv2", pb.Invocation_FINALIZED, map[string]any{"BaselineId": "try:linux-rel"}),
		)...)

		// Split the test results between parent and included to ensure that the query
		// fetches results for all included invs.
		ms := make([]*spanner.Mutation, 0)
		for i := 1; i <= 100; i++ {
			ms = append(ms, insert.TestResults("inv1", fmt.Sprintf("testId%d", i), pbutil.Variant("a", "b"), pb.TestStatus_PASS)...)
		}
		for i := 101; i <= 200; i++ {
			ms = append(ms, insert.TestResults("inv2", fmt.Sprintf("testId%d", i), pbutil.Variant("a", "b"), pb.TestStatus_PASS)...)
		}

		testutil.MustApply(ctx, testutil.CombineMutations(ms)...)
		err := tryMarkInvocationSubmitted(ctx, invocations.ID("inv1"))
		So(err, ShouldBeNil)

		// fetch the 200 entry to ensure that test results from subinvocations
		// are being processed
		res, err := btv.Read(span.Single(ctx), "testproject", "try:linux-rel", "testId200", pbutil.VariantHash(pbutil.Variant("a", "b")))
		So(err, ShouldBeNil)
		So(res.Project, ShouldEqual, "testproject")
		So(res.BaselineID, ShouldEqual, "try:linux-rel")
		So(res.TestID, ShouldEqual, "testId200")

		// find the baseline in the baselines table
		baseline, err := baselines.Read(span.Single(ctx), "testproject", "try:linux-rel")
		So(err, ShouldBeNil)
		So(res.Project, ShouldEqual, baseline.Project)
		So(res.BaselineID, ShouldEqual, baseline.BaselineID)

		Convey(`Missing baseline`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_FINALIZED, nil),
			)...)

			_, err := invocations.Read(span.Single(ctx), "a")
			So(err, ShouldBeNil)

			err = tryMarkInvocationSubmitted(ctx, invocations.ID("a"))
			// invocations without a baseline specified will terminate early with no error.
			So(err, ShouldBeNil)
		})

		Convey(`Mark existing baseline test variant submitted`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				[]*spanner.Mutation{insert.Invocation("inv3", pb.Invocation_FINALIZED, map[string]any{"BaselineId": "try:linux-rel"})},
				insert.TestResults("inv3", "testId200", pbutil.Variant("a", "b"), pb.TestStatus_PASS),
			)...)

			err := tryMarkInvocationSubmitted(ctx, invocations.ID("inv3"))
			So(err, ShouldBeNil)

			exp := &btv.BaselineTestVariant{
				Project:     "testproject",
				BaselineID:  "try:linux-rel",
				TestID:      "testId200",
				VariantHash: pbutil.VariantHash(pbutil.Variant("a", "b")),
			}

			res, err := btv.Read(span.Single(ctx), exp.Project, exp.BaselineID, exp.TestID, exp.VariantHash)
			// zero out Timestamp
			res.LastUpdated = time.Time{}
			So(err, ShouldBeNil)
			So(res, ShouldResemble, exp)
		})

		Convey(`Mark skipped test should be skipped`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				[]*spanner.Mutation{insert.Invocation("inv4", pb.Invocation_FINALIZED, map[string]any{"BaselineId": "try:linux-rel"})},
				insert.TestResults("inv4", "testId202", pbutil.Variant("a", "b"), pb.TestStatus_SKIP),
			)...)

			err := tryMarkInvocationSubmitted(ctx, invocations.ID("inv4"))
			So(err, ShouldBeNil)

			exp := &btv.BaselineTestVariant{
				Project:     "testproject",
				BaselineID:  "try:linux-rel",
				TestID:      "testId202",
				VariantHash: pbutil.VariantHash(pbutil.Variant("a", "b")),
			}

			_, err = btv.Read(span.Single(ctx), exp.Project, exp.BaselineID, exp.TestID, exp.VariantHash)
			So(err, ShouldErrLike, btv.NotFound)
		})
	})
}
