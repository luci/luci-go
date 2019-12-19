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
	"testing"
	"time"

	tspb "github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	internalpb "go.chromium.org/luci/resultdb/internal/proto"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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
		ctx := testutil.SpannerTestContext(t)
		recorder := &recorderServer{}
		ct := testclock.TestRecentTimeUTC

		const token = "update token"
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(updateTokenMetadataKey, token))

		Convey(`finalized failed`, func() {
			testutil.MustApply(ctx,
				testutil.InsertInvocation("inv", pb.Invocation_COMPLETED, token, ct, true),
			)
			inv, err := recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/inv"})
			So(err, ShouldErrLike, `"invocations/inv" has already been finalized with different interrupted flag`)
			So(inv, ShouldBeNil)
		})

		Convey(`complete expired invocation failed`, func() {
			testutil.MustApply(ctx,
				testutil.InsertInvocation("inv", pb.Invocation_ACTIVE, token, ct, false),
			)
			// Mock now to be after deadline.
			clock.Get(ctx).(testclock.TestClock).Add(2 * time.Hour)

			inv, err := recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/inv"})
			So(err, ShouldErrLike, `"invocations/inv" has already been finalized with different interrupted flag`)
			So(inv, ShouldBeNil)
		})

		Convey(`interrupt expired invocation passed`, func() {
			testutil.MustApply(ctx,
				testutil.InsertInvocation("inv", pb.Invocation_ACTIVE, token, ct, false),
			)
			// Mock now to be after deadline.
			clock.Get(ctx).(testclock.TestClock).Add(2 * time.Hour)

			inv, err := recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/inv", Interrupted: true})
			So(err, ShouldBeNil)
			So(inv.State, ShouldEqual, pb.Invocation_COMPLETED)
			So(inv.Interrupted, ShouldEqual, true)
		})

		Convey(`idempotent`, func() {
			testutil.MustApply(ctx,
				testutil.InsertInvocation("inv", pb.Invocation_ACTIVE, token, ct, false),
			)

			inv, err := recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/inv"})
			So(err, ShouldBeNil)
			So(inv.State, ShouldEqual, pb.Invocation_COMPLETED)

			inv, err = recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/inv"})
			So(err, ShouldBeNil)
			So(inv.State, ShouldEqual, pb.Invocation_COMPLETED)
		})

		Convey(`finalized`, func() {
			now := testclock.TestRecentTimeUTC
			nowTimestamp := pbutil.MustTimestampProto(now)
			origProcessAfter := now.Add(2 * day)

			invID := span.InvocationID("inv")
			invTask := &internalpb.InvocationTask{
				BigqueryExport: &pb.BigQueryExport{}}

			test := func(resetOnFinalize bool, expected *tspb.Timestamp) {
				testutil.MustApply(ctx,
					testutil.InsertInvocation(invID, pb.Invocation_ACTIVE, token, ct, false),
					insertInvocationTask(invID, taskID(taskTypeBqExport, 0), invTask, origProcessAfter, resetOnFinalize),
				)
				inv, err := recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/inv"})
				So(err, ShouldBeNil)
				So(inv.State, ShouldEqual, pb.Invocation_COMPLETED)
				So(inv.FinalizeTime, ShouldResemble, pbutil.MustTimestampProto(testclock.TestRecentTimeUTC))
				// Read the invocation from spanner to confirm it's really finalized.
				txn := span.Client(ctx).ReadOnlyTransaction()
				defer txn.Close()

				inv, err = span.ReadInvocationFull(ctx, txn, "inv")
				So(err, ShouldBeNil)
				So(inv.State, ShouldEqual, pb.Invocation_COMPLETED)
				So(inv.FinalizeTime, ShouldResemble, nowTimestamp)

				// Read InvocationTask to confirm it's reset.
				key := invID.Key(taskID(taskTypeBqExport, 0))
				var processAfter *tspb.Timestamp
				testutil.MustReadRow(ctx, "InvocationTasks", key, map[string]interface{}{
					"ProcessAfter": &processAfter,
				})
				So(processAfter, ShouldResemble, expected)
			}

			Convey(`finalized and reset InvocationTasks`, func() {
				test(true, nowTimestamp)
			})

			Convey(`finalized and not reset InvocationTasks`, func() {
				test(false, pbutil.MustTimestampProto(origProcessAfter))
			})
		})
	})
}
