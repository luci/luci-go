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
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
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
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	configpb "go.chromium.org/luci/resultdb/proto/config"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestBatchCreateTestResults(t *testing.T) {
	ftt.Run(`BatchCreateTestResults`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx) // For config in-process cache.
		ctx = memory.Use(ctx)                    // For config datastore cache.
		err := config.SetServiceConfigForTesting(ctx, config.CreatePlaceHolderServiceConfig())
		assert.NoErr(t, err)

		recorder := newTestRecorderServer()
		now := clock.Now(ctx).UTC()

		rootInvID := rootinvocations.ID("root-inv-id")
		wuID1 := workunits.ID{
			RootInvocationID: "root-inv-id",
			WorkUnitID:       "work-unit:child-1",
		}
		wuID2 := workunits.ID{
			RootInvocationID: "root-inv-id",
			WorkUnitID:       "work-unit:child-2",
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
		legacyModuleID := &pb.ModuleIdentifier{
			ModuleName:   "legacy",
			ModuleScheme: "legacy",
		}

		// Create some sample work units and a sample invocation.
		var ms []*spanner.Mutation
		ms = append(ms, insert.RootInvocationWithRootWorkUnit(rootinvocations.NewBuilder(rootInvID).Build())...)
		ms = append(ms, insert.WorkUnit(workunits.NewBuilder(rootInvID, "work-unit:child-1").WithState(pb.WorkUnit_ACTIVE).WithModuleID(junitModuleID).Build())...)
		ms = append(ms, insert.WorkUnit(workunits.NewBuilder(rootInvID, "work-unit:child-2").WithState(pb.WorkUnit_ACTIVE).WithModuleID(legacyModuleID).Build())...)
		testutil.MustApply(ctx, t, ms...)

		tvID := &pb.TestIdentifier{
			ModuleName:    junitModuleID.ModuleName,
			ModuleScheme:  junitModuleID.ModuleScheme,
			ModuleVariant: junitModuleID.ModuleVariant,
			CoarseName:    "org.chromium.go.luci",
			FineName:      "ValidationTests",
			CaseName:      "FooBar",
		}
		reqItem1 := validCreateTestResultRequest(now, wuID1, tvID)
		reqItem1.TestResult.ResultId = "result-id-0"
		reqItem2 := legacyTestIDCreateTestResultRequest(now, wuID2, "some-legacy-test-id")
		reqItem2.TestResult.ResultId = "result-id-1"
		req := &pb.BatchCreateTestResultsRequest{
			Requests:  []*pb.CreateTestResultRequest{reqItem1, reqItem2},
			RequestId: "request-id-123",
		}

		token, err := generateWorkUnitUpdateToken(ctx, wuID1)
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		t.Run("request validation", func(t *ftt.Test) {
			t.Run("parent", func(t *ftt.Test) {
				t.Run("invalid", func(t *ftt.Test) {
					req.Parent = "invalid value"
					_, err := recorder.BatchCreateTestResults(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("parent: does not match pattern"))
				})
				t.Run("both parent and invocation set", func(t *ftt.Test) {
					req.Parent = wuID1.Name()
					req.Invocation = "invocations/inv-id"
					_, err := recorder.BatchCreateTestResults(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("invocation: must not be specified if parent is specified"))
				})
			})
			t.Run("invocation", func(t *ftt.Test) {
				t.Run("invalid", func(t *ftt.Test) {
					req.Parent = ""
					req.Invocation = "this is an invalid invocation name"
					_, err := recorder.BatchCreateTestResults(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("invocation: does not match"))
				})
			})
			t.Run(`too many requests`, func(t *ftt.Test) {
				req.Requests = make([]*pb.CreateTestResultRequest, 1000)
				_, err := recorder.BatchCreateTestResults(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`requests: the number of requests in the batch (1000) exceeds 500`))
			})
			t.Run(`requests too large`, func(t *ftt.Test) {
				req.Requests[0].Parent = strings.Repeat("a", pbutil.MaxBatchRequestSize)
				_, err := recorder.BatchCreateTestResults(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`requests: the size of all requests is too large`))
			})
			t.Run("sub-request", func(t *ftt.Test) {
				t.Run("parent", func(t *ftt.Test) {
					t.Run("unspecified", func(t *ftt.Test) {
						req.Requests[1].Parent = ""
						_, err := recorder.BatchCreateTestResults(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("requests[1]: parent: unspecified"))
					})
					t.Run("invalid", func(t *ftt.Test) {
						req.Requests[1].Parent = "invalid value"
						_, err := recorder.BatchCreateTestResults(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("requests[1]: parent: does not match pattern "))
					})
					t.Run("parents in different root invocations", func(t *ftt.Test) {
						req.Requests[0].Parent = "rootInvocations/root-inv-1/workUnits/work-unit"
						req.Requests[1].Parent = "rootInvocations/root-inv-2/workUnits/work-unit"
						_, err := recorder.BatchCreateTestResults(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.That(t, err, should.ErrLike(`requests[1]: parent: all test results in a batch must belong to the same root invocation; got "rootInvocations/root-inv-2", want "rootInvocations/root-inv-1"`))
					})
					t.Run("unmatched parent in requests", func(t *ftt.Test) {
						req.Parent = wuID1.Name()
						_, err := recorder.BatchCreateTestResults(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`requests[1]: parent: must be either empty or equal to "rootInvocations/root-inv-id/workUnits/work-unit:child-1", but got "rootInvocations/root-inv-id/workUnits/work-unit:child-2"`))
					})
					t.Run("set in conjuntion with invocation at the top-level", func(t *ftt.Test) {
						req.Invocation = "invocations/foo"
						_, err := recorder.BatchCreateTestResults(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("requests[0]: parent: must not be set if `invocation` is set on top-level batch request"))
					})
					t.Run("mixed invocations and parents set at the child request-level", func(t *ftt.Test) {
						req.Requests[1].Invocation = "invocations/foo"
						req.Requests[1].Parent = ""
						_, err := recorder.BatchCreateTestResults(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`requests[1]: invocation: may not be set at the request-level unless also set at the batch-level`))
					})
				})
				t.Run("invocation", func(t *ftt.Test) {
					req.Parent = ""
					for _, r := range req.Requests {
						r.Parent = ""
					}

					t.Run("set in conjunction with parent at the top-level", func(t *ftt.Test) {
						req.Parent = wuID1.Name()
						req.Requests[1].Invocation = "invocations/bar"
						_, err := recorder.BatchCreateTestResults(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("requests[1]: invocation: must not be set if `parent` is set on top-level batch request"))
					})
					t.Run("unmatched invocation in requests", func(t *ftt.Test) {
						req.Invocation = "invocations/foo"
						req.Requests[1].Invocation = "invocations/bar"
						_, err := recorder.BatchCreateTestResults(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`requests[1]: invocation: must be either empty or equal to "invocations/foo", but got "invocations/bar"`))
					})
				})
				t.Run("test result", func(t *ftt.Test) {
					// validateTestResult has its own tests, so only test a few cases to make sure it is correctly integrated.
					t.Run(`unspecified`, func(t *ftt.Test) {
						req.Requests[0].TestResult = nil
						_, err := recorder.BatchCreateTestResults(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`requests[0]: test_result: unspecified`))
					})
					t.Run("test ID using invalid scheme", func(t *ftt.Test) {
						req.Requests[0].TestResult.TestIdStructured.ModuleScheme = "undefined"
						_, err := recorder.BatchCreateTestResults(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("requests[0]: test_result: test_id_structured: module_scheme: scheme \"undefined\" is not a known scheme"))
					})
					t.Run("invalid test_result", func(t *ftt.Test) {
						req.Requests[0].TestResult.TestIdStructured = nil
						_, err := recorder.BatchCreateTestResults(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("requests[0]: test_result: test_id_structured: unspecified"))
					})

					// The following tests validation that is not part of validateTestResult.
					// Duplicates checking.

					t.Run("duplicated test_results", func(t *ftt.Test) {
						// Set result 1 test_id to be the same as result 0 test ID (albeit specified in flat form).
						req.Requests[1].TestResult.TestId = pbutil.EncodeTestID(pbutil.ExtractBaseTestIdentifier(tvID))
						req.Requests[0].TestResult.ResultId = "result-id"
						req.Requests[1].TestResult.ResultId = "result-id"
						_, err := recorder.BatchCreateTestResults(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`requests[1]: a test result with the same test ID and result ID already exists at request[0]; testID "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar", resultID "result-id"`))
					})

					// Test IDs match work unit module IDs.
					t.Run("test ID does not match work unit module name", func(t *ftt.Test) {
						req.Requests[0].TestResult.TestIdStructured.ModuleName = "other"
						_, err := recorder.BatchCreateTestResults(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
						assert.Loosely(t, err, should.ErrLike(`requests[0]: test_result: test_id_structured: module_name: does not match parent work unit module_id.module_name; got "other", want "//infra/junit_tests"`))
					})
					t.Run("test ID does not match work unit module scheme", func(t *ftt.Test) {
						req.Requests[0].TestResult.TestIdStructured.ModuleScheme = "gtest"
						req.Requests[0].TestResult.TestIdStructured.CoarseName = ""
						_, err := recorder.BatchCreateTestResults(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
						assert.Loosely(t, err, should.ErrLike(`requests[0]: test_result: test_id_structured: module_scheme: does not match parent work unit module_id.module_scheme; got "gtest", want "junit"`))
					})
					t.Run("test ID does not match work unit module variant", func(t *ftt.Test) {
						req.Requests[0].TestResult.TestIdStructured.ModuleVariant = &pb.Variant{}
						_, err := recorder.BatchCreateTestResults(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
						assert.Loosely(t, err, should.ErrLike(`requests[0]: test_result: test_id_structured: module_variant: does not match parent work unit module_id.module_variant; got {}, want {"a/b":"1","c":"2"}`))
					})
					t.Run("flat test ID does not match work unit module", func(t *ftt.Test) {
						req.Requests[0].TestResult.TestIdStructured = nil
						req.Requests[0].TestResult.TestId = ":other!junit:org.chromium.go.luci:ValidationTests#FooBar"
						req.Requests[0].TestResult.Variant = junitModuleID.ModuleVariant
						_, err := recorder.BatchCreateTestResults(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
						assert.Loosely(t, err, should.ErrLike(`requests[0]: test_result: test_id_structured: module_name: does not match parent work unit module_id.module_name; got "other", want "//infra/junit_tests"`))
					})
					t.Run("flat test ID variant does not match work unit module variant", func(t *ftt.Test) {
						req.Requests[0].TestResult.TestIdStructured = nil
						req.Requests[0].TestResult.TestId = "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar"
						req.Requests[0].TestResult.Variant = pbutil.Variant("k2", "v2")
						_, err := recorder.BatchCreateTestResults(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
						assert.Loosely(t, err, should.ErrLike(`requests[0]: test_result: test_id_structured: module_variant: does not match parent work unit module_id.module_variant; got {"k2":"v2"}, want {"a/b":"1","c":"2"}`))
					})
					t.Run("structured test ID set and work unit does not have module ID", func(t *ftt.Test) {
						wuIDNoModule := workunits.ID{
							RootInvocationID: "root-inv-id",
							WorkUnitID:       "work-unit:nomodule",
						}
						testutil.MustApply(ctx, t, insert.WorkUnit(workunits.NewBuilder(rootInvID, "work-unit:nomodule").WithState(pb.WorkUnit_ACTIVE).WithModuleID(nil).Build())...)
						req.Requests[0].Parent = wuIDNoModule.Name()
						_, err := recorder.BatchCreateTestResults(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
						assert.Loosely(t, err, should.ErrLike("equests[0]: test_result: to upload test results or test result artifacts, you must set the module_id on the parent work unit first"))
					})
					t.Run("legacy test ID set and work unit does not have module ID", func(t *ftt.Test) {
						wuIDNoModule := workunits.ID{
							RootInvocationID: "root-inv-id",
							WorkUnitID:       "work-unit:nomodule",
						}
						testutil.MustApply(ctx, t, insert.WorkUnit(workunits.NewBuilder(rootInvID, "work-unit:nomodule").WithState(pb.WorkUnit_ACTIVE).WithModuleID(nil).Build())...)
						req.Requests[1].Parent = wuIDNoModule.Name()
						_, err := recorder.BatchCreateTestResults(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
						assert.Loosely(t, err, should.ErrLike("requests[1]: test_result: to upload test results or test result artifacts, you must set the module_id on the parent work unit first"))
					})
				})
			})
			t.Run("request ID", func(t *ftt.Test) {
				t.Run("unspecified", func(t *ftt.Test) {
					req.RequestId = ""
					_, err := recorder.BatchCreateTestResults(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: request_id: unspecified (please provide a per-request UUID to ensure idempotence)"))
				})
				t.Run("invalid", func(t *ftt.Test) {
					// non-ascii character
					req.RequestId = string(rune(244))
					_, err := recorder.BatchCreateTestResults(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: request_id: does not match"))
				})
				t.Run("unmatched request_id in requests", func(t *ftt.Test) {
					req.RequestId = "foo"
					req.Requests[0].RequestId = "bar"
					_, err := recorder.BatchCreateTestResults(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: requests[0]: request_id: must be either empty or equal"))
				})
			})
		})
		t.Run("request authorization", func(t *ftt.Test) {
			t.Run("request cannot be authorised with single update token", func(t *ftt.Test) {
				req.Requests[0].Parent = "rootInvocations/root-inv/workUnits/work-unit-1"
				req.Requests[1].Parent = "rootInvocations/root-inv/workUnits/work-unit-2"

				ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid-token"))
				_, err := recorder.BatchCreateTestResults(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike(`requests[1]: parent: work unit "rootInvocations/root-inv/workUnits/work-unit-2" requires a different update token to request[0]'s "rootInvocations/root-inv/workUnits/work-unit-1", but this RPC only accepts one update token`))
			})
			t.Run("invalid update token", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid-token"))
				_, err := recorder.BatchCreateTestResults(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.That(t, err, should.ErrLike(`invalid update token`))
			})
			t.Run("missing update token", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.MD{})
				_, err := recorder.BatchCreateTestResults(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.Unauthenticated))
				assert.That(t, err, should.ErrLike(`missing update-token metadata value in the request`))
			})
			t.Run("with invocations", func(t *ftt.Test) {
				// The implementation uses a different path for legacy invocations, so verify authorisation still implemented correctly.
				req.Invocation = "invocations/u-build-1"
				for _, r := range req.Requests {
					r.Parent = ""
				}
				t.Run("invalid update token", func(t *ftt.Test) {
					ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid-token"))
					_, err := recorder.BatchCreateTestResults(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.That(t, err, should.ErrLike(`invalid update token`))
				})
				t.Run("missing update token", func(t *ftt.Test) {
					ctx := metadata.NewIncomingContext(ctx, metadata.MD{})
					_, err := recorder.BatchCreateTestResults(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.Unauthenticated))
					assert.That(t, err, should.ErrLike(`missing update-token metadata value in the request`))
				})
			})
		})
		t.Run("parent", func(t *ftt.Test) {
			t.Run("parent not active", func(t *ftt.Test) {
				testutil.MustApply(ctx, t, spanner.UpdateMap("WorkUnits", map[string]any{
					"RootInvocationShardId": wuID2.RootInvocationShardID().RowID(),
					"WorkUnitId":            wuID2.WorkUnitID,
					"State":                 pb.WorkUnit_FINALIZING,
				}))

				_, err := recorder.BatchCreateTestResults(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
				assert.Loosely(t, err, should.ErrLike(`requests[1]: parent "rootInvocations/root-inv-id/workUnits/work-unit:child-2" is not active`))
			})
			t.Run("parent not found", func(t *ftt.Test) {
				testutil.MustApply(ctx, t, spanner.Delete("WorkUnits", wuID2.Key()))

				_, err := recorder.BatchCreateTestResults(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike(`"rootInvocations/root-inv-id/workUnits/work-unit:child-2" not found`))
			})
		})
		t.Run("with legacy invocation", func(t *ftt.Test) {
			invID := invocations.ID("u-build-1")
			testutil.MustApply(ctx, t, insert.Invocation(invID, pb.Invocation_ACTIVE, map[string]any{
				"ModuleName":        "//infra/junit_tests",
				"ModuleScheme":      "junit",
				"ModuleVariant":     pbutil.Variant("a/b", "1", "c", "2"),
				"ModuleVariantHash": pbutil.VariantHash(pbutil.Variant("a/b", "1", "c", "2")),
			}))

			tok, err := generateInvocationToken(ctx, "u-build-1")
			assert.Loosely(t, err, should.BeNil)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, tok))

			reqItem1 := validCreateTestResultRequest(now, wuID1, tvID)
			reqItem1.TestResult.ResultId = "result-id-0"
			reqItem1.Invocation = "invocations/u-build-1"
			reqItem1.Parent = ""
			reqItem2 := validCreateTestResultRequest(now, wuID2, tvID)
			reqItem2.TestResult.ResultId = "result-id-1"
			reqItem2.Parent = ""
			req := &pb.BatchCreateTestResultsRequest{
				Invocation: "invocations/u-build-1",
				Requests:   []*pb.CreateTestResultRequest{reqItem1, reqItem2},
				RequestId:  "request-id-123",
			}

			expected := make([]*pb.TestResult, 2)
			expected[0] = proto.Clone(req.Requests[0].TestResult).(*pb.TestResult)
			// Populate output-only fields.
			expected[0].Name = "invocations/u-build-1/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/result-id-0"
			expected[0].TestIdStructured.ModuleVariantHash = "c8643f74854d84b4"
			expected[0].TestId = "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar"
			expected[0].Variant = pbutil.Variant("a/b", "1", "c", "2")
			expected[0].VariantHash = "c8643f74854d84b4"
			expected[0].Status = pb.TestStatus_PASS
			expected[0].Expected = true

			// Populate output-only fields.
			expected[1] = proto.Clone(req.Requests[1].TestResult).(*pb.TestResult)
			expected[1].Name = "invocations/u-build-1/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/result-id-1"
			expected[1].TestIdStructured.ModuleVariantHash = "c8643f74854d84b4"
			expected[1].TestId = "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar"
			expected[1].Variant = pbutil.Variant("a/b", "1", "c", "2")
			expected[1].VariantHash = "c8643f74854d84b4"
			expected[1].Status = pb.TestStatus_PASS
			expected[1].Expected = true

			t.Run("with a non-existing invocation", func(t *ftt.Test) {
				tok, err = generateInvocationToken(ctx, "inv")
				assert.Loosely(t, err, should.BeNil)
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, tok))
				req.Invocation = "invocations/inv"
				for _, r := range req.Requests {
					r.Invocation = ""
				}

				_, err := recorder.BatchCreateTestResults(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("invocations/inv not found"))
			})
			t.Run("with an inactive invocation", func(t *ftt.Test) {
				testutil.MustApply(ctx, t, spanutil.UpdateMap("Invocations", map[string]any{
					"InvocationId": invID,
					"State":        pb.Invocation_FINALIZED,
				}))
				_, err := recorder.BatchCreateTestResults(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
				assert.Loosely(t, err, should.ErrLike(`invocations/u-build-1 is not active`))
			})
			t.Run("with no request ID", func(t *ftt.Test) {
				// This is a legacy behaviour and is valid for test result uploads to invocations only.
				req.RequestId = ""
				for _, r := range req.Requests {
					r.RequestId = ""
				}

				t.Run("success", func(t *ftt.Test) {
					_, err := recorder.BatchCreateTestResults(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("empty in batch-level, but not in requests", func(t *ftt.Test) {
					// This error can only be triggered when using invocations, as having
					// an empty batch-level request_id is not allowed when using work units.
					req.Requests[0].RequestId = "123"
					_, err := recorder.BatchCreateTestResults(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike(`bad request: requests[0]: request_id: must be either empty or equal to "", but got "123"`))
				})
			})
			t.Run("with module_id unset", func(t *ftt.Test) {
				testutil.MustApply(ctx, t, spanutil.UpdateMap("Invocations", map[string]any{
					"InvocationId":      invID,
					"ModuleName":        spanner.NullString{},
					"ModuleScheme":      spanner.NullString{},
					"ModuleVariant":     []string(nil),
					"ModuleVariantHash": spanner.NullString{},
				}))
				t.Run("uploading structured test IDs", func(t *ftt.Test) {
					_, err := recorder.BatchCreateTestResults(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("uploading legacy IDs", func(t *ftt.Test) {
					req.Requests[0].TestResult.TestIdStructured = nil
					req.Requests[0].TestResult.TestId = "some-legacy-test-id"
					req.Requests = req.Requests[:1]
					_, err := recorder.BatchCreateTestResults(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
			})
			t.Run("success", func(t *ftt.Test) {
				createTestResults := func(req *pb.BatchCreateTestResultsRequest, expectedTRs []*pb.TestResult, expectedCommonPrefix string) {
					response, err := recorder.BatchCreateTestResults(ctx, req)
					assert.Loosely(t, err, should.BeNil, truth.LineContext(1))

					assert.Loosely(t, len(response.TestResults), should.Equal(len(expectedTRs)), truth.LineContext(1))
					for i := range req.Requests {
						expectedWireTR := proto.Clone(expectedTRs[i]).(*pb.TestResult)
						assert.Loosely(t, response.TestResults[i], should.Match(expectedWireTR), truth.LineContext(1))

						// double-check it with the database
						row, err := testresults.Read(span.Single(ctx), expectedTRs[i].Name)
						assert.Loosely(t, err, should.BeNil, truth.LineContext(1))
						assert.Loosely(t, row, should.Match(expectedTRs[i]), truth.LineContext(1))

						var invCommonTestIDPrefix string
						var invVars []string
						err = invocations.ReadColumns(
							span.Single(ctx), invocations.ID("u-build-1"),
							map[string]any{
								"CommonTestIDPrefix":     &invCommonTestIDPrefix,
								"TestResultVariantUnion": &invVars,
							})
						assert.Loosely(t, err, should.BeNil, truth.LineContext(1))
						assert.Loosely(t, invCommonTestIDPrefix, should.Equal(expectedCommonPrefix), truth.LineContext(1))
						assert.Loosely(t, invVars, should.Match([]string{
							"a/b:1",
							"c:2",
						}), truth.LineContext(1))
					}
				}

				createTestResults(req, expected, "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar")

				ctx, cancel := span.ReadOnlyTransaction(ctx)
				defer cancel()
				trNum, err := resultcount.ReadTestResultCount(ctx, invocations.NewIDSet(invID))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, trNum, should.Equal(2))
			})
		})
		t.Run("with work units", func(t *ftt.Test) {
			expected := make([]*pb.TestResult, 2)
			expected[0] = proto.Clone(req.Requests[0].TestResult).(*pb.TestResult)
			// Populate output-only fields.
			expected[0].Name = "rootInvocations/root-inv-id/workUnits/work-unit:child-1/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/result-id-0"
			expected[0].TestIdStructured.ModuleVariantHash = "c8643f74854d84b4"
			expected[0].TestId = "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar"
			expected[0].Variant = pbutil.Variant("a/b", "1", "c", "2")
			expected[0].VariantHash = "c8643f74854d84b4"
			expected[0].Status = pb.TestStatus_PASS
			expected[0].Expected = true

			// Populate output-only fields, and other fields populated by backwards-compatibility logic.
			expected[1] = proto.Clone(req.Requests[1].TestResult).(*pb.TestResult)
			expected[1].Name = "rootInvocations/root-inv-id/workUnits/work-unit:child-2/tests/some-legacy-test-id/results/result-id-1"
			expected[1].TestIdStructured = &pb.TestIdentifier{
				ModuleName:        "legacy",
				ModuleScheme:      "legacy",
				ModuleVariant:     pbutil.Variant("a/b", "1", "c", "2"),
				ModuleVariantHash: "c8643f74854d84b4",
				CaseName:          "some-legacy-test-id",
			}
			expected[1].VariantHash = "c8643f74854d84b4"
			expected[1].FailureReason = &pb.FailureReason{
				PrimaryErrorMessage: "got 1, want 0",
				Errors: []*pb.FailureReason_Error{
					{Message: "got 1, want 0"},
				},
			}
			expected[1].StatusV2 = pb.TestResult_PASSED

			createTestResults := func(req *pb.BatchCreateTestResultsRequest, expectedTRs []*pb.TestResult) {
				response, err := recorder.BatchCreateTestResults(ctx, req)
				assert.Loosely(t, err, should.BeNil, truth.LineContext(1))

				assert.Loosely(t, len(response.TestResults), should.Equal(len(expectedTRs)), truth.LineContext(1))
				for i := range req.Requests {
					expectedWireTR := proto.Clone(expectedTRs[i]).(*pb.TestResult)
					assert.Loosely(t, response.TestResults[i], should.Match(expectedWireTR), truth.LineContext(1))

					// double-check it with the database
					row, err := testresults.Read(span.Single(ctx), expectedTRs[i].Name)
					assert.Loosely(t, err, should.BeNil, truth.LineContext(1))
					assert.Loosely(t, row, should.Match(expectedTRs[i]), truth.LineContext(1))
				}
			}

			t.Run("base case", func(t *ftt.Test) {
				createTestResults(req, expected)

				ctx, cancel := span.ReadOnlyTransaction(ctx)
				defer cancel()

				// Each work unit should have one test result.
				trNum, err := resultcount.ReadTestResultCount(ctx, invocations.NewIDSet(wuID1.LegacyInvocationID()))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, trNum, should.Equal(1))

				trNum, err = resultcount.ReadTestResultCount(ctx, invocations.NewIDSet(wuID2.LegacyInvocationID()))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, trNum, should.Equal(1))
			})
			t.Run("with common parent", func(t *ftt.Test) {
				req.Parent = wuID1.Name()
				req.Requests[0].Parent = ""

				// Drop the second request as it will be invalid for work unit 1.
				req.Requests = req.Requests[:1]
				expected = expected[:1]

				createTestResults(req, expected)
			})
			t.Run("with failure reason", func(t *ftt.Test) {
				req.Requests[0].TestResult.StatusV2 = pb.TestResult_FAILED
				req.Requests[0].TestResult.FailureReason = &pb.FailureReason{
					Kind: pb.FailureReason_CRASH,
					Errors: []*pb.FailureReason_Error{
						{Message: "got 1, want 0"},
						{Message: "got 2, want 1"},
					},
					TruncatedErrorsCount: 0,
				}

				expected[0].StatusV2 = pb.TestResult_FAILED
				expected[0].Expected = false
				expected[0].Status = pb.TestStatus_CRASH
				expected[0].FailureReason = &pb.FailureReason{
					Kind:                pb.FailureReason_CRASH,
					PrimaryErrorMessage: "got 1, want 0",
					Errors: []*pb.FailureReason_Error{
						{Message: "got 1, want 0"},
						{Message: "got 2, want 1"},
					},
					TruncatedErrorsCount: 0,
				}
				createTestResults(req, expected)
			})
			t.Run("with skipped reason", func(t *ftt.Test) {
				req.Requests[0].TestResult.StatusV2 = pb.TestResult_SKIPPED
				req.Requests[0].TestResult.SkippedReason = &pb.SkippedReason{
					Kind:          pb.SkippedReason_DISABLED_AT_DECLARATION,
					ReasonMessage: "An explanatory message.",
					Trace:         "Some stack trace\nMore stack trace",
				}

				expected[0].StatusV2 = pb.TestResult_SKIPPED
				expected[0].Status = pb.TestStatus_SKIP
				expected[0].Expected = true
				expected[0].SkippedReason = &pb.SkippedReason{
					Kind:          pb.SkippedReason_DISABLED_AT_DECLARATION,
					ReasonMessage: "An explanatory message.",
					Trace:         "Some stack trace\nMore stack trace",
				}
				createTestResults(req, expected)
			})
			t.Run("with framework extensions", func(t *ftt.Test) {
				req.Requests[0].TestResult.StatusV2 = pb.TestResult_PASSED
				req.Requests[0].TestResult.FrameworkExtensions = &pb.FrameworkExtensions{
					WebTest: &pb.WebTest{
						Status:     pb.WebTest_TIMEOUT,
						IsExpected: true,
					},
				}

				expected[0].StatusV2 = pb.TestResult_PASSED
				expected[0].Status = pb.TestStatus_ABORT
				expected[0].Expected = true
				expected[0].FrameworkExtensions = &pb.FrameworkExtensions{
					WebTest: &pb.WebTest{
						Status:     pb.WebTest_TIMEOUT,
						IsExpected: true,
					},
				}
				createTestResults(req, expected)
			})
			t.Run("with legacy-form test result", func(t *ftt.Test) {
				legacyResult := req.Requests[1]

				t.Run("v2 status is set based on v1 status", func(t *ftt.Test) {
					t.Run("unexpected crash", func(t *ftt.Test) {
						legacyResult.TestResult.Status = pb.TestStatus_CRASH
						legacyResult.TestResult.Expected = false

						expected[1].Status = pb.TestStatus_CRASH
						expected[1].Expected = false
						expected[1].StatusV2 = pb.TestResult_FAILED
						expected[1].FailureReason.Kind = pb.FailureReason_CRASH

						createTestResults(req, expected)
					})
					t.Run("unexpected pass", func(t *ftt.Test) {
						legacyResult.TestResult.Status = pb.TestStatus_PASS
						legacyResult.TestResult.Expected = false
						legacyResult.TestResult.FailureReason = nil

						expected[1].Status = pb.TestStatus_PASS
						expected[1].Expected = false
						expected[1].StatusV2 = pb.TestResult_FAILED
						expected[1].FailureReason = &pb.FailureReason{
							Kind: pb.FailureReason_ORDINARY,
						}
						expected[1].FrameworkExtensions = &pb.FrameworkExtensions{
							WebTest: &pb.WebTest{
								Status:     pb.WebTest_PASS,
								IsExpected: false,
							},
						}
						createTestResults(req, expected)
					})
					t.Run("expected abort", func(t *ftt.Test) {
						legacyResult.TestResult.Status = pb.TestStatus_ABORT
						legacyResult.TestResult.Expected = true

						expected[1].Status = pb.TestStatus_ABORT
						expected[1].Expected = true
						expected[1].StatusV2 = pb.TestResult_PASSED
						expected[1].FrameworkExtensions = &pb.FrameworkExtensions{
							WebTest: &pb.WebTest{
								Status:     pb.WebTest_TIMEOUT,
								IsExpected: true,
							},
						}
						createTestResults(req, expected)
					})
					t.Run("expected skip", func(t *ftt.Test) {
						legacyResult.TestResult.Status = pb.TestStatus_SKIP
						legacyResult.TestResult.Expected = true

						expected[1].Status = pb.TestStatus_SKIP
						expected[1].Expected = true
						expected[1].StatusV2 = pb.TestResult_SKIPPED
						createTestResults(req, expected)
					})
					t.Run("unexpected skip", func(t *ftt.Test) {
						legacyResult.TestResult.Status = pb.TestStatus_SKIP
						legacyResult.TestResult.Expected = false

						expected[1].Status = pb.TestStatus_SKIP
						expected[1].Expected = false
						expected[1].StatusV2 = pb.TestResult_EXECUTION_ERRORED
						createTestResults(req, expected)
					})
				})
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

func TestStatusV1Roundtrip(t *testing.T) {
	t.Parallel()
	ftt.Run(`StatusV1Roundtrip`, t, func(t *ftt.Test) {
		for _, tc := range []struct {
			status   pb.TestStatus
			expected bool
		}{
			{pb.TestStatus_PASS, true},
			{pb.TestStatus_PASS, false},
			{pb.TestStatus_FAIL, true},
			{pb.TestStatus_FAIL, false},
			{pb.TestStatus_ABORT, true},
			{pb.TestStatus_ABORT, false},
			{pb.TestStatus_CRASH, false},
			{pb.TestStatus_CRASH, false},
			{pb.TestStatus_SKIP, true},
			{pb.TestStatus_SKIP, false},
		} {
			statusV2, kind, webTest := statusV2FromV1(tc.status, tc.expected)
			status, expected := statusV1FromV2(statusV2, kind, webTest)
			assert.Loosely(t, status, should.Equal(tc.status))
			assert.Loosely(t, expected, should.Equal(tc.expected))
		}
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
