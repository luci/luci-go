// Copyright 2023 The LUCI Authors.
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

package importer

import (
	"context"
	"sort"
	"testing"

	"github.com/golang/mock/gomock"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/proto/gitiles/mock_gitiles"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"
	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/internal/model"
	"go.chromium.org/luci/config_service/internal/taskpb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestImportAllConfigs(t *testing.T) {
	t.Parallel()

	Convey("import all configs", t, func() {
		ctx := memory.UseWithAppID(context.Background(), "dev~app-id")
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, sch := tq.TestingContext(ctx, nil)

		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockClient := mock_gitiles.NewMockGitilesClient(ctl)
		ctx = context.WithValue(ctx, &clients.MockGitilesClientKey, mockClient)

		Convey("ok", func() {
			mockClient.EXPECT().ListFiles(gomock.Any(), proto.MatcherEqual(
				&gitilespb.ListFilesRequest{
					Project:    "infradata/config",
					Committish: "main",
					Path:       "dev-configs",
				},
			)).Return(&gitilespb.ListFilesResponse{
				Files: []*git.File{
					{
						Id:   "hash1",
						Path: "service1",
						Type: git.File_TREE,
					},
					{
						Id:   "hash2",
						Path: "service2",
						Type: git.File_TREE,
					},
					{
						Id:   "hash3",
						Path: "file1",
						Type: git.File_BLOB,
					},
				},
			}, nil)

			Convey("projects.cfg File entity not exist", func() {
				err := ImportAllConfigs(ctx)
				So(err, ShouldBeNil)
				cfgSetsInQueue := getCfgSetsInTaskQueue(sch)
				So(cfgSetsInQueue, ShouldResemble, []string{"services/service1", "services/service2"})
			})

			Convey("projects.cfg File entity exist", func() {
				cfgSetID := config.MustServiceSet(info.AppID(ctx))
				cs := &model.ConfigSet{
					ID:             cfgSetID,
					LatestRevision: model.RevisionInfo{ID: "rev"},
				}
				prjCfgBytes, err := prototext.Marshal(&cfgcommonpb.ProjectsCfg{
					Projects: []*cfgcommonpb.Project{
						{Id: "proj1"},
					},
				})
				So(err, ShouldBeNil)
				f := &model.File{
					Path:     "projects.cfg",
					Revision: datastore.MakeKey(ctx, model.ConfigSetKind, string(cfgSetID), model.RevisionKind, "rev"),
					Content:  prjCfgBytes,
				}
				So(datastore.Put(ctx, cs, f), ShouldBeNil)
				err = ImportAllConfigs(ctx)
				So(err, ShouldBeNil)
				cfgSetsInQueue := getCfgSetsInTaskQueue(sch)
				So(cfgSetsInQueue, ShouldResemble, []string{"projects/proj1", "services/service1", "services/service2"})
			})

			Convey("delete stale config set", func() {
				stale := &model.ConfigSet{ID: config.MustServiceSet("stale")}
				So(datastore.Put(ctx, stale), ShouldBeNil)
				err := ImportAllConfigs(ctx)
				So(err, ShouldBeNil)
				So(datastore.Get(ctx, stale), ShouldEqual, datastore.ErrNoSuchEntity)
			})
		})

		Convey("error", func() {
			Convey("gitiles", func() {
				mockClient.EXPECT().ListFiles(gomock.Any(), gomock.Any()).Return(nil, errors.New("gitiles error"))
				err := ImportAllConfigs(ctx)
				So(err, ShouldErrLike, "failed to load service config sets: failed to call Gitiles to list files: gitiles error")
			})

			Convey("bad projects.cfg content", func() {
				cfgSetID := config.MustServiceSet(info.AppID(ctx))
				mockClient.EXPECT().ListFiles(gomock.Any(), gomock.Any()).Return(&gitilespb.ListFilesResponse{}, nil)
				cs := &model.ConfigSet{
					ID:             cfgSetID,
					LatestRevision: model.RevisionInfo{ID: "rev"},
				}
				f := &model.File{
					Path:     "projects.cfg",
					Revision: datastore.MakeKey(ctx, model.ConfigSetKind, string(cfgSetID), model.RevisionKind, "rev"),
					Content:  []byte("bad"),
				}
				So(datastore.Put(ctx, cs, f), ShouldBeNil)
				err := ImportAllConfigs(ctx)
				So(err, ShouldErrLike, `failed to load project config sets: failed to unmarshal projects.cfg file content`)
			})
		})
	})
}

func getCfgSetsInTaskQueue(sch *tqtesting.Scheduler) []string {
	cfgSets := make([]string, len(sch.Tasks()))
	for i, task := range sch.Tasks() {
		cfgSets[i] = task.Payload.(*taskpb.ImportConfigs).ConfigSet
	}
	sort.Slice(cfgSets, func(i, j int) bool { return cfgSets[i] < cfgSets[j] })
	return cfgSets
}
