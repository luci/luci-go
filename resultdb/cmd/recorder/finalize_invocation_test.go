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

	"cloud.google.com/go/spanner"
	tspb "github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	internalpb "go.chromium.org/luci/resultdb/internal/proto"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
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
		recorder := &recorderServer{}
		ct := testclock.TestRecentTimeUTC

		const token = "update token"
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(updateTokenMetadataKey, token))

		Convey(`finalized failed`, func() {
			MustApply(ctx,
				InsertInvocation("inv", pb.Invocation_FINALIZED, ct, map[string]interface{}{"Interrupted": true, "UpdateToken": token}),
			)
			_, err := recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/inv"})
			So(err, ShouldHaveAppStatus, codes.FailedPrecondition, `invocations/inv has already been finalized with different interrupted flag`)
		})

		Convey(`complete expired invocation failed`, func() {
			MustApply(ctx,
				InsertInvocation("inv", pb.Invocation_ACTIVE, ct, map[string]interface{}{"UpdateToken": token}),
			)
			// Mock now to be after deadline.
			clock.Get(ctx).(testclock.TestClock).Add(2 * time.Hour)

			_, err := recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/inv"})
			So(err, ShouldHaveAppStatus, codes.FailedPrecondition, `invocations/inv has already been finalized with different interrupted flag`)
		})

		Convey(`interrupt expired invocation passed`, func() {
			MustApply(ctx,
				InsertInvocation("inv", pb.Invocation_ACTIVE, ct, map[string]interface{}{"UpdateToken": token}),
			)
			// Mock now to be after deadline.
			clock.Get(ctx).(testclock.TestClock).Add(2 * time.Hour)

			inv, err := recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/inv", Interrupted: true})
			So(err, ShouldBeNil)
			So(inv.State, ShouldEqual, pb.Invocation_FINALIZED)
			So(inv.Interrupted, ShouldEqual, true)
		})

		Convey(`idempotent`, func() {
			MustApply(ctx,
				InsertInvocation("inv", pb.Invocation_ACTIVE, ct, map[string]interface{}{"UpdateToken": token}),
			)

			inv, err := recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/inv"})
			So(err, ShouldBeNil)
			So(inv.State, ShouldEqual, pb.Invocation_FINALIZED)

			inv, err = recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/inv"})
			So(err, ShouldBeNil)
			So(inv.State, ShouldEqual, pb.Invocation_FINALIZED)
		})

		Convey(`finalized`, func() {
			now := testclock.TestRecentTimeUTC
			nowTimestamp := pbutil.MustTimestampProto(now)

			Convey(`finalized and add InvocationTasks`, func() {
				extra := map[string]interface{}{
					"UpdateToken": token,
					"BigQueryExports": span.CompressedProto{&internalpb.BigQueryExports{
						BigqueryExports: []*pb.BigQueryExport{&pb.BigQueryExport{}}}},
				}
				MustApply(ctx,
					InsertInvocation("inv", pb.Invocation_ACTIVE, ct, extra),
				)
				inv, err := recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/inv"})
				So(err, ShouldBeNil)
				So(inv.State, ShouldEqual, pb.Invocation_FINALIZED)
				So(inv.FinalizeTime, ShouldResemble, pbutil.MustTimestampProto(testclock.TestRecentTimeUTC))
				// Read the invocation from spanner to confirm it's really finalized.
				txn := span.Client(ctx).ReadOnlyTransaction()
				defer txn.Close()

				inv, err = span.ReadInvocationFull(ctx, txn, "inv")
				So(err, ShouldBeNil)
				So(inv.State, ShouldEqual, pb.Invocation_FINALIZED)
				So(inv.FinalizeTime, ShouldResemble, nowTimestamp)

				// Read InvocationTask to confirm it's added.
				var processAfter *tspb.Timestamp
				var invID span.InvocationID
				MustReadRow(ctx, "InvocationTasks", spanner.Key{bqTaskID("inv", 0)}, map[string]interface{}{
					"InvocationID": &invID,
					"ProcessAfter": &processAfter,
				})
				So(invID, ShouldEqual, "inv")
				So(processAfter, ShouldResemble, nowTimestamp)
			})
		})
	})
}
