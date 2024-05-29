// Copyright 2024 The LUCI Authors.
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

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidQueryArtifactFailureOnlyLinesRequest(t *testing.T) {
	t.Parallel()
	Convey(`TestValidateQueryArtifactFailureOnlyLinesRequest`, t, func() {

		Convey(`Valid, invocation level`, func() {
			err := validateListArtifactLinesRequest(&pb.ListArtifactLinesRequest{
				Parent:   "invocations/x/artifacts/artifact-id",
				PageSize: 50,
			})
			So(err, ShouldBeNil)
		})

		Convey(`Valid, test result level`, func() {
			err := validateListArtifactLinesRequest(&pb.ListArtifactLinesRequest{
				Parent:   "invocations/x/tests/t%20t/results/r/artifacts/artifact-id",
				PageSize: 50,
			})
			So(err, ShouldBeNil)
		})

		Convey(`Invalid parent`, func() {
			err := validateListArtifactLinesRequest(&pb.ListArtifactLinesRequest{
				Parent: "x",
			})
			So(err, ShouldErrLike, `parent: invalid artifact name`)
		})

		Convey(`Invalid page size`, func() {
			err := validateListArtifactLinesRequest(&pb.ListArtifactLinesRequest{
				Parent:   "invocations/x/artifacts/artifact-id",
				PageSize: -1,
			})
			So(err, ShouldErrLike, `page_size: negative`)
		})
	})
}

func TestQueryArtifactFailureOnlyLines(t *testing.T) {
	contentString := `2024-05-06T05:58:57.490076Z ERROR test[9617:9617]: log line 1
2024-05-06T05:58:57.491037Z VERBOSE1 test[9617:9617]: [file.cc(845)] log line 2
2024-05-06T05:58:57.577095Z WARNING test[9617:9617]: [file.cc(89)] log line 3.
2024-05-06T05:58:57.577324Z INFO test[9617:9617]: [file.cc(140)] log line 4 {
	log line no timestamp
}`

	Convey("TestListArtifactLines", t, func() {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermGetArtifact},
			},
		})
		srv := newTestResultDBServiceWithArtifactContent(contentString)

		Convey("given invalid permissions, then should return permission denied error", func() {
			// Insert a Artifact.
			testutil.MustApply(ctx,
				insert.Invocation("inv", pb.Invocation_ACTIVE, map[string]any{"Realm": "secretproject:testrealm"}),
				insert.Artifact("inv", "", "a", nil),
			)
			req := &pb.QueryArtifactFailureOnlyLinesRequest{Parent: "invocations/inv/artifacts/a"}
			_, err := srv.QueryArtifactFailureOnlyLines(ctx, req)
			So(err, ShouldBeRPCPermissionDenied, "caller does not have permission resultdb.artifacts.get")
		})

		Convey("given an invocation level artifact, then should return invalid argument error", func() {
			// Insert a Artifact.
			testutil.MustApply(ctx,
				insert.Invocation("inv", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:testrealm"}),
				insert.Artifact("inv", "", "a", nil),
			)
			req := &pb.QueryArtifactFailureOnlyLinesRequest{Parent: "invocations/inv/artifacts/b"}
			_, err := srv.QueryArtifactFailureOnlyLines(ctx, req)
			So(err, ShouldBeRPCInvalidArgument, "only test artifacts are supported")
		})

		Convey("given a non-existent invocation, then should return not found error", func() {
			// Insert a Artifact.
			testutil.MustApply(ctx,
				insert.Invocation("inv", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:testrealm"}),
				insert.Artifact("inv", "", "a", nil),
			)
			req := &pb.QueryArtifactFailureOnlyLinesRequest{Parent: "invocations/inv2/artifacts/a"}
			_, err := srv.QueryArtifactFailureOnlyLines(ctx, req)
			So(err, ShouldBeRPCNotFound, "invocations/inv2 not found")
		})
	})
}
