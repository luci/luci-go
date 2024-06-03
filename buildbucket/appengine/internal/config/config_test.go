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

	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/impl/memory"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestConfig(t *testing.T) {
	t.Parallel()
	Convey("get settings.cfg", t, func() {
		settingsCfg := &pb.SettingsCfg{Resultdb: &pb.ResultDBSettings{Hostname: "testing.results.api.cr.dev"}}
		ctx := memory.Use(context.Background())
		SetTestSettingsCfg(ctx, settingsCfg)
		cfg, err := GetSettingsCfg(ctx)
		So(err, ShouldBeNil)
		So(cfg, ShouldResembleProto, settingsCfg)
	})

	Convey("validate settings.cfg", t, func() {
		vctx := &validation.Context{
			Context: context.Background(),
		}
		configSet := "services/${appid}"
		path := "settings.cfg"

		Convey("bad proto", func() {
			content := []byte(` bad: "bad" `)
			So(validateSettingsCfg(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "invalid SettingsCfg proto message")
		})

		Convey("empty settings.cfg", func() {
			content := []byte(` `)
			So(validateSettingsCfg(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "logdog.hostname unspecified")
		})

		Convey("no swarming cfg", func() {
			content := []byte(`
				logdog {
					hostname: "logs.chromium.org"
				}
				resultdb {
					hostname: "results.api.cr.dev"
				}
			`)
			So(validateSettingsCfg(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldBeNil)
		})

		Convey("no milo", func() {
			content := []byte(`swarming{}`)
			So(validateSettingsCfg(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "milo_hostname unspecified")
		})

		Convey("invalid user_packages", func() {
			content := []byte(`
				swarming {
					milo_hostname: "ci.chromium.org"
					user_packages {
						package_name: ""
						version: "git_revision:c84736ceb5ddcc3f6e6d1e6c4d602bb024ceb1b2"
					}
				}
			`)
			So(validateSettingsCfg(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "(swarming / user_packages #0): package_name is required")
		})

		Convey("invalid alternative_agent_packages", func() {
			content := []byte(`
				swarming {
					milo_hostname: "ci.chromium.org"
					alternative_agent_packages {
						package_name: "bbagent_alternative"
						version: "git_revision:c84736ceb5ddcc3f6e6d1e6c4d602bb024ceb1b2"
					}
				}
			`)
			So(validateSettingsCfg(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "alternative_agent_package must set constraints on either omit_on_experiment or include_on_experiment")
		})

		Convey("no /${platform} in bbagent", func() {
			content := []byte(`
				swarming {
					milo_hostname: "ci.chromium.org"
					bbagent_package {
						package_name: "infra/tools/luci/bbagent"
						version: "git_revision:60be805bf35a766cdf7d80bdf0a066dce30691e8"
					}
				}
			`)
			So(validateSettingsCfg(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "(swarming / bbagent_package): package_name must end with '/${platform}'")
		})

		Convey("invalid builders.regex in experiments", func() {
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
			So(validateSettingsCfg(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "(experiment.experiments #0): builders.regex \"(no right parenthesis\": invalid regex")
		})

		Convey("wrong minimum_value in experiments", func() {
			content := []byte(`
				experiment {
					experiments {
						name: "luci.buildbucket.bbagent_getbuild"
						minimum_value: 101
					}
				}
			`)
			So(validateSettingsCfg(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "minimum_value must be in [0,100]")
		})

		Convey("wrong default_value in experiments", func() {
			content := []byte(`
				experiment {
					experiments {
						name: "luci.buildbucket.bbagent_getbuild"
						default_value: 5
						minimum_value: 10
					}
				}
			`)
			So(validateSettingsCfg(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "default_value must be in [${minimum_value},100]")
		})

		Convey("invalid inactive in experiments", func() {
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
			So(validateSettingsCfg(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "default_value and minimum_value must both be 0 when inactive is true")
		})

		Convey("invalid backend", func() {
			content := []byte(`
				backends {
					target: "swarming://chromium-swarm"
					hostname: "swarming://chromium-swarm"
				}
			`)
			So(validateSettingsCfg(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "BackendSetting.hostname must not contain '://")
		})

		Convey("valid empty target backend", func() {
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
			So(validateSettingsCfg(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldBeNil)
		})

		Convey("invalid backend full mode", func() {
			Convey("build_sync_setting", func() {
				Convey("sync_interval_seconds", func() {
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
					So(validateSettingsCfg(vctx, configSet, path, content), ShouldBeNil)
					So(vctx.Finalize().Error(), ShouldContainSubstring, "sync_interval_seconds must be greater than or equal to 60")
				})
				Convey("shards", func() {
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
					So(validateSettingsCfg(vctx, configSet, path, content), ShouldBeNil)
					So(vctx.Finalize().Error(), ShouldContainSubstring, "shards must be greater than or equal to 0")
				})
			})
			Convey("pubsub_id", func() {
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
				So(validateSettingsCfg(vctx, configSet, path, content), ShouldBeNil)
				So(vctx.Finalize().Error(), ShouldContainSubstring, "pubsub_id for UpdateBuildTask must be specified")
			})

			Convey("empty", func() {
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
				So(validateSettingsCfg(vctx, configSet, path, content), ShouldBeNil)
				So(vctx.Finalize().Error(), ShouldContainSubstring, "mode field is not set or its type is unsupported")
			})
		})

		Convey("invalid custom metrics", func() {
			Convey("invalid name", func() {
				content := []byte(`
							logdog {
								hostname: "logs.chromium.org"
							}
							resultdb {
								hostname: "results.api.cr.dev"
							}
							custom_metrics {
								name: "/chrome/infra/custom/builds/started/",
								metric_base: CUSTOM_BUILD_METRIC_BASE_STARTED,
							}
						`)
				So(validateSettingsCfg(vctx, configSet, path, content), ShouldBeNil)
				So(vctx.Finalize().Error(), ShouldContainSubstring, `invalid metric name "/chrome/infra/custom/builds/started/": doesn't match ^(/[a-zA-Z0-9_-]+)+$`)
			})

			Convey("name without / prefix", func() {
				content := []byte(`
							logdog {
								hostname: "logs.chromium.org"
							}
							resultdb {
								hostname: "results.api.cr.dev"
							}
							custom_metrics {
								name: "custom/builds/started/",
								metric_base: CUSTOM_BUILD_METRIC_BASE_STARTED,
							}
						`)
				So(validateSettingsCfg(vctx, configSet, path, content), ShouldBeNil)
				So(vctx.Finalize().Error(), ShouldContainSubstring, `invalid metric name "custom/builds/started/": must starts with "/"`)
			})

			Convey("bb reserved name", func() {
				content := []byte(`
							logdog {
								hostname: "logs.chromium.org"
							}
							resultdb {
								hostname: "results.api.cr.dev"
							}
							custom_metrics {
								name: "/chrome/infra/buildbucket/v2/builds/started",
								metric_base: CUSTOM_BUILD_METRIC_BASE_STARTED,
							}
						`)
				So(validateSettingsCfg(vctx, configSet, path, content), ShouldBeNil)
				So(vctx.Finalize().Error(), ShouldContainSubstring, `"/chrome/infra/buildbucket/v2/builds/started" is reserved by Buildbucket`)
			})

			Convey("duplicated names", func() {
				content := []byte(`
						logdog {
							hostname: "logs.chromium.org"
						}
						resultdb {
							hostname: "results.api.cr.dev"
						}
						custom_metrics {
							name: "/chrome/infra/custom/builds/started",
							metric_base: CUSTOM_BUILD_METRIC_BASE_STARTED,
						}
						custom_metrics {
							name: "/chrome/infra/custom/builds/started",
							metric_base: CUSTOM_BUILD_METRIC_BASE_STARTED,
						}
					`)
				So(validateSettingsCfg(vctx, configSet, path, content), ShouldBeNil)
				So(vctx.Finalize().Error(), ShouldContainSubstring, "duplicated name is not allowed: /chrome/infra/custom/builds/started")
			})

			Convey("invalid field", func() {
				content := []byte(`
						logdog {
							hostname: "logs.chromium.org"
						}
						resultdb {
							hostname: "results.api.cr.dev"
						}
						custom_metrics {
							name: "/chrome/infra/custom/builds/started",
							fields: "$status",
							metric_base: CUSTOM_BUILD_METRIC_BASE_STARTED,
						}
					`)
				So(validateSettingsCfg(vctx, configSet, path, content), ShouldBeNil)
				So(vctx.Finalize().Error(), ShouldContainSubstring, `invalid field name "$status": doesn't match ^[A-Za-z_][A-Za-z0-9_]*$`)
			})

			Convey("duplicated field", func() {
				content := []byte(`
						logdog {
							hostname: "logs.chromium.org"
						}
						resultdb {
							hostname: "results.api.cr.dev"
						}
						custom_metrics {
							name: "/chrome/infra/custom/builds/started",
							fields: "status",
							fields: "status",
							metric_base: CUSTOM_BUILD_METRIC_BASE_STARTED,
						}
					`)
				So(validateSettingsCfg(vctx, configSet, path, content), ShouldBeNil)
				So(vctx.Finalize().Error(), ShouldContainSubstring, `"status" is duplicated`)
			})
		})

		Convey("OK", func() {
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
					fields: "status",
					fields: "branch",
					metric_base: CUSTOM_BUILD_METRIC_BASE_STARTED,
				}
				custom_metrics {
					name: "/chrome/infra/custom/builds/completed",
					fields: "status",
					fields: "final_step",
					metric_base: CUSTOM_BUILD_METRIC_BASE_COMPLETED,
				}
			`
			So(validateSettingsCfg(vctx, configSet, path, []byte(okCfg)), ShouldBeNil)
			So(vctx.Finalize(), ShouldBeNil)
		})
	})
}
