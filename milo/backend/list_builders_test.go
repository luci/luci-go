// Copyright 2021 The LUCI Authors.
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

package backend

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/buildbucket/access"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	milopb "go.chromium.org/luci/milo/api/service/v1"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/cachingtest"
)

func TestListBuilders(t *testing.T) {
	t.Parallel()
	Convey(`TestListBuilders`, t, func() {
		ctx := memory.Use(context.Background())

		caches := make(map[string]caching.BlobCache)
		ctx = caching.WithGlobalCache(ctx, func(namespace string) caching.BlobCache {
			cache, ok := caches[namespace]
			if !ok {
				cache = &cachingtest.BlobCache{LRU: lru.New(0)}
				caches[namespace] = cache
			}
			return cache
		})

		datastore.GetTestable(ctx).AddIndexes(&datastore.IndexDefinition{
			Kind: "BuildSummary",
			SortBy: []datastore.IndexColumn{
				{Property: "BuilderID"},
				{Property: "Created", Descending: true},
			},
		})
		datastore.GetTestable(ctx).Consistent(true)
		mockBuildersClient := buildbucketpb.NewMockBuildersClient(gomock.NewController(t))
		accessClient := access.TestClient{}
		srv := &MiloInternalService{
			GetBuildersClient: func(c context.Context, as auth.RPCAuthorityKind) (buildbucketpb.BuildersClient, error) {
				return mockBuildersClient, nil
			},
			GetCachedAccessClient: func(c context.Context) (*common.CachedAccessClient, error) {
				return common.NewTestCachedAccessClient(&accessClient, caching.GlobalCache(c, "TestListBuilders")), nil
			},
		}

		err := datastore.Put(ctx, []*common.Project{
			{
				ID:  "this project",
				ACL: common.ACL{Identities: []identity.Identity{"user"}},
				ExternalBuilderIDs: []string{
					"other project/bucket without.access/fake.builder 1",
					"other project/fake.bucket 2/fake.builder 1",
					"other project/fake.bucket 2/fake.builder 2",
				},
			},
			{
				ID: "other project",
			},
		})
		So(err, ShouldBeNil)

		err = datastore.Put(ctx, []*common.Console{
			{
				Parent: datastore.MakeKey(ctx, "Project", "this project"),
				ID:     "console1",
				Builders: []string{
					"buildbucket/luci.other project.fake.bucket 2/fake.builder 2",
					"buildbucket/luci.other project.bucket without.access/fake.builder 1",
					"buildbucket/luci.this project.fake.bucket 2/fake.builder 1",
					"buildbucket/luci.this project.fake.bucket 1/fake.builder 1",
				},
			},
			{
				Parent: datastore.MakeKey(ctx, "Project", "this project"),
				ID:     "console2",
				Builders: []string{
					"buildbucket/luci.other project.fake.bucket 2/fake.builder 1",
					"buildbucket/luci.this project.fake.bucket 2/fake.builder 1",
					"buildbucket/luci.this project.fake.bucket 1/fake.builder 2",
				},
			},
		})
		So(err, ShouldBeNil)

		Convey(`list project builders E2E`, func() {
			c := auth.WithState(ctx, &authtest.FakeState{Identity: "user"})

			// Mock the first buildbucket ListBuilders response.
			expectedReq := &buildbucketpb.ListBuildersRequest{
				Project:  "this project",
				PageSize: 2,
			}
			mockBuildersClient.EXPECT().ListBuilders(gomock.Any(), common.NewShouldResemberMatcher(expectedReq)).Return(&buildbucketpb.ListBuildersResponse{
				Builders: []*buildbucketpb.BuilderItem{
					{
						Id: &buildbucketpb.BuilderID{
							Project: "this project",
							Bucket:  "fake.bucket 1",
							Builder: "fake.builder 1",
						},
					},
					{
						Id: &buildbucketpb.BuilderID{
							Project: "this project",
							Bucket:  "fake.bucket 1",
							Builder: "fake.builder 2",
						},
					},
				},
				NextPageToken: "page 2",
			}, nil)

			// Test the first page.
			// It should return builders from the first page of the buildbucket.ListBuilders Response.
			res, err := srv.ListBuilders(c, &milopb.ListBuildersRequest{
				Project:  "this project",
				PageSize: 2,
			})
			So(err, ShouldBeNil)
			So(res.Builders, ShouldResemble, []*buildbucketpb.BuilderItem{
				{
					Id: &buildbucketpb.BuilderID{
						Project: "this project",
						Bucket:  "fake.bucket 1",
						Builder: "fake.builder 1",
					},
				},
				{
					Id: &buildbucketpb.BuilderID{
						Project: "this project",
						Bucket:  "fake.bucket 1",
						Builder: "fake.builder 2",
					},
				},
			})
			So(res.NextPageToken, ShouldNotBeEmpty)

			// Mock the second buildbucket ListBuilders response.
			expectedReq = &buildbucketpb.ListBuildersRequest{
				Project:   "this project",
				PageSize:  2,
				PageToken: "page 2",
			}
			mockBuildersClient.EXPECT().ListBuilders(gomock.Any(), common.NewShouldResemberMatcher(expectedReq)).Return(&buildbucketpb.ListBuildersResponse{
				Builders: []*buildbucketpb.BuilderItem{
					{
						Id: &buildbucketpb.BuilderID{
							Project: "this project",
							Bucket:  "fake.bucket 2",
							Builder: "fake.builder 1",
						},
					},
				},
			}, nil)

			// Mock the access client response.
			accessClient.PermittedActionsResponse = access.Permissions{
				"luci.other project.fake.bucket 2": access.AccessBucket,
			}.ToProto(time.Hour)

			// Test the second page.
			// It should return builders from the second page of the buildbucket.ListBuilders Response.
			// with accessable external builders filling the rest of the page.
			res, err = srv.ListBuilders(c, &milopb.ListBuildersRequest{
				Project:   "this project",
				PageSize:  2,
				PageToken: res.NextPageToken,
			})
			So(err, ShouldBeNil)
			So(res.Builders, ShouldResemble, []*buildbucketpb.BuilderItem{
				{
					Id: &buildbucketpb.BuilderID{
						Project: "this project",
						Bucket:  "fake.bucket 2",
						Builder: "fake.builder 1",
					},
				},
				{
					Id: &buildbucketpb.BuilderID{
						Project: "other project",
						Bucket:  "fake.bucket 2",
						Builder: "fake.builder 1",
					},
				},
			})
			So(res.NextPageToken, ShouldNotBeEmpty)

			// Test the third page.
			// It should return the remaining accessible external builders.
			res, err = srv.ListBuilders(c, &milopb.ListBuildersRequest{
				Project:   "this project",
				PageSize:  2,
				PageToken: res.NextPageToken,
			})
			So(err, ShouldBeNil)
			So(res.Builders, ShouldResemble, []*buildbucketpb.BuilderItem{
				{
					Id: &buildbucketpb.BuilderID{
						Project: "other project",
						Bucket:  "fake.bucket 2",
						Builder: "fake.builder 2",
					},
				},
			})
			So(res.NextPageToken, ShouldBeEmpty)
		})

		Convey(`list group builders E2E`, func() {
			c := auth.WithState(ctx, &authtest.FakeState{Identity: "user"})

			// Mock the access client response.
			accessClient.PermittedActionsResponse = access.Permissions{
				"luci.other project.fake.bucket 2": access.AccessBucket,
				"luci.this project.fake.bucket 1":  access.AccessBucket,
				"luci.this project.fake.bucket 2":  access.AccessBucket,
			}.ToProto(time.Hour)

			// Test the first page.
			// It should return accessible internal builders first.
			res, err := srv.ListBuilders(c, &milopb.ListBuildersRequest{
				Project:  "this project",
				Group:    "console1",
				PageSize: 2,
			})
			So(err, ShouldBeNil)
			So(res.Builders, ShouldResemble, []*buildbucketpb.BuilderItem{
				{
					Id: &buildbucketpb.BuilderID{
						Project: "this project",
						Bucket:  "fake.bucket 1",
						Builder: "fake.builder 1",
					},
				},
				{
					Id: &buildbucketpb.BuilderID{
						Project: "this project",
						Bucket:  "fake.bucket 2",
						Builder: "fake.builder 1",
					},
				},
			})
			So(res.NextPageToken, ShouldNotBeEmpty)

			// Test the second page.
			// It should return the remaining accessible external builders.
			res, err = srv.ListBuilders(c, &milopb.ListBuildersRequest{
				Project:   "this project",
				PageSize:  2,
				PageToken: res.NextPageToken,
			})
			So(err, ShouldBeNil)
			So(res.Builders, ShouldResemble, []*buildbucketpb.BuilderItem{
				{
					Id: &buildbucketpb.BuilderID{
						Project: "other project",
						Bucket:  "fake.bucket 2",
						Builder: "fake.builder 2",
					},
				},
			})
			So(res.NextPageToken, ShouldBeEmpty)
		})

		Convey(`reject users without access to the project`, func() {
			c := auth.WithState(ctx, &authtest.FakeState{Identity: "user2"})

			_, err := srv.ListBuilders(c, &milopb.ListBuildersRequest{
				Project:  "this project",
				Group:    "console1",
				PageSize: 2,
			})
			So(err, ShouldNotBeNil)
		})
	})
}
