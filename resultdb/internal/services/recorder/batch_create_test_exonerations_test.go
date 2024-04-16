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

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/exonerations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

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

		Convey(`Too many requests`, func() {
			err := validateBatchCreateTestExonerationsRequest(&pb.BatchCreateTestExonerationsRequest{
				Invocation: "invocations/a",
				Requests:   make([]*pb.CreateTestExonerationRequest, 1000),
			})
			So(err, ShouldErrLike, `the number of requests in the batch exceeds 500`)
		})

		Convey(`Invalid sub-request`, func() {
			err := validateBatchCreateTestExonerationsRequest(&pb.BatchCreateTestExonerationsRequest{
				Invocation: "invocations/inv",
				Requests: []*pb.CreateTestExonerationRequest{
					{
						TestExoneration: &pb.TestExoneration{
							TestId:          "\x01",
							ExplanationHtml: "Unexpected pass.",
							Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
						},
					},
				},
			})
			So(err, ShouldErrLike, `requests[0]: test_exoneration: test_id: non-printable rune`)
		})

		Convey(`Inconsistent invocation`, func() {
			err := validateBatchCreateTestExonerationsRequest(&pb.BatchCreateTestExonerationsRequest{
				Invocation: "invocations/inv",
				Requests: []*pb.CreateTestExonerationRequest{
					{
						Invocation: "invocations/x",
						TestExoneration: &pb.TestExoneration{
							TestId:          "ninja://ab/cd.ef",
							ExplanationHtml: "Unexpected pass.",
							Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
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
							TestId:          "ninja://ab/cd.ef",
							ExplanationHtml: "Unexpected pass.",
							Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
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
							TestId:          "ninja://ab/cd.ef",
							ExplanationHtml: "Test failed also when tried without patch.",
							Reason:          pb.ExonerationReason_OCCURS_ON_MAINLINE,
						},
					},
					{
						TestExoneration: &pb.TestExoneration{
							TestId:          "ninja://ab/cd.ef",
							Variant:         pbutil.Variant("a/b", "1", "c", "2"),
							ExplanationHtml: "Test is known to be failing on other CLs.",
							Reason:          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
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

		recorder := newTestRecorderServer()

		token, err := generateInvocationToken(ctx, "inv")
		So(err, ShouldBeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		Convey(`Invalid request`, func() {
			req := &pb.BatchCreateTestExonerationsRequest{
				Invocation: "x",
			}
			_, err := recorder.BatchCreateTestExonerations(ctx, req)
			So(err, ShouldBeRPCInvalidArgument, `bad request: invocation: does not match`)
		})

		Convey(`No invocation`, func() {
			req := &pb.BatchCreateTestExonerationsRequest{
				Invocation: "invocations/inv",
			}
			_, err := recorder.BatchCreateTestExonerations(ctx, req)
			So(err, ShouldBeRPCNotFound, `invocations/inv not found`)
		})

		// Insert the invocation.
		testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))

		e2eTest := func(withRequestID bool) {
			req := &pb.BatchCreateTestExonerationsRequest{
				Invocation: "invocations/inv",
				Requests: []*pb.CreateTestExonerationRequest{
					{
						TestExoneration: &pb.TestExoneration{
							TestId:          "a",
							Variant:         pbutil.Variant("a", "1", "b", "2"),
							ExplanationHtml: "Unexpected pass.",
							Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
						},
					},
					{
						TestExoneration: &pb.TestExoneration{
							TestId:          "b/c",
							Variant:         pbutil.Variant("a", "1", "b", "2"),
							ExplanationHtml: "Unexpected pass.",
							Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
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
					Name:          pbutil.TestExonerationName("inv", expected.TestId, actual.ExonerationId),
					ExonerationId: actual.ExonerationId,
					VariantHash:   pbutil.VariantHash(expected.Variant),
				})

				So(actual, ShouldResembleProto, expected)

				// Now check the database.
				row, err := exonerations.Read(span.Single(ctx), actual.Name)
				So(err, ShouldBeNil)
				So(row.Variant, ShouldResembleProto, expected.Variant)
				So(row.ExplanationHtml, ShouldEqual, expected.ExplanationHtml)
				So(row.Reason, ShouldEqual, expected.Reason)
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
