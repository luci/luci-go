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

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateBatchCreateTestExonerationsRequest(t *testing.T) {
	t.Parallel()
	Convey(`TestValidateBatchCreateTestExonerationsRequest`, t, func() {
		Convey(`Empty`, func() {
			err := validateBatchCreateTestExonerationsRequest(&pb.BatchCreateTestExonerationsRequest{})
			So(err, ShouldErrLike, `invocation: unspecified`)
		})

		Convey(`Invalid invocation`, func() {
			err := validateBatchCreateTestExonerationsRequest(&pb.BatchCreateTestExonerationsRequest{
				Invocation: "x",
			})
			So(err, ShouldErrLike, `invocation: does not match`)
		})

		Convey(`Invalid request id`, func() {
			err := validateBatchCreateTestExonerationsRequest(&pb.BatchCreateTestExonerationsRequest{
				Invocation: "invocations/a",
				RequestId:  "😃",
			})
			So(err, ShouldErrLike, `request_id: does not match`)
		})

		Convey(`Invalid sub-request`, func() {
			err := validateBatchCreateTestExonerationsRequest(&pb.BatchCreateTestExonerationsRequest{
				Invocation: "invocations/inv",
				Requests: []*pb.CreateTestExonerationRequest{
					{
						TestExoneration: &pb.TestExoneration{
							TestPath: "\x01",
						},
					},
				},
			})
			So(err, ShouldErrLike, `requests[0]: test_exoneration: test_path: does not match`)
		})

		Convey(`Inconsistent invocation`, func() {
			err := validateBatchCreateTestExonerationsRequest(&pb.BatchCreateTestExonerationsRequest{
				Invocation: "invocations/inv",
				Requests: []*pb.CreateTestExonerationRequest{
					{
						Invocation: "invocations/x",
						TestExoneration: &pb.TestExoneration{
							TestPath: "gn://ab/cd.ef",
						},
					},
				},
			})
			So(err, ShouldErrLike, `requests[0]: invocation: inconsistent with top-level invocation`)
		})

		Convey(`Inconsistent request_id`, func() {
			err := validateBatchCreateTestExonerationsRequest(&pb.BatchCreateTestExonerationsRequest{
				Invocation: "invocations/inv",
				RequestId:  "req1",
				Requests: []*pb.CreateTestExonerationRequest{
					{
						RequestId: "req2",
						TestExoneration: &pb.TestExoneration{
							TestPath: "gn://ab/cd.ef",
						},
					},
				},
			})
			So(err, ShouldErrLike, `requests[0]: request_id: inconsistent with top-level request_id`)
		})

		Convey(`Valid`, func() {
			err := validateBatchCreateTestExonerationsRequest(&pb.BatchCreateTestExonerationsRequest{
				Invocation: "invocations/inv",
				Requests: []*pb.CreateTestExonerationRequest{
					{
						TestExoneration: &pb.TestExoneration{
							TestPath: "gn://ab/cd.ef",
						},
					},
					{
						TestExoneration: &pb.TestExoneration{
							TestPath: "gn://ab/cd.ef",
							Variant:  pbutil.Variant("a/b", "1", "c", "2"),
						},
					},
				},
				RequestId: "a",
			})
			So(err, ShouldBeNil)
		})
	})
}

func TestBatchCreateTestExonerations(t *testing.T) {
	Convey(`TestBatchCreateTestExonerations`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		recorder := &recorderServer{}

		const token = "update token"
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(updateTokenMetadataKey, token))

		Convey(`Invalid request`, func() {
			req := &pb.BatchCreateTestExonerationsRequest{
				Invocation: "x",
			}
			_, err := recorder.BatchCreateTestExonerations(ctx, req)
			So(err, ShouldErrLike, `bad request: invocation: does not match`)
			So(grpcutil.Code(err), ShouldEqual, codes.InvalidArgument)
		})

		Convey(`No invocation`, func() {
			req := &pb.BatchCreateTestExonerationsRequest{
				Invocation: "invocations/inv",
			}
			_, err := recorder.BatchCreateTestExonerations(ctx, req)
			So(err, ShouldErrLike, `"invocations/inv" not found`)
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
		})

		// Insert the invocation.
		testutil.MustApply(ctx, testutil.InsertInvocation("inv", pb.Invocation_ACTIVE, token, testclock.TestRecentTimeUTC))

		e2eTest := func(withRequestID bool) {
			req := &pb.BatchCreateTestExonerationsRequest{
				Invocation: "invocations/inv",
				Requests: []*pb.CreateTestExonerationRequest{
					{
						TestExoneration: &pb.TestExoneration{
							TestPath: "a",
							Variant:  pbutil.Variant("a", "1", "b", "2"),
						},
					},
					{
						TestExoneration: &pb.TestExoneration{
							TestPath: "b/c",
							Variant:  pbutil.Variant("a", "1", "b", "2"),
						},
					},
				},
			}

			if withRequestID {
				req.RequestId = "request id"
			}

			res, err := recorder.BatchCreateTestExonerations(ctx, req)
			So(err, ShouldBeNil)

			So(res.TestExonerations, ShouldHaveLength, len(req.Requests))
			for i := range req.Requests {
				actual := res.TestExonerations[i]

				expected := proto.Clone(req.Requests[i].TestExoneration).(*pb.TestExoneration)
				proto.Merge(expected, &pb.TestExoneration{
					Name:          pbutil.TestExonerationName("inv", expected.TestPath, actual.ExonerationId),
					ExonerationId: actual.ExonerationId,
				})

				So(actual, ShouldResembleProto, expected)

				// Now check the database.
				row, err := span.ReadTestExonerationFull(ctx, span.Client(ctx).Single(), actual.Name)
				So(err, ShouldBeNil)
				So(row.Variant, ShouldResembleProto, expected.Variant)
				So(row.ExplanationMarkdown, ShouldEqual, expected.ExplanationMarkdown)
			}

			if withRequestID {
				// Test idempotency.
				res2, err := recorder.BatchCreateTestExonerations(ctx, req)
				So(err, ShouldBeNil)
				So(res2, ShouldResembleProto, res)
			}
		}

		Convey(`Without request id, e2e`, func() {
			e2eTest(false)
		})
		Convey(`With request id, e2e`, func() {
			e2eTest(true)
		})
	})
}
