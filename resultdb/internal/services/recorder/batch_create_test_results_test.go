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
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/check"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/validate"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/resultcount"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	configpb "go.chromium.org/luci/resultdb/proto/config"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestValidateBatchCreateTestResultRequest(t *testing.T) {
	t.Parallel()

	now := testclock.TestRecentTimeUTC
	ftt.Run("ValidateBatchCreateTestResultsRequest", t, func(t *ftt.Test) {
		req := validBatchCreateTestResultRequest(now, "invocations/u-build-1")
		cfg, err := config.NewCompiledServiceConfig(config.CreatePlaceHolderServiceConfig(), "revision")
		assert.NoErr(t, err)

		t.Run("succeeds", func(t *ftt.Test) {
			assert.Loosely(t, validateBatchCreateTestResultsRequest(req, cfg, now), should.BeNil)

			t.Run("with empty request_id", func(t *ftt.Test) {
				t.Run("in requests", func(t *ftt.Test) {
					req.Requests[0].RequestId = ""
					req.Requests[1].RequestId = ""
					assert.Loosely(t, validateBatchCreateTestResultsRequest(req, cfg, now), should.BeNil)
				})
				t.Run("in both batch-level and requests", func(t *ftt.Test) {
					req.Requests[0].RequestId = ""
					req.Requests[1].RequestId = ""
					req.RequestId = ""
					assert.Loosely(t, validateBatchCreateTestResultsRequest(req, cfg, now), should.BeNil)
				})
			})
			t.Run("with empty invocation in requests", func(t *ftt.Test) {
				req.Requests[0].Invocation = ""
				req.Requests[1].Invocation = ""
				assert.Loosely(t, validateBatchCreateTestResultsRequest(req, cfg, now), should.BeNil)
			})
		})

		t.Run("fails with", func(t *ftt.Test) {
			t.Run(`Too many requests`, func(t *ftt.Test) {
				req.Requests = make([]*pb.CreateTestResultRequest, 1000)
				err := validateBatchCreateTestResultsRequest(req, cfg, now)
				assert.Loosely(t, err, should.ErrLike(`the number of requests in the batch`))
				assert.Loosely(t, err, should.ErrLike(`exceeds 500`))
			})

			t.Run("invocation", func(t *ftt.Test) {
				t.Run("empty in batch-level", func(t *ftt.Test) {
					req.Invocation = ""
					err := validateBatchCreateTestResultsRequest(req, cfg, now)
					assert.Loosely(t, err, should.ErrLike("invocation: unspecified"))
				})
				t.Run("unmatched invocation in requests", func(t *ftt.Test) {
					req.Invocation = "invocations/foo"
					req.Requests[0].Invocation = "invocations/bar"
					err := validateBatchCreateTestResultsRequest(req, cfg, now)
					assert.Loosely(t, err, should.ErrLike("requests: 0: invocation must be either empty or equal"))
				})
			})

			t.Run("invalid test_result", func(t *ftt.Test) {
				req.Requests[0].TestResult.TestVariantIdentifier = nil
				err := validateBatchCreateTestResultsRequest(req, cfg, now)
				assert.Loosely(t, err, should.ErrLike("test_result: test_variant_identifier: unspecified"))
			})

			t.Run("duplicated test_results", func(t *ftt.Test) {
				req.Requests[0].TestResult.TestId = "test-id"
				req.Requests[0].TestResult.ResultId = "result-id"
				req.Requests[1].TestResult.TestId = "test-id"
				req.Requests[1].TestResult.ResultId = "result-id"
				err := validateBatchCreateTestResultsRequest(req, cfg, now)
				assert.Loosely(t, err, should.ErrLike("duplicate test results in request"))
			})

			t.Run("request_id", func(t *ftt.Test) {
				t.Run("with an invalid character", func(t *ftt.Test) {
					// non-ascii character
					req.RequestId = string(rune(244))
					err := validateBatchCreateTestResultsRequest(req, cfg, now)
					assert.Loosely(t, err, should.ErrLike("request_id: does not match"))
				})
				t.Run("empty in batch-level, but not in requests", func(t *ftt.Test) {
					req.RequestId = ""
					req.Requests[0].RequestId = "123"
					err := validateBatchCreateTestResultsRequest(req, cfg, now)
					assert.Loosely(t, err, should.ErrLike("requests: 0: request_id must be either empty or equal"))
				})
				t.Run("unmatched request_id in requests", func(t *ftt.Test) {
					req.RequestId = "foo"
					req.Requests[0].RequestId = "bar"
					err := validateBatchCreateTestResultsRequest(req, cfg, now)
					assert.Loosely(t, err, should.ErrLike("requests: 0: request_id must be either empty or equal"))
				})
			})
		})
	})
}

// validBatchCreateTestResultsRequest returns a valid BatchCreateTestResultsRequest message.
func validBatchCreateTestResultRequest(now time.Time, invName string) *pb.BatchCreateTestResultsRequest {
	tvID := &pb.TestVariantIdentifier{
		ModuleName:   "//infra/junit_tests",
		ModuleScheme: "junit",
		ModuleVariant: pbutil.Variant(
			"a/b", "1",
			"c", "2",
		),
		CoarseName: "org.chromium.go.luci",
		FineName:   "ValidationTests",
		CaseName:   "FooBar",
	}
	tr1 := validCreateTestResultRequest(now, invName, tvID)
	tr2 := validCreateTestResultRequest(now, invName, tvID)
	tr1.TestResult.ResultId = "result-id-0"
	tr2.TestResult.ResultId = "result-id-1"

	return &pb.BatchCreateTestResultsRequest{
		Invocation: invName,
		Requests:   []*pb.CreateTestResultRequest{tr1, tr2},
		RequestId:  "request-id-123",
	}
}

func TestBatchCreateTestResults(t *testing.T) {
	ftt.Run(`BatchCreateTestResults`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx) // For config in-process cache.
		ctx = memory.Use(ctx)                    // For config datastore cache.
		err := config.SetServiceConfigForTesting(ctx, config.CreatePlaceHolderServiceConfig())
		assert.NoErr(t, err)

		recorder := newTestRecorderServer()
		now := clock.Now(ctx).UTC()
		invName := "invocations/u-build-1"
		req := validBatchCreateTestResultRequest(now, invName)

		createTestResults := func(req *pb.BatchCreateTestResultsRequest, expectedTRs []*pb.TestResult, expectedCommonPrefix string) {
			response, err := recorder.BatchCreateTestResults(ctx, req)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, len(response.TestResults), should.Equal(len(expectedTRs)))
			for i, r := range req.Requests {
				expectedWireTR := proto.Clone(expectedTRs[i]).(*pb.TestResult)
				if r.TestResult.TestVariantIdentifier == nil {
					// For legacy create requests, expect the response to omit the TestVariantIdentifier.
					expectedWireTR.TestVariantIdentifier = nil
				}
				assert.Loosely(t, response.TestResults[i], should.Match(expectedWireTR))

				// double-check it with the database
				row, err := testresults.Read(span.Single(ctx), expectedTRs[i].Name)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, row, should.Match(expectedTRs[i]))

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
				assert.Loosely(t, invVars, should.Match([]string{
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
			expected := make([]*pb.TestResult, 2)
			expected[0] = proto.Clone(req.Requests[0].TestResult).(*pb.TestResult)
			expected[0].Name = "invocations/u-build-1/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/result-id-0"
			expected[0].TestVariantIdentifier.ModuleVariantHash = "c8643f74854d84b4"
			expected[0].TestId = "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar"
			expected[0].Variant = pbutil.Variant("a/b", "1", "c", "2")
			expected[0].VariantHash = "c8643f74854d84b4"

			expected[1] = proto.Clone(req.Requests[1].TestResult).(*pb.TestResult)
			expected[1].Name = "invocations/u-build-1/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/result-id-1"
			expected[1].TestVariantIdentifier.ModuleVariantHash = "c8643f74854d84b4"
			expected[1].TestId = "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar"
			expected[1].Variant = pbutil.Variant("a/b", "1", "c", "2")
			expected[1].VariantHash = "c8643f74854d84b4"

			t.Run("with a request ID", func(t *ftt.Test) {
				createTestResults(req, expected, "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar")

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
				createTestResults(req, expected, "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar")
			})
			t.Run("with uncommon test id", func(t *ftt.Test) {
				tvID := &pb.TestVariantIdentifier{
					ModuleName:   "//infra/gtest_tests",
					ModuleScheme: "gtest",
					ModuleVariant: pbutil.Variant(
						"a/b", "1",
						"c", "2",
					),
					FineName: "MySuite",
					CaseName: "TestCase",
				}
				newTr := validCreateTestResultRequest(now, invName, tvID)
				newTr.TestResult.ResultId = "result-id-2"
				req.Requests = append(req.Requests, newTr)

				expectedTR := proto.Clone(newTr.TestResult).(*pb.TestResult)
				expectedTR.Name = "invocations/u-build-1/tests/:%2F%2Finfra%2Fgtest_tests%21gtest::MySuite%23TestCase/results/result-id-2"
				expectedTR.TestVariantIdentifier.ModuleVariantHash = "c8643f74854d84b4"
				expectedTR.TestId = "://infra/gtest_tests!gtest::MySuite#TestCase"
				expectedTR.Variant = pbutil.Variant("a/b", "1", "c", "2")
				expectedTR.VariantHash = "c8643f74854d84b4"
				expected = append(expected, expectedTR)

				createTestResults(req, expected, "://infra/")
			})

			t.Run("with legacy test ID", func(t *ftt.Test) {
				req.Requests = []*pb.CreateTestResultRequest{
					legacyCreateTestResultRequest(now, invName, "some-other-test-one"),
					legacyCreateTestResultRequest(now, invName, "some-other-test-two"),
				}
				expected := make([]*pb.TestResult, 2)
				expected[0] = proto.Clone(req.Requests[0].TestResult).(*pb.TestResult)
				expected[0].Name = "invocations/u-build-1/tests/some-other-test-one/results/result-id-0"
				expected[0].TestId = "some-other-test-one"
				expected[0].TestVariantIdentifier = &pb.TestVariantIdentifier{
					ModuleName:   "legacy",
					ModuleScheme: "legacy",
					ModuleVariant: pbutil.Variant(
						"a/b", "1",
						"c", "2",
					),
					ModuleVariantHash: "c8643f74854d84b4",
					CaseName:          "some-other-test-one",
				}
				expected[0].VariantHash = pbutil.VariantHash(expected[0].Variant)
				expected[1] = proto.Clone(req.Requests[1].TestResult).(*pb.TestResult)
				expected[1].Name = "invocations/u-build-1/tests/some-other-test-two/results/result-id-0"
				expected[1].TestId = "some-other-test-two"
				expected[1].TestVariantIdentifier = &pb.TestVariantIdentifier{
					ModuleName:   "legacy",
					ModuleScheme: "legacy",
					ModuleVariant: pbutil.Variant(
						"a/b", "1",
						"c", "2",
					),
					ModuleVariantHash: "c8643f74854d84b4",
					CaseName:          "some-other-test-two",
				}
				expected[1].VariantHash = pbutil.VariantHash(expected[1].Variant)

				createTestResults(req, expected, "some-other-test-")
			})
		})

		t.Run("fails", func(t *ftt.Test) {
			t.Run("with Test ID using invalid scheme", func(t *ftt.Test) {
				req.Requests[0].TestResult.TestVariantIdentifier.ModuleScheme = "undefined"
				_, err := recorder.BatchCreateTestResults(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad request: requests: 0: test_result: test_variant_identifier: module_scheme: scheme \"undefined\" is not a known scheme"))
			})
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

func TestValidateTestResult(t *testing.T) {
	t.Parallel()
	ftt.Run(`ValidateTestResult`, t, func(t *ftt.Test) {
		now := testclock.TestRecentTimeUTC
		cfg, err := config.NewCompiledServiceConfig(config.CreatePlaceHolderServiceConfig(), "revision")
		assert.Loosely(t, err, should.BeNil)

		validateTR := func(result *pb.TestResult) error {
			return validateTestResult(now, cfg, result)
		}

		msg := validTestResult(now)
		assert.Loosely(t, validateTR(msg), should.BeNil)

		t.Run("test variant identifier", func(t *ftt.Test) {
			t.Run("unspecified", func(t *ftt.Test) {
				msg.TestVariantIdentifier = nil
				assert.Loosely(t, validateTR(msg), should.ErrLike("test_variant_identifier: unspecified"))
			})
			t.Run("structure", func(t *ftt.Test) {
				// ParseAndValidateTestID has its own extensive test cases, these do not need to be repeated here.
				t.Run("case name invalid", func(t *ftt.Test) {
					msg.TestVariantIdentifier.CaseName = "case name \x00"
					assert.Loosely(t, validateTR(msg), should.ErrLike("test_variant_identifier: case_name: non-printable rune '\\x00' at byte index 10"))
				})
				t.Run("variant invalid", func(t *ftt.Test) {
					msg.TestVariantIdentifier.ModuleVariant = pbutil.Variant("key\x00", "case name")
					assert.Loosely(t, validateTR(msg), should.ErrLike("test_variant_identifier: module_variant: \"key\\x00\":\"case name\": key: does not match pattern"))
				})
			})
			t.Run("scheme", func(t *ftt.Test) {
				// Only test a couple of cases to make sure ValidateTestIDToScheme is correctly invoked.
				// That method has its own extensive test cases, which don't need to be repeated here.
				t.Run("Scheme not defined", func(t *ftt.Test) {
					msg.TestVariantIdentifier.ModuleScheme = "undefined"
					assert.Loosely(t, validateTR(msg), should.ErrLike("test_variant_identifier: module_scheme: scheme \"undefined\" is not a known scheme by the ResultDB deployment"))
				})
				t.Run("Coarse name missing", func(t *ftt.Test) {
					msg.TestVariantIdentifier.CoarseName = ""
					assert.Loosely(t, validateTR(msg), should.ErrLike("test_variant_identifier: coarse_name: required, please set a Package (scheme \"junit\")"))
				})
			})
			t.Run("legacy", func(t *ftt.Test) {
				msg.TestVariantIdentifier = nil
				msg.TestId = "this is a test ID"
				msg.Variant = pbutil.Variant("key", "value")
				t.Run("valid", func(t *ftt.Test) {
					assert.Loosely(t, validateTR(msg), should.BeNil)
				})
				t.Run("valid with no variant", func(t *ftt.Test) {
					msg.Variant = nil
					assert.Loosely(t, validateTR(msg), should.BeNil)
				})
				t.Run("invalid Test ID scheme", func(t *ftt.Test) {
					msg.TestId = ":myModule!undefined:Package:Class#Method"
					assert.Loosely(t, validateTR(msg), should.ErrLike("test_id: module_scheme: scheme \"undefined\" is not a known scheme by the ResultDB deployment"))
				})
				t.Run("invalid test ID", func(t *ftt.Test) {
					// Uses printable unicode character 'µ'.
					msg.TestId = "this is a test ID\x00"
					assert.Loosely(t, validateTR(msg), should.ErrLike("test_id: non-printable rune '\\x00' at byte index 17"))
				})
				t.Run("invalid Variant", func(t *ftt.Test) {
					badInputs := []*pb.Variant{
						pbutil.Variant("", ""),
						pbutil.Variant("", "val"),
					}
					for _, in := range badInputs {
						msg.Variant = in
						assert.Loosely(t, validateTR(msg), should.ErrLike("key: unspecified"))
					}
				})
				t.Run("with invalid TestID", func(t *ftt.Test) {
					badInputs := []struct {
						badID  string
						errStr string
					}{
						// TestID is too long
						{strings.Repeat("1", 512+1), "longer than 512 bytes"},
						// [[:print:]] matches with [ -~] and [[:graph:]]
						{string(rune(7)), "non-printable"},
						// UTF8 text that is not in normalization form C.
						{string("cafe\u0301"), "not in unicode normalized form C"},
					}
					for _, tc := range badInputs {
						msg.TestId = tc.badID
						check.Loosely(t, validateTR(msg), should.ErrLike(tc.errStr))
					}
				})
			})
		})
		t.Run("Name", func(t *ftt.Test) {
			// ValidateTestResult should not validate TestResult.Name.
			msg.Name = "this is not a valid name for TestResult.Name"
			assert.Loosely(t, validateTR(msg), should.BeNil)
		})

		t.Run("Summary HTML", func(t *ftt.Test) {
			t.Run("with valid summary", func(t *ftt.Test) {
				msg.SummaryHtml = strings.Repeat("1", pbutil.MaxLenSummaryHTML)
				assert.Loosely(t, validateTR(msg), should.BeNil)
			})
			t.Run("with too big summary", func(t *ftt.Test) {
				msg.SummaryHtml = strings.Repeat("☕", pbutil.MaxLenSummaryHTML)
				assert.Loosely(t, validateTR(msg), should.ErrLike("summary_html: exceeds the maximum size"))
			})
		})

		t.Run("Tags", func(t *ftt.Test) {
			t.Run("with empty tags", func(t *ftt.Test) {
				msg.Tags = nil
				assert.Loosely(t, validateTR(msg), should.BeNil)
			})
			t.Run("with invalid StringPairs", func(t *ftt.Test) {
				msg.Tags = pbutil.StringPairs("", "")
				assert.Loosely(t, validateTR(msg), should.ErrLike(`"":"": key: unspecified`))
			})

		})

		t.Run("with nil", func(t *ftt.Test) {
			assert.Loosely(t, validateTR(nil), should.ErrLike(validate.Unspecified()))
		})

		t.Run("Result ID", func(t *ftt.Test) {
			t.Run("with empty ResultID", func(t *ftt.Test) {
				msg.ResultId = ""
				assert.Loosely(t, validateTR(msg), should.ErrLike("result_id: unspecified"))
			})

			t.Run("with invalid ResultID", func(t *ftt.Test) {
				badInputs := []string{
					strings.Repeat("1", 32+1),
					string(rune(7)),
				}
				for _, in := range badInputs {
					msg.ResultId = in
					assert.Loosely(t, validateTR(msg), should.ErrLike("result_id: does not match pattern \"^[a-z0-9\\\\-_.]{1,32}$\""))
				}
			})
		})
		t.Run("Status", func(t *ftt.Test) {
			t.Run("with invalid Status", func(t *ftt.Test) {
				msg.Status = pb.TestStatus(len(pb.TestStatus_name) + 1)
				assert.Loosely(t, validateTR(msg), should.ErrLike("status: invalid value"))
			})
			t.Run("with STATUS_UNSPECIFIED", func(t *ftt.Test) {
				msg.Status = pb.TestStatus_STATUS_UNSPECIFIED
				assert.Loosely(t, validateTR(msg), should.ErrLike("status: cannot be STATUS_UNSPECIFIED"))
			})
		})
		t.Run("Skip Reason", func(t *ftt.Test) {
			t.Run("valid", func(t *ftt.Test) {
				msg.Status = pb.TestStatus_SKIP
				msg.SkipReason = pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS
				assert.Loosely(t, validateTR(msg), should.BeNil)
			})
			t.Run("with skip reason but not skip status", func(t *ftt.Test) {
				msg.Status = pb.TestStatus_ABORT
				msg.SkipReason = pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS
				assert.Loosely(t, validateTR(msg), should.ErrLike("skip_reason: value must be zero (UNSPECIFIED) when status is not SKIP"))
			})
		})
		t.Run("StartTime and Duration", func(t *ftt.Test) {
			// Valid cases.
			t.Run("with nil start_time", func(t *ftt.Test) {
				msg.StartTime = nil
				assert.Loosely(t, validateTR(msg), should.BeNil)
			})
			t.Run("with nil duration", func(t *ftt.Test) {
				msg.Duration = nil
				assert.Loosely(t, validateTR(msg), should.BeNil)
			})
			// Invalid cases.
			t.Run("because start_time is in the future", func(t *ftt.Test) {
				future := timestamppb.New(now.Add(time.Hour))
				msg.StartTime = future
				assert.Loosely(t, validateTR(msg), should.ErrLike(fmt.Sprintf("start_time: cannot be > (now + %s)", pbutil.MaxClockSkew)))
			})

			t.Run("because duration is < 0", func(t *ftt.Test) {
				msg.Duration = durationpb.New(-1 * time.Minute)
				assert.Loosely(t, validateTR(msg), should.ErrLike("duration: is < 0"))
			})

			t.Run("because (start_time + duration) is in the future", func(t *ftt.Test) {
				st := timestamppb.New(now.Add(-1 * time.Hour))
				msg.StartTime = st
				msg.Duration = durationpb.New(2 * time.Hour)
				expected := fmt.Sprintf("start_time + duration: cannot be > (now + %s)", pbutil.MaxClockSkew)
				assert.Loosely(t, validateTR(msg), should.ErrLike(expected))
			})
		})

		t.Run("Test metadata", func(t *ftt.Test) {
			t.Run("no location and no bug component", func(t *ftt.Test) {
				msg.TestMetadata = &pb.TestMetadata{Name: "name"}
				assert.Loosely(t, validateTR(msg), should.BeNil)
			})
			t.Run("Location", func(t *ftt.Test) {
				msg.TestMetadata.Location = &pb.TestLocation{
					Repo:     "https://git.example.com",
					FileName: "//a_test.go",
					Line:     54,
				}
				t.Run("unspecified", func(t *ftt.Test) {
					msg.TestMetadata.Location = nil
					assert.Loosely(t, validateTR(msg), should.BeNil)
				})
				t.Run("filename", func(t *ftt.Test) {
					t.Run("unspecified", func(t *ftt.Test) {
						msg.TestMetadata.Location.FileName = ""
						assert.Loosely(t, validateTR(msg), should.ErrLike("test_metadata: location: file_name: unspecified"))
					})
					t.Run("too long", func(t *ftt.Test) {
						msg.TestMetadata.Location.FileName = "//" + strings.Repeat("super long", 100)
						assert.Loosely(t, validateTR(msg), should.ErrLike("test_metadata: location: file_name: length exceeds 512"))
					})
					t.Run("no double slashes", func(t *ftt.Test) {
						msg.TestMetadata.Location.FileName = "file_name"
						assert.Loosely(t, validateTR(msg), should.ErrLike("test_metadata: location: file_name: doesn't start with //"))
					})
					t.Run("back slash", func(t *ftt.Test) {
						msg.TestMetadata.Location.FileName = "//dir\\file"
						assert.Loosely(t, validateTR(msg), should.ErrLike("test_metadata: location: file_name: has \\"))
					})
					t.Run("trailing slash", func(t *ftt.Test) {
						msg.TestMetadata.Location.FileName = "//file_name/"
						assert.Loosely(t, validateTR(msg), should.ErrLike("test_metadata: location: file_name: ends with /"))
					})
				})
				t.Run("line", func(t *ftt.Test) {
					msg.TestMetadata.Location.Line = -1
					assert.Loosely(t, validateTR(msg), should.ErrLike("test_metadata: location: line: must not be negative"))
				})
				t.Run("repo", func(t *ftt.Test) {
					t.Run("invalid", func(t *ftt.Test) {
						msg.TestMetadata.Location.Repo = "https://chromium.googlesource.com/chromium/src.git"
						assert.Loosely(t, validateTR(msg), should.ErrLike("test_metadata: location: repo: must not end with .git"))
					})
					t.Run("unspecified", func(t *ftt.Test) {
						msg.TestMetadata.Location.Repo = ""
						assert.Loosely(t, validateTR(msg), should.ErrLike("test_metadata: location: repo: required"))
					})
				})
			})
			t.Run("Bug component", func(t *ftt.Test) {

				t.Run("nil bug system in bug component", func(t *ftt.Test) {
					msg.TestMetadata = &pb.TestMetadata{
						Name: "name",
						BugComponent: &pb.BugComponent{
							System: nil,
						},
					}
					assert.Loosely(t, validateTR(msg), should.ErrLike("bug system is required for bug components"))
				})
				t.Run("valid monorail bug component", func(t *ftt.Test) {
					msg.TestMetadata = &pb.TestMetadata{
						Name: "name",
						BugComponent: &pb.BugComponent{
							System: &pb.BugComponent_Monorail{
								Monorail: &pb.MonorailComponent{
									Project: "1chromium1",
									Value:   "Component>Value",
								},
							},
						},
					}
					assert.Loosely(t, validateTR(msg), should.BeNil)
				})
				t.Run("wrong size monorail bug component value", func(t *ftt.Test) {
					msg.TestMetadata = &pb.TestMetadata{
						Name: "name",
						BugComponent: &pb.BugComponent{
							System: &pb.BugComponent_Monorail{
								Monorail: &pb.MonorailComponent{
									Project: "chromium",
									Value:   strings.Repeat("a", 601),
								},
							},
						},
					}
					assert.Loosely(t, validateTR(msg), should.ErrLike("monorail.value: is invalid"))
				})
				t.Run("invalid monorail bug component value", func(t *ftt.Test) {
					msg.TestMetadata = &pb.TestMetadata{
						Name: "name",
						BugComponent: &pb.BugComponent{
							System: &pb.BugComponent_Monorail{
								Monorail: &pb.MonorailComponent{
									Project: "chromium",
									Value:   "Component<><>Value",
								},
							},
						},
					}
					assert.Loosely(t, validateTR(msg), should.ErrLike("monorail.value: is invalid"))
				})
				t.Run("wrong size monorail bug component project", func(t *ftt.Test) {
					msg.TestMetadata = &pb.TestMetadata{
						Name: "name",
						BugComponent: &pb.BugComponent{
							System: &pb.BugComponent_Monorail{
								Monorail: &pb.MonorailComponent{
									Project: strings.Repeat("a", 64),
									Value:   "Component>Value",
								},
							},
						},
					}
					assert.Loosely(t, validateTR(msg), should.ErrLike("monorail.project: is invalid"))
				})
				t.Run("using invalid characters in monorail bug component project", func(t *ftt.Test) {
					msg.TestMetadata = &pb.TestMetadata{
						Name: "name",
						BugComponent: &pb.BugComponent{
							System: &pb.BugComponent_Monorail{
								Monorail: &pb.MonorailComponent{
									Project: "$%^ $$^%",
									Value:   "Component>Value",
								},
							},
						},
					}
					assert.Loosely(t, validateTR(msg), should.ErrLike("monorail.project: is invalid"))
				})
				t.Run("using only numbers in monorail bug component project", func(t *ftt.Test) {
					msg.TestMetadata = &pb.TestMetadata{
						Name: "name",
						BugComponent: &pb.BugComponent{
							System: &pb.BugComponent_Monorail{
								Monorail: &pb.MonorailComponent{
									Project: "11111",
									Value:   "Component>Value",
								},
							},
						},
					}
					assert.Loosely(t, validateTR(msg), should.ErrLike("monorail.project: is invalid"))
				})
				t.Run("valid buganizer component", func(t *ftt.Test) {
					msg.TestMetadata = &pb.TestMetadata{
						Name: "name",
						BugComponent: &pb.BugComponent{
							System: &pb.BugComponent_IssueTracker{
								IssueTracker: &pb.IssueTrackerComponent{
									ComponentId: 1234,
								},
							},
						},
					}
					assert.Loosely(t, validateTR(msg), should.BeNil)
				})
				t.Run("invalid buganizer component id", func(t *ftt.Test) {
					msg.TestMetadata = &pb.TestMetadata{
						Name: "name",
						BugComponent: &pb.BugComponent{
							System: &pb.BugComponent_IssueTracker{
								IssueTracker: &pb.IssueTrackerComponent{
									ComponentId: -1,
								},
							},
						},
					}
					assert.Loosely(t, validateTR(msg), should.ErrLike("issue_tracker.component_id: is invalid"))
				})
			})
			t.Run("Properties", func(t *ftt.Test) {
				t.Run("with valid properties", func(t *ftt.Test) {
					msg.Properties = &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key": structpb.NewStringValue("value"),
						},
					}
					assert.Loosely(t, validateTR(msg), should.BeNil)
				})
				t.Run("with too big properties", func(t *ftt.Test) {
					msg.TestMetadata = &pb.TestMetadata{
						PropertiesSchema: "package.message",
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"key": structpb.NewStringValue(strings.Repeat("1", pbutil.MaxSizeTestMetadataProperties)),
							},
						},
					}
					assert.Loosely(t, validateTR(msg), should.ErrLike("exceeds the maximum size"))
				})
				t.Run("no properties_schema with non-empty properties", func(t *ftt.Test) {
					msg.TestMetadata = &pb.TestMetadata{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"key": structpb.NewStringValue("1"),
							},
						},
					}
					assert.Loosely(t, validateTR(msg), should.ErrLike("properties_schema must be specified with non-empty properties"))
				})
				t.Run("invalid properties_schema", func(t *ftt.Test) {
					msg.TestMetadata = &pb.TestMetadata{
						PropertiesSchema: "package",
					}
					assert.Loosely(t, validateTR(msg), should.ErrLike("properties_schema: does not match"))
				})
				t.Run("valid properties_schema and non-empty properties", func(t *ftt.Test) {
					msg.TestMetadata = &pb.TestMetadata{
						PropertiesSchema: "package.message",
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"key": structpb.NewStringValue("1"),
							},
						},
					}
					assert.Loosely(t, validateTR(msg), should.BeNil)
				})
			})
		})
		t.Run("Failure reason", func(t *ftt.Test) {
			errorMessage1 := "error1"
			errorMessage2 := "error2"
			longErrorMessage := strings.Repeat("a very long error message", 100)
			t.Run("valid failure reason", func(t *ftt.Test) {
				msg.FailureReason = &pb.FailureReason{
					PrimaryErrorMessage: errorMessage1,
					Errors: []*pb.FailureReason_Error{
						{Message: errorMessage1},
						{Message: errorMessage2},
					},
					TruncatedErrorsCount: 0,
				}
				assert.Loosely(t, validateTR(msg), should.BeNil)
			})

			t.Run("primary_error_message exceeds the maximum limit", func(t *ftt.Test) {
				msg.FailureReason = &pb.FailureReason{
					PrimaryErrorMessage: longErrorMessage,
				}
				assert.Loosely(t, validateTR(msg), should.ErrLike("primary_error_message: "+
					"exceeds the maximum"))
			})

			t.Run("one of the error messages exceeds the maximum limit", func(t *ftt.Test) {
				msg.FailureReason = &pb.FailureReason{
					PrimaryErrorMessage: errorMessage1,
					Errors: []*pb.FailureReason_Error{
						{Message: errorMessage1},
						{Message: longErrorMessage},
					},
					TruncatedErrorsCount: 0,
				}
				assert.Loosely(t, validateTR(msg), should.ErrLike(
					"errors[1]: message: exceeds the maximum size of 1024 "+
						"bytes"))
			})

			t.Run("the first error doesn't match primary_error_message", func(t *ftt.Test) {
				msg.FailureReason = &pb.FailureReason{
					PrimaryErrorMessage: errorMessage1,
					Errors: []*pb.FailureReason_Error{
						{Message: errorMessage2},
					},
					TruncatedErrorsCount: 0,
				}
				assert.Loosely(t, validateTR(msg), should.ErrLike(
					"errors[0]: message: must match primary_error_message"))
			})

			t.Run("the total size of the errors list exceeds the limit", func(t *ftt.Test) {
				maxErrorMessage := strings.Repeat(".", 1024)
				msg.FailureReason = &pb.FailureReason{
					PrimaryErrorMessage: maxErrorMessage,
					Errors: []*pb.FailureReason_Error{
						{Message: maxErrorMessage},
						{Message: maxErrorMessage},
						{Message: maxErrorMessage},
						{Message: maxErrorMessage},
					},
					TruncatedErrorsCount: 1,
				}
				assert.Loosely(t, validateTR(msg), should.ErrLike(
					"errors: exceeds the maximum total size of 3172 bytes"))
			})

			t.Run("invalid truncated error count", func(t *ftt.Test) {
				msg.FailureReason = &pb.FailureReason{
					PrimaryErrorMessage: errorMessage1,
					Errors: []*pb.FailureReason_Error{
						{Message: errorMessage1},
						{Message: errorMessage2},
					},
					TruncatedErrorsCount: -1,
				}
				assert.Loosely(t, validateTR(msg), should.ErrLike("truncated_errors_count: "+
					"must be non-negative"))
			})
		})
	})
}

func TestValidateTestIDToScheme(t *testing.T) {
	t.Parallel()
	ftt.Run(`ValidateTestIDToScheme`, t, func(t *ftt.Test) {
		referenceConfig := &configpb.Config{
			Schemes: []*configpb.Scheme{
				{
					Id:                "junit",
					HumanReadableName: "JUnit",
					Coarse: &configpb.Scheme_Level{
						ValidationRegexp:  "[a-z][a-z_0-9.]+",
						HumanReadableName: "Package",
					},
					Fine: &configpb.Scheme_Level{
						ValidationRegexp:  "[a-zA-Z_][a-zA-Z_0-9]+",
						HumanReadableName: "Class",
					},
					Case: &configpb.Scheme_Level{
						ValidationRegexp:  "[a-zA-Z_][a-zA-Z_0-9]+",
						HumanReadableName: "Method",
					},
				},
				{
					Id:                "basic",
					HumanReadableName: "Basic",
					Case: &configpb.Scheme_Level{
						HumanReadableName: "Method",
					},
				},
			},
		}
		cfg, err := config.NewCompiledServiceConfig(referenceConfig, "revision")
		assert.NoErr(t, err)

		testID := pbutil.TestIdentifier{
			ModuleName:   "myModule",
			ModuleScheme: "junit",
			CoarseName:   "com.example.package",
			FineName:     "ExampleClass",
			CaseName:     "testMethod",
		}

		t.Run("valid", func(t *ftt.Test) {
			assert.Loosely(t, validateTestIDToScheme(cfg, testID), should.BeNil)
		})
		t.Run("Scheme not defined", func(t *ftt.Test) {
			testID.ModuleScheme = "undefined"
			assert.Loosely(t, validateTestIDToScheme(cfg, testID), should.ErrLike("module_scheme: scheme \"undefined\" is not a known scheme by the ResultDB deployment"))
		})
		t.Run("Coarse Name", func(t *ftt.Test) {
			t.Run("missing", func(t *ftt.Test) {
				testID.CoarseName = ""
				assert.Loosely(t, validateTestIDToScheme(cfg, testID), should.ErrLike("coarse_name: required, please set a Package (scheme \"junit\")"))
			})
			t.Run("invalid", func(t *ftt.Test) {
				testID.CoarseName = "1com.example.package"
				assert.Loosely(t, validateTestIDToScheme(cfg, testID), should.ErrLike("coarse_name: does not match validation regexp \"^[a-z][a-z_0-9.]+$\", please set a valid Package (scheme \"junit\")"))
			})
			t.Run("set when not expected", func(t *ftt.Test) {
				testID.ModuleScheme = "basic"
				testID.CoarseName = "value"
				assert.Loosely(t, validateTestIDToScheme(cfg, testID), should.ErrLike("coarse_name: expected empty value (level is not defined by scheme \"basic\")"))
			})
		})
		t.Run("Fine Name", func(t *ftt.Test) {
			t.Run("missing", func(t *ftt.Test) {
				testID.FineName = ""
				assert.Loosely(t, validateTestIDToScheme(cfg, testID), should.ErrLike("fine_name: required, please set a Class (scheme \"junit\")"))
			})
			t.Run("invalid", func(t *ftt.Test) {
				testID.FineName = "1com.example.package"
				assert.Loosely(t, validateTestIDToScheme(cfg, testID), should.ErrLike("fine_name: does not match validation regexp \"^[a-zA-Z_][a-zA-Z_0-9]+$\", please set a valid Class (scheme \"junit\")"))
			})
			t.Run("set when not expected", func(t *ftt.Test) {
				testID.ModuleScheme = "basic"
				testID.CoarseName = ""
				testID.FineName = "value"
				assert.Loosely(t, validateTestIDToScheme(cfg, testID), should.ErrLike("fine_name: expected empty value (level is not defined by scheme \"basic\")"))
			})
		})
		t.Run("Case Name", func(t *ftt.Test) {
			t.Run("missing", func(t *ftt.Test) {
				testID.CaseName = ""
				assert.Loosely(t, validateTestIDToScheme(cfg, testID), should.ErrLike("case_name: required, please set a Method (scheme \"junit\")"))
			})
			t.Run("invalid", func(t *ftt.Test) {
				testID.CaseName = "1method"
				assert.Loosely(t, validateTestIDToScheme(cfg, testID), should.ErrLike("case_name: does not match validation regexp \"^[a-zA-Z_][a-zA-Z_0-9]+$\", please set a valid Method (scheme \"junit\")"))
			})
		})
	})
}

// validTestResult returns a valid TestResult sample.
func validTestResult(now time.Time) *pb.TestResult {
	st := timestamppb.New(now.Add(-2 * time.Minute))
	return &pb.TestResult{
		ResultId: "result_id1",
		TestVariantIdentifier: &pb.TestVariantIdentifier{
			ModuleName:    "//infra/java_tests",
			ModuleVariant: pbutil.Variant("a", "b"),
			ModuleScheme:  "junit",
			CoarseName:    "org.chromium.go.luci",
			FineName:      "ValidationTests",
			CaseName:      "FooBar",
		},
		Expected:    true,
		Status:      pb.TestStatus_PASS,
		SummaryHtml: "HTML summary",
		StartTime:   st,
		Duration:    durationpb.New(time.Minute),
		TestMetadata: &pb.TestMetadata{
			Location: &pb.TestLocation{
				Repo:     "https://git.example.com",
				FileName: "//a_test.go",
				Line:     54,
			},
			BugComponent: &pb.BugComponent{
				System: &pb.BugComponent_Monorail{
					Monorail: &pb.MonorailComponent{
						Project: "chromium",
						Value:   "Component>Value",
					},
				},
			},
		},
		Tags: pbutil.StringPairs("k1", "v1"),
	}
}
