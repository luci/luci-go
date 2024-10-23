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

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestValidateBatchCreateTestExonerationsRequest(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestValidateBatchCreateTestExonerationsRequest`, t, func(t *ftt.Test) {
		t.Run(`Empty`, func(t *ftt.Test) {
			err := validateBatchCreateTestExonerationsRequest(&pb.BatchCreateTestExonerationsRequest{})
			assert.Loosely(t, err, should.ErrLike(`invocation: unspecified`))
		})

		t.Run(`Invalid invocation`, func(t *ftt.Test) {
			err := validateBatchCreateTestExonerationsRequest(&pb.BatchCreateTestExonerationsRequest{
				Invocation: "x",
			})
			assert.Loosely(t, err, should.ErrLike(`invocation: does not match`))
		})

		t.Run(`Invalid request id`, func(t *ftt.Test) {
			err := validateBatchCreateTestExonerationsRequest(&pb.BatchCreateTestExonerationsRequest{
				Invocation: "invocations/a",
				RequestId:  "😃",
			})
			assert.Loosely(t, err, should.ErrLike(`request_id: does not match`))
		})

		t.Run(`Too many requests`, func(t *ftt.Test) {
			err := validateBatchCreateTestExonerationsRequest(&pb.BatchCreateTestExonerationsRequest{
				Invocation: "invocations/a",
				Requests:   make([]*pb.CreateTestExonerationRequest, 1000),
			})
			assert.Loosely(t, err, should.ErrLike(`the number of requests in the batch exceeds 500`))
		})

		t.Run(`Invalid sub-request`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.ErrLike(`requests[0]: test_exoneration: test_id: non-printable rune`))
		})

		t.Run(`Inconsistent invocation`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.ErrLike(`requests[0]: invocation: inconsistent with top-level invocation`))
		})

		t.Run(`Inconsistent request_id`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.ErrLike(`requests[0]: request_id: inconsistent with top-level request_id`))
		})

		t.Run(`Valid`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
		})
	})
}

func TestBatchCreateTestExonerations(t *testing.T) {
	ftt.Run(`TestBatchCreateTestExonerations`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		recorder := newTestRecorderServer()

		token, err := generateInvocationToken(ctx, "inv")
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		t.Run(`Invalid request`, func(t *ftt.Test) {
			req := &pb.BatchCreateTestExonerationsRequest{
				Invocation: "x",
			}
			_, err := recorder.BatchCreateTestExonerations(ctx, req)
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)(`bad request: invocation: does not match`))
		})

		t.Run(`No invocation`, func(t *ftt.Test) {
			req := &pb.BatchCreateTestExonerationsRequest{
				Invocation: "invocations/inv",
			}
			_, err := recorder.BatchCreateTestExonerations(ctx, req)
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCNotFound)(`invocations/inv not found`))
		})

		e2eTest := func(withRequestID bool) {

			// Insert the invocation.
			testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))

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
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, res.TestExonerations, should.HaveLength(len(req.Requests)))
			for i := range req.Requests {
				actual := res.TestExonerations[i]

				expected := proto.Clone(req.Requests[i].TestExoneration).(*pb.TestExoneration)
				proto.Merge(expected, &pb.TestExoneration{
					Name:          pbutil.TestExonerationName("inv", expected.TestId, actual.ExonerationId),
					ExonerationId: actual.ExonerationId,
					VariantHash:   pbutil.VariantHash(expected.Variant),
				})

				assert.Loosely(t, actual, should.Resemble(expected))

				// Now check the database.
				row, err := exonerations.Read(span.Single(ctx), actual.Name)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, row.Variant, should.Resemble(expected.Variant))
				assert.Loosely(t, row.ExplanationHtml, should.Equal(expected.ExplanationHtml))
				assert.Loosely(t, row.Reason, should.Equal(expected.Reason))
			}

			if withRequestID {
				// Test idempotency.
				res2, err := recorder.BatchCreateTestExonerations(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res2, should.Resemble(res))
			}
		}

		t.Run(`Without request id, e2e`, func(t *ftt.Test) {
			e2eTest(false)
		})
		t.Run(`With request id, e2e`, func(t *ftt.Test) {
			e2eTest(true)
		})
	})
}
