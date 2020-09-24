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

package config

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmemory "go.chromium.org/luci/config/impl/memory"
	pb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/gae/filter/txndefer"
	gaememory "go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/types/known/durationpb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var TestNow = testclock.TestTimeUTC.Round(1 * time.Millisecond)

func TestSubmitRefreshTasks(t *testing.T) {
	t.Parallel()

	Convey("Submit Refresh Task", t, func() {
		ctx, tqScheduler := mkTestingCtx()

		Convey("New Project", func() {
			ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{
				config.ProjectSet("chromium"): {configFileName: ""},
			}))
			// project chromium doesn't exist in datastore
			err := SubmitRefreshTasks(ctx)
			So(err, ShouldBeNil)
			So(tqScheduler.Tasks().Payloads(), ShouldResembleProto, []*RefreshProjectConfigTask{
				{Project: "chromium"},
			})
		})

		Convey("Existing Project", func() {
			ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{
				config.ProjectSet("chromium"): {configFileName: ""},
			}))
			datastore.Put(ctx, &ProjectConfig{
				Project: "chromium",
				Enabled: true,
			})
			err := SubmitRefreshTasks(ctx)
			So(err, ShouldBeNil)
			So(tqScheduler.Tasks().Payloads(), ShouldResembleProto, []*RefreshProjectConfigTask{
				{Project: "chromium"},
			})
		})

		Convey("Skip project that doesn't have CV config", func() {
			ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{
				config.ProjectSet("chromium"): {"other.cfg": ""},
			}))
			err := SubmitRefreshTasks(ctx)
			So(err, ShouldBeNil)
			So(tqScheduler.Tasks(), ShouldBeEmpty)
		})

		Convey("De-registered Project", func() {
			ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{}))
			// project chromium doesn't exist in LUCI Config
			So(datastore.Put(ctx, &ProjectConfig{
				Project: "chromium",
				Enabled: true,
			}), ShouldBeNil)
			err := SubmitRefreshTasks(ctx)
			So(err, ShouldBeNil)
			So(tqScheduler.Tasks().Payloads(), ShouldResembleProto, []*RefreshProjectConfigTask{
				{Project: "chromium"},
			})
		})

		Convey("Skip disabled Project", func() {
			ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{}))
			datastore.Put(ctx, &ProjectConfig{
				Project: "foo",
				Enabled: false,
			})
			err := SubmitRefreshTasks(ctx)
			So(err, ShouldBeNil)
			So(tqScheduler.Tasks(), ShouldBeEmpty)
		})
	})
}

func TestRefreshConfig(t *testing.T) {
	Convey("RefreshConfig", t, func() {
		ctx, _ := mkTestingCtx()
		chromiumConfig := &pb.Config{
			DrainingStartTime: "2017-12-23T15:47:58Z",
			SubmitOptions: &pb.SubmitOptions{
				MaxBurst:   100,
				BurstDelay: durationpb.New(1 * time.Second),
			},
			ConfigGroups: []*pb.ConfigGroup{
				{
					Name: "branch_m100",
					Gerrit: []*pb.ConfigGroup_Gerrit{
						{
							Url: "https://chromium-review.googlesource.com/",
							Projects: []*pb.ConfigGroup_Gerrit_Project{
								{
									Name:      "chromium/src",
									RefRegexp: []string{"refs/heads/branch_m100"},
								},
							},
						},
					},
				},
				{
					Gerrit: []*pb.ConfigGroup_Gerrit{
						{
							Url: "https://chromium-review.googlesource.com/",
							Projects: []*pb.ConfigGroup_Gerrit_Project{
								{
									Name:      "chromium/src",
									RefRegexp: []string{"refs/heads/main"},
								},
							},
						},
					},
				},
			},
		}
		bs, err := prototext.Marshal(chromiumConfig)
		So(err, ShouldBeNil)
		chromiumConfigFileContent := string(bs)

		Convey("Update config", func() {
			ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{
				config.ProjectSet("chromium"): {
					configFileName: chromiumConfigFileContent,
				},
			}))
			var meta config.Meta
			So(cfgclient.Get(ctx, config.ProjectSet("chromium"), configFileName, cfgclient.ProtoText(&pb.Config{}), &meta), ShouldErrLike, nil)
			configHash := meta.ContentHash

			runAndVerify := func(existingPC *ProjectConfig) {
				if existingPC != nil {
					So(datastore.Put(ctx, existingPC), ShouldErrLike, nil)
				}

				So(RefreshConfig(ctx, "chromium"), ShouldErrLike, nil)
				pc := ProjectConfig{Project: "chromium"}
				So(datastore.Get(ctx, &pc), ShouldErrLike, nil)
				expectedVersion := int64(1)
				if existingPC != nil {
					expectedVersion = existingPC.Version + 1
				}
				So(pc, ShouldResemble, ProjectConfig{
					Project:     "chromium",
					Enabled:     true,
					ConfigHash:  configHash,
					Version:     expectedVersion,
					UpdatedTime: TestNow,
					ConfigGroupIDs: []ConfigGroupID{
						ConfigGroupID(fmt.Sprintf("%s/%s", configHash, "branch_m100")),
						ConfigGroupID(fmt.Sprintf("%s/%s", configHash, "index_1")),
					},
				})
				namedConfigGroup := ConfigGroup{
					ID:      ConfigGroupID(fmt.Sprintf("%s/%s", configHash, "branch_m100")),
					Project: datastore.MakeKey(ctx, projectConfigKind, "chromium"),
				}
				So(datastore.Get(ctx, &namedConfigGroup), ShouldErrLike, nil)
				So(namedConfigGroup.Version, ShouldEqual, expectedVersion)
				So(namedConfigGroup.CreateTime, ShouldResemble, TestNow)
				So(namedConfigGroup.DrainingStartTime, ShouldEqual, chromiumConfig.DrainingStartTime)
				So(&(namedConfigGroup.SubmitOptions), ShouldResembleProto, chromiumConfig.SubmitOptions)
				So(&(namedConfigGroup.Content), ShouldResembleProto, chromiumConfig.ConfigGroups[0])

				unnamedConfigGroup := ConfigGroup{
					ID:      ConfigGroupID(fmt.Sprintf("%s/%s", configHash, "index_1")),
					Project: datastore.MakeKey(ctx, projectConfigKind, "chromium"),
				}
				So(datastore.Get(ctx, &unnamedConfigGroup), ShouldErrLike, nil)
				So(unnamedConfigGroup.Version, ShouldEqual, expectedVersion)
				So(unnamedConfigGroup.CreateTime, ShouldResemble, TestNow)
				So(unnamedConfigGroup.DrainingStartTime, ShouldEqual, chromiumConfig.DrainingStartTime)
				So(&(unnamedConfigGroup.SubmitOptions), ShouldResembleProto, chromiumConfig.SubmitOptions)
				So(&(unnamedConfigGroup.Content), ShouldResembleProto, chromiumConfig.ConfigGroups[1])
			}

			Convey("New Project", func() {
				runAndVerify(nil)
			})

			Convey("Existing Project", func() {
				runAndVerify(&ProjectConfig{
					Project:        "chromium",
					Enabled:        true,
					ConfigHash:     "old_hash",
					Version:        123,
					UpdatedTime:    TestNow.AddDate(0, 0, -1),
					ConfigGroupIDs: []ConfigGroupID{"old_hash/default"},
				})
			})

			Convey("No-Op if existing config has the same hash", func() {
				existingProjectConfig := ProjectConfig{
					Project:     "chromium",
					Enabled:     true,
					ConfigHash:  configHash,
					Version:     123,
					UpdatedTime: TestNow.AddDate(0, 0, -1),
					ConfigGroupIDs: []ConfigGroupID{
						ConfigGroupID(fmt.Sprintf("%s/%s", configHash, "default")),
					},
				}
				So(datastore.Put(ctx, &existingProjectConfig), ShouldErrLike, nil)
				So(RefreshConfig(ctx, "chromium"), ShouldErrLike, nil)
				pc := ProjectConfig{Project: "chromium"}
				So(datastore.Get(ctx, &pc), ShouldErrLike, nil)
				So(pc, ShouldResemble, existingProjectConfig)
			})
		})

		Convey("Disable Project", func() {
			ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{}))
			pc := ProjectConfig{
				Project:        "chromium",
				Enabled:        true,
				ConfigHash:     "hash",
				Version:        123,
				UpdatedTime:    TestNow.AddDate(0, 0, -1),
				ConfigGroupIDs: []ConfigGroupID{"hash/default"},
			}
			So(datastore.Put(ctx, &pc), ShouldErrLike, nil)
			So(RefreshConfig(ctx, "chromium"), ShouldErrLike, nil)
			disabled := ProjectConfig{Project: "chromium"}
			So(datastore.Get(ctx, &disabled), ShouldErrLike, nil)
			So(disabled, ShouldResemble, ProjectConfig{
				Project:     "chromium",
				Enabled:     false,
				Version:     124,
				UpdatedTime: TestNow,
			})

			Convey("No-Op when disabling non-existent project", func() {
				proj := ProjectConfig{Project: "non-existent-proj"}
				So(RefreshConfig(ctx, proj.Project), ShouldErrLike, nil)
				So(datastore.IsErrNoSuchEntity(datastore.Get(ctx, &proj)), ShouldBeTrue)
			})
		})
	})
}

func mkTestingCtx() (context.Context, *tqtesting.Scheduler) {
	ctx := txndefer.FilterRDS(gaememory.Use(context.Background()))
	ctx, _ = testclock.UseTime(ctx, TestNow)
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)

	ctx, scheduler := tq.TestingContext(ctx, nil)
	return ctx, scheduler
}
