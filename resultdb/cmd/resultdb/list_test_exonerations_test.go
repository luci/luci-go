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

	"go.chromium.org/luci/common/clock"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateListTestExonerationsRequest(t *testing.T) {
	t.Parallel()
	Convey(`Valid`, t, func() {
		req := &pb.ListTestExonerationsRequest{Invocation: "invocations/inv", PageSize: 50}
		So(validateListTestExonerationsRequest(req), ShouldBeNil)
	})

	Convey(`Invalid invocation`, t, func() {
		req := &pb.ListTestExonerationsRequest{Invocation: "bad_name", PageSize: 50}
		So(validateListTestExonerationsRequest(req), ShouldErrLike, "invocation: does not match")
	})

	Convey(`Invalid page size`, t, func() {
		req := &pb.ListTestExonerationsRequest{Invocation: "invocations/inv", PageSize: -50}
		So(validateListTestExonerationsRequest(req), ShouldErrLike, "page_size: negative")
	})
}

func TestListTestExonerations(t *testing.T) {
	Convey(`ListTestExonerations`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		now := clock.Now(ctx)

		// Insert some TestExonerations.
		invID := span.InvocationID("inv")
		testPath := "gn://chrome/test:foo_tests/BarTest.DoBaz"
		var0 := pbutil.Variant("k1", "v1", "k2", "v2")
		testutil.MustApply(ctx,
			testutil.InsertInvocation("inv", pb.Invocation_ACTIVE, "", now),
			span.InsertMap("TestExonerations", map[string]interface{}{
				"InvocationId":        invID,
				"TestPath":            testPath,
				"ExonerationId":       "0",
				"Variant":             var0,
				"VariantHash":         "deadbeef",
				"ExplanationMarkdown": span.Compressed("broken"),
			}),
			span.InsertMap("TestExonerations", map[string]interface{}{
				"InvocationId":  invID,
				"TestPath":      testPath,
				"ExonerationId": "1",
				"Variant":       pbutil.Variant(),
				"VariantHash":   "deadbeef",
			}),
			span.InsertMap("TestExonerations", map[string]interface{}{
				"InvocationId":  invID,
				"TestPath":      testPath,
				"ExonerationId": "2",
				"Variant":       pbutil.Variant(),
				"VariantHash":   "deadbeef",
			}),
		)

		all := []*pb.TestExoneration{
			{
				Name:                pbutil.TestExonerationName("inv", testPath, "0"),
				TestPath:            testPath,
				Variant:             var0,
				ExonerationId:       "0",
				ExplanationMarkdown: "broken",
			},
			{
				Name:          pbutil.TestExonerationName("inv", testPath, "1"),
				TestPath:      testPath,
				ExonerationId: "1",
			},
			{
				Name:          pbutil.TestExonerationName("inv", testPath, "2"),
				TestPath:      testPath,
				ExonerationId: "2",
			},
		}
		srv := &resultDBServer{}

		Convey(`Basic`, func() {
			req := &pb.ListTestExonerationsRequest{Invocation: "invocations/inv"}
			resp, err := srv.ListTestExonerations(ctx, req)
			So(err, ShouldBeNil)
			So(resp, ShouldNotBeNil)
			So(resp.TestExonerations, ShouldResembleProto, all)
			So(resp.NextPageToken, ShouldEqual, "")
		})

		Convey(`With pagination`, func() {
			req := &pb.ListTestExonerationsRequest{
				Invocation: "invocations/inv",
				PageSize:   1,
			}
			res, err := srv.ListTestExonerations(ctx, req)
			So(err, ShouldBeNil)
			So(res, ShouldNotBeNil)
			So(res.TestExonerations, ShouldResembleProto, all[:1])
			So(res.NextPageToken, ShouldNotEqual, "")

			Convey(`Next one`, func() {
				req.PageToken = res.NextPageToken
				res, err = srv.ListTestExonerations(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldNotBeNil)
				So(res.TestExonerations, ShouldResembleProto, all[1:2])
				So(res.NextPageToken, ShouldNotEqual, "")
			})
			Convey(`Next all`, func() {
				req.PageToken = res.NextPageToken
				req.PageSize = 100
				res, err = srv.ListTestExonerations(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldNotBeNil)
				So(res.TestExonerations, ShouldResembleProto, all[1:])
				So(res.NextPageToken, ShouldEqual, "")
			})
		})
	})
}
