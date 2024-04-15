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

package refresher

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmemory "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/filter/txndefer"
	gaememory "go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/configs/srvcfg"
	listenerpb "go.chromium.org/luci/cv/settings/listener"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var testNow = testclock.TestTimeLocal.Round(1 * time.Millisecond)
var testCfg = &cfgpb.Config{
	DrainingStartTime: "2014-05-11T14:37:57Z",
	SubmitOptions: &cfgpb.SubmitOptions{
		MaxBurst:   50,
		BurstDelay: durationpb.New(2 * time.Second),
	},
	CqStatusHost: "chromium-cq-status.appspot.com",
	ConfigGroups: []*cfgpb.ConfigGroup{
		{
			Name: "group_foo",
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
	HonorGerritLinkedAccounts: true,
}

func TestUpdateProject(t *testing.T) {
	Convey("Update Project", t, func() {
		ctx, testClock, _ := mkTestingCtx()
		chromiumConfig := &cfgpb.Config{
			DrainingStartTime: "2017-12-23T15:47:58Z",
			CqStatusHost:      "chromium-cq-status.appspot.com",
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
					Name: "main",
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
			HonorGerritLinkedAccounts: true,
		}
		verifyEntitiesInDatastore := func(ctx context.Context, expectedEVersion int64) {
			cfg, meta := &cfgpb.Config{}, &config.Meta{}
			err := cfgclient.Get(ctx, config.MustProjectSet("chromium"), ConfigFileName, cfgclient.ProtoText(cfg), meta)
			So(err, ShouldBeNil)
			localHash := prjcfg.ComputeHash(cfg)
			projKey := prjcfg.ProjectConfigKey(ctx, "chromium")
			cgNames := make([]string, len(cfg.GetConfigGroups()))
			// Verify ConfigGroups.
			for i, cgpb := range cfg.GetConfigGroups() {
				cgNames[i] = cgpb.GetName()
				cg := prjcfg.ConfigGroup{
					ID:      prjcfg.MakeConfigGroupID(localHash, cgNames[i]),
					Project: projKey,
				}
				err := datastore.Get(ctx, &cg)
				So(err, ShouldBeNil)
				So(cg.DrainingStartTime, ShouldEqual, cfg.GetDrainingStartTime())
				So(cg.SubmitOptions, ShouldResembleProto, cfg.GetSubmitOptions())
				So(cg.Content, ShouldResembleProto, cfg.GetConfigGroups()[i])
				So(cg.CQStatusHost, ShouldResemble, cfg.GetCqStatusHost())
				So(cg.HonorGerritLinkedAccounts, ShouldEqual, cfg.GetHonorGerritLinkedAccounts())
			}
			// Verify ProjectConfig.
			pc := prjcfg.ProjectConfig{Project: "chromium"}
			err = datastore.Get(ctx, &pc)
			So(err, ShouldBeNil)
			So(pc, ShouldResemble, prjcfg.ProjectConfig{
				Project:          "chromium",
				SchemaVersion:    prjcfg.SchemaVersion,
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
			hashInfo := prjcfg.ConfigHashInfo{Hash: localHash, Project: projKey}
			err = datastore.Get(ctx, &hashInfo)
			So(err, ShouldBeNil)
			So(len(hashInfo.GitRevision), ShouldEqual, 40)
			hashInfo.GitRevision = ""
			// Verify the rest of ConfigHashInfo.
			So(hashInfo, ShouldResemble, prjcfg.ConfigHashInfo{
				Hash:             localHash,
				Project:          projKey,
				SchemaVersion:    prjcfg.SchemaVersion,
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
				config.MustProjectSet("chromium"): {
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
				pc := prjcfg.ProjectConfig{Project: "chromium"}
				So(datastore.Get(ctx, &pc), ShouldBeNil)
				So(pc.EVersion, ShouldEqual, 1)
				prevUpdatedTime := testClock.Now().Add(-10 * time.Minute)
				So(pc.UpdateTime, ShouldResemble, prevUpdatedTime.UTC())
				So(notifyCalled, ShouldBeFalse)

				Convey("But not noop if SchemaVersion changed", func() {
					old := pc // copy
					old.SchemaVersion--
					So(datastore.Put(ctx, &old), ShouldBeNil)

					err := UpdateProject(ctx, "chromium", notify)
					So(err, ShouldBeNil)
					So(notifyCalled, ShouldBeTrue)
					So(datastore.Get(ctx, &pc), ShouldBeNil)
					So(pc.EVersion, ShouldEqual, 2)
					So(pc.SchemaVersion, ShouldEqual, prjcfg.SchemaVersion)
				})
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
					config.MustProjectSet("chromium"): {
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
						config.MustProjectSet("chromium"): {
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
					before := prjcfg.ProjectConfig{Project: "chromium"}
					So(datastore.Get(ctx, &before), ShouldBeNil)
					// Delete config entities.
					projKey := prjcfg.ProjectConfigKey(ctx, "chromium")
					err := datastore.Delete(ctx,
						&prjcfg.ConfigHashInfo{Hash: before.Hash, Project: projKey},
						&prjcfg.ConfigGroup{
							ID:      prjcfg.MakeConfigGroupID(before.Hash, before.ConfigGroupNames[0]),
							Project: projKey,
						},
					)
					So(err, ShouldBeNil)

					testClock.Add(10 * time.Minute)
					So(UpdateProject(ctx, "chromium", notify), ShouldBeNil)
					after := prjcfg.ProjectConfig{Project: "chromium"}
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
			pc := prjcfg.ProjectConfig{
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
			actual := prjcfg.ProjectConfig{Project: "chromium"}
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
			actual := prjcfg.ProjectConfig{Project: "chromium"}
			So(datastore.Get(ctx, &actual), ShouldBeNil)
			So(actual.Enabled, ShouldBeFalse)
			So(actual.EVersion, ShouldEqual, 100)
			So(notifyCalled, ShouldBeFalse)
		})

		Convey("non-existing Project", func() {
			err := DisableProject(ctx, "non-existing", notify)
			So(err, ShouldBeNil)
			So(datastore.Get(ctx, &prjcfg.ProjectConfig{Project: "non-existing"}), ShouldErrLike, datastore.ErrNoSuchEntity)
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
	if err := srvcfg.SetTestListenerConfig(ctx, &listenerpb.Settings{}, nil); err != nil {
		panic(err)
	}
	return ctx, clock, scheduler
}

func toProtoText(msg proto.Message) string {
	bs, err := prototext.Marshal(msg)
	So(err, ShouldBeNil)
	return string(bs)
}

func TestPutConfigGroups(t *testing.T) {
	t.Parallel()

	Convey("PutConfigGroups", t, func() {
		ctx := gaememory.Use(context.Background())
		if testing.Verbose() {
			ctx = logging.SetLevel(gologger.StdConfig.Use(ctx), logging.Debug)
		}

		Convey("New Configs", func() {
			hash := prjcfg.ComputeHash(testCfg)
			err := putConfigGroups(ctx, testCfg, "chromium", hash)
			So(err, ShouldBeNil)
			stored := prjcfg.ConfigGroup{
				ID:      prjcfg.MakeConfigGroupID(hash, "group_foo"),
				Project: prjcfg.ProjectConfigKey(ctx, "chromium"),
			}
			So(datastore.Get(ctx, &stored), ShouldBeNil)
			So(stored.DrainingStartTime, ShouldEqual, testCfg.GetDrainingStartTime())
			So(stored.SubmitOptions, ShouldResembleProto, testCfg.GetSubmitOptions())
			So(stored.Content, ShouldResembleProto, testCfg.GetConfigGroups()[0])
			So(stored.HonorGerritLinkedAccounts, ShouldEqual, testCfg.GetHonorGerritLinkedAccounts())
			So(stored.SchemaVersion, ShouldEqual, prjcfg.SchemaVersion)

			Convey("Skip if already exists", func() {
				ctx := datastore.AddRawFilters(ctx, func(_ context.Context, rds datastore.RawInterface) datastore.RawInterface {
					return readOnlyFilter{rds}
				})
				err := putConfigGroups(ctx, testCfg, "chromium", prjcfg.ComputeHash(testCfg))
				So(err, ShouldBeNil)
			})

			Convey("Update existing due to SchemaVersion", func() {
				old := stored // copy
				old.SchemaVersion = prjcfg.SchemaVersion - 1
				So(datastore.Put(ctx, &old), ShouldBeNil)

				err := putConfigGroups(ctx, testCfg, "chromium", prjcfg.ComputeHash(testCfg))
				So(err, ShouldBeNil)

				So(datastore.Get(ctx, &stored), ShouldBeNil)
				So(stored.SchemaVersion, ShouldEqual, prjcfg.SchemaVersion)
			})
		})
	})
}

type readOnlyFilter struct{ datastore.RawInterface }

func (f readOnlyFilter) PutMulti(keys []*datastore.Key, vals []datastore.PropertyMap, cb datastore.NewKeyCB) error {
	panic("write is not supported")
}
