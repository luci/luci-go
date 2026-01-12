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
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/testverdictsv2"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestQueryTestVerdicts(t *testing.T) {
	ftt.Run(`QueryTestVerdicts`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx) // For config in-process cache.
		ctx = memory.Use(ctx)                    // For config datastore cache.

		// Setup service config.
		err := config.SetServiceConfigForTesting(ctx, config.CreatePlaceholderServiceConfig())
		assert.NoErr(t, err)

		authState := &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestResults},
				{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestExonerations},
			},
		}
		ctx = auth.WithState(ctx, authState)

		srv := newTestResultDBService()

		// Set up data.
		rootInvID := rootinvocations.ID("root-inv1")
		ms := insert.RootInvocationWithRootWorkUnit(rootinvocations.NewBuilder(rootInvID).WithRealm("testproject:testrealm").Build())
		testutil.MustApply(ctx, t, ms...)

		req := &pb.QueryTestVerdictsRequest{
			Parent: "rootInvocations/root-inv1",
		}

		t.Run(`Request validation`, func(t *ftt.Test) {
			t.Run(`Parent`, func(t *ftt.Test) {
				t.Run(`Empty`, func(t *ftt.Test) {
					req.Parent = ""
					_, err := srv.QueryTestVerdicts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("parent: unspecified"))
				})
				t.Run(`Invalid`, func(t *ftt.Test) {
					req.Parent = "invalid"
					_, err := srv.QueryTestVerdicts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("parent: does not match pattern"))
				})
			})
			t.Run(`Page size`, func(t *ftt.Test) {
				t.Run(`Default`, func(t *ftt.Test) {
					// Valid, will use a default size.
					req.PageSize = 0
					_, err := srv.QueryTestVerdicts(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run(`Negative`, func(t *ftt.Test) {
					req.PageSize = -1
					_, err := srv.QueryTestVerdicts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("page_size: negative"))
				})
			})
			t.Run(`Page token`, func(t *ftt.Test) {
				t.Run(`Empty`, func(t *ftt.Test) {
					// Valid, will start from first page.
					req.PageToken = ""
					_, err := srv.QueryTestVerdicts(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run(`Invalid`, func(t *ftt.Test) {
					req.PageToken = "some invalid token"
					_, err := srv.QueryTestVerdicts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("page_token: invalid page token"))
				})
			})
			t.Run(`Order by`, func(t *ftt.Test) {
				t.Run(`Empty`, func(t *ftt.Test) {
					// Valid, will use default sort order.
					req.OrderBy = ""
					_, err := srv.QueryTestVerdicts(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run(`Invalid`, func(t *ftt.Test) {
					req.OrderBy = "invalid"
					_, err := srv.QueryTestVerdicts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike(`order_by: unsupported order by clause: "invalid"; supported orders are `))
				})
			})
			t.Run(`Predicate`, func(t *ftt.Test) {
				t.Run(`Unspecified`, func(t *ftt.Test) {
					req.Predicate = nil
					_, err := srv.QueryTestVerdicts(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run(`Test prefix filter`, func(t *ftt.Test) {
					req.Predicate = &pb.TestVerdictPredicate{}
					t.Run(`Unspecified`, func(t *ftt.Test) {
						// Valid.
						req.Predicate.TestPrefixFilter = nil
						_, err := srv.QueryTestVerdicts(ctx, req)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run(`Invalid - structurally invalid (empty)`, func(t *ftt.Test) {
						req.Predicate.TestPrefixFilter = &pb.TestIdentifierPrefix{Level: pb.AggregationLevel_INVOCATION}
						_, err := srv.QueryTestVerdicts(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("predicate: test_prefix_filter: id: unspecified"))
					})
				})
				t.Run(`Contains test result filter`, func(t *ftt.Test) {
					req.Predicate = &pb.TestVerdictPredicate{}
					t.Run(`Invalid syntax`, func(t *ftt.Test) {
						req.Predicate.ContainsTestResultFilter = "status ="
						_, err := srv.QueryTestVerdicts(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("predicate: contains_test_result_filter: expected arg after ="))
					})
				})
				t.Run(`Filter`, func(t *ftt.Test) {
					req.Predicate = &pb.TestVerdictPredicate{}
					t.Run(`Invalid syntax`, func(t *ftt.Test) {
						req.Predicate.Filter = "status ="
						_, err := srv.QueryTestVerdicts(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("predicate: filter: expected arg after ="))
					})
				})
				t.Run(`View`, func(t *ftt.Test) {
					t.Run(`Empty`, func(t *ftt.Test) {
						// Valid, will use a default view.
						req.View = pb.TestVerdictView_TEST_VERDICT_VIEW_UNSPECIFIED
						_, err := srv.QueryTestVerdicts(ctx, req)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run(`Valid`, func(t *ftt.Test) {
						req.View = pb.TestVerdictView_TEST_VERDICT_VIEW_BASIC
						_, err := srv.QueryTestVerdicts(ctx, req)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run(`Invalid`, func(t *ftt.Test) {
						req.View = pb.TestVerdictView_TEST_VERDICT_VIEW_FULL
						_, err := srv.QueryTestVerdicts(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`view: if set, may only be set to TEST_VERDICT_VIEW_BASIC`))
					})
				})
			})
		})

		t.Run(`Request authorisation`, func(t *ftt.Test) {
			t.Run(`Without list test results permission`, func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, rdbperms.PermListTestResults)
				_, err := srv.QueryTestVerdicts(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike(`caller does not have permissions [resultdb.testResults.listLimited,`+
					` resultdb.testExonerations.listLimited] (or [resultdb.testResults.list, resultdb.testExonerations.list])`+
					` in realm of root invocation "rootInvocations/root-inv1"`))
			})
			t.Run(`Without list test exonerations permission`, func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, rdbperms.PermListTestExonerations)
				_, err := srv.QueryTestVerdicts(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike(`caller does not have permissions [resultdb.testResults.listLimited,`+
					` resultdb.testExonerations.listLimited] (or [resultdb.testResults.list, resultdb.testExonerations.list])`+
					` in realm of root invocation "rootInvocations/root-inv1"`))
			})
		})

		t.Run(`End-to-End`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testverdictsv2.CreateTestData(rootInvID)...)

			t.Run(`Valid request`, func(t *ftt.Test) {
				res, err := srv.QueryTestVerdicts(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.NextPageToken, should.BeEmpty)
				assert.Loosely(t, res.TestVerdicts, should.Match(testverdictsv2.ExpectedVerdicts(rootInvID, pb.TestVerdictView_TEST_VERDICT_VIEW_BASIC)))
			})

			t.Run(`With UI priority order`, func(t *ftt.Test) {
				req.OrderBy = "ui_priority desc, test_id_structured"
				res, err := srv.QueryTestVerdicts(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				// Verify order is different from default.
				// We rely on unit tests in testverdictsv2/query_test.go to verify exact order correctness.
				// Here we just check it returns results and doesn't error.
				assert.Loosely(t, res.TestVerdicts, should.HaveLength(7))
				assert.Loosely(t, res.TestVerdicts[0].Status, should.Equal(pb.TestVerdict_FAILED)) // Highest priority
			})

			t.Run(`With prefix filter`, func(t *ftt.Test) {
				req.Predicate = &pb.TestVerdictPredicate{
					TestPrefixFilter: &pb.TestIdentifierPrefix{
						Level: pb.AggregationLevel_MODULE,
						Id: &pb.TestIdentifier{
							ModuleName:    "m1",
							ModuleScheme:  "junit",
							ModuleVariant: pbutil.Variant("key", "value"),
						},
					},
				}
				res, err := srv.QueryTestVerdicts(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.NextPageToken, should.BeEmpty)

				// Should match all verdicts except t7 (which is in m2).
				expected := testverdictsv2.ExpectedVerdicts(rootInvID, pb.TestVerdictView_TEST_VERDICT_VIEW_BASIC)
				assert.Loosely(t, res.TestVerdicts, should.Match(expected[0:6]))
			})

			t.Run(`With contains test results filter`, func(t *ftt.Test) {
				req.Predicate = &pb.TestVerdictPredicate{
					ContainsTestResultFilter: `status = SKIPPED`,
				}
				res, err := srv.QueryTestVerdicts(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				// Only t4 has a SKIPPED result.
				expected := testverdictsv2.ExpectedVerdicts(rootInvID, pb.TestVerdictView_TEST_VERDICT_VIEW_BASIC)
				assert.Loosely(t, res.TestVerdicts, should.Match(expected[3:4]))
			})

			t.Run(`With filter`, func(t *ftt.Test) {
				req.Predicate = &pb.TestVerdictPredicate{
					Filter: "status = FAILED",
				}
				res, err := srv.QueryTestVerdicts(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				// t2 (FAILED) and t5 (FAILED and EXONERATED) match status=FAILED.
				expected := testverdictsv2.ExpectedVerdicts(rootInvID, pb.TestVerdictView_TEST_VERDICT_VIEW_BASIC)
				assert.Loosely(t, res.TestVerdicts, should.Match([]*pb.TestVerdict{expected[1], expected[4]}))
			})

			t.Run(`With pagination`, func(t *ftt.Test) {
				req.PageSize = 2
				expected := testverdictsv2.ExpectedVerdicts(rootInvID, pb.TestVerdictView_TEST_VERDICT_VIEW_BASIC)

				res, err := srv.QueryTestVerdicts(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.TestVerdicts, should.Match(expected[0:2]))
				assert.Loosely(t, res.NextPageToken, should.NotBeEmpty)

				req.PageToken = res.NextPageToken
				res, err = srv.QueryTestVerdicts(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.TestVerdicts, should.Match(expected[2:4]))
				assert.Loosely(t, res.NextPageToken, should.NotBeEmpty)

				req.PageToken = res.NextPageToken
				res, err = srv.QueryTestVerdicts(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.TestVerdicts, should.Match(expected[4:6]))
				assert.Loosely(t, res.NextPageToken, should.NotBeEmpty)

				req.PageToken = res.NextPageToken
				res, err = srv.QueryTestVerdicts(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.TestVerdicts, should.Match(expected[6:]))
				assert.Loosely(t, res.NextPageToken, should.BeEmpty)
			})

			t.Run(`With limited access`, func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					// Limited access to the root invocation.
					{Realm: "testproject:testrealm", Permission: rdbperms.PermListLimitedTestResults},
					{Realm: "testproject:testrealm", Permission: rdbperms.PermListLimitedTestExonerations},

					// Ability to upgrade to full access in a realm used by a test result in t3.
					// t3 has r1 (testproject:t3-r1) and r2 (testproject:t3-r2).
					// Let's give access to t3-r1.
					{Realm: "testproject:t3-r1", Permission: rdbperms.PermGetTestResult},
					{Realm: "testproject:t3-r1", Permission: rdbperms.PermGetTestExoneration},
				}

				res, err := srv.QueryTestVerdicts(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.NextPageToken, should.BeEmpty)

				// We expect t3 to be partially unmasked (r1 unmasked, r2 masked).
				// All others masked.
				expected := testverdictsv2.ExpectedVerdictsMasked(rootInvID, pb.TestVerdictView_TEST_VERDICT_VIEW_BASIC, []string{"testproject:t3-r1"})
				assert.Loosely(t, res.TestVerdicts, should.Match(expected))
			})
		})
	})
}
