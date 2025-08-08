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

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/resultcount"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// validCreateTestResultRequest returns a valid CreateTestResultRequest message.
func validCreateTestResultRequest(now time.Time, workunitID workunits.ID, testIdentifier *pb.TestIdentifier) *pb.CreateTestResultRequest {
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
		Parent:    workunitID.Name(),
		RequestId: "request-id-123",

		TestResult: &pb.TestResult{
			TestIdStructured: testIdentifier,
			ResultId:         "result-id-0",
			StatusV2:         pb.TestResult_PASSED,
			// Round start time to microseconds to avoid errors from lossy Spanner roundtrip.
			StartTime: timestamppb.New(time.Date(2024, 10, 11, 23, 59, 59, 999999000, time.UTC)),
			Duration:  durationpb.New(12 * time.Microsecond),
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
			Properties: properties,
		},
	}
}

func legacyTestIDCreateTestResultRequest(now time.Time, workunitID workunits.ID, testID string) *pb.CreateTestResultRequest {
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
		Parent:    workunitID.Name(),
		RequestId: "request-id-123",

		TestResult: &pb.TestResult{
			// Legacy behaviour: an arbitrary/legacy test ID may be set instead of a structured test ID.
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
			// Legacy behaviour: failure reason may be set for non-failing results.
			FailureReason: &pb.FailureReason{
				// Legacy behaviour: old clients may set primary error message (before it was made output only)
				// and omit setting teh kind.
				PrimaryErrorMessage:  "got 1, want 0",
				TruncatedErrorsCount: 0,
			},
			Properties: properties,
		},
	}
}

func TestCreateTestResults(t *testing.T) {
	ftt.Run(`CreateTestResults`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx) // For config in-process cache.
		ctx = memory.Use(ctx)                    // For config datastore cache.
		err := config.SetServiceConfigForTesting(ctx, config.CreatePlaceHolderServiceConfig())
		assert.NoErr(t, err)

		recorder := newTestRecorderServer()
		now := clock.Now(ctx).UTC()

		rootInvID := rootinvocations.ID("root-inv-id")
		wuID := workunits.ID{
			RootInvocationID: "root-inv-id",
			WorkUnitID:       "work-unit:child-1",
		}
		junitModuleID := &pb.ModuleIdentifier{
			ModuleName:   "//infra/junit_tests",
			ModuleScheme: "junit",
			ModuleVariant: pbutil.Variant(
				"a/b", "1",
				"c", "2",
			),
			ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("a/b", "1", "c", "2")),
		}

		// Create some sample work units and a sample invocation.
		var ms []*spanner.Mutation
		ms = append(ms, insert.RootInvocationWithRootWorkUnit(rootinvocations.NewBuilder(rootInvID).Build())...)
		ms = append(ms, insert.WorkUnit(workunits.NewBuilder(rootInvID, "work-unit:child-1").WithState(pb.WorkUnit_ACTIVE).WithModuleID(junitModuleID).Build())...)
		testutil.MustApply(ctx, t, ms...)

		tvID := &pb.TestIdentifier{
			ModuleName:    junitModuleID.ModuleName,
			ModuleScheme:  junitModuleID.ModuleScheme,
			ModuleVariant: junitModuleID.ModuleVariant,
			CoarseName:    "org.chromium.go.luci",
			FineName:      "ValidationTests",
			CaseName:      "FooBar",
		}

		req := validCreateTestResultRequest(now, wuID, tvID)

		token, err := generateWorkUnitUpdateToken(ctx, wuID)
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		// This RPC largely reuses the implementation in BatchCreateTestResults.
		// With any errors returned, we check carefully that they do not
		// refer to "requests[0]" and refer to fields in this request.
		t.Run("request validation", func(t *ftt.Test) {
			t.Run("parent", func(t *ftt.Test) {
				t.Run("unspecified", func(t *ftt.Test) {
					req.Parent = ""
					_, err := recorder.CreateTestResult(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: parent: unspecified"))
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.Parent = "invalid value"
					_, err := recorder.CreateTestResult(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: parent: does not match pattern"))
				})
				t.Run("both parent and invocation set", func(t *ftt.Test) {
					req.Parent = wuID.Name()
					req.Invocation = "invocations/inv-id"
					_, err := recorder.CreateTestResult(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: invocation: must not be specified if parent is specified"))
				})
			})
			t.Run("invocation", func(t *ftt.Test) {
				req.Parent = ""
				t.Run("invalid", func(t *ftt.Test) {
					req.Invocation = "this is an invalid invocation name"
					_, err := recorder.CreateTestResult(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: invocation: does not match"))
				})
			})
			t.Run("test result", func(t *ftt.Test) {
				// validateTestResult has its own tests, so only test a few cases to make sure it is correctly integrated.
				t.Run("test ID using invalid scheme", func(t *ftt.Test) {
					req.TestResult.TestIdStructured.ModuleScheme = "undefined"
					_, err := recorder.CreateTestResult(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: test_result: test_id_structured: module_scheme: scheme \"undefined\" is not a known scheme"))
				})
				t.Run("invalid test_result", func(t *ftt.Test) {
					req.TestResult.TestIdStructured = nil
					_, err := recorder.CreateTestResult(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: test_result: test_id_structured: unspecified"))
				})

				// The following tests validation that is not part of validateTestResult.

				// Test IDs match work unit module IDs.
				t.Run("test ID does not match work unit module name", func(t *ftt.Test) {
					req.TestResult.TestIdStructured.ModuleName = "other"
					_, err := recorder.CreateTestResult(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
					assert.Loosely(t, err, should.ErrLike(`desc = test_result: test_id_structured: module_name: does not match parent work unit module_id.module_name; got "other", want "//infra/junit_tests"`))
				})
			})
			t.Run("request ID", func(t *ftt.Test) {
				t.Run("unspecified", func(t *ftt.Test) {
					req.RequestId = ""
					_, err := recorder.CreateTestResult(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: request_id: unspecified (please provide a per-request UUID to ensure idempotence)"))
				})
				t.Run("invalid", func(t *ftt.Test) {
					// non-ascii character
					req.RequestId = string(rune(244))
					_, err := recorder.CreateTestResult(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: request_id: does not match"))
				})
			})
		})
		t.Run("request authorization", func(t *ftt.Test) {
			t.Run("invalid update token", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid-token"))
				_, err := recorder.CreateTestResult(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.That(t, err, should.ErrLike(`invalid update token`))
			})
			t.Run("missing update token", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.MD{})
				_, err := recorder.CreateTestResult(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.Unauthenticated))
				assert.That(t, err, should.ErrLike(`missing update-token metadata value in the request`))
			})
		})
		t.Run("parent", func(t *ftt.Test) {
			t.Run("parent not active", func(t *ftt.Test) {
				testutil.MustApply(ctx, t, spanner.UpdateMap("WorkUnits", map[string]any{
					"RootInvocationShardId": wuID.RootInvocationShardID().RowID(),
					"WorkUnitId":            wuID.WorkUnitID,
					"State":                 pb.WorkUnit_FINALIZING,
				}))

				_, err := recorder.CreateTestResult(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
				assert.Loosely(t, err, should.ErrLike(`desc = parent "rootInvocations/root-inv-id/workUnits/work-unit:child-1" is not active`))
			})
			t.Run("parent not found", func(t *ftt.Test) {
				testutil.MustApply(ctx, t, spanner.Delete("WorkUnits", wuID.Key()))

				_, err := recorder.CreateTestResult(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike(`desc = "rootInvocations/root-inv-id/workUnits/work-unit:child-1" not found`))
			})
		})
		t.Run("with legacy invocation", func(t *ftt.Test) {
			invID := invocations.ID("u-build-1")
			testutil.MustApply(ctx, t, insert.Invocation(invID, pb.Invocation_ACTIVE, map[string]any{
				"ModuleName":        "legacy",
				"ModuleScheme":      "legacy",
				"ModuleVariant":     pbutil.Variant(),
				"ModuleVariantHash": pbutil.VariantHash(pbutil.Variant()),
			}))

			tok, err := generateInvocationToken(ctx, "u-build-1")
			assert.Loosely(t, err, should.BeNil)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, tok))

			req := legacyTestIDCreateTestResultRequest(now, wuID, "some-legacy-test-id")
			req.Invocation = "invocations/u-build-1"
			req.Parent = ""

			// Populate output-only fields, and other fields populated by backwards-compatibility logic.
			expected := proto.Clone(req.TestResult).(*pb.TestResult)
			expected.Name = "invocations/u-build-1/tests/some-legacy-test-id/results/result-id-0"
			expected.TestIdStructured = &pb.TestIdentifier{
				ModuleName:        "legacy",
				ModuleScheme:      "legacy",
				ModuleVariant:     pbutil.Variant("a/b", "1", "c", "2"),
				ModuleVariantHash: "c8643f74854d84b4",
				CaseName:          "some-legacy-test-id",
			}
			expected.VariantHash = "c8643f74854d84b4"
			expected.FailureReason = &pb.FailureReason{
				PrimaryErrorMessage: "got 1, want 0",
				Errors: []*pb.FailureReason_Error{
					{Message: "got 1, want 0"},
				},
			}
			expected.StatusV2 = pb.TestResult_PASSED

			t.Run("with an non-existing invocation", func(t *ftt.Test) {
				tok, err = generateInvocationToken(ctx, "inv")
				assert.Loosely(t, err, should.BeNil)
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, tok))
				req.Invocation = "invocations/inv"

				_, err := recorder.CreateTestResult(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("invocations/inv not found"))
			})
			t.Run("with no request ID", func(t *ftt.Test) {
				// This is a legacy behaviour and is valid for test result uploads to invocations only.
				req.RequestId = ""

				// Succeeds.
				_, err := recorder.CreateTestResult(ctx, req)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("success", func(t *ftt.Test) {
				createTestResult := func(req *pb.CreateTestResultRequest, expected *pb.TestResult, expectedCommonPrefix string) {
					expectedWireTR := proto.Clone(expected).(*pb.TestResult)
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

				createTestResult(req, expected, "some-legacy-test-id")

				ctx, cancel := span.ReadOnlyTransaction(ctx)
				defer cancel()
				trNum, err := resultcount.ReadTestResultCount(ctx, invocations.NewIDSet(invID))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, trNum, should.Equal(1))
			})
		})
		t.Run("with work units", func(t *ftt.Test) {
			expected := proto.Clone(req.TestResult).(*pb.TestResult)
			// Populate output-only fields.
			expected.Name = "rootInvocations/root-inv-id/workUnits/work-unit:child-1/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/result-id-0"
			expected.TestIdStructured.ModuleVariantHash = "c8643f74854d84b4"
			expected.TestId = "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar"
			expected.Variant = pbutil.Variant("a/b", "1", "c", "2")
			expected.VariantHash = "c8643f74854d84b4"
			expected.Status = pb.TestStatus_PASS
			expected.Expected = true

			createTestResult := func(req *pb.CreateTestResultRequest, expected *pb.TestResult) {
				expectedWireTR := proto.Clone(expected).(*pb.TestResult)
				res, err := recorder.CreateTestResult(ctx, req)
				assert.Loosely(t, err, should.BeNil, truth.LineContext(1))
				assert.Loosely(t, res, should.Match(expectedWireTR), truth.LineContext(1))

				// double-check it with the database
				row, err := testresults.Read(span.Single(ctx), res.Name)
				assert.Loosely(t, err, should.BeNil, truth.LineContext(1))
				assert.Loosely(t, row, should.Match(expected), truth.LineContext(1))
			}

			t.Run("base case", func(t *ftt.Test) {
				createTestResult(req, expected)

				ctx, cancel := span.ReadOnlyTransaction(ctx)
				defer cancel()

				// Each work unit should have one test result.
				trNum, err := resultcount.ReadTestResultCount(ctx, invocations.NewIDSet(wuID.LegacyInvocationID()))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, trNum, should.Equal(1))
			})
			// Test result variations are already covered in BatchCreateTestResults
			// tests, no need to repeat them here.
		})
	})
}
