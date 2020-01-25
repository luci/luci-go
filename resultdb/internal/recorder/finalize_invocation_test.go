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

package recorder

import (
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/tasks"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	. "go.chromium.org/luci/resultdb/internal/testutil"
)

func TestValidateFinalizeInvocationRequest(t *testing.T) {
	Convey(`TestValidateFinalizeInvocationRequest`, t, func() {
		Convey(`Valid`, func() {
			err := validateFinalizeInvocationRequest(&pb.FinalizeInvocationRequest{
				Name: "invocations/a",
			})
			So(err, ShouldBeNil)
		})

		Convey(`Invalid name`, func() {
			err := validateFinalizeInvocationRequest(&pb.FinalizeInvocationRequest{
				Name: "x",
			})
			So(err, ShouldErrLike, `name: does not match`)
		})
	})
}

func TestFinalizeInvocation(t *testing.T) {
	Convey(`TestFinalizeInvocation`, t, func() {
		ctx := SpannerTestContext(t)
		recorder := newTestRecorderServer()
		ct := testclock.TestRecentTimeUTC

		const token = "update token"
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(updateTokenMetadataKey, token))

		Convey(`finalized failed`, func() {
			MustApply(ctx,
				InsertInvocation("inv", pb.Invocation_FINALIZED, ct, map[string]interface{}{"Interrupted": true, "UpdateToken": token}),
			)
			_, err := recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/inv"})
			So(err, ShouldHaveAppStatus, codes.FailedPrecondition, `has already been finalized with different interrupted flag`)
		})

		Convey(`Interrupt expired invocation passed`, func() {
			MustApply(ctx,
				InsertInvocation("inv", pb.Invocation_ACTIVE, ct, map[string]interface{}{"UpdateToken": token}),
			)
			// Mock now to be after deadline.
			clock.Get(ctx).(testclock.TestClock).Add(2 * time.Hour)

			inv, err := recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{
				Name:        "invocations/inv",
				Interrupted: true,
			})
			So(err, ShouldBeNil)
			So(inv.State, ShouldEqual, pb.Invocation_FINALIZING)
			So(inv.Interrupted, ShouldEqual, true)

			// Read the invocation from Spanner to confirm it's really interrupted.
			inv, err = span.ReadInvocationFull(ctx, span.Client(ctx).Single(), "inv")
			So(err, ShouldBeNil)
			So(inv.Interrupted, ShouldBeTrue)

		})

		Convey(`Idempotent`, func() {
			MustApply(ctx,
				InsertInvocation("inv", pb.Invocation_ACTIVE, ct, map[string]interface{}{"UpdateToken": token}),
			)

			inv, err := recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/inv"})
			So(err, ShouldBeNil)
			So(inv.State, ShouldEqual, pb.Invocation_FINALIZING)

			inv, err = recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/inv"})
			So(err, ShouldBeNil)
			So(inv.State, ShouldEqual, pb.Invocation_FINALIZING)
		})

		Convey(`Success`, func() {
			extra := map[string]interface{}{
				"UpdateToken": token,
			}
			MustApply(ctx,
				InsertInvocation("inv", pb.Invocation_ACTIVE, ct, extra),
			)
			inv, err := recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/inv"})
			So(err, ShouldBeNil)
			So(inv.State, ShouldEqual, pb.Invocation_FINALIZING)

			// Read the invocation from Spanner to confirm it's really FINALIZING.
			inv, err = span.ReadInvocationFull(ctx, span.Client(ctx).Single(), "inv")
			So(err, ShouldBeNil)
			So(inv.State, ShouldEqual, pb.Invocation_FINALIZING)

			var taskInv span.InvocationID
			taskID := "finalize/" + span.InvocationID("inv").RowID()
			MustReadRow(ctx, "InvocationTasks", tasks.TryFinalizeInvocation.Key(taskID), map[string]interface{}{
				"InvocationId": &taskInv,
			})
			So(taskInv, ShouldEqual, span.InvocationID("inv"))
		})
	})
}
