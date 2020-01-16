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
	"testing"

	"go.chromium.org/luci/resultdb/internal/span"

	internalpb "go.chromium.org/luci/resultdb/internal/proto"
	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTryFinalizeInvocation(t *testing.T) {
	Convey(`TryFinalizeInvocation`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		assertState := func(id span.InvocationID, expectedState pb.Invocation_State) {
			actualState, err := span.ReadInvocationState(ctx, span.Client(ctx).Single(), "a")
			So(err, ShouldBeNil)
			So(actualState, ShouldEqual, expectedState)
		}

		Convey(`Not ready to finalize`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				testutil.InsertInvocationWithInclusions("a", pb.Invocation_FINALIZING, "b", "c"),
				testutil.InsertInvocationWithInclusions("b", pb.Invocation_FINALIZED),
				testutil.InsertInvocationWithInclusions("c", pb.Invocation_FINALIZED),
			)...)

			err := tryFinalizeInvocation(ctx, "a")
			So(err, ShouldBeNil)

			assertState("a", pb.Invocation_FINALIZED)
		})

		Convey(`Almost ready to finalize`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				testutil.InsertInvocationWithInclusions("a", pb.Invocation_FINALIZING, "b", "c"),
				testutil.InsertInvocationWithInclusions("b", pb.Invocation_FINALIZED),
				testutil.InsertInvocationWithInclusions("c", pb.Invocation_FINALIZED),
			)...)

			err := tryFinalizeInvocation(ctx, "a")
			So(err, ShouldBeNil)

			assertState("a", pb.Invocation_FINALIZED)
		})

		Convey(`Ready to finalize`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				testutil.InsertInvocationWithInclusions("a", pb.Invocation_FINALIZING, "b", "c"),
				testutil.InsertInvocationWithInclusions("b", pb.Invocation_FINALIZED),
				testutil.InsertInvocationWithInclusions("c", pb.Invocation_FINALIZED),
				testutil.InsertInvocationWithInclusions("finalizing_parent", pb.Invocation_FINALIZING, "a"),
				testutil.InsertInvocationWithInclusions("active_parent", pb.Invocation_ACTIVE, "a"),
			)...)

			err := tryFinalizeInvocation(ctx, "a")
			So(err, ShouldBeNil)

			assertState("a", pb.Invocation_FINALIZED)

			// Assert we schedule a task for the finalizing parent.
			newTaskKey := span.TaskKey{
				InvocationID: "finalizing_parent",
				TaskID:       "finalize:from_inv:%s",
			}
			var actualTask internalpb.InvocationTask
			testutil.MustReadRow(ctx, "InvocationTasks", newTaskKey.Key(), map[string]interface{}{
				"Payload": &actualTask,
			})
			So(actualTask, ShouldResembleProtoText, `try_finalize_invocation {}`)

			// Assert we did not schedule a task for the active parent.
			newTaskKey.InvocationID = span.InvocationID("finalized_parent")
			_, err = span.Client(ctx).Single().ReadRow(ctx, "InvocationTasks", newTaskKey.InvocationID.Key(), []string{"Payload"})
			So(err, ShouldErrLike, "not found")
		})
	})
}
