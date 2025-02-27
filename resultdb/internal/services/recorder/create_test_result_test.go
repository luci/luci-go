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
	"google.golang.org/protobuf/types/known/structpb"

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
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// validCreateTestResultRequest returns a valid CreateTestResultRequest message.
func validCreateTestResultRequest(now time.Time, invName string, testVariantID *pb.TestVariantIdentifier) *pb.CreateTestResultRequest {
	properties, err := structpb.NewStruct(map[string]any{
		"key_1": "value_1",
		"key_2": map[string]any{
			"child_key": 1,
		},
	})
	if err != nil {
		// Should never fail.
		panic(err)
	}

	return &pb.CreateTestResultRequest{
		Invocation: invName,
		RequestId:  "request-id-123",

		TestResult: &pb.TestResult{
			TestVariantIdentifier: testVariantID,
			ResultId:              "result-id-0",
			Expected:              true,
			Status:                pb.TestStatus_PASS,
			TestMetadata: &pb.TestMetadata{
				Name: "original_name",
				Location: &pb.TestLocation{
					Repo:     "https://chromium.googlesource.com/chromium/src",
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
			FailureReason: &pb.FailureReason{
				PrimaryErrorMessage: "got 1, want 0",
				Errors: []*pb.FailureReason_Error{
					{Message: "got 1, want 0"},
					{Message: "got 2, want 1"},
				},
				TruncatedErrorsCount: 0,
			},
			Properties: properties,
		},
	}
}

func legacyCreateTestResultRequest(now time.Time, invName, testID string) *pb.CreateTestResultRequest {
	properties, err := structpb.NewStruct(map[string]any{
		"key_1": "value_1",
		"key_2": map[string]any{
			"child_key": 1,
		},
	})
	if err != nil {
		// Should never fail.
		panic(err)
	}

	return &pb.CreateTestResultRequest{
		Invocation: invName,
		RequestId:  "request-id-123",

		TestResult: &pb.TestResult{
			TestId:   testID,
			ResultId: "result-id-0",
			Expected: true,
			Status:   pb.TestStatus_PASS,
			Variant: pbutil.Variant(
				"a/b", "1",
				"c", "2",
			),
			TestMetadata: &pb.TestMetadata{
				Name: "original_name",
				Location: &pb.TestLocation{
					Repo:     "https://chromium.googlesource.com/chromium/src",
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
			FailureReason: &pb.FailureReason{
				PrimaryErrorMessage: "got 1, want 0",
				Errors: []*pb.FailureReason_Error{
					{Message: "got 1, want 0"},
					{Message: "got 2, want 1"},
				},
				TruncatedErrorsCount: 0,
			},
			Properties: properties,
		},
	}
}

func TestValidateCreateTestResultRequest(t *testing.T) {
	t.Parallel()

	now := testclock.TestRecentTimeUTC
	ftt.Run("ValidateCreateTestResultRequest", t, func(t *ftt.Test) {
		tvID := &pb.TestVariantIdentifier{
			ModuleName:   "//infra/junit_tests",
			ModuleScheme: "junit",
			ModuleVariant: pbutil.Variant(
				"key", "value",
			),
			CoarseName: "org.chromium.go.luci",
			FineName:   "ValidationTests",
			CaseName:   "FooBar",
		}
		req := validCreateTestResultRequest(now, "invocations/u-build-1", tvID)
		cfg, err := config.NewCompiledServiceConfig(config.CreatePlaceHolderServiceConfig(), "revision")
		assert.NoErr(t, err)

		t.Run("suceeeds", func(t *ftt.Test) {
			assert.Loosely(t, validateCreateTestResultRequest(req, cfg, now), should.BeNil)

			t.Run("with empty request_id", func(t *ftt.Test) {
				req.RequestId = ""
				assert.Loosely(t, validateCreateTestResultRequest(req, cfg, now), should.BeNil)
			})
		})

		t.Run("fails with", func(t *ftt.Test) {
			t.Run(`empty invocation`, func(t *ftt.Test) {
				req.Invocation = ""
				err := validateCreateTestResultRequest(req, cfg, now)
				assert.Loosely(t, err, should.ErrLike("invocation: unspecified"))
			})
			t.Run(`invalid invocation`, func(t *ftt.Test) {
				req.Invocation = " invalid "
				err := validateCreateTestResultRequest(req, cfg, now)
				assert.Loosely(t, err, should.ErrLike("invocation: does not match"))
			})

			t.Run(`empty test_result`, func(t *ftt.Test) {
				req.TestResult = nil
				err := validateCreateTestResultRequest(req, cfg, now)
				assert.Loosely(t, err, should.ErrLike("test_result: unspecified"))
			})
			t.Run(`invalid test_result`, func(t *ftt.Test) {
				req.TestResult.TestVariantIdentifier = nil
				err := validateCreateTestResultRequest(req, cfg, now)
				assert.Loosely(t, err, should.ErrLike("test_result: test_variant_identifier: unspecified"))
			})

			t.Run("invalid request_id", func(t *ftt.Test) {
				// non-ascii character
				req.RequestId = string(rune(244))
				err := validateCreateTestResultRequest(req, cfg, now)
				assert.Loosely(t, err, should.ErrLike("request_id: does not match"))
			})
		})

		t.Run(`legacy uploaders`, func(t *ftt.Test) {
			req := legacyCreateTestResultRequest(now, "invocations/u-build-1", "test-id")
			t.Run("suceeeds", func(t *ftt.Test) {
				assert.Loosely(t, validateCreateTestResultRequest(req, cfg, now), should.BeNil)
			})

			t.Run("fails with", func(t *ftt.Test) {
				t.Run(`invalid test_id`, func(t *ftt.Test) {
					req.TestResult.TestId = "\x00"
					err := validateCreateTestResultRequest(req, cfg, now)
					assert.Loosely(t, err, should.ErrLike("test_result: test_id: non-printable rune '\\x00' at byte index 0"))
				})
			})
		})
	})
}

func TestCreateTestResult(t *testing.T) {
	ftt.Run(`CreateTestResult`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx) // For config in-process cache.
		ctx = memory.Use(ctx)                    // For config datastore cache.
		err := config.SetServiceConfigForTesting(ctx, config.CreatePlaceHolderServiceConfig())
		assert.NoErr(t, err)

		recorder := newTestRecorderServer()

		tvID := &pb.TestVariantIdentifier{
			ModuleName:   "//infra/junit_tests",
			ModuleScheme: "junit",
			ModuleVariant: pbutil.Variant(
				"key", "value",
			),
			CoarseName: "org.chromium.go.luci",
			FineName:   "ValidationTests",
			CaseName:   "FooBar",
		}

		req := validCreateTestResultRequest(
			clock.Now(ctx).UTC(), "invocations/u-build-1", tvID,
		)

		createTestResult := func(req *pb.CreateTestResultRequest, expected *pb.TestResult, expectedCommonPrefix string) {
			expectedWireTR := proto.Clone(expected).(*pb.TestResult)
			if req.TestResult.TestVariantIdentifier == nil {
				// For legacy create requests, expect the response to omit the TestVariantIdentifier.
				expectedWireTR.TestVariantIdentifier = nil
			}

			res, err := recorder.CreateTestResult(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Match(expectedWireTR))

			// double-check it with the database
			row, err := testresults.Read(span.Single(ctx), res.Name)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, row, should.Match(expected))

			var invCommonTestIdPrefix string
			err = invocations.ReadColumns(span.Single(ctx), invocations.ID("u-build-1"), map[string]any{"CommonTestIDPrefix": &invCommonTestIdPrefix})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, invCommonTestIdPrefix, should.Equal(expectedCommonPrefix))
		}

		// Insert a sample invocation
		tok, err := generateInvocationToken(ctx, "u-build-1")
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, tok))
		invID := invocations.ID("u-build-1")
		testutil.MustApply(ctx, t, insert.Invocation(invID, pb.Invocation_ACTIVE, nil))

		t.Run("succeeds", func(t *ftt.Test) {
			expected := proto.Clone(req.TestResult).(*pb.TestResult)
			expected.Name = "invocations/u-build-1/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/result-id-0"
			expected.TestVariantIdentifier.ModuleVariantHash = "5d8482c3056d8635"
			expected.TestId = "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar"
			expected.Variant = pbutil.Variant("key", "value")
			expected.VariantHash = "5d8482c3056d8635"

			t.Run("with a request ID", func(t *ftt.Test) {
				createTestResult(req, expected, "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar")

				ctx, cancel := span.ReadOnlyTransaction(ctx)
				defer cancel()
				trNum, err := resultcount.ReadTestResultCount(ctx, invocations.NewIDSet("u-build-1"))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, trNum, should.Equal(1))
			})

			t.Run("without a request ID", func(t *ftt.Test) {
				req.RequestId = ""
				createTestResult(req, expected, "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar")
			})

			t.Run("with legacy test result", func(t *ftt.Test) {
				req = legacyCreateTestResultRequest(clock.Now(ctx).UTC(), "invocations/u-build-1", "test-id")

				expected := proto.Clone(req.TestResult).(*pb.TestResult)
				expected.Name = "invocations/u-build-1/tests/test-id/results/result-id-0"
				expected.VariantHash = "c8643f74854d84b4"
				expected.TestVariantIdentifier = &pb.TestVariantIdentifier{
					ModuleName:   "legacy",
					ModuleScheme: "legacy",
					ModuleVariant: pbutil.Variant(
						"a/b", "1",
						"c", "2",
					),
					ModuleVariantHash: "c8643f74854d84b4",
					CaseName:          "test-id",
				}

				createTestResult(req, expected, "test-id")
			})
		})

		t.Run("fails", func(t *ftt.Test) {
			t.Run("with an invalid request", func(t *ftt.Test) {
				req.Invocation = "this is an invalid invocation name"
				_, err := recorder.CreateTestResult(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad request: invocation: does not match"))
			})

			t.Run("with an non-existing invocation", func(t *ftt.Test) {
				tok, err = generateInvocationToken(ctx, "inv")
				assert.Loosely(t, err, should.BeNil)
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, tok))
				req.Invocation = "invocations/inv"
				_, err := recorder.CreateTestResult(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("invocations/inv not found"))
			})
		})
	})
}
