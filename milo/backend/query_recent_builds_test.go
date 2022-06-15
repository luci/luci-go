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
	"fmt"
	"testing"
	"time"

	"github.com/google/tink/go/aead"
	"github.com/google/tink/go/keyset"
	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/buildbucket/bbperms"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	milopb "go.chromium.org/luci/milo/api/service/v1"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/milo/common/model/milostatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/secrets"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestQueryRecentBuilds(t *testing.T) {
	t.Parallel()
	Convey(`TestQueryRecentBuilds`, t, func() {
		ctx := memory.Use(context.Background())
		ctx = common.SetUpTestGlobalCache(ctx)

		kh, err := keyset.NewHandle(aead.AES256GCMKeyTemplate())
		So(err, ShouldBeNil)
		aead, err := aead.New(kh)
		So(err, ShouldBeNil)
		ctx = secrets.SetPrimaryTinkAEADForTest(ctx, aead)

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
			builderID := common.LegacyBuilderIDString(builder)
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

		err = datastore.Put(ctx, builds)
		So(err, ShouldBeNil)

		Convey(`get all recent builds`, func() {
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
			So(err, ShouldBeNil)
			So(res.Builds, ShouldResemble, []*buildbucketpb.Build{
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
			})
			So(res.NextPageToken, ShouldNotBeEmpty)

			res, err = srv.QueryRecentBuilds(ctx, &milopb.QueryRecentBuildsRequest{
				Builder:   builder1,
				PageSize:  2,
				PageToken: res.NextPageToken,
			})
			So(err, ShouldBeNil)
			So(res.Builds, ShouldResemble, []*buildbucketpb.Build{
				{
					Builder:    builder1,
					Number:     2,
					Status:     buildbucketpb.Status_SUCCESS,
					CreateTime: timestamppb.New(builds[1].Created),
				},
			})
			So(res.NextPageToken, ShouldBeEmpty)
		})

		Convey(`reject users with no access`, func() {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user",
			})
			_, err := srv.QueryRecentBuilds(ctx, &milopb.QueryRecentBuildsRequest{
				Builder:  builder1,
				PageSize: 2,
			})
			So(err, ShouldNotBeNil)
		})
	})
}
