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
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateCreateTestExonerationRequest(t *testing.T) {
	t.Parallel()
	Convey(`TestValidateCreateTestExonerationRequest`, t, func() {
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

		Convey(`Empty Invocation`, func() {
			req.Invocation = ""
			err := validateCreateTestExonerationRequest(req, true)
			So(err, ShouldErrLike, `invocation: unspecified`)
		})

		Convey(`Empty Exoneration`, func() {
			req.TestExoneration = nil
			err := validateCreateTestExonerationRequest(req, true)
			So(err, ShouldErrLike, `test_exoneration: test_id: unspecified`)
		})

		Convey(`NUL in test id`, func() {
			req.TestExoneration.TestId = "\x01"
			err := validateCreateTestExonerationRequest(req, true)
			So(err, ShouldErrLike, "test_id: non-printable rune")
		})

		Convey(`Invalid variant`, func() {
			req.TestExoneration.Variant = pbutil.Variant("", "")
			err := validateCreateTestExonerationRequest(req, true)
			So(err, ShouldErrLike, `variant: "":"": key: unspecified`)
		})

		Convey(`Reason not specified`, func() {
			req.TestExoneration.Reason = pb.ExonerationReason_EXONERATION_REASON_UNSPECIFIED
			err := validateCreateTestExonerationRequest(req, true)
			So(err, ShouldErrLike, `test_exoneration: reason: unspecified`)
		})

		Convey(`Explanation HTML not specified`, func() {
			req.TestExoneration.ExplanationHtml = ""
			err := validateCreateTestExonerationRequest(req, true)
			So(err, ShouldErrLike, `test_exoneration: explanation_html: unspecified`)
		})

		Convey(`Valid`, func() {
			err := validateCreateTestExonerationRequest(req, true)
			So(err, ShouldBeNil)
		})

		Convey(`Mismatching variant hashes`, func() {
			req.TestExoneration.VariantHash = "doesn't match"
			err := validateCreateTestExonerationRequest(req, true)
			So(err, ShouldErrLike, `computed and supplied variant hash don't match`)
		})

		Convey(`Matching variant hashes`, func() {
			req.TestExoneration.Variant = pbutil.Variant("a", "b")
			req.TestExoneration.VariantHash = "c467ccce5a16dc72"
			err := validateCreateTestExonerationRequest(req, true)
			So(err, ShouldBeNil)
		})
	})
}

func TestCreateTestExoneration(t *testing.T) {
	Convey(`TestCreateTestExoneration`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		recorder := newTestRecorderServer()

		token, err := generateInvocationToken(ctx, "inv")
		So(err, ShouldBeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		Convey(`Invalid request`, func() {
			req := &pb.CreateTestExonerationRequest{
				Invocation: "invocations/inv",
				TestExoneration: &pb.TestExoneration{
					TestId:          "\x01",
					ExplanationHtml: "Unexpected pass.",
					Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
				},
			}
			_, err := recorder.CreateTestExoneration(ctx, req)
			So(err, ShouldBeRPCInvalidArgument, `bad request: test_exoneration: test_id: non-printable rune`)
		})

		Convey(`No invocation`, func() {
			req := &pb.CreateTestExonerationRequest{
				Invocation: "invocations/inv",
				TestExoneration: &pb.TestExoneration{
					TestId:          "a",
					ExplanationHtml: "Unexpected pass.",
					Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
				},
			}
			_, err := recorder.CreateTestExoneration(ctx, req)
			So(err, ShouldBeRPCNotFound, `invocations/inv not found`)
		})

		// Insert the invocation.
		testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))

		e2eTest := func(req *pb.CreateTestExonerationRequest, expectedVariantHash, expectedId string) {
			res, err := recorder.CreateTestExoneration(ctx, req)
			So(err, ShouldBeNil)

			if expectedId == "" {
				So(res.ExonerationId, ShouldStartWith, expectedVariantHash+":")
			} else {
				So(res.ExonerationId, ShouldEqual, expectedVariantHash+":"+expectedId)
			}

			expected := proto.Clone(req.TestExoneration).(*pb.TestExoneration)
			proto.Merge(expected, &pb.TestExoneration{
				Name:          pbutil.TestExonerationName("inv", "a", res.ExonerationId),
				ExonerationId: res.ExonerationId,
				VariantHash:   expectedVariantHash,
			})
			So(res, ShouldResembleProto, expected)

			// Now check the database.
			row, err := exonerations.Read(span.Single(ctx), res.Name)
			So(err, ShouldBeNil)
			So(row.Variant, ShouldResembleProto, expected.Variant)
			So(row.ExplanationHtml, ShouldEqual, expected.ExplanationHtml)

			// Check variant hash.
			key := invocations.ID("inv").Key(res.TestId, res.ExonerationId)
			var variantHash string
			testutil.MustReadRow(ctx, "TestExonerations", key, map[string]any{
				"VariantHash": &variantHash,
			})
			So(variantHash, ShouldEqual, expectedVariantHash)

			if req.RequestId != "" {
				// Test idempotency.
				res2, err := recorder.CreateTestExoneration(ctx, req)
				So(err, ShouldBeNil)
				So(res2, ShouldResembleProto, res)
			}
		}

		Convey(`Without request id, e2e`, func() {
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

		Convey(`With request id, e2e`, func() {
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

		Convey(`With hash but no variant, e2e`, func() {
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
