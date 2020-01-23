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
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestShouldFinalize(t *testing.T) {
	Convey(`ShouldFinalize`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		assertReady := func(invID span.InvocationID, expected bool) {
			should, err := readyToFinalize(ctx, invID)
			So(err, ShouldBeNil)
			So(should, ShouldEqual, expected)
		}

		Convey(`Includes two ACTIVE`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				testutil.InsertInvocationWithInclusions("a", pb.Invocation_FINALIZING, "b", "c"),
				testutil.InsertInvocationWithInclusions("b", pb.Invocation_ACTIVE),
				testutil.InsertInvocationWithInclusions("c", pb.Invocation_ACTIVE),
			)...)

			assertReady("a", false)
		})

		Convey(`Includes ACTIVE and FINALIZED`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				testutil.InsertInvocationWithInclusions("a", pb.Invocation_FINALIZING, "b", "c"),
				testutil.InsertInvocationWithInclusions("b", pb.Invocation_ACTIVE),
				testutil.InsertInvocationWithInclusions("c", pb.Invocation_FINALIZED),
			)...)

			assertReady("a", false)
		})

		Convey(`INCLUDES ACTIVE and FINALIZING`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				testutil.InsertInvocationWithInclusions("a", pb.Invocation_FINALIZING, "b", "c"),
				testutil.InsertInvocationWithInclusions("b", pb.Invocation_ACTIVE),
				testutil.InsertInvocationWithInclusions("c", pb.Invocation_FINALIZING),
			)...)

			assertReady("a", false)
		})

		Convey(`INCLUDES FINALIZING which includes ACTIVE`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				testutil.InsertInvocationWithInclusions("a", pb.Invocation_FINALIZING, "b"),
				testutil.InsertInvocationWithInclusions("b", pb.Invocation_FINALIZING, "c"),
				testutil.InsertInvocationWithInclusions("c", pb.Invocation_ACTIVE),
			)...)

			assertReady("a", false)
		})

		Convey(`Cycle with one node`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				testutil.InsertInvocationWithInclusions("a", pb.Invocation_FINALIZING, "a"),
			)...)

			assertReady("a", true)
		})

		Convey(`Cycle with two nodes`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				testutil.InsertInvocationWithInclusions("a", pb.Invocation_FINALIZING, "b"),
				testutil.InsertInvocationWithInclusions("b", pb.Invocation_FINALIZING, "a"),
			)...)

			assertReady("a", true)
		})
	})
}

func TestFinalizeInvocation(t *testing.T) {
	Convey(`FinalizeInvocation`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		Convey(`Changes the state and finalization time`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				testutil.InsertInvocationWithInclusions("x", pb.Invocation_FINALIZING),
			)...)

			err := finalizeInvocation(ctx, "x")
			So(err, ShouldBeNil)

			var state pb.Invocation_State
			var finalizeTime time.Time
			testutil.MustReadRow(ctx, "Invocations", span.InvocationID("x").Key(), map[string]interface{}{
				"State":        &state,
				"FinalizeTime": &finalizeTime,
			})
			So(state, ShouldEqual, pb.Invocation_FINALIZED)
			So(finalizeTime, ShouldNotResemble, time.Time{})
		})

		Convey(`Enqueues more finalizing tasks`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				testutil.InsertInvocationWithInclusions("active", pb.Invocation_ACTIVE, "x"),
				testutil.InsertInvocationWithInclusions("finalizing1", pb.Invocation_FINALIZING, "x"),
				testutil.InsertInvocationWithInclusions("finalizing2", pb.Invocation_FINALIZING, "x"),
				testutil.InsertInvocationWithInclusions("x", pb.Invocation_FINALIZING),
			)...)

			err := finalizeInvocation(ctx, "x")
			So(err, ShouldBeNil)

			st := spanner.NewStatement(`
				SELECT InvocationId
				FROM InvocationTasks
				WHERE TaskType = @taskType
			`)
			st.Params["taskType"] = string(tasks.TryFinalizeInvocation)
			var b span.Buffer
			nextInvs := span.InvocationIDSet{}
			err = span.Client(ctx).Single().Query(ctx, st).Do(func(r *spanner.Row) error {
				var nextInv span.InvocationID
				err := b.FromSpanner(r, &nextInv)
				So(err, ShouldBeNil)
				nextInvs.Add(nextInv)
				return nil
			})
			So(err, ShouldBeNil)
			So(nextInvs, ShouldResemble, span.NewInvocationIDSet("finalizing1", "finalizing2"))
		})
	})
}
