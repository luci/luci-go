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
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/grpc/codes"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/resultdb/internal/gsutil"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
)

func TestValidateListArtifactsRequest(t *testing.T) {
	t.Parallel()
	Convey(`TestValidateListArtifactsRequest`, t, func() {
		Convey(`Valid, invocation level`, func() {
			err := validateListArtifactsRequest(&pb.ListArtifactsRequest{
				Parent:   "invocations/x",
				PageSize: 50,
			})
			So(err, ShouldBeNil)
		})

		Convey(`Valid, test result level`, func() {
			err := validateListArtifactsRequest(&pb.ListArtifactsRequest{
				Parent:   "invocations/x/tests/t%20t/results/r",
				PageSize: 50,
			})
			So(err, ShouldBeNil)
		})

		Convey(`Invalid parent`, func() {
			err := validateListArtifactsRequest(&pb.ListArtifactsRequest{
				Parent: "x",
			})
			So(err, ShouldErrLike, `parent: neither valid invocation name nor valid test result name`)
		})

		Convey(`Invalid page size`, func() {
			err := validateListArtifactsRequest(&pb.ListArtifactsRequest{
				Parent:   "invocations/x",
				PageSize: -1,
			})
			So(err, ShouldErrLike, `page_size: negative`)
		})
	})
}

func TestListArtifacts(t *testing.T) {
	Convey(`ListArtifacts`, t, func() {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermListArtifacts},
			},
		})

		testutil.MustApply(ctx,
			insert.Invocation("inv1", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:testrealm"}),
			insert.Invocation("invx", pb.Invocation_ACTIVE, map[string]any{"Realm": "secretproject:testrealm"}),
		)
		req := &pb.ListArtifactsRequest{
			Parent:   "invocations/inv1",
			PageSize: 100,
		}

		srv := newTestResultDBService()

		mustFetch := func(req *pb.ListArtifactsRequest) (arts []*pb.Artifact, token string) {
			// Add SignedURL opts to ctx
			opts := testutil.GetSignedURLOptions(ctx)
			ctx := context.WithValue(ctx, gsutil.Key("signedURLOpts"), opts)

			res, err := srv.ListArtifacts(ctx, req)
			So(err, ShouldBeNil)
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

		Convey(`Permission denied`, func() {
			req.Parent = "invocations/invx/tests/t%20t/results/r"
			_, err := srv.ListArtifacts(ctx, req)
			So(err, ShouldHaveAppStatus, codes.PermissionDenied)
		})

		Convey(`With both invocation and test result artifacts`, func() {
			testutil.MustApply(ctx,
				insert.Artifact("inv1", "", "a", nil),
				spanutil.InsertMap("Artifacts", map[string]any{
					"InvocationId": invocations.ID("inv1"),
					"ParentID":     "tr/t t/r",
					"ArtifactId":   "a",
				}),
			)

			Convey(`Reads only invocation artifacts`, func() {
				req.Parent = "invocations/inv1"
				actual := mustFetchNames(req)
				So(actual, ShouldResemble, []string{
					"invocations/inv1/artifacts/a",
				})
			})

			Convey(`Reads only test result artifacts`, func() {
				req.Parent = "invocations/inv1/tests/t%20t/results/r"
				actual := mustFetchNames(req)
				So(actual, ShouldResemble, []string{
					"invocations/inv1/tests/t%20t/results/r/artifacts/a",
				})
			})
		})

		Convey(`Fetch URL`, func() {
			testutil.MustApply(ctx,
				insert.Artifact("inv1", "", "a", nil),
			)
			actual, _ := mustFetch(req)
			So(actual, ShouldHaveLength, 1)
			So(actual[0].FetchUrl, ShouldEqual, "https://signed-url.example.com/invocations/inv1/artifacts/a")
		})

		Convey(`Fetch URL with Gcs URI`, func() {
			testutil.MustApply(ctx,
				insert.Artifact("inv1", "", "a", map[string]any{"GcsURI": "gs://bucket1/file1.txt"}),
			)

			actual, _ := mustFetch(req)
			So(actual, ShouldHaveLength, 1)
			So(actual[0].FetchUrl, ShouldStartWith, "https://storage.googleapis.com/bucket1/file1.txt?X-Goog-Algorithm=GOOG4-RSA-SHA256&X-Goog-Credential")
		})

	})
}
