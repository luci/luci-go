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
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"

	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTestVariantsServer(t *testing.T) {
	Convey("Given a projects server", t, func() {
		ctx := testutil.IntegrationTestContext(t)

		// For user identification.
		ctx = authtest.MockAuthConfig(ctx)
		authState := &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"luci-analysis-access"},
			IdentityPermissions: []authtest.RealmPermission{
				{
					Realm:      "project:realm",
					Permission: rdbperms.PermListTestResults,
				},
			},
		}
		ctx = auth.WithState(ctx, authState)
		ctx = secrets.Use(ctx, &testsecrets.Store{})

		// Provides datastore implementation needed for project config.
		ctx = memory.Use(ctx)
		server := NewTestVariantsServer()

		Convey("Unauthorised requests are rejected", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				// Not a member of luci-analysis-access.
				IdentityGroups: []string{"other-group"},
			})

			// Make some request (the request should not matter, as
			// a common decorator is used for all requests.)
			request := &pb.QueryTestVariantStabilityRequest{}

			response, err := server.QueryStability(ctx, request)
			So(err, ShouldBeRPCPermissionDenied, "not a member of luci-analysis-access")
			So(response, ShouldBeNil)
		})
		Convey("QueryFailureRate", func() {
			err := testresults.CreateQueryFailureRateTestData(ctx)
			So(err, ShouldBeNil)

			Convey("Valid input", func() {
				project, asAtTime, tvs := testresults.QueryFailureRateSampleRequest()
				request := &pb.QueryTestVariantFailureRateRequest{
					Project:      project,
					TestVariants: tvs,
				}
				ctx, _ := testclock.UseTime(ctx, asAtTime)

				response, err := server.QueryFailureRate(ctx, request)
				So(err, ShouldBeNil)

				expectedResult := testresults.QueryFailureRateSampleResponse()
				So(response, ShouldResembleProto, expectedResult)
			})
			Convey("Query by VariantHash", func() {
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
				So(err, ShouldBeNil)

				expectedResult := testresults.QueryFailureRateSampleResponse()
				for _, tv := range expectedResult.TestVariants {
					tv.VariantHash = pbutil.VariantHash(tv.Variant)
					tv.Variant = nil
				}
				So(response, ShouldResembleProto, expectedResult)
			})
			Convey("No list test results permission", func() {
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
				So(err, ShouldBeRPCPermissionDenied, "caller does not have permissions [resultdb.testResults.list] in any realm")
				So(response, ShouldBeNil)
			})
			Convey("Invalid input", func() {
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
				So(st.Code(), ShouldEqual, codes.InvalidArgument)
				So(st.Message(), ShouldEqual, `project: unspecified`)
				So(response, ShouldBeNil)
			})
		})
		Convey("QueryStability", func() {
			request := &pb.QueryTestVariantStabilityRequest{
				Project: "project",
				TestVariants: []*pb.QueryTestVariantStabilityRequest_TestVariantPosition{
					{
						TestId:  "test_id",
						Variant: pbutil.Variant("key1", "val1", "key2", "val1"),
						Sources: testSources(),
					},
					{
						TestId:  "test_id",
						Variant: pbutil.Variant("key1", "val2", "key2", "val2"),
						Sources: testSources(),
					},
				},
			}

			Convey("Valid input", func() {
				_, err := server.QueryStability(ctx, request)
				So(err, ShouldHaveRPCCode, codes.Unimplemented)
			})
			Convey("Query by VariantHash", func() {
				for _, tv := range request.TestVariants {
					tv.VariantHash = pbutil.VariantHash(tv.Variant)
					tv.Variant = nil
				}
				_, err := server.QueryStability(ctx, request)
				So(err, ShouldHaveRPCCode, codes.Unimplemented)
			})
			Convey("No list test results permission", func() {
				authState.IdentityPermissions = []authtest.RealmPermission{
					{
						// This permission is for a project other than the one
						// being queried.
						Realm:      "otherproject:realm",
						Permission: rdbperms.PermListTestResults,
					},
				}

				response, err := server.QueryStability(ctx, request)
				So(err, ShouldBeRPCPermissionDenied, "caller does not have permissions [resultdb.testResults.list] in any realm")
				So(response, ShouldBeNil)
			})
			Convey("Invalid input", func() {
				// This checks at least one case of invalid input is detected, sufficient to verify
				// validation is invoked.
				// Exhaustive checking of request validation is performed in TestValidateQueryRateRequest.
				request.Project = ""

				response, err := server.QueryStability(ctx, request)
				So(err, ShouldBeRPCInvalidArgument, `project: unspecified`)
				So(response, ShouldBeNil)
			})
		})
	})
}

func TestValidateQueryFailureRateRequest(t *testing.T) {
	Convey("ValidateQueryFailureRateRequest", t, func() {
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

		Convey("valid", func() {
			err := validateQueryTestVariantFailureRateRequest(req)
			So(err, ShouldBeNil)
		})

		Convey("no project", func() {
			req.Project = ""
			err := validateQueryTestVariantFailureRateRequest(req)
			So(err, ShouldErrLike, "project: unspecified")
		})

		Convey("invalid project", func() {
			req.Project = ":"
			err := validateQueryTestVariantFailureRateRequest(req)
			So(err, ShouldErrLike, `project: must match ^[a-z0-9\-]{1,40}$`)
		})

		Convey("no test variants", func() {
			req.TestVariants = nil
			err := validateQueryTestVariantFailureRateRequest(req)
			So(err, ShouldErrLike, `test_variants: unspecified`)
		})

		Convey("too many test variants", func() {
			req.TestVariants = make([]*pb.TestVariantIdentifier, 0, 101)
			for i := 0; i < 101; i++ {
				req.TestVariants = append(req.TestVariants, &pb.TestVariantIdentifier{
					TestId: fmt.Sprintf("test_id%v", i),
				})
			}
			err := validateQueryTestVariantFailureRateRequest(req)
			So(err, ShouldErrLike, `no more than 100 may be queried at a time`)
		})

		Convey("no test id", func() {
			req.TestVariants[1].TestId = ""
			err := validateQueryTestVariantFailureRateRequest(req)
			So(err, ShouldErrLike, `test_variants[1]: test_id: unspecified`)
		})

		Convey("variant_hash invalid", func() {
			req.TestVariants[1].VariantHash = "invalid"
			err := validateQueryTestVariantFailureRateRequest(req)
			So(err, ShouldErrLike, `test_variants[1]: variant_hash: must match ^[0-9a-f]{16}$`)
		})

		Convey("variant_hash mismatch with variant", func() {
			req.TestVariants[1].VariantHash = "0123456789abcdef"
			err := validateQueryTestVariantFailureRateRequest(req)
			So(err, ShouldErrLike, `test_variants[1]: variant and variant_hash mismatch`)
		})

		Convey("duplicate test variants", func() {
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
			So(err, ShouldErrLike, `test_variants[1]: already requested in the same request`)
		})
	})
}

func TestValidateQueryTestVariantStabilityRequest(t *testing.T) {
	Convey("ValidateQueryTestVariantStabilityRequest", t, func() {
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

		Convey("valid", func() {
			err := validateQueryTestVariantStabilityRequest(req)
			So(err, ShouldBeNil)
		})

		Convey("no project", func() {
			req.Project = ""
			err := validateQueryTestVariantStabilityRequest(req)
			So(err, ShouldErrLike, "project: unspecified")
		})

		Convey("invalid project", func() {
			req.Project = ":"
			err := validateQueryTestVariantStabilityRequest(req)
			So(err, ShouldErrLike, `project: must match ^[a-z0-9\-]{1,40}$`)
		})

		Convey("no test variants", func() {
			req.TestVariants = nil
			err := validateQueryTestVariantStabilityRequest(req)
			So(err, ShouldErrLike, `test_variants: unspecified`)
		})

		Convey("too many test variants", func() {
			req.TestVariants = make([]*pb.QueryTestVariantStabilityRequest_TestVariantPosition, 0, 101)
			for i := 0; i < 101; i++ {
				req.TestVariants = append(req.TestVariants, &pb.QueryTestVariantStabilityRequest_TestVariantPosition{
					TestId:  fmt.Sprintf("test_id%v", i),
					Sources: testSources(),
				})
			}
			err := validateQueryTestVariantStabilityRequest(req)
			So(err, ShouldErrLike, `no more than 100 may be queried at a time`)
		})

		Convey("no test id", func() {
			req.TestVariants[1].TestId = ""
			err := validateQueryTestVariantStabilityRequest(req)
			So(err, ShouldErrLike, `test_variants[1]: test_id: unspecified`)
		})

		Convey("variant_hash invalid", func() {
			req.TestVariants[1].VariantHash = "invalid"
			err := validateQueryTestVariantStabilityRequest(req)
			So(err, ShouldErrLike, `test_variants[1]: variant_hash: must match ^[0-9a-f]{16}$`)
		})

		Convey("variant_hash mismatch with variant", func() {
			req.TestVariants[1].VariantHash = "0123456789abcdef"
			err := validateQueryTestVariantStabilityRequest(req)
			So(err, ShouldErrLike, `test_variants[1]: variant and variant_hash mismatch`)
		})

		Convey("variant_hash only", func() {
			req.TestVariants[1].Variant = nil
			req.TestVariants[1].VariantHash = "0123456789abcdef"
			err := validateQueryTestVariantStabilityRequest(req)
			So(err, ShouldBeNil)
		})

		Convey("no sources", func() {
			req.TestVariants[1].Sources = nil
			err := validateQueryTestVariantStabilityRequest(req)
			So(err, ShouldErrLike, `test_variants[1]: sources: unspecified`)
		})

		Convey("invalid sources", func() {
			// This checks at least one case of invalid input is detected, sufficient to verify
			// sources validation is invoked.
			// Exhaustive checking of sources validation is performed in pbutil.
			req.TestVariants[1].Sources.GitilesCommit.Host = ""
			err := validateQueryTestVariantStabilityRequest(req)
			So(err, ShouldErrLike, `test_variants[1]: sources: gitiles_commit: host: unspecified`)
		})

		Convey("duplicate test variants", func() {
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
			So(err, ShouldErrLike, `test_variants[3]: same test variant already requested at index 2`)
		})
	})
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
