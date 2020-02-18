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

	"go.chromium.org/luci/common/clock/testclock"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	. "go.chromium.org/luci/resultdb/internal/testutil"
)

// validBatchCreateTestResultsRequest returns a valid BatchCreateTestResultsRequest message.
func validBatchCreateTestResultRequest(now time.Time) *pb.BatchCreateTestResultsRequest {
	tr1 := validCreateTestResultRequest(now)
	tr2 := validCreateTestResultRequest(now)
	tr1.Invocation = "invocations/u:build-abc"
	tr2.Invocation = "invocations/u:build-abc"
	tr1.RequestId = "request-id-123"
	tr2.RequestId = "request-id-123"
	tr1.TestResult.TestId = "test-id-123"
	tr2.TestResult.TestId = "test-id-123"
	tr1.TestResult.ResultId = "result-id-1"
	tr2.TestResult.ResultId = "result-id-2"

	return &pb.BatchCreateTestResultsRequest{
		Invocation: "invocations/u:build-abc",
		Requests:   []*pb.CreateTestResultRequest{tr1, tr2},
		RequestId:  "request-id-123",
	}
}

func TestValidateBatchCreateTestResultRequest(t *testing.T) {
	t.Parallel()

	now := testclock.TestRecentTimeUTC
	Convey("ValidateBatchCreateTestResultsRequest", t, func() {
		req := validBatchCreateTestResultRequest(now)

		Convey("suceeeds", func() {
			So(validateBatchCreateTestResultsRequest(req, now), ShouldBeNil)

			Convey("with empty request_id", func() {
				Convey("in requests", func() {
					req.Requests[0].RequestId = ""
					req.Requests[1].RequestId = ""
					So(validateBatchCreateTestResultsRequest(req, now), ShouldBeNil)
				})
				Convey("in both batch-level and requests", func() {
					req.Requests[0].RequestId = ""
					req.Requests[1].RequestId = ""
					req.RequestId = ""
					So(validateBatchCreateTestResultsRequest(req, now), ShouldBeNil)
				})
			})
			Convey("with empty invocation in requests", func() {
				req.Requests[0].Invocation = ""
				req.Requests[1].Invocation = ""
				So(validateBatchCreateTestResultsRequest(req, now), ShouldBeNil)
			})
		})

		Convey("fails with", func() {
			Convey("empty requests", func() {
				req.Requests = []*pb.CreateTestResultRequest{}
				err := validateBatchCreateTestResultsRequest(req, now)
				So(err, ShouldErrLike, "requests: unspecified")
			})

			Convey("invocation", func() {
				Convey("empty in batch-level", func() {
					req.Invocation = ""
					err := validateBatchCreateTestResultsRequest(req, now)
					So(err, ShouldErrLike, "invocation: unspecified")
				})
				Convey("unmatched invocation in requests", func() {
					req.Invocation = "invocations/foo"
					req.Requests[0].Invocation = "invocations/bar"
					err := validateBatchCreateTestResultsRequest(req, now)
					So(err, ShouldErrLike, "requests: 0: invocation must be either empty or equal")
				})
			})

			Convey("invalid test_result", func() {
				req.Requests[0].TestResult.TestId = ""
				err := validateBatchCreateTestResultsRequest(req, now)
				So(err, ShouldErrLike, "test_result: test_id: unspecified")
			})

			Convey("request_id", func() {
				Convey("with an invalid character", func() {
					// non-ascii character
					req.RequestId = string(rune(244))
					err := validateBatchCreateTestResultsRequest(req, now)
					So(err, ShouldErrLike, "request_id: does not match")
				})
				Convey("empty in batch-level, but not in requests", func() {
					req.RequestId = ""
					req.Requests[0].RequestId = "123"
					err := validateBatchCreateTestResultsRequest(req, now)
					So(err, ShouldErrLike, "requests: 0: request_id must be either empty or equal")
				})
				Convey("unmatched request_id in requests", func() {
					req.RequestId = "foo"
					req.Requests[0].RequestId = "bar"
					err := validateBatchCreateTestResultsRequest(req, now)
					So(err, ShouldErrLike, "requests: 0: request_id must be either empty or equal")
				})
			})
		})
	})
}

func TestBatchCreateTestResults(t *testing.T) {
	now := testclock.TestRecentTimeUTC

	Convey(`BatchCreateTestResults`, t, func() {
		ctx := SpannerTestContext(t)
		recorder := newTestRecorderServer()
		req := validBatchCreateTestResultRequest(now)
		invID, err := pbutil.ParseInvocationName(req.Invocation)
		So(err, ShouldBeNil)

		createTestResults := func(req *pb.BatchCreateTestResultsRequest) {
			response, err := recorder.BatchCreateTestResults(ctx, req)
			So(err, ShouldBeNil)

			for i, r := range req.Requests {
				reqTR := r.TestResult
				resTR := response.TestResults[i]
				expected := proto.Clone(reqTR).(*pb.TestResult)
				expected.Name = pbutil.TestResultName(invID, reqTR.TestId, reqTR.ResultId)
				So(resTR, ShouldResembleProto, expected)

				// double-check it with the database
				spanClient := span.Client(ctx).Single()
				row, err := span.ReadTestResult(ctx, spanClient, expected.Name)
				So(err, ShouldBeNil)
				So(row, ShouldResembleProto, expected)

				// variant hash
				key := span.InvocationID(invID).Key(reqTR.TestId, reqTR.ResultId)
				var variantHash string
				MustReadRow(ctx, "TestResults", key, map[string]interface{}{
					"VariantHash": &variantHash,
				})
				So(variantHash, ShouldEqual, pbutil.VariantHash(reqTR.Variant))
			}
		}

		// Insert a sample invocation
		const tok = "update token"
		vs := map[string]interface{}{"UpdateToken": tok}
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(UpdateTokenMetadataKey, tok))
		MustApply(ctx, InsertInvocation(span.InvocationID(invID), pb.Invocation_ACTIVE, vs))

		Convey("succeeds", func() {
			Convey("with a request ID", func() {
				createTestResults(req)
			})

			Convey("without a request ID", func() {
				req.RequestId = ""
				req.Requests[0].RequestId = ""
				req.Requests[1].RequestId = ""
				createTestResults(req)
			})
		})

		Convey("fails", func() {
			Convey("with an invalid request", func() {
				req.Invocation = "this is an invalid invocation name"
				req.Requests[0].Invocation = ""
				req.Requests[1].Invocation = ""
				_, err := recorder.BatchCreateTestResults(ctx, req)
				So(err, ShouldHaveAppStatus, codes.InvalidArgument, "bad request: invocation: does not match")
			})

			Convey("with an non-existing invocation", func() {
				req.Invocation = "invocations/inv"
				req.Requests[0].Invocation = ""
				req.Requests[1].Invocation = ""
				_, err := recorder.BatchCreateTestResults(ctx, req)
				So(err, ShouldHaveAppStatus, codes.NotFound, "invocations/inv not found")
			})
		})
	})
}
