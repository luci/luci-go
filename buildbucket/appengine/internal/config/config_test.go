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

package config

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/impl/memory"

	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestConfig(t *testing.T) {
	t.Parallel()
	ftt.Run("get settings.cfg", t, func(t *ftt.Test) {
		settingsCfg := &pb.SettingsCfg{Resultdb: &pb.ResultDBSettings{Hostname: "testing.results.api.cr.dev"}}
		ctx := memory.Use(context.Background())
		SetTestSettingsCfg(ctx, settingsCfg)
		cfg, err := GetSettingsCfg(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cfg, should.Match(settingsCfg))
	})

	ftt.Run("validate settings.cfg", t, func(t *ftt.Test) {
		vctx := &validation.Context{
			Context: context.Background(),
		}
		configSet := "services/${appid}"
		path := "settings.cfg"

		t.Run("bad proto", func(t *ftt.Test) {
			content := []byte(` bad: "bad" `)
			assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("invalid SettingsCfg proto message"))
		})

		t.Run("empty settings.cfg", func(t *ftt.Test) {
			content := []byte(` `)
			assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("logdog.hostname unspecified"))
		})

		t.Run("no swarming cfg", func(t *ftt.Test) {
			content := []byte(`
				logdog {
					hostname: "logs.chromium.org"
				}
				resultdb {
					hostname: "results.api.cr.dev"
				}
			`)
			assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
		})

		t.Run("no milo", func(t *ftt.Test) {
			content := []byte(`swarming{}`)
			assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("milo_hostname unspecified"))
		})

		t.Run("invalid user_packages", func(t *ftt.Test) {
			content := []byte(`
				swarming {
					milo_hostname: "ci.chromium.org"
					user_packages {
						package_name: ""
						version: "git_revision:c84736ceb5ddcc3f6e6d1e6c4d602bb024ceb1b2"
					}
				}
			`)
			assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("(swarming / user_packages #0): package_name is required"))
		})

		t.Run("invalid alternative_agent_packages", func(t *ftt.Test) {
			content := []byte(`
				swarming {
					milo_hostname: "ci.chromium.org"
					alternative_agent_packages {
						package_name: "bbagent_alternative"
						version: "git_revision:c84736ceb5ddcc3f6e6d1e6c4d602bb024ceb1b2"
					}
				}
			`)
			assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("alternative_agent_package must set constraints on either omit_on_experiment or include_on_experiment"))
		})

		t.Run("no /${platform} in bbagent", func(t *ftt.Test) {
			content := []byte(`
				swarming {
					milo_hostname: "ci.chromium.org"
					bbagent_package {
						package_name: "infra/tools/luci/bbagent"
						version: "git_revision:60be805bf35a766cdf7d80bdf0a066dce30691e8"
					}
				}
			`)
			assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("(swarming / bbagent_package): package_name must end with '/${platform}'"))
		})

		t.Run("invalid builders.regex in experiments", func(t *ftt.Test) {
			content := []byte(`
				experiment {
					experiments {
						name: "luci.buildbucket.bbagent_getbuild"
						builders {
							regex: "(no right parenthesis"
						}
					}
				}
			`)
			assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("(experiment.experiments #0): builders.regex \"(no right parenthesis\": invalid regex"))
		})

		t.Run("wrong minimum_value in experiments", func(t *ftt.Test) {
			content := []byte(`
				experiment {
					experiments {
						name: "luci.buildbucket.bbagent_getbuild"
						minimum_value: 101
					}
				}
			`)
			assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("minimum_value must be in [0,100]"))
		})

		t.Run("wrong default_value in experiments", func(t *ftt.Test) {
			content := []byte(`
				experiment {
					experiments {
						name: "luci.buildbucket.bbagent_getbuild"
						default_value: 5
						minimum_value: 10
					}
				}
			`)
			assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("default_value must be in [${minimum_value},100]"))
		})

		t.Run("invalid inactive in experiments", func(t *ftt.Test) {
			content := []byte(`
				experiment {
					experiments {
						name: "luci.buildbucket.bbagent_getbuild"
						minimum_value: 5
						default_value: 5
						inactive: true
					}
				}
			`)
			assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("default_value and minimum_value must both be 0 when inactive is true"))
		})

		t.Run("invalid backend", func(t *ftt.Test) {
			content := []byte(`
				backends {
					target: "swarming://chromium-swarm"
					hostname: "swarming://chromium-swarm"
				}
			`)
			assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("BackendSetting.hostname must not contain '://"))
		})

		t.Run("valid empty target backend", func(t *ftt.Test) {
			content := []byte(`
				logdog {
					hostname: "logs.chromium.org"
				}
				resultdb {
					hostname: "results.api.cr.dev"
				}
				backends {
					target: ""
					hostname: "chromium-swarm-dev.appspot.com"
					lite_mode {}
				}
			`)
			assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
		})

		t.Run("invalid backend full mode", func(t *ftt.Test) {
			t.Run("build_sync_setting", func(t *ftt.Test) {
				t.Run("sync_interval_seconds", func(t *ftt.Test) {
					content := []byte(`
						logdog {
							hostname: "logs.chromium.org"
						}
						resultdb {
							hostname: "results.api.cr.dev"
						}
						backends {
							target: "swarming://chromium-swarm"
							hostname: "chromium-swarm.appspot.com"
							full_mode {
								build_sync_setting {
									sync_interval_seconds: 30
								}
								pubsub_id: "topic"
							}
						}
					`)
					assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
					assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("sync_interval_seconds must be greater than or equal to 60"))
				})
				t.Run("shards", func(t *ftt.Test) {
					content := []byte(`
						logdog {
							hostname: "logs.chromium.org"
						}
						resultdb {
							hostname: "results.api.cr.dev"
						}
						backends {
							target: "swarming://chromium-swarm"
							hostname: "chromium-swarm.appspot.com"
							full_mode {
								build_sync_setting {
									shards: -60
								}
								pubsub_id: "topic"
							}
						}
					`)
					assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
					assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("shards must be greater than or equal to 0"))
				})
			})
			t.Run("pubsub_id", func(t *ftt.Test) {
				content := []byte(`
						logdog {
							hostname: "logs.chromium.org"
						}
						resultdb {
							hostname: "results.api.cr.dev"
						}
						backends {
							target: "swarming://chromium-swarm"
							hostname: "chromium-swarm.appspot.com"
							full_mode {
							}
						}
					`)
				assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
				assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("pubsub_id for UpdateBuildTask must be specified"))
			})

			t.Run("empty", func(t *ftt.Test) {
				content := []byte(`
						logdog {
							hostname: "logs.chromium.org"
						}
						resultdb {
							hostname: "results.api.cr.dev"
						}
						backends {
							target: "swarming://chromium-swarm"
							hostname: "chromium-swarm.appspot.com"
						}
					`)
				assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
				assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("mode field is not set or its type is unsupported"))
			})
		})

		t.Run("invalid custom metrics", func(t *ftt.Test) {
			t.Run("invalid name", func(t *ftt.Test) {
				content := []byte(`
							logdog {
								hostname: "logs.chromium.org"
							}
							resultdb {
								hostname: "results.api.cr.dev"
							}
							custom_metrics {
								name: "/chrome/infra/custom/builds/started/",
								metric_base: CUSTOM_METRIC_BASE_STARTED,
							}
						`)
				assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
				assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring(`invalid metric name "/chrome/infra/custom/builds/started/": doesn't match ^(/[a-zA-Z0-9_-]+)+$`))
			})

			t.Run("name without / prefix", func(t *ftt.Test) {
				content := []byte(`
							logdog {
								hostname: "logs.chromium.org"
							}
							resultdb {
								hostname: "results.api.cr.dev"
							}
							custom_metrics {
								name: "custom/builds/started/",
								metric_base: CUSTOM_METRIC_BASE_STARTED,
							}
						`)
				assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
				assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring(`invalid metric name "custom/builds/started/": must starts with "/"`))
			})

			t.Run("bb reserved name", func(t *ftt.Test) {
				content := []byte(`
							logdog {
								hostname: "logs.chromium.org"
							}
							resultdb {
								hostname: "results.api.cr.dev"
							}
							custom_metrics {
								name: "/chrome/infra/buildbucket/v2/builds/started",
								metric_base: CUSTOM_METRIC_BASE_STARTED,
							}
						`)
				assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
				assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring(`"/chrome/infra/buildbucket/v2/builds/started" is reserved by Buildbucket`))
			})

			t.Run("duplicated names", func(t *ftt.Test) {
				content := []byte(`
						logdog {
							hostname: "logs.chromium.org"
						}
						resultdb {
							hostname: "results.api.cr.dev"
						}
						custom_metrics {
							name: "/chrome/infra/custom/builds/started",
							extra_fields: "experiments",
							metric_base: CUSTOM_METRIC_BASE_STARTED,
						}
						custom_metrics {
							name: "/chrome/infra/custom/builds/started",
							extra_fields: "experiments",
							metric_base: CUSTOM_METRIC_BASE_STARTED,
						}
					`)
				assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
				assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("duplicated name is not allowed: /chrome/infra/custom/builds/started"))
			})

			t.Run("invalid base", func(t *ftt.Test) {
				content := []byte(`
						logdog {
							hostname: "logs.chromium.org"
						}
						resultdb {
							hostname: "results.api.cr.dev"
						}
						custom_metrics {
							name: "/chrome/infra/custom/builds/started",
							metric_base: CUSTOM_METRIC_BASE_UNSET,
						}
					`)
				assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
				assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring(`base CUSTOM_METRIC_BASE_UNSET is invalid`))
			})

			t.Run("invalid field", func(t *ftt.Test) {
				content := []byte(`
						logdog {
							hostname: "logs.chromium.org"
						}
						resultdb {
							hostname: "results.api.cr.dev"
						}
						custom_metrics {
							name: "/chrome/infra/custom/builds/started",
							extra_fields: "$status",
							metric_base: CUSTOM_METRIC_BASE_STARTED,
						}
					`)
				assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
				assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring(`invalid field name "$status": doesn't match ^[A-Za-z_][A-Za-z0-9_]*$`))
			})

			t.Run("duplicated field", func(t *ftt.Test) {
				content := []byte(`
						logdog {
							hostname: "logs.chromium.org"
						}
						resultdb {
							hostname: "results.api.cr.dev"
						}
						custom_metrics {
							name: "/chrome/infra/custom/builds/started",
							extra_fields: "os",
							extra_fields: "os",
							metric_base: CUSTOM_METRIC_BASE_STARTED,
						}
					`)
				assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
				assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring(`"os" is duplicated`))
			})

			t.Run("extra_fields contain base field", func(t *ftt.Test) {
				content := []byte(`
						logdog {
							hostname: "logs.chromium.org"
						}
						resultdb {
							hostname: "results.api.cr.dev"
						}
						custom_metrics {
							name: "/chrome/infra/custom/builds/completed",
							extra_fields: "os",
							extra_fields: "status",
							metric_base: CUSTOM_METRIC_BASE_COMPLETED,
						}
					`)
				assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
				assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring(`cannot contain base fields ["status"] in extra_fields`))
			})

			t.Run("extra_fields for builder metrics", func(t *ftt.Test) {
				content := []byte(`
						logdog {
							hostname: "logs.chromium.org"
						}
						resultdb {
							hostname: "results.api.cr.dev"
						}
						custom_metrics {
							name: "/chrome/infra/custom/builds/count",
							extra_fields: "os",
							metric_base: CUSTOM_METRIC_BASE_COUNT,
						}
					`)
				assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, content), should.BeNil)
				assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring(`custom builder metric cannot have extra_fields`))
			})
		})

		t.Run("OK", func(t *ftt.Test) {
			var okCfg = `
				swarming {
					milo_hostname: "ci.chromium.org"
					global_caches {
						path: "git"
					}
					bbagent_package {
						package_name: "infra/tools/luci/bbagent/${platform}"
						version: "git_revision:60be805bf35a766cdf7d80bdf0a066dce30691e8"
					}
					kitchen_package {
						package_name: "infra/tools/luci/kitchen/${platform}"
						version: "git_revision:63874080a20260642c8df82d4f4885ff30b33fb6"
					}
					user_packages {
						package_name: "infra/3pp/tools/git/${platform}"
						version: "version:2@2.33.0.chromium.6"
					}
					alternative_agent_packages {
						package_name: "infra/tools/luci/bbagent_alternative/${platform}"
						version: "git_revision:60be805bf35a766cdf7d80bdf0a066dce30691e8"
						include_on_experiment: "luci.buildbucket.use_bbagent_alternative"
					}
				}
				logdog {
					hostname: "logs.chromium.org"
				}
				resultdb {
					hostname: "results.api.cr.dev"
				}
				known_public_gerrit_hosts: "swiftshader.googlesource.com"
				known_public_gerrit_hosts: "webrtc.googlesource.com"
				experiment {
					experiments {
						name: "luci.buildbucket.bbagent_getbuild"
					}
					# inactive experiments
					experiments { name: "luci.use_realms" inactive: true }
				}
				backends {
					target: "swarming://chromium-swarm"
					hostname: "chromium-swarm.appspot.com"
					full_mode {
						pubsub_id: "topic"
					}
				}
				custom_metrics {
					name: "/chrome/infra/custom/builds/started",
					extra_fields: "os",
					extra_fields: "branch",
					extra_fields: "experiments",
					metric_base: CUSTOM_METRIC_BASE_STARTED,
				}
				custom_metrics {
					name: "/chrome/infra/custom/builds/completed",
					extra_fields: "os",
					extra_fields: "final_step",
					extra_fields: "experiments",
					metric_base: CUSTOM_METRIC_BASE_COMPLETED,
				}
				custom_metrics {
					name: "/chrome/infra/custom/builds/count",
					metric_base: CUSTOM_METRIC_BASE_COUNT,
				}
			`
			assert.Loosely(t, validateSettingsCfg(vctx, configSet, path, []byte(okCfg)), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
		})
	})
}
