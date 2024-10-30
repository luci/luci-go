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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/resultcount"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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
	ftt.Run("ValidateBatchCreateTestResultsRequest", t, func(t *ftt.Test) {
		req := validBatchCreateTestResultRequest(now, "invocations/u-build-1", "test-id")

		t.Run("succeeds", func(t *ftt.Test) {
			assert.Loosely(t, validateBatchCreateTestResultsRequest(req, now), should.BeNil)

			t.Run("with empty request_id", func(t *ftt.Test) {
				t.Run("in requests", func(t *ftt.Test) {
					req.Requests[0].RequestId = ""
					req.Requests[1].RequestId = ""
					assert.Loosely(t, validateBatchCreateTestResultsRequest(req, now), should.BeNil)
				})
				t.Run("in both batch-level and requests", func(t *ftt.Test) {
					req.Requests[0].RequestId = ""
					req.Requests[1].RequestId = ""
					req.RequestId = ""
					assert.Loosely(t, validateBatchCreateTestResultsRequest(req, now), should.BeNil)
				})
			})
			t.Run("with empty invocation in requests", func(t *ftt.Test) {
				req.Requests[0].Invocation = ""
				req.Requests[1].Invocation = ""
				assert.Loosely(t, validateBatchCreateTestResultsRequest(req, now), should.BeNil)
			})
		})

		t.Run("fails with", func(t *ftt.Test) {
			t.Run(`Too many requests`, func(t *ftt.Test) {
				req.Requests = make([]*pb.CreateTestResultRequest, 1000)
				err := validateBatchCreateTestResultsRequest(req, now)
				assert.Loosely(t, err, should.ErrLike(`the number of requests in the batch exceeds 500`))
			})

			t.Run("invocation", func(t *ftt.Test) {
				t.Run("empty in batch-level", func(t *ftt.Test) {
					req.Invocation = ""
					err := validateBatchCreateTestResultsRequest(req, now)
					assert.Loosely(t, err, should.ErrLike("invocation: unspecified"))
				})
				t.Run("unmatched invocation in requests", func(t *ftt.Test) {
					req.Invocation = "invocations/foo"
					req.Requests[0].Invocation = "invocations/bar"
					err := validateBatchCreateTestResultsRequest(req, now)
					assert.Loosely(t, err, should.ErrLike("requests: 0: invocation must be either empty or equal"))
				})
			})

			t.Run("invalid test_result", func(t *ftt.Test) {
				req.Requests[0].TestResult.TestId = ""
				err := validateBatchCreateTestResultsRequest(req, now)
				assert.Loosely(t, err, should.ErrLike("test_result: test_id: unspecified"))
			})

			t.Run("duplicated test_results", func(t *ftt.Test) {
				req.Requests[0].TestResult.TestId = "test-id"
				req.Requests[0].TestResult.ResultId = "result-id"
				req.Requests[1].TestResult.TestId = "test-id"
				req.Requests[1].TestResult.ResultId = "result-id"
				err := validateBatchCreateTestResultsRequest(req, now)
				assert.Loosely(t, err, should.ErrLike("duplicate test results in request"))
			})

			t.Run("request_id", func(t *ftt.Test) {
				t.Run("with an invalid character", func(t *ftt.Test) {
					// non-ascii character
					req.RequestId = string(rune(244))
					err := validateBatchCreateTestResultsRequest(req, now)
					assert.Loosely(t, err, should.ErrLike("request_id: does not match"))
				})
				t.Run("empty in batch-level, but not in requests", func(t *ftt.Test) {
					req.RequestId = ""
					req.Requests[0].RequestId = "123"
					err := validateBatchCreateTestResultsRequest(req, now)
					assert.Loosely(t, err, should.ErrLike("requests: 0: request_id must be either empty or equal"))
				})
				t.Run("unmatched request_id in requests", func(t *ftt.Test) {
					req.RequestId = "foo"
					req.Requests[0].RequestId = "bar"
					err := validateBatchCreateTestResultsRequest(req, now)
					assert.Loosely(t, err, should.ErrLike("requests: 0: request_id must be either empty or equal"))
				})
			})
		})
	})
}

func TestBatchCreateTestResults(t *testing.T) {
	ftt.Run(`BatchCreateTestResults`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		recorder := newTestRecorderServer()
		now := clock.Now(ctx).UTC()
		invName := "invocations/u-build-1"
		req := validBatchCreateTestResultRequest(
			now, invName, "test-id",
		)

		createTestResults := func(req *pb.BatchCreateTestResultsRequest, expectedCommonPrefix string) {
			response, err := recorder.BatchCreateTestResults(ctx, req)
			assert.Loosely(t, err, should.BeNil)

			for i, r := range req.Requests {
				expected := proto.Clone(r.TestResult).(*pb.TestResult)
				expected.Name = fmt.Sprintf("invocations/u-build-1/tests/%s/results/result-id-%d", expected.TestId, i)
				expected.VariantHash = pbutil.VariantHash(expected.Variant)
				assert.Loosely(t, response.TestResults[i], should.Resemble(expected))

				// double-check it with the database
				row, err := testresults.Read(span.Single(ctx), expected.Name)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, row, should.Resemble(expected))

				var invCommonTestIDPrefix string
				var invVars []string
				err = invocations.ReadColumns(
					span.Single(ctx), invocations.ID("u-build-1"),
					map[string]any{
						"CommonTestIDPrefix":     &invCommonTestIDPrefix,
						"TestResultVariantUnion": &invVars,
					})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, invCommonTestIDPrefix, should.Equal(expectedCommonPrefix))
				assert.Loosely(t, invVars, should.Resemble([]string{
					"a/b:1",
					"c:2",
				}))
			}
		}

		// Insert a sample invocation
		tok, err := generateInvocationToken(ctx, "u-build-1")
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, tok))
		invID := invocations.ID("u-build-1")
		testutil.MustApply(ctx, t, insert.Invocation(invID, pb.Invocation_ACTIVE, nil))

		t.Run("succeeds", func(t *ftt.Test) {
			t.Run("with a request ID", func(t *ftt.Test) {
				createTestResults(req, "test-id")

				ctx, cancel := span.ReadOnlyTransaction(ctx)
				defer cancel()
				trNum, err := resultcount.ReadTestResultCount(ctx, invocations.NewIDSet(invID))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, trNum, should.Equal(2))
			})

			t.Run("without a request ID", func(t *ftt.Test) {
				req.RequestId = ""
				req.Requests[0].RequestId = ""
				req.Requests[1].RequestId = ""
				createTestResults(req, "test-id")
			})

			t.Run("with uncommon test id", func(t *ftt.Test) {
				newTr := validCreateTestResultRequest(now, invName, "some-other-test-id")
				newTr.TestResult.ResultId = "result-id-2"
				req.Requests = append(req.Requests, newTr)
				createTestResults(req, "")
			})
		})

		t.Run("fails", func(t *ftt.Test) {
			t.Run("with an invalid request", func(t *ftt.Test) {
				req.Invocation = "this is an invalid invocation name"
				req.Requests[0].Invocation = ""
				req.Requests[1].Invocation = ""
				_, err := recorder.BatchCreateTestResults(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad request: invocation: does not match"))
			})

			t.Run("with an non-existing invocation", func(t *ftt.Test) {
				tok, err = generateInvocationToken(ctx, "inv")
				assert.Loosely(t, err, should.BeNil)
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, tok))
				req.Invocation = "invocations/inv"
				req.Requests[0].Invocation = ""
				req.Requests[1].Invocation = ""
				_, err := recorder.BatchCreateTestResults(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("invocations/inv not found"))
			})
		})
	})
}

func TestLongestCommonPrefix(t *testing.T) {
	t.Parallel()
	ftt.Run("empty", t, func(t *ftt.Test) {
		assert.Loosely(t, longestCommonPrefix("", "str"), should.BeEmpty)
	})

	ftt.Run("no common prefix", t, func(t *ftt.Test) {
		assert.Loosely(t, longestCommonPrefix("str", "other"), should.BeEmpty)
	})

	ftt.Run("common prefix", t, func(t *ftt.Test) {
		assert.Loosely(t, longestCommonPrefix("prefix_1", "prefix_2"), should.Equal("prefix_"))
	})
}
