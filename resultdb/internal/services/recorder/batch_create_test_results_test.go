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
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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
				req.Requests[0].TestResult.TestIdStructured = nil
				err := validateBatchCreateTestResultsRequest(req, cfg, now)
				assert.Loosely(t, err, should.ErrLike("test_result: test_id_structured: unspecified"))
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
	tvID := &pb.TestIdentifier{
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
				if r.TestResult.TestIdStructured == nil {
					// For legacy create requests, expect the response to omit the TestVariantIdentifier.
					expectedWireTR.TestIdStructured = nil
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
			expected[0].TestIdStructured.ModuleVariantHash = "c8643f74854d84b4"
			expected[0].TestId = "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar"
			expected[0].Variant = pbutil.Variant("a/b", "1", "c", "2")
			expected[0].VariantHash = "c8643f74854d84b4"

			expected[1] = proto.Clone(req.Requests[1].TestResult).(*pb.TestResult)
			expected[1].Name = "invocations/u-build-1/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/result-id-1"
			expected[1].TestIdStructured.ModuleVariantHash = "c8643f74854d84b4"
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
				tvID := &pb.TestIdentifier{
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
				expectedTR.TestIdStructured.ModuleVariantHash = "c8643f74854d84b4"
				expectedTR.TestId = "://infra/gtest_tests!gtest::MySuite#TestCase"
				expectedTR.Variant = pbutil.Variant("a/b", "1", "c", "2")
				expectedTR.VariantHash = "c8643f74854d84b4"
				expected = append(expected, expectedTR)

				createTestResults(req, expected, "://infra/")
			})
		})

		t.Run("fails", func(t *ftt.Test) {
			t.Run("with Test ID using invalid scheme", func(t *ftt.Test) {
				req.Requests[0].TestResult.TestIdStructured.ModuleScheme = "undefined"
				_, err := recorder.BatchCreateTestResults(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad request: requests: 0: test_result: test_id_structured: module_scheme: scheme \"undefined\" is not a known scheme"))
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

		t.Run("with legacy-form test result", func(t *ftt.Test) {
			req.Requests = []*pb.CreateTestResultRequest{
				legacyCreateTestResultRequest(now, invName, "some-other-test-one"),
				legacyCreateTestResultRequest(now, invName, "some-other-test-two"),
			}
			expected := make([]*pb.TestResult, 2)
			expected[0] = proto.Clone(req.Requests[0].TestResult).(*pb.TestResult)
			expected[0].Name = "invocations/u-build-1/tests/some-other-test-one/results/result-id-0"
			expected[0].TestId = "some-other-test-one"
			expected[0].TestIdStructured = &pb.TestIdentifier{
				ModuleName:   "legacy",
				ModuleScheme: "legacy",
				ModuleVariant: pbutil.Variant(
					"a/b", "1",
					"c", "2",
				),
				ModuleVariantHash: "c8643f74854d84b4",
				CaseName:          "some-other-test-one",
			}
			expected[0].FailureReason = &pb.FailureReason{
				PrimaryErrorMessage: "got 1, want 0",
				Errors: []*pb.FailureReason_Error{
					{Message: "got 1, want 0"},
				},
			}
			expected[0].VariantHash = pbutil.VariantHash(expected[0].Variant)

			expected[1] = proto.Clone(req.Requests[1].TestResult).(*pb.TestResult)
			expected[1].Name = "invocations/u-build-1/tests/some-other-test-two/results/result-id-0"
			expected[1].TestId = "some-other-test-two"
			expected[1].TestIdStructured = &pb.TestIdentifier{
				ModuleName:   "legacy",
				ModuleScheme: "legacy",
				ModuleVariant: pbutil.Variant(
					"a/b", "1",
					"c", "2",
				),
				ModuleVariantHash: "c8643f74854d84b4",
				CaseName:          "some-other-test-two",
			}
			expected[1].FailureReason = &pb.FailureReason{
				PrimaryErrorMessage: "got 1, want 0",
				Errors: []*pb.FailureReason_Error{
					{Message: "got 1, want 0"},
				},
			}
			expected[1].VariantHash = pbutil.VariantHash(expected[1].Variant)

			t.Run("base case", func(t *ftt.Test) {
				createTestResults(req, expected, "some-other-test-")
			})
			t.Run("failure reason kind is set based on v1 status", func(t *ftt.Test) {
				req.Requests[0].TestResult.Status = pb.TestStatus_CRASH
				expected[0].Status = pb.TestStatus_CRASH
				expected[0].FailureReason.Kind = pb.FailureReason_CRASH

				createTestResults(req, expected, "some-other-test-")
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
	ftt.Run(`validateTestResult`, t, func(t *ftt.Test) {
		now := testclock.TestRecentTimeUTC
		cfg, err := config.NewCompiledServiceConfig(config.CreatePlaceHolderServiceConfig(), "revision")
		assert.Loosely(t, err, should.BeNil)

		validateTR := func(result *pb.TestResult) error {
			return validateTestResult(now, cfg, result)
		}

		msg := validTestResult(now)
		assert.Loosely(t, validateTR(msg), should.BeNil)

		t.Run("valid", func(t *ftt.Test) {
			assert.Loosely(t, validateTR(msg), should.BeNil)
		})
		t.Run("invalid", func(t *ftt.Test) {
			// Test that pbutil.ValidateTestResult is being called, but do not test all error cases
			// as it has its own tests.
			msg.TestIdStructured = nil
			assert.Loosely(t, validateTR(msg), should.ErrLike("test_id_structured: unspecified"))
		})
		t.Run("test ID scheme", func(t *ftt.Test) {
			// Only test a couple of cases to make sure validateTestIDToScheme is correctly invoked.
			// That method has its own extensive test cases, which don't need to be repeated here.
			t.Run("Scheme not defined", func(t *ftt.Test) {
				msg.TestIdStructured.ModuleScheme = "undefined"
				assert.Loosely(t, validateTR(msg), should.ErrLike("test_id_structured: module_scheme: scheme \"undefined\" is not a known scheme by the ResultDB deployment"))
			})
			t.Run("Coarse name missing", func(t *ftt.Test) {
				msg.TestIdStructured.CoarseName = ""
				assert.Loosely(t, validateTR(msg), should.ErrLike("test_id_structured: coarse_name: required, please set a Package (scheme \"junit\")"))
			})
		})
		t.Run("Name", func(t *ftt.Test) {
			// validateTestResult should not validate TestResult.Name.
			msg.Name = "this is not a valid name for TestResult.Name"
			assert.Loosely(t, validateTR(msg), should.BeNil)
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
						ValidationRegexp:  "^[a-z][a-z_0-9.]+$",
						HumanReadableName: "Package",
					},
					Fine: &configpb.Scheme_Level{
						ValidationRegexp:  "^[a-zA-Z_][a-zA-Z_0-9]+$",
						HumanReadableName: "Class",
					},
					Case: &configpb.Scheme_Level{
						ValidationRegexp:  "^[a-zA-Z_][a-zA-Z_0-9]+$",
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

		testID := pbutil.BaseTestIdentifier{
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
		t.Run("Invalid with respect to scheme", func(t *ftt.Test) {
			// We do not need to test all ways a test result could be invalid,
			// Scheme.Validate has plenty of test cases already. We just need to
			// check that method is invoked.

			testID.CoarseName = ""
			assert.Loosely(t, validateTestIDToScheme(cfg, testID), should.ErrLike("coarse_name: required, please set a Package (scheme \"junit\")"))
		})
	})
}

// validTestResult returns a valid TestResult sample.
func validTestResult(now time.Time) *pb.TestResult {
	st := timestamppb.New(now.Add(-2 * time.Minute))
	return &pb.TestResult{
		ResultId: "result_id1",
		TestIdStructured: &pb.TestIdentifier{
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
