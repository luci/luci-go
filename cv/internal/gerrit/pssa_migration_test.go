// Copyright 2022 The LUCI Authors.
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

package gerrit

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/authtest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestPSSAMigrationClient(t *testing.T) {
	t.Parallel()

	Convey("PSSAMigrationClient", t, func() {
		ctx := memory.Use(context.Background())
		ctx, tclock := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = authtest.MockAuthConfig(ctx)
		const migratedProject = "lProject"
		So(projectsToMigrate.Has(migratedProject), ShouldBeFalse)
		notMigratedProject, ok := projectsToMigrate.Peek()
		So(ok, ShouldBeTrue)
		So(notMigratedProject, ShouldNotBeEmpty)
		var httpRequests []*http.Request
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// This test calls ListChanges RPC because it is the easiest to mock empty
			// response for.
			httpRequests = append(httpRequests, r)
			_, _ = w.Write([]byte(")]}'\n[]")) // no changes.
		}))
		defer srv.Close()
		u, err := url.Parse(srv.URL)
		So(err, ShouldBeNil)
		gHost := u.Host
		So(datastore.Put(ctx, &netrcToken{gHost, "legacy-tok"}), ShouldBeNil)
		factory := pssaMigrationFactory{
			passClientfactory: fakeFactory{},
			baseTransport:     srv.Client().Transport,
			legacyTokenCache:  lru.New(10),
		}

		Convey("For migrated project", func() {
			Convey("Work", func() {
				client, err := factory.MakeClient(ctx, gHost, migratedProject)
				So(err, ShouldBeNil)
				So(client.(pssaMigrationClient).legacyClient, ShouldBeNil)
				res, err := client.ListChanges(ctx, &gerritpb.ListChangesRequest{})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &gerritpb.ListChangesResponse{})
				So(httpRequests, ShouldBeEmpty)
			})

			Convey("Error is passed through", func() {
				factory.passClientfactory = fakeFactory{returnErr: errors.New("bad request")}
				client, err := factory.MakeClient(ctx, gHost, migratedProject)
				So(err, ShouldBeNil)
				_, err = client.ListChanges(ctx, &gerritpb.ListChangesRequest{})
				So(err, ShouldErrLike, "bad request")
			})
		})

		Convey("For not migrated project", func() {
			Convey("Request goes through using pssa", func() {
				client, err := factory.MakeClient(ctx, gHost, notMigratedProject)
				So(err, ShouldBeNil)
				res, err := client.ListChanges(ctx, &gerritpb.ListChangesRequest{})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &gerritpb.ListChangesResponse{})
				So(httpRequests, ShouldBeEmpty)
			})

			Convey("Fallback to legacy netrc token", func() {
				Convey("When pssa client returns empty project token error", func() {
					factory.passClientfactory = fakeFactory{
						returnErr: errors.Annotate(errEmptyProjectToken, "project %q", notMigratedProject).Err(),
					}
				})
				Convey("When pssa client returns permission denied", func() {
					factory.passClientfactory = fakeFactory{
						returnErr: status.Errorf(codes.PermissionDenied, "no access"),
					}
				})
				client, err := factory.MakeClient(ctx, gHost, notMigratedProject)
				So(err, ShouldBeNil)
				res, err := client.ListChanges(ctx, &gerritpb.ListChangesRequest{})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &gerritpb.ListChangesResponse{})
				So(httpRequests, ShouldHaveLength, 1)
				tokenB64 := base64.StdEncoding.EncodeToString([]byte("legacy-tok"))
				So(httpRequests[0].Header["Authorization"], ShouldResemble, []string{"Basic " + tokenB64})
			})

			Convey("Error when missing netrc token", func() {
				So(datastore.Delete(ctx, &netrcToken{gHost, "legacy-tok"}), ShouldBeNil)
				factory.passClientfactory = fakeFactory{
					returnErr: errors.Annotate(errEmptyProjectToken, "project %q", notMigratedProject).Err(),
				}
				client, err := factory.MakeClient(ctx, gHost, notMigratedProject)
				So(err, ShouldBeNil)
				_, err = client.ListChanges(ctx, &gerritpb.ListChangesRequest{})
				So(err, ShouldErrLike, "No legacy credentials for host", gHost)
			})

			Convey("Pass through other errors", func() {
				factory.passClientfactory = fakeFactory{
					returnErr: status.Errorf(codes.InvalidArgument, "invalid query"),
				}
				client, err := factory.MakeClient(ctx, gHost, notMigratedProject)
				So(err, ShouldBeNil)
				_, err = client.ListChanges(ctx, &gerritpb.ListChangesRequest{})
				So(err, ShouldErrLike, "invalid query")
				So(httpRequests, ShouldBeEmpty)
			})

			Convey("Cache token for 10 minutes", func() {
				factory.passClientfactory = fakeFactory{
					returnErr: errors.Annotate(errEmptyProjectToken, "project %q", notMigratedProject).Err(),
				}
				client, err := factory.MakeClient(ctx, gHost, notMigratedProject)
				So(err, ShouldBeNil)
				_, err = client.ListChanges(ctx, &gerritpb.ListChangesRequest{})
				So(err, ShouldBeNil)
				tclock.Add(10*time.Minute + 1*time.Second)
				So(datastore.Put(ctx, &netrcToken{gHost, "legacy-tok-2"}), ShouldBeNil)
				_, err = client.ListChanges(ctx, &gerritpb.ListChangesRequest{})
				So(err, ShouldBeNil)
				So(httpRequests, ShouldHaveLength, 2)
				So(httpRequests[0].Header["Authorization"], ShouldResemble, []string{"Basic " + base64.StdEncoding.EncodeToString([]byte("legacy-tok"))})
				So(httpRequests[1].Header["Authorization"], ShouldResemble, []string{"Basic " + base64.StdEncoding.EncodeToString([]byte("legacy-tok-2"))})
			})
		})
	})
}

type fakeFactory struct {
	returnErr error
}

func (f fakeFactory) MakeClient(ctx context.Context, gerritHost, luciProject string) (Client, error) {
	return fakePSSAClient{err: f.returnErr}, nil
}
func (f fakeFactory) MakeMirrorIterator(ctx context.Context) *MirrorIterator {
	return nil
}

type fakePSSAClient struct {
	err error
}

func (f fakePSSAClient) ListChanges(ctx context.Context, in *gerritpb.ListChangesRequest, opts ...grpc.CallOption) (*gerritpb.ListChangesResponse, error) {
	// ListChanges is the only rpc called.
	if f.err != nil {
		return nil, f.err
	}
	return &gerritpb.ListChangesResponse{}, nil
}

func (f fakePSSAClient) GetChange(ctx context.Context, in *gerritpb.GetChangeRequest, opts ...grpc.CallOption) (*gerritpb.ChangeInfo, error) {
	panic(fmt.Errorf("not implemented"))
}

func (f fakePSSAClient) GetRelatedChanges(ctx context.Context, in *gerritpb.GetRelatedChangesRequest, opts ...grpc.CallOption) (*gerritpb.GetRelatedChangesResponse, error) {
	panic(fmt.Errorf("not implemented"))
}

func (f fakePSSAClient) ListFiles(ctx context.Context, in *gerritpb.ListFilesRequest, opts ...grpc.CallOption) (*gerritpb.ListFilesResponse, error) {
	panic(fmt.Errorf("not implemented"))
}

func (f fakePSSAClient) SetReview(ctx context.Context, in *gerritpb.SetReviewRequest, opts ...grpc.CallOption) (*gerritpb.ReviewResult, error) {
	panic(fmt.Errorf("not implemented"))
}

func (f fakePSSAClient) SubmitRevision(ctx context.Context, in *gerritpb.SubmitRevisionRequest, opts ...grpc.CallOption) (*gerritpb.SubmitInfo, error) {
	panic(fmt.Errorf("not implemented"))
}
