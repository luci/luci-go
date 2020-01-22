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

package main

import (
	"context"
	"testing"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/clock/testclock"

	internalpb "go.chromium.org/luci/resultdb/internal/proto"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestShouldFinalize(t *testing.T) {
	Convey(`ShouldFinalize`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		asseretShould := func(invID span.InvocationID, expected bool) {
			txn := span.Client(ctx).ReadOnlyTransaction()
			defer txn.Close()

			should, err := shouldFinalize(ctx, txn, invID)
			So(err, ShouldBeNil)
			So(should, ShouldEqual, expected)
		}

		Convey(`Includes two ACTIVE`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				testutil.InsertInvocationWithInclusions("a", pb.Invocation_FINALIZING, "b", "c"),
				testutil.InsertInvocationWithInclusions("b", pb.Invocation_ACTIVE),
				testutil.InsertInvocationWithInclusions("c", pb.Invocation_ACTIVE),
			)...)

			asseretShould("a", false)
		})

		Convey(`Includes ACTIVE and FINALIZED`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				testutil.InsertInvocationWithInclusions("a", pb.Invocation_FINALIZING, "b", "c"),
				testutil.InsertInvocationWithInclusions("b", pb.Invocation_ACTIVE),
				testutil.InsertInvocationWithInclusions("c", pb.Invocation_FINALIZED),
			)...)

			asseretShould("a", false)
		})

		Convey(`INCLUDES ACTIVE and FINALIZING`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				testutil.InsertInvocationWithInclusions("a", pb.Invocation_FINALIZING, "b", "c"),
				testutil.InsertInvocationWithInclusions("b", pb.Invocation_ACTIVE),
				testutil.InsertInvocationWithInclusions("c", pb.Invocation_FINALIZING),
			)...)

			asseretShould("a", false)
		})

		Convey(`INCLUDES FINALIZING which includes ACTIVE`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				testutil.InsertInvocationWithInclusions("a", pb.Invocation_FINALIZING, "b"),
				testutil.InsertInvocationWithInclusions("b", pb.Invocation_FINALIZING, "c"),
				testutil.InsertInvocationWithInclusions("c", pb.Invocation_ACTIVE),
			)...)

			asseretShould("a", false)
		})

		Convey(`Cycle with one node`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				testutil.InsertInvocationWithInclusions("a", pb.Invocation_FINALIZING, "a"),
			)...)

			asseretShould("a", true)
		})

		Convey(`Cycle with two nodes`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				testutil.InsertInvocationWithInclusions("a", pb.Invocation_FINALIZING, "b"),
				testutil.InsertInvocationWithInclusions("b", pb.Invocation_FINALIZING, "a"),
			)...)

			asseretShould("a", true)
		})
	})
}

func TestFinalizeInvocation(t *testing.T) {
	Convey(`FinalizeInvocation`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		finalize := func(invID span.InvocationID) {
			_, err := span.Client(ctx).ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
				return finalizeInvocation(ctx, txn, invID)
			})
			So(err, ShouldBeNil)
		}

		Convey(`Enqueues more finalizing tasks`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				testutil.InsertInvocationWithInclusions("active", pb.Invocation_ACTIVE, "x"),
				testutil.InsertInvocationWithInclusions("finalizing1", pb.Invocation_FINALIZING, "x"),
				testutil.InsertInvocationWithInclusions("finalizing2", pb.Invocation_FINALIZING, "x"),
				testutil.InsertInvocationWithInclusions("x", pb.Invocation_FINALIZING),
			)...)

			finalize("x")

			actualState, err := span.ReadInvocationState(ctx, span.Client(ctx).Single(), "x")
			So(err, ShouldBeNil)
			So(actualState, ShouldEqual, pb.Invocation_FINALIZED)

			st := spanner.NewStatement(`
				SELECT InvocationId, Payload
				FROM InvocationTasks
				WHERE STARTS_WITH(TaskId, "finalize/x/")
			`)
			var b span.Buffer
			nextInvs := span.InvocationIDSet{}
			err = span.Client(ctx).Single().Query(ctx, st).Do(func(r *spanner.Row) error {
				var nextInv span.InvocationID
				payload := &internalpb.InvocationTask{}
				err := b.FromSpanner(r, &nextInv, payload)
				So(err, ShouldBeNil)
				nextInvs.Add(nextInv)
				So(payload, ShouldResembleProtoText, `try_finalize_invocation{}`)
				return nil
			})
			So(err, ShouldBeNil)
			So(nextInvs, ShouldResemble, span.NewInvocationIDSet("finalizing1", "finalizing2"))
		})

		Convey(`BigQuery tasks`, func() {
			testutil.MustApply(ctx,
				testutil.InsertInvocation("inv", pb.Invocation_ACTIVE, testclock.TestRecentTimeUTC, map[string]interface{}{
					"BigQueryExports": span.CompressedProto{&internalpb.BigQueryExports{
						BigqueryExports: []*pb.BigQueryExport{&pb.BigQueryExport{}},
					}},
				}),
			)

			finalize("inv")

			var invID span.InvocationID
			testutil.MustReadRow(ctx, "InvocationTasks", spanner.Key{"bq_export:inv:0"}, map[string]interface{}{
				"InvocationID": &invID,
			})
			So(invID, ShouldEqual, "inv")
		})
	})
}
