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

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateIncludeRequest(t *testing.T) {
	Convey(`TestValidateIncludeRequest`, t, func() {
		Convey(`Valid`, func() {
			err := validateIncludeRequest(&pb.IncludeRequest{
				IncludingInvocation: "invocations/a",
				IncludedInvocation:  "invocations/b",
				OverrideInvocation:  "invocations/c",
			})
			So(err, ShouldBeNil)
		})

		Convey(`Invalid including_invocation`, func() {
			err := validateIncludeRequest(&pb.IncludeRequest{
				IncludingInvocation: "x",
				IncludedInvocation:  "invocations/c",
			})
			So(err, ShouldErrLike, `including_invocation: does not match`)
		})

		Convey(`Invalid included_invocation`, func() {
			err := validateIncludeRequest(&pb.IncludeRequest{
				IncludingInvocation: "invocations/a",
				IncludedInvocation:  "x",
			})
			So(err, ShouldErrLike, `included_invocation: does not match`)
		})

		Convey(`include itself`, func() {
			err := validateIncludeRequest(&pb.IncludeRequest{
				IncludingInvocation: "invocations/a",
				IncludedInvocation:  "invocations/a",
			})
			So(err, ShouldErrLike, `cannot include itself`)
		})

		Convey(`Invalid override_invocation`, func() {
			err := validateIncludeRequest(&pb.IncludeRequest{
				IncludingInvocation: "invocations/a",
				IncludedInvocation:  "invocations/b",
				OverrideInvocation:  "x",
			})
			So(err, ShouldErrLike, `override_invocation: does not match`)
		})

		Convey(`override itself`, func() {
			err := validateIncludeRequest(&pb.IncludeRequest{
				IncludingInvocation: "invocations/a",
				IncludedInvocation:  "invocations/b",
				OverrideInvocation:  "invocations/b",
			})
			So(err, ShouldErrLike, `cannot override itself`)
		})

		Convey(`include itself via override`, func() {
			err := validateIncludeRequest(&pb.IncludeRequest{
				IncludingInvocation: "invocations/a",
				IncludedInvocation:  "invocations/b",
				OverrideInvocation:  "invocations/a",
			})
			So(err, ShouldErrLike, `cannot include itself`)
		})
	})
}

func TestInclude(t *testing.T) {
	Convey(`TestInclude`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		recorder := NewRecorderServer()

		const token = "update token"
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(updateTokenMetadataKey, token))

		insInv := testutil.InsertInvocation
		insIncl := testutil.InsertInclusion
		ct := testclock.TestRecentTimeUTC

		readInclusionColumn := func(includedInvID, column string, ptr interface{}) {
			row, err := span.Client(ctx).Single().ReadRow(ctx, "Inclusions", spanner.Key{"including", includedInvID}, []string{column})
			So(err, ShouldBeNil)
			So(row.Columns(ptr), ShouldBeNil)
		}

		Convey(`without overriding`, func() {
			Convey(`invalid request`, func() {
				_, err := recorder.Include(ctx, &pb.IncludeRequest{})
				So(err, ShouldErrLike, `bad request: including_invocation: does not match`)
				So(grpcutil.Code(err), ShouldEqual, codes.InvalidArgument)
			})

			req := &pb.IncludeRequest{
				IncludingInvocation: "invocations/including",
				IncludedInvocation:  "invocations/included",
			}

			Convey(`no including invocation`, func() {
				_, err := recorder.Include(ctx, req)
				So(err, ShouldErrLike, `"invocations/including" not found`)
				So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
			})

			Convey(`no included invocation`, func() {
				testutil.MustApply(ctx,
					insInv("including", pb.Invocation_ACTIVE, token, ct),
				)
				_, err := recorder.Include(ctx, req)
				So(err, ShouldErrLike, `"invocations/included" not found`)
				So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
			})

			Convey(`idempotent`, func() {
				testutil.MustApply(ctx,
					insInv("including", pb.Invocation_ACTIVE, token, ct),
					insInv("included", pb.Invocation_COMPLETED, "", ct),
				)

				_, err := recorder.Include(ctx, req)
				So(err, ShouldBeNil)

				_, err = recorder.Include(ctx, req)
				So(err, ShouldBeNil)
			})

			Convey(`success`, func() {
				testutil.MustApply(ctx,
					insInv("including", pb.Invocation_ACTIVE, token, ct),
					insInv("included", pb.Invocation_COMPLETED, "", ct),
				)

				_, err := recorder.Include(ctx, req)
				So(err, ShouldBeNil)

				var ready bool
				readInclusionColumn("included", "Ready", &ready)
				So(ready, ShouldBeTrue)
			})

			Convey(`unready`, func() {
				testutil.MustApply(ctx,
					insInv("including", pb.Invocation_ACTIVE, token, ct),
					insInv("included", pb.Invocation_ACTIVE, "", ct),
				)

				_, err := recorder.Include(ctx, req)
				So(err, ShouldBeNil)

				var ready bool
				readInclusionColumn("included", "Ready", &ready)
				So(ready, ShouldBeFalse)
			})
		})

		Convey(`with overriding`, func() {
			req := &pb.IncludeRequest{
				IncludingInvocation: "invocations/including",
				IncludedInvocation:  "invocations/included",
				OverrideInvocation:  "invocations/overridden",
			}

			testutil.MustApply(ctx,
				insInv("including", pb.Invocation_ACTIVE, token, ct),
				insInv("included", pb.Invocation_COMPLETED, "", ct),
			)

			Convey(`invocation being overridden does not exist`, func() {
				_, err := recorder.Include(ctx, req)
				So(err, ShouldErrLike, `"invocations/overridden" does not exist or is not included in "invocations/including"`)
				So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
			})

			Convey(`inclusion already overridden by another invocation`, func() {
				testutil.MustApply(ctx,
					insInv("overridden", pb.Invocation_COMPLETED, "", ct),
					insIncl("including", "overridden", false, "else"),
				)

				_, err := recorder.Include(ctx, req)
				So(err, ShouldErrLike, `inclusion of "invocations/overridden" is already overridden by "invocations/else"`)
				So(grpcutil.Code(err), ShouldEqual, codes.FailedPrecondition)
			})

			Convey(`success`, func() {
				testutil.MustApply(ctx,
					insInv("overridden", pb.Invocation_COMPLETED, "", ct),
					insIncl("including", "overridden", false, ""),
				)

				_, err := recorder.Include(ctx, req)
				So(err, ShouldBeNil)

				var actualOverriddenByID spanner.NullString
				readInclusionColumn("overridden", "OverriddenByIncludedInvocationId", &actualOverriddenByID)
				So(actualOverriddenByID.StringVal, ShouldEqual, "included")
			})

			Convey(`idempotent`, func() {
				testutil.MustApply(ctx,
					insInv("overridden", pb.Invocation_COMPLETED, "", ct),
					insIncl("including", "overridden", false, ""),
				)

				_, err := recorder.Include(ctx, req)
				So(err, ShouldBeNil)

				_, err = recorder.Include(ctx, req)
				So(err, ShouldBeNil)
			})
		})
	})
}
