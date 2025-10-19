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

	"cloud.google.com/go/spanner"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/span"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/exonerations"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestValidateTestExoneration(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestValidateTestExoneration`, t, func(t *ftt.Test) {
		cfg, err := config.NewCompiledServiceConfig(config.CreatePlaceHolderServiceConfig(), "revision")
		assert.NoErr(t, err)

		t.Run(`Unspecified`, func(t *ftt.Test) {
			err := validateTestExoneration(nil, cfg, true)
			assert.Loosely(t, err, should.ErrLike(`unspecified`))
		})

		ex := &pb.TestExoneration{
			TestIdStructured: &pb.TestIdentifier{
				ModuleName:   "//infra/junit_tests",
				ModuleScheme: "junit",
				ModuleVariant: pbutil.Variant(
					"key", "value",
				),
				CoarseName: "org.chromium.go.luci",
				FineName:   "ValidationTests",
				CaseName:   "FooBar",
			},
			ExplanationHtml: "Unexpected pass.",
			Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
		}
		t.Run(`Valid`, func(t *ftt.Test) {
			err := validateTestExoneration(ex, cfg, true)
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run("Structured test identifier", func(t *ftt.Test) {
			t.Run("Structure", func(t *ftt.Test) {
				// ParseAndValidateTestID has its own extensive test cases, these do not need to be repeated here.
				t.Run(`Invalid case name`, func(t *ftt.Test) {
					ex.TestIdStructured.CaseName = "case name \x00"
					assert.Loosely(t, validateTestExoneration(ex, cfg, true), should.ErrLike("test_id_structured: case_name: non-printable rune '\\x00' at byte index 10"))
				})
				t.Run(`Invalid module variant`, func(t *ftt.Test) {
					ex.TestIdStructured.ModuleVariant = pbutil.Variant("key\x00", "value")
					assert.Loosely(t, validateTestExoneration(ex, cfg, true), should.ErrLike("test_id_structured: module_variant: \"key\\x00\":\"value\": key: does not match pattern"))
				})
			})
			t.Run("Scheme", func(t *ftt.Test) {
				// Only test a couple of cases to make sure ValidateTestIDToScheme is correctly invoked.
				// That method has its own extensive test cases, which don't need to be repeated here.
				t.Run("Scheme not defined", func(t *ftt.Test) {
					ex.TestIdStructured.ModuleScheme = "undefined"
					assert.Loosely(t, validateTestExoneration(ex, cfg, true), should.ErrLike("test_id_structured: module_scheme: scheme \"undefined\" is not a known scheme by the ResultDB deployment"))
				})
				t.Run(`Coarse name missing`, func(t *ftt.Test) {
					ex.TestIdStructured = nil
					ex.TestId = ":myModule!junit::Class#Method"
					ex.Variant = pbutil.Variant("a", "1", "b", "2")
					assert.Loosely(t, validateTestExoneration(ex, cfg, true), should.ErrLike("test_id: coarse_name: required, please set a Package (scheme \"junit\")"))
				})
			})
		})
		t.Run("Legacy", func(t *ftt.Test) {
			t.Run("Legacy fields are ignored unless structured test identifier cleared", func(t *ftt.Test) {
				ex.TestId = "something\x00"
				ex.Variant = pbutil.Variant("", "")
				err := validateTestExoneration(ex, cfg, true)
				assert.Loosely(t, err, should.BeNil)
			})

			ex.TestIdStructured = nil
			ex.TestId = "something"
			ex.Variant = pbutil.Variant("a", "1", "b", "2")

			t.Run(`Valid`, func(t *ftt.Test) {
				err := validateTestExoneration(ex, cfg, true)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("Test ID", func(t *ftt.Test) {
				t.Run(`Bad scheme`, func(t *ftt.Test) {
					ex.TestId = ":MyModule!undefined:Package:Class#Method"
					err := validateTestExoneration(ex, cfg, true)
					assert.Loosely(t, err, should.ErrLike("test_id: module_scheme: scheme \"undefined\" is not a known scheme by the ResultDB deployment"))
				})
				t.Run(`Non-printable runes`, func(t *ftt.Test) {
					ex.TestId = "\x01"
					err := validateTestExoneration(ex, cfg, true)
					assert.Loosely(t, err, should.ErrLike("test_id: non-printable rune"))
				})
			})
			t.Run("Variant/VariantHash", func(t *ftt.Test) {
				t.Run(`Mismatching variant hashes`, func(t *ftt.Test) {
					ex.VariantHash = "doesn't match"
					err := validateTestExoneration(ex, cfg, true)
					assert.Loosely(t, err, should.ErrLike(`computed and supplied variant hash don't match`))
				})
				t.Run(`Matching variant hashes`, func(t *ftt.Test) {
					ex.Variant = pbutil.Variant("a", "b")
					ex.VariantHash = "c467ccce5a16dc72"
					err := validateTestExoneration(ex, cfg, true)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run(`Variant hash only`, func(t *ftt.Test) {
					// Unfortunately we have to support this case for legacy clients, even though
					// it means we are unable to capture a Variant.
					ex.Variant = nil
					ex.VariantHash = "c467ccce5a16dc72"

					t.Run(`Allowed in unstrict mode`, func(t *ftt.Test) {
						err := validateTestExoneration(ex, cfg, false)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run(`Disallowed in strict mode`, func(t *ftt.Test) {
						err := validateTestExoneration(ex, cfg, true)
						assert.Loosely(t, err, should.ErrLike(`variant: unspecified`))
					})
				})
				t.Run(`Invalid variant`, func(t *ftt.Test) {
					ex.Variant = pbutil.Variant("", "")
					err := validateTestExoneration(ex, cfg, true)
					assert.Loosely(t, err, should.ErrLike(`variant: "":"": key: unspecified`))
				})
			})
		})
		t.Run(`Reason is not specified`, func(t *ftt.Test) {
			ex.Reason = pb.ExonerationReason_EXONERATION_REASON_UNSPECIFIED
			err := validateTestExoneration(ex, cfg, true)
			assert.Loosely(t, err, should.ErrLike(`reason: unspecified`))
		})
		t.Run(`Explanation HTML not specified`, func(t *ftt.Test) {
			ex.ExplanationHtml = ""
			err := validateTestExoneration(ex, cfg, true)
			assert.Loosely(t, err, should.ErrLike(`explanation_html: unspecified`))
		})
	})
}

func TestCreateTestExoneration(t *testing.T) {
	ftt.Run(`CreateTestExoneration`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx) // For config in-process cache.
		ctx = memory.Use(ctx)                    // For config datastore cache.
		err := config.SetServiceConfigForTesting(ctx, config.CreatePlaceHolderServiceConfig())
		assert.NoErr(t, err)

		recorder := newTestRecorderServer()

		rootInvID := rootinvocations.ID("root-inv-id")
		wuID := workunits.ID{
			RootInvocationID: "root-inv-id",
			WorkUnitID:       "work-unit:child-1",
		}

		// Create some sample work units and a sample invocation.
		var ms []*spanner.Mutation
		ms = append(ms, insert.RootInvocationWithRootWorkUnit(rootinvocations.NewBuilder(rootInvID).Build())...)
		ms = append(ms, insert.WorkUnit(workunits.NewBuilder(rootInvID, "work-unit:child-1").WithFinalizationState(pb.WorkUnit_ACTIVE).Build())...)
		testutil.MustApply(ctx, t, ms...)

		req := &pb.CreateTestExonerationRequest{
			Parent: wuID.Name(),
			TestExoneration: &pb.TestExoneration{
				TestIdStructured: &pb.TestIdentifier{
					ModuleName:    "//infra/junit_tests",
					ModuleScheme:  "junit",
					ModuleVariant: pbutil.Variant("key", "value"),
					CoarseName:    "org.chromium.go.luci",
					FineName:      "ATests",
					CaseName:      "A",
				},
				ExplanationHtml: "Unexpected pass.",
				Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
			},
			RequestId: "request-id",
		}

		token, err := generateWorkUnitUpdateToken(ctx, wuID)
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		// This RPC largely reuses the implementation in BatchCreateTestExonerations.
		// With any errors returned, we check carefully that they do not
		// refer to "requests[0]" and refer to fields in this request.
		t.Run("request validation", func(t *ftt.Test) {
			t.Run("parent", func(t *ftt.Test) {
				t.Run("unspecified", func(t *ftt.Test) {
					req.Parent = ""
					_, err := recorder.CreateTestExoneration(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: parent: unspecified"))
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.Parent = "invalid value"
					_, err := recorder.CreateTestExoneration(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: parent: does not match pattern"))
				})
				t.Run("both parent and invocation set", func(t *ftt.Test) {
					req.Parent = wuID.Name()
					req.Invocation = "invocations/inv-id"
					_, err := recorder.CreateTestExoneration(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: invocation: must not be specified if parent is specified"))
				})
			})
			t.Run("invocation", func(t *ftt.Test) {
				req.Parent = ""
				t.Run("invalid", func(t *ftt.Test) {
					req.Invocation = "this is an invalid invocation name"
					_, err := recorder.CreateTestExoneration(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: invocation: does not match"))
				})
			})
			t.Run("test exoneration", func(t *ftt.Test) {
				// validateTestExoneration has its own tests, so only test a few cases to make sure it is correctly integrated.
				t.Run("test ID using invalid scheme", func(t *ftt.Test) {
					req.TestExoneration.TestIdStructured.ModuleScheme = "undefined"
					_, err := recorder.CreateTestExoneration(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: test_exoneration: test_id_structured: module_scheme: scheme \"undefined\" is not a known scheme"))
				})
				t.Run("invalid test_exoneration", func(t *ftt.Test) {
					req.TestExoneration.TestIdStructured = nil
					_, err := recorder.CreateTestExoneration(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: test_exoneration: test_id_structured: unspecified"))
				})
			})
			t.Run("request ID", func(t *ftt.Test) {
				t.Run("unspecified", func(t *ftt.Test) {
					req.RequestId = ""
					_, err := recorder.CreateTestExoneration(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: request_id: unspecified (please provide a per-request UUID to ensure idempotence)"))
				})
				t.Run("invalid", func(t *ftt.Test) {
					// non-ascii character
					req.RequestId = string(rune(244))
					_, err := recorder.CreateTestExoneration(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: request_id: does not match"))
				})
			})
		})
		t.Run("request authorization", func(t *ftt.Test) {
			t.Run("invalid update token", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid-token"))
				_, err := recorder.CreateTestExoneration(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.That(t, err, should.ErrLike(`invalid update token`))
			})
			t.Run("missing update token", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.MD{})
				_, err := recorder.CreateTestExoneration(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.Unauthenticated))
				assert.That(t, err, should.ErrLike(`missing update-token metadata value in the request`))
			})
		})
		t.Run("parent", func(t *ftt.Test) {
			t.Run("parent not active", func(t *ftt.Test) {
				testutil.MustApply(ctx, t, spanner.UpdateMap("WorkUnits", map[string]any{
					"RootInvocationShardId": wuID.RootInvocationShardID().RowID(),
					"WorkUnitId":            wuID.WorkUnitID,
					"FinalizationState":     pb.WorkUnit_FINALIZING,
				}))

				_, err := recorder.CreateTestExoneration(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
				assert.Loosely(t, err, should.ErrLike(`desc = parent "rootInvocations/root-inv-id/workUnits/work-unit:child-1" is not active`))
			})
			t.Run("parent not found", func(t *ftt.Test) {
				testutil.MustApply(ctx, t, spanner.Delete("WorkUnits", wuID.Key()))

				_, err := recorder.CreateTestExoneration(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike(`desc = "rootInvocations/root-inv-id/workUnits/work-unit:child-1" not found`))
			})
		})
		t.Run("with work units", func(t *ftt.Test) {
			variantHash := pbutil.VariantHash(pbutil.Variant("key", "value"))
			expectedExonerationID1 := fmt.Sprintf("%s:d:%s", variantHash, deterministicExonerationIDSuffix(ctx, req.RequestId, 0))
			expected := proto.Clone(req.TestExoneration).(*pb.TestExoneration)
			// Populate output-only fields.
			expected.Name = pbutil.TestExonerationName(string(wuID.RootInvocationID), wuID.WorkUnitID, "://infra/junit_tests!junit:org.chromium.go.luci:ATests#A", expectedExonerationID1)
			expected.TestIdStructured.ModuleVariantHash = pbutil.VariantHash(pbutil.Variant("key", "value"))
			expected.TestId = "://infra/junit_tests!junit:org.chromium.go.luci:ATests#A"
			expected.Variant = pbutil.Variant("key", "value")
			expected.VariantHash = pbutil.VariantHash(pbutil.Variant("key", "value"))
			expected.ExonerationId = expectedExonerationID1

			createTestExoneration := func(req *pb.CreateTestExonerationRequest, expected *pb.TestExoneration) {
				res, err := recorder.CreateTestExoneration(ctx, req)
				assert.Loosely(t, err, should.BeNil, truth.LineContext(1))
				assert.Loosely(t, res, should.Match(expected), truth.LineContext(1))

				// double-check it with the database
				row, err := exonerations.Read(span.Single(ctx), res.Name)
				assert.Loosely(t, err, should.BeNil, truth.LineContext(1))
				assert.Loosely(t, row, should.Match(expected), truth.LineContext(1))
			}

			t.Run("base case (success)", func(t *ftt.Test) {
				createTestExoneration(req, expected)
			})
			// Many variations are already covered in BatchCreateTestExonerations
			// tests, no need to repeat them here.
		})
		t.Run("with legacy invocation", func(t *ftt.Test) {
			invID := invocations.ID("u-build-1")
			testutil.MustApply(ctx, t, insert.Invocation(invID, pb.Invocation_ACTIVE, nil))

			tok, err := generateInvocationToken(ctx, "u-build-1")
			assert.Loosely(t, err, should.BeNil)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, tok))

			req := &pb.CreateTestExonerationRequest{
				Invocation: invID.Name(),
				TestExoneration: &pb.TestExoneration{
					TestId:          "://infra/junit_tests!junit:org.chromium.go.luci:ATests#A",
					Variant:         pbutil.Variant("a", "1", "b", "2"),
					ExplanationHtml: "Unexpected pass.",
					Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
				},
				RequestId: "request-id",
			}
			expectedExonerationID := fmt.Sprintf("%s:d:%s", pbutil.VariantHash(pbutil.Variant("a", "1", "b", "2")), deterministicExonerationIDSuffix(ctx, req.RequestId, 0))

			// Expected should be same as requests, plus output only fields set.
			expected := proto.Clone(req.TestExoneration).(*pb.TestExoneration)

			expected.Name = pbutil.LegacyTestExonerationName(string(invID), "://infra/junit_tests!junit:org.chromium.go.luci:ATests#A", expectedExonerationID)
			expected.TestIdStructured = &pb.TestIdentifier{
				ModuleName:        "//infra/junit_tests",
				ModuleScheme:      "junit",
				ModuleVariant:     pbutil.Variant("a", "1", "b", "2"),
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("a", "1", "b", "2")),
				CoarseName:        "org.chromium.go.luci",
				FineName:          "ATests",
				CaseName:          "A",
			}
			expected.VariantHash = pbutil.VariantHash(pbutil.Variant("a", "1", "b", "2"))
			expected.ExonerationId = expectedExonerationID

			t.Run("with a finalized invocation", func(t *ftt.Test) {
				testutil.MustApply(ctx, t, spanutil.UpdateMap("Invocations", map[string]any{
					"InvocationId": invID,
					"State":        pb.Invocation_FINALIZED,
				}))

				_, err := recorder.CreateTestExoneration(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
				assert.Loosely(t, err, should.ErrLike("invocations/u-build-1 is not active"))
			})
			t.Run("with a non-existing invocation", func(t *ftt.Test) {
				tok, err = generateInvocationToken(ctx, "inv")
				assert.Loosely(t, err, should.BeNil)
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, tok))
				req.Invocation = "invocations/inv"

				_, err := recorder.CreateTestExoneration(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("invocations/inv not found"))
			})
			t.Run("with no request ID", func(t *ftt.Test) {
				// This is a legacy behaviour and is valid for test exonerations uploads to invocations only.
				req.RequestId = ""

				// Succeeds.
				_, err := recorder.CreateTestExoneration(ctx, req)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("base case (success)", func(t *ftt.Test) {
				createTestExoneration := func(req *pb.CreateTestExonerationRequest, expected *pb.TestExoneration) {
					res, err := recorder.CreateTestExoneration(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res, should.Match(expected))

					// double-check it with the database
					row, err := exonerations.Read(span.Single(ctx), res.Name)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, row, should.Match(expected))
				}

				createTestExoneration(req, expected)
			})
		})
	})
}
