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
	"fmt"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/resultcount"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

// validBatchCreateTestResultsRequest returns a valid BatchCreateTestResultsRequest message.
func validBatchCreateTestResultRequest(now time.Time, invName, testID string) *pb.BatchCreateTestResultsRequest {
	tr1 := validCreateTestResultRequest(now, invName, testID)
	tr2 := validCreateTestResultRequest(now, invName, testID)
	tr1.TestResult.ResultId = "result-id-0"
	tr2.TestResult.ResultId = "result-id-1"

	return &pb.BatchCreateTestResultsRequest{
		Invocation: invName,
		Requests:   []*pb.CreateTestResultRequest{tr1, tr2},
		RequestId:  "request-id-123",
	}
}

func TestValidateBatchCreateTestResultRequest(t *testing.T) {
	t.Parallel()

	now := testclock.TestRecentTimeUTC
	Convey("ValidateBatchCreateTestResultsRequest", t, func() {
		req := validBatchCreateTestResultRequest(now, "invocations/u-build-1", "test-id")

		Convey("succeeds", func() {
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
			Convey(`Too many requests`, func() {
				req.Requests = make([]*pb.CreateTestResultRequest, 1000)
				err := validateBatchCreateTestResultsRequest(req, now)
				So(err, ShouldErrLike, `the number of requests in the batch exceeds 500`)
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

			Convey("duplicated test_results", func() {
				req.Requests[0].TestResult.TestId = "test-id"
				req.Requests[0].TestResult.ResultId = "result-id"
				req.Requests[1].TestResult.TestId = "test-id"
				req.Requests[1].TestResult.ResultId = "result-id"
				err := validateBatchCreateTestResultsRequest(req, now)
				So(err, ShouldErrLike, "duplicate test results in request")
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
	Convey(`BatchCreateTestResults`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		recorder := newTestRecorderServer()
		now := clock.Now(ctx).UTC()
		invName := "invocations/u-build-1"
		req := validBatchCreateTestResultRequest(
			now, invName, "test-id",
		)

		createTestResults := func(req *pb.BatchCreateTestResultsRequest, expectedCommonPrefix string) {
			response, err := recorder.BatchCreateTestResults(ctx, req)
			So(err, ShouldBeNil)

			for i, r := range req.Requests {
				expected := proto.Clone(r.TestResult).(*pb.TestResult)
				expected.Name = fmt.Sprintf("invocations/u-build-1/tests/%s/results/result-id-%d", expected.TestId, i)
				expected.VariantHash = pbutil.VariantHash(expected.Variant)
				So(response.TestResults[i], ShouldResembleProto, expected)

				// double-check it with the database
				row, err := testresults.Read(span.Single(ctx), expected.Name)
				So(err, ShouldBeNil)
				So(row, ShouldResembleProto, expected)

				var invCommonTestIDPrefix string
				var invVars []string
				err = invocations.ReadColumns(
					span.Single(ctx), invocations.ID("u-build-1"),
					map[string]any{
						"CommonTestIDPrefix":     &invCommonTestIDPrefix,
						"TestResultVariantUnion": &invVars,
					})
				So(err, ShouldBeNil)
				So(invCommonTestIDPrefix, ShouldEqual, expectedCommonPrefix)
				So(invVars, ShouldResemble, []string{
					"a/b:1",
					"c:2",
				})
			}
		}

		// Insert a sample invocation
		tok, err := generateInvocationToken(ctx, "u-build-1")
		So(err, ShouldBeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, tok))
		invID := invocations.ID("u-build-1")
		testutil.MustApply(ctx, insert.Invocation(invID, pb.Invocation_ACTIVE, nil))

		Convey("succeeds", func() {
			Convey("with a request ID", func() {
				createTestResults(req, "test-id")

				ctx, cancel := span.ReadOnlyTransaction(ctx)
				defer cancel()
				trNum, err := resultcount.ReadTestResultCount(ctx, invocations.NewIDSet(invID))
				So(err, ShouldBeNil)
				So(trNum, ShouldEqual, 2)
			})

			Convey("without a request ID", func() {
				req.RequestId = ""
				req.Requests[0].RequestId = ""
				req.Requests[1].RequestId = ""
				createTestResults(req, "test-id")
			})

			Convey("with uncommon test id", func() {
				newTr := validCreateTestResultRequest(now, invName, "some-other-test-id")
				newTr.TestResult.ResultId = "result-id-2"
				req.Requests = append(req.Requests, newTr)
				createTestResults(req, "")
			})
		})

		Convey("fails", func() {
			Convey("with an invalid request", func() {
				req.Invocation = "this is an invalid invocation name"
				req.Requests[0].Invocation = ""
				req.Requests[1].Invocation = ""
				_, err := recorder.BatchCreateTestResults(ctx, req)
				So(err, ShouldBeRPCInvalidArgument, "bad request: invocation: does not match")
			})

			Convey("with an non-existing invocation", func() {
				tok, err = generateInvocationToken(ctx, "inv")
				So(err, ShouldBeNil)
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, tok))
				req.Invocation = "invocations/inv"
				req.Requests[0].Invocation = ""
				req.Requests[1].Invocation = ""
				_, err := recorder.BatchCreateTestResults(ctx, req)
				So(err, ShouldBeRPCNotFound, "invocations/inv not found")
			})
		})
	})
}

func TestLongestCommonPrefix(t *testing.T) {
	t.Parallel()
	Convey("empty", t, func() {
		So(longestCommonPrefix("", "str"), ShouldEqual, "")
	})

	Convey("no common prefix", t, func() {
		So(longestCommonPrefix("str", "other"), ShouldEqual, "")
	})

	Convey("common prefix", t, func() {
		So(longestCommonPrefix("prefix_1", "prefix_2"), ShouldEqual, "prefix_")
	})
}
