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

	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestOverrideInclusion(t *testing.T) {
	Convey("TestCreateInclusion", t, func() {
		ctx := testutil.SpannerTestContext(t)
		recorder := &RecorderServer{}

		const token = "update token"
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(updateTokenMetadataKey, token))

		insInv := testutil.InsertInvocation
		insIncl := testutil.InsertInclusion

		req := &pb.OverrideInclusionRequest{
			IncludingInvocation:          "invocations/including",
			OverridingIncludedInvocation: "invocations/overriding",
			OverriddenIncludedInvocation: "invocations/overridden",
		}

		Convey("no including invocation", func() {
			_, err := recorder.OverrideInclusion(ctx, req)
			So(err, ShouldErrLike, `invocation "invocations/including" not found`)
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
		})

		Convey("no overriding invocation", func() {
			testutil.MustApply(
				ctx,
				insInv("including", pb.Invocation_ACTIVE, token),
				insInv("overridden", pb.Invocation_COMPLETED, token),
				insIncl("including", "overridden", false),
			)

			_, err := recorder.OverrideInclusion(ctx, req)
			So(err, ShouldErrLike, `invocation "invocations/overriding" not found`)
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
		})

		Convey("no overridden invocation", func() {
			testutil.MustApply(
				ctx,
				insInv("including", pb.Invocation_ACTIVE, token),
				insInv("overriding", pb.Invocation_COMPLETED, token),
				insIncl("including", "overridden", false),
			)

			_, err := recorder.OverrideInclusion(ctx, req)
			So(err, ShouldErrLike, `invocation "invocations/overridden" not found`)
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
		})

		Convey("inclusion does not exist", func() {
			testutil.MustApply(ctx,
				insInv("including", pb.Invocation_ACTIVE, token),
				insInv("overridden", pb.Invocation_COMPLETED, ""),
				insInv("overriding", pb.Invocation_COMPLETED, ""),
			)

			_, err := recorder.OverrideInclusion(ctx, req)
			So(err, ShouldErrLike, `inclusion "invocations/including/inclusions/overridden" not found`)
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
		})

		Convey("inclusion already overridden by the same invocation", func() {
			testutil.MustApply(ctx,
				insInv("including", pb.Invocation_ACTIVE, token),
				insInv("overridden", pb.Invocation_COMPLETED, ""),
				insInv("overriding", pb.Invocation_COMPLETED, ""),
				spanner.InsertMap("Inclusions", map[string]interface{}{
					"InvocationId":                     "including",
					"IncludedInvocationId":             "overridden",
					"Ready":                            true,
					"OverriddenByIncludedInvocationID": "something else",
				}),
			)

			_, err := recorder.OverrideInclusion(ctx, req)
			So(err, ShouldErrLike, `inclusion "invocations/including/inclusions/overridden" is already overridden by "invocations/including/inclusions/something else"`)
			So(grpcutil.Code(err), ShouldEqual, codes.FailedPrecondition)
		})

		Convey("success", func() {
			testutil.MustApply(ctx,
				insInv("including", pb.Invocation_ACTIVE, token),
				insInv("overridden", pb.Invocation_COMPLETED, ""),
				insInv("overriding", pb.Invocation_COMPLETED, ""),
				insIncl("including", "overridden", false),
			)

			res, err := recorder.OverrideInclusion(ctx, req)
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &pb.OverrideInclusionResponse{
				OverriddenInclusion: &pb.Inclusion{
					Name:               "invocations/including/inclusions/overridden",
					IncludedInvocation: "invocations/overridden",
					Ready:              true,
				},
				OverridingInclusion: &pb.Inclusion{
					Name:               "invocations/including/inclusions/overriding",
					IncludedInvocation: "invocations/overriding",
					Ready:              true,
				},
			})
		})

		Convey("overridden unready", func() {
			testutil.MustApply(ctx,
				insInv("including", pb.Invocation_ACTIVE, token),
				insInv("overridden", pb.Invocation_ACTIVE, ""),
				insInv("overriding", pb.Invocation_COMPLETED, ""),
				insIncl("including", "overridden", false),
			)

			res, err := recorder.OverrideInclusion(ctx, req)
			So(err, ShouldBeNil)
			So(res.OverriddenInclusion.Ready, ShouldBeFalse)
			So(res.OverridingInclusion.Ready, ShouldBeTrue)
		})

		Convey("overriding unready", func() {
			testutil.MustApply(ctx,
				insInv("including", pb.Invocation_ACTIVE, token),
				insInv("overridden", pb.Invocation_COMPLETED, ""),
				insInv("overriding", pb.Invocation_ACTIVE, ""),
				insIncl("including", "overridden", false),
			)

			res, err := recorder.OverrideInclusion(ctx, req)
			So(err, ShouldBeNil)
			So(res.OverriddenInclusion.Ready, ShouldBeTrue)
			So(res.OverridingInclusion.Ready, ShouldBeFalse)
		})
	})
}
