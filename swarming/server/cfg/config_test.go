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
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/cfg/internalcfgpb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestUpdateConfigs(t *testing.T) {
	t.Parallel()

	Convey("With test context", t, func() {
		ctx := testCtx()

		call := func(files cfgmem.Files) error {
			return UpdateConfigs(cfgclient.Use(ctx, cfgmem.New(map[config.Set]cfgmem.Files{
				"services/${appid}": files,
			})))
		}

		fetchDS := func() *configBundle {
			bundle := &configBundle{Key: configBundleKey(ctx)}
			rev := &configBundleRev{Key: configBundleRevKey(ctx)}
			So(datastore.Get(ctx, bundle, rev), ShouldBeNil)
			So(bundle.Revision, ShouldEqual, rev.Revision)
			So(bundle.Digest, ShouldEqual, rev.Digest)
			So(bundle.Fetched.Equal(rev.Fetched), ShouldBeTrue)
			return bundle
		}

		// Start with no configs. Should store default empty config.
		So(call(nil), ShouldBeNil)
		So(fetchDS().Bundle, ShouldResembleProto, defaultConfigs())
		// Call it again. Still no configs.
		So(call(nil), ShouldBeNil)
		So(fetchDS().Bundle, ShouldResembleProto, defaultConfigs())

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
		So(call(configSet1), ShouldBeNil)
		So(fetchDS().Bundle, ShouldResembleProto, configBundle1)

		Convey("Updates good configs", func() {
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
			So(call(configSet2), ShouldBeNil)
			So(fetchDS().Bundle, ShouldResembleProto, configBundle2)
			// Call it again, the same config is there.
			So(call(configSet1), ShouldBeNil)
			So(fetchDS().Bundle, ShouldResembleProto, configBundle1)
		})

		Convey("Ignores bad configs", func() {
			// A broken config appears. The old valid config is left unchanged.
			So(call(cfgmem.Files{"bots.cfg": `what is this`}), ShouldNotBeNil)
			So(fetchDS().Bundle, ShouldResembleProto, configBundle1)
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

	Convey("Good", t, func() {
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
			"scripts/script1.py": "script1 body",
			"scripts/script2.py": "script2 body",
			"scripts/ignored.py": "ignored",
		})
		So(err, ShouldBeNil)
		So(bundle, ShouldResembleProto, &internalcfgpb.ConfigBundle{
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
		})
	})

	Convey("Empty", t, func() {
		empty := defaultConfigs()
		empty.Revision = "rev"
		bundle, err := call(nil)
		So(err, ShouldBeNil)
		So(bundle, ShouldResembleProto, empty)
	})

	Convey("Broken", t, func() {
		_, err := call(map[string]string{"bots.cfg": "what is this"})
		So(err, ShouldErrLike, "bots.cfg: proto")
	})

	Convey("Missing scripts", t, func() {
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
		So(err, ShouldErrLike, `bot group #2 refers to undefined bot config script "missing.py"`)
	})
}

func TestFetchFromDatastore(t *testing.T) {
	t.Parallel()

	Convey("With test context", t, func() {
		ctx := testCtx()

		update := func(files cfgmem.Files) error {
			return UpdateConfigs(cfgclient.Use(ctx, cfgmem.New(map[config.Set]cfgmem.Files{
				"services/${appid}": files,
			})))
		}

		fetch := func(cur *Config) (*Config, error) {
			return fetchFromDatastore(ctx, cur)
		}

		Convey("Empty datastore", func() {
			cfg, err := fetch(nil)
			So(err, ShouldBeNil)
			So(cfg.Revision, ShouldEqual, emptyRev)
			So(cfg.Digest, ShouldEqual, emptyDigest)
			So(cfg.settings, ShouldResembleProto, defaultConfigs().Settings)
		})

		Convey("Default configs in datastore", func() {
			// A cron job runs and discovers no configs.
			So(update(nil), ShouldBeNil)

			// Fetches initial copy of default config.
			cfg, err := fetch(nil)
			So(err, ShouldBeNil)
			So(cfg.Revision, ShouldEqual, emptyRev)
			So(cfg.Digest, ShouldEqual, emptyDigest)
			So(cfg.settings, ShouldResembleProto, defaultConfigs().Settings)

			// Does nothing, there's still no real config.
			cfg, err = fetch(cfg)
			So(err, ShouldBeNil)
			So(cfg.Revision, ShouldEqual, emptyRev)
			So(cfg.Digest, ShouldEqual, emptyDigest)
			So(cfg.settings, ShouldResembleProto, defaultConfigs().Settings)

			// A real config appears.
			So(update(cfgmem.Files{"settings.cfg": `google_analytics: "boo"`}), ShouldBeNil)

			// It replaces the empty config when fetched (with defaults filled in).
			cfg, err = fetch(cfg)
			So(err, ShouldBeNil)
			So(cfg.Revision, ShouldEqual, "bcf7460a098890cc7efc8eda1c8279658ec25eb3")
			So(cfg.Digest, ShouldEqual, "xv7iFT37ovx5Qc9kjYK0kEa3Eq47cNNC0ZbEd61eOYQ")
			So(cfg.settings, ShouldResembleProto, &configpb.SettingsCfg{
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
			})

			// Suddenly the config is completely gone.
			So(datastore.Delete(ctx, configBundleKey(ctx), configBundleRevKey(ctx)), ShouldBeNil)

			// The default config is used again.
			cfg, err = fetch(cfg)
			So(err, ShouldBeNil)
			So(cfg.Revision, ShouldEqual, emptyRev)
			So(cfg.Digest, ShouldEqual, emptyDigest)
			So(cfg.settings, ShouldResembleProto, defaultConfigs().Settings)
		})

		Convey("Some configs in datastore", func() {
			// Some initial config.
			So(update(cfgmem.Files{"settings.cfg": `google_analytics: "boo"`}), ShouldBeNil)

			// Fetches initial copy of this config.
			cfg, err := fetch(nil)
			So(err, ShouldBeNil)
			So(cfg.Revision, ShouldEqual, "bcf7460a098890cc7efc8eda1c8279658ec25eb3")
			So(cfg.Digest, ShouldEqual, "xv7iFT37ovx5Qc9kjYK0kEa3Eq47cNNC0ZbEd61eOYQ")
			So(cfg.settings, ShouldResembleProto, withDefaultSettings(&configpb.SettingsCfg{
				GoogleAnalytics: "boo",
			}))

			// Nothing change. The same config is returned.
			cfg, err = fetch(cfg)
			So(err, ShouldBeNil)
			So(cfg.Revision, ShouldEqual, "bcf7460a098890cc7efc8eda1c8279658ec25eb3")
			So(cfg.Digest, ShouldEqual, "xv7iFT37ovx5Qc9kjYK0kEa3Eq47cNNC0ZbEd61eOYQ")
			So(cfg.settings, ShouldResembleProto, withDefaultSettings(&configpb.SettingsCfg{
				GoogleAnalytics: "boo",
			}))

			// A noop config change happens.
			So(update(cfgmem.Files{
				"settings.cfg": `google_analytics: "boo"`,
				"unrelated":    "change",
			}), ShouldBeNil)

			// Has the new revision, but the same digest and the exact same body.
			prev := cfg.settings
			cfg, err = fetch(cfg)
			So(err, ShouldBeNil)
			So(cfg.Revision, ShouldEqual, "725455c37c09b5dfa3dc31b47c9f787d555cbfb3")
			So(cfg.Digest, ShouldEqual, "xv7iFT37ovx5Qc9kjYK0kEa3Eq47cNNC0ZbEd61eOYQ")
			So(cfg.settings == prev, ShouldBeTrue) // equal as pointers

			// A real config change happens.
			So(update(cfgmem.Files{"settings.cfg": `google_analytics: "blah"`}), ShouldBeNil)

			// Causes the config update.
			cfg, err = fetch(cfg)
			So(err, ShouldBeNil)
			So(cfg.Revision, ShouldEqual, "3a7123badfd5426f2a75932433f99b0aee8baf9b")
			So(cfg.Digest, ShouldEqual, "+JawsgfwWonAz8wFE/iR2AcdVmMhK3OwFbNSgCjBiRo")
			So(cfg.settings, ShouldResembleProto, withDefaultSettings(&configpb.SettingsCfg{
				GoogleAnalytics: "blah",
			}))
		})
	})
}

func TestBuildQueriableConfig(t *testing.T) {
	t.Parallel()

	build := func(bundle *internalcfgpb.ConfigBundle) (*Config, error) {
		return buildQueriableConfig(context.Background(), &configBundle{
			Revision: "some-revision",
			Digest:   "some-digest",
			Bundle:   bundle,
		})
	}

	Convey("OK", t, func() {
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
		So(err, ShouldBeNil)

		// Pools.cfg processed correctly.
		So(cfg.Pools(), ShouldResemble, []string{"a", "b"})
		So(cfg.Pool("a").Realm, ShouldEqual, "realm:a")
		So(cfg.Pool("b").Realm, ShouldEqual, "realm:b")
		So(cfg.Pool("unknown"), ShouldBeNil)

		// Bots.cfg processed correctly.
		botPool := func(botID string) string {
			return cfg.BotGroup(botID).Dimensions["pool"][0]
		}
		So(botPool("host-0-0"), ShouldEqual, "a")
		So(botPool("host-0-1--abc"), ShouldEqual, "a")
		So(botPool("host-0-1"), ShouldEqual, "b")
		So(botPool("host-1-0"), ShouldEqual, "c")
		So(botPool("host-0"), ShouldEqual, "default")

		// Traffic routing rules are processed.
		So(cfg.RouteToGoPercent("/prpc/unknown"), ShouldEqual, 0)
		So(cfg.RouteToGoPercent("/prpc/service/method1"), ShouldEqual, 0)
		So(cfg.RouteToGoPercent("/prpc/service/method2"), ShouldEqual, 50)
		So(cfg.RouteToGoPercent("/prpc/service/method3"), ShouldEqual, 100)
	})
}

func testCtx() context.Context {
	ctx := context.Background()
	ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
	ctx = memory.Use(ctx)
	return ctx
}
