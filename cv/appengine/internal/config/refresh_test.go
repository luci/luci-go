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
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var TestNow = testclock.TestTimeUTC.Round(1 * time.Millisecond)

func TestSubmitRefreshTasks(t *testing.T) {
	t.Parallel()

	Convey("Submit Refresh Task", t, func() {
		ctx, _, tqScheduler := mkTestingCtx()

		Convey("New Project", func() {
			ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{
				config.ProjectSet("chromium"): {configFileName: ""},
			}))
			// Project chromium doesn't exist in datastore.
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
			// Project chromium doesn't exist in LUCI Config.
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
		ctx, testClock, _ := mkTestingCtx()
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

		Convey("Update config", func() {
			verify := func(ctx context.Context, expectedEVersion int64) {
				cfg, meta := &pb.Config{}, &config.Meta{}
				switch err := cfgclient.Get(ctx, config.ProjectSet("chromium"), configFileName, cfgclient.ProtoText(cfg), meta); {
				case err == nil:
					localHash := computeHash(cfg)
					projKey := datastore.MakeKey(ctx, projectConfigKind, "chromium")
					cgNames := make([]string, len(cfg.GetConfigGroups()))
					for i, cgpb := range cfg.GetConfigGroups() {
						cgNames[i] = makeConfigGroupName(cgpb.GetName(), i)
						cg := ConfigGroup{
							ID:      makeConfigGroupID(localHash, cgNames[i], i),
							Project: projKey,
						}
						err := datastore.Get(ctx, &cg)
						So(err, ShouldBeNil)
						So(cg.DrainingStartTime, ShouldEqual, cfg.GetDrainingStartTime())
						So(cg.SubmitOptions, ShouldResembleProto, cfg.GetSubmitOptions())
						So(cg.Content, ShouldResembleProto, cfg.GetConfigGroups()[i])
					}

					pc := ProjectConfig{Project: "chromium"}
					err := datastore.Get(ctx, &pc)
					So(err, ShouldBeNil)
					So(pc, ShouldResemble, ProjectConfig{
						Project:          "chromium",
						Enabled:          true,
						EVersion:         expectedEVersion,
						Hash:             localHash,
						ExternalHash:     meta.ContentHash,
						UpdateTime:      datastore.RoundTime(testClock.Now()),
						ConfigGroupNames: cgNames,
					})

					hashInfo := ConfigHashInfo{Hash: localHash, Project: projKey}
					err = datastore.Get(ctx, &hashInfo)
					So(err, ShouldBeNil)
					So(hashInfo, ShouldResemble, ConfigHashInfo{
						Hash:             localHash,
						Project:          projKey,
						ProjectEVersion:  expectedEVersion,
						UpdateTime:      datastore.RoundTime(testClock.Now()),
						ConfigGroupNames: cgNames,
					})

				case err == config.ErrNoConfig:
					pc := ProjectConfig{Project: "chromium"}
					err := datastore.Get(ctx, &pc)
					So(err, ShouldBeNil)
					So(pc.Enabled, ShouldBeFalse)
					So(pc.EVersion, ShouldEqual, expectedEVersion)
					So(pc.UpdateTime, ShouldResemble, datastore.RoundTime(testClock.Now()))

				default:
					So(err, ShouldBeNil)
				}
			}

			Convey("Create New ProjectConfig", func() {
				ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{
					config.ProjectSet("chromium"): {
						configFileName: toProtoText(chromiumConfig),
					},
				}))
				err := RefreshConfig(ctx, "chromium")
				So(err, ShouldBeNil)
				verify(ctx, 1)
				testClock.Add(10 * time.Minute)

				Convey("Noop if config unchanged", func() {
					err := RefreshConfig(ctx, "chromium")
					So(err, ShouldBeNil)
					pc := ProjectConfig{Project: "chromium"}
					err = datastore.Get(ctx, &pc)
					So(err, ShouldBeNil)
					So(pc.EVersion, ShouldEqual, 1)
					prevUpdatedTime := testClock.Now().Add(-10 * time.Minute)
					So(pc.UpdateTime, ShouldResemble, prevUpdatedTime)
				})

				Convey("Disable Project", func() {
					ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{}))
					err := RefreshConfig(ctx, "chromium")
					So(err, ShouldBeNil)
					verify(ctx, 2)
				})

				Convey("Update Existing ProjectConfig", func() {
					updatedConfig := proto.Clone(chromiumConfig).(*pb.Config)
					updatedConfig.ConfigGroups = append(updatedConfig.ConfigGroups, &pb.ConfigGroup{
						Name: "experimental",
						Gerrit: []*pb.ConfigGroup_Gerrit{
							{
								Url: "https://chromium-review.googlesource.com/",
								Projects: []*pb.ConfigGroup_Gerrit_Project{
									{
										Name:      "chromium/src/experimental",
										RefRegexp: []string{"refs/heads/main"},
									},
								},
							},
						},
					})
					ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{
						config.ProjectSet("chromium"): {
							configFileName: toProtoText(updatedConfig),
						},
					}))
					err := RefreshConfig(ctx, "chromium")
					So(err, ShouldBeNil)
					verify(ctx, 2)
					testClock.Add(10 * time.Minute)

					Convey("Rolled back to previous version", func() {
						ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{
							config.ProjectSet("chromium"): {
								configFileName: toProtoText(chromiumConfig),
							},
						}))

						err := RefreshConfig(ctx, "chromium")
						So(err, ShouldBeNil)
						verify(ctx, 3)
					})
				})
			})

			Convey("No-Op when disabling non-existent project", func() {
				ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{}))
				proj := ProjectConfig{Project: "non-existent-proj"}
				err := RefreshConfig(ctx, proj.Project)
				So(err, ShouldBeNil)
				So(datastore.IsErrNoSuchEntity(datastore.Get(ctx, &proj)), ShouldBeTrue)
			})
		})
	})
}

func mkTestingCtx() (context.Context, testclock.TestClock, *tqtesting.Scheduler) {
	ctx, clock := testclock.UseTime(context.Background(), TestNow)
	ctx = txndefer.FilterRDS(gaememory.Use(ctx))
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)

	ctx, scheduler := tq.TestingContext(ctx, nil)
	return ctx, clock, scheduler
}

func toProtoText(msg proto.Message) string {
	bs, err := prototext.Marshal(msg)
	So(err, ShouldBeNil)
	return string(bs)
}
