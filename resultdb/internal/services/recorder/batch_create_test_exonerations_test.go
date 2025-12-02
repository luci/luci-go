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

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/exonerations"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testexonerationsv2"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestBatchCreateTestExonerations(t *testing.T) {
	ftt.Run(`BatchCreateTestExonerations`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx) // For config in-process cache.
		ctx = memory.Use(ctx)                    // For config datastore cache.
		err := config.SetServiceConfigForTesting(ctx, config.CreatePlaceholderServiceConfig())
		assert.NoErr(t, err)

		recorder := newTestRecorderServer()

		rootInvID := rootinvocations.ID("root-inv-id")
		wuID1 := workunits.ID{
			RootInvocationID: "root-inv-id",
			WorkUnitID:       "work-unit:child-1",
		}
		wuID2 := workunits.ID{
			RootInvocationID: "root-inv-id",
			WorkUnitID:       "work-unit:child-2",
		}

		// Create some sample work units and a sample invocation.
		var ms []*spanner.Mutation
		ms = append(ms, insert.RootInvocationWithRootWorkUnit(rootinvocations.NewBuilder(rootInvID).Build())...)
		ms = append(ms, insert.WorkUnit(workunits.NewBuilder(rootInvID, "work-unit:child-1").WithRealm("testproject:child-1-realm").WithFinalizationState(pb.WorkUnit_ACTIVE).Build())...)
		ms = append(ms, insert.WorkUnit(workunits.NewBuilder(rootInvID, "work-unit:child-2").WithRealm("testproject:child-2-realm").WithFinalizationState(pb.WorkUnit_ACTIVE).Build())...)
		testutil.MustApply(ctx, t, ms...)

		req := &pb.BatchCreateTestExonerationsRequest{
			Requests: []*pb.CreateTestExonerationRequest{
				{
					Parent: wuID1.Name(),
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
					Parent: wuID2.Name(),
					TestExoneration: &pb.TestExoneration{
						TestIdStructured: &pb.TestIdentifier{
							ModuleName:    "//infra/special_junit_tests",
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
			RequestId: "request-id",
		}

		token, err := generateWorkUnitUpdateToken(ctx, wuID1)
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		t.Run("request validation", func(t *ftt.Test) {
			t.Run("parent", func(t *ftt.Test) {
				t.Run("invalid", func(t *ftt.Test) {
					req.Parent = "invalid value"
					_, err := recorder.BatchCreateTestExonerations(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("parent: does not match pattern"))
				})
				t.Run("both parent and invocation set", func(t *ftt.Test) {
					req.Parent = wuID1.Name()
					req.Invocation = "invocations/inv-id"
					_, err := recorder.BatchCreateTestExonerations(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("invocation: must not be specified if parent is specified"))
				})
			})
			t.Run("invocation", func(t *ftt.Test) {
				t.Run("invalid", func(t *ftt.Test) {
					req.Parent = ""
					req.Invocation = "this is an invalid invocation name"
					_, err := recorder.BatchCreateTestExonerations(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("invocation: does not match"))
				})
			})
			t.Run(`too many requests`, func(t *ftt.Test) {
				req.Requests = make([]*pb.CreateTestExonerationRequest, 1000)
				_, err := recorder.BatchCreateTestExonerations(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`requests: the number of requests in the batch (1000) exceeds 500`))
			})
			t.Run(`requests too large`, func(t *ftt.Test) {
				req.Requests[0].Parent = strings.Repeat("a", pbutil.MaxBatchRequestSize)
				_, err := recorder.BatchCreateTestExonerations(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`requests: the size of all requests is too large`))
			})
			t.Run("sub-request", func(t *ftt.Test) {
				t.Run("parent", func(t *ftt.Test) {
					t.Run("unspecified", func(t *ftt.Test) {
						req.Requests[1].Parent = ""
						_, err := recorder.BatchCreateTestExonerations(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("requests[1]: parent: unspecified"))
					})
					t.Run("invalid", func(t *ftt.Test) {
						req.Requests[1].Parent = "invalid value"
						_, err := recorder.BatchCreateTestExonerations(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("requests[1]: parent: does not match pattern "))
					})
					t.Run("parents in different root invocations", func(t *ftt.Test) {
						req.Requests[0].Parent = "rootInvocations/root-inv-1/workUnits/work-unit"
						req.Requests[1].Parent = "rootInvocations/root-inv-2/workUnits/work-unit"
						_, err := recorder.BatchCreateTestExonerations(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.That(t, err, should.ErrLike(`requests[1]: parent: all test exonerations in a batch must belong to the same root invocation; got "rootInvocations/root-inv-2", want "rootInvocations/root-inv-1"`))
					})
					t.Run("unmatched parent in requests", func(t *ftt.Test) {
						req.Parent = wuID1.Name()
						_, err := recorder.BatchCreateTestExonerations(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`requests[1]: parent: must be either empty or equal to "rootInvocations/root-inv-id/workUnits/work-unit:child-1", but got "rootInvocations/root-inv-id/workUnits/work-unit:child-2"`))
					})
					t.Run("set in conjuntion with invocation at the top-level", func(t *ftt.Test) {
						req.Invocation = "invocations/foo"
						_, err := recorder.BatchCreateTestExonerations(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("requests[0]: parent: must not be set if `invocation` is set on top-level batch request"))
					})
					t.Run("mixed invocations and parents set at the child request-level", func(t *ftt.Test) {
						req.Requests[1].Invocation = "invocations/foo"
						req.Requests[1].Parent = ""
						_, err := recorder.BatchCreateTestExonerations(ctx, req)
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
						_, err := recorder.BatchCreateTestExonerations(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("requests[1]: invocation: must not be set if `parent` is set on top-level batch request"))
					})
					t.Run("unmatched invocation in requests", func(t *ftt.Test) {
						req.Invocation = "invocations/foo"
						req.Requests[1].Invocation = "invocations/bar"
						_, err := recorder.BatchCreateTestExonerations(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`requests[1]: invocation: must be either empty or equal to "invocations/foo", but got "invocations/bar"`))
					})
				})
				t.Run("test exoneration", func(t *ftt.Test) {
					// validateTestExoneration has its own tests, so only test a few cases to make sure it is correctly integrated.
					t.Run(`unspecified`, func(t *ftt.Test) {
						req.Requests[0].TestExoneration = nil
						_, err := recorder.BatchCreateTestExonerations(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`requests[0]: test_exoneration: unspecified`))
					})
					t.Run("test ID using invalid scheme", func(t *ftt.Test) {
						req.Requests[0].TestExoneration.TestIdStructured.ModuleScheme = "undefined"
						_, err := recorder.BatchCreateTestExonerations(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("requests[0]: test_exoneration: test_id_structured: module_scheme: scheme \"undefined\" is not a known scheme"))
					})
					t.Run("test_exoneration has test_id_structured unspecified", func(t *ftt.Test) {
						req.Requests[0].TestExoneration.TestIdStructured = nil
						_, err := recorder.BatchCreateTestExonerations(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("requests[0]: test_exoneration: test_id_structured: unspecified"))
					})
					t.Run("missing variant", func(t *ftt.Test) {
						// This case is no longer allowed with work units. It was allowed with invocations.
						req.Requests[0].TestExoneration.TestIdStructured = nil
						req.Requests[0].TestExoneration.TestId = "my_test"
						req.Requests[0].TestExoneration.VariantHash = pbutil.VariantHash(pbutil.Variant("mykey", "myvalue"))
						_, err := recorder.BatchCreateTestExonerations(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("requests[0]: test_exoneration: variant: unspecified"))
					})
				})
			})
			t.Run("request ID", func(t *ftt.Test) {
				t.Run("unspecified", func(t *ftt.Test) {
					req.RequestId = ""
					_, err := recorder.BatchCreateTestExonerations(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: request_id: unspecified (please provide a per-request UUID to ensure idempotence)"))
				})
				t.Run("invalid", func(t *ftt.Test) {
					// non-ascii character
					req.RequestId = string(rune(244))
					_, err := recorder.BatchCreateTestExonerations(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: request_id: does not match"))
				})
				t.Run("unmatched request_id in requests", func(t *ftt.Test) {
					req.RequestId = "foo"
					req.Requests[0].RequestId = "bar"
					_, err := recorder.BatchCreateTestExonerations(ctx, req)
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
				_, err := recorder.BatchCreateTestExonerations(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike(`requests[1]: parent: work unit "rootInvocations/root-inv/workUnits/work-unit-2" requires a different update token to request[0]'s "rootInvocations/root-inv/workUnits/work-unit-1", but this RPC only accepts one update token`))
			})
			t.Run("invalid update token", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid-token"))
				_, err := recorder.BatchCreateTestExonerations(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.That(t, err, should.ErrLike(`invalid update token`))
			})
			t.Run("missing update token", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.MD{})
				_, err := recorder.BatchCreateTestExonerations(ctx, req)
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
					_, err := recorder.BatchCreateTestExonerations(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.That(t, err, should.ErrLike(`invalid update token`))
				})
				t.Run("missing update token", func(t *ftt.Test) {
					ctx := metadata.NewIncomingContext(ctx, metadata.MD{})
					_, err := recorder.BatchCreateTestExonerations(ctx, req)
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
					"FinalizationState":     pb.WorkUnit_FINALIZING,
				}))

				_, err := recorder.BatchCreateTestExonerations(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
				assert.Loosely(t, err, should.ErrLike(`requests[1]: parent "rootInvocations/root-inv-id/workUnits/work-unit:child-2" is not active`))
			})
			t.Run("parent not found", func(t *ftt.Test) {
				testutil.MustApply(ctx, t, spanner.Delete("WorkUnits", wuID2.Key()))

				_, err := recorder.BatchCreateTestExonerations(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike(`"rootInvocations/root-inv-id/workUnits/work-unit:child-2" not found`))
			})
		})
		t.Run("with work units", func(t *ftt.Test) {
			variantHash := pbutil.VariantHash(pbutil.Variant("key", "value"))
			expectedExonerationID1 := fmt.Sprintf("%s:d:%s", variantHash, deterministicExonerationIDSuffix(ctx, req.RequestId, 0))
			expectedExonerationID2 := fmt.Sprintf("%s:d:%s", variantHash, deterministicExonerationIDSuffix(ctx, req.RequestId, 1))

			// Expected should be same as requests, plus output only fields set.
			expected := []*pb.TestExoneration{
				proto.Clone(req.Requests[0].TestExoneration).(*pb.TestExoneration),
				proto.Clone(req.Requests[1].TestExoneration).(*pb.TestExoneration),
			}
			expected[0].Name = pbutil.TestExonerationName(string(wuID1.RootInvocationID), wuID1.WorkUnitID, "://infra/junit_tests!junit:org.chromium.go.luci:ATests#A", expectedExonerationID1)
			expected[0].TestIdStructured.ModuleVariantHash = pbutil.VariantHash(pbutil.Variant("key", "value"))
			expected[0].TestId = "://infra/junit_tests!junit:org.chromium.go.luci:ATests#A"
			expected[0].Variant = pbutil.Variant("key", "value")
			expected[0].VariantHash = pbutil.VariantHash(pbutil.Variant("key", "value"))
			expected[0].ExonerationId = expectedExonerationID1
			expected[1].Name = pbutil.TestExonerationName(string(wuID2.RootInvocationID), wuID2.WorkUnitID, "://infra/special_junit_tests!junit:org.chromium.go.luci:BTests#B", expectedExonerationID2)
			expected[1].TestIdStructured.ModuleVariantHash = pbutil.VariantHash(pbutil.Variant("key", "value"))
			expected[1].TestId = "://infra/special_junit_tests!junit:org.chromium.go.luci:BTests#B"
			expected[1].Variant = pbutil.Variant("key", "value")
			expected[1].VariantHash = pbutil.VariantHash(pbutil.Variant("key", "value"))
			expected[1].ExonerationId = expectedExonerationID2

			expectedRealms := []string{"testproject:child-1-realm", "testproject:child-2-realm"}

			createExonerations := func(req *pb.BatchCreateTestExonerationsRequest, expected []*pb.TestExoneration, expectedRealms []string) {
				response, err := recorder.BatchCreateTestExonerations(ctx, req)
				assert.Loosely(t, err, should.BeNil, truth.LineContext(1))

				assert.Loosely(t, len(response.TestExonerations), should.Equal(len(expected)), truth.LineContext(1))
				var updatedExpectations []*pb.TestExoneration
				for i := range req.Requests {
					actual := response.TestExonerations[i]
					expected := proto.Clone(expected[i]).(*pb.TestExoneration)
					if expected.ExonerationId == "" {
						assert.Loosely(t, actual.ExonerationId, should.NotBeNil, truth.LineContext(1))
						expected.ExonerationId = actual.ExonerationId // Accept the server-assigned ID.
					}
					updatedExpectations = append(updatedExpectations, expected)
					assert.Loosely(t, actual, should.Match(expected))

					// Now check the database.
					row, err := exonerations.Read(span.Single(ctx), actual.Name)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, row, should.Match(expected))
				}

				// Assert the created exonerations in the v2 table match the expectations.
				v2Rows, err := testexonerationsv2.ReadAllForTesting(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)

				v2Protos := make([]*pb.TestExoneration, 0, len(v2Rows))
				for i, row := range v2Rows {
					v2Protos = append(v2Protos, row.ToProto())
					assert.That(t, row.Realm, should.Equal(expectedRealms[i]))
				}
				assert.That(t, v2Protos, should.Match(updatedExpectations), truth.LineContext(1))
			}

			t.Run("base case (success)", func(t *ftt.Test) {
				createExonerations(req, expected, expectedRealms)

				t.Run("idempotent", func(t *ftt.Test) {
					createExonerations(req, expected, expectedRealms)
				})
			})
			t.Run("with common parent", func(t *ftt.Test) {
				req.Parent = wuID1.Name()
				req.Requests[0].Parent = ""
				req.Requests[1].Parent = ""

				// Update the expectations for the second request as it will be created in a different work unit.
				expected[1].Name = pbutil.TestExonerationName(string(wuID1.RootInvocationID), wuID1.WorkUnitID, "://infra/special_junit_tests!junit:org.chromium.go.luci:BTests#B", expectedExonerationID2)
				expectedRealms[1] = "testproject:child-1-realm"

				createExonerations(req, expected, expectedRealms)
			})
		})
		t.Run("with legacy invocation", func(t *ftt.Test) {
			invID := invocations.ID("u-build-1")
			testutil.MustApply(ctx, t, insert.Invocation(invID, pb.Invocation_ACTIVE, nil))

			tok, err := generateInvocationToken(ctx, "u-build-1")
			assert.Loosely(t, err, should.BeNil)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, tok))

			req := &pb.BatchCreateTestExonerationsRequest{
				Invocation: invID.Name(),
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
							TestId: "b/c",
							// Due to unfortunate legacy reasons, this is a valid input.
							VariantHash:     pbutil.VariantHash(pbutil.Variant("mykey", "myvalue")),
							ExplanationHtml: "Unexpected pass.",
							Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
						},
					},
				},
				RequestId: "request-id",
			}
			expectedExonerationID1 := fmt.Sprintf("%s:d:%s", pbutil.VariantHash(pbutil.Variant("a", "1", "b", "2")), deterministicExonerationIDSuffix(ctx, req.RequestId, 0))
			expectedExonerationID2 := fmt.Sprintf("%s:d:%s", pbutil.VariantHash(pbutil.Variant("mykey", "myvalue")), deterministicExonerationIDSuffix(ctx, req.RequestId, 1))

			// Expected should be same as requests, plus output only fields set.
			expected := []*pb.TestExoneration{
				proto.Clone(req.Requests[0].TestExoneration).(*pb.TestExoneration),
				proto.Clone(req.Requests[1].TestExoneration).(*pb.TestExoneration),
			}
			expected[0].Name = pbutil.LegacyTestExonerationName(string(invID), "://infra/junit_tests!junit:org.chromium.go.luci:ATests#A", expectedExonerationID1)
			expected[0].TestIdStructured = &pb.TestIdentifier{
				ModuleName:        "//infra/junit_tests",
				ModuleScheme:      "junit",
				ModuleVariant:     pbutil.Variant("a", "1", "b", "2"),
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("a", "1", "b", "2")),
				CoarseName:        "org.chromium.go.luci",
				FineName:          "ATests",
				CaseName:          "A",
			}
			expected[0].VariantHash = pbutil.VariantHash(pbutil.Variant("a", "1", "b", "2"))
			expected[0].ExonerationId = expectedExonerationID1

			expected[1].Name = pbutil.LegacyTestExonerationName(string(invID), "b/c", expectedExonerationID2)
			expected[1].TestIdStructured = &pb.TestIdentifier{
				ModuleName:        "legacy",
				ModuleScheme:      "legacy",
				ModuleVariant:     pbutil.Variant(),
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("mykey", "myvalue")),
				CoarseName:        "",
				FineName:          "",
				CaseName:          "b/c",
			}
			expected[1].Variant = &pb.Variant{}
			expected[1].ExonerationId = expectedExonerationID2

			t.Run("with a non-existing invocation", func(t *ftt.Test) {
				tok, err = generateInvocationToken(ctx, "inv")
				assert.Loosely(t, err, should.BeNil)
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, tok))
				req.Invocation = "invocations/inv"
				for _, r := range req.Requests {
					r.Invocation = ""
				}

				_, err := recorder.BatchCreateTestExonerations(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("invocations/inv not found"))
			})
			t.Run("with an inactive invocation", func(t *ftt.Test) {
				testutil.MustApply(ctx, t, spanutil.UpdateMap("Invocations", map[string]any{
					"InvocationId": invID,
					"State":        pb.Invocation_FINALIZED,
				}))

				_, err := recorder.BatchCreateTestExonerations(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
				assert.Loosely(t, err, should.ErrLike("invocations/u-build-1 is not active"))
			})
			t.Run("with no request ID", func(t *ftt.Test) {
				// This is a legacy behaviour and is valid for test exoneration uploads to invocations only.
				req.RequestId = ""
				for _, r := range req.Requests {
					r.RequestId = ""
				}

				t.Run("success", func(t *ftt.Test) {
					_, err := recorder.BatchCreateTestExonerations(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("empty in batch-level, but not in requests", func(t *ftt.Test) {
					// This error can only be triggered when using invocations, as having
					// an empty batch-level request_id is not allowed when using work units.
					req.Requests[0].RequestId = "123"
					_, err := recorder.BatchCreateTestExonerations(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike(`bad request: requests[0]: request_id: must be either empty or equal to "", but got "123"`))
				})
			})
			t.Run("base case (success)", func(t *ftt.Test) {
				createExonerations := func(req *pb.BatchCreateTestExonerationsRequest, expected []*pb.TestExoneration) {
					response, err := recorder.BatchCreateTestExonerations(ctx, req)
					assert.Loosely(t, err, should.BeNil, truth.LineContext(1))
					assert.Loosely(t, len(response.TestExonerations), should.Equal(len(expected)), truth.LineContext(1))

					expectedRefined := make([]*pb.TestExoneration, len(expected))
					for i := range expected {
						expected := proto.Clone(expected[i]).(*pb.TestExoneration)
						if expected.ExonerationId == "" {
							assert.Loosely(t, response.TestExonerations[i].ExonerationId, should.NotBeEmpty, truth.LineContext(1))
							expected.ExonerationId = response.TestExonerations[i].ExonerationId // Accept the server-assigned ID.
						}
						if expected.Name == "" {
							assert.Loosely(t, response.TestExonerations[i].Name, should.NotBeEmpty, truth.LineContext(1))
							expected.Name = response.TestExonerations[i].Name // Accept the server-assigned name.
						}
						expectedRefined[i] = expected
					}
					assert.Loosely(t, response.TestExonerations, should.Match(expectedRefined))

					for i := range req.Requests {
						// Now check the database.
						row, err := exonerations.Read(span.Single(ctx), response.TestExonerations[i].Name)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, row, should.Match(expectedRefined[i]))
					}
				}

				createExonerations(req, expected)
			})
		})
	})
}
