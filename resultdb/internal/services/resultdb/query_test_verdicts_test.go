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
	"strings"
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
	"go.chromium.org/luci/resultdb/internal/testresultsv2"
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
					t.Run(`Too long`, func(t *ftt.Test) {
						req.Predicate.ContainsTestResultFilter = strings.Repeat("a", testresultsv2.MaxFilterLengthBytes+1)
						_, err := srv.QueryTestVerdicts(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("predicate: contains_test_result_filter: filter is too long"))
					})
				})
				t.Run(`Filter`, func(t *ftt.Test) {
					req.Predicate = &pb.TestVerdictPredicate{}
					t.Run(`Invalid`, func(t *ftt.Test) {
						req.Predicate.EffectiveVerdictStatus = []pb.TestVerdictPredicate_VerdictEffectiveStatus{
							pb.TestVerdictPredicate_VERDICT_EFFECTIVE_STATUS_UNSPECIFIED,
						}
						_, err := srv.QueryTestVerdicts(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("predicate: effective_verdict_status: must not contain VERDICT_EFFECTIVE_STATUS_UNSPECIFIED"))
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
						req.View = pb.TestVerdictView_TEST_VERDICT_VIEW_FULL
						_, err := srv.QueryTestVerdicts(ctx, req)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run(`Invalid`, func(t *ftt.Test) {
						req.View = pb.TestVerdictView(123)
						_, err := srv.QueryTestVerdicts(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`view: invalid value 123`))
					})
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
					assert.Loosely(t, err, should.ErrLike("page_token: invalid page token: "))
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
			expected := testverdictsv2.ToBasicView(testverdictsv2.ExpectedVerdicts(rootInvID))

			t.Run(`Valid request`, func(t *ftt.Test) {
				res, err := srv.QueryTestVerdicts(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.NextPageToken, should.BeEmpty)
				assert.Loosely(t, res.TestVerdicts, should.Match(expected))
			})

			t.Run(`With ordering`, func(t *ftt.Test) {
				// We rely on unit tests in testverdictsv2/query_test.go to verify exact order correctness
				// of all the ordering options.
				// Here we just do a spot check.
				req.OrderBy = "ui_priority, test_id_structured"
				res, err := srv.QueryTestVerdicts(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, res.TestVerdicts[0].Status, should.Equal(pb.TestVerdict_FAILED)) // Highest priority first
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
				expected = testverdictsv2.FilterVerdicts(expected, func(tv *pb.TestVerdict) bool {
					return tv.TestIdStructured.ModuleName == "m1"
				})
				assert.Loosely(t, res.TestVerdicts, should.Match(expected))
			})

			t.Run(`With contains test results filter`, func(t *ftt.Test) {
				req.Predicate = &pb.TestVerdictPredicate{
					ContainsTestResultFilter: `status = SKIPPED`,
				}
				res, err := srv.QueryTestVerdicts(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				// Only t4 has a SKIPPED result.
				expected = []*pb.TestVerdict{
					testverdictsv2.VerdictByCaseName(expected, "t4"),
				}
				assert.Loosely(t, res.TestVerdicts, should.Match(expected))
			})

			t.Run(`With filter`, func(t *ftt.Test) {
				req.Predicate = &pb.TestVerdictPredicate{
					EffectiveVerdictStatus: []pb.TestVerdictPredicate_VerdictEffectiveStatus{
						pb.TestVerdictPredicate_FAILED,
						pb.TestVerdictPredicate_PRECLUDED,
					},
				}
				res, err := srv.QueryTestVerdicts(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				expected = testverdictsv2.FilterVerdicts(expected, func(tv *pb.TestVerdict) bool {
					return tv.TestIdStructured.CaseName == "t2" || // Failed
						tv.TestIdStructured.CaseName == "t6" // Precluded
				})
				assert.Loosely(t, res.TestVerdicts, should.Match(expected))
			})

			t.Run(`With pagination`, func(t *ftt.Test) {
				req.PageSize = 2

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

			t.Run("With view", func(t *ftt.Test) {
				t.Run("Basic", func(t *ftt.Test) {
					req.View = pb.TestVerdictView_TEST_VERDICT_VIEW_BASIC
					expected := testverdictsv2.ToBasicView(testverdictsv2.ExpectedVerdicts(rootInvID))

					res, err := srv.QueryTestVerdicts(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res.TestVerdicts, should.Match(expected))
					assert.Loosely(t, res.NextPageToken, should.BeEmpty)
				})
				t.Run("Full", func(t *ftt.Test) {
					req.View = pb.TestVerdictView_TEST_VERDICT_VIEW_FULL
					expected := testverdictsv2.ExpectedVerdicts(rootInvID)

					res, err := srv.QueryTestVerdicts(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res.TestVerdicts, should.Match(expected))
					assert.Loosely(t, res.NextPageToken, should.BeEmpty)
				})
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
				expected := testverdictsv2.ToBasicView(testverdictsv2.ExpectedMaskedVerdicts(testverdictsv2.ExpectedVerdicts(rootInvID), []string{"testproject:t3-r1"}))
				assert.Loosely(t, res.TestVerdicts, should.Match(expected))
			})
		})
	})
}
