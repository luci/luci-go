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

	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/gsutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestValidateQueryArtifactsRequest(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestValidateQueryArtifactsRequest`, t, func(t *ftt.Test) {
		t.Run(`Valid`, func(t *ftt.Test) {
			err := validateQueryArtifactsRequest(&pb.QueryArtifactsRequest{
				Invocations: []string{"invocations/x"},
				PageSize:    50,
			})
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`Invalid invocation`, func(t *ftt.Test) {
			err := validateQueryArtifactsRequest(&pb.QueryArtifactsRequest{
				Invocations: []string{"x"},
			})
			assert.Loosely(t, err, should.ErrLike(`invocations: "x": does not match`))
		})

		t.Run(`Invalid test result predicate`, func(t *ftt.Test) {
			err := validateQueryArtifactsRequest(&pb.QueryArtifactsRequest{
				Invocations: []string{"x"},
				Predicate: &pb.ArtifactPredicate{
					TestResultPredicate: &pb.TestResultPredicate{TestIdRegexp: ")"},
				},
			})
			assert.Loosely(t, err, should.ErrLike(`predicate: test_id_regexp: error parsing regexp`))
		})
	})
}

func TestQueryArtifacts(t *testing.T) {
	ftt.Run(`QueryArtifacts`, t, func(t *ftt.Test) {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermListArtifacts},
				{Realm: "testproject2:testrealm2", Permission: rdbperms.PermListArtifacts},
			},
		})
		ctx, _ = tsmon.WithDummyInMemory(ctx)

		testutil.MustApply(
			ctx, t,
			insert.InvocationWithInclusions("inv1", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:testrealm"}, "inv2")...,
		)
		testutil.MustApply(
			ctx, t,
			insert.Invocation("inv2", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:testrealm"}),
			insert.Invocation("inv3", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject2:testrealm2"}),
			insert.Invocation("invx", pb.Invocation_ACTIVE, map[string]any{"Realm": "secretproject:testrealm"}),
		)

		req := &pb.QueryArtifactsRequest{
			Invocations: []string{"invocations/inv1"},
			PageSize:    100,
			Predicate: &pb.ArtifactPredicate{
				TestResultPredicate: &pb.TestResultPredicate{},
			},
		}

		srv := newTestResultDBService()

		mustFetch := func(t testing.TB, req *pb.QueryArtifactsRequest) (arts []*pb.Artifact, token string) {
			t.Helper()
			// Set up cloud storage test client.
			ctx = context.WithValue(ctx, &gsutil.MockedGSClientKey, &gsutil.MockClient{})

			res, err := srv.QueryArtifacts(ctx, req)
			assert.Loosely(t, err, should.BeNil, truth.LineContext())
			return res.Artifacts, res.NextPageToken
		}

		mustFetchNames := func(t testing.TB, req *pb.QueryArtifactsRequest) []string {
			t.Helper()
			arts, _ := mustFetch(t, req)
			names := make([]string, len(arts))
			for i, a := range arts {
				names[i] = a.Name
			}
			return names
		}

		t.Run(`Permission denied`, func(t *ftt.Test) {
			req.Invocations = []string{"invocations/invx"}
			_, err := srv.QueryArtifacts(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("caller does not have permission resultdb.artifacts.list in realm of invocation invx"))
		})

		t.Run(`Read mask`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Artifact("inv1", "tr/t t/r", "aa", map[string]any{"ContentType": "text/plain", "Size": 100}),
			)
			t.Run(`Invalid mask`, func(t *ftt.Test) {
				_, err := srv.QueryArtifacts(ctx, &pb.QueryArtifactsRequest{
					Invocations: []string{"invocations/inv1"},
					ReadMask:    &field_mask.FieldMask{Paths: []string{"artifact_id", "bad_path"}},
				})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`field "bad_path" does not exist in message Artifact`))
			})

			t.Run(`Exclude fetch url`, func(t *ftt.Test) {
				actual, _ := mustFetch(t, &pb.QueryArtifactsRequest{
					Invocations: []string{"invocations/inv1"},
					ReadMask:    &field_mask.FieldMask{Paths: []string{"name", "content_type"}},
				})
				assert.Loosely(t, actual, should.HaveLength(1))
				assert.Loosely(t, actual[0], should.Match(&pb.Artifact{
					Name:        "invocations/inv1/tests/t%20t/results/r/artifacts/aa",
					ContentType: "text/plain",
				}))
			})

			t.Run(`Can pagination with read mask`, func(t *ftt.Test) {
				testutil.MustApply(ctx, t,
					insert.Artifact("inv1", "tr/t t/r", "aa1", map[string]any{"ContentType": "text/plain", "Size": 100}),
				)
				actual, token := mustFetch(t, &pb.QueryArtifactsRequest{
					Invocations: []string{"invocations/inv1"},
					ReadMask:    &field_mask.FieldMask{Paths: []string{"artifact_id"}},
					PageSize:    1,
				})
				assert.Loosely(t, actual, should.HaveLength(1))
				assert.Loosely(t, actual[0], should.Match(&pb.Artifact{
					Name:       "invocations/inv1/tests/t%20t/results/r/artifacts/aa",
					ArtifactId: "aa",
				}))
				assert.Loosely(t, token, should.NotBeEmpty)

				actual, _ = mustFetch(t, &pb.QueryArtifactsRequest{
					Invocations: []string{"invocations/inv1"},
					ReadMask:    &field_mask.FieldMask{Paths: []string{"artifact_id"}},
					PageSize:    1,
					PageToken:   token,
				})
				assert.Loosely(t, actual, should.HaveLength(1))
				assert.Loosely(t, actual[0], should.Match(&pb.Artifact{
					Name:       "invocations/inv1/tests/t%20t/results/r/artifacts/aa1",
					ArtifactId: "aa1",
				}))
			})

			t.Run(`Empty mask`, func(t *ftt.Test) {
				actual, _ := mustFetch(t, &pb.QueryArtifactsRequest{
					Invocations: []string{"invocations/inv1"},
				})

				assert.Loosely(t, actual, should.HaveLength(1))
				expected := &pb.Artifact{
					Name:        "invocations/inv1/tests/t%20t/results/r/artifacts/aa",
					ArtifactId:  "aa",
					ContentType: "text/plain",
					HasLines:    true,
					SizeBytes:   100,
				}
				expected.FetchUrl = actual[0].FetchUrl
				expected.FetchUrlExpiration = actual[0].FetchUrlExpiration
				assert.Loosely(t, actual[0], should.Match(expected))
				assert.Loosely(t, strings.HasPrefix(actual[0].FetchUrl, "https://signed-url.example.com/invocations/inv1/tests/t%20t/results/r/artifacts/aa"), should.BeTrue)
			})
		})

		t.Run(`ArtifactId filter works`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Artifact("inv1", "", "a", nil),
				insert.Artifact("inv1", "tr/t t/r", "aa", nil),
				insert.Artifact("inv2", "", "baa", nil),
				insert.Artifact("inv2", "tr/t t/r", "aaa", nil),
			)

			req.Predicate.ArtifactIdRegexp = "a+"

			actual := mustFetchNames(t, req)
			assert.Loosely(t, actual, should.Match([]string{
				"invocations/inv1/artifacts/a",
				"invocations/inv1/tests/t%20t/results/r/artifacts/aa",
				"invocations/inv2/tests/t%20t/results/r/artifacts/aaa",
			}))
		})

		t.Run(`Reads test result artifacts by invocation with included invocation`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Artifact("inv1", "", "a", nil),
				insert.Artifact("inv1", "tr/t t/r", "a", nil),
				insert.Artifact("inv2", "", "b", nil),
				insert.Artifact("inv2", "tr/t t/r", "b", nil),
			)
			actual := mustFetchNames(t, req)
			assert.Loosely(t, actual, should.Match([]string{
				"invocations/inv1/artifacts/a",
				"invocations/inv1/tests/t%20t/results/r/artifacts/a",
				"invocations/inv2/artifacts/b",
				"invocations/inv2/tests/t%20t/results/r/artifacts/b",
			}))
		})

		t.Run(`Reads test result artifacts by invocation without included invocation`, func(t *ftt.Test) {
			req.Invocations = []string{"invocations/inv2"}
			testutil.MustApply(ctx, t,
				insert.Artifact("inv2", "", "b", nil),
				insert.Artifact("inv2", "tr/t t/r", "b", nil),
			)
			actual := mustFetchNames(t, req)
			assert.Loosely(t, actual, should.Match([]string{
				"invocations/inv2/artifacts/b",
				"invocations/inv2/tests/t%20t/results/r/artifacts/b",
			}))
		})

		t.Run(`Reads test result artifacts by invocation with missing included invocation`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				// The invocation missinginv is missing in Invocations table.
				insert.Inclusion("inv1", "missinginv"),
				insert.Artifact("inv1", "", "a", nil),
				insert.Artifact("inv1", "tr/t t/r", "a", nil),
			)
			req.Invocations = []string{"invocations/inv1"}
			actual := mustFetchNames(t, req)
			assert.Loosely(t, actual, should.Match([]string{
				"invocations/inv1/artifacts/a",
				"invocations/inv1/tests/t%20t/results/r/artifacts/a",
			}))
		})

		t.Run(`Fetch URL`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Artifact("inv1", "", "a", nil),
			)
			actual, _ := mustFetch(t, req)
			assert.Loosely(t, actual, should.HaveLength(1))
			assert.Loosely(t, strings.HasPrefix(actual[0].FetchUrl, "https://signed-url.example.com/invocations/inv1/artifacts/a"), should.BeTrue)
		})

		t.Run(`Fetch URL with GcsURI`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Artifact("inv1", "", "a", map[string]any{"GcsURI": "gs://bucket1/file1.txt"}),
			)

			actual, _ := mustFetch(t, req)
			assert.Loosely(t, actual, should.HaveLength(1))
			assert.Loosely(t, actual[0].FetchUrl, should.Equal("https://fake-signed-url/bucket1/file1.txt?x-project=testproject"))
		})

		t.Run(`Fetch URL wtih GCSURI from multiple projects`, func(t *ftt.Test) {
			// This test makes sure the correct project-scoped service account is used for generating signed URL.
			testutil.MustApply(ctx, t,
				// Include inv3 into inv1.
				insert.Inclusion("inv1", "inv3"),
				insert.Artifact("inv1", "", "a", map[string]any{"GcsURI": "gs://bucket1/file1.txt"}),
				insert.Artifact("inv3", "", "a", map[string]any{"GcsURI": "gs://bucket2/file2.txt"}),
				insert.Artifact("inv2", "tr/test/resultid", "a", map[string]any{"GcsURI": "gs://bucket1/testfile2.txt"}),
			)

			actual, _ := mustFetch(t, req)
			assert.Loosely(t, actual, should.HaveLength(3))
			assert.Loosely(t, actual[0].FetchUrl, should.Equal("https://fake-signed-url/bucket2/file2.txt?x-project=testproject2"))
			assert.Loosely(t, actual[1].FetchUrl, should.Equal("https://fake-signed-url/bucket1/file1.txt?x-project=testproject"))
			assert.Loosely(t, actual[2].FetchUrl, should.Equal("https://fake-signed-url/bucket1/testfile2.txt?x-project=testproject"))
		})
	})
}
