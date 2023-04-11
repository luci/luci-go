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

package rpc

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/buildbucket/bbperms"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/internal/projectconfig"
	configpb "go.chromium.org/luci/milo/proto/config"
	milopb "go.chromium.org/luci/milo/proto/v1"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
)

func TestListBuilders(t *testing.T) {
	t.Parallel()
	Convey(`TestListBuilders`, t, func() {
		ctx := memory.Use(context.Background())
		ctx = caching.WithEmptyProcessCache(ctx)
		ctx = common.SetUpTestGlobalCache(ctx)

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user",
			IdentityPermissions: []authtest.RealmPermission{
				{
					Realm:      "this_project:fake.bucket_1",
					Permission: bbperms.BuildersList,
				},
				{
					Realm:      "this_project:fake.bucket_2",
					Permission: bbperms.BuildersList,
				},
				{
					Realm:      "other_project:fake.bucket_2",
					Permission: bbperms.BuildersList,
				},
			},
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
		srv := &MiloInternalService{
			GetSettings: func(c context.Context) (*configpb.Settings, error) {
				return &configpb.Settings{
					Buildbucket: &configpb.Settings_Buildbucket{
						Host: "buildbucket_host",
					},
				}, nil
			},
			GetBuildersClient: func(c context.Context, host string, as auth.RPCAuthorityKind) (buildbucketpb.BuildersClient, error) {
				return mockBuildersClient, nil
			},
		}

		err := datastore.Put(ctx, []*projectconfig.Project{
			{
				ID:  "this_project",
				ACL: projectconfig.ACL{Identities: []identity.Identity{"user"}},
				ExternalBuilderIDs: []string{
					"other_project/bucket_without.access/fake.builder 1",
					"other_project/fake.bucket_2/fake.builder 1",
					"other_project/fake.bucket_2/fake.builder 2",
				},
			},
			{
				ID: "other_project",
			},
		})
		So(err, ShouldBeNil)

		err = datastore.Put(ctx, []*projectconfig.Console{
			{
				Parent: datastore.MakeKey(ctx, "Project", "this_project"),
				ID:     "console1",
				Builders: []string{
					"buildbucket/luci.other_project.fake.bucket_2/fake.builder 2",
					"buildbucket/luci.other_project.bucket_without.access/fake.builder 1",
					"buildbucket/luci.this_project.fake.bucket_2/fake.builder 1",
					"buildbucket/luci.this_project.fake.bucket_1/fake.builder 1",
				},
			},
			{
				Parent: datastore.MakeKey(ctx, "Project", "this_project"),
				ID:     "console2",
				Builders: []string{
					"buildbucket/luci.other_project.fake.bucket_2/fake.builder 1",
					"buildbucket/luci.this_project.fake.bucket_2/fake.builder 1",
					"buildbucket/luci.this_project.fake.bucket_1/fake.builder 2",
				},
			},
		})
		So(err, ShouldBeNil)

		// Mock the first buildbucket ListBuilders response.
		expectedReq := &buildbucketpb.ListBuildersRequest{
			PageSize: 1000,
		}
		mockBuildersClient.
			EXPECT().
			ListBuilders(gomock.Any(), proto.MatcherEqual(expectedReq)).
			MaxTimes(1).
			Return(&buildbucketpb.ListBuildersResponse{
				Builders: []*buildbucketpb.BuilderItem{
					{
						Id: &buildbucketpb.BuilderID{
							Project: "other_project",
							Bucket:  "fake.bucket_2",
							Builder: "fake.builder 1",
						},
					},
					{
						Id: &buildbucketpb.BuilderID{
							Project: "this_project",
							Bucket:  "fake.bucket_1",
							Builder: "fake.builder 1",
						},
					},
					{
						Id: &buildbucketpb.BuilderID{
							Project: "this_project",
							Bucket:  "fake.bucket_1",
							Builder: "fake.builder 2",
						},
					},
				},
				NextPageToken: "page 2",
			}, nil)

		// Mock the second buildbucket ListBuilders response.
		expectedReq = &buildbucketpb.ListBuildersRequest{
			PageSize:  1000,
			PageToken: "page 2",
		}
		mockBuildersClient.
			EXPECT().
			ListBuilders(gomock.Any(), proto.MatcherEqual(expectedReq)).
			MaxTimes(1).
			Return(&buildbucketpb.ListBuildersResponse{
				Builders: []*buildbucketpb.BuilderItem{
					{
						Id: &buildbucketpb.BuilderID{
							Project: "this_project",
							Bucket:  "fake.bucket_2",
							Builder: "fake.builder 1",
						},
					},
					{
						Id: &buildbucketpb.BuilderID{
							Project: "this_project",
							Bucket:  "bucket_without.access",
							Builder: "fake.builder 1",
						},
					},
				},
			}, nil)

		Convey(`list all builders E2E`, func() {
			// Test the first page.
			// It should return builders from the first page of the buildbucket.ListBuilders Response.
			res, err := srv.ListBuilders(ctx, &milopb.ListBuildersRequest{
				PageSize: 3,
			})
			So(err, ShouldBeNil)
			So(res.Builders, ShouldResemble, []*buildbucketpb.BuilderItem{
				{
					Id: &buildbucketpb.BuilderID{
						Project: "other_project",
						Bucket:  "fake.bucket_2",
						Builder: "fake.builder 1",
					},
				},
				{
					Id: &buildbucketpb.BuilderID{
						Project: "this_project",
						Bucket:  "fake.bucket_1",
						Builder: "fake.builder 1",
					},
				},
				{
					Id: &buildbucketpb.BuilderID{
						Project: "this_project",
						Bucket:  "fake.bucket_1",
						Builder: "fake.builder 2",
					},
				},
			})
			So(res.NextPageToken, ShouldNotBeEmpty)

			// Test the second page.
			// It should return builders from the second page of the buildbucket.ListBuilders Response.
			// without returning anything from the external builders list defined in the consoles, since
			// they should be included in the list builders response already.
			res, err = srv.ListBuilders(ctx, &milopb.ListBuildersRequest{
				PageSize:  3,
				PageToken: res.NextPageToken,
			})
			So(err, ShouldBeNil)
			So(res.Builders, ShouldResemble, []*buildbucketpb.BuilderItem{
				{
					Id: &buildbucketpb.BuilderID{
						Project: "this_project",
						Bucket:  "fake.bucket_2",
						Builder: "fake.builder 1",
					},
				},
			})
			So(res.NextPageToken, ShouldBeEmpty)
		})

		Convey(`list project builders E2E`, func() {
			// Test the first page.
			// It should return builders from the first page of the buildbucket.ListBuilders Response.
			// With builders from other_project filtered out.
			res, err := srv.ListBuilders(ctx, &milopb.ListBuildersRequest{
				Project:  "this_project",
				PageSize: 2,
			})
			So(err, ShouldBeNil)
			So(res.Builders, ShouldResemble, []*buildbucketpb.BuilderItem{
				{
					Id: &buildbucketpb.BuilderID{
						Project: "this_project",
						Bucket:  "fake.bucket_1",
						Builder: "fake.builder 1",
					},
				},
				{
					Id: &buildbucketpb.BuilderID{
						Project: "this_project",
						Bucket:  "fake.bucket_1",
						Builder: "fake.builder 2",
					},
				},
			})
			So(res.NextPageToken, ShouldNotBeEmpty)

			// Test the second page.
			// It should return builders from the second page of the buildbucket.ListBuilders Response.
			// with accessable external builders filling the rest of the page.
			res, err = srv.ListBuilders(ctx, &milopb.ListBuildersRequest{
				Project:   "this_project",
				PageSize:  2,
				PageToken: res.NextPageToken,
			})
			So(err, ShouldBeNil)
			So(res.Builders, ShouldResemble, []*buildbucketpb.BuilderItem{
				{
					Id: &buildbucketpb.BuilderID{
						Project: "this_project",
						Bucket:  "fake.bucket_2",
						Builder: "fake.builder 1",
					},
				},
				{
					Id: &buildbucketpb.BuilderID{
						Project: "other_project",
						Bucket:  "fake.bucket_2",
						Builder: "fake.builder 1",
					},
				},
			})
			So(res.NextPageToken, ShouldNotBeEmpty)

			// Test the third page.
			// It should return the remaining accessible external builders.
			res, err = srv.ListBuilders(ctx, &milopb.ListBuildersRequest{
				Project:   "this_project",
				PageSize:  2,
				PageToken: res.NextPageToken,
			})
			So(err, ShouldBeNil)
			So(res.Builders, ShouldResemble, []*buildbucketpb.BuilderItem{
				{
					Id: &buildbucketpb.BuilderID{
						Project: "other_project",
						Bucket:  "fake.bucket_2",
						Builder: "fake.builder 2",
					},
				},
			})
			So(res.NextPageToken, ShouldBeEmpty)
		})

		Convey(`list group builders E2E`, func() {
			// Test the first page.
			// It should return accessible internal builders first.
			res, err := srv.ListBuilders(ctx, &milopb.ListBuildersRequest{
				Project:  "this_project",
				Group:    "console1",
				PageSize: 2,
			})
			So(err, ShouldBeNil)
			So(res.Builders, ShouldResemble, []*buildbucketpb.BuilderItem{
				{
					Id: &buildbucketpb.BuilderID{
						Project: "this_project",
						Bucket:  "fake.bucket_1",
						Builder: "fake.builder 1",
					},
				},
				{
					Id: &buildbucketpb.BuilderID{
						Project: "this_project",
						Bucket:  "fake.bucket_2",
						Builder: "fake.builder 1",
					},
				},
			})
			So(res.NextPageToken, ShouldNotBeEmpty)

			// Test the second page.
			// It should return the remaining accessible external builders.
			res, err = srv.ListBuilders(ctx, &milopb.ListBuildersRequest{
				Project:   "this_project",
				Group:     "console1",
				PageSize:  2,
				PageToken: res.NextPageToken,
			})
			So(err, ShouldBeNil)
			So(res.Builders, ShouldResemble, []*buildbucketpb.BuilderItem{
				{
					Id: &buildbucketpb.BuilderID{
						Project: "other_project",
						Bucket:  "fake.bucket_2",
						Builder: "fake.builder 2",
					},
				},
			})
			So(res.NextPageToken, ShouldBeEmpty)
		})

		Convey(`reject users without access to the project`, func() {
			c := auth.WithState(ctx, &authtest.FakeState{Identity: "user2"})

			_, err := srv.ListBuilders(c, &milopb.ListBuildersRequest{
				Project:  "this_project",
				Group:    "console1",
				PageSize: 2,
			})
			So(err, ShouldNotBeNil)
		})
	})
}
