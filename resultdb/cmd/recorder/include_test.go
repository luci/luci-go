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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateIncludeRequest(t *testing.T) {
	Convey(`TestValidateIncludeRequest`, t, func() {
		Convey(`Valid`, func() {
			err := validateIncludeRequest(&pb.IncludeRequest{
				IncludingInvocation: "invocations/a",
				IncludedInvocation:  "invocations/b",
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
	})
}

func TestInclude(t *testing.T) {
	Convey(`TestInclude`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		recorder := NewRecorderServer()

		const token = "update token"
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(updateTokenMetadataKey, token))

		insInv := testutil.InsertInvocation
		ct := testclock.TestRecentTimeUTC

		assertIncluded := func(includedInvID span.InvocationID) {
			var throwAway span.InvocationID
			testutil.MustReadRow(ctx, "IncludedInvocations", span.InclusionKey("including", includedInvID), map[string]interface{}{
				"IncludedInvocationID": &throwAway,
			})
		}

		Convey(`invalid request`, func() {
			_, err := recorder.Include(ctx, &pb.IncludeRequest{})
			So(err, ShouldErrLike, `bad request: including_invocation: unspecified`)
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

		Convey(`included invocation is active`, func() {
			testutil.MustApply(ctx,
				insInv("including", pb.Invocation_ACTIVE, token, ct),
				insInv("included", pb.Invocation_ACTIVE, "", ct),
			)
			_, err := recorder.Include(ctx, req)
			So(err, ShouldErrLike, `"invocations/included" is not finalized`)
			So(grpcutil.Code(err), ShouldEqual, codes.FailedPrecondition)
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
			assertIncluded("included")
		})
	})
}
