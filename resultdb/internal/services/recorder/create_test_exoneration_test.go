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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/exonerations"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestValidateCreateTestExonerationRequest(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestValidateCreateTestExonerationRequest`, t, func(t *ftt.Test) {
		req := &pb.CreateTestExonerationRequest{
			Invocation: "invocations/inv",
			TestExoneration: &pb.TestExoneration{
				TestId: "ninja://ab/cd.ef",
				Variant: pbutil.Variant(
					"a/b", "1",
					"c", "2",
				),
				ExplanationHtml: "The test also failed without patch",
				Reason:          pb.ExonerationReason_OCCURS_ON_MAINLINE,
			},
		}

		t.Run(`Empty Invocation`, func(t *ftt.Test) {
			req.Invocation = ""
			err := validateCreateTestExonerationRequest(req, true)
			assert.Loosely(t, err, should.ErrLike(`invocation: unspecified`))
		})

		t.Run(`Empty Exoneration`, func(t *ftt.Test) {
			req.TestExoneration = nil
			err := validateCreateTestExonerationRequest(req, true)
			assert.Loosely(t, err, should.ErrLike(`test_exoneration: test_id: unspecified`))
		})

		t.Run(`NUL in test id`, func(t *ftt.Test) {
			req.TestExoneration.TestId = "\x01"
			err := validateCreateTestExonerationRequest(req, true)
			assert.Loosely(t, err, should.ErrLike("test_id: non-printable rune"))
		})

		t.Run(`Invalid variant`, func(t *ftt.Test) {
			req.TestExoneration.Variant = pbutil.Variant("", "")
			err := validateCreateTestExonerationRequest(req, true)
			assert.Loosely(t, err, should.ErrLike(`variant: "":"": key: unspecified`))
		})

		t.Run(`Reason not specified`, func(t *ftt.Test) {
			req.TestExoneration.Reason = pb.ExonerationReason_EXONERATION_REASON_UNSPECIFIED
			err := validateCreateTestExonerationRequest(req, true)
			assert.Loosely(t, err, should.ErrLike(`test_exoneration: reason: unspecified`))
		})

		t.Run(`Explanation HTML not specified`, func(t *ftt.Test) {
			req.TestExoneration.ExplanationHtml = ""
			err := validateCreateTestExonerationRequest(req, true)
			assert.Loosely(t, err, should.ErrLike(`test_exoneration: explanation_html: unspecified`))
		})

		t.Run(`Valid`, func(t *ftt.Test) {
			err := validateCreateTestExonerationRequest(req, true)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`Mismatching variant hashes`, func(t *ftt.Test) {
			req.TestExoneration.VariantHash = "doesn't match"
			err := validateCreateTestExonerationRequest(req, true)
			assert.Loosely(t, err, should.ErrLike(`computed and supplied variant hash don't match`))
		})

		t.Run(`Matching variant hashes`, func(t *ftt.Test) {
			req.TestExoneration.Variant = pbutil.Variant("a", "b")
			req.TestExoneration.VariantHash = "c467ccce5a16dc72"
			err := validateCreateTestExonerationRequest(req, true)
			assert.Loosely(t, err, should.BeNil)
		})
	})
}

func TestCreateTestExoneration(t *testing.T) {
	ftt.Run(`TestCreateTestExoneration`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		recorder := newTestRecorderServer()

		token, err := generateInvocationToken(ctx, "inv")
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		t.Run(`Invalid request`, func(t *ftt.Test) {
			req := &pb.CreateTestExonerationRequest{
				Invocation: "invocations/inv",
				TestExoneration: &pb.TestExoneration{
					TestId:          "\x01",
					ExplanationHtml: "Unexpected pass.",
					Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
				},
			}
			_, err := recorder.CreateTestExoneration(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`bad request: test_exoneration: test_id: non-printable rune`))
		})

		t.Run(`No invocation`, func(t *ftt.Test) {
			req := &pb.CreateTestExonerationRequest{
				Invocation: "invocations/inv",
				TestExoneration: &pb.TestExoneration{
					TestId:          "a",
					ExplanationHtml: "Unexpected pass.",
					Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
				},
			}
			_, err := recorder.CreateTestExoneration(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike(`invocations/inv not found`))
		})

		e2eTest := func(req *pb.CreateTestExonerationRequest, expectedVariantHash, expectedId string) {
			// Insert the invocation.
			testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))

			res, err := recorder.CreateTestExoneration(ctx, req)
			assert.Loosely(t, err, should.BeNil)

			if expectedId == "" {
				assert.Loosely(t, res.ExonerationId, should.HavePrefix(expectedVariantHash+":"))
			} else {
				assert.Loosely(t, res.ExonerationId, should.Equal(expectedVariantHash+":"+expectedId))
			}

			expected := proto.Clone(req.TestExoneration).(*pb.TestExoneration)
			proto.Merge(expected, &pb.TestExoneration{
				Name:          pbutil.TestExonerationName("inv", "a", res.ExonerationId),
				ExonerationId: res.ExonerationId,
				VariantHash:   expectedVariantHash,
			})
			assert.Loosely(t, res, should.Resemble(expected))

			// Now check the database.
			row, err := exonerations.Read(span.Single(ctx), res.Name)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, row.Variant, should.Resemble(expected.Variant))
			assert.Loosely(t, row.ExplanationHtml, should.Equal(expected.ExplanationHtml))

			// Check variant hash.
			key := invocations.ID("inv").Key(res.TestId, res.ExonerationId)
			var variantHash string
			testutil.MustReadRow(ctx, t, "TestExonerations", key, map[string]any{
				"VariantHash": &variantHash,
			})
			assert.Loosely(t, variantHash, should.Equal(expectedVariantHash))

			if req.RequestId != "" {
				// Test idempotency.
				res2, err := recorder.CreateTestExoneration(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res2, should.Resemble(res))
			}
		}

		t.Run(`Without request id, e2e`, func(t *ftt.Test) {
			e2eTest(&pb.CreateTestExonerationRequest{
				Invocation: "invocations/inv",
				TestExoneration: &pb.TestExoneration{
					TestId:          "a",
					Variant:         pbutil.Variant("a", "1", "b", "2"),
					ExplanationHtml: "Test is known flaky. Similar test failures have been observed in other CLs.",
					Reason:          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
				},
			}, "6408fdc5c36df5df", "")
		})

		t.Run(`With request id, e2e`, func(t *ftt.Test) {
			e2eTest(&pb.CreateTestExonerationRequest{
				RequestId:  "request id",
				Invocation: "invocations/inv",
				TestExoneration: &pb.TestExoneration{
					TestId:          "a",
					Variant:         pbutil.Variant("a", "1", "b", "2"),
					ExplanationHtml: "Test also failed when tried without patch.",
					Reason:          pb.ExonerationReason_OCCURS_ON_MAINLINE,
				},
			}, "6408fdc5c36df5df", "d:2960f0231ce23039cdf7d4a62e31939ecd897bbf465e0fb2d35bf425ae1c5ae14eb0714d6dd0a0c244eaa66ae2b645b0637f58e91ed1b820bb1f01d8d4a72e67")
		})

		t.Run(`With hash but no variant, e2e`, func(t *ftt.Test) {
			e2eTest(&pb.CreateTestExonerationRequest{
				RequestId:  "request id",
				Invocation: "invocations/inv",
				TestExoneration: &pb.TestExoneration{
					TestId:          "a",
					VariantHash:     "deadbeefdeadbeef",
					ExplanationHtml: "Unexpected pass.",
					Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
				},
			}, "deadbeefdeadbeef", "")
		})
	})
}
