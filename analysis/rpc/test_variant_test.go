// Copyright 2022 The LUCI Authors.
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

package rpc

import (
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	grpcStatus "google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"

	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/perms"
	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/internal/testresults/stability"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestTestVariantsServer(t *testing.T) {
	ftt.Run("Given a test variants server", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)

		// For user identification.
		ctx = authtest.MockAuthConfig(ctx)
		authState := &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"luci-analysis-access"},
		}
		ctx = auth.WithState(ctx, authState)
		ctx = secrets.Use(ctx, &testsecrets.Store{})

		// Provides datastore implementation needed for project config.
		ctx = memory.Use(ctx)
		server := NewTestVariantsServer()

		t.Run("Unauthorised requests are rejected", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				// Not a member of luci-analysis-access.
				IdentityGroups: []string{"other-group"},
			})

			// Make some request (the request should not matter, as
			// a common decorator is used for all requests.)
			request := &pb.QueryTestVariantStabilityRequest{}

			response, err := server.QueryStability(ctx, request)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("not a member of luci-analysis-access"))
			assert.Loosely(t, response, should.BeNil)
		})
		t.Run("QueryFailureRate", func(t *ftt.Test) {
			// Grant the permissions needed for this RPC.
			authState.IdentityPermissions = []authtest.RealmPermission{
				{
					Realm:      "project:realm",
					Permission: rdbperms.PermListTestResults,
				},
			}

			err := testresults.CreateQueryFailureRateTestData(ctx)
			assert.Loosely(t, err, should.BeNil)

			t.Run("Valid input", func(t *ftt.Test) {
				project, asAtTime, tvs := testresults.QueryFailureRateSampleRequest()
				request := &pb.QueryTestVariantFailureRateRequest{
					Project:      project,
					TestVariants: tvs,
				}
				ctx, _ := testclock.UseTime(ctx, asAtTime)

				response, err := server.QueryFailureRate(ctx, request)
				assert.Loosely(t, err, should.BeNil)

				expectedResult := testresults.QueryFailureRateSampleResponse()
				assert.Loosely(t, response, should.Resemble(expectedResult))
			})
			t.Run("Query by VariantHash", func(t *ftt.Test) {
				project, asAtTime, tvs := testresults.QueryFailureRateSampleRequest()
				for _, tv := range tvs {
					tv.VariantHash = pbutil.VariantHash(tv.Variant)
					tv.Variant = nil
				}
				request := &pb.QueryTestVariantFailureRateRequest{
					Project:      project,
					TestVariants: tvs,
				}
				ctx, _ := testclock.UseTime(ctx, asAtTime)

				response, err := server.QueryFailureRate(ctx, request)
				assert.Loosely(t, err, should.BeNil)

				expectedResult := testresults.QueryFailureRateSampleResponse()
				for _, tv := range expectedResult.TestVariants {
					tv.VariantHash = pbutil.VariantHash(tv.Variant)
					tv.Variant = nil
				}
				assert.Loosely(t, response, should.Resemble(expectedResult))
			})
			t.Run("No list test results permission", func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					{
						// This permission is for a project other than the one
						// being queried.
						Realm:      "otherproject:realm",
						Permission: rdbperms.PermListTestResults,
					},
				}

				project, asAtTime, tvs := testresults.QueryFailureRateSampleRequest()
				request := &pb.QueryTestVariantFailureRateRequest{
					Project:      project,
					TestVariants: tvs,
				}
				ctx, _ := testclock.UseTime(ctx, asAtTime)

				response, err := server.QueryFailureRate(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("caller does not have permissions [resultdb.testResults.list] in any realm"))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("Invalid input", func(t *ftt.Test) {
				// This checks at least one case of invalid input is detected, sufficient to verify
				// validation is invoked.
				// Exhaustive checking of request validation is performed in TestValidateQueryRateRequest.
				request := &pb.QueryTestVariantFailureRateRequest{
					Project: "",
					TestVariants: []*pb.TestVariantIdentifier{
						{
							TestId: "my_test",
						},
					},
				}

				response, err := server.QueryFailureRate(ctx, request)
				st, _ := grpcStatus.FromError(err)
				assert.Loosely(t, st.Code(), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, st.Message(), should.Equal(`project: unspecified`))
				assert.Loosely(t, response, should.BeNil)
			})
		})
		t.Run("QueryStability", func(t *ftt.Test) {
			// Grant the permissions needed for this RPC.
			authState.IdentityPermissions = []authtest.RealmPermission{
				{
					Realm:      "project:realm",
					Permission: rdbperms.PermListTestResults,
				},
				{
					Realm:      "project:@project",
					Permission: perms.PermGetConfig,
				},
			}

			err := stability.CreateQueryStabilityTestData(ctx)
			assert.Loosely(t, err, should.BeNil)

			opts := stability.QueryStabilitySampleRequest()
			request := &pb.QueryTestVariantStabilityRequest{
				Project:      opts.Project,
				TestVariants: opts.TestVariantPositions,
			}
			ctx, _ := testclock.UseTime(ctx, opts.AsAtTime)

			projectCfg := config.CreateConfigWithBothBuganizerAndMonorail(configpb.BugSystem_MONORAIL)
			projectCfg.TestStabilityCriteria = toTestStabilityCriteriaConfig(opts.Criteria)
			configs := make(map[string]*configpb.ProjectConfig)
			configs["project"] = projectCfg
			err = config.SetTestProjectConfig(ctx, configs)
			assert.Loosely(t, err, should.BeNil)

			t.Run("Valid input", func(t *ftt.Test) {
				rsp, err := server.QueryStability(ctx, request)
				assert.Loosely(t, err, should.BeNil)

				expectedResult := &pb.QueryTestVariantStabilityResponse{
					TestVariants: stability.QueryStabilitySampleResponse(),
					Criteria:     opts.Criteria,
				}
				assert.Loosely(t, rsp, should.Resemble(expectedResult))
			})
			t.Run("Query by VariantHash", func(t *ftt.Test) {
				for _, tv := range request.TestVariants {
					tv.VariantHash = pbutil.VariantHash(tv.Variant)
					tv.Variant = nil
				}
				rsp, err := server.QueryStability(ctx, request)
				assert.Loosely(t, err, should.BeNil)

				expectedAnalysis := stability.QueryStabilitySampleResponse()
				for _, tv := range expectedAnalysis {
					tv.VariantHash = pbutil.VariantHash(tv.Variant)
					tv.Variant = nil
				}
				expectedResult := &pb.QueryTestVariantStabilityResponse{
					TestVariants: expectedAnalysis,
					Criteria:     opts.Criteria,
				}
				assert.Loosely(t, rsp, should.Resemble(expectedResult))
			})
			t.Run("No test stability configuration", func(t *ftt.Test) {
				// Remove test stability configuration.
				projectCfg.TestStabilityCriteria = nil
				err = config.SetTestProjectConfig(ctx, configs)
				assert.Loosely(t, err, should.BeNil)

				response, err := server.QueryStability(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
				assert.Loosely(t, err, should.ErrLike("project has not defined test stability criteria; set test_stability_criteria in project configuration and try again"))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("No list test results permission", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, rdbperms.PermListTestResults)

				response, err := server.QueryStability(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("caller does not have permissions [resultdb.testResults.list] in any realm"))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("No get project config permission", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetConfig)

				response, err := server.QueryStability(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike(`caller does not have permission analysis.config.get in realm "project:@project"`))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("Invalid input", func(t *ftt.Test) {
				// This checks at least one case of invalid input is detected, sufficient to verify
				// validation is invoked.
				// Exhaustive checking of request validation is performed in
				// TestValidateQueryTestVariantStabilityRequest.
				request.Project = ""

				response, err := server.QueryStability(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`project: unspecified`))
				assert.Loosely(t, response, should.BeNil)
			})
		})
	})
}

func TestValidateQueryFailureRateRequest(t *testing.T) {
	ftt.Run("ValidateQueryFailureRateRequest", t, func(t *ftt.Test) {
		req := &pb.QueryTestVariantFailureRateRequest{
			Project: "project",
			TestVariants: []*pb.TestVariantIdentifier{
				{
					TestId: "my_test",
					// Variant is optional as not all tests have variants.
				},
				{
					TestId:  "my_test2",
					Variant: pbutil.Variant("key1", "val1", "key2", "val2"),
				},
			},
		}

		t.Run("valid", func(t *ftt.Test) {
			err := validateQueryTestVariantFailureRateRequest(req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("no project", func(t *ftt.Test) {
			req.Project = ""
			err := validateQueryTestVariantFailureRateRequest(req)
			assert.Loosely(t, err, should.ErrLike("project: unspecified"))
		})

		t.Run("invalid project", func(t *ftt.Test) {
			req.Project = ":"
			err := validateQueryTestVariantFailureRateRequest(req)
			assert.Loosely(t, err, should.ErrLike(`project: must match ^[a-z0-9\-]{1,40}$`))
		})

		t.Run("no test variants", func(t *ftt.Test) {
			req.TestVariants = nil
			err := validateQueryTestVariantFailureRateRequest(req)
			assert.Loosely(t, err, should.ErrLike(`test_variants: unspecified`))
		})

		t.Run("too many test variants", func(t *ftt.Test) {
			req.TestVariants = make([]*pb.TestVariantIdentifier, 0, 101)
			for i := 0; i < 101; i++ {
				req.TestVariants = append(req.TestVariants, &pb.TestVariantIdentifier{
					TestId: fmt.Sprintf("test_id%v", i),
				})
			}
			err := validateQueryTestVariantFailureRateRequest(req)
			assert.Loosely(t, err, should.ErrLike(`no more than 100 may be queried at a time`))
		})

		t.Run("no test id", func(t *ftt.Test) {
			req.TestVariants[1].TestId = ""
			err := validateQueryTestVariantFailureRateRequest(req)
			assert.Loosely(t, err, should.ErrLike(`test_variants[1]: test_id: unspecified`))
		})

		t.Run("variant_hash invalid", func(t *ftt.Test) {
			req.TestVariants[1].VariantHash = "invalid"
			err := validateQueryTestVariantFailureRateRequest(req)
			assert.Loosely(t, err, should.ErrLike(`test_variants[1]: variant_hash: must match ^[0-9a-f]{16}$`))
		})

		t.Run("variant_hash mismatch with variant", func(t *ftt.Test) {
			req.TestVariants[1].VariantHash = "0123456789abcdef"
			err := validateQueryTestVariantFailureRateRequest(req)
			assert.Loosely(t, err, should.ErrLike(`test_variants[1]: variant and variant_hash mismatch`))
		})

		t.Run("duplicate test variants", func(t *ftt.Test) {
			req.TestVariants = []*pb.TestVariantIdentifier{
				{
					TestId:  "my_test",
					Variant: pbutil.Variant("key1", "val1", "key2", "val2"),
				},
				{
					TestId:  "my_test",
					Variant: pbutil.Variant("key1", "val1", "key2", "val2"),
				},
			}
			err := validateQueryTestVariantFailureRateRequest(req)
			assert.Loosely(t, err, should.ErrLike(`test_variants[1]: already requested in the same request`))
		})
	})
}

func TestValidateQueryTestVariantStabilityRequest(t *testing.T) {
	ftt.Run("ValidateQueryTestVariantStabilityRequest", t, func(t *ftt.Test) {
		req := &pb.QueryTestVariantStabilityRequest{
			Project: "project",
			TestVariants: []*pb.QueryTestVariantStabilityRequest_TestVariantPosition{
				{
					TestId: "my_test",
					// Variant is optional as not all tests have variants.
					Sources: testSources(),
				},
				{
					TestId:  "my_test2",
					Variant: pbutil.Variant("key1", "val1", "key2", "val2"),
					Sources: testSources(),
				},
			},
		}

		t.Run("valid", func(t *ftt.Test) {
			err := validateQueryTestVariantStabilityRequest(req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("no project", func(t *ftt.Test) {
			req.Project = ""
			err := validateQueryTestVariantStabilityRequest(req)
			assert.Loosely(t, err, should.ErrLike("project: unspecified"))
		})

		t.Run("invalid project", func(t *ftt.Test) {
			req.Project = ":"
			err := validateQueryTestVariantStabilityRequest(req)
			assert.Loosely(t, err, should.ErrLike(`project: must match ^[a-z0-9\-]{1,40}$`))
		})

		t.Run("no test variants", func(t *ftt.Test) {
			req.TestVariants = nil
			err := validateQueryTestVariantStabilityRequest(req)
			assert.Loosely(t, err, should.ErrLike(`test_variants: unspecified`))
		})

		t.Run("too many test variants", func(t *ftt.Test) {
			req.TestVariants = make([]*pb.QueryTestVariantStabilityRequest_TestVariantPosition, 0, 101)
			for i := 0; i < 101; i++ {
				req.TestVariants = append(req.TestVariants, &pb.QueryTestVariantStabilityRequest_TestVariantPosition{
					TestId:  fmt.Sprintf("test_id%v", i),
					Sources: testSources(),
				})
			}
			err := validateQueryTestVariantStabilityRequest(req)
			assert.Loosely(t, err, should.ErrLike(`no more than 100 may be queried at a time`))
		})

		t.Run("no test id", func(t *ftt.Test) {
			req.TestVariants[1].TestId = ""
			err := validateQueryTestVariantStabilityRequest(req)
			assert.Loosely(t, err, should.ErrLike(`test_variants[1]: test_id: unspecified`))
		})

		t.Run("variant_hash invalid", func(t *ftt.Test) {
			req.TestVariants[1].VariantHash = "invalid"
			err := validateQueryTestVariantStabilityRequest(req)
			assert.Loosely(t, err, should.ErrLike(`test_variants[1]: variant_hash: must match ^[0-9a-f]{16}$`))
		})

		t.Run("variant_hash mismatch with variant", func(t *ftt.Test) {
			req.TestVariants[1].VariantHash = "0123456789abcdef"
			err := validateQueryTestVariantStabilityRequest(req)
			assert.Loosely(t, err, should.ErrLike(`test_variants[1]: variant and variant_hash mismatch`))
		})

		t.Run("variant_hash only", func(t *ftt.Test) {
			req.TestVariants[1].Variant = nil
			req.TestVariants[1].VariantHash = "0123456789abcdef"
			err := validateQueryTestVariantStabilityRequest(req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("no sources", func(t *ftt.Test) {
			req.TestVariants[1].Sources = nil
			err := validateQueryTestVariantStabilityRequest(req)
			assert.Loosely(t, err, should.ErrLike(`test_variants[1]: sources: unspecified`))
		})

		t.Run("invalid sources", func(t *ftt.Test) {
			// This checks at least one case of invalid input is detected, sufficient to verify
			// sources validation is invoked.
			// Exhaustive checking of sources validation is performed in pbutil.
			req.TestVariants[1].Sources.GitilesCommit.Host = ""
			err := validateQueryTestVariantStabilityRequest(req)
			assert.Loosely(t, err, should.ErrLike(`test_variants[1]: sources: gitiles_commit: host: unspecified`))
		})

		t.Run("multiple branches of same test variant", func(t *ftt.Test) {
			sources2 := testSources()
			sources2.GitilesCommit.Ref = "refs/heads/other"
			req.TestVariants = append(req.TestVariants, []*pb.QueryTestVariantStabilityRequest_TestVariantPosition{
				{
					TestId:  "my_test",
					Variant: pbutil.Variant("key1", "val1", "key2", "val2"),
					Sources: testSources(),
				},
				{
					TestId:  "my_test",
					Variant: pbutil.Variant("key1", "val1", "key2", "val2"),
					Sources: sources2,
				},
			}...)
			err := validateQueryTestVariantStabilityRequest(req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("duplicate test variant branches", func(t *ftt.Test) {
			req.TestVariants = append(req.TestVariants, []*pb.QueryTestVariantStabilityRequest_TestVariantPosition{
				{
					TestId:  "my_test",
					Variant: pbutil.Variant("key1", "val1", "key2", "val2"),
					Sources: testSources(),
				},
				{
					TestId:  "my_test",
					Variant: pbutil.Variant("key1", "val1", "key2", "val2"),
					Sources: testSources(),
				},
			}...)
			err := validateQueryTestVariantStabilityRequest(req)
			assert.Loosely(t, err, should.ErrLike(`test_variants[3]: same test variant branch already requested at index 2`))
		})
	})
}

func toTestStabilityCriteriaConfig(criteria *pb.TestStabilityCriteria) *configpb.TestStabilityCriteria {
	return &configpb.TestStabilityCriteria{
		FailureRate: &configpb.TestStabilityCriteria_FailureRateCriteria{
			FailureThreshold:            criteria.FailureRate.FailureThreshold,
			ConsecutiveFailureThreshold: criteria.FailureRate.ConsecutiveFailureThreshold,
		},
		FlakeRate: &configpb.TestStabilityCriteria_FlakeRateCriteria{
			MinWindow:          criteria.FlakeRate.MinWindow,
			FlakeThreshold:     criteria.FlakeRate.FlakeThreshold,
			FlakeRateThreshold: criteria.FlakeRate.FlakeRateThreshold,
			FlakeThreshold_1Wd: criteria.FlakeRate.FlakeThreshold_1Wd,
		},
	}
}

func testSources() *pb.Sources {
	result := &pb.Sources{
		GitilesCommit: &pb.GitilesCommit{
			Host:       "chromium.googlesource.com",
			Project:    "infra/infra",
			Ref:        "refs/heads/main",
			CommitHash: "1234567890abcdefabcd1234567890abcdefabcd",
			Position:   12345,
		},
		IsDirty: true,
		Changelists: []*pb.GerritChange{
			{
				Host:     "chromium-review.googlesource.com",
				Project:  "myproject",
				Change:   87654,
				Patchset: 321,
			},
		},
	}
	return result
}
