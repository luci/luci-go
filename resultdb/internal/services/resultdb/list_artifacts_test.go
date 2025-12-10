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

	"cloud.google.com/go/spanner"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/gsutil"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestListArtifacts(t *testing.T) {
	ftt.Run(`ListArtifacts`, t, func(t *ftt.Test) {
		authState := &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:invrealm", Permission: rdbperms.PermListArtifacts},
				{Realm: "testproject:rootrealm", Permission: rdbperms.PermListArtifacts},
			},
		}
		ctx := auth.WithState(testutil.SpannerTestContext(t), authState)

		rootInv := rootinvocations.NewBuilder("root-inv1").WithFinalizationState(pb.RootInvocation_ACTIVE).WithRealm("testproject:rootrealm").Build()
		wu := workunits.NewBuilder(rootInv.RootInvocationID, "wu1").WithFinalizationState(pb.WorkUnit_ACTIVE).WithRealm("testproject:wurealm").Build()

		var ms []*spanner.Mutation
		ms = append(ms, insert.RootInvocationWithRootWorkUnit(rootInv)...)
		ms = append(ms, insert.WorkUnit(wu)...)
		ms = append(ms, insert.Invocation("inv1", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:invrealm"}))
		ms = append(ms, insert.Invocation("invx", pb.Invocation_ACTIVE, map[string]any{"Realm": "secretproject:invrealm"}))
		testutil.MustApply(ctx, t, ms...)

		srv := newTestResultDBService()

		mustFetch := func(req *pb.ListArtifactsRequest) (arts []*pb.Artifact, token string) {
			// Set up cloud storage test client.
			ctx = context.WithValue(ctx, &gsutil.MockedGSClientKey, &gsutil.MockClient{})

			res, err := srv.ListArtifacts(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			return res.Artifacts, res.NextPageToken
		}

		artifactsNames := func(arts []*pb.Artifact) []string {
			names := make([]string, len(arts))
			for i, a := range arts {
				names[i] = a.Name
			}
			return names
		}

		// Define a valid request.
		req := &pb.ListArtifactsRequest{
			Parent:   "rootInvocations/root-inv1/workUnits/wu1",
			PageSize: 100,
		}

		t.Run(`request validation`, func(t *ftt.Test) {
			t.Run(`parent`, func(t *ftt.Test) {
				t.Run(`empty`, func(t *ftt.Test) {
					req.Parent = ""
					_, err := srv.ListArtifacts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike(`parent: unspecified`))
				})
				t.Run(`invalid`, func(t *ftt.Test) {
					req.Parent = "x"
					_, err := srv.ListArtifacts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike(`parent: neither a valid work unit name, test result name or legacy invocation name`))
				})
				t.Run(`invalid (legacy invocation-like)`, func(t *ftt.Test) {
					req.Parent = "invocations/ x"
					_, err := srv.ListArtifacts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike(`parent: neither a valid work unit name, test result name or legacy invocation name`))
				})
			})
			t.Run(`page size`, func(t *ftt.Test) {
				t.Run(`invalid`, func(t *ftt.Test) {
					req.PageSize = -1
					_, err := srv.ListArtifacts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike(`page_size: negative`))
				})
			})
		})

		t.Run(`request authorization`, func(t *ftt.Test) {
			t.Run(`work units`, func(t *ftt.Test) {
				req.Parent = "rootInvocations/root-inv1/workUnits/wu1"

				t.Run(`no root-level list access`, func(t *ftt.Test) {
					authState.IdentityPermissions = []authtest.RealmPermission{}

					_, err := srv.ListArtifacts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.Loosely(t, err, should.ErrLike(`caller does not have permission resultdb.artifacts.list or resultdb.artifacts.listLimited in realm of "rootInvocations/root-inv1"`))
				})
				t.Run(`with limited list access only`, func(t *ftt.Test) {
					authState.IdentityPermissions = []authtest.RealmPermission{
						{Realm: "testproject:rootrealm", Permission: rdbperms.PermListLimitedArtifacts},
					}

					_, err := srv.ListArtifacts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.Loosely(t, err, should.ErrLike(`caller does not have permission resultdb.artifacts.get in the realm of "rootInvocations/root-inv1/workUnits/wu1" (trying to upgrade limited artifact access to full access)`))
				})
				t.Run(`with limited list access upgraded to full access`, func(t *ftt.Test) {
					authState.IdentityPermissions = []authtest.RealmPermission{
						{Realm: "testproject:rootrealm", Permission: rdbperms.PermListLimitedArtifacts},
						{Realm: "testproject:wurealm", Permission: rdbperms.PermGetArtifact},
					}

					// Authorized.
					_, err := srv.ListArtifacts(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
			})
			t.Run(`legacy invocation`, func(t *ftt.Test) {
				req.Parent = "invocations/invx/tests/t%20t/results/r"
				_, err := srv.ListArtifacts(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("caller does not have permission resultdb.artifacts.list in realm of \"invocations/invx\""))
			})
		})
		t.Run(`work units`, func(t *ftt.Test) {
			t.Run(`with both work unit and test result artifacts`, func(t *ftt.Test) {
				testutil.MustApply(ctx, t,
					insert.Artifact(wu.ID.LegacyInvocationID(), "", "a", nil),
					insert.Artifact(wu.ID.LegacyInvocationID(), "", "b", map[string]any{"GcsURI": "gs://bucket1/file1.txt"}),
					insert.Artifact(wu.ID.LegacyInvocationID(), "", "c", map[string]any{"RbeURI": "projects/testproject/instances/default_instance/blobs/work_unit_artifact/20"}),
					insert.Artifact(wu.ID.LegacyInvocationID(), "tr/t t/r", "a", nil),
					insert.Artifact(wu.ID.LegacyInvocationID(), "tr/t t/r", "e", map[string]any{"GcsURI": "gs://bucket1/file2.txt"}),
					insert.Artifact(wu.ID.LegacyInvocationID(), "tr/t t/r", "f", map[string]any{"RbeURI": "projects/testproject/instances/default_instance/blobs/test_artifact/10"}),
				)

				t.Run(`Reads only invocation artifacts`, func(t *ftt.Test) {
					req.Parent = "rootInvocations/root-inv1/workUnits/wu1"
					arts, nextPageToken := mustFetch(req)
					assert.Loosely(t, artifactsNames(arts), should.Match([]string{
						"rootInvocations/root-inv1/workUnits/wu1/artifacts/a",
						"rootInvocations/root-inv1/workUnits/wu1/artifacts/b",
						"rootInvocations/root-inv1/workUnits/wu1/artifacts/c",
					}))
					assert.Loosely(t, nextPageToken, should.BeEmpty)

					// Verify fetch URLs were generated for each type of artifact.
					assert.Loosely(t, arts[0].FetchUrl, should.HavePrefix("https://signed-url.example.com/rootInvocations/root-inv1/workUnits/wu1/artifacts/a"))
					assert.Loosely(t, arts[1].FetchUrl, should.HavePrefix("https://fake-signed-url/bucket1/file1.txt?x-project=testproject"))
					assert.Loosely(t, arts[2].FetchUrl, should.HavePrefix("https://signed-url.example.com/rootInvocations/root-inv1/workUnits/wu1/artifacts/c"))
				})

				t.Run(`Reads only test result artifacts`, func(t *ftt.Test) {
					req.Parent = "rootInvocations/root-inv1/workUnits/wu1/tests/t%20t/results/r"
					arts, nextPageToken := mustFetch(req)
					assert.Loosely(t, artifactsNames(arts), should.Match([]string{
						"rootInvocations/root-inv1/workUnits/wu1/tests/t%20t/results/r/artifacts/a",
						"rootInvocations/root-inv1/workUnits/wu1/tests/t%20t/results/r/artifacts/e",
						"rootInvocations/root-inv1/workUnits/wu1/tests/t%20t/results/r/artifacts/f",
					}))
					assert.Loosely(t, nextPageToken, should.BeEmpty)

					// Verify fetch URLs were generated for each type of artifact.
					assert.Loosely(t, arts[0].FetchUrl, should.HavePrefix("https://signed-url.example.com/rootInvocations/root-inv1/workUnits/wu1/tests/t%20t/results/r/artifacts/a"))
					assert.Loosely(t, arts[1].FetchUrl, should.HavePrefix("https://fake-signed-url/bucket1/file2.txt?x-project=testproject"))
					assert.Loosely(t, arts[2].FetchUrl, should.HavePrefix("https://signed-url.example.com/rootInvocations/root-inv1/workUnits/wu1/tests/t%20t/results/r/artifacts/f"))
				})
			})
		})

		t.Run(`Read mask`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Artifact(wu.ID.LegacyInvocationID(), "", "a", map[string]any{"ContentType": "text/plain", "Size": 100}),
				insert.Artifact(wu.ID.LegacyInvocationID(), "", "b", map[string]any{"GcsURI": "gs://bucket1/file1.txt"}),
			)
			req.Parent = "rootInvocations/root-inv1/workUnits/wu1"

			t.Run(`Invalid mask`, func(t *ftt.Test) {
				req.ReadMask = &field_mask.FieldMask{Paths: []string{"artifact_id", "bad_path"}}
				_, err := srv.ListArtifacts(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`field "bad_path" does not exist in message Artifact`))
			})

			t.Run(`Exclude fetch url`, func(t *ftt.Test) {
				req.ReadMask = &field_mask.FieldMask{Paths: []string{"name", "content_type"}}
				actual, _ := mustFetch(req)
				assert.Loosely(t, actual, should.HaveLength(2))
				assert.Loosely(t, actual[0], should.Match(&pb.Artifact{
					Name:        "rootInvocations/root-inv1/workUnits/wu1/artifacts/a",
					ContentType: "text/plain",
				}))
				assert.Loosely(t, actual[1], should.Match(&pb.Artifact{
					Name: "rootInvocations/root-inv1/workUnits/wu1/artifacts/b",
				}))
			})

			t.Run(`Empty mask`, func(t *ftt.Test) {
				req.ReadMask = nil
				actual, _ := mustFetch(req)
				assert.Loosely(t, actual, should.HaveLength(2))

				expected0 := &pb.Artifact{
					Name:        "rootInvocations/root-inv1/workUnits/wu1/artifacts/a",
					ArtifactId:  "a",
					ContentType: "text/plain",
					HasLines:    true,
					SizeBytes:   100,
				}
				expected0.FetchUrl = actual[0].FetchUrl
				expected0.FetchUrlExpiration = actual[0].FetchUrlExpiration
				assert.Loosely(t, actual[0], should.Match(expected0))
				assert.Loosely(t, strings.HasPrefix(actual[0].FetchUrl, "https://signed-url.example.com/rootInvocations/root-inv1/workUnits/wu1/artifacts/a"), should.BeTrue)

				expected1 := &pb.Artifact{
					Name:       "rootInvocations/root-inv1/workUnits/wu1/artifacts/b",
					ArtifactId: "b",
					HasLines:   true,
					GcsUri:     "gs://bucket1/file1.txt",
				}
				expected1.FetchUrl = actual[1].FetchUrl
				expected1.FetchUrlExpiration = actual[1].FetchUrlExpiration
				assert.Loosely(t, actual[1], should.Match(expected1))
				assert.Loosely(t, actual[1].FetchUrl, should.Equal("https://fake-signed-url/bucket1/file1.txt?x-project=testproject"))
			})
		})

		t.Run(`legacy invocations`, func(t *ftt.Test) {
			req.Parent = "invocations/inv1"

			t.Run(`With both invocation and test result artifacts`, func(t *ftt.Test) {
				testutil.MustApply(ctx, t,
					insert.Artifact("inv1", "", "a", nil),
					insert.Artifact("inv1", "tr/t t/r", "a", nil),
				)

				t.Run(`Reads only invocation artifacts`, func(t *ftt.Test) {
					req.Parent = "invocations/inv1"
					arts, _ := mustFetch(req)
					assert.Loosely(t, artifactsNames(arts), should.Match([]string{
						"invocations/inv1/artifacts/a",
					}))
				})

				t.Run(`Reads only test result artifacts`, func(t *ftt.Test) {
					req.Parent = "invocations/inv1/tests/t%20t/results/r"
					arts, _ := mustFetch(req)
					assert.Loosely(t, artifactsNames(arts), should.Match([]string{
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

			t.Run(`Fetch URL with Rbe URI`, func(t *ftt.Test) {
				testutil.MustApply(ctx, t,
					insert.Artifact("inv1", "", "a", map[string]any{"RbeURI": "projects/testproject/instances/default_instance/blobs/test_artifact/10"}),
				)

				actual, _ := mustFetch(req)
				assert.Loosely(t, actual, should.HaveLength(1))
				assert.Loosely(t, strings.HasPrefix(actual[0].FetchUrl, "https://signed-url.example.com/invocations/inv1/artifacts/a"), should.BeTrue)
			})


		})
	})
}
