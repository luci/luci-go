// Copyright 2020 The LUCI Authors.
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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/gsutil"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestValidateListArtifactsRequest(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestValidateListArtifactsRequest`, t, func(t *ftt.Test) {
		t.Run(`Valid, invocation level`, func(t *ftt.Test) {
			err := validateListArtifactsRequest(&pb.ListArtifactsRequest{
				Parent:   "invocations/x",
				PageSize: 50,
			})
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`Valid, test result level`, func(t *ftt.Test) {
			err := validateListArtifactsRequest(&pb.ListArtifactsRequest{
				Parent:   "invocations/x/tests/t%20t/results/r",
				PageSize: 50,
			})
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`Invalid parent`, func(t *ftt.Test) {
			err := validateListArtifactsRequest(&pb.ListArtifactsRequest{
				Parent: "x",
			})
			assert.Loosely(t, err, should.ErrLike(`parent: neither valid invocation name nor valid test result name`))
		})

		t.Run(`Invalid page size`, func(t *ftt.Test) {
			err := validateListArtifactsRequest(&pb.ListArtifactsRequest{
				Parent:   "invocations/x",
				PageSize: -1,
			})
			assert.Loosely(t, err, should.ErrLike(`page_size: negative`))
		})
	})
}

func TestListArtifacts(t *testing.T) {
	ftt.Run(`ListArtifacts`, t, func(t *ftt.Test) {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermListArtifacts},
			},
		})

		testutil.MustApply(ctx, t,
			insert.Invocation("inv1", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:testrealm"}),
			insert.Invocation("invx", pb.Invocation_ACTIVE, map[string]any{"Realm": "secretproject:testrealm"}),
		)
		req := &pb.ListArtifactsRequest{
			Parent:   "invocations/inv1",
			PageSize: 100,
		}

		srv := newTestResultDBService()

		mustFetch := func(req *pb.ListArtifactsRequest) (arts []*pb.Artifact, token string) {
			// Set up cloud storage test client.
			ctx = context.WithValue(ctx, &gsutil.MockedGSClientKey, &gsutil.MockClient{})

			res, err := srv.ListArtifacts(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			return res.Artifacts, res.NextPageToken
		}

		mustFetchNames := func(req *pb.ListArtifactsRequest) []string {
			arts, _ := mustFetch(req)
			names := make([]string, len(arts))
			for i, a := range arts {
				names[i] = a.Name
			}
			return names
		}

		t.Run(`Permission denied`, func(t *ftt.Test) {
			req.Parent = "invocations/invx/tests/t%20t/results/r"
			_, err := srv.ListArtifacts(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("caller does not have permission resultdb.artifacts.list in realm of invocation invx"))
		})

		t.Run(`With both invocation and test result artifacts`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Artifact("inv1", "", "a", nil),
				spanutil.InsertMap("Artifacts", map[string]any{
					"InvocationId": invocations.ID("inv1"),
					"ParentID":     "tr/t t/r",
					"ArtifactId":   "a",
				}),
			)

			t.Run(`Reads only invocation artifacts`, func(t *ftt.Test) {
				req.Parent = "invocations/inv1"
				actual := mustFetchNames(req)
				assert.Loosely(t, actual, should.Match([]string{
					"invocations/inv1/artifacts/a",
				}))
			})

			t.Run(`Reads only test result artifacts`, func(t *ftt.Test) {
				req.Parent = "invocations/inv1/tests/t%20t/results/r"
				actual := mustFetchNames(req)
				assert.Loosely(t, actual, should.Match([]string{
					"invocations/inv1/tests/t%20t/results/r/artifacts/a",
				}))
			})
		})

		t.Run(`Fetch URL`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Artifact("inv1", "", "a", nil),
			)
			actual, _ := mustFetch(req)
			assert.Loosely(t, actual, should.HaveLength(1))
			assert.Loosely(t, strings.HasPrefix(actual[0].FetchUrl, "https://signed-url.example.com/invocations/inv1/artifacts/a"), should.BeTrue)
		})

		t.Run(`Fetch URL with Gcs URI`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Artifact("inv1", "", "a", map[string]any{"GcsURI": "gs://bucket1/file1.txt"}),
			)

			actual, _ := mustFetch(req)
			assert.Loosely(t, actual, should.HaveLength(1))
			assert.Loosely(t, actual[0].FetchUrl, should.Equal("https://fake-signed-url/bucket1/file1.txt?x-project=testproject"))
		})
	})
}
