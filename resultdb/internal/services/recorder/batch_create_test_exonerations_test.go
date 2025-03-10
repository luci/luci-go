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

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

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

func TestValidateBatchCreateTestExonerationsRequest(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestValidateBatchCreateTestExonerationsRequest`, t, func(t *ftt.Test) {
		cfg, err := config.NewCompiledServiceConfig(config.CreatePlaceHolderServiceConfig(), "revision")
		assert.NoErr(t, err)

		t.Run(`Empty`, func(t *ftt.Test) {
			err := validateBatchCreateTestExonerationsRequest(&pb.BatchCreateTestExonerationsRequest{}, cfg)
			assert.Loosely(t, err, should.ErrLike(`invocation: unspecified`))
		})

		t.Run(`Invalid invocation`, func(t *ftt.Test) {
			err := validateBatchCreateTestExonerationsRequest(&pb.BatchCreateTestExonerationsRequest{
				Invocation: "x",
			}, cfg)
			assert.Loosely(t, err, should.ErrLike(`invocation: does not match`))
		})

		t.Run(`Invalid request id`, func(t *ftt.Test) {
			err := validateBatchCreateTestExonerationsRequest(&pb.BatchCreateTestExonerationsRequest{
				Invocation: "invocations/a",
				RequestId:  "😃",
			}, cfg)
			assert.Loosely(t, err, should.ErrLike(`request_id: does not match`))
		})

		t.Run(`Too many requests`, func(t *ftt.Test) {
			err := validateBatchCreateTestExonerationsRequest(&pb.BatchCreateTestExonerationsRequest{
				Invocation: "invocations/a",
				Requests:   make([]*pb.CreateTestExonerationRequest, 1000),
			}, cfg)
			assert.Loosely(t, err, should.ErrLike(`the number of requests in the batch`))
			assert.Loosely(t, err, should.ErrLike(`exceeds 500`))
		})

		t.Run(`Invalid sub-request`, func(t *ftt.Test) {
			err := validateBatchCreateTestExonerationsRequest(&pb.BatchCreateTestExonerationsRequest{
				Invocation: "invocations/inv",
				Requests: []*pb.CreateTestExonerationRequest{
					{
						TestExoneration: &pb.TestExoneration{
							TestId:          "\x01",
							ExplanationHtml: "Unexpected pass.",
							Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
						},
					},
				},
			}, cfg)
			assert.Loosely(t, err, should.ErrLike(`requests[0]: test_exoneration: test_id: non-printable rune`))
		})

		t.Run(`Inconsistent invocation`, func(t *ftt.Test) {
			err := validateBatchCreateTestExonerationsRequest(&pb.BatchCreateTestExonerationsRequest{
				Invocation: "invocations/inv",
				Requests: []*pb.CreateTestExonerationRequest{
					{
						Invocation: "invocations/x",
						TestExoneration: &pb.TestExoneration{
							TestId:          "ninja://ab/cd.ef",
							ExplanationHtml: "Unexpected pass.",
							Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
						},
					},
				},
			}, cfg)
			assert.Loosely(t, err, should.ErrLike(`requests[0]: invocation: inconsistent with top-level invocation`))
		})

		t.Run(`Inconsistent request_id`, func(t *ftt.Test) {
			err := validateBatchCreateTestExonerationsRequest(&pb.BatchCreateTestExonerationsRequest{
				Invocation: "invocations/inv",
				RequestId:  "req1",
				Requests: []*pb.CreateTestExonerationRequest{
					{
						RequestId: "req2",
						TestExoneration: &pb.TestExoneration{
							TestId:          "ninja://ab/cd.ef",
							ExplanationHtml: "Unexpected pass.",
							Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
						},
					},
				},
			}, cfg)
			assert.Loosely(t, err, should.ErrLike(`requests[0]: request_id: inconsistent with top-level request_id`))
		})

		t.Run(`Valid`, func(t *ftt.Test) {
			err := validateBatchCreateTestExonerationsRequest(&pb.BatchCreateTestExonerationsRequest{
				Invocation: "invocations/inv",
				Requests: []*pb.CreateTestExonerationRequest{
					{
						TestExoneration: &pb.TestExoneration{
							TestId:          "ninja://ab/cd.ef",
							ExplanationHtml: "Test failed also when tried without patch.",
							Reason:          pb.ExonerationReason_OCCURS_ON_MAINLINE,
						},
					},
					{
						TestExoneration: &pb.TestExoneration{
							TestId:          "ninja://ab/cd.ef",
							Variant:         pbutil.Variant("a/b", "1", "c", "2"),
							ExplanationHtml: "Test is known to be failing on other CLs.",
							Reason:          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
						},
					},
				},
				RequestId: "a",
			}, cfg)
			assert.Loosely(t, err, should.BeNil)
		})
	})
}

func TestBatchCreateTestExonerations(t *testing.T) {
	ftt.Run(`TestBatchCreateTestExonerations`, t, func(t *ftt.Test) {
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
			req := &pb.BatchCreateTestExonerationsRequest{
				Invocation: "x",
			}
			_, err := recorder.BatchCreateTestExonerations(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`bad request: invocation: does not match`))
		})

		t.Run(`No invocation`, func(t *ftt.Test) {
			req := &pb.BatchCreateTestExonerationsRequest{
				Invocation: "invocations/inv",
			}
			_, err := recorder.BatchCreateTestExonerations(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike(`invocations/inv not found`))
		})

		e2eTest := func(req *pb.BatchCreateTestExonerationsRequest, expected []*pb.TestExoneration) {

			// Insert the invocation.
			testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))

			res, err := recorder.BatchCreateTestExonerations(ctx, req)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, res.TestExonerations, should.HaveLength(len(req.Requests)))
			for i := range req.Requests {
				actual := res.TestExonerations[i]

				expectedDB := proto.Clone(expected[i]).(*pb.TestExoneration)
				proto.Merge(expectedDB, &pb.TestExoneration{
					Name:          pbutil.TestExonerationName("inv", expectedDB.TestId, actual.ExonerationId),
					ExonerationId: actual.ExonerationId, // Accept the server-assigned ID.
				})

				expectedWire := proto.Clone(expectedDB).(*pb.TestExoneration)
				if req.Requests[i].TestExoneration.TestIdStructured == nil {
					// If the request from a legacy client, expect TestVariantIdentifier
					// to be unset on the response.
					expectedWire.TestIdStructured = nil
				}

				assert.Loosely(t, actual, should.Match(expectedWire))

				// Now check the database.
				row, err := exonerations.Read(span.Single(ctx), actual.Name)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, row, should.Match(expectedDB))
			}

			if req.RequestId != "" {
				// Test idempotency.
				res2, err := recorder.BatchCreateTestExonerations(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res2, should.Match(res))
			}
		}

		req := &pb.BatchCreateTestExonerationsRequest{
			Invocation: "invocations/inv",
			Requests: []*pb.CreateTestExonerationRequest{
				{
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
				},
				{
					TestExoneration: &pb.TestExoneration{
						TestIdStructured: &pb.TestIdentifier{
							ModuleName:    "//infra/junit_tests",
							ModuleScheme:  "junit",
							ModuleVariant: pbutil.Variant("key", "value"),
							CoarseName:    "org.chromium.go.luci",
							FineName:      "BTests",
							CaseName:      "B",
						},
						ExplanationHtml: "Unexpected pass.",
						Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
					},
				},
			},
		}
		expected := []*pb.TestExoneration{
			{
				TestIdStructured: &pb.TestIdentifier{
					ModuleName:        "//infra/junit_tests",
					ModuleScheme:      "junit",
					ModuleVariant:     pbutil.Variant("key", "value"),
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
					CoarseName:        "org.chromium.go.luci",
					FineName:          "ATests",
					CaseName:          "A",
				},
				TestId:          "://infra/junit_tests!junit:org.chromium.go.luci:ATests#A",
				Variant:         pbutil.Variant("key", "value"),
				VariantHash:     pbutil.VariantHash(pbutil.Variant("key", "value")),
				ExplanationHtml: "Unexpected pass.",
				Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
			},
			{
				TestIdStructured: &pb.TestIdentifier{
					ModuleName:        "//infra/junit_tests",
					ModuleScheme:      "junit",
					ModuleVariant:     pbutil.Variant("key", "value"),
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
					CoarseName:        "org.chromium.go.luci",
					FineName:          "BTests",
					CaseName:          "B",
				},
				TestId:          "://infra/junit_tests!junit:org.chromium.go.luci:BTests#B",
				Variant:         pbutil.Variant("key", "value"),
				VariantHash:     pbutil.VariantHash(pbutil.Variant("key", "value")),
				ExplanationHtml: "Unexpected pass.",
				Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
			},
		}

		t.Run(`Without request id, e2e`, func(t *ftt.Test) {
			e2eTest(req, expected)
		})
		t.Run(`With request id, e2e`, func(t *ftt.Test) {
			req.RequestId = "request id"
			e2eTest(req, expected)
		})

		t.Run(`Legacy uploader`, func(t *ftt.Test) {
			req := &pb.BatchCreateTestExonerationsRequest{
				Invocation: "invocations/inv",
				Requests: []*pb.CreateTestExonerationRequest{
					{
						TestExoneration: &pb.TestExoneration{
							TestId:          "://infra/junit_tests!junit:org.chromium.go.luci:ATests#A",
							Variant:         pbutil.Variant("a", "1", "b", "2"),
							ExplanationHtml: "Unexpected pass.",
							Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
						},
					},
					{
						TestExoneration: &pb.TestExoneration{
							TestId:          "b/c",
							Variant:         pbutil.Variant("a", "1", "b", "2"),
							ExplanationHtml: "Unexpected pass.",
							Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
						},
					},
				},
			}
			expected := []*pb.TestExoneration{
				{
					TestIdStructured: &pb.TestIdentifier{
						ModuleName:        "//infra/junit_tests",
						ModuleScheme:      "junit",
						ModuleVariant:     pbutil.Variant("a", "1", "b", "2"),
						ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("a", "1", "b", "2")),
						CoarseName:        "org.chromium.go.luci",
						FineName:          "ATests",
						CaseName:          "A",
					},
					TestId:          "://infra/junit_tests!junit:org.chromium.go.luci:ATests#A",
					Variant:         pbutil.Variant("a", "1", "b", "2"),
					VariantHash:     pbutil.VariantHash(pbutil.Variant("a", "1", "b", "2")),
					ExplanationHtml: "Unexpected pass.",
					Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
				},
				{
					TestIdStructured: &pb.TestIdentifier{
						ModuleName:        "legacy",
						ModuleScheme:      "legacy",
						ModuleVariant:     pbutil.Variant("a", "1", "b", "2"),
						ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("a", "1", "b", "2")),
						CoarseName:        "",
						FineName:          "",
						CaseName:          "b/c",
					},
					TestId:          "b/c",
					Variant:         pbutil.Variant("a", "1", "b", "2"),
					VariantHash:     pbutil.VariantHash(pbutil.Variant("a", "1", "b", "2")),
					ExplanationHtml: "Unexpected pass.",
					Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
				},
			}

			t.Run(`Without request id, e2e`, func(t *ftt.Test) {
				e2eTest(req, expected)
			})
			t.Run(`With request id, e2e`, func(t *ftt.Test) {
				req.RequestId = "request id"
				e2eTest(req, expected)
			})
		})
	})
}
