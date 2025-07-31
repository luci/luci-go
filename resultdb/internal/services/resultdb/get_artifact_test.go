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
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/gsutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

var textPBMultiline = prototext.MarshalOptions{
	Multiline: true,
}

func TestValidateGetArtifactRequest(t *testing.T) {
	t.Parallel()
	ftt.Run(`ValidateGetArtifactRequest`, t, func(t *ftt.Test) {
		t.Run(`Valid`, func(t *ftt.Test) {
			req := &pb.GetArtifactRequest{Name: "invocations/inv/artifacts/a"}
			assert.Loosely(t, validateGetArtifactRequest(req), should.BeNil)
		})

		t.Run(`Invalid name`, func(t *ftt.Test) {
			req := &pb.GetArtifactRequest{}
			assert.Loosely(t, validateGetArtifactRequest(req), should.ErrLike("unspecified"))
		})
	})
}

func AssertFetchURLCorrectness(ctx context.Context, t testing.TB, a *pb.Artifact) {
	t.Helper()
	fetchURL, err := url.Parse(a.FetchUrl)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	assert.Loosely(t, fetchURL.Query().Get("token"), should.NotBeEmpty, truth.LineContext())
	assert.Loosely(t, fetchURL.RawPath, should.Equal("/"+a.Name), truth.LineContext())

	assert.Loosely(t, a.FetchUrlExpiration, should.NotBeNil, truth.LineContext())
	assert.Loosely(t, pbutil.MustTimestamp(a.FetchUrlExpiration), should.HappenWithin(10*time.Second, clock.Now(ctx)), truth.LineContext())
}

func TestGetArtifact(t *testing.T) {
	ftt.Run(`GetArtifact`, t, func(t *ftt.Test) {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermGetArtifact},
			},
		})
		srv := newTestResultDBService()

		t.Run(`Permission denied`, func(t *ftt.Test) {
			// Insert a Artifact.
			testutil.MustApply(ctx, t,
				insert.Invocation("inv", pb.Invocation_ACTIVE, map[string]any{"Realm": "secretproject:testrealm"}),
				insert.Artifact("inv", "", "a", nil),
			)
			req := &pb.GetArtifactRequest{Name: "invocations/inv/artifacts/a"}
			_, err := srv.GetArtifact(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("caller does not have permission resultdb.artifacts.get"))
		})

		t.Run(`Exists`, func(t *ftt.Test) {
			// Insert a Artifact.
			testutil.MustApply(ctx, t,
				insert.Invocation("inv", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:testrealm"}),
				insert.Artifact("inv", "", "a", nil),
			)
			const name = "invocations/inv/artifacts/a"
			req := &pb.GetArtifactRequest{Name: name}
			art, err := srv.GetArtifact(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, art.Name, should.Equal(name))
			assert.Loosely(t, art.ArtifactId, should.Equal("a"))
			assert.Loosely(t, strings.HasPrefix(art.FetchUrl, "https://signed-url.example.com/invocations/inv/artifacts/a"), should.BeTrue)
		})

		t.Run(`Exists with gcsURI`, func(t *ftt.Test) {
			const realm = "testrealm"
			const project = "testproject"
			const bucket = "bucket1"
			const object = "file1.txt"
			var globalRealm = fmt.Sprintf("%s:%s", project, realm)

			// Insert an Artifact.
			testutil.MustApply(ctx, t,
				insert.Invocation("inv", pb.Invocation_ACTIVE, map[string]any{"Realm": globalRealm}),
				insert.Artifact("inv", "", "a", map[string]any{"GcsURI": fmt.Sprintf("gs://%s/%s", bucket, object)}),
			)
			const name = "invocations/inv/artifacts/a"
			req := &pb.GetArtifactRequest{Name: name}
			// Set up cloud storage test client.
			ctx = context.WithValue(ctx, &gsutil.MockedGSClientKey, &gsutil.MockClient{})

			art, err := srv.GetArtifact(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, art.Name, should.Equal(name))
			assert.Loosely(t, art.ArtifactId, should.Equal("a"))
			assert.Loosely(t, art.FetchUrl, should.Equal("https://fake-signed-url/bucket1/file1.txt?x-project=testproject"))
		})

		t.Run(`Does not exist`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Invocation("inv", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:testrealm"}))
			req := &pb.GetArtifactRequest{Name: "invocations/inv/artifacts/a"}
			_, err := srv.GetArtifact(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("invocations/inv/artifacts/a not found"))
		})
		t.Run(`Invocation does not exist`, func(t *ftt.Test) {
			req := &pb.GetArtifactRequest{Name: "invocations/inv/artifacts/a"}
			_, err := srv.GetArtifact(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("invocations/inv not found"))
		})
	})
}
