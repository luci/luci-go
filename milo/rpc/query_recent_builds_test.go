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
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/buildbucket/bbperms"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/milo/internal/model"
	"go.chromium.org/luci/milo/internal/model/milostatus"
	"go.chromium.org/luci/milo/internal/testutils"
	"go.chromium.org/luci/milo/internal/utils"
	milopb "go.chromium.org/luci/milo/proto/v1"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/secrets"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestQueryRecentBuilds(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestQueryRecentBuilds`, t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = testutils.SetUpTestGlobalCache(ctx)
		ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)

		datastore.GetTestable(ctx).AddIndexes(&datastore.IndexDefinition{
			Kind: "BuildSummary",
			SortBy: []datastore.IndexColumn{
				{Property: "BuilderID"},
				{Property: "Created", Descending: true},
			},
		})
		datastore.GetTestable(ctx).Consistent(true)

		srv := &MiloInternalService{}

		builder1 := &buildbucketpb.BuilderID{
			Project: "fake_project",
			Bucket:  "fake_bucket",
			Builder: "fake_builder1",
		}
		builder2 := &buildbucketpb.BuilderID{
			Project: "fake_project",
			Bucket:  "fake_bucket",
			Builder: "fake_builder2",
		}

		createFakeBuild := func(builder *buildbucketpb.BuilderID, buildNum int, buildID int64, createdAt time.Time, status milostatus.Status) *model.BuildSummary {
			builderID := utils.LegacyBuilderIDString(builder)
			bsBuildID := fmt.Sprintf("%s/%d", builderID, buildNum)
			if buildID != 0 {
				bsBuildID = fmt.Sprintf("buildbucket/%d", buildID)
			}
			return &model.BuildSummary{
				BuildKey:  datastore.MakeKey(ctx, "buildbucket.Build", bsBuildID),
				ProjectID: builder.Project,
				BuilderID: builderID,
				BuildID:   bsBuildID,
				Summary: model.Summary{
					Status: status,
				},
				Created: createdAt,
			}
		}

		baseTime := time.Date(2020, 0, 0, 0, 0, 0, 0, time.UTC)
		builds := []*model.BuildSummary{
			createFakeBuild(builder1, 1, 0, baseTime.AddDate(0, 0, -5), milostatus.Running),
			createFakeBuild(builder1, 2, 0, baseTime.AddDate(0, 0, -4), milostatus.Success),
			createFakeBuild(builder2, 1, 0, baseTime.AddDate(0, 0, -3), milostatus.Success),
			createFakeBuild(builder1, 0, 999, baseTime.AddDate(0, 0, -2), milostatus.Failure),
			createFakeBuild(builder1, 0, 998, baseTime.AddDate(0, 0, -1), milostatus.InfraFailure),
		}

		err := datastore.Put(ctx, builds)
		assert.Loosely(t, err, should.BeNil)

		t.Run(`get all recent builds`, func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user",
				IdentityPermissions: []authtest.RealmPermission{
					{
						Realm:      "fake_project:fake_bucket",
						Permission: bbperms.BuildsList,
					},
				},
			})

			res, err := srv.QueryRecentBuilds(ctx, &milopb.QueryRecentBuildsRequest{
				Builder:  builder1,
				PageSize: 2,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Builds, should.Resemble([]*buildbucketpb.Build{
				{
					Builder:    builder1,
					Id:         998,
					Status:     buildbucketpb.Status_INFRA_FAILURE,
					CreateTime: timestamppb.New(builds[4].Created),
				},
				{
					Builder:    builder1,
					Id:         999,
					Status:     buildbucketpb.Status_FAILURE,
					CreateTime: timestamppb.New(builds[3].Created),
				},
			}))
			assert.Loosely(t, res.NextPageToken, should.NotBeEmpty)

			res, err = srv.QueryRecentBuilds(ctx, &milopb.QueryRecentBuildsRequest{
				Builder:   builder1,
				PageSize:  2,
				PageToken: res.NextPageToken,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Builds, should.Resemble([]*buildbucketpb.Build{
				{
					Builder:    builder1,
					Number:     2,
					Status:     buildbucketpb.Status_SUCCESS,
					CreateTime: timestamppb.New(builds[1].Created),
				},
			}))
			assert.Loosely(t, res.NextPageToken, should.BeEmpty)
		})

		t.Run(`reject users with no access`, func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user",
			})
			_, err := srv.QueryRecentBuilds(ctx, &milopb.QueryRecentBuildsRequest{
				Builder:  builder1,
				PageSize: 2,
			})
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}

func TestValidatesQueryRecentBuildsRequest(t *testing.T) {
	t.Parallel()
	ftt.Run("validatesQueryRecentBuildsRequest", t, func(t *ftt.Test) {
		t.Run("negative page size builder", func(t *ftt.Test) {
			err := validatesQueryRecentBuildsRequest(&milopb.QueryRecentBuildsRequest{
				Builder: &buildbucketpb.BuilderID{
					Project: "fake_project",
					Bucket:  "fake_bucket",
					Builder: "fake_builder1",
				},
				PageSize: -10,
			})
			assert.Loosely(t, err, should.ErrLike("page_size can not be negative"))
		})

		t.Run("no builder", func(t *ftt.Test) {
			err := validatesQueryRecentBuildsRequest(&milopb.QueryRecentBuildsRequest{})
			assert.Loosely(t, err, should.ErrLike("builder: project must match"))
		})

		t.Run("invalid builder", func(t *ftt.Test) {
			err := validatesQueryRecentBuildsRequest(&milopb.QueryRecentBuildsRequest{
				Builder: &buildbucketpb.BuilderID{
					Project: "fake_proj",
					Bucket:  "fake[]ucket",
					Builder: "fake_/uilder1",
				},
			})
			assert.Loosely(t, err, should.ErrLike("builder: bucket must match"))
		})

		t.Run("valid", func(t *ftt.Test) {
			err := validatesQueryRecentBuildsRequest(&milopb.QueryRecentBuildsRequest{
				Builder: &buildbucketpb.BuilderID{
					Project: "fake_project",
					Bucket:  "fake_bucket",
					Builder: "fake_builder1",
				},
			})
			assert.Loosely(t, err, should.BeNil)
		})
	})
}
