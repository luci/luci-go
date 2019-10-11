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

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCreateInclusion(t *testing.T) {
	Convey("TestCreateInclusion", t, func() {
		ctx := testutil.SpannerTestContext(t)
		recorder := &RecorderServer{}

		const token = "update token"
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(updateTokenMetadataKey, token))

		insInv := testutil.InsertInvocation

		req := &pb.CreateInclusionRequest{
			IncludingInvocation: "invocations/including",
			Inclusion: &pb.Inclusion{
				IncludedInvocation: "invocations/included",
			},
		}

		Convey("no including invocation", func() {
			testutil.MustApply(ctx, insInv("included", pb.Invocation_ACTIVE, token))

			_, err := recorder.CreateInclusion(ctx, req)
			So(err, ShouldErrLike, `invocation "invocations/including": not found`)
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
		})

		Convey("no included invocation", func() {
			testutil.MustApply(ctx, insInv("including", pb.Invocation_ACTIVE, token))

			_, err := recorder.CreateInclusion(ctx, req)
			So(err, ShouldErrLike, `invocation "invocations/included": not found`)
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
		})

		Convey("success", func() {
			testutil.MustApply(ctx,
				insInv("including", pb.Invocation_ACTIVE, token),
				insInv("included", pb.Invocation_COMPLETED, ""),
			)

			incl, err := recorder.CreateInclusion(ctx, req)
			So(err, ShouldBeNil)
			So(incl, ShouldResembleProto, &pb.Inclusion{
				Name:               "invocations/including/inclusions/included",
				IncludedInvocation: "invocations/included",
				Ready:              true,
			})

			var ready bool
			err = span.ReadRow(ctx, span.Client(ctx).Single(), "Inclusions", spanner.Key{"including", "included"}, map[string]interface{}{"Ready": &ready})
			So(err, ShouldBeNil)
			So(ready, ShouldBeTrue)
		})

		Convey("unready", func() {
			testutil.MustApply(ctx,
				insInv("including", pb.Invocation_ACTIVE, token),
				insInv("included", pb.Invocation_ACTIVE, ""),
			)

			incl, err := recorder.CreateInclusion(ctx, &pb.CreateInclusionRequest{
				IncludingInvocation: "invocations/including",
				Inclusion: &pb.Inclusion{
					IncludedInvocation: "invocations/included",
				},
			})
			So(err, ShouldBeNil)
			So(incl.Ready, ShouldBeFalse)
		})
	})
}
