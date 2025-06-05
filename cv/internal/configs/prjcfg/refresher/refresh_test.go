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

	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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
)

func init() {
	registry.RegisterCmpOption(cmpopts.IgnoreUnexported(prjcfg.ProjectConfig{}))
	registry.RegisterCmpOption(cmpopts.IgnoreUnexported(prjcfg.ConfigHashInfo{}))
}

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
	GerritListenerType:        cfgpb.Config_GERRIT_LISTENER_TYPE_PUBSUB,
}

func TestUpdateProject(t *testing.T) {
	ftt.Run("Update Project", t, func(t *ftt.Test) {
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
			GerritListenerType:        cfgpb.Config_GERRIT_LISTENER_TYPE_PUBSUB,
		}
		configFileName := prjcfg.ConfigFileName(ctx)
		verifyEntitiesInDatastore := func(ctx context.Context, expectedEVersion int64) {
			cfg, meta := &cfgpb.Config{}, &config.Meta{}
			err := cfgclient.Get(ctx, config.MustProjectSet("chromium"), configFileName, cfgclient.ProtoText(cfg), meta)
			assert.NoErr(t, err)
			localHash := prjcfg.MustComputeHash(cfg)
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
				assert.NoErr(t, err)
				assert.That(t, cg.DrainingStartTime, should.Equal(cfg.GetDrainingStartTime()))
				assert.That(t, cg.SubmitOptions, should.Match(cfg.GetSubmitOptions()))
				assert.That(t, cg.Content, should.Match(cfg.GetConfigGroups()[i]))
				assert.That(t, cg.CQStatusHost, should.Match(cfg.GetCqStatusHost()))
				assert.That(t, cg.HonorGerritLinkedAccounts, should.Equal(cfg.GetHonorGerritLinkedAccounts()))
				assert.That(t, cg.GerritListenerType, should.Equal(cfg.GetGerritListenerType()))
			}
			// Verify ProjectConfig.
			pc := prjcfg.ProjectConfig{Project: "chromium"}
			err = datastore.Get(ctx, &pc)
			assert.NoErr(t, err)
			assert.That(t, pc, should.Match(prjcfg.ProjectConfig{
				Project:          "chromium",
				SchemaVersion:    prjcfg.SchemaVersion,
				Enabled:          true,
				EVersion:         expectedEVersion,
				Hash:             localHash,
				ExternalHash:     meta.ContentHash,
				UpdateTime:       datastore.RoundTime(testClock.Now()).UTC(),
				ConfigGroupNames: cgNames,
			}))
			// The revision in the memory-based config fake is a fake
			// 40-character sha256 hash digest. The particular value is
			// internally determined by the memory-based implementation
			// and isn't important here, so just assert that something
			// that looks like a hash digest is filled in.
			hashInfo := prjcfg.ConfigHashInfo{Hash: localHash, Project: projKey}
			err = datastore.Get(ctx, &hashInfo)
			assert.NoErr(t, err)
			assert.Loosely(t, len(hashInfo.GitRevision), should.Equal(40))
			hashInfo.GitRevision = ""
			// Verify the rest of ConfigHashInfo.
			assert.That(t, hashInfo, should.Match(prjcfg.ConfigHashInfo{
				Hash:             localHash,
				Project:          projKey,
				SchemaVersion:    prjcfg.SchemaVersion,
				ProjectEVersion:  expectedEVersion,
				UpdateTime:       datastore.RoundTime(testClock.Now()).UTC(),
				ConfigGroupNames: cgNames,
			}))
		}

		notifyCalled := false
		notify := func(context.Context) error {
			notifyCalled = true
			return nil
		}

		t.Run("Creates new ProjectConfig", func(t *ftt.Test) {
			ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{
				config.MustProjectSet("chromium"): {
					configFileName: toProtoText(t, chromiumConfig),
				},
			}))
			err := UpdateProject(ctx, "chromium", notify)
			assert.NoErr(t, err)
			verifyEntitiesInDatastore(ctx, 1)
			assert.Loosely(t, notifyCalled, should.BeTrue)

			notifyCalled = false
			testClock.Add(10 * time.Minute)

			t.Run("Noop if config is up-to-date", func(t *ftt.Test) {
				err := UpdateProject(ctx, "chromium", notify)
				assert.NoErr(t, err)
				pc := prjcfg.ProjectConfig{Project: "chromium"}
				assert.NoErr(t, datastore.Get(ctx, &pc))
				assert.Loosely(t, pc.EVersion, should.Equal(1))
				prevUpdatedTime := testClock.Now().Add(-10 * time.Minute)
				assert.That(t, pc.UpdateTime, should.Match(prevUpdatedTime.UTC()))
				assert.Loosely(t, notifyCalled, should.BeFalse)

				t.Run("But not noop if SchemaVersion changed", func(t *ftt.Test) {
					old := pc // copy
					old.SchemaVersion--
					assert.NoErr(t, datastore.Put(ctx, &old))

					err := UpdateProject(ctx, "chromium", notify)
					assert.NoErr(t, err)
					assert.Loosely(t, notifyCalled, should.BeTrue)
					assert.NoErr(t, datastore.Get(ctx, &pc))
					assert.Loosely(t, pc.EVersion, should.Equal(2))
					assert.Loosely(t, pc.SchemaVersion, should.Equal(prjcfg.SchemaVersion))
				})
			})

			t.Run("Update existing ProjectConfig", func(t *ftt.Test) {
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
						configFileName: toProtoText(t, updatedConfig),
					},
				}))
				err := UpdateProject(ctx, "chromium", notify)
				assert.NoErr(t, err)
				verifyEntitiesInDatastore(ctx, 2)
				assert.Loosely(t, notifyCalled, should.BeTrue)

				notifyCalled = false
				testClock.Add(10 * time.Minute)

				t.Run("Roll back to previous version", func(t *ftt.Test) {
					ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{
						config.MustProjectSet("chromium"): {
							configFileName: toProtoText(t, chromiumConfig),
						},
					}))

					err := UpdateProject(ctx, "chromium", notify)
					assert.NoErr(t, err)
					verifyEntitiesInDatastore(ctx, 3)
					assert.Loosely(t, notifyCalled, should.BeTrue)
				})

				t.Run("Re-enables project even if config hash is the same", func(t *ftt.Test) {
					testClock.Add(10 * time.Minute)
					assert.NoErr(t, DisableProject(ctx, "chromium", notify))
					before := prjcfg.ProjectConfig{Project: "chromium"}
					assert.NoErr(t, datastore.Get(ctx, &before))
					// Delete config entities.
					projKey := prjcfg.ProjectConfigKey(ctx, "chromium")
					err := datastore.Delete(ctx,
						&prjcfg.ConfigHashInfo{Hash: before.Hash, Project: projKey},
						&prjcfg.ConfigGroup{
							ID:      prjcfg.MakeConfigGroupID(before.Hash, before.ConfigGroupNames[0]),
							Project: projKey,
						},
					)
					assert.NoErr(t, err)

					testClock.Add(10 * time.Minute)
					assert.NoErr(t, UpdateProject(ctx, "chromium", notify))
					after := prjcfg.ProjectConfig{Project: "chromium"}
					assert.NoErr(t, datastore.Get(ctx, &after))

					assert.Loosely(t, after.Enabled, should.BeTrue)
					assert.Loosely(t, after.EVersion, should.Equal(before.EVersion+1))
					assert.That(t, after.Hash, should.Match(before.Hash))
					// Ensure deleted entities are re-created.
					verifyEntitiesInDatastore(ctx, 4)
					assert.Loosely(t, notifyCalled, should.BeTrue)
				})
			})
		})
	})
}

func TestDisableProject(t *testing.T) {
	ftt.Run("Disable", t, func(t *ftt.Test) {
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
			assert.NoErr(t, datastore.Put(ctx, &pc))
			testClock.Add(10 * time.Minute)
		}

		notifyCalled := false
		notify := func(context.Context) error {
			notifyCalled = true
			return nil
		}

		t.Run("currently enabled Project", func(t *ftt.Test) {
			writeProjectConfig(true)
			err := DisableProject(ctx, "chromium", notify)
			assert.NoErr(t, err)
			actual := prjcfg.ProjectConfig{Project: "chromium"}
			assert.NoErr(t, datastore.Get(ctx, &actual))
			assert.Loosely(t, actual.Enabled, should.BeFalse)
			assert.Loosely(t, actual.EVersion, should.Equal(101))
			assert.That(t, actual.UpdateTime, should.Match(datastore.RoundTime(testClock.Now()).UTC()))
			assert.Loosely(t, notifyCalled, should.BeTrue)
		})

		t.Run("currently disabled Project", func(t *ftt.Test) {
			writeProjectConfig(false)
			err := DisableProject(ctx, "chromium", notify)
			assert.NoErr(t, err)
			actual := prjcfg.ProjectConfig{Project: "chromium"}
			assert.NoErr(t, datastore.Get(ctx, &actual))
			assert.Loosely(t, actual.Enabled, should.BeFalse)
			assert.Loosely(t, actual.EVersion, should.Equal(100))
			assert.Loosely(t, notifyCalled, should.BeFalse)
		})

		t.Run("non-existing Project", func(t *ftt.Test) {
			err := DisableProject(ctx, "non-existing", notify)
			assert.NoErr(t, err)
			assert.ErrIsLike(t, datastore.Get(ctx, &prjcfg.ProjectConfig{Project: "non-existing"}), datastore.ErrNoSuchEntity)
			assert.Loosely(t, notifyCalled, should.BeFalse)
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

func toProtoText(t testing.TB, msg proto.Message) string {
	bs, err := prototext.Marshal(msg)
	assert.NoErr(t, err)
	return string(bs)
}

func TestPutConfigGroups(t *testing.T) {
	t.Parallel()

	ftt.Run("PutConfigGroups", t, func(t *ftt.Test) {
		ctx := gaememory.Use(context.Background())
		if testing.Verbose() {
			ctx = logging.SetLevel(gologger.StdConfig.Use(ctx), logging.Debug)
		}

		t.Run("New Configs", func(t *ftt.Test) {
			hash := prjcfg.MustComputeHash(testCfg)
			err := putConfigGroups(ctx, testCfg, "chromium", hash)
			assert.NoErr(t, err)
			stored := prjcfg.ConfigGroup{
				ID:      prjcfg.MakeConfigGroupID(hash, "group_foo"),
				Project: prjcfg.ProjectConfigKey(ctx, "chromium"),
			}
			assert.NoErr(t, datastore.Get(ctx, &stored))
			assert.Loosely(t, stored.DrainingStartTime, should.Equal(testCfg.GetDrainingStartTime()))
			assert.That(t, stored.SubmitOptions, should.Match(testCfg.GetSubmitOptions()))
			assert.That(t, stored.Content, should.Match(testCfg.GetConfigGroups()[0]))
			assert.That(t, stored.HonorGerritLinkedAccounts, should.Equal(testCfg.GetHonorGerritLinkedAccounts()))
			assert.That(t, stored.GerritListenerType, should.Equal(testCfg.GetGerritListenerType()))
			assert.That(t, stored.SchemaVersion, should.Equal(prjcfg.SchemaVersion))

			t.Run("Skip if already exists", func(t *ftt.Test) {
				ctx := datastore.AddRawFilters(ctx, func(_ context.Context, rds datastore.RawInterface) datastore.RawInterface {
					return readOnlyFilter{rds}
				})
				err := putConfigGroups(ctx, testCfg, "chromium", prjcfg.MustComputeHash(testCfg))
				assert.NoErr(t, err)
			})

			t.Run("Update existing due to SchemaVersion", func(t *ftt.Test) {
				old := stored // copy
				old.SchemaVersion = prjcfg.SchemaVersion - 1
				assert.NoErr(t, datastore.Put(ctx, &old))

				err := putConfigGroups(ctx, testCfg, "chromium", prjcfg.MustComputeHash(testCfg))
				assert.NoErr(t, err)

				assert.NoErr(t, datastore.Get(ctx, &stored))
				assert.Loosely(t, stored.SchemaVersion, should.Equal(prjcfg.SchemaVersion))
			})
		})
	})
}

type readOnlyFilter struct{ datastore.RawInterface }

func (f readOnlyFilter) PutMulti(keys []*datastore.Key, vals []datastore.PropertyMap, cb datastore.NewKeyCB) error {
	panic("write is not supported")
}
