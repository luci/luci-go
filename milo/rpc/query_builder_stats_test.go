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

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/buildbucket/bbperms"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/milo/internal/model"
	"go.chromium.org/luci/milo/internal/model/milostatus"
	"go.chromium.org/luci/milo/internal/projectconfig"
	"go.chromium.org/luci/milo/internal/testutils"
	"go.chromium.org/luci/milo/internal/utils"
	milopb "go.chromium.org/luci/milo/proto/v1"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
)

func TestQueryBuilderStats(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestQueryBuilderStats`, t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = testutils.SetUpTestGlobalCache(ctx)

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

		createFakeBuild := func(builder *buildbucketpb.BuilderID, buildNum int, createdAt time.Time, status milostatus.Status) *model.BuildSummary {
			builderID := utils.LegacyBuilderIDString(builder)
			buildID := fmt.Sprintf("%s/%d", builderID, buildNum)
			return &model.BuildSummary{
				BuildKey:  datastore.MakeKey(ctx, "build", buildID),
				ProjectID: builder.Project,
				BuilderID: builderID,
				BuildID:   buildID,
				Summary: model.Summary{
					Status: status,
				},
				Created: createdAt,
			}
		}

		baseTime := time.Date(2020, 0, 0, 0, 0, 0, 0, time.UTC)
		builds := []*model.BuildSummary{
			createFakeBuild(builder1, 1, baseTime.AddDate(0, 0, -6), milostatus.Running),
			createFakeBuild(builder1, 2, baseTime.AddDate(0, 0, -5), milostatus.Running),
			createFakeBuild(builder1, 3, baseTime.AddDate(0, 0, -4), milostatus.Success),
			createFakeBuild(builder2, 4, baseTime.AddDate(0, 0, -3), milostatus.Running),
			createFakeBuild(builder1, 5, baseTime.AddDate(0, 0, -2), milostatus.Failure),
			createFakeBuild(builder2, 6, baseTime.AddDate(0, 0, -3), milostatus.NotRun),
			createFakeBuild(builder1, 7, baseTime.AddDate(0, 0, -1), milostatus.NotRun),
		}

		err := datastore.Put(ctx, builds)
		assert.Loosely(t, err, should.BeNil)

		err = datastore.Put(ctx, &projectconfig.Project{
			ID:      "fake_project",
			ACL:     projectconfig.ACL{Identities: []identity.Identity{"user"}},
			LogoURL: "https://logo.com",
		})
		assert.Loosely(t, err, should.BeNil)

		t.Run(`get build stats`, func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user",
				IdentityPermissions: []authtest.RealmPermission{
					{
						Realm:      "fake_project:fake_bucket",
						Permission: bbperms.BuildsList,
					},
				},
			})

			res, err := srv.QueryBuilderStats(ctx, &milopb.QueryBuilderStatsRequest{
				Builder: builder1,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.PendingBuildsCount, should.Equal(1))
			assert.Loosely(t, res.RunningBuildsCount, should.Equal(2))
		})

		t.Run(`reject users with no access`, func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user",
			})
			_, err := srv.QueryBuilderStats(ctx, &milopb.QueryBuilderStatsRequest{
				Builder: builder1,
			})
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}

func TestValidatesQueryBuilderStatsRequest(t *testing.T) {
	t.Parallel()
	ftt.Run("validatesQueryBuilderStatsRequest", t, func(t *ftt.Test) {
		t.Run("no builder", func(t *ftt.Test) {
			err := validatesQueryBuilderStatsRequest(&milopb.QueryBuilderStatsRequest{})
			assert.Loosely(t, err, should.ErrLike("builder: project must match"))
		})

		t.Run("invalid builder", func(t *ftt.Test) {
			err := validatesQueryBuilderStatsRequest(&milopb.QueryBuilderStatsRequest{
				Builder: &buildbucketpb.BuilderID{
					Project: "fake_proj",
					Bucket:  "fake[]ucket",
					Builder: "fake_/uilder1",
				},
			})
			assert.Loosely(t, err, should.ErrLike("builder: bucket must match"))
		})

		t.Run("valid", func(t *ftt.Test) {
			err := validatesQueryBuilderStatsRequest(&milopb.QueryBuilderStatsRequest{
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
