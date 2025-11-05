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
	"context"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"google.golang.org/genproto/googleapis/bytestream"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/artifactcontent"
	artifactcontenttest "go.chromium.org/luci/resultdb/internal/artifactcontent/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

// newTestResultDBServiceForCompare creates a test service with a mock CAS
// reader that can serve different content for different hashes.
func newTestResultDBServiceForCompare(contentMap map[string][]byte) pb.ResultDBServer {
	svr := &resultDBServer{
		contentServer: &artifactcontent.Server{
			HostnameProvider: func(string) string {
				return "signed-url.example.com"
			},
			RBECASInstanceName: "projects/example/instances/artifacts",
			ReadCASBlob: func(ctx context.Context, req *bytestream.ReadRequest) (bytestream.ByteStream_ReadClient, error) {
				// The resource name is projects/{p}/instances/{i}/blobs/{hash}/{size}.
				// We use the hash as the key in our contentMap.
				parts := strings.Split(req.ResourceName, "/")
				if len(parts) < 3 || parts[len(parts)-3] != "blobs" {
					return nil, status.Errorf(codes.InvalidArgument, "invalid resource name format: %s", req.ResourceName)
				}
				hash := parts[len(parts)-2]

				content, ok := contentMap[hash]
				if !ok {
					return nil, status.Errorf(codes.NotFound, "hash %q not found in test content map", hash)
				}

				// Return a new fake reader configured with the correct content for this request.
				return &artifactcontenttest.FakeCASReader{
					Res: []*bytestream.ReadResponse{
						{Data: content},
					},
				}, nil
			},
		},
	}
	return &pb.DecoratedResultDB{
		Service:  svr,
		Postlude: internal.CommonPostlude,
	}
}

// insertTestResultLegacy is a helper to create and insert a single legacy test result.
func insertTestResultLegacy(t *ftt.Test, invID, testID, resultID string, status pb.TestStatus) []*spanner.Mutation {
	tr := &pb.TestResult{
		Name:     pbutil.LegacyTestResultName(invID, testID, resultID),
		TestId:   testID,
		ResultId: resultID,
		Status:   status,
	}
	return insert.TestResultMessagesLegacy(t, []*pb.TestResult{tr})
}

func TestValidateCompareArtifactLinesRequest(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestValidateCompareArtifactLinesRequest`, t, func(t *ftt.Test) {
		t.Run(`Valid, legacy test artifact`, func(t *ftt.Test) {
			err := validateCompareArtifactLinesRequest(&pb.CompareArtifactLinesRequest{
				Name:           "invocations/inv/tests/t/results/r/artifacts/a",
				PassingResults: []string{"invocations/inv-pass/tests/t/results/r"},
			})
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`Valid, V2 work-unit artifact`, func(t *ftt.Test) {
			err := validateCompareArtifactLinesRequest(&pb.CompareArtifactLinesRequest{
				Name:           "rootInvocations/inv/workUnits/wu/artifacts/a",
				PassingResults: []string{"rootInvocations/inv-pass/workUnits/wu/tests/t/results/r"},
			})
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`Invalid name`, func(t *ftt.Test) {
			err := validateCompareArtifactLinesRequest(&pb.CompareArtifactLinesRequest{
				Name:           "bad-name",
				PassingResults: []string{"invocations/inv-pass/tests/t/results/r"},
			})
			assert.Loosely(t, err, should.ErrLike(`name: invalid artifact name`))
		})

		t.Run(`No passing results`, func(t *ftt.Test) {
			err := validateCompareArtifactLinesRequest(&pb.CompareArtifactLinesRequest{
				Name:           "invocations/inv/tests/t/results/r/artifacts/a",
				PassingResults: []string{},
			})
			assert.Loosely(t, err, should.ErrLike(`passing_result_names: must provide at least one`))
		})

		t.Run(`Invalid passing result name`, func(t *ftt.Test) {
			err := validateCompareArtifactLinesRequest(&pb.CompareArtifactLinesRequest{
				Name:           "invocations/inv/tests/t/results/r/artifacts/a",
				PassingResults: []string{"bad-passing-name"},
			})
			assert.Loosely(t, err, should.ErrLike(`passing_result_names[0]: invalid test result name`))
		})

		t.Run(`Invalid page size`, func(t *ftt.Test) {
			err := validateCompareArtifactLinesRequest(&pb.CompareArtifactLinesRequest{
				Name:           "invocations/inv/tests/t/results/r/artifacts/a",
				PassingResults: []string{"invocations/inv-pass/tests/t/results/r"},
				PageSize:       -1,
			})
			assert.Loosely(t, err, should.ErrLike(`page_size: negative`))
		})
	})
}

func TestCompareArtifactLines(t *testing.T) {
	ftt.Run("CompareArtifactLines", t, func(t *ftt.Test) {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermGetArtifact},
			},
		})

		contentMap := map[string][]byte{
			"rbscas-hash-pass": []byte("passing line"),
			"rbscas-hash-fail": []byte("failing line"),
		}
		srv := newTestResultDBServiceForCompare(contentMap)

		t.Run("Permission denied", func(t *ftt.Test) {
			muts := []*spanner.Mutation{
				insert.Invocation("inv-secret", pb.Invocation_FINALIZED, map[string]any{"Realm": "secretproject:testrealm"}),
				// FIX: Use the "tr/..." format for ParentId.
				insert.Artifact("inv-secret", "tr/t/r", "a", map[string]any{"RBECASHash": "rbscas-hash-fail"}),
			}
			muts = append(muts, insertTestResultLegacy(t, "inv-secret", "t", "r", pb.TestStatus_FAIL)...)
			testutil.MustApply(ctx, t, muts...)

			req := &pb.CompareArtifactLinesRequest{
				Name:           "invocations/inv-secret/tests/t/results/r/artifacts/a",
				PassingResults: []string{"invocations/inv-pass/tests/t/results/r"},
			}
			_, err := srv.CompareArtifactLines(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("resultdb.artifacts.get"))
		})

		t.Run("Not found", func(t *ftt.Test) {
			req := &pb.CompareArtifactLinesRequest{
				Name:           "invocations/inv-nonexistent/tests/t/results/r/artifacts/a",
				PassingResults: []string{"invocations/inv-pass/tests/t/results/r"},
			}
			_, err := srv.CompareArtifactLines(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("invocations/inv-nonexistent not found"))
		})

		t.Run("Happy path - test-level artifact", func(t *ftt.Test) {
			muts := []*spanner.Mutation{
				insert.Invocation("inv-fail", pb.Invocation_FINALIZED, map[string]any{"Realm": "testproject:testrealm"}),
				// FIX: Changed parentId to the "tr/testID/resultID" format.
				insert.Artifact("inv-fail", "tr/t/r-fail", "a", map[string]any{"RBECASHash": "rbscas-hash-fail"}),
				insert.Invocation("inv-pass", pb.Invocation_FINALIZED, map[string]any{"Realm": "testproject:testrealm"}),
				// FIX: Changed parentId to the "tr/testID/resultID" format.
				insert.Artifact("inv-pass", "tr/t/r-pass", "a", map[string]any{"RBECASHash": "rbscas-hash-pass"}),
			}
			muts = append(muts, insertTestResultLegacy(t, "inv-fail", "t", "r-fail", pb.TestStatus_FAIL)...)
			muts = append(muts, insertTestResultLegacy(t, "inv-pass", "t", "r-pass", pb.TestStatus_PASS)...)
			testutil.MustApply(ctx, t, muts...)

			req := &pb.CompareArtifactLinesRequest{
				Name:           "invocations/inv-fail/tests/t/results/r-fail/artifacts/a",
				PassingResults: []string{"invocations/inv-pass/tests/t/results/r-pass"},
			}
			res, err := srv.CompareArtifactLines(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.NotBeNil)
			assert.Loosely(t, len(res.FailureOnlyRanges), should.Equal(1))
		})

		t.Run("Happy path - invocation-level artifact", func(t *ftt.Test) {
			muts := []*spanner.Mutation{
				insert.Invocation("inv-fail-inv", pb.Invocation_FINALIZED, map[string]any{"Realm": "testproject:testrealm"}),
				insert.Artifact("inv-fail-inv", "", "inv-log", map[string]any{"RBECASHash": "rbscas-hash-fail"}),
				insert.Invocation("inv-pass-inv", pb.Invocation_FINALIZED, map[string]any{"Realm": "testproject:testrealm"}),
				insert.Artifact("inv-pass-inv", "", "inv-log", map[string]any{"RBECASHash": "rbscas-hash-pass"}),
			}
			muts = append(muts, insertTestResultLegacy(t, "inv-fail-inv", "t", "r-fail", pb.TestStatus_FAIL)...)
			muts = append(muts, insertTestResultLegacy(t, "inv-pass-inv", "t", "r-pass", pb.TestStatus_PASS)...)
			testutil.MustApply(ctx, t, muts...)

			req := &pb.CompareArtifactLinesRequest{
				Name:           "invocations/inv-fail-inv/artifacts/inv-log",
				PassingResults: []string{"invocations/inv-pass-inv/tests/t/results/r-pass"},
			}
			res, err := srv.CompareArtifactLines(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.NotBeNil)
			assert.Loosely(t, len(res.FailureOnlyRanges), should.Equal(1))
		})
	})
}
