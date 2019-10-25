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
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

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
		recorder := NewRecorderServer()
		ct := testclock.TestRecentTimeUTC

		const token = "update token"
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(updateTokenMetadataKey, token))

		readInclusionColumn := func(includingInvID, includedInvID, column string, ptr interface{}) {
			testutil.MustReadRow(ctx, "Inclusions", spanner.Key{includingInvID, includedInvID}, map[string]interface{}{
				column: ptr,
			})
		}

		Convey(`finalized failed`, func() {
			testutil.MustApply(ctx,
				testutil.InsertInvocation("inv", pb.Invocation_INTERRUPTED, token, ct),
			)
			inv, err := recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/inv"})
			So(err, ShouldErrLike, `"invocations/inv" has already been finalized with different state`)
			So(inv, ShouldBeNil)
		})

		Convey(`complete expired invocation failed`, func() {
			testutil.MustApply(ctx,
				testutil.InsertInvocation("inv", pb.Invocation_ACTIVE, token, ct),
			)
			// Mock now to be after deadline.
			clock.Get(ctx).(testclock.TestClock).Add(2 * time.Hour)

			inv, err := recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/inv"})
			So(err, ShouldErrLike, `"invocations/inv" has already been finalized with different state`)
			So(inv, ShouldBeNil)
		})

		Convey(`interrupt expired invocation passed`, func() {
			testutil.MustApply(ctx,
				testutil.InsertInvocation("inv", pb.Invocation_ACTIVE, token, ct),
			)
			// Mock now to be after deadline.
			clock.Get(ctx).(testclock.TestClock).Add(2 * time.Hour)

			inv, err := recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/inv", Interrupted: true})
			So(err, ShouldBeNil)
			So(inv.State, ShouldEqual, pb.Invocation_INTERRUPTED)
		})

		Convey(`idempotent`, func() {
			testutil.MustApply(ctx,
				testutil.InsertInvocation("inv", pb.Invocation_ACTIVE, token, ct),
			)

			inv, err := recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/inv"})
			So(err, ShouldBeNil)
			So(inv.State, ShouldEqual, pb.Invocation_COMPLETED)

			inv, err = recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/inv"})
			So(err, ShouldBeNil)
			So(inv.State, ShouldEqual, pb.Invocation_COMPLETED)
		})

		Convey(`finalized`, func() {
			testutil.MustApply(ctx,
				testutil.InsertInvocation("inv", pb.Invocation_ACTIVE, token, ct),
			)
			inv, err := recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/inv"})
			So(err, ShouldBeNil)
			So(inv.State, ShouldEqual, pb.Invocation_COMPLETED)
			So(inv.FinalizeTime, ShouldResemble, pbutil.MustTimestampProto(testclock.TestRecentTimeUTC))

			// Read the invocation from spanner to confirm it's really finalized.
			txn, err := span.Client(ctx).BatchReadOnlyTransaction(ctx, spanner.StrongRead())
			So(err, ShouldBeNil)
			defer txn.Close()

			inv, err = span.ReadInvocationFull(ctx, txn, "inv")
			So(err, ShouldBeNil)
			So(inv.State, ShouldEqual, pb.Invocation_COMPLETED)
			So(inv.FinalizeTime, ShouldResemble, pbutil.MustTimestampProto(testclock.TestRecentTimeUTC))
		})

		Convey(`finalization updates inclusions readiness`, func() {

			testutil.MustApply(ctx,
				testutil.InsertInvocation("including_1", pb.Invocation_ACTIVE, token, ct),
				testutil.InsertInvocation("including_2", pb.Invocation_COMPLETED, token, ct.Add(2*time.Hour)),
				testutil.InsertInvocation("included", pb.Invocation_ACTIVE, token, ct.Add(2*time.Hour)),
				testutil.InsertInclusion("including_1", "included", false, ""),
				testutil.InsertInclusion("including_2", "included", false, ""),
			)

			inv, err := recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/included"})
			So(err, ShouldBeNil)
			So(inv.State, ShouldEqual, pb.Invocation_COMPLETED)
			So(inv.FinalizeTime, ShouldResemble, pbutil.MustTimestampProto(testclock.TestRecentTimeUTC))

			var ready bool

			readInclusionColumn("including_1", "included", "Ready", &ready)
			So(ready, ShouldEqual, true)

			readInclusionColumn("including_2", "included", "Ready", &ready)
			So(ready, ShouldEqual, false)
		})

		Convey(`finalization updates inclusions readiness in chain`, func() {

			testutil.MustApply(ctx,
				testutil.InsertInvocation("including_1", pb.Invocation_ACTIVE, token, ct.Add(2*time.Hour)),
				testutil.InsertInvocation("including_2", pb.Invocation_ACTIVE, token, ct),
				testutil.InsertInvocation("included", pb.Invocation_ACTIVE, token, ct.Add(2*time.Hour)),
				testutil.InsertInclusion("including_1", "including_2", false, ""),
				testutil.InsertInclusion("including_2", "included", false, ""),
			)

			// Mock now to make invocation "including_21" expired.
			clock.Get(ctx).(testclock.TestClock).Add(2 * time.Hour)

			inv, err := recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: "invocations/included"})
			So(err, ShouldBeNil)
			So(inv.State, ShouldEqual, pb.Invocation_COMPLETED)
			So(inv.FinalizeTime, ShouldResemble, pbutil.MustTimestampProto(testclock.TestRecentTimeUTC.Add(2*time.Hour)))

			var ready bool

			readInclusionColumn("including_2", "included", "Ready", &ready)
			So(ready, ShouldEqual, false)

			readInclusionColumn("including_1", "including_2", "Ready", &ready)
			So(ready, ShouldEqual, true)
		})
	})
}
