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

package prjcfg

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
	"go.chromium.org/luci/gae/filter/txndefer"
	gaememory "go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var testNow = testclock.TestTimeLocal.Round(1 * time.Millisecond)

func TestUpdateProject(t *testing.T) {
	Convey("Update Project", t, func() {
		ctx, testClock, _ := mkTestingCtx()
		chromiumConfig := &cfgpb.Config{
			DrainingStartTime: "2017-12-23T15:47:58Z",
			SubmitOptions: &cfgpb.SubmitOptions{
				MaxBurst:   100,
				BurstDelay: durationpb.New(1 * time.Second),
			},
			ConfigGroups: []*cfgpb.ConfigGroup{
				{
					Name: "branch_m100",
					Gerrit: []*cfgpb.ConfigGroup_Gerrit{
						{
							Url: "https://chromium-review.googlesource.com/",
							Projects: []*cfgpb.ConfigGroup_Gerrit_Project{
								{
									Name:      "chromium/src",
									RefRegexp: []string{"refs/heads/branch_m100"},
								},
							},
						},
					},
				},
				{
					Gerrit: []*cfgpb.ConfigGroup_Gerrit{
						{
							Url: "https://chromium-review.googlesource.com/",
							Projects: []*cfgpb.ConfigGroup_Gerrit_Project{
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
		verifyEntitiesInDatastore := func(ctx context.Context, expectedEVersion int64) {
			cfg, meta := &cfgpb.Config{}, &config.Meta{}
			err := cfgclient.Get(ctx, config.ProjectSet("chromium"), ConfigFileName, cfgclient.ProtoText(cfg), meta)
			So(err, ShouldBeNil)
			localHash := computeHash(cfg)
			projKey := datastore.MakeKey(ctx, projectConfigKind, "chromium")
			cgNames := make([]string, len(cfg.GetConfigGroups()))
			// Verify ConfigGroups.
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
			// Verify ProjectConfig.
			pc := ProjectConfig{Project: "chromium"}
			err = datastore.Get(ctx, &pc)
			So(err, ShouldBeNil)
			So(pc, ShouldResemble, ProjectConfig{
				Project:          "chromium",
				Enabled:          true,
				EVersion:         expectedEVersion,
				Hash:             localHash,
				ExternalHash:     meta.ContentHash,
				UpdateTime:       datastore.RoundTime(testClock.Now()).UTC(),
				ConfigGroupNames: cgNames,
			})
			// The revision in the memory-based config fake is a fake
			// 40-character sha256 hash digest. The particular value is
			// internally determined by the memory-based implementation
			// and isn't important here, so just assert that something
			// that looks like a hash digest is filled in.
			hashInfo := ConfigHashInfo{Hash: localHash, Project: projKey}
			err = datastore.Get(ctx, &hashInfo)
			So(err, ShouldBeNil)
			So(len(hashInfo.GitRevision), ShouldEqual, 40)
			hashInfo.GitRevision = ""
			// Verify the rest of ConfigHashInfo.
			So(hashInfo, ShouldResemble, ConfigHashInfo{
				Hash:             localHash,
				Project:          projKey,
				ProjectEVersion:  expectedEVersion,
				UpdateTime:       datastore.RoundTime(testClock.Now()).UTC(),
				ConfigGroupNames: cgNames,
			})
		}

		notifyCalled := false
		notify := func(context.Context) error {
			notifyCalled = true
			return nil
		}

		Convey("Creates new ProjectConfig", func() {
			ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{
				config.ProjectSet("chromium"): {
					ConfigFileName: toProtoText(chromiumConfig),
				},
			}))
			err := UpdateProject(ctx, "chromium", notify)
			So(err, ShouldBeNil)
			verifyEntitiesInDatastore(ctx, 1)
			So(notifyCalled, ShouldBeTrue)

			notifyCalled = false
			testClock.Add(10 * time.Minute)

			Convey("Noop if config is up-to-date", func() {
				err := UpdateProject(ctx, "chromium", notify)
				So(err, ShouldBeNil)
				pc := ProjectConfig{Project: "chromium"}
				err = datastore.Get(ctx, &pc)
				So(err, ShouldBeNil)
				So(pc.EVersion, ShouldEqual, 1)
				prevUpdatedTime := testClock.Now().Add(-10 * time.Minute)
				So(pc.UpdateTime, ShouldResemble, prevUpdatedTime.UTC())
				So(notifyCalled, ShouldBeFalse)
			})

			Convey("Update existing ProjectConfig", func() {
				updatedConfig := proto.Clone(chromiumConfig).(*cfgpb.Config)
				updatedConfig.ConfigGroups = append(updatedConfig.ConfigGroups, &cfgpb.ConfigGroup{
					Name: "experimental",
					Gerrit: []*cfgpb.ConfigGroup_Gerrit{
						{
							Url: "https://chromium-review.googlesource.com/",
							Projects: []*cfgpb.ConfigGroup_Gerrit_Project{
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
						ConfigFileName: toProtoText(updatedConfig),
					},
				}))
				err := UpdateProject(ctx, "chromium", notify)
				So(err, ShouldBeNil)
				verifyEntitiesInDatastore(ctx, 2)
				So(notifyCalled, ShouldBeTrue)

				notifyCalled = false
				testClock.Add(10 * time.Minute)

				Convey("Roll back to previous version", func() {
					ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{
						config.ProjectSet("chromium"): {
							ConfigFileName: toProtoText(chromiumConfig),
						},
					}))

					err := UpdateProject(ctx, "chromium", notify)
					So(err, ShouldBeNil)
					verifyEntitiesInDatastore(ctx, 3)
					So(notifyCalled, ShouldBeTrue)
				})

				Convey("Re-enables project even if config hash is the same", func() {
					testClock.Add(10 * time.Minute)
					So(DisableProject(ctx, "chromium", notify), ShouldBeNil)
					before := ProjectConfig{Project: "chromium"}
					So(datastore.Get(ctx, &before), ShouldBeNil)
					// Delete config entities.
					projKey := datastore.MakeKey(ctx, projectConfigKind, "chromium")
					err := datastore.Delete(ctx,
						&ConfigHashInfo{Hash: before.Hash, Project: projKey},
						&ConfigGroup{
							ID:      makeConfigGroupID(before.Hash, before.ConfigGroupNames[0], 0),
							Project: projKey,
						},
					)
					So(err, ShouldBeNil)

					testClock.Add(10 * time.Minute)
					So(UpdateProject(ctx, "chromium", notify), ShouldBeNil)
					after := ProjectConfig{Project: "chromium"}
					So(datastore.Get(ctx, &after), ShouldBeNil)

					So(after.Enabled, ShouldBeTrue)
					So(after.EVersion, ShouldEqual, before.EVersion+1)
					So(after.Hash, ShouldResemble, before.Hash)
					// Ensure deleted entities are re-created.
					verifyEntitiesInDatastore(ctx, 4)
					So(notifyCalled, ShouldBeTrue)
				})
			})
		})
	})
}

func TestDisableProject(t *testing.T) {
	Convey("Disable", t, func() {
		ctx, testClock, _ := mkTestingCtx()
		writeProjectConfig := func(enabled bool) {
			pc := ProjectConfig{
				Project:          "chromium",
				Enabled:          enabled,
				EVersion:         100,
				Hash:             "hash",
				ExternalHash:     "externalHash",
				UpdateTime:       datastore.RoundTime(testClock.Now()).UTC(),
				ConfigGroupNames: []string{"default"},
			}
			So(datastore.Put(ctx, &pc), ShouldBeNil)
			testClock.Add(10 * time.Minute)
		}

		notifyCalled := false
		notify := func(context.Context) error {
			notifyCalled = true
			return nil
		}

		Convey("currently enabled Project", func() {
			writeProjectConfig(true)
			err := DisableProject(ctx, "chromium", notify)
			So(err, ShouldBeNil)
			actual := ProjectConfig{Project: "chromium"}
			So(datastore.Get(ctx, &actual), ShouldBeNil)
			So(actual.Enabled, ShouldBeFalse)
			So(actual.EVersion, ShouldEqual, 101)
			So(actual.UpdateTime, ShouldResemble, datastore.RoundTime(testClock.Now()).UTC())
			So(notifyCalled, ShouldBeTrue)
		})

		Convey("currently disabled Project", func() {
			writeProjectConfig(false)
			err := DisableProject(ctx, "chromium", notify)
			So(err, ShouldBeNil)
			actual := ProjectConfig{Project: "chromium"}
			So(datastore.Get(ctx, &actual), ShouldBeNil)
			So(actual.Enabled, ShouldBeFalse)
			So(actual.EVersion, ShouldEqual, 100)
			So(notifyCalled, ShouldBeFalse)
		})

		Convey("non-existing Project", func() {
			err := DisableProject(ctx, "non-existing", notify)
			So(err, ShouldBeNil)
			So(datastore.Get(ctx, &ProjectConfig{Project: "non-existing"}), ShouldErrLike, datastore.ErrNoSuchEntity)
			So(notifyCalled, ShouldBeFalse)
		})
	})
}

func mkTestingCtx() (context.Context, testclock.TestClock, *tqtesting.Scheduler) {
	ctx, clock := testclock.UseTime(context.Background(), testNow)
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
