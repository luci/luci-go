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
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	artifactcontenttest "go.chromium.org/luci/resultdb/internal/artifactcontent/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestValidListArtifactLinesRequest(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestValidateListArtifactsRequest`, t, func(t *ftt.Test) {

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

func TestListArtifactLines(t *testing.T) {

	verifyArtifactLine := func(line *pb.ArtifactLine, timestamp string, severity pb.ArtifactLine_Severity) {
		if timestamp != "" {
			ts, err := time.Parse(time.RFC3339Nano, timestamp)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, line.Timestamp, should.Match(timestamppb.New(ts)))
		}
		assert.Loosely(t, line.Severity, should.Equal(severity))
	}

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
			req := &pb.ListArtifactLinesRequest{Parent: "invocations/inv/artifacts/a"}
			_, err := srv.ListArtifactLines(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("caller does not have permission resultdb.artifacts.get"))
		})

		t.Run("given a non-existent artifact, then should return not found error", func(t *ftt.Test) {
			// Insert a Artifact.
			testutil.MustApply(ctx, t,
				insert.Invocation("inv", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:testrealm"}),
				insert.Artifact("inv", "", "a", nil),
			)
			req := &pb.ListArtifactLinesRequest{Parent: "invocations/inv/artifacts/b"}
			_, err := srv.ListArtifactLines(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("invocations/inv/artifacts/b not found"))
		})

		t.Run("given a non-existent invocation, then should return not found error", func(t *ftt.Test) {
			// Insert a Artifact.
			testutil.MustApply(ctx, t,
				insert.Invocation("inv", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:testrealm"}),
				insert.Artifact("inv", "", "a", nil),
			)
			req := &pb.ListArtifactLinesRequest{Parent: "invocations/inv2/artifacts/a"}
			_, err := srv.ListArtifactLines(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("invocations/inv2 not found"))
		})

		t.Run("given an not found error from the content reader, then should return not found", func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Invocation("inv", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:testrealm"}),
				insert.Artifact("inv", "", "a", map[string]any{
					"Size": len(contentString),
				}),
			)
			const name = "invocations/inv/artifacts/a"
			req := &pb.ListArtifactLinesRequest{Parent: name}
			casReader := &artifactcontenttest.FakeCASReader{
				ResErr: status.Errorf(codes.NotFound, "not found"),
			}
			errSrv := newTestResultDBServiceWithCASReader(casReader)
			_, err := errSrv.ListArtifactLines(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.ErrIsLike(t, err, "artifact not found")
		})

		t.Run("given valid content, then should return valid lines", func(t *ftt.Test) {
			// Insert a Artifact.
			testutil.MustApply(ctx, t,
				insert.Invocation("inv", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:testrealm"}),
				insert.Artifact("inv", "", "a", map[string]any{
					"Size": len(contentString),
				}),
			)
			const name = "invocations/inv/artifacts/a"
			req := &pb.ListArtifactLinesRequest{Parent: name}
			res, err := srv.ListArtifactLines(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(res.Lines), should.Equal(6))
			lines := res.Lines
			assert.Loosely(t, lines[0].Content, should.Match([]byte("2024-05-06T05:58:57.490076Z ERROR test[9617:9617]: log line 1")))
			verifyArtifactLine(lines[0], "2024-05-06T05:58:57.490076Z", pb.ArtifactLine_ERROR)
			assert.Loosely(t, lines[1].Content, should.Match([]byte("2024-05-06T05:58:57.491037Z VERBOSE1 test[9617:9617]: [file.cc(845)] log line 2")))
			verifyArtifactLine(lines[1], "2024-05-06T05:58:57.491037Z", pb.ArtifactLine_VERBOSE)
			assert.Loosely(t, lines[2].Content, should.Match([]byte("2024-05-06T05:58:57.577095Z WARNING test[9617:9617]: [file.cc(89)] log line 3.")))
			verifyArtifactLine(lines[2], "2024-05-06T05:58:57.577095Z", pb.ArtifactLine_WARNING)
			assert.Loosely(t, lines[3].Content, should.Match([]byte("2024-05-06T05:58:57.577324Z INFO test[9617:9617]: [file.cc(140)] log line 4 {")))
			verifyArtifactLine(lines[3], "2024-05-06T05:58:57.577324Z", pb.ArtifactLine_INFO)
			assert.Loosely(t, lines[4].Content, should.Match([]byte("	log line no timestamp")))
			verifyArtifactLine(lines[4], "", pb.ArtifactLine_SEVERITY_UNSPECIFIED)
		})

		t.Run("given a page size, should return at most that size", func(t *ftt.Test) {
			// Insert a Artifact.
			testutil.MustApply(ctx, t,
				insert.Invocation("inv", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:testrealm"}),
				insert.Artifact("inv", "", "a", map[string]any{
					"Size": len(contentString),
				}),
			)
			const name = "invocations/inv/artifacts/a"
			req := &pb.ListArtifactLinesRequest{
				Parent:   name,
				PageSize: 3,
			}
			res, err := srv.ListArtifactLines(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(res.Lines), should.Equal(3))
		})
	})
}
