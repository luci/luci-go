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

	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestValidQueryArtifactFailureOnlyLinesRequest(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestValidateQueryArtifactFailureOnlyLinesRequest`, t, func(t *ftt.Test) {

		t.Run(`Valid, invocation level`, func(t *ftt.Test) {
			err := validateListArtifactLinesRequest(&pb.ListArtifactLinesRequest{
				Parent:   "invocations/x/artifacts/artifact-id",
				PageSize: 50,
			})
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`Valid, test result level`, func(t *ftt.Test) {
			err := validateListArtifactLinesRequest(&pb.ListArtifactLinesRequest{
				Parent:   "invocations/x/tests/t%20t/results/r/artifacts/artifact-id",
				PageSize: 50,
			})
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`Invalid parent`, func(t *ftt.Test) {
			err := validateListArtifactLinesRequest(&pb.ListArtifactLinesRequest{
				Parent: "x",
			})
			assert.Loosely(t, err, should.ErrLike(`parent: invalid artifact name`))
		})

		t.Run(`Invalid page size`, func(t *ftt.Test) {
			err := validateListArtifactLinesRequest(&pb.ListArtifactLinesRequest{
				Parent:   "invocations/x/artifacts/artifact-id",
				PageSize: -1,
			})
			assert.Loosely(t, err, should.ErrLike(`page_size: negative`))
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

	ftt.Run("TestListArtifactLines", t, func(t *ftt.Test) {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermGetArtifact},
			},
		})
		srv := newTestResultDBServiceWithArtifactContent(contentString)

		t.Run("given invalid permissions, then should return permission denied error", func(t *ftt.Test) {
			// Insert a Artifact.
			testutil.MustApply(ctx, t,
				insert.Invocation("inv", pb.Invocation_ACTIVE, map[string]any{"Realm": "secretproject:testrealm"}),
				insert.Artifact("inv", "", "a", nil),
			)
			req := &pb.QueryArtifactFailureOnlyLinesRequest{Parent: "invocations/inv/artifacts/a"}
			_, err := srv.QueryArtifactFailureOnlyLines(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("caller does not have permission resultdb.artifacts.get"))
		})

		t.Run("given an invocation level artifact, then should return invalid argument error", func(t *ftt.Test) {
			// Insert a Artifact.
			testutil.MustApply(ctx, t,
				insert.Invocation("inv", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:testrealm"}),
				insert.Artifact("inv", "", "a", nil),
			)
			req := &pb.QueryArtifactFailureOnlyLinesRequest{Parent: "invocations/inv/artifacts/b"}
			_, err := srv.QueryArtifactFailureOnlyLines(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("only test artifacts are supported"))
		})

		t.Run("given a non-existent invocation, then should return not found error", func(t *ftt.Test) {
			// Insert a Artifact.
			testutil.MustApply(ctx, t,
				insert.Invocation("inv", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:testrealm"}),
				insert.Artifact("inv", "", "a", nil),
			)
			req := &pb.QueryArtifactFailureOnlyLinesRequest{Parent: "invocations/inv2/artifacts/a"}
			_, err := srv.QueryArtifactFailureOnlyLines(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("invocations/inv2 not found"))
		})
	})
}
