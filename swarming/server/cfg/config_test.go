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

package cfg

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/cfg/internalcfgpb"
)

// An overall empty configs digest (including bot versions).
var emptyDigest = emptyVersionInfo().Digest

func TestUpdateConfigs(t *testing.T) {
	t.Parallel()

	ftt.Run("With test context", t, func(t *ftt.Test) {
		ctx := testCtx()

		call := func(files cfgmem.Files) error {
			return UpdateConfigs(cfgclient.Use(ctx, cfgmem.New(map[config.Set]cfgmem.Files{
				"services/${appid}": files,
			})), nil, nil)
		}

		fetchDS := func() *configBundle {
			bundle := &configBundle{Key: configBundleKey(ctx)}
			rev := &configBundleRev{Key: configBundleRevKey(ctx)}
			assert.Loosely(t, datastore.Get(ctx, bundle, rev), should.BeNil)
			assert.Loosely(t, bundle.Revision, should.Equal(rev.Revision))
			assert.Loosely(t, bundle.Digest, should.Equal(rev.Digest))
			assert.Loosely(t, bundle.Fetched.Equal(rev.Fetched), should.BeTrue)
			return bundle
		}

		// Start with no configs. Should store default empty config.
		assert.Loosely(t, call(nil), should.BeNil)
		assert.Loosely(t, fetchDS().Bundle, should.Resemble(defaultConfigs()))
		// Call it again. Still no configs.
		assert.Loosely(t, call(nil), should.BeNil)
		assert.Loosely(t, fetchDS().Bundle, should.Resemble(defaultConfigs()))

		configSet1 := cfgmem.Files{"bots.cfg": `
			trusted_dimensions: "pool"
			trusted_dimensions: "boo"
		`}
		configBundle1 := &internalcfgpb.ConfigBundle{
			Revision: "05005a8f6c612324b96acefed49d2bdb2e18032c",
			Digest:   "IH0p/sGs1VVrZznds3TIk+XnC+QI9kSx0sNjr4tzYmI",
			Settings: defaultConfigs().Settings,
			Pools:    defaultConfigs().Pools,
			Bots: &configpb.BotsCfg{
				TrustedDimensions: []string{"pool", "boo"},
			},
		}

		// A config appears. It is stored.
		assert.Loosely(t, call(configSet1), should.BeNil)
		assert.Loosely(t, fetchDS().Bundle, should.Resemble(configBundle1))

		t.Run("Updates good configs", func(t *ftt.Test) {
			configSet2 := cfgmem.Files{"bots.cfg": `
				trusted_dimensions: "pool"
				trusted_dimensions: "blah"
			`}
			configBundle2 := &internalcfgpb.ConfigBundle{
				Revision: "33f6ad5030e26987ee3d875e5eaef977680d1c88",
				Digest:   "yyMjJF0NqFzIHJbHtCwuUFkYMW1NgaM3bSAdAnai7sI",
				Settings: defaultConfigs().Settings,
				Pools:    defaultConfigs().Pools,
				Bots: &configpb.BotsCfg{
					TrustedDimensions: []string{"pool", "blah"},
				},
			}

			// Another config appears. It is store.
			assert.Loosely(t, call(configSet2), should.BeNil)
			assert.Loosely(t, fetchDS().Bundle, should.Resemble(configBundle2))
			// Call it again, the same config is there.
			assert.Loosely(t, call(configSet1), should.BeNil)
			assert.Loosely(t, fetchDS().Bundle, should.Resemble(configBundle1))
		})

		t.Run("Ignores bad configs", func(t *ftt.Test) {
			// A broken config appears. The old valid config is left unchanged.
			assert.Loosely(t, call(cfgmem.Files{"bots.cfg": `what is this`}), should.NotBeNil)
			assert.Loosely(t, fetchDS().Bundle, should.Resemble(configBundle1))
		})
	})
}

func TestParseAndValidateConfigs(t *testing.T) {
	t.Parallel()

	call := func(files map[string]string) (*internalcfgpb.ConfigBundle, error) {
		cfgs := make(map[string]config.Config, len(files))
		for k, v := range files {
			cfgs[k] = config.Config{Content: v}
		}
		return parseAndValidateConfigs(context.Background(), "rev", cfgs)
	}

	ftt.Run("Good", t, func(t *ftt.Test) {
		bundle, err := call(map[string]string{
			"settings.cfg": `
				google_analytics: "boo"
			`,
			"pools.cfg": `
				pool {
					name: "blah"
					realm: "test:bleh"
				}
			`,
			"bots.cfg": `
				trusted_dimensions: "pool"
				bot_group {
					bot_id: "id1"
					bot_config_script: "script1.py"
					auth {
						require_luci_machine_token: true
					}
				}
				bot_group {
					bot_id: "id2"
					bot_config_script: "script1.py"
					auth {
						require_luci_machine_token: true
					}
				}
				bot_group {
					bot_id: "id3"
					bot_config_script: "script2.py"
					auth {
						require_luci_machine_token: true
					}
				}
			`,
			"scripts/bot_config.py": "hooks",
			"scripts/script1.py":    "script1 body",
			"scripts/script2.py":    "script2 body",
			"scripts/ignored.py":    "ignored",
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, bundle, should.Resemble(&internalcfgpb.ConfigBundle{
			Revision: "rev",
			Digest:   "LBLN3ToniTYTTcn5xamgqOWbmwiFx32hNaQfOdrW3mQ",
			Settings: &configpb.SettingsCfg{
				GoogleAnalytics: "boo",
			},
			Pools: &configpb.PoolsCfg{
				Pool: []*configpb.Pool{
					{Name: []string{"blah"}, Realm: "test:bleh"},
				},
			},
			Bots: &configpb.BotsCfg{
				TrustedDimensions: []string{"pool"},
				BotGroup: []*configpb.BotGroup{
					{
						BotId:           []string{"id1"},
						BotConfigScript: "script1.py",
						Auth:            []*configpb.BotAuth{{RequireLuciMachineToken: true}},
					},
					{
						BotId:           []string{"id2"},
						BotConfigScript: "script1.py",
						Auth:            []*configpb.BotAuth{{RequireLuciMachineToken: true}},
					},
					{
						BotId:           []string{"id3"},
						BotConfigScript: "script2.py",
						Auth:            []*configpb.BotAuth{{RequireLuciMachineToken: true}},
					},
				},
			},
			Scripts: map[string]string{
				"script1.py": "script1 body",
				"script2.py": "script2 body",
			},
		}))
	})

	ftt.Run("Empty", t, func(t *ftt.Test) {
		empty := defaultConfigs()
		empty.Revision = "rev"
		bundle, err := call(nil)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, bundle, should.Resemble(empty))
	})

	ftt.Run("Broken", t, func(t *ftt.Test) {
		_, err := call(map[string]string{"bots.cfg": "what is this"})
		assert.Loosely(t, err, should.ErrLike("bots.cfg: proto"))
	})

	ftt.Run("Missing scripts", t, func(t *ftt.Test) {
		_, err := call(map[string]string{
			"bots.cfg": `
				trusted_dimensions: "pool"
				bot_group {
					bot_id: "id1"
					bot_config_script: "script1.py"
					auth {
						require_luci_machine_token: true
					}
				}
				bot_group {
					bot_id: "id2"
					bot_config_script: "missing.py"
					auth {
						require_luci_machine_token: true
					}
				}
			`,
			"scripts/script1.py": "script1 body",
			"scripts/ignored.py": "ignored",
		})
		assert.Loosely(t, err, should.ErrLike(`bot group #2 refers to undefined bot config script "missing.py"`))
	})
}

func TestFetchFromDatastore(t *testing.T) {
	t.Parallel()

	ftt.Run("With test context", t, func(t *ftt.Test) {
		ctx := testCtx()

		update := func(files cfgmem.Files) error {
			return UpdateConfigs(cfgclient.Use(ctx, cfgmem.New(map[config.Set]cfgmem.Files{
				"services/${appid}": files,
			})), nil, nil)
		}

		t.Run("Empty datastore", func(t *ftt.Test) {
			cfg, err := fetchFromDatastore(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg.VersionInfo.Revision, should.Equal(emptyRev))
			assert.Loosely(t, cfg.VersionInfo.Digest, should.Equal(emptyDigest))
			assert.Loosely(t, cfg.settings, should.Resemble(defaultConfigs().Settings))
		})

		t.Run("Default configs in datastore", func(t *ftt.Test) {
			// A cron job runs and discovers no configs.
			assert.Loosely(t, update(nil), should.BeNil)

			// Fetches initial copy of default config.
			cfg, err := fetchFromDatastore(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg.VersionInfo.Revision, should.Equal(emptyRev))
			assert.Loosely(t, cfg.VersionInfo.Digest, should.Equal(emptyDigest))
			assert.Loosely(t, cfg.settings, should.Resemble(defaultConfigs().Settings))

			// A real config appears.
			assert.Loosely(t, update(cfgmem.Files{"settings.cfg": `google_analytics: "boo"`}), should.BeNil)

			// It replaces the empty config when fetched (with defaults filled in).
			cfg, err = fetchFromDatastore(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg.VersionInfo.Revision, should.Equal("bcf7460a098890cc7efc8eda1c8279658ec25eb3"))
			assert.Loosely(t, cfg.VersionInfo.Digest, should.Equal("qGYmlgeHI+w+f9q08A5MDAt/eTXeK2uXNqyvCH5MoIg"))
			assert.Loosely(t, cfg.settings, should.Resemble(&configpb.SettingsCfg{
				GoogleAnalytics: "boo",
				Auth: &configpb.AuthSettings{
					AdminsGroup:          "administrators",
					BotBootstrapGroup:    "administrators",
					PrivilegedUsersGroup: "administrators",
					UsersGroup:           "administrators",
					ViewAllBotsGroup:     "administrators",
					ViewAllTasksGroup:    "administrators",
				},
				BotDeathTimeoutSecs: 600,
				ReusableTaskAgeSecs: 604800,
			}))
		})
	})
}

func TestBuildQueriableConfig(t *testing.T) {
	t.Parallel()

	build := func(bundle *internalcfgpb.ConfigBundle) (*Config, error) {
		return buildQueriableConfig(context.Background(), &configBundle{
			VersionInfo: VersionInfo{
				Revision: "some-revision",
				Digest:   "some-digest",
			},
			Bundle: bundle,
		})
	}

	ftt.Run("OK", t, func(t *ftt.Test) {
		cfg, err := build(&internalcfgpb.ConfigBundle{
			Settings: withDefaultSettings(&configpb.SettingsCfg{
				TrafficMigration: &configpb.TrafficMigration{
					Routes: []*configpb.TrafficMigration_Route{
						{Name: "/prpc/service/method1", RouteToGoPercent: 0},
						{Name: "/prpc/service/method2", RouteToGoPercent: 50},
						{Name: "/prpc/service/method3", RouteToGoPercent: 100},
					},
				},
			}),
			Pools: &configpb.PoolsCfg{
				Pool: []*configpb.Pool{
					{
						Name:  []string{"a"},
						Realm: "realm:a",
					},
					{
						Name:  []string{"b"},
						Realm: "realm:b",
					},
				},
			},
			Bots: &configpb.BotsCfg{
				BotGroup: []*configpb.BotGroup{
					{
						BotId:      []string{"host-0-0", "host-0-1--abc"},
						Dimensions: []string{"pool:a"},
					},
					{
						BotIdPrefix: []string{"host-0-"},
						Dimensions:  []string{"pool:b"},
					},
					{
						BotIdPrefix: []string{"host-1-"},
						Dimensions:  []string{"pool:c"},
					},
					{
						Dimensions: []string{"pool:default"},
					},
				},
			},
		})
		assert.Loosely(t, err, should.BeNil)

		// Pools.cfg processed correctly.
		assert.Loosely(t, cfg.Pools(), should.Resemble([]string{"a", "b"}))
		assert.Loosely(t, cfg.Pool("a").Realm, should.Equal("realm:a"))
		assert.Loosely(t, cfg.Pool("b").Realm, should.Equal("realm:b"))
		assert.Loosely(t, cfg.Pool("unknown"), should.BeNil)

		// Bots.cfg processed correctly.
		botPool := func(botID string) string {
			return cfg.BotGroup(botID).Dimensions["pool"][0]
		}
		assert.Loosely(t, botPool("host-0-0"), should.Equal("a"))
		assert.Loosely(t, botPool("host-0-1--abc"), should.Equal("a"))
		assert.Loosely(t, botPool("host-0-1"), should.Equal("b"))
		assert.Loosely(t, botPool("host-1-0"), should.Equal("c"))
		assert.Loosely(t, botPool("host-0"), should.Equal("default"))

		// Traffic routing rules are processed.
		assert.Loosely(t, cfg.RouteToGoPercent("/prpc/unknown"), should.BeZero)
		assert.Loosely(t, cfg.RouteToGoPercent("/prpc/service/method1"), should.BeZero)
		assert.Loosely(t, cfg.RouteToGoPercent("/prpc/service/method2"), should.Equal(50))
		assert.Loosely(t, cfg.RouteToGoPercent("/prpc/service/method3"), should.Equal(100))
	})
}

func TestBotRBEConfig(t *testing.T) {
	t.Parallel()

	build := func(dims []string, pools []*configpb.Pool) *Config {
		cfg, err := buildQueriableConfig(context.Background(), &configBundle{
			Bundle: &internalcfgpb.ConfigBundle{
				Settings: withDefaultSettings(&configpb.SettingsCfg{}),
				Bots: &configpb.BotsCfg{
					BotGroup: []*configpb.BotGroup{
						{
							BotId:      []string{"bot"},
							Dimensions: dims,
						},
					},
				},
				Pools: &configpb.PoolsCfg{
					Pool: pools,
				},
			},
		})
		if err != nil {
			panic(err)
		}
		return cfg
	}

	ftt.Run("Unknown pools", t, func(t *ftt.Test) {
		cfg := build([]string{"pool:unknown", "pool:another-unknown"}, nil)
		assert.That(t, cfg.RBEConfig("bot"), should.Equal(RBEConfig{
			Mode: configpb.Pool_RBEMigration_BotModeAllocation_SWARMING,
		}))
	})

	ftt.Run("Pure RBE mode", t, func(t *ftt.Test) {
		cfg := build([]string{"pool:a", "pool:b"}, []*configpb.Pool{
			{
				Name: []string{"a", "b"},
				RbeMigration: &configpb.Pool_RBEMigration{
					RbeInstance: "some-instance",
					BotModeAllocation: []*configpb.Pool_RBEMigration_BotModeAllocation{
						{
							Mode:    configpb.Pool_RBEMigration_BotModeAllocation_RBE,
							Percent: 100,
						},
					},
				},
			},
		})
		assert.That(t, cfg.RBEConfig("bot"), should.Equal(RBEConfig{
			Mode:     configpb.Pool_RBEMigration_BotModeAllocation_RBE,
			Instance: "some-instance",
		}))
	})

	ftt.Run("Pure Swarming mode", t, func(t *ftt.Test) {
		cfg := build([]string{"pool:a", "pool:b"}, []*configpb.Pool{
			{
				Name: []string{"a", "b"},
			},
		})
		assert.That(t, cfg.RBEConfig("bot"), should.Equal(RBEConfig{
			Mode: configpb.Pool_RBEMigration_BotModeAllocation_SWARMING,
		}))
	})

	ftt.Run("Hybrid RBE mode", t, func(t *ftt.Test) {
		cfg := build([]string{"pool:a", "pool:b"}, []*configpb.Pool{
			{
				Name: []string{"a"},
			},
			{
				Name: []string{"b"},
				RbeMigration: &configpb.Pool_RBEMigration{
					RbeInstance: "some-instance",
					BotModeAllocation: []*configpb.Pool_RBEMigration_BotModeAllocation{
						{
							Mode:    configpb.Pool_RBEMigration_BotModeAllocation_RBE,
							Percent: 100,
						},
					},
				},
			},
		})
		assert.That(t, cfg.RBEConfig("bot"), should.Equal(RBEConfig{
			Mode:     configpb.Pool_RBEMigration_BotModeAllocation_HYBRID,
			Instance: "some-instance",
		}))
	})
}

func testCtx() context.Context {
	ctx := context.Background()
	ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
	ctx = memory.Use(ctx)
	return ctx
}
