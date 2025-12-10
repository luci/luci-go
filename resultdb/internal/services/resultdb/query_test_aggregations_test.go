// Copyright 2025 The LUCI Authors.
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

package resultdb

import (
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testaggregations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestQueryTestAggregations(t *testing.T) {
	ftt.Run(`QueryTestAggregations`, t, func(t *ftt.Test) {
		authState := &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermListWorkUnits},
				{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestResults},
				{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestExonerations},
			},
		}
		ctx := testutil.SpannerTestContext(t)
		ctx = auth.WithState(ctx, authState)

		srv := newTestResultDBService()

		// Set up data.
		rootInvID := rootinvocations.ID("root-inv1")

		ms := insert.RootInvocationWithRootWorkUnit(rootinvocations.NewBuilder(rootInvID).WithRealm("testproject:testrealm").Build())
		testutil.MustApply(ctx, t, ms...)

		req := &pb.QueryTestAggregationsRequest{
			Parent: "rootInvocations/root-inv1",
			Predicate: &pb.TestAggregationPredicate{
				AggregationLevel: pb.AggregationLevel_MODULE,
			},
		}

		t.Run(`Request validation`, func(t *ftt.Test) {
			t.Run(`Parent`, func(t *ftt.Test) {
				t.Run(`Empty`, func(t *ftt.Test) {
					req.Parent = ""
					_, err := srv.QueryTestAggregations(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("parent: unspecified"))
				})
				t.Run(`Invalid`, func(t *ftt.Test) {
					req.Parent = "invalid"
					_, err := srv.QueryTestAggregations(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("parent: does not match pattern"))
				})
			})
			t.Run(`Predicate`, func(t *ftt.Test) {
				t.Run(`Unspecified`, func(t *ftt.Test) {
					req.Predicate = nil
					_, err := srv.QueryTestAggregations(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("predicate: unspecified"))
				})
				t.Run(`Aggregation level`, func(t *ftt.Test) {
					t.Run(`Unspecified`, func(t *ftt.Test) {
						req.Predicate.AggregationLevel = pb.AggregationLevel_AGGREGATION_LEVEL_UNSPECIFIED
						_, err := srv.QueryTestAggregations(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("predicate: aggregation_level: unspecified"))
					})
					t.Run(`Invalid`, func(t *ftt.Test) {
						req.Predicate.AggregationLevel = pb.AggregationLevel(10)
						_, err := srv.QueryTestAggregations(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("predicate: aggregation_level: unknown aggregation level 10"))
					})
					t.Run(`Too fine`, func(t *ftt.Test) {
						req.Predicate.AggregationLevel = pb.AggregationLevel_CASE
						_, err := srv.QueryTestAggregations(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("predicate: aggregation_level: CASE is not a valid aggregation level; to query test case verdicts, use the QueryTestVerdicts RPC"))
					})
				})
				t.Run(`Test prefix filter`, func(t *ftt.Test) {
					t.Run(`Unspecified`, func(t *ftt.Test) {
						// Valid.
						req.Predicate.TestPrefixFilter = nil
						_, err := srv.QueryTestAggregations(ctx, req)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run(`Invalid - structurally invalid (empty)`, func(t *ftt.Test) {
						req.Predicate.TestPrefixFilter = &pb.TestIdentifierPrefix{Level: pb.AggregationLevel_INVOCATION}
						_, err := srv.QueryTestAggregations(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("predicate: test_prefix_filter: id: unspecified"))
					})
					t.Run(`Invalid - structurally invalid (too many fields set for level)`, func(t *ftt.Test) {
						req.Predicate.TestPrefixFilter = &pb.TestIdentifierPrefix{
							Level: pb.AggregationLevel_MODULE,
							Id: &pb.TestIdentifier{
								ModuleName:        "module",
								ModuleScheme:      "scheme",
								ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
								CoarseName:        "coarse_name",
								CaseName:          "case_name",
							},
						}
						_, err := srv.QueryTestAggregations(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("predicate: test_prefix_filter: id: coarse_name: must be empty for MODULE level prefix"))
					})
					t.Run(`Invalid - finer than aggregation level`, func(t *ftt.Test) {
						req.Predicate.AggregationLevel = pb.AggregationLevel_MODULE
						req.Predicate.TestPrefixFilter = &pb.TestIdentifierPrefix{
							Level: pb.AggregationLevel_COARSE,
							Id: &pb.TestIdentifier{
								ModuleName:    "module",
								ModuleScheme:  "scheme",
								ModuleVariant: pbutil.Variant("key", "value"),
								CoarseName:    "",
							},
						}
						_, err := srv.QueryTestAggregations(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("predicate: test_prefix_filter: level: must be equal to, or coarser than, the requested aggregation_level (MODULE)"))
					})
				})
			})
			t.Run(`Order by`, func(t *ftt.Test) {
				t.Run(`Empty`, func(t *ftt.Test) {
					// Valid, will use default sort order.
					req.OrderBy = ""
					_, err := srv.QueryTestAggregations(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run(`Invalid`, func(t *ftt.Test) {
					req.OrderBy = "invalid"
					_, err := srv.QueryTestAggregations(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("order_by: unsupported order by clause: \"invalid\"; supported orders are `id.level,id.id` or `id.level,ui_priority desc,id.id` (id.level may be elided if only one level is requested)"))
				})
				t.Run(`Invalid sort order`, func(t *ftt.Test) {
					// UI Priority must be descending.
					req.OrderBy = "id.level, ui_priority, id.id"
					_, err := srv.QueryTestAggregations(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("order_by: can only sort by `ui_priority` in descending order"))
				})
			})
			t.Run(`Page size`, func(t *ftt.Test) {
				t.Run(`Default`, func(t *ftt.Test) {
					// Valid, will use a default size.
					req.PageSize = 0
					_, err := srv.QueryTestAggregations(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run(`Negative`, func(t *ftt.Test) {
					req.PageSize = -1
					_, err := srv.QueryTestAggregations(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("page_size: negative"))
				})
			})
			t.Run(`Page token`, func(t *ftt.Test) {
				t.Run(`Empty`, func(t *ftt.Test) {
					// Valid, will start from first page.
					req.PageToken = ""
					_, err := srv.QueryTestAggregations(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run(`Invalid`, func(t *ftt.Test) {
					req.PageToken = "some invalid token"
					_, err := srv.QueryTestAggregations(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("page_token: invalid page token"))
				})
			})
		})
		t.Run(`Request authorisation`, func(t *ftt.Test) {
			t.Run(`Without list work units permission`, func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, rdbperms.PermListWorkUnits)
				_, err := srv.QueryTestAggregations(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike(`caller does not have permission "resultdb.workUnits.list" in realm "testproject:testrealm"`))
			})
			t.Run(`Without list test results permission`, func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, rdbperms.PermListTestResults)
				_, err := srv.QueryTestAggregations(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike(`caller does not have permission "resultdb.testResults.list" in realm "testproject:testrealm"`))
			})
			t.Run(`Without list test exonerations permission`, func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, rdbperms.PermListTestExonerations)
				_, err := srv.QueryTestAggregations(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike(`caller does not have permission "resultdb.testExonerations.list" in realm "testproject:testrealm"`))
			})
		})

		t.Run(`End-to-End`, func(t *ftt.Test) {
			ms := testaggregations.CreateTestData(rootInvID)
			testutil.MustApply(ctx, t, ms...)

			t.Run(`Aggregation Level`, func(t *ftt.Test) {
				testCases := []struct {
					name     string
					level    pb.AggregationLevel
					expected []*pb.TestAggregation
				}{
					{
						name:     "Invocation-level",
						level:    pb.AggregationLevel_INVOCATION,
						expected: []*pb.TestAggregation{testaggregations.ExpectedRootInvocationAggregation()},
					},
					{
						name:     "Module-level",
						level:    pb.AggregationLevel_MODULE,
						expected: testaggregations.ExpectedModuleAggregationsIDOrder(),
					},
					{
						name:     "Coarse-level",
						level:    pb.AggregationLevel_COARSE,
						expected: testaggregations.ExpectedCoarseAggregationsIDOrder(),
					},
					{
						name:     "Fine-level",
						level:    pb.AggregationLevel_FINE,
						expected: testaggregations.ExpectedFineAggregationsIDOrder(),
					},
				}
				for _, tc := range testCases {
					t.Run(tc.name, func(t *ftt.Test) {
						req.Predicate.AggregationLevel = tc.level
						res, err := srv.QueryTestAggregations(ctx, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, res.Aggregations, should.Match(tc.expected))
					})
				}
			})
			t.Run(`With UI priority order`, func(t *ftt.Test) {
				req.Predicate.AggregationLevel = pb.AggregationLevel_MODULE
				req.OrderBy = "ui_priority desc, id.id"
				expected := testaggregations.ExpectedModuleAggregationsUIOrder()

				res, err := srv.QueryTestAggregations(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.Aggregations, should.Match(expected))
			})
			t.Run(`With prefix filter`, func(t *ftt.Test) {
				req.Predicate.AggregationLevel = pb.AggregationLevel_COARSE
				req.Predicate.TestPrefixFilter = &pb.TestIdentifierPrefix{
					Level: pb.AggregationLevel_MODULE,
					Id: &pb.TestIdentifier{
						ModuleName:    "m1",
						ModuleScheme:  "junit",
						ModuleVariant: pbutil.Variant("key", "value"),
					},
				}

				expected := testaggregations.ExpectedCoarseAggregationsIDOrder()
				expected = expected[0:2]

				res, err := srv.QueryTestAggregations(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.Aggregations, should.Match(expected))
			})
			t.Run(`With pagination`, func(t *ftt.Test) {
				req.PageSize = 2
				req.Predicate.AggregationLevel = pb.AggregationLevel_FINE
				expected := testaggregations.ExpectedFineAggregationsIDOrder()

				res, err := srv.QueryTestAggregations(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.Aggregations, should.Match(expected[0:2]))

				req.PageToken = res.NextPageToken
				res, err = srv.QueryTestAggregations(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.Aggregations, should.Match(expected[2:4]))

				req.PageToken = res.NextPageToken
				res, err = srv.QueryTestAggregations(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.Aggregations, should.Match(expected[4:]))
			})
		})
	})
}
