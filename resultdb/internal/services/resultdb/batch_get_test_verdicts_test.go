// Copyright 2026 The LUCI Authors.
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
	"google.golang.org/protobuf/proto"

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
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestBatchGetTestVerdicts(t *testing.T) {
	ftt.Run(`BatchGetTestVerdicts`, t, func(t *ftt.Test) {
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
		inv := rootinvocations.NewBuilder(rootInvID).
			WithRealm("testproject:testrealm").
			WithTestShardingAlgorithm(testverdictsv2.TestShardingAlgorithm.ID()).
			Build()

		ms := insert.RootInvocationWithRootWorkUnit(inv)
		ms = append(ms, testverdictsv2.CreateTestData(rootInvID)...)
		testutil.MustApply(ctx, t, ms...)

		testData := testverdictsv2.ToBasicView(testverdictsv2.ExpectedVerdicts(rootInvID))

		// Helpers for looking up expected verdicts by case name.
		verdictByCaseName := func(caseName string) *pb.TestVerdict {
			return testverdictsv2.VerdictByCaseName(testData, caseName)
		}

		req := &pb.BatchGetTestVerdictsRequest{
			Parent: "rootInvocations/root-inv1",
			Tests: []*pb.BatchGetTestVerdictsRequest_NominatedTest{
				{TestIdStructured: verdictByCaseName("t1").TestIdStructured},
				{TestIdStructured: verdictByCaseName("t2").TestIdStructured},
			},
		}

		t.Run(`Request validation`, func(t *ftt.Test) {
			t.Run(`Parent`, func(t *ftt.Test) {
				t.Run(`Unspecified`, func(t *ftt.Test) {
					req.Parent = ""
					_, err := srv.BatchGetTestVerdicts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("parent: unspecified"))
				})
				t.Run(`Invalid`, func(t *ftt.Test) {
					req.Parent = "invocations/build-123"
					_, err := srv.BatchGetTestVerdicts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike(`parent: does not match pattern "^rootInvocations/([a-z][a-z0-9_\\-.]*)$"`))
				})
			})
			t.Run(`Tests`, func(t *ftt.Test) {
				t.Run(`Unspecified`, func(t *ftt.Test) {
					req.Tests = nil
					_, err := srv.BatchGetTestVerdicts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("test_variants: unspecified"))
				})
				t.Run(`Invalid entry`, func(t *ftt.Test) {
					t.Run(`Empty`, func(t *ftt.Test) {
						req.Tests = []*pb.BatchGetTestVerdictsRequest_NominatedTest{
							{},
						}
						_, err := srv.BatchGetTestVerdicts(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("test_variants[0]: either test_id_structured or (test_id and variant_hash) must be set"))
					})
					t.Run(`Invalid flat test ID`, func(t *ftt.Test) {
						req.Tests = []*pb.BatchGetTestVerdictsRequest_NominatedTest{
							{
								TestId:      "\x01", // Invalid printable
								VariantHash: "0000000000000000",
							},
						}
						_, err := srv.BatchGetTestVerdicts(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("test_variants[0]: test_id: non-printable rune"))
					})
					t.Run(`Missing variant hash`, func(t *ftt.Test) {
						req.Tests = []*pb.BatchGetTestVerdictsRequest_NominatedTest{
							{
								TestId:      "my_test",
								VariantHash: "",
							},
						}
						_, err := srv.BatchGetTestVerdicts(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("test_variants[0]: variant_hash: unspecified"))
					})
					t.Run(`Invalid structured test ID`, func(t *ftt.Test) {
						req.Tests = []*pb.BatchGetTestVerdictsRequest_NominatedTest{
							{
								TestIdStructured: &pb.TestIdentifier{
									ModuleName:        "\x01",
									ModuleScheme:      "junit",
									ModuleVariantHash: "0000000000000000",
									CoarseName:        "my.package",
									FineName:          "MyTestClass",
									CaseName:          "testMethod",
								},
							},
						}
						_, err := srv.BatchGetTestVerdicts(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("test_variants[0]: test_id_structured: module_name: non-printable rune '\\x01' at byte index 0"))
					})
					t.Run(`Specified both flat and structured`, func(t *ftt.Test) {
						req.Tests = []*pb.BatchGetTestVerdictsRequest_NominatedTest{
							{
								TestIdStructured: &pb.TestIdentifier{
									ModuleName:        "module",
									ModuleScheme:      "junit",
									ModuleVariantHash: "0000000000000000",
									CoarseName:        "my.package",
									FineName:          "MyTestClass",
									CaseName:          "testMethod",
								},
								TestId:      "some_test_id",
								VariantHash: "0000000000000000",
							},
						}
						_, err := srv.BatchGetTestVerdicts(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("test_variants[0]: test_id: may not be set at same time as test_id_structured"))
					})
				})
				t.Run(`Too many items`, func(t *ftt.Test) {
					req.Tests = make([]*pb.BatchGetTestVerdictsRequest_NominatedTest, 501)
					_, err := srv.BatchGetTestVerdicts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("test_variants: a maximum of 500 test variants can be requested at once"))
				})
				t.Run(`Too many items (Full view)`, func(t *ftt.Test) {
					req.Tests = make([]*pb.BatchGetTestVerdictsRequest_NominatedTest, 51)
					req.View = pb.TestVerdictView_TEST_VERDICT_VIEW_FULL
					_, err := srv.BatchGetTestVerdicts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("test_variants: a maximum of 50 test variants can be requested at once"))
				})
			})
			t.Run(`View`, func(t *ftt.Test) {
				t.Run(`Invalid`, func(t *ftt.Test) {
					req.View = pb.TestVerdictView(123)
					_, err := srv.BatchGetTestVerdicts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike(`view: invalid value 123`))
				})
			})
		})

		t.Run(`Access control`, func(t *ftt.Test) {
			authState.IdentityPermissions = []authtest.RealmPermission{}
			_, err := srv.BatchGetTestVerdicts(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
		})

		t.Run(`Root invocation not found`, func(t *ftt.Test) {
			req.Parent = "rootInvocations/not-found"
			_, err := srv.BatchGetTestVerdicts(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike(`"rootInvocations/not-found" not found`))
		})

		t.Run(`End-to-end`, func(t *ftt.Test) {
			t.Run(`With Structured IDs`, func(t *ftt.Test) {
				req.Tests = []*pb.BatchGetTestVerdictsRequest_NominatedTest{
					{TestIdStructured: proto.Clone(verdictByCaseName("t1").TestIdStructured).(*pb.TestIdentifier)},
					{TestIdStructured: proto.Clone(verdictByCaseName("t3").TestIdStructured).(*pb.TestIdentifier)},
					{TestIdStructured: proto.Clone(verdictByCaseName("t2").TestIdStructured).(*pb.TestIdentifier)},
					{TestIdStructured: proto.Clone(verdictByCaseName("t5").TestIdStructured).(*pb.TestIdentifier)},
					// Include a duplicate for good measure.
					{TestIdStructured: proto.Clone(verdictByCaseName("t2").TestIdStructured).(*pb.TestIdentifier)},
				}
				// On one request, leave module variant empty, on another leave module variant
				// hash empty. The caller is only required to specify one.
				req.Tests[0].TestIdStructured.ModuleVariant = nil
				req.Tests[1].TestIdStructured.ModuleVariantHash = ""

				res, err := srv.BatchGetTestVerdicts(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				expected := []*pb.TestVerdict{
					verdictByCaseName("t1"),
					verdictByCaseName("t3"),
					verdictByCaseName("t2"),
					verdictByCaseName("t5"),
					verdictByCaseName("t2"),
				}
				assert.Loosely(t, res.TestVerdicts, should.Match(expected))
			})

			t.Run(`With Flat IDs`, func(t *ftt.Test) {
				t1 := verdictByCaseName("t1")
				req.Tests = []*pb.BatchGetTestVerdictsRequest_NominatedTest{
					{
						TestId:      t1.TestId,
						VariantHash: t1.TestIdStructured.ModuleVariantHash,
					},
				}
				res, err := srv.BatchGetTestVerdicts(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.TestVerdicts, should.HaveLength(1))
				assert.Loosely(t, res.TestVerdicts[0], should.Match(t1))
			})

			t.Run(`With empty test verdicts`, func(t *ftt.Test) {
				expectedPlaceholderVerdict := func(testID, variantHash string) *pb.TestVerdict {
					return &pb.TestVerdict{
						TestIdStructured: &pb.TestIdentifier{
							ModuleName:        "legacy",
							ModuleScheme:      "legacy",
							ModuleVariantHash: variantHash,
							CaseName:          testID,
						},
						TestId: testID,
						Status: pb.TestVerdict_STATUS_UNSPECIFIED,
					}
				}

				// Try verdicts with no results in different positions.
				req.Tests = []*pb.BatchGetTestVerdictsRequest_NominatedTest{
					{TestId: "non-existent1", VariantHash: "0000000000000001"},
					{TestIdStructured: verdictByCaseName("t2").TestIdStructured},
					{TestId: "non-existent3", VariantHash: "0000000000000003"},
					{TestIdStructured: verdictByCaseName("t1").TestIdStructured},
					{TestId: "non-existent2", VariantHash: "0000000000000002"},
				}
				rsp, err := srv.BatchGetTestVerdicts(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				expected := []*pb.TestVerdict{
					expectedPlaceholderVerdict("non-existent1", "0000000000000001"),
					verdictByCaseName("t2"),
					expectedPlaceholderVerdict("non-existent3", "0000000000000003"),
					verdictByCaseName("t1"),
					expectedPlaceholderVerdict("non-existent2", "0000000000000002"),
				}
				assert.Loosely(t, rsp.TestVerdicts, should.Match(expected))
			})

			t.Run(`With view FULL`, func(t *ftt.Test) {
				req.Tests = []*pb.BatchGetTestVerdictsRequest_NominatedTest{
					{TestIdStructured: verdictByCaseName("t5").TestIdStructured}, // t5 has exonerations
				}
				req.View = pb.TestVerdictView_TEST_VERDICT_VIEW_FULL
				res, err := srv.BatchGetTestVerdicts(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				testDataFull := testverdictsv2.ExpectedVerdicts(rootInvID)
				expected := []*pb.TestVerdict{
					testverdictsv2.VerdictByCaseName(testDataFull, "t5"),
				}

				assert.Loosely(t, res.TestVerdicts, should.Match(expected))
			})

			t.Run(`With limited access`, func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					// Limited access to the root invocation.
					{Realm: "testproject:testrealm", Permission: rdbperms.PermListLimitedTestResults},
					{Realm: "testproject:testrealm", Permission: rdbperms.PermListLimitedTestExonerations},
					// Full access to test t3, result r1.
					{Realm: "testproject:t3-r1", Permission: rdbperms.PermGetTestResult},
					{Realm: "testproject:t3-r1", Permission: rdbperms.PermGetTestExoneration},
				}

				// Request t3 (mixed access) and t2 (full masked).
				req.Tests = []*pb.BatchGetTestVerdictsRequest_NominatedTest{
					{TestIdStructured: verdictByCaseName("t3").TestIdStructured},
					{TestIdStructured: verdictByCaseName("t2").TestIdStructured},
				}

				res, err := srv.BatchGetTestVerdicts(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				testDataMasked := testverdictsv2.ToBasicView(testverdictsv2.ExpectedMaskedVerdicts(testverdictsv2.ExpectedVerdicts(rootInvID), []string{"testproject:t3-r1"}))
				expected := []*pb.TestVerdict{
					testverdictsv2.VerdictByCaseName(testDataMasked, "t3"),
					testverdictsv2.VerdictByCaseName(testDataMasked, "t2"),
				}

				assert.Loosely(t, res.TestVerdicts, should.Match(expected))
			})
		})
	})
}
