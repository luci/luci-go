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
	internalpb "go.chromium.org/luci/resultdb/internal/proto"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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

		Convey(`Includes two ACTIVE`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				testutil.InsertInvocationWithInclusions("active", pb.Invocation_ACTIVE, "x"),
				testutil.InsertInvocationWithInclusions("finalizing", pb.Invocation_FINALIZING, "x"),
				testutil.InsertInvocationWithInclusions("x", pb.Invocation_FINALIZING),
			)...)

			_, err := span.Client(ctx).ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
				return finalizeInvocation(ctx, txn, "x")
			})
			So(err, ShouldBeNil)

			actualState, err := span.ReadInvocationState(ctx, span.Client(ctx).Single(), "x")
			So(err, ShouldBeNil)
			So(actualState, ShouldEqual, pb.Invocation_FINALIZED)

			finalizingTaskKey := span.TaskKey{InvocationID: "finalizing", TaskID: "finalize:from_inv:x"}
			payload := &internalpb.InvocationTask{}
			testutil.MustReadRow(ctx, "InvocationTasks", finalizingTaskKey.Key(), map[string]interface{}{
				"Payload": payload,
			})
			So(payload, ShouldResembleProtoText, `try_finalize_invocation{}`)

			activeTaskKey := span.TaskKey{InvocationID: "active", TaskID: "finalize:from_inv:x"}
			_, err = span.Client(ctx).Single().ReadRow(ctx, "InvocationTasks", activeTaskKey.Key(), []string{"Payload"})
			So(status.Code(err), ShouldEqual, codes.NotFound)
		})
	})
}
