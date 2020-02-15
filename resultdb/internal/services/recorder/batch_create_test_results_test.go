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

	"go.chromium.org/luci/common/clock/testclock"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

// validBatchCreateTestResultsRequest returns a valid BatchCreateTestResultsRequest message.
func validBatchCreateTestResultRequest(now time.Time) *pb.BatchCreateTestResultsRequest {
	tr := validCreateTestResultRequest(now)
	tr.Invocation = "invocations/u:build-abc"
	tr.RequestId = "request-id-123"

	return &pb.BatchCreateTestResultsRequest{
		Invocation: "invocations/u:build-abc",
		Requests:   []*pb.CreateTestResultRequest{tr},
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
					So(validateBatchCreateTestResultsRequest(req, now), ShouldBeNil)
				})
				Convey("in both batch-level and requests", func() {
					req.Requests[0].RequestId = ""
					req.RequestId = ""
					So(validateBatchCreateTestResultsRequest(req, now), ShouldBeNil)
				})
			})
			Convey("with empty invocation in requests", func() {
				req.Requests[0].Invocation = ""
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
