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

package testhistory

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/analysis/internal/testrealms"
	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/internal/testresults/lowlatency"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestQueryRecentPasses(t *testing.T) {
	ftt.Run("QueryRecentPasses", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:            "user:someone@example.com",
			IdentityPermissions: defaultAuthPermissions,
		})
		now := referenceTime.Add(time.Minute * 20)
		ctx, _ = testclock.UseTime(ctx, now)

		err := createPassingResultsTestData(ctx, t)
		assert.Loosely(t, err, should.BeNil)

		searchClient := &testrealms.FakeClient{}
		server := NewTestHistoryServer(searchClient)

		baseReq := &pb.QueryRecentPassesRequest{
			Project:     "project",
			TestId:      "ninja://test/id",
			VariantHash: "e5aa3a34a834a74f",
			Sources: &pb.Sources{
				BaseSources: &pb.Sources_GitilesCommit{
					GitilesCommit: &pb.GitilesCommit{
						Host:     "my-g-host",
						Project:  "my-g-proj",
						Ref:      "refs/heads/main",
						Position: 105,
					},
				},
			},
			Limit: 5,
		}

		t.Run("invalid requests are rejected", func(t *ftt.Test) {
			req := proto.Clone(baseReq).(*pb.QueryRecentPassesRequest)
			req.Project = ""
			_, err := server.QueryRecentPasses(ctx, req)
			assert.Loosely(t, err, should.ErrLike("project: unspecified"))
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))

			req = proto.Clone(baseReq).(*pb.QueryRecentPassesRequest)
			req.TestId = "\xff" // Invalid UTF-8.
			_, err = server.QueryRecentPasses(ctx, req)
			assert.Loosely(t, err, should.ErrLike("test_id: not a valid utf8 string"))
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))

			req = proto.Clone(baseReq).(*pb.QueryRecentPassesRequest)
			req.VariantHash = "invalid"
			_, err = server.QueryRecentPasses(ctx, req)
			assert.Loosely(t, err, should.ErrLike("variant_hash: variant hash invalid must match"))
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))

			req = proto.Clone(baseReq).(*pb.QueryRecentPassesRequest)
			req.Limit = -1
			_, err = server.QueryRecentPasses(ctx, req)
			assert.Loosely(t, err, should.ErrLike("limit: must be non-negative"))
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})

		t.Run("unauthorised requests are rejected", func(t *ftt.Test) {
			// No permission at all.
			unauthCtx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
			})
			_, err := server.QueryRecentPasses(unauthCtx, baseReq)
			assert.Loosely(t, err, should.ErrLike(`caller does not have permissions [resultdb.testResults.list] in any realm in project "project"`))
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
		})

		t.Run("e2e - source position only (gitiles)", func(t *ftt.Test) {
			req := proto.Clone(baseReq).(*pb.QueryRecentPassesRequest)
			res, err := server.QueryRecentPasses(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			// inv-3 has position 102, inv-2 has position 101, inv-legacy-1 & inv-legacy-2 have position 103 & 104. All are <= 105.
			// inv-1 (pos 106) is excluded.
			assert.Loosely(t, res.PassingResults, should.HaveLength(3))
			// From both inv-legacy-1 & inv-legacy-2.
			assert.That(t, res.PassingResults[0].Name, should.Equal("invocations/task-1/tests/ninja:%2F%2Ftest%2Fid/results/result-id"))
			assert.That(t, res.PassingResults[1].Name, should.Equal("rootInvocations/inv-3/workUnits/wu-id/tests/ninja:%2F%2Ftest%2Fid/results/result-id"))
			assert.That(t, res.PassingResults[2].Name, should.Equal("rootInvocations/inv-2/workUnits/wu-id/tests/ninja:%2F%2Ftest%2Fid/results/result-id"))
		})

		t.Run("e2e - source position only (android)", func(t *ftt.Test) {
			req := proto.Clone(baseReq).(*pb.QueryRecentPassesRequest)
			req.Sources = &pb.Sources{
				BaseSources: &pb.Sources_SubmittedAndroidBuild{
					SubmittedAndroidBuild: &pb.SubmittedAndroidBuild{
						DataRealm: "prod",
						Branch:    "git_main",
						BuildId:   2002,
					},
				},
			}
			res, err := server.QueryRecentPasses(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.PassingResults, should.HaveLength(2))
			assert.That(t, res.PassingResults[0].Name, should.Equal("rootInvocations/inv-android-2/workUnits/wu-id/tests/ninja:%2F%2Ftest%2Fid/results/result-id"))
			assert.That(t, res.PassingResults[1].Name, should.Equal("rootInvocations/inv-android-1/workUnits/wu-id/tests/ninja:%2F%2Ftest%2Fid/results/result-id"))
		})

		t.Run("e2e - limit is respected", func(t *ftt.Test) {
			req := proto.Clone(baseReq).(*pb.QueryRecentPassesRequest)
			req.Limit = 2
			res, err := server.QueryRecentPasses(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.PassingResults, should.HaveLength(2))
			assert.That(t, res.PassingResults[0].Name, should.Equal("invocations/task-1/tests/ninja:%2F%2Ftest%2Fid/results/result-id"))
			assert.That(t, res.PassingResults[1].Name, should.Equal("rootInvocations/inv-3/workUnits/wu-id/tests/ninja:%2F%2Ftest%2Fid/results/result-id"))
		})
	})
}

var (
	gitilesRef = &pb.SourceRef{
		System: &pb.SourceRef_Gitiles{
			Gitiles: &pb.GitilesRef{Host: "my-g-host", Project: "my-g-proj", Ref: "refs/heads/main"},
		},
	}
	gitilesRefHash = pbutil.SourceRefHash(gitilesRef)

	androidRef = &pb.SourceRef{
		System: &pb.SourceRef_AndroidBuild{
			AndroidBuild: &pb.AndroidBuildBranch{DataRealm: "prod", Branch: "git_main"},
		},
	}
	androidRefHash = pbutil.SourceRefHash(androidRef)
)

func createPassingResultsTestData(ctx context.Context, t *ftt.Test) error {
	// Common values.
	project := "project"
	testID := "ninja://test/id"
	variantHash := "e5aa3a34a834a74f"
	baseBuilder := lowlatency.NewTestResult().
		WithProject(project).
		WithTestID(testID).
		WithVariantHash(variantHash).
		WithIsUnexpected(false)

	results := []*lowlatency.TestResult{
		// Gitiles passes
		baseBuilder.WithSubRealm("realm").WithSources(testresults.Sources{RefHash: gitilesRefHash, Position: 101}).WithRootInvocationID("inv-2").WithStatus(pb.TestResultStatus_PASS).Build(),
		baseBuilder.WithSubRealm("other-realm").WithSources(testresults.Sources{RefHash: gitilesRefHash, Position: 102}).WithRootInvocationID("inv-3").WithStatus(pb.TestResultStatus_PASS).Build(),
		baseBuilder.WithSubRealm("realm").WithSources(testresults.Sources{RefHash: gitilesRefHash, Position: 103}).WithLegacyRootInvocationID("inv-legacy1").WithLegacyInvocationID("task-1").WithStatus(pb.TestResultStatus_PASS).Build(),
		baseBuilder.WithSubRealm("realm").WithSources(testresults.Sources{RefHash: gitilesRefHash, Position: 104}).WithLegacyRootInvocationID("inv-legacy2").WithLegacyInvocationID("task-1").WithStatus(pb.TestResultStatus_PASS).Build(),
		// A fail to ensure we filter by status.
		baseBuilder.WithSubRealm("realm").WithSources(testresults.Sources{RefHash: gitilesRefHash, Position: 103}).WithRootInvocationID("inv-fail").WithStatus(pb.TestResultStatus_FAIL).Build(),
		// A pass in an unauthorized realm.
		baseBuilder.WithSubRealm("forbidden-realm").WithSources(testresults.Sources{RefHash: gitilesRefHash, Position: 104}).WithRootInvocationID("inv-unauth").WithStatus(pb.TestResultStatus_PASS).Build(),
		// Should be excluded because position is > 105.
		baseBuilder.WithSubRealm("realm").WithSources(testresults.Sources{RefHash: gitilesRefHash, Position: 106}).WithRootInvocationID("inv-1").WithStatus(pb.TestResultStatus_PASS).Build(),

		// Android passes
		baseBuilder.WithSubRealm("realm").WithSources(testresults.Sources{RefHash: androidRefHash, Position: 2001}).WithRootInvocationID("inv-android-1").WithStatus(pb.TestResultStatus_PASS).Build(),
		baseBuilder.WithSubRealm("realm").WithSources(testresults.Sources{RefHash: androidRefHash, Position: 2002}).WithRootInvocationID("inv-android-2").WithStatus(pb.TestResultStatus_PASS).Build(),
	}

	return lowlatency.SetForTesting(ctx, t, results)
}
