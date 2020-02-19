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
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	. "go.chromium.org/luci/resultdb/internal/testutil"
)

// validCreateTestResultRequest returns a valid CreateTestResultRequest message.
func validCreateTestResultRequest(now time.Time) *pb.CreateTestResultRequest {
	return &pb.CreateTestResultRequest{
		Invocation: "invocations/u:build-1",
		RequestId:  "this is a requestID 123",

		TestResult: &pb.TestResult{
			Name:     "invocations/a/tests/invocation_id1/results/result_id1",
			TestId:   "this is a testID",
			ResultId: "result_id1",
			Expected: true,
			Status:   pb.TestStatus_PASS,
		},
	}
}

func TestValidateCreateTestResultRequest(t *testing.T) {
	t.Parallel()

	now := testclock.TestRecentTimeUTC
	Convey("ValidateCreateTestResultRequest", t, func() {
		req := validCreateTestResultRequest(now)

		Convey("suceeeds", func() {
			So(validateCreateTestResultRequest(req, now), ShouldBeNil)

			Convey("with empty request_id", func() {
				req.RequestId = ""
				So(validateCreateTestResultRequest(req, now), ShouldBeNil)
			})
		})

		Convey("fails with ", func() {
			Convey(`empty invocation`, func() {
				req.Invocation = ""
				err := validateCreateTestResultRequest(req, now)
				So(err, ShouldErrLike, "invocation: unspecified")
			})
			Convey(`invalid invocation`, func() {
				req.Invocation = " invalid "
				err := validateCreateTestResultRequest(req, now)
				So(err, ShouldErrLike, "invocation: does not match")
			})

			Convey(`empty test_result`, func() {
				req.TestResult = nil
				err := validateCreateTestResultRequest(req, now)
				So(err, ShouldErrLike, "test_result: unspecified")
			})
			Convey(`invalid test_result`, func() {
				req.TestResult.TestId = ""
				err := validateCreateTestResultRequest(req, now)
				So(err, ShouldErrLike, "test_result: test_id: unspecified")
			})

			Convey("invalid request_id", func() {
				// non-ascii character
				req.RequestId = string(rune(244))
				err := validateCreateTestResultRequest(req, now)
				So(err, ShouldErrLike, "request_id: does not match")
			})
		})
	})
}

func TestCreateTestResult(t *testing.T) {
	Convey(`CreateTestResult`, t, func() {
		ctx := SpannerTestContext(t)
		recorder := newTestRecorderServer()
		req := validCreateTestResultRequest(clock.Now(ctx).UTC())
		invID := span.MustParseInvocationName(req.Invocation)

		createTestResult := func(req *pb.CreateTestResultRequest) {
			tr := req.TestResult
			expected := proto.Clone(tr).(*pb.TestResult)
			expected.Name = pbutil.TestResultName(string(invID), tr.TestId, tr.ResultId)
			res, err := recorder.CreateTestResult(ctx, req)
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, expected)

			// double-check it with the database
			row, err := span.ReadTestResult(ctx, span.Client(ctx).Single(), res.Name)
			So(err, ShouldBeNil)
			So(row, ShouldResembleProto, expected)

			// variant hash
			key := span.InvocationID(invID).Key(res.TestId, res.ResultId)
			var variantHash string
			MustReadRow(ctx, "TestResults", key, map[string]interface{}{
				"VariantHash": &variantHash,
			})
			So(variantHash, ShouldEqual, pbutil.VariantHash(res.Variant))
		}

		// Insert a sample invocation
		const tok = "update token"
		vs := map[string]interface{}{"UpdateToken": tok}
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(UpdateTokenMetadataKey, tok))
		MustApply(ctx, InsertInvocation(span.InvocationID(invID), pb.Invocation_ACTIVE, vs))

		Convey("succeeds", func() {
			Convey("with a request ID", func() {
				createTestResult(req)
			})

			Convey("without a request ID", func() {
				req.RequestId = ""
				createTestResult(req)
			})
		})

		Convey("fails", func() {
			Convey("with an invalid request", func() {
				req.Invocation = "this is an invalid invocation name"
				_, err := recorder.CreateTestResult(ctx, req)
				So(err, ShouldHaveAppStatus, codes.InvalidArgument, "bad request: invocation: does not match")
			})

			Convey("with an non-existing invocation", func() {
				req.Invocation = "invocations/inv"
				_, err := recorder.CreateTestResult(ctx, req)
				So(err, ShouldHaveAppStatus, codes.NotFound, "invocations/inv not found")
			})
		})
	})
}
