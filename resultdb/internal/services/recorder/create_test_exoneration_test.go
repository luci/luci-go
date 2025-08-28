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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/exonerations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestValidateCreateTestExonerationRequest(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestValidateCreateTestExonerationRequest`, t, func(t *ftt.Test) {
		cfg, err := config.NewCompiledServiceConfig(config.CreatePlaceHolderServiceConfig(), "revision")
		assert.NoErr(t, err)

		req := &pb.CreateTestExonerationRequest{
			Invocation: "invocations/inv",
			TestExoneration: &pb.TestExoneration{
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
				ExplanationHtml: "The test also failed without patch",
				Reason:          pb.ExonerationReason_OCCURS_ON_MAINLINE,
			},
		}

		t.Run(`Empty invocation`, func(t *ftt.Test) {
			req.Invocation = ""
			err := validateCreateTestExonerationRequest(req, cfg, true)
			assert.Loosely(t, err, should.ErrLike(`invocation: unspecified`))
		})

		t.Run(`Empty exoneration`, func(t *ftt.Test) {
			req.TestExoneration = nil
			err := validateCreateTestExonerationRequest(req, cfg, true)
			assert.Loosely(t, err, should.ErrLike(`test_exoneration: unspecified`))
		})
		t.Run(`Invalid exoneration`, func(t *ftt.Test) {
			// There are many more cases, but these are already tested in the test for
			// validateTestExoneration. This test is to ensure that method is called.
			req.TestExoneration.TestIdStructured.CaseName = "test\x00"
			err := validateCreateTestExonerationRequest(req, cfg, true)
			assert.Loosely(t, err, should.ErrLike(`test_exoneration: test_id_structured: case_name: non-printable rune '\x00' at byte index 4`))
		})

		t.Run(`Request id`, func(t *ftt.Test) {
			req.RequestId = "\u0123"
			err := validateCreateTestExonerationRequest(req, cfg, true)
			assert.Loosely(t, err, should.ErrLike(`request_id: does not match`))
		})

		t.Run(`Valid request`, func(t *ftt.Test) {
			err := validateCreateTestExonerationRequest(req, cfg, true)
			assert.Loosely(t, err, should.BeNil)
		})
	})
}

func TestValidateTestExoneration(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestValidateTestExoneration`, t, func(t *ftt.Test) {
		cfg, err := config.NewCompiledServiceConfig(config.CreatePlaceHolderServiceConfig(), "revision")
		assert.NoErr(t, err)

		t.Run(`Unspecified`, func(t *ftt.Test) {
			err := validateTestExoneration(nil, cfg)
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
			err := validateTestExoneration(ex, cfg)
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run("Structured test identifier", func(t *ftt.Test) {
			t.Run("Structure", func(t *ftt.Test) {
				// ParseAndValidateTestID has its own extensive test cases, these do not need to be repeated here.
				t.Run(`Invalid case name`, func(t *ftt.Test) {
					ex.TestIdStructured.CaseName = "case name \x00"
					assert.Loosely(t, validateTestExoneration(ex, cfg), should.ErrLike("test_id_structured: case_name: non-printable rune '\\x00' at byte index 10"))
				})
				t.Run(`Invalid module variant`, func(t *ftt.Test) {
					ex.TestIdStructured.ModuleVariant = pbutil.Variant("key\x00", "value")
					assert.Loosely(t, validateTestExoneration(ex, cfg), should.ErrLike("test_id_structured: module_variant: \"key\\x00\":\"value\": key: does not match pattern"))
				})
			})
			t.Run("Scheme", func(t *ftt.Test) {
				// Only test a couple of cases to make sure ValidateTestIDToScheme is correctly invoked.
				// That method has its own extensive test cases, which don't need to be repeated here.
				t.Run("Scheme not defined", func(t *ftt.Test) {
					ex.TestIdStructured.ModuleScheme = "undefined"
					assert.Loosely(t, validateTestExoneration(ex, cfg), should.ErrLike("test_id_structured: module_scheme: scheme \"undefined\" is not a known scheme by the ResultDB deployment"))
				})
				t.Run(`Coarse name missing`, func(t *ftt.Test) {
					ex.TestIdStructured = nil
					ex.TestId = ":myModule!junit::Class#Method"
					assert.Loosely(t, validateTestExoneration(ex, cfg), should.ErrLike("test_id: coarse_name: required, please set a Package (scheme \"junit\")"))
				})
			})
		})
		t.Run("Legacy", func(t *ftt.Test) {
			t.Run("Legacy fields are ignored unless structured test identifier cleared", func(t *ftt.Test) {
				ex.TestId = "something\x00"
				ex.Variant = pbutil.Variant("", "")
				err := validateTestExoneration(ex, cfg)
				assert.Loosely(t, err, should.BeNil)
			})

			ex.TestIdStructured = nil
			ex.TestId = "something"
			ex.Variant = pbutil.Variant("a", "1", "b", "2")

			t.Run(`Valid`, func(t *ftt.Test) {
				err := validateTestExoneration(ex, cfg)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("Test ID", func(t *ftt.Test) {
				t.Run(`Bad scheme`, func(t *ftt.Test) {
					ex.TestId = ":MyModule!undefined:Package:Class#Method"
					err := validateTestExoneration(ex, cfg)
					assert.Loosely(t, err, should.ErrLike("test_id: module_scheme: scheme \"undefined\" is not a known scheme by the ResultDB deployment"))
				})
				t.Run(`Non-printable runes`, func(t *ftt.Test) {
					ex.TestId = "\x01"
					err := validateTestExoneration(ex, cfg)
					assert.Loosely(t, err, should.ErrLike("test_id: non-printable rune"))
				})
			})
			t.Run("Variant/VariantHash", func(t *ftt.Test) {
				t.Run(`Mismatching variant hashes`, func(t *ftt.Test) {
					ex.VariantHash = "doesn't match"
					err := validateTestExoneration(ex, cfg)
					assert.Loosely(t, err, should.ErrLike(`computed and supplied variant hash don't match`))
				})
				t.Run(`Matching variant hashes`, func(t *ftt.Test) {
					ex.Variant = pbutil.Variant("a", "b")
					ex.VariantHash = "c467ccce5a16dc72"
					err := validateTestExoneration(ex, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run(`Variant hash only`, func(t *ftt.Test) {
					// Unfortunately we have to support this case for legacy clients, even though
					// it means we are unable to capture a Variant.
					ex.Variant = nil
					ex.VariantHash = "c467ccce5a16dc72"

					err := validateTestExoneration(ex, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run(`Invalid variant`, func(t *ftt.Test) {
					ex.Variant = pbutil.Variant("", "")
					err := validateTestExoneration(ex, cfg)
					assert.Loosely(t, err, should.ErrLike(`variant: "":"": key: unspecified`))
				})
			})
		})
		t.Run(`Reason is not specified`, func(t *ftt.Test) {
			ex.Reason = pb.ExonerationReason_EXONERATION_REASON_UNSPECIFIED
			err := validateTestExoneration(ex, cfg)
			assert.Loosely(t, err, should.ErrLike(`reason: unspecified`))
		})
		t.Run(`Explanation HTML not specified`, func(t *ftt.Test) {
			ex.ExplanationHtml = ""
			err := validateTestExoneration(ex, cfg)
			assert.Loosely(t, err, should.ErrLike(`explanation_html: unspecified`))
		})
	})
}

func TestCreateTestExoneration(t *testing.T) {
	ftt.Run(`TestCreateTestExoneration`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx) // For config in-process cache.
		ctx = memory.Use(ctx)                    // For config datastore cache.

		err := config.SetServiceConfigForTesting(ctx, config.CreatePlaceHolderServiceConfig())
		assert.NoErr(t, err)

		recorder := newTestRecorderServer()

		token, err := generateInvocationToken(ctx, "inv")
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		t.Run(`Invalid request`, func(t *ftt.Test) {
			req := &pb.CreateTestExonerationRequest{
				Invocation: "invocations/inv",
				TestExoneration: &pb.TestExoneration{
					TestId:          "\x01",
					ExplanationHtml: "Unexpected pass.",
					Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
				},
			}
			_, err := recorder.CreateTestExoneration(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`bad request: test_exoneration: test_id: non-printable rune`))
		})

		t.Run(`No invocation`, func(t *ftt.Test) {
			req := &pb.CreateTestExonerationRequest{
				Invocation: "invocations/inv",
				TestExoneration: &pb.TestExoneration{
					TestId:          "a",
					ExplanationHtml: "Unexpected pass.",
					Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
				},
			}
			_, err := recorder.CreateTestExoneration(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike(`invocations/inv not found`))
		})

		e2eTest := func(req *pb.CreateTestExonerationRequest, expected *pb.TestExoneration, expectedId string) {
			// Insert the invocation.
			testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))

			res, err := recorder.CreateTestExoneration(ctx, req)
			assert.Loosely(t, err, should.BeNil)

			// Check the Exoneration ID.
			if expectedId == "" {
				assert.Loosely(t, res.ExonerationId, should.HavePrefix(expected.VariantHash+":"))
			} else {
				assert.Loosely(t, res.ExonerationId, should.Equal(expected.VariantHash+":"+expectedId))
			}

			expectedWireProto := proto.Clone(expected).(*pb.TestExoneration)
			expectedWireProto.ExonerationId = res.ExonerationId
			expectedWireProto.Name = pbutil.LegacyTestExonerationName("inv", expected.TestId, res.ExonerationId)
			if req.TestExoneration.TestIdStructured == nil {
				// If the request is a legacy request, do not expect the structured test identifier
				// to be set on the response. This is to minimise changes to existing clients.
				expectedWireProto.TestIdStructured = nil
			}
			assert.Loosely(t, res, should.Match(expectedWireProto))

			// Now check the database.
			expectedDBProto := proto.Clone(expected).(*pb.TestExoneration)
			expectedDBProto.ExonerationId = res.ExonerationId
			expectedDBProto.Name = pbutil.LegacyTestExonerationName("inv", expected.TestId, res.ExonerationId)

			row, err := exonerations.Read(span.Single(ctx), res.Name)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, row, should.Match(expectedDBProto))

			if req.RequestId != "" {
				// Test idempotency.
				res2, err := recorder.CreateTestExoneration(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res2, should.Match(res))
			}
		}

		request := &pb.CreateTestExonerationRequest{
			Invocation: "invocations/inv",
			TestExoneration: &pb.TestExoneration{
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
				ExplanationHtml: "Test is known flaky. Similar test failures have been observed in other CLs.",
				Reason:          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
			},
		}
		expected := &pb.TestExoneration{
			TestIdStructured: &pb.TestIdentifier{
				ModuleName:   "//infra/junit_tests",
				ModuleScheme: "junit",
				ModuleVariant: pbutil.Variant(
					"key", "value",
				),
				ModuleVariantHash: "5d8482c3056d8635",
				CoarseName:        "org.chromium.go.luci",
				FineName:          "ValidationTests",
				CaseName:          "FooBar",
			},
			TestId:          "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar",
			Variant:         pbutil.Variant("key", "value"),
			VariantHash:     "5d8482c3056d8635",
			ExplanationHtml: "Test is known flaky. Similar test failures have been observed in other CLs.",
			Reason:          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
		}
		t.Run(`Without request id, e2e`, func(t *ftt.Test) {
			e2eTest(request, expected, "")
		})
		t.Run(`With request id, e2e`, func(t *ftt.Test) {
			request.RequestId = "request id"
			e2eTest(request, expected, "d:2960f0231ce23039cdf7d4a62e31939ecd897bbf465e0fb2d35bf425ae1c5ae14eb0714d6dd0a0c244eaa66ae2b645b0637f58e91ed1b820bb1f01d8d4a72e67")
		})

		t.Run(`Legacy Uploader`, func(t *ftt.Test) {
			request := &pb.CreateTestExonerationRequest{
				RequestId:  "request id",
				Invocation: "invocations/inv",
				TestExoneration: &pb.TestExoneration{
					TestId:          "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar",
					Variant:         pbutil.Variant("a", "1", "b", "2"),
					ExplanationHtml: "Test also failed when tried without patch.",
					Reason:          pb.ExonerationReason_OCCURS_ON_MAINLINE,
				},
			}
			expected := &pb.TestExoneration{
				TestIdStructured: &pb.TestIdentifier{
					ModuleName:        "//infra/junit_tests",
					ModuleScheme:      "junit",
					ModuleVariant:     pbutil.Variant("a", "1", "b", "2"),
					ModuleVariantHash: "6408fdc5c36df5df",
					CoarseName:        "org.chromium.go.luci",
					FineName:          "ValidationTests",
					CaseName:          "FooBar",
				},
				TestId:          "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar",
				Variant:         pbutil.Variant("a", "1", "b", "2"),
				VariantHash:     "6408fdc5c36df5df",
				ExplanationHtml: "Test also failed when tried without patch.",
				Reason:          pb.ExonerationReason_OCCURS_ON_MAINLINE,
			}
			t.Run(`Base case`, func(t *ftt.Test) {
				e2eTest(request, expected, "d:2960f0231ce23039cdf7d4a62e31939ecd897bbf465e0fb2d35bf425ae1c5ae14eb0714d6dd0a0c244eaa66ae2b645b0637f58e91ed1b820bb1f01d8d4a72e67")
			})
			t.Run(`With hash only, e2e`, func(t *ftt.Test) {
				request.TestExoneration.Variant = nil
				request.TestExoneration.VariantHash = "deadbeefdeadbeef"

				expected.Variant = &pb.Variant{}
				expected.VariantHash = "deadbeefdeadbeef"
				expected.TestIdStructured.ModuleVariant = &pb.Variant{}
				expected.TestIdStructured.ModuleVariantHash = "deadbeefdeadbeef"

				e2eTest(request, expected, "d:2960f0231ce23039cdf7d4a62e31939ecd897bbf465e0fb2d35bf425ae1c5ae14eb0714d6dd0a0c244eaa66ae2b645b0637f58e91ed1b820bb1f01d8d4a72e67")
			})
		})
	})
}
