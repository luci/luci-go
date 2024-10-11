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
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"google.golang.org/api/googleapi"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/internal/clients"
	"go.chromium.org/luci/buildbucket/appengine/model"
	modeldefs "go.chromium.org/luci/buildbucket/appengine/model/defs"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestValidateProject(t *testing.T) {
	t.Parallel()

	ftt.Run("validate buildbucket cfg", t, func(t *ftt.Test) {
		vctx := &validation.Context{
			Context: memory.Use(context.Background()),
		}
		configSet := "projects/test"
		path := "cr-buildbucket.cfg"
		settingsCfg := &pb.SettingsCfg{}
		assert.Loosely(t, SetTestSettingsCfg(vctx.Context, settingsCfg), should.BeNil)

		t.Run("OK", func(t *ftt.Test) {
			var okCfg = `
				buckets {
					name: "good.name"
				}
				buckets {
					name: "good.name2"
				}
			`
			assert.Loosely(t, validateProjectCfg(vctx, configSet, path, []byte(okCfg)), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
		})

		t.Run("bad proto", func(t *ftt.Test) {
			content := []byte(` bad: "bad" `)
			assert.Loosely(t, validateProjectCfg(vctx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("invalid BuildbucketCfg proto message"))
		})

		t.Run("empty cr-buildbucket.cfg", func(t *ftt.Test) {
			content := []byte(` `)
			assert.Loosely(t, validateProjectCfg(vctx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
		})

		t.Run("fail", func(t *ftt.Test) {
			var badCfg = `
				buckets {
					name: "a"
				}
				buckets {
					name: "a"
				}
				buckets {}
				buckets { name: "luci.x" }
			`
			assert.Loosely(t, validateProjectCfg(vctx, configSet, path, []byte(badCfg)), should.BeNil)
			ve, ok := vctx.Finalize().(*validation.Error)
			assert.Loosely(t, ok, should.Equal(true))
			assert.Loosely(t, len(ve.Errors), should.Equal(3))
			assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring("(buckets #1 - a): duplicate bucket name \"a\""))
			assert.Loosely(t, ve.Errors[1].Error(), should.ContainSubstring("(buckets #2 - ): invalid name \"\": bucket name is not specified"))
			assert.Loosely(t, ve.Errors[2].Error(), should.ContainSubstring("(buckets #3 - luci.x): invalid name \"luci.x\": must start with 'luci.test.' because it starts with 'luci.' and is defined in the \"test\" project"))
		})

		t.Run("buckets unsorted", func(t *ftt.Test) {
			badCfg := `
				buckets { name: "c" }
				buckets { name: "b" }
				buckets { name: "a" }
			`
			assert.Loosely(t, validateProjectCfg(vctx, configSet, path, []byte(badCfg)), should.BeNil)
			ve, ok := vctx.Finalize().(*validation.Error)
			assert.Loosely(t, ok, should.Equal(true))
			warnings := ve.WithSeverity(validation.Warning).(errors.MultiError)
			assert.Loosely(t, warnings[0].Error(), should.ContainSubstring("bucket \"b\" out of order"))
			assert.Loosely(t, warnings[1].Error(), should.ContainSubstring("bucket \"a\" out of order"))
		})

		t.Run("swarming and dynamic_builder_template co-exist", func(t *ftt.Test) {
			badCfg := `
				buckets {
					name: "a"
					swarming: {}
					dynamic_builder_template: {}
				}
			`
			assert.Loosely(t, validateProjectCfg(vctx, configSet, path, []byte(badCfg)), should.BeNil)
			ve, ok := vctx.Finalize().(*validation.Error)
			assert.Loosely(t, ok, should.Equal(true))
			assert.Loosely(t, len(ve.Errors), should.Equal(1))
			assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring("mutually exclusive fields swarming and dynamic_builder_template both exist in bucket \"a\""))
		})

		t.Run("dynamic_builder_template", func(t *ftt.Test) {
			t.Run("builder name", func(t *ftt.Test) {
				var badCfg = `
					buckets {
						name: "a"
						dynamic_builder_template: {
							template: {
								name: "foo"
								swarming_host: "swarming_hostname"
							}
						}
					}
			`
				assert.Loosely(t, validateProjectCfg(vctx, configSet, path, []byte(badCfg)), should.BeNil)
				ve, ok := vctx.Finalize().(*validation.Error)
				assert.Loosely(t, ok, should.Equal(true))
				assert.Loosely(t, len(ve.Errors), should.Equal(1))
				assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring("builder name should not be set in a dynamic bucket"))
			})

			t.Run("shadow", func(t *ftt.Test) {
				var badCfg = `
					buckets {
						name: "a"
						shadow: "a.shadow"
						dynamic_builder_template: {
							template: {
								auto_builder_dimension: YES
								swarming_host: "swarming_hostname"
								shadow_builder_adjustments {
									pool: "shadow_pool"
								}
							}
						}
					}
			`
				assert.Loosely(t, validateProjectCfg(vctx, configSet, path, []byte(badCfg)), should.BeNil)
				ve, ok := vctx.Finalize().(*validation.Error)
				assert.Loosely(t, ok, should.Equal(true))
				assert.Loosely(t, len(ve.Errors), should.Equal(3))
				assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring(`dynamic bucket "a" cannot have a shadow bucket "a.shadow"`))
				assert.Loosely(t, ve.Errors[1].Error(), should.ContainSubstring("should not toggle on auto_builder_dimension in a dynamic bucket"))
				assert.Loosely(t, ve.Errors[2].Error(), should.ContainSubstring("cannot set shadow_builder_adjustments in a dynamic builder template"))
			})

			t.Run("empty builder", func(t *ftt.Test) {
				var badCfg = `
					buckets {
						name: "a"
						dynamic_builder_template: {
							template: {}
						}
					}
				`
				settingsCfg := &pb.SettingsCfg{Backends: []*pb.BackendSetting{}}
				_ = SetTestSettingsCfg(vctx.Context, settingsCfg)

				assert.Loosely(t, validateProjectCfg(vctx, configSet, path, []byte(badCfg)), should.BeNil)
				ve, ok := vctx.Finalize().(*validation.Error)
				assert.Loosely(t, ok, should.Equal(true))
				assert.Loosely(t, len(ve.Errors), should.Equal(1))
				assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring("either swarming host or task backend must be set"))
			})

			t.Run("empty dynamic_builder_template", func(t *ftt.Test) {
				var goodCfg = `
					buckets {
						name: "a"
						dynamic_builder_template: {
						}
					}
				`
				settingsCfg := &pb.SettingsCfg{Backends: []*pb.BackendSetting{}}
				_ = SetTestSettingsCfg(vctx.Context, settingsCfg)

				assert.Loosely(t, validateProjectCfg(vctx, configSet, path, []byte(goodCfg)), should.BeNil)
				assert.Loosely(t, vctx.Finalize(), should.BeNil)
			})

			t.Run("valid", func(t *ftt.Test) {
				var goodCfg = `
					buckets {
						name: "a"
						dynamic_builder_template: {
							template: {
								backend: {
									target: "lite://foo-lite"
								}
							}
						}
					}
				`
				settingsCfg := &pb.SettingsCfg{Backends: []*pb.BackendSetting{
					{
						Target:   "lite://foo-lite",
						Hostname: "foo_hostname",
					},
				}}
				_ = SetTestSettingsCfg(vctx.Context, settingsCfg)

				assert.Loosely(t, validateProjectCfg(vctx, configSet, path, []byte(goodCfg)), should.BeNil)
				assert.Loosely(t, vctx.Finalize(), should.BeNil)
			})
		})
	})

	ftt.Run("validate project_config.Swarming", t, func(t *ftt.Test) {
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockBackend := clients.NewMockTaskBackendClient(ctl)
		ctx := context.Background()
		ctx = context.WithValue(ctx, clients.MockTaskBackendClientKey, mockBackend)
		vctx := &validation.Context{
			Context: memory.Use(ctx),
		}
		wellKnownExperiments := stringset.NewFromSlice("luci.well_known")
		toBBSwarmingCfg := func(content string) *pb.Swarming {
			cfg := pb.Swarming{}
			err := prototext.Unmarshal([]byte(content), &cfg)
			assert.Loosely(t, err, should.BeNil)
			return &cfg
		}
		t.Run("OK", func(t *ftt.Test) {
			content := `
				builders {
					name: "release"
					swarming_host: "example.com"
					dimensions: "os:Linux"
					dimensions: "cpu:x86-64"
					dimensions: "cores:8"
					dimensions: "60:cores:64"
					service_account: "robot@example.com"
					caches {
						name: "git_chromium"
						path: "git_cache"
					}
					recipe {
						name: "foo"
						cipd_package: "infra/recipe_bundle"
						cipd_version: "refs/heads/main"
						properties: "a:b'"
						properties_j: "x:true"
					}
				}
				builders {
					name: "custom exe"
					swarming_host: "example.com"
					dimensions: "os:Linux"
					service_account: "robot@example.com"
					caches {
						name: "git_chromium"
						path: "git_cache"
					}
					exe {
						cipd_package: "infra/executable/foo"
						cipd_version: "refs/heads/main"
					}
					properties: '{"a":"b","x":true}'
					resultdb {
						enable: true
						history_options {
							use_invocation_timestamp: true
						}
					}
				}
				builders {
					name: "another custom exe"
					swarming_host: "example.com"
					dimensions: "os:Linux"
					service_account: "robot@example.com"
					caches {
						name: "git_chromium"
						path: "git_cache"
					}
					exe {
						cipd_package: "infra/executable/bar"
						cipd_version: "refs/heads/main"
					}
					properties: "{}"
				}
				builders {
					name: "release cipd"
					swarming_host: "example.com"
					recipe {
						cipd_package: "some/package"
						name: "foo"
					}
					shadow_builder_adjustments {
						pool: "shadow_pool"
						properties: '{"a":"b","x":true}'
						dimensions: "pool:shadow_pool"
						dimensions: "allowempty:"
					}
				}
			`
			validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "", nil)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
		})

		t.Run("shadow_builder_adjustments", func(t *ftt.Test) {
			t.Run("properties", func(t *ftt.Test) {
				content := `
								builders {
					name: "release cipd"
					swarming_host: "example.com"
					recipe {
						cipd_package: "some/package"
						name: "foo"
					}
					shadow_builder_adjustments {
						properties: "a:b'"
					}
				}
			`
				validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "", nil)
				ve, ok := vctx.Finalize().(*validation.Error)
				assert.Loosely(t, ok, should.Equal(true))
				assert.Loosely(t, len(ve.Errors), should.Equal(1))
				assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring("(swarming / builders #0 - release cipd / shadow_builder_adjustments): properties is not a JSON object"))
			})
			t.Run("set pool without setting dimensions is allowed", func(t *ftt.Test) {
				content := `
								builders {
					name: "release cipd"
					swarming_host: "example.com"
					recipe {
						cipd_package: "some/package"
						name: "foo"
					}
					shadow_builder_adjustments {
						pool: "shadow_pool"
					}
				}
			`
				validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "", nil)
				ve, ok := vctx.Finalize().(*validation.Error)
				assert.Loosely(t, ok, should.Equal(true))
				assert.Loosely(t, len(ve.Errors), should.Equal(1))
				assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring("(swarming / builders #0 - release cipd / shadow_builder_adjustments): dimensions.pool must be consistent with pool"))
			})
			t.Run("set dimensions without setting pool", func(t *ftt.Test) {
				content := `
								builders {
					name: "release cipd"
					swarming_host: "example.com"
					recipe {
						cipd_package: "some/package"
						name: "foo"
					}
					shadow_builder_adjustments {
						dimensions: "pool:shadow_pool"
					}
				}
			`
				validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "", nil)
				ve, ok := vctx.Finalize().(*validation.Error)
				assert.Loosely(t, ok, should.Equal(true))
				assert.Loosely(t, len(ve.Errors), should.Equal(1))
				assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring("(swarming / builders #0 - release cipd / shadow_builder_adjustments): dimensions.pool must be consistent with pool"))
			})
			t.Run("pool and dimensions different value", func(t *ftt.Test) {
				content := `
								builders {
					name: "release cipd"
					swarming_host: "example.com"
					recipe {
						cipd_package: "some/package"
						name: "foo"
					}
					shadow_builder_adjustments {
						pool: "shadow_pool"
						dimensions: "pool:another_pool"
					}
				}
			`
				validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "", nil)
				ve, ok := vctx.Finalize().(*validation.Error)
				assert.Loosely(t, ok, should.Equal(true))
				assert.Loosely(t, len(ve.Errors), should.Equal(1))
				assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring("(swarming / builders #0 - release cipd / shadow_builder_adjustments): dimensions.pool must be consistent with pool"))
			})
			t.Run("same key dimensions", func(t *ftt.Test) {
				content := `
								builders {
					name: "release cipd"
					swarming_host: "example.com"
					recipe {
						cipd_package: "some/package"
						name: "foo"
					}
					shadow_builder_adjustments {
						dimensions: "dup:v1"
						dimensions: "dup:v2"
						dimensions: "conflict:v1"
						dimensions: "conflict:"
					}
				}
			`
				validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "", nil)
				ve, ok := vctx.Finalize().(*validation.Error)
				assert.Loosely(t, ok, should.Equal(true))
				assert.Loosely(t, len(ve.Errors), should.Equal(1))
				assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring(`(swarming / builders #0 - release cipd / shadow_builder_adjustments): dimensions contain both empty and non-empty value for the same key - "conflict"`))
			})
		})

		t.Run("empty builders", func(t *ftt.Test) {
			content := `builders {}`
			validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "", nil)
			ve, ok := vctx.Finalize().(*validation.Error)
			assert.Loosely(t, ok, should.Equal(true))
			assert.Loosely(t, len(ve.Errors), should.Equal(3))
			assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring("(swarming / builders #0 - ): name must match "+builderRegex.String()))
			assert.Loosely(t, ve.Errors[1].Error(), should.ContainSubstring("(swarming / builders #0 - ): either swarming host or task backend must be set"))
			assert.Loosely(t, ve.Errors[2].Error(), should.ContainSubstring("(swarming / builders #0 - ): exactly one of exe or recipe must be specified"))
		})

		t.Run("bad builders cfg 1", func(t *ftt.Test) {
			content := `
				builders {
					name: "both"
					swarming_host: "example.com"
					exe {
						cipd_package: "infra/executable"
					}
					recipe {
						name: "foo"
						cipd_package: "infra/recipe_bundle"
					}
				}
				builders {
					name: "bad exe"
					swarming_host: "example.com"
					exe {
						cipd_version: "refs/heads/main"
					}
				}
				builders {
					name: "non json properties"
					swarming_host: "example.com"
					exe {
						cipd_package: "infra/executable"
					}
					properties: "{1:2}"
				}
				builders {
					name: "non dict properties"
					swarming_host: "example.com"
					exe {
						cipd_package: "infra/executable"
					}
					properties: "[]"
				}
				builders {
					name: "bad recipe"
					swarming_host: "example.com"
					recipe {
						cipd_version: "refs/heads/master"
					}
				}
				builders {
					name: "recipe and properties"
					swarming_host: "example.com"
					recipe {
						name: "foo"
						cipd_package: "infra/recipe_bundle"
					}
					properties: "{}"
				}
				builders {
					name: "very_long_name_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
								"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
								"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
					swarming_host: "example.com"
					recipe {
						cipd_package: "some/package"
						name: "foo"
					}
				}
				builders {
					name: "bad resultdb"
					swarming_host: "example.com"
					exe {
						cipd_package: "infra/executable/bar"
						cipd_version: "refs/heads/main"
					}
					properties: "{}"
					resultdb {
						enable: true
						history_options {
							commit{
								position: 123
							}
						}
						bq_exports {
							project: "project"
							dataset: "dataset"
							table: "table"
							test_results {}
						}
						bq_exports {
						}
					}
				}
			`
			validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "", nil)
			ve, ok := vctx.Finalize().(*validation.Error)
			assert.Loosely(t, ok, should.Equal(true))
			assert.Loosely(t, len(ve.Errors), should.Equal(10))
			assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring("(swarming / builders #0 - both): exactly one of exe or recipe must be specified"))
			assert.Loosely(t, ve.Errors[1].Error(), should.ContainSubstring("(swarming / builders #1 - bad exe): exe.cipd_package: unspecified"))
			assert.Loosely(t, ve.Errors[2].Error(), should.ContainSubstring("(swarming / builders #2 - non json properties): properties is not a JSON object"))
			assert.Loosely(t, ve.Errors[3].Error(), should.ContainSubstring("(swarming / builders #3 - non dict properties): properties is not a JSON object"))
			assert.Loosely(t, ve.Errors[4].Error(), should.ContainSubstring("(swarming / builders #4 - bad recipe / recipe): name: unspecified"))
			assert.Loosely(t, ve.Errors[5].Error(), should.ContainSubstring("(swarming / builders #4 - bad recipe / recipe): cipd_package: unspecified"))
			assert.Loosely(t, ve.Errors[6].Error(), should.ContainSubstring("(swarming / builders #5 - recipe and properties): recipe and properties cannot be set together"))
			assert.Loosely(t, ve.Errors[7].Error(), should.ContainSubstring("name must match ^[a-zA-Z0-9\\-_.\\(\\) ]{1,128}$"))
			assert.Loosely(t, ve.Errors[8].Error(), should.ContainSubstring("(swarming / builders #7 - bad resultdb): resultdb.history_options.commit must be unset"))
			assert.Loosely(t, ve.Errors[9].Error(), should.ContainSubstring("(swarming / builders #7 - bad resultdb): error validating resultdb.bq_exports[1]"))
		})

		t.Run("bad builders cfg 2", func(t *ftt.Test) {
			content := `
				task_template_canary_percentage { value: 102 }
				builders {
					name: "meep"
					swarming_host: "example.com"
					recipe {
						cipd_package: "some/package"
						name: "foo"
					}
				}
				builders {
					name: "meep"
					swarming_host: "example.com"
					recipe {
						cipd_package: "some/package"
						name: "foo"
					}
					swarming_tags: "wrong"
				}
				builders {
					name: "another"
					swarming_host: "example.com"
					service_account: "not an email"
					recipe {
						cipd_package: "some/package"
						name: "foo"
					}
					priority: 300
				}
			`
			validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "", nil)
			ve, ok := vctx.Finalize().(*validation.Error)
			assert.Loosely(t, ok, should.Equal(true))
			assert.Loosely(t, len(ve.Errors), should.Equal(5))
			assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring("task_template_canary_percentage.value must must be in [0, 100]"))
			assert.Loosely(t, ve.Errors[1].Error(), should.ContainSubstring("(swarming / builders #1 - meep / swarming_tags #0): Deprecated. Used only to enable \"vpython:native-python-wrapper\""))
			assert.Loosely(t, ve.Errors[2].Error(), should.ContainSubstring("(swarming / builders #1 - meep): name: duplicate"))
			assert.Loosely(t, ve.Errors[3].Error(), should.ContainSubstring("priority: must be in [20, 255] range; got 300"))
			assert.Loosely(t, ve.Errors[4].Error(), should.ContainSubstring(`service_account "not an email" doesn't match "^[0-9a-zA-Z_\\-\\.\\+\\%]+@[0-9a-zA-Z_\\-\\.]+$"`))
		})

		t.Run("bad caches in builders cfg", func(t *ftt.Test) {
			content := `
				builders {
					name: "b1"
					swarming_host: "example.com"
					caches {}
					caches { name: "a/b" path: "a" }
					caches { name: "b" path: "a\\c" }
					caches { name: "c" path: "a/.." }
					caches { name: "d" path: "/a" }
					exe {
						cipd_package: "infra/executable/bar"
						cipd_version: "refs/heads/main"
					}
					properties: "{}"
				}
				builders {
					name: "rel"
					swarming_host: "swarming.example.com"
					caches { path: "a" name: "a" }
					caches { path: "a" name: "a" }
					exe {
						cipd_package: "infra/executable/bar"
						cipd_version: "refs/heads/main"
					}
					properties: "{}"
				}
				builders {
					name: "bad secs"
					swarming_host: "swarming.example.com"
					caches { path: "aa" name: "aa" wait_for_warm_cache_secs: 61 }
					caches { path: "bb" name: "bb" wait_for_warm_cache_secs: 59 }
					exe {
						cipd_package: "infra/executable/bar"
						cipd_version: "refs/heads/main"
					}
					properties: "{}"
				}
				builders {
					name: "many"
					swarming_host: "swarming.example.com"
					caches { path: "a" name: "a" wait_for_warm_cache_secs: 60 }
					caches { path: "b" name: "b" wait_for_warm_cache_secs: 120 }
					caches { path: "c" name: "c" wait_for_warm_cache_secs: 180 }
					caches { path: "d" name: "d" wait_for_warm_cache_secs: 240 }
					caches { path: "e" name: "e" wait_for_warm_cache_secs: 300 }
					caches { path: "f" name: "f" wait_for_warm_cache_secs: 360 }
					caches { path: "g" name: "g" wait_for_warm_cache_secs: 420 }
					caches { path: "h" name: "h" wait_for_warm_cache_secs: 480 }
					exe {
						cipd_package: "infra/executable/bar"
						cipd_version: "refs/heads/main"
					}
					properties: "{}"
				}
			`
			validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "", nil)
			ve, ok := vctx.Finalize().(*validation.Error)
			assert.Loosely(t, ok, should.Equal(true))
			assert.Loosely(t, len(ve.Errors), should.Equal(11))
			assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring("(swarming / builders #0 - b1 / caches #0): name: required"))
			assert.Loosely(t, ve.Errors[1].Error(), should.ContainSubstring("(swarming / builders #0 - b1 / caches #0 / path): required"))
			assert.Loosely(t, ve.Errors[2].Error(), should.ContainSubstring(`(swarming / builders #0 - b1 / caches #1): name: "a/b" does not match "^[a-z0-9_]+$"`))
			assert.Loosely(t, ve.Errors[3].Error(), should.ContainSubstring(`(swarming / builders #0 - b1 / caches #2 / path): cannot contain \. On Windows forward-slashes will be replaced with back-slashes.`))
			assert.Loosely(t, ve.Errors[4].Error(), should.ContainSubstring("(swarming / builders #0 - b1 / caches #3 / path): cannot contain '..'"))
			assert.Loosely(t, ve.Errors[5].Error(), should.ContainSubstring("(swarming / builders #0 - b1 / caches #4 / path): cannot start with '/'"))
			assert.Loosely(t, ve.Errors[6].Error(), should.ContainSubstring("(swarming / builders #1 - rel / caches #1): duplicate name"))
			assert.Loosely(t, ve.Errors[7].Error(), should.ContainSubstring("(swarming / builders #1 - rel / caches #1): duplicate path"))
			assert.Loosely(t, ve.Errors[8].Error(), should.ContainSubstring("(swarming / builders #2 - bad secs / caches #0): wait_for_warm_cache_secs must be rounded on 60 seconds"))
			assert.Loosely(t, ve.Errors[9].Error(), should.ContainSubstring("(swarming / builders #2 - bad secs / caches #1): wait_for_warm_cache_secs must be at least 60 seconds"))
			assert.Loosely(t, ve.Errors[10].Error(), should.ContainSubstring("(swarming / builders #3 - many): 'too many different (8) wait_for_warm_cache_secs values; max 7"))
		})

		t.Run("bad experiments in builders cfg", func(t *ftt.Test) {
			content := `
				builders {
					name: "b1"
					swarming_host: "example.com"
					experiments {
						key: "bad!"
						value: 105
					}
					experiments {
						key: "negative"
						value: -10
					}
					experiments {
						key: "my.cool.experiment"
						value: 10
					}
					experiments {
						key: "luci.bad"
						value: 10
					}
					experiments {
						key: "luci.well_known"
						value: 10
					}
					exe {
						cipd_package: "infra/executable/bar"
						cipd_version: "refs/heads/main"
					}
					properties: "{}"
				}
			`
			validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "", nil)
			ve, ok := vctx.Finalize().(*validation.Error)
			assert.Loosely(t, ok, should.Equal(true))
			assert.Loosely(t, len(ve.Errors), should.Equal(4))
			// Have to concatenate all error strings because experiments is Map and iteration over Map is non-deterministic.
			allErrs := fmt.Sprintf("%s\n%s\n%s\n%s", ve.Errors[0].Error(), ve.Errors[1].Error(), ve.Errors[2].Error(), ve.Errors[3].Error())
			assert.Loosely(t, allErrs, should.ContainSubstring(`(swarming / builders #0 - b1 / experiments "bad!"): does not match "^[a-z][a-z0-9_]*(?:\\.[a-z][a-z0-9_]*)*$"`))
			assert.Loosely(t, allErrs, should.ContainSubstring(`(swarming / builders #0 - b1 / experiments "bad!"): value must be in [0, 100]`))
			assert.Loosely(t, allErrs, should.ContainSubstring(`(swarming / builders #0 - b1 / experiments "negative"): value must be in [0, 100]`))
			assert.Loosely(t, allErrs, should.ContainSubstring(`(swarming / builders #0 - b1 / experiments "luci.bad"): unknown experiment has reserved prefix "luci."`))
		})

		t.Run("backend and swarming in builder", func(t *ftt.Test) {
			backendSettings := []*pb.BackendSetting{
				{
					Target:   "swarming://chromium-swarm",
					Hostname: "swarming_hostname",
				},
			}
			settingsCfg := &pb.SettingsCfg{Backends: backendSettings}
			_ = SetTestSettingsCfg(vctx.Context, settingsCfg)
			content := `
				builders {
					name: "b1"
					swarming_host: "example.com"
					exe {
						cipd_package: "infra/executable/bar"
						cipd_version: "refs/heads/main"
					}
					properties: "{}"
					backend: {
						target: "swarming://chromium-swarm"
					}
				}
			`
			validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "", nil)
			ve, ok := vctx.Finalize().(*validation.Error)
			assert.Loosely(t, ok, should.Equal(true))
			assert.Loosely(t, len(ve.Errors), should.Equal(1))
			assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring("only one of swarming host or task backend is allowed"))
		})

		t.Run("backend and no swarming in builder; valid config_json present", func(t *ftt.Test) {
			mockBackend.EXPECT().ValidateConfigs(gomock.Any(), gomock.Any()).Return(&pb.ValidateConfigsResponse{
				ConfigErrors: []*pb.ValidateConfigsResponse_ErrorDetail{},
			}, nil)
			backendSettings := []*pb.BackendSetting{
				{
					Target:   "swarming://chromium-swarm",
					Hostname: "swarming_hostname",
				},
			}
			settingsCfg := &pb.SettingsCfg{Backends: backendSettings}
			_ = SetTestSettingsCfg(vctx.Context, settingsCfg)
			content := `
				builders {
					name: "b1"
					exe {
						cipd_package: "infra/executable/bar"
						cipd_version: "refs/heads/main"
					}
					properties: "{}"
					backend: {
						target: "swarming://chromium-swarm"
						config_json: '{"a":"b","x":true}'
					}
				}
			`
			validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "myluciproject", nil)
			_, ok := vctx.Finalize().(*validation.Error)
			assert.Loosely(t, ok, should.Equal(false))
		})

		t.Run("backend, backend_alt and no swarming in builder", func(t *ftt.Test) {
			backendSettings := []*pb.BackendSetting{
				{
					Target:   "swarming://chromium-swarm",
					Hostname: "swarming_hostname",
				},
				{
					Target:   "swarming://chromium-swarm-alt",
					Hostname: "swarming_hostname-alt",
				},
			}
			settingsCfg := &pb.SettingsCfg{Backends: backendSettings}
			_ = SetTestSettingsCfg(vctx.Context, settingsCfg)
			content := `
				builders {
					name: "b1"
					exe {
						cipd_package: "infra/executable/bar"
						cipd_version: "refs/heads/main"
					}
					properties: "{}"
					backend: {
						target: "swarming://chromium-swarm"
					}
					backend_alt: {
						target: "swarming://chromium-swarm-alt"
					}
				}
			`
			validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "", nil)
			_, ok := vctx.Finalize().(*validation.Error)
			assert.Loosely(t, ok, should.Equal(false))
		})

		t.Run("no backend, backend_alt and no swarming in builder", func(t *ftt.Test) {
			backendSettings := []*pb.BackendSetting{
				{
					Target:   "swarming://chromium-swarm",
					Hostname: "swarming_hostname",
				},
				{
					Target:   "swarming://chromium-swarm-alt",
					Hostname: "swarming_hostname-alt",
				},
			}
			settingsCfg := &pb.SettingsCfg{Backends: backendSettings}
			_ = SetTestSettingsCfg(vctx.Context, settingsCfg)
			content := `
				builders {
					name: "b1"
					exe {
						cipd_package: "infra/executable/bar"
						cipd_version: "refs/heads/main"
					}
					properties: "{}"
					backend_alt: {
						target: "swarming://chromium-swarm-alt"
					}
				}
			`
			validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "", nil)
			ve, ok := vctx.Finalize().(*validation.Error)
			assert.Loosely(t, ok, should.Equal(true))
			assert.Loosely(t, len(ve.Errors), should.Equal(1))
			assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring("either swarming host or task backend must be set"))
		})

		t.Run("backend and no swarming in builder; invalid config_json present", func(t *ftt.Test) {
			mockBackend.EXPECT().ValidateConfigs(gomock.Any(), gomock.Any()).Return(&pb.ValidateConfigsResponse{
				ConfigErrors: []*pb.ValidateConfigsResponse_ErrorDetail{
					{
						Index: 1,
						Error: "really bad error",
					},
					{
						Index: 1,
						Error: "the worst possible error",
					},
				},
			}, nil)
			backendSettings := []*pb.BackendSetting{
				{
					Target:   "swarming://chromium-swarm",
					Hostname: "swarming_hostname",
				},
			}
			settingsCfg := &pb.SettingsCfg{Backends: backendSettings}
			_ = SetTestSettingsCfg(vctx.Context, settingsCfg)
			content := `
				builders {
					name: "b1"
					exe {
						cipd_package: "infra/executable/bar"
						cipd_version: "refs/heads/main"
					}
					properties: "{}"
					backend: {
						target: "swarming://chromium-swarm"
						config_json: '{"a":"b","x":true}'
					}
				}
			`
			validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "myluciproject", nil)
			ve, ok := vctx.Finalize().(*validation.Error)
			assert.Loosely(t, ok, should.Equal(true))
			assert.Loosely(t, len(ve.Errors), should.Equal(2))
			assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring("error validating task backend ConfigJson at index 1: really bad error"))
			assert.Loosely(t, ve.Errors[1].Error(), should.ContainSubstring("error validating task backend ConfigJson at index 1: the worst possible error"))
		})

		t.Run("backend and no swarming in builder; error validating config_json", func(t *ftt.Test) {
			mockBackend.EXPECT().ValidateConfigs(gomock.Any(), gomock.Any()).Return(nil, &googleapi.Error{Code: 400})
			backendSettings := []*pb.BackendSetting{
				{
					Target:   "swarming://chromium-swarm",
					Hostname: "swarming_hostname",
				},
			}
			settingsCfg := &pb.SettingsCfg{Backends: backendSettings}
			_ = SetTestSettingsCfg(vctx.Context, settingsCfg)
			content := `
				builders {
					name: "b1"
					exe {
						cipd_package: "infra/executable/bar"
						cipd_version: "refs/heads/main"
					}
					properties: "{}"
					backend: {
						target: "swarming://chromium-swarm"
						config_json: '{"a":"b","x":true}'
					}
				}
			`
			validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "myluciproject", nil)
			ve, ok := vctx.Finalize().(*validation.Error)
			assert.Loosely(t, ok, should.Equal(true))
			assert.Loosely(t, len(ve.Errors), should.Equal(1))
			assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring("error validating task backend ConfigJson: googleapi: got HTTP response code 400 "))
		})

		t.Run("no config_json allowed for TaskBackendLite", func(t *ftt.Test) {
			settingsCfg := &pb.SettingsCfg{Backends: []*pb.BackendSetting{
				{
					Target:   "lite://foo-lite",
					Hostname: "foo_hostname",
					Mode: &pb.BackendSetting_LiteMode_{
						LiteMode: &pb.BackendSetting_LiteMode{},
					},
				},
			}}
			_ = SetTestSettingsCfg(vctx.Context, settingsCfg)

			content := `
				builders {
					name: "bar"
					exe {
						cipd_package: "infra/executable/bar"
						cipd_version: "refs/heads/main"
					}
					properties: "{}"
					backend: {
						target: "lite://foo-lite"
						config_json: '{"a":"b","x":true}'
					}
				}
			`
			validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "myluciproject", settingsCfg)
			ve, ok := vctx.Finalize().(*validation.Error)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring("no config_json allowed for TaskBackendLite"))
		})

		t.Run("hearbeat_timeout", func(t *ftt.Test) {
			settingsCfg := &pb.SettingsCfg{Backends: []*pb.BackendSetting{
				{
					Target:   "lite://foo-lite",
					Hostname: "foo_hostname",
					Mode: &pb.BackendSetting_LiteMode_{
						LiteMode: &pb.BackendSetting_LiteMode{},
					},
				},
				{
					Target:   "swarming://chromium-swarm",
					Hostname: "swarming_hostname",
					Mode: &pb.BackendSetting_FullMode_{
						FullMode: &pb.BackendSetting_FullMode{
							PubsubId: "pubsub",
						},
					},
				},
			}}
			_ = SetTestSettingsCfg(vctx.Context, settingsCfg)

			t.Run("backend is TaskBackendLite", func(t *ftt.Test) {
				content := `
				builders {
					name: "bar"
					exe {
						cipd_package: "infra/executable/bar"
						cipd_version: "refs/heads/main"
					}
					properties: "{}"
					backend: {
						target: "lite://foo-lite"
					}
					heartbeat_timeout_secs: 30
				}
			`
				validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "myluciproject", settingsCfg)
				_, ok := vctx.Finalize().(*validation.Error)
				assert.Loosely(t, ok, should.BeFalse)
			})

			t.Run("backend_alt is TaskBackendLite", func(t *ftt.Test) {
				content := `
				builders {
					name: "bar"
					exe {
						cipd_package: "infra/executable/bar"
						cipd_version: "refs/heads/main"
					}
					properties: "{}"
					swarming_host: "swarming_hostname"
					backend_alt: {
						target: "lite://foo-lite"
					}
					heartbeat_timeout_secs: 30
				}
			`
				validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "myluciproject", settingsCfg)
				_, ok := vctx.Finalize().(*validation.Error)
				assert.Loosely(t, ok, should.BeFalse)
			})
			t.Run("No backend is TaskBackendLite", func(t *ftt.Test) {
				content := `
				builders {
					name: "bar"
					exe {
						cipd_package: "infra/executable/bar"
						cipd_version: "refs/heads/main"
					}
					properties: "{}"
					backend: {
						target: "swarming://chromium-swarm"
					}
					heartbeat_timeout_secs: 30
				}
			`
				validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "myluciproject", settingsCfg)
				ve, ok := vctx.Finalize().(*validation.Error)
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, len(ve.Errors), should.Equal(1))
				assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring("heartbeat_timeout_secs should only be set for builders using a TaskBackendLite backend"))
			})
		})

		t.Run("timeout", func(t *ftt.Test) {
			content := `
				builders {
					name: "both default"
					swarming_host: "example.com"
					dimensions: "os:Linux"
					dimensions: "cpu:x86-64"
					dimensions: "cores:8"
					dimensions: "60:cores:64"
					service_account: "robot@example.com"
					caches {
						name: "git_chromium"
						path: "git_cache"
					}
					recipe {
						name: "foo"
						cipd_package: "infra/recipe_bundle"
						cipd_version: "refs/heads/main"
						properties: "a:b'"
						properties_j: "x:true"
					}
				}
				builders {
					name: "only execution_timeout"
					swarming_host: "example.com"
					dimensions: "os:Linux"
					service_account: "robot@example.com"
					caches {
						name: "git_chromium"
						path: "git_cache"
					}
					exe {
						cipd_package: "infra/executable/foo"
						cipd_version: "refs/heads/main"
					}
					properties: '{"a":"b","x":true}'
					resultdb {
						enable: true
						history_options {
							use_invocation_timestamp: true
						}
					}
					execution_timeout_secs: 432000
				}
				builders {
					name: "only expiration_secs"
					swarming_host: "example.com"
					dimensions: "os:Linux"
					service_account: "robot@example.com"
					caches {
						name: "git_chromium"
						path: "git_cache"
					}
					exe {
						cipd_package: "infra/executable/bar"
						cipd_version: "refs/heads/main"
					}
					properties: "{}"
					expiration_secs: 432000
				}
				builders {
					name: "both specified"
					swarming_host: "example.com"
					recipe {
						cipd_package: "some/package"
						name: "foo"
					}
					shadow_builder_adjustments {
						pool: "shadow_pool"
						properties: '{"a":"b","x":true}'
						dimensions: "pool:shadow_pool"
						dimensions: "allowempty:"
					}
					expiration_secs: 432000
					execution_timeout_secs: 172800
				}
			`
			validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "", nil)
			ve, ok := vctx.Finalize().(*validation.Error)
			assert.Loosely(t, ok, should.Equal(true))
			assert.Loosely(t, len(ve.Errors), should.Equal(3))
			assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring("execution_timeout_secs 432000 + (default) expiration_secs 21600 exceeds max build completion time 432000"))
			assert.Loosely(t, ve.Errors[1].Error(), should.ContainSubstring("(default) execution_timeout_secs 10800 + expiration_secs 432000 exceeds max build completion time 432000"))
			assert.Loosely(t, ve.Errors[2].Error(), should.ContainSubstring("execution_timeout_secs 172800 + expiration_secs 432000 exceeds max build completion time 432000"))
		})

		t.Run("custom metrics", func(t *ftt.Test) {
			settingsCfg := &pb.SettingsCfg{
				CustomMetrics: []*pb.CustomMetric{
					{
						Name: "chrome/infra/custom/builds/started",
						Class: &pb.CustomMetric_MetricBase{
							MetricBase: pb.CustomMetricBase_CUSTOM_METRIC_BASE_STARTED,
						},
						ExtraFields: []string{"os"},
					},
					{
						Name: "chrome/infra/custom/builds/completed",
						Class: &pb.CustomMetric_MetricBase{
							MetricBase: pb.CustomMetricBase_CUSTOM_METRIC_BASE_COMPLETED,
						},
						ExtraFields: []string{"os"},
					},
					{
						Name: "chrome/infra/custom/builds/count",
						Class: &pb.CustomMetric_MetricBase{
							MetricBase: pb.CustomMetricBase_CUSTOM_METRIC_BASE_COUNT,
						},
						ExtraFields: []string{"os"},
					},
				},
			}
			_ = SetTestSettingsCfg(vctx.Context, settingsCfg)

			t.Run("metric name empty", func(t *ftt.Test) {
				content := `
					builders {
						name: "both default"
						swarming_host: "example.com"
						dimensions: "os:Linux"
						dimensions: "cpu:x86-64"
						dimensions: "cores:8"
						dimensions: "60:cores:64"
						service_account: "robot@example.com"
						caches {
							name: "git_chromium"
							path: "git_cache"
						}
						recipe {
							name: "foo"
							cipd_package: "infra/recipe_bundle"
							cipd_version: "refs/heads/main"
							properties: "a:b'"
							properties_j: "x:true"
						}
						custom_metric_definitions {}
					}
				`
				validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "", settingsCfg)
				ve, ok := vctx.Finalize().(*validation.Error)
				assert.Loosely(t, ok, should.Equal(true))
				assert.Loosely(t, len(ve.Errors), should.Equal(1))
				assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring("name is required"))
			})

			t.Run("metric name not registered", func(t *ftt.Test) {
				content := `
					builders {
						name: "both default"
						swarming_host: "example.com"
						dimensions: "os:Linux"
						dimensions: "cpu:x86-64"
						dimensions: "cores:8"
						dimensions: "60:cores:64"
						service_account: "robot@example.com"
						caches {
							name: "git_chromium"
							path: "git_cache"
						}
						recipe {
							name: "foo"
							cipd_package: "infra/recipe_bundle"
							cipd_version: "refs/heads/main"
							properties: "a:b'"
							properties_j: "x:true"
						}
						custom_metric_definitions {
							name: "chrome/infra/not/registered"
						}
					}
				`
				validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "", settingsCfg)
				ve, ok := vctx.Finalize().(*validation.Error)
				assert.Loosely(t, ok, should.Equal(true))
				assert.Loosely(t, len(ve.Errors), should.Equal(1))
				assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring("not registered in Buildbucket service config"))
			})

			t.Run("metric predicates empty", func(t *ftt.Test) {
				content := `
					builders {
						name: "both default"
						swarming_host: "example.com"
						dimensions: "os:Linux"
						dimensions: "cpu:x86-64"
						dimensions: "cores:8"
						dimensions: "60:cores:64"
						service_account: "robot@example.com"
						caches {
							name: "git_chromium"
							path: "git_cache"
						}
						recipe {
							name: "foo"
							cipd_package: "infra/recipe_bundle"
							cipd_version: "refs/heads/main"
							properties: "a:b'"
							properties_j: "x:true"
						}
						custom_metric_definitions {
							name: "chrome/infra/custom/builds/started"
						}
					}
				`

				validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "", settingsCfg)
				ve, ok := vctx.Finalize().(*validation.Error)
				assert.Loosely(t, ok, should.Equal(true))
				assert.Loosely(t, len(ve.Errors), should.Equal(2))
				assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring("predicates are required"))
			})

			t.Run("metric predicates invalid", func(t *ftt.Test) {
				content := `
					builders {
						name: "both default"
						swarming_host: "example.com"
						dimensions: "os:Linux"
						dimensions: "cpu:x86-64"
						dimensions: "cores:8"
						dimensions: "60:cores:64"
						service_account: "robot@example.com"
						caches {
							name: "git_chromium"
							path: "git_cache"
						}
						recipe {
							name: "foo"
							cipd_package: "infra/recipe_bundle"
							cipd_version: "refs/heads/main"
							properties: "a:b'"
							properties_j: "x:true"
						}
						custom_metric_definitions {
							name: "chrome/infra/custom/builds/started"
							predicates: "not_a_bool_expression"
						}
					}
				`

				validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "", settingsCfg)
				ve, ok := vctx.Finalize().(*validation.Error)
				assert.Loosely(t, ok, should.Equal(true))
				assert.Loosely(t, len(ve.Errors), should.Equal(2))
				assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring("failed to generate CEL expression"))
			})

			t.Run("metric extra_fields empty", func(t *ftt.Test) {
				content := `
					builders {
						name: "both default"
						swarming_host: "example.com"
						dimensions: "os:Linux"
						dimensions: "cpu:x86-64"
						dimensions: "cores:8"
						dimensions: "60:cores:64"
						service_account: "robot@example.com"
						caches {
							name: "git_chromium"
							path: "git_cache"
						}
						recipe {
							name: "foo"
							cipd_package: "infra/recipe_bundle"
							cipd_version: "refs/heads/main"
							properties: "a:b'"
							properties_j: "x:true"
						}
						custom_metric_definitions {
							name: "chrome/infra/custom/builds/started"
							predicates: "build.tags.get_value(\"os\")!=\"\""
						}
					}
				`

				validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "", settingsCfg)
				ve, ok := vctx.Finalize().(*validation.Error)
				assert.Loosely(t, ok, should.Equal(true))
				assert.Loosely(t, len(ve.Errors), should.Equal(1))
				assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring(`field(s) ["os"] must be included`))
			})

			t.Run("metric fields invalid", func(t *ftt.Test) {
				content := `
					builders {
						name: "both default"
						swarming_host: "example.com"
						dimensions: "os:Linux"
						dimensions: "cpu:x86-64"
						dimensions: "cores:8"
						dimensions: "60:cores:64"
						service_account: "robot@example.com"
						caches {
							name: "git_chromium"
							path: "git_cache"
						}
						recipe {
							name: "foo"
							cipd_package: "infra/recipe_bundle"
							cipd_version: "refs/heads/main"
							properties: "a:b'"
							properties_j: "x:true"
						}
						custom_metric_definitions {
							name: "chrome/infra/custom/builds/started"
							predicates: "build.tags.get_value(\"os\")!=\"\""
							extra_fields {
								key: "os",
								value: "build.tags.get_value(\"os\")!=\"\"",
							}
						}
					}
				`

				validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "", settingsCfg)
				ve, ok := vctx.Finalize().(*validation.Error)
				assert.Loosely(t, ok, should.Equal(true))
				assert.Loosely(t, len(ve.Errors), should.Equal(1))
				assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring(`failed to generate CEL expression`))
			})

			t.Run("metric fields for builder metric", func(t *ftt.Test) {
				content := `
					builders {
						name: "both default"
						swarming_host: "example.com"
						dimensions: "os:Linux"
						dimensions: "cpu:x86-64"
						dimensions: "cores:8"
						dimensions: "60:cores:64"
						service_account: "robot@example.com"
						caches {
							name: "git_chromium"
							path: "git_cache"
						}
						recipe {
							name: "foo"
							cipd_package: "infra/recipe_bundle"
							cipd_version: "refs/heads/main"
							properties: "a:b'"
							properties_j: "x:true"
						}
						custom_metric_definitions {
							name: "chrome/infra/custom/builds/count"
							predicates: "build.tags.get_value(\"os\")!=\"\""
							extra_fields {
								key: "os",
								value: "build.tags.get_value(\"os\")",
							}
						}
					}
				`

				validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "", settingsCfg)
				ve, ok := vctx.Finalize().(*validation.Error)
				assert.Loosely(t, ok, should.Equal(true))
				assert.Loosely(t, len(ve.Errors), should.Equal(1))
				assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring(`custom builder metric cannot have extra_fields`))
			})

			t.Run("OK", func(t *ftt.Test) {
				content := `
					builders {
						name: "both default"
						swarming_host: "example.com"
						dimensions: "os:Linux"
						dimensions: "cpu:x86-64"
						dimensions: "cores:8"
						dimensions: "60:cores:64"
						service_account: "robot@example.com"
						caches {
							name: "git_chromium"
							path: "git_cache"
						}
						recipe {
							name: "foo"
							cipd_package: "infra/recipe_bundle"
							cipd_version: "refs/heads/main"
							properties: "a:b'"
							properties_j: "x:true"
						}
						custom_metric_definitions {
							name: "chrome/infra/custom/builds/started"
							predicates: "build.tags.get_value(\"os\")!=\"\""
							extra_fields {
								key: "os",
								value: "build.tags.get_value(\"os\")",
							}
							extra_fields {
								key: "additional",
								value: "build.tags.get_value(\"additional\")",
							}
						}
					}
				`

				validateProjectSwarming(vctx, toBBSwarmingCfg(content), wellKnownExperiments, "", settingsCfg)
				_, ok := vctx.Finalize().(*validation.Error)
				assert.Loosely(t, ok, should.Equal(false))
			})
		})
	})

	ftt.Run("validate dimensions", t, func(t *ftt.Test) {
		helper := func(expectedErr string, dimensions []string) {
			vctx := &validation.Context{
				Context: context.Background(),
			}
			validateDimensions(vctx, dimensions, false)
			if strings.HasPrefix(expectedErr, "ok") {
				assert.Loosely(t, vctx.Finalize(), should.BeNil)
			} else {
				assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring(expectedErr))
			}
		}

		testData := map[string][]string{
			"ok1": {"a:b"},
			"ok2": {"a:b1", "a:b2", "60:a:b3"},
			`ok3`: {"1814400:a:1"}, // 21*24*60*6
			`expiration_secs is outside valid range; up to 504h0m0s`:                                                     {"1814401:a:1"}, // 21*24*60*60+
			`(dimensions #0 - ""): "" does not have ':'`:                                                                 {""},
			`(dimensions #0 - "caches:a"): dimension key must not be 'caches'; caches must be declared via caches field`: {"caches:a"},
			`(dimensions #0 - ":"): missing key`:                                                                         {":"},
			`(dimensions #0 - "a.b:c"): key "a.b" does not match pattern "^[a-zA-Z\\_\\-]+$"`:                            {"a.b:c"},
			`(dimensions #0 - "0:"): missing key`:                                                                        {"0:"},
			`(dimensions #0 - "a:"): missing value`:                                                                      {"a:", "60:a:b"},
			`(dimensions #0 - "-1:a:1"): expiration_secs is outside valid range; up to 504h0m0s`:                         {"-1:a:1"},
			`(dimensions #0 - "1:a:b"): expiration_secs must be a multiple of 60 seconds`:                                {"1:a:b"},
			"at most 6 different expiration_secs values can be used": {
				"60:a:1",
				"120:a:1",
				"180:a:1",
				"240:a:1",
				"300:a:1",
				"360:a:1",
				"420:a:1",
			},
		}
		for expectedErr, dims := range testData {
			helper(expectedErr, dims)
		}
	})

	ftt.Run("validate builder recipe", t, func(t *ftt.Test) {
		vctx := &validation.Context{
			Context: context.Background(),
		}

		t.Run("ok", func(t *ftt.Test) {
			recipe := &pb.BuilderConfig_Recipe{
				Name:        "foo",
				CipdPackage: "infra/recipe_bundle",
				CipdVersion: "refs/heads/main",
				Properties:  []string{"a:b"},
				PropertiesJ: []string{"x:null", "y:true", "z:{\"zz\":true}"},
			}
			validateBuilderRecipe(vctx, recipe)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
		})

		t.Run("bad", func(t *ftt.Test) {
			recipe := &pb.BuilderConfig_Recipe{
				Properties:  []string{"", ":", "buildbucket:foobar", "x:y"},
				PropertiesJ: []string{"x:'y'", "y:b", "z"},
			}
			validateBuilderRecipe(vctx, recipe)
			ve := vctx.Finalize().(*validation.Error)
			assert.Loosely(t, len(ve.Errors), should.Equal(8))
			assert.Loosely(t, ve.Errors[0].Error(), should.ContainSubstring("name: unspecified"))
			assert.Loosely(t, ve.Errors[1].Error(), should.ContainSubstring("cipd_package: unspecified"))
			assert.Loosely(t, ve.Errors[2].Error(), should.ContainSubstring("(properties #0 - ): doesn't have a colon"))
			assert.Loosely(t, ve.Errors[3].Error(), should.ContainSubstring("(properties #1 - :): key not specified"))
			assert.Loosely(t, ve.Errors[4].Error(), should.ContainSubstring("(properties #2 - buildbucket:foobar): reserved property"))
			assert.Loosely(t, ve.Errors[5].Error(), should.ContainSubstring("(properties_j #0 - x:'y'): duplicate property"))
			assert.Loosely(t, ve.Errors[6].Error(), should.ContainSubstring("(properties_j #1 - y:b): not a JSON object"))
			assert.Loosely(t, ve.Errors[7].Error(), should.ContainSubstring("(properties_j #2 - z): doesn't have a colon"))
		})

		t.Run("bad $recipe_engine/runtime props", func(t *ftt.Test) {
			runtime := `$recipe_engine/runtime:{"is_luci": false,"is_experimental": true, "unrecognized_is_fine": 1}`
			recipe := &pb.BuilderConfig_Recipe{
				Name:        "foo",
				CipdPackage: "infra/recipe_bundle",
				CipdVersion: "refs/heads/main",
				PropertiesJ: []string{runtime},
			}
			validateBuilderRecipe(vctx, recipe)
			ve, ok := vctx.Finalize().(*validation.Error)
			assert.Loosely(t, ok, should.Equal(true))
			assert.Loosely(t, len(ve.Errors), should.Equal(2))
			allErrs := fmt.Sprintf("%s\n%s", ve.Errors[0].Error(), ve.Errors[1].Error())
			assert.Loosely(t, allErrs, should.ContainSubstring(`key "is_luci": reserved key`))
			assert.Loosely(t, allErrs, should.ContainSubstring(`key "is_experimental": reserved key`))
		})
	})

}

func TestUpdateProject(t *testing.T) {
	t.Parallel()

	// Strips the Proto field from each of the given *model.Bucket, returning a
	// slice whose ith index is the stripped *pb.Bucket value.
	// Needed because model.Bucket.Proto can only be compared with ShouldResembleProto
	// while model.Bucket can only be compared with ShouldResemble.
	stripBucketProtos := func(buckets []*model.Bucket) []*pb.Bucket {
		ret := make([]*pb.Bucket, len(buckets))
		for i, bkt := range buckets {
			if bkt == nil {
				ret[i] = nil
			} else {
				ret[i] = bkt.Proto
				bkt.Proto = nil
			}
		}
		return ret
	}
	stripBuilderProtos := func(buckets []*model.Builder) []*pb.BuilderConfig {
		ret := make([]*pb.BuilderConfig, len(buckets))
		for i, bldr := range buckets {
			if bldr == nil {
				ret[i] = nil
			} else {
				ret[i] = bldr.Config
				bldr.Config = nil
			}
		}
		return ret
	}

	ftt.Run("update", t, func(t *ftt.Test) {
		ctx := memory.UseWithAppID(context.Background(), "fake-cr-buildbucket")

		cfgClient := newFakeCfgClient()

		ctx = cfgclient.Use(ctx, cfgClient)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx = txndefer.FilterRDS(ctx)

		settingsCfg := &pb.SettingsCfg{}
		assert.Loosely(t, SetTestSettingsCfg(ctx, settingsCfg), should.BeNil)

		// Datastore is empty. Mimic the first time receiving configs and store all
		// of them into Datastore.
		assert.Loosely(t, UpdateProjectCfg(ctx), should.BeNil)
		var actualBkts []*model.Bucket
		assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.BucketKind), &actualBkts), should.BeNil)
		assert.Loosely(t, len(actualBkts), should.Equal(5))
		assert.Loosely(t, stripBucketProtos(actualBkts), should.Resemble([]*pb.Bucket{
			{
				Name: "master.tryserver.chromium.linux",
			},
			{
				Name: "master.tryserver.chromium.win",
			},
			{
				Name: "try",
				Swarming: &pb.Swarming{
					Builders:                     []*pb.BuilderConfig{},
					TaskTemplateCanaryPercentage: &wrapperspb.UInt32Value{Value: uint32(10)},
				},
			},
			{
				Name: "try",
				Swarming: &pb.Swarming{
					Builders: []*pb.BuilderConfig{},
				},
			},
			{
				Name: "master.tryserver.v8",
			},
		}))

		assert.Loosely(t, actualBkts, should.Resemble([]*model.Bucket{
			{
				ID:       "master.tryserver.chromium.linux",
				Parent:   model.ProjectKey(ctx, "chromium"),
				Bucket:   "master.tryserver.chromium.linux",
				Schema:   CurrentBucketSchemaVersion,
				Revision: "deadbeef",
			},
			{
				ID:       "master.tryserver.chromium.win",
				Parent:   model.ProjectKey(ctx, "chromium"),
				Bucket:   "master.tryserver.chromium.win",
				Schema:   CurrentBucketSchemaVersion,
				Revision: "deadbeef",
			},
			{
				ID:       "try",
				Parent:   model.ProjectKey(ctx, "chromium"),
				Bucket:   "try",
				Schema:   CurrentBucketSchemaVersion,
				Revision: "deadbeef",
			},
			{
				ID:       "try",
				Parent:   model.ProjectKey(ctx, "dart"),
				Bucket:   "try",
				Schema:   CurrentBucketSchemaVersion,
				Revision: "deadbeef",
			},
			{
				ID:       "master.tryserver.v8",
				Parent:   model.ProjectKey(ctx, "v8"),
				Bucket:   "master.tryserver.v8",
				Schema:   CurrentBucketSchemaVersion,
				Revision: "sha1:502558141dd8e90ed88de7f1bf3fa430d4128966",
			},
		}))

		var actualBuilders []*model.Builder
		assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind), &actualBuilders), should.BeNil)
		assert.Loosely(t, len(actualBuilders), should.Equal(2))
		expectedBuilder1 := &pb.BuilderConfig{
			Name:                         "linux",
			Dimensions:                   []string{"os:Linux", "pool:luci.chromium.try"},
			SwarmingHost:                 "swarming.example.com",
			TaskTemplateCanaryPercentage: &wrapperspb.UInt32Value{Value: uint32(10)},
			Exe: &pb.Executable{
				CipdPackage: "infra/recipe_bundle",
				CipdVersion: "refs/heads/main",
				Cmd:         []string{"luciexe"},
			},
		}
		expectedBuilder2 := &pb.BuilderConfig{
			Name:       "linux",
			Dimensions: []string{"pool:Dart.LUCI"},
			Exe: &pb.Executable{
				CipdPackage: "infra/recipe_bundle",
				CipdVersion: "refs/heads/main",
				Cmd:         []string{"luciexe"},
			},
		}
		expectedBldrHash1, _, _ := computeBuilderHash(expectedBuilder1)
		expectedBldrHash2, _, _ := computeBuilderHash(expectedBuilder2)
		assert.Loosely(t, stripBuilderProtos(actualBuilders), should.Resemble([]*pb.BuilderConfig{expectedBuilder1, expectedBuilder2}))
		assert.Loosely(t, actualBuilders, should.Resemble([]*model.Builder{
			{
				ID:         "linux",
				Parent:     model.BucketKey(ctx, "chromium", "try"),
				ConfigHash: expectedBldrHash1,
			},
			{
				ID:         "linux",
				Parent:     model.BucketKey(ctx, "dart", "try"),
				ConfigHash: expectedBldrHash2,
			},
		}))

		t.Run("with existing", func(t *ftt.Test) {
			// Add master.tryserver.chromium.mac
			// Update luci.chromium.try
			// Delete master.tryserver.chromium.win
			// Add shadow bucket try.shadow which shadows try
			cfgClient.chromiumBuildbucketCfg = `
			buckets {
				name: "master.tryserver.chromium.linux"
			}
			buckets {
				name: "master.tryserver.chromium.mac"
			}
			buckets {
				name: "try"
				swarming {
					task_template_canary_percentage { value: 10 }
					builders {
						name: "linux"
						swarming_host: "swarming.updated.example.com"
						task_template_canary_percentage { value: 10 }
						dimensions: "os:Linux"
						exe {
							cipd_version: "refs/heads/main"
							cipd_package: "infra/recipe_bundle"
							cmd: ["luciexe"]
						}
					}
				}
				shadow: "try.shadow"
			}
			buckets {
				name: "try.shadow"
				dynamic_builder_template {}
			}
			`
			cfgClient.chromiumRevision = "new!"
			// Delete the entire v8 cfg
			cfgClient.v8BuildbucketCfg = ""

			assert.Loosely(t, UpdateProjectCfg(ctx), should.BeNil)
			var actualBkts []*model.Bucket
			assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.BucketKind), &actualBkts), should.BeNil)
			assert.Loosely(t, len(actualBkts), should.Equal(5))
			assert.Loosely(t, stripBucketProtos(actualBkts), should.Resemble([]*pb.Bucket{
				{
					Name: "master.tryserver.chromium.linux",
				},
				{
					Name: "master.tryserver.chromium.mac",
				},
				{
					Name: "try",
					Swarming: &pb.Swarming{
						Builders:                     []*pb.BuilderConfig{},
						TaskTemplateCanaryPercentage: &wrapperspb.UInt32Value{Value: uint32(10)},
					},
					Shadow: "try.shadow",
				},
				{
					Name:                   "try.shadow",
					DynamicBuilderTemplate: &pb.Bucket_DynamicBuilderTemplate{},
				},
				{
					Name: "try",
					Swarming: &pb.Swarming{
						Builders: []*pb.BuilderConfig{},
					},
				},
			}))
			assert.Loosely(t, actualBkts, should.Resemble([]*model.Bucket{
				{
					ID:       "master.tryserver.chromium.linux",
					Parent:   model.ProjectKey(ctx, "chromium"),
					Bucket:   "master.tryserver.chromium.linux",
					Schema:   CurrentBucketSchemaVersion,
					Revision: "new!",
				},
				{
					ID:       "master.tryserver.chromium.mac",
					Parent:   model.ProjectKey(ctx, "chromium"),
					Bucket:   "master.tryserver.chromium.mac",
					Schema:   CurrentBucketSchemaVersion,
					Revision: "new!",
				},
				{
					ID:       "try",
					Parent:   model.ProjectKey(ctx, "chromium"),
					Bucket:   "try",
					Schema:   CurrentBucketSchemaVersion,
					Revision: "new!",
				},
				{
					ID:       "try.shadow",
					Parent:   model.ProjectKey(ctx, "chromium"),
					Bucket:   "try.shadow",
					Schema:   CurrentBucketSchemaVersion,
					Revision: "new!",
					Shadows:  []string{"try"},
				},
				{
					ID:       "try",
					Parent:   model.ProjectKey(ctx, "dart"),
					Bucket:   "try",
					Schema:   CurrentBucketSchemaVersion,
					Revision: "deadbeef",
				},
			}))

			var actualBuilders []*model.Builder
			assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind), &actualBuilders), should.BeNil)
			assert.Loosely(t, len(actualBuilders), should.Equal(2))
			expectedBuilder1 := &pb.BuilderConfig{
				Name:                         "linux",
				Dimensions:                   []string{"os:Linux", "pool:luci.chromium.try"},
				SwarmingHost:                 "swarming.updated.example.com",
				TaskTemplateCanaryPercentage: &wrapperspb.UInt32Value{Value: uint32(10)},
				Exe: &pb.Executable{
					CipdPackage: "infra/recipe_bundle",
					CipdVersion: "refs/heads/main",
					Cmd:         []string{"luciexe"},
				},
			}
			expectedBuilder2 := &pb.BuilderConfig{
				Name:       "linux",
				Dimensions: []string{"pool:Dart.LUCI"},
				Exe: &pb.Executable{
					CipdPackage: "infra/recipe_bundle",
					CipdVersion: "refs/heads/main",
					Cmd:         []string{"luciexe"},
				},
			}
			expectedBldrHash1, _, _ := computeBuilderHash(expectedBuilder1)
			expectedBldrHash2, _, _ := computeBuilderHash(expectedBuilder2)
			assert.Loosely(t, stripBuilderProtos(actualBuilders), should.Resemble([]*pb.BuilderConfig{expectedBuilder1, expectedBuilder2}))
			assert.Loosely(t, actualBuilders, should.Resemble([]*model.Builder{
				{
					ID:         "linux",
					Parent:     model.BucketKey(ctx, "chromium", "try"),
					ConfigHash: expectedBldrHash1,
				},
				{
					ID:         "linux",
					Parent:     model.BucketKey(ctx, "dart", "try"),
					ConfigHash: expectedBldrHash2,
				},
			}))
		})

		t.Run("max_concurent_builds", func(t *ftt.Test) {
			ctx, sch := tq.TestingContext(ctx, nil)
			// RegisterTaskClass should be called only once.
			if tq.Default.TaskClassRef("pop-pending-builds") == nil {
				tq.RegisterTaskClass(tq.TaskClass{
					ID:        "pop-pending-builds",
					Kind:      tq.FollowsContext,
					Prototype: (*taskdefs.PopPendingBuildTask)(nil),
					Queue:     "pop-pending-builds",
					Handler: func(ctx context.Context, payload proto.Message) error {
						return nil
					},
				})
			}

			cfgClient.chromiumBuildbucketCfg = `
			buckets {
				name: "try"
				swarming {
					builders {
						name: "linux"
						max_concurrent_builds: 2
					}
				}
			}
			`
			cfgClient.chromiumRevision = "new!"
			assert.Loosely(t, UpdateProjectCfg(ctx), should.BeNil)
			assert.Loosely(t, sch.Tasks(), should.HaveLength(0))

			t.Run("increased", func(t *ftt.Test) {
				cfgClient.chromiumBuildbucketCfg = `
				buckets {
					name: "try"
					swarming {
						builders {
							name: "linux"
							max_concurrent_builds: 4
						}
					}
				}
				`
				cfgClient.chromiumRevision = "new!!"
				assert.Loosely(t, UpdateProjectCfg(ctx), should.BeNil)
				assert.Loosely(t, sch.Tasks(), should.HaveLength(1))
				assert.Loosely(t, sch.Tasks()[0].Payload.(*taskdefs.PopPendingBuildTask).GetBuildId(), should.BeZero)
				assert.Loosely(t, sch.Tasks()[0].Payload.(*taskdefs.PopPendingBuildTask).GetBuilderId(), should.Match(&pb.BuilderID{
					Project: "chromium",
					Bucket:  "try",
					Builder: "linux",
				}))
			})

			t.Run("decreased", func(t *ftt.Test) {
				cfgClient.chromiumBuildbucketCfg = `
				buckets {
					name: "try"
					swarming {
						builders {
							name: "linux"
							max_concurrent_builds: 2
						}
					}
				}
				`
				cfgClient.chromiumRevision = "new!!!"
				assert.Loosely(t, UpdateProjectCfg(ctx), should.BeNil)
				assert.Loosely(t, sch.Tasks(), should.HaveLength(0))
			})

			t.Run("reset", func(t *ftt.Test) {
				cfgClient.chromiumBuildbucketCfg = `
				buckets {
					name: "try"
					swarming {
						builders {
							name: "linux"
						}
					}
				}
				`
				cfgClient.chromiumRevision = "new!!!!"
				assert.Loosely(t, UpdateProjectCfg(ctx), should.BeNil)
				assert.Loosely(t, sch.Tasks(), should.HaveLength(1))
			})

		})

		t.Run("test custom builder metrics", func(t *ftt.Test) {
			settingsCfg := &pb.SettingsCfg{
				CustomMetrics: []*pb.CustomMetric{
					{
						Name:        "chrome/infra/custom/builds/count",
						ExtraFields: []string{"os"},
						Class: &pb.CustomMetric_MetricBase{
							MetricBase: pb.CustomMetricBase_CUSTOM_METRIC_BASE_COUNT,
						},
					},
				},
			}
			_ = SetTestSettingsCfg(ctx, settingsCfg)

			// Update luci.chromium.try
			// Delete master.tryserver.chromium.linux
			// Delete master.tryserver.chromium.win
			cfgClient.chromiumBuildbucketCfg = `
			buckets {
				name: "try"
				swarming {
					task_template_canary_percentage { value: 10 }
					builders {
						name: "linux"
						swarming_host: "swarming.updated.example.com"
						task_template_canary_percentage { value: 10 }
						dimensions: "os:Linux"
						exe {
							cipd_version: "refs/heads/main"
							cipd_package: "infra/recipe_bundle"
							cmd: ["luciexe"]
						}
						custom_metric_definitions {
							name: "chrome/infra/custom/builds/count"
							predicates: "build.tags.get_value(\"os\")!=\"\""
							extra_fields {
								key: "os",
								value: "build.tags.get_value(\"os\")",
							}
						}
					}
				}
			}
			`
			cfgClient.chromiumRevision = "new!"
			// Delete the entire dart cfg
			cfgClient.dartBuildbucketCfg = ""

			assert.Loosely(t, UpdateProjectCfg(ctx), should.BeNil)
			var actualBkts []*model.Bucket
			assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.BucketKind), &actualBkts), should.BeNil)
			assert.Loosely(t, len(actualBkts), should.Equal(2))
			assert.Loosely(t, stripBucketProtos(actualBkts)[0], should.Resemble(&pb.Bucket{
				Name: "try",
				Swarming: &pb.Swarming{
					Builders:                     []*pb.BuilderConfig{},
					TaskTemplateCanaryPercentage: &wrapperspb.UInt32Value{Value: uint32(10)},
				},
			}))
			assert.Loosely(t, actualBkts[0], should.Resemble(&model.Bucket{
				ID:       "try",
				Parent:   model.ProjectKey(ctx, "chromium"),
				Bucket:   "try",
				Schema:   CurrentBucketSchemaVersion,
				Revision: "new!",
			}))

			var actualBuilders []*model.Builder
			assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind), &actualBuilders), should.BeNil)
			assert.Loosely(t, len(actualBuilders), should.Equal(1))
			expectedBuilder1 := &pb.BuilderConfig{
				Name:                         "linux",
				Dimensions:                   []string{"os:Linux", "pool:luci.chromium.try"},
				SwarmingHost:                 "swarming.updated.example.com",
				TaskTemplateCanaryPercentage: &wrapperspb.UInt32Value{Value: uint32(10)},
				Exe: &pb.Executable{
					CipdPackage: "infra/recipe_bundle",
					CipdVersion: "refs/heads/main",
					Cmd:         []string{"luciexe"},
				},
				CustomMetricDefinitions: []*pb.CustomMetricDefinition{
					{
						Name:        "chrome/infra/custom/builds/count",
						Predicates:  []string{`build.tags.get_value("os")!=""`},
						ExtraFields: map[string]string{"os": `build.tags.get_value("os")`},
					},
				},
			}
			expectedBldrHash1, _, _ := computeBuilderHash(expectedBuilder1)
			assert.Loosely(t, stripBuilderProtos(actualBuilders), should.Resemble([]*pb.BuilderConfig{expectedBuilder1}))
			assert.Loosely(t, actualBuilders, should.Resemble([]*model.Builder{
				{
					ID:         "linux",
					Parent:     model.BucketKey(ctx, "chromium", "try"),
					ConfigHash: expectedBldrHash1,
				},
			}))

			bldrMetrics := &model.CustomBuilderMetrics{Key: model.CustomBuilderMetricsKey(ctx)}
			assert.Loosely(t, datastore.Get(ctx, bldrMetrics), should.BeNil)
			expectedBldrMetricsP := &modeldefs.CustomBuilderMetrics{
				Metrics: []*modeldefs.CustomBuilderMetric{
					{
						Name: "chrome/infra/custom/builds/count",
						Builders: []*pb.BuilderID{
							{
								Project: "chromium",
								Bucket:  "try",
								Builder: "linux",
							},
						},
					},
				},
			}
			assert.Loosely(t, bldrMetrics.Metrics, should.Resemble(expectedBldrMetricsP))

			// Delete the entire v8 cfg
			cfgClient.v8BuildbucketCfg = ""
			assert.Loosely(t, UpdateProjectCfg(ctx), should.BeNil)
			assert.Loosely(t, datastore.Get(ctx, bldrMetrics), should.BeNil)
			assert.Loosely(t, bldrMetrics.Metrics, should.Resemble(expectedBldrMetricsP))

			// no change on the configs.
			assert.Loosely(t, UpdateProjectCfg(ctx), should.BeNil)
			assert.Loosely(t, datastore.Get(ctx, bldrMetrics), should.BeNil)
			assert.Loosely(t, bldrMetrics.Metrics, should.Resemble(expectedBldrMetricsP))
		})

		t.Run("test shadow", func(t *ftt.Test) {
			// Add shadow bucket try.shadow which shadows try
			cfgClient.chromiumBuildbucketCfg = `
			buckets {
				name: "try"
				swarming {
					task_template_canary_percentage { value: 10 }
					builders {
						name: "linux"
						swarming_host: "swarming.updated.example.com"
						task_template_canary_percentage { value: 10 }
						dimensions: "os:Linux"
						exe {
							cipd_version: "refs/heads/main"
							cipd_package: "infra/recipe_bundle"
							cmd: ["luciexe"]
						}
					}
				}
				shadow: "try.shadow"
			}
			buckets {
				name: "try.shadow"
				dynamic_builder_template {}
			}
			`
			cfgClient.chromiumRevision = "new!"
			// Delete the entire v8 cfg
			cfgClient.v8BuildbucketCfg = ""

			assert.Loosely(t, UpdateProjectCfg(ctx), should.BeNil)

			// Add a new bucket also shadowed by try.shadow
			cfgClient.chromiumBuildbucketCfg = `
			buckets {
				name: "try"
				swarming {
					task_template_canary_percentage { value: 10 }
					builders {
						name: "linux"
						swarming_host: "swarming.updated.example.com"
						task_template_canary_percentage { value: 10 }
						dimensions: "os:Linux"
						exe {
							cipd_version: "refs/heads/main"
							cipd_package: "infra/recipe_bundle"
							cmd: ["luciexe"]
						}
					}
				}
				shadow: "try.shadow"
			}
			buckets {
				name: "try.shadow"
				dynamic_builder_template {}
			}
			buckets {
				name: "another"
				shadow: "try.shadow"
			}
			`
			// Delete the entire v8 cfg
			cfgClient.v8BuildbucketCfg = ""

			assert.Loosely(t, UpdateProjectCfg(ctx), should.BeNil)
			var actualBkts []*model.Bucket
			assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.BucketKind), &actualBkts), should.BeNil)
			assert.Loosely(t, len(actualBkts), should.Equal(4))
			assert.Loosely(t, stripBucketProtos(actualBkts), should.Resemble([]*pb.Bucket{
				{
					Name:   "another",
					Shadow: "try.shadow",
				},
				{
					Name: "try",
					Swarming: &pb.Swarming{
						Builders:                     []*pb.BuilderConfig{},
						TaskTemplateCanaryPercentage: &wrapperspb.UInt32Value{Value: uint32(10)},
					},
					Shadow: "try.shadow",
				},
				{
					Name:                   "try.shadow",
					DynamicBuilderTemplate: &pb.Bucket_DynamicBuilderTemplate{},
				},
				{
					Name: "try",
					Swarming: &pb.Swarming{
						Builders: []*pb.BuilderConfig{},
					},
				},
			}))
			assert.Loosely(t, actualBkts, should.Resemble([]*model.Bucket{
				{
					ID:       "another",
					Parent:   model.ProjectKey(ctx, "chromium"),
					Bucket:   "another",
					Schema:   CurrentBucketSchemaVersion,
					Revision: "new!",
				},
				{
					ID:       "try",
					Parent:   model.ProjectKey(ctx, "chromium"),
					Bucket:   "try",
					Schema:   CurrentBucketSchemaVersion,
					Revision: "new!",
				},
				{
					ID:       "try.shadow",
					Parent:   model.ProjectKey(ctx, "chromium"),
					Bucket:   "try.shadow",
					Schema:   CurrentBucketSchemaVersion,
					Revision: "new!",
					Shadows:  []string{"another", "try"},
				},
				{
					ID:       "try",
					Parent:   model.ProjectKey(ctx, "dart"),
					Bucket:   "try",
					Schema:   CurrentBucketSchemaVersion,
					Revision: "deadbeef",
				},
			}))

		})

		t.Run("with broken configs", func(t *ftt.Test) {
			// Delete chromium and v8 configs
			cfgClient.chromiumBuildbucketCfg = ""
			cfgClient.v8BuildbucketCfg = ""

			cfgClient.dartBuildbucketCfg = "broken bucket cfg"
			cfgClient.dartBuildbucketCfg = "new!"

			assert.Loosely(t, UpdateProjectCfg(ctx), should.BeNil)

			// We must not delete buckets or builders defined in a project that
			// currently have a broken config.
			var actualBkts []*model.Bucket
			assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.BucketKind), &actualBkts), should.BeNil)
			assert.Loosely(t, len(actualBkts), should.Equal(1))
			assert.Loosely(t, stripBucketProtos(actualBkts), should.Resemble([]*pb.Bucket{
				{
					Name: "try",
					Swarming: &pb.Swarming{
						Builders: []*pb.BuilderConfig{},
					},
				},
			}))
			assert.Loosely(t, actualBkts, should.Resemble([]*model.Bucket{
				{
					ID:       "try",
					Parent:   model.ProjectKey(ctx, "dart"),
					Bucket:   "try",
					Schema:   CurrentBucketSchemaVersion,
					Revision: "deadbeef",
				},
			}))

			var actualBuilders []*model.Builder
			assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind), &actualBuilders), should.BeNil)
			assert.Loosely(t, len(actualBuilders), should.Equal(1))
			dartBuilder := &pb.BuilderConfig{
				Name:       "linux",
				Dimensions: []string{"pool:Dart.LUCI"},
				Exe: &pb.Executable{
					CipdPackage: "infra/recipe_bundle",
					CipdVersion: "refs/heads/main",
					Cmd:         []string{"luciexe"},
				},
			}
			dartBuilderHash, _, _ := computeBuilderHash(dartBuilder)
			assert.Loosely(t, stripBuilderProtos(actualBuilders), should.Resemble([]*pb.BuilderConfig{dartBuilder}))
			assert.Loosely(t, actualBuilders, should.Resemble([]*model.Builder{
				{
					ID:         "linux",
					Parent:     model.BucketKey(ctx, "dart", "try"),
					ConfigHash: dartBuilderHash,
				},
			}))
		})

		t.Run("dart config return error", func(t *ftt.Test) {
			// Delete chromium and v8 configs
			cfgClient.chromiumBuildbucketCfg = ""
			cfgClient.v8BuildbucketCfg = ""

			// luci-config return server error when fetching dart config.
			cfgClient.dartBuildbucketCfg = "error"

			assert.Loosely(t, UpdateProjectCfg(ctx), should.BeNil)

			// Don't delete the stored buckets and builders when luci-config returns
			// an error for fetching that project config.
			var actualBkts []*model.Bucket
			assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.BucketKind), &actualBkts), should.BeNil)
			assert.Loosely(t, len(actualBkts), should.Equal(1))
			assert.Loosely(t, stripBucketProtos(actualBkts), should.Resemble([]*pb.Bucket{
				{
					Name: "try",
					Swarming: &pb.Swarming{
						Builders: []*pb.BuilderConfig{},
					},
				},
			}))
			assert.Loosely(t, actualBkts, should.Resemble([]*model.Bucket{
				{
					ID:       "try",
					Parent:   model.ProjectKey(ctx, "dart"),
					Bucket:   "try",
					Schema:   CurrentBucketSchemaVersion,
					Revision: "deadbeef",
				},
			}))

			var actualBuilders []*model.Builder
			assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind), &actualBuilders), should.BeNil)
			assert.Loosely(t, len(actualBuilders), should.Equal(1))
			dartBuilder := &pb.BuilderConfig{
				Name:       "linux",
				Dimensions: []string{"pool:Dart.LUCI"},
				Exe: &pb.Executable{
					CipdPackage: "infra/recipe_bundle",
					CipdVersion: "refs/heads/main",
					Cmd:         []string{"luciexe"},
				},
			}
			dartBuilderHash, _, _ := computeBuilderHash(dartBuilder)
			assert.Loosely(t, stripBuilderProtos(actualBuilders), should.Resemble([]*pb.BuilderConfig{dartBuilder}))
			assert.Loosely(t, actualBuilders, should.Resemble([]*model.Builder{
				{
					ID:         "linux",
					Parent:     model.BucketKey(ctx, "dart", "try"),
					ConfigHash: dartBuilderHash,
				},
			}))
		})

		t.Run("large builders count", func(t *ftt.Test) {
			// clear dart configs first
			cfgClient.dartBuildbucketCfg = `buckets {name: "try"}`
			cfgClient.dartRevision = `clear_dart`

			assert.Loosely(t, UpdateProjectCfg(ctx), should.BeNil)
			actualBucket := &model.Bucket{ID: "try", Parent: model.ProjectKey(ctx, "dart")}
			assert.Loosely(t, datastore.Get(ctx, actualBucket), should.BeNil)
			assert.Loosely(t, actualBucket.Revision, should.Equal("clear_dart"))
			var actualBuilders []*model.Builder
			assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind).Ancestor(model.BucketKey(ctx, "dart", "try")).Order("__key__"), &actualBuilders), should.BeNil)
			assert.Loosely(t, len(actualBuilders), should.BeZero)

			t.Run("to put 499 builders", func(t *ftt.Test) {
				bldrsCfg := ""
				for i := 0; i < 499; i++ {
					bldrsCfg += fmt.Sprintf("builders {name: \"builder%d\"}\n", i)
				}
				cfgClient.dartBuildbucketCfg = fmt.Sprintf(`buckets {name: "try"swarming {%s}}`, bldrsCfg)
				cfgClient.dartRevision = "put499"

				assert.Loosely(t, UpdateProjectCfg(ctx), should.BeNil)

				actualBucket := &model.Bucket{ID: "try", Parent: model.ProjectKey(ctx, "dart")}
				assert.Loosely(t, datastore.Get(ctx, actualBucket), should.BeNil)
				assert.Loosely(t, actualBucket.Revision, should.Equal("put499"))
				var actualBuilders []*model.Builder
				assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind).Ancestor(model.BucketKey(ctx, "dart", "try")).Order("__key__"), &actualBuilders), should.BeNil)
				assert.Loosely(t, len(actualBuilders), should.Equal(499))
			})

			t.Run("to put 500 builders", func(t *ftt.Test) {
				bldrsCfg := ""
				for i := 0; i < 500; i++ {
					bldrsCfg += fmt.Sprintf("builders {name: \"builder%d\"}\n", i)
				}
				cfgClient.dartBuildbucketCfg = fmt.Sprintf(`buckets {name: "try"swarming {%s}}`, bldrsCfg)
				cfgClient.dartRevision = "put500"

				assert.Loosely(t, UpdateProjectCfg(ctx), should.BeNil)

				actualBucket := &model.Bucket{ID: "try", Parent: model.ProjectKey(ctx, "dart")}
				assert.Loosely(t, datastore.Get(ctx, actualBucket), should.BeNil)
				assert.Loosely(t, actualBucket.Revision, should.Equal("put500"))
				var actualBuilders []*model.Builder
				assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind).Ancestor(model.BucketKey(ctx, "dart", "try")).Order("__key__"), &actualBuilders), should.BeNil)
				assert.Loosely(t, len(actualBuilders), should.Equal(500))
			})

			t.Run("to put 1105 builders", func(t *ftt.Test) {
				bldrsCfg := ""
				for i := 0; i < 1105; i++ {
					bldrsCfg += fmt.Sprintf("builders {name: \"builder%d\"}\n", i)
				}
				cfgClient.dartBuildbucketCfg = fmt.Sprintf(`buckets {name: "try"swarming {%s}}`, bldrsCfg)
				cfgClient.dartRevision = "put1105"

				assert.Loosely(t, UpdateProjectCfg(ctx), should.BeNil)

				actualBucket := &model.Bucket{ID: "try", Parent: model.ProjectKey(ctx, "dart")}
				assert.Loosely(t, datastore.Get(ctx, actualBucket), should.BeNil)
				assert.Loosely(t, actualBucket.Revision, should.Equal("put1105"))
				var actualBuilders []*model.Builder
				assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind).Ancestor(model.BucketKey(ctx, "dart", "try")).Order("__key__"), &actualBuilders), should.BeNil)
				assert.Loosely(t, len(actualBuilders), should.Equal(1105))

				t.Run("delete 111 and update 994", func(t *ftt.Test) {
					bldrsCfg := ""
					for i := 0; i < 1105; i++ {
						// delete builders which the name ends with "1".
						if i%10 == 1 {
							continue
						}
						bldrsCfg += fmt.Sprintf("builders {name: \"builder%d\" \n dimensions: \"pool:newly_added\"}\n", i)
					}
					cfgClient.dartBuildbucketCfg = fmt.Sprintf(`buckets {name: "try"swarming {%s}}`, bldrsCfg)
					cfgClient.dartRevision = "del111_update994"

					assert.Loosely(t, UpdateProjectCfg(ctx), should.BeNil)

					actualBucket := &model.Bucket{ID: "try", Parent: model.ProjectKey(ctx, "dart")}
					assert.Loosely(t, datastore.Get(ctx, actualBucket), should.BeNil)
					assert.Loosely(t, actualBucket.Revision, should.Equal("del111_update994"))
					var actualBuilders []*model.Builder
					assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind).Ancestor(model.BucketKey(ctx, "dart", "try")).Order("__key__"), &actualBuilders), should.BeNil)
					assert.Loosely(t, len(actualBuilders), should.Equal(994))
					for _, bldr := range actualBuilders {
						assert.Loosely(t, strings.HasSuffix(bldr.ID, "1"), should.BeFalse)
						assert.Loosely(t, bldr.Config.Dimensions[0], should.Equal("pool:newly_added"))
					}
				})

				t.Run("delete 994 and update 111", func(t *ftt.Test) {
					bldrsCfg := ""
					for i := 0; i < 1105; i++ {
						// only keep builders which the name ends with "1" and update them.
						if i%10 == 1 {
							bldrsCfg += fmt.Sprintf("builders {name: \"builder%d\" \n dimensions: \"pool:newly_added\"}\n", i)
						}
					}
					cfgClient.dartBuildbucketCfg = fmt.Sprintf(`buckets {name: "try"swarming {%s}}`, bldrsCfg)
					cfgClient.dartRevision = "del994_update111"

					assert.Loosely(t, UpdateProjectCfg(ctx), should.BeNil)

					actualBucket := &model.Bucket{ID: "try", Parent: model.ProjectKey(ctx, "dart")}
					assert.Loosely(t, datastore.Get(ctx, actualBucket), should.BeNil)
					assert.Loosely(t, actualBucket.Revision, should.Equal("del994_update111"))
					var actualBuilders []*model.Builder
					assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind).Ancestor(model.BucketKey(ctx, "dart", "try")).Order("__key__"), &actualBuilders), should.BeNil)
					assert.Loosely(t, len(actualBuilders), should.Equal(111))
					for _, bldr := range actualBuilders {
						assert.Loosely(t, strings.HasSuffix(bldr.ID, "1"), should.BeTrue)
						assert.Loosely(t, bldr.Config.Dimensions[0], should.Equal("pool:newly_added"))
					}
				})
			})
		})

		t.Run("large builder content", func(t *ftt.Test) {
			// clear dart configs first
			cfgClient.dartBuildbucketCfg = `buckets {name: "try"}`
			cfgClient.dartRevision = `clear_dart`

			assert.Loosely(t, UpdateProjectCfg(ctx), should.BeNil)
			actualBucket := &model.Bucket{ID: "try", Parent: model.ProjectKey(ctx, "dart")}
			assert.Loosely(t, datastore.Get(ctx, actualBucket), should.BeNil)
			assert.Loosely(t, actualBucket.Revision, should.Equal("clear_dart"))
			var actualBuilders []*model.Builder
			assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind).Ancestor(model.BucketKey(ctx, "dart", "try")).Order("__key__"), &actualBuilders), should.BeNil)
			assert.Loosely(t, len(actualBuilders), should.BeZero)

			originalMaxBatchSize := maxBatchSize
			defer func() {
				maxBatchSize = originalMaxBatchSize
			}()
			maxBatchSize = 200

			t.Run("a single too large", func(t *ftt.Test) {
				large := ""
				for i := 0; i < 30; i++ {
					large += "0123456789"
				}
				cfgClient.dartBuildbucketCfg = fmt.Sprintf(`buckets {name: "try" swarming {builders {name: "%s"}}}`, large)
				cfgClient.dartRevision = "one_large"

				err := UpdateProjectCfg(ctx)
				assert.Loosely(t, err, should.ErrLike("size exceeds 200 bytes"))
			})

			t.Run("the sum > maxBatchSize while builders count < 500", func(t *ftt.Test) {
				bldrsCfg := ""
				for i := 0; i < 212; i++ {
					bldrsCfg += fmt.Sprintf("builders {name: \"medium_size_builder_%d\"}\n", i)
				}

				cfgClient.dartBuildbucketCfg = fmt.Sprintf(`buckets {name: "try"swarming {%s}}`, bldrsCfg)
				cfgClient.dartRevision = "sum_large"

				assert.Loosely(t, UpdateProjectCfg(ctx), should.BeNil)

				actualBucket := &model.Bucket{ID: "try", Parent: model.ProjectKey(ctx, "dart")}
				assert.Loosely(t, datastore.Get(ctx, actualBucket), should.BeNil)
				assert.Loosely(t, actualBucket.Revision, should.Equal("sum_large"))
				var actualBuilders []*model.Builder
				assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind).Ancestor(model.BucketKey(ctx, "dart", "try")).Order("__key__"), &actualBuilders), should.BeNil)
				assert.Loosely(t, len(actualBuilders), should.Equal(212))
			})
		})

		t.Run("builds_notification_topics", func(t *ftt.Test) {
			topicSort := func(topics []*pb.BuildbucketCfg_Topic) {
				sort.Slice(topics, func(i, j int) bool {
					if topics[i].Name == topics[j].Name {
						return topics[i].Compression < topics[j].Compression
					}
					return topics[i].Name < topics[j].Name
				})
			}
			cfgClient.dartBuildbucketCfg = defaultDartBuildbucketCfg + `
	common_config {
		builds_notification_topics {
			name: "projects/dart/topics/my-dart-topic1"
		}
		builds_notification_topics {
			name: "projects/dart/topics/my-dart-topic2"
		}
	}`
			cfgClient.dartRevision = "dart_add_topics"

			// before UpdateProjectCfg, no project entity.
			assert.Loosely(t, datastore.Get(ctx, &model.Project{ID: "dart"}), should.Equal(datastore.ErrNoSuchEntity))
			assert.Loosely(t, UpdateProjectCfg(ctx), should.BeNil)
			actualProj := &model.Project{ID: "dart"}
			assert.Loosely(t, datastore.Get(ctx, actualProj), should.BeNil)
			topicSort(actualProj.CommonConfig.BuildsNotificationTopics)
			assert.Loosely(t, actualProj.CommonConfig.BuildsNotificationTopics, should.Resemble([]*pb.BuildbucketCfg_Topic{
				{
					Name:        "projects/dart/topics/my-dart-topic1",
					Compression: pb.Compression_ZLIB,
				},
				{
					Name:        "projects/dart/topics/my-dart-topic2",
					Compression: pb.Compression_ZLIB,
				},
			}))

			t.Run("modify", func(t *ftt.Test) {
				cfgClient.dartBuildbucketCfg = defaultDartBuildbucketCfg + `
	common_config {
		builds_notification_topics {
			name: "projects/dart/topics/my-dart-topic1"
			compression: ZSTD
		}
		builds_notification_topics {
			name: "projects/dart/topics/my-dart-topic2"
		}
	}`
				cfgClient.dartRevision = "dart_modify_topics"
				assert.Loosely(t, UpdateProjectCfg(ctx), should.BeNil)
				actualProj := &model.Project{ID: "dart"}
				assert.Loosely(t, datastore.Get(ctx, actualProj), should.BeNil)
				topicSort(actualProj.CommonConfig.BuildsNotificationTopics)
				assert.Loosely(t, actualProj.CommonConfig.BuildsNotificationTopics, should.Resemble([]*pb.BuildbucketCfg_Topic{
					{
						Name:        "projects/dart/topics/my-dart-topic1",
						Compression: pb.Compression_ZSTD,
					},
					{
						Name:        "projects/dart/topics/my-dart-topic2",
						Compression: pb.Compression_ZLIB,
					},
				}))

			})

			t.Run("delete all topics", func(t *ftt.Test) {
				cfgClient.dartBuildbucketCfg = defaultDartBuildbucketCfg
				cfgClient.dartRevision = "dart_empty_topics"
				assert.Loosely(t, UpdateProjectCfg(ctx), should.BeNil)
				actualProj := &model.Project{ID: "dart"}
				assert.Loosely(t, datastore.Get(ctx, actualProj), should.BeNil)
				assert.Loosely(t, actualProj.CommonConfig, should.BeNil)
			})

			t.Run("delete the project", func(t *ftt.Test) {
				cfgClient.dartBuildbucketCfg = ``
				cfgClient.dartRevision = "dart_is_deleted"
				assert.Loosely(t, UpdateProjectCfg(ctx), should.BeNil)
				actualProj := &model.Project{ID: "dart"}
				assert.Loosely(t, datastore.Get(ctx, actualProj), should.Equal(datastore.ErrNoSuchEntity))
				dartBuckets := []*model.Bucket{}
				assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.BucketKind).Ancestor(model.ProjectKey(ctx, "dart")), &dartBuckets), should.BeNil)
				assert.Loosely(t, dartBuckets, should.BeEmpty)
				dartBuilders := []*model.Builder{}
				assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.BucketKind).Ancestor(model.BucketKey(ctx, "dart", "try")), &dartBuilders), should.BeNil)
				assert.Loosely(t, dartBuilders, should.BeEmpty)
			})
		})
	})
}

func TestPrepareBuilderMetricsToPut(t *testing.T) {
	t.Parallel()

	ftt.Run("prepareBuilderMetricsToPut", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		now := testclock.TestRecentTimeUTC
		lastUpdate := now.Add(-time.Hour)
		bldrMetrics := &model.CustomBuilderMetrics{
			Key:        model.CustomBuilderMetricsKey(ctx),
			LastUpdate: lastUpdate,
			Metrics: &modeldefs.CustomBuilderMetrics{
				Metrics: []*modeldefs.CustomBuilderMetric{
					{
						Name: "chrome/infra/custom/builds/count",
						Builders: []*pb.BuilderID{
							{
								Project: "chromium",
								Bucket:  "try",
								Builder: "linux",
							},
							{
								Project: "chromium",
								Bucket:  "try",
								Builder: "mac",
							},
						},
					},
					{
						Name: "chrome/infra/custom/builds/max_age",
						Builders: []*pb.BuilderID{
							{
								Project: "chromium",
								Bucket:  "try",
								Builder: "linux",
							},
						},
					},
				},
			},
		}
		assert.Loosely(t, datastore.Put(ctx, bldrMetrics), should.BeNil)

		t.Run("no update", func(t *ftt.Test) {
			t.Run("exactly same content", func(t *ftt.Test) {
				bldrMetrics := map[string]stringset.Set{
					"chrome/infra/custom/builds/count":   stringset.NewFromSlice([]string{"chromium/try/linux", "chromium/try/mac"}...),
					"chrome/infra/custom/builds/max_age": stringset.NewFromSlice([]string{"chromium/try/linux"}...),
				}
				newEnt, err := prepareBuilderMetricsToPut(ctx, bldrMetrics)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, newEnt, should.BeNil)
			})

			t.Run("same content, different metric order", func(t *ftt.Test) {
				bldrMetrics := map[string]stringset.Set{
					"chrome/infra/custom/builds/max_age": stringset.NewFromSlice([]string{"chromium/try/linux"}...),
					"chrome/infra/custom/builds/count":   stringset.NewFromSlice([]string{"chromium/try/linux", "chromium/try/mac"}...),
				}
				newEnt, err := prepareBuilderMetricsToPut(ctx, bldrMetrics)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, newEnt, should.BeNil)
			})

			t.Run("same content, different builder order", func(t *ftt.Test) {
				bldrMetrics := map[string]stringset.Set{
					"chrome/infra/custom/builds/count":   stringset.NewFromSlice([]string{"chromium/try/mac", "chromium/try/linux"}...),
					"chrome/infra/custom/builds/max_age": stringset.NewFromSlice([]string{"chromium/try/linux"}...),
				}
				newEnt, err := prepareBuilderMetricsToPut(ctx, bldrMetrics)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, newEnt, should.BeNil)
			})
		})

		t.Run("updated", func(t *ftt.Test) {
			t.Run("add metric", func(t *ftt.Test) {
				bldrMetrics := map[string]stringset.Set{
					"chrome/infra/custom/builds/count":    stringset.NewFromSlice([]string{"chromium/try/linux", "chromium/try/mac"}...),
					"chrome/infra/custom/builds/max_age":  stringset.NewFromSlice([]string{"chromium/try/linux"}...),
					"chrome/infra/custom/builds/max_age2": stringset.NewFromSlice([]string{"chromium/try/linux"}...),
				}
				newEnt, err := prepareBuilderMetricsToPut(ctx, bldrMetrics)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, newEnt.LastUpdate, should.Match(now))
				expected := &modeldefs.CustomBuilderMetrics{
					Metrics: []*modeldefs.CustomBuilderMetric{
						{
							Name: "chrome/infra/custom/builds/count",
							Builders: []*pb.BuilderID{
								{
									Project: "chromium",
									Bucket:  "try",
									Builder: "linux",
								},
								{
									Project: "chromium",
									Bucket:  "try",
									Builder: "mac",
								},
							},
						},
						{
							Name: "chrome/infra/custom/builds/max_age",
							Builders: []*pb.BuilderID{
								{
									Project: "chromium",
									Bucket:  "try",
									Builder: "linux",
								},
							},
						},
						{
							Name: "chrome/infra/custom/builds/max_age2",
							Builders: []*pb.BuilderID{
								{
									Project: "chromium",
									Bucket:  "try",
									Builder: "linux",
								},
							},
						},
					},
				}
				assert.Loosely(t, newEnt.Metrics, should.Resemble(expected))
			})
			t.Run("delete metric", func(t *ftt.Test) {
				bldrMetrics := map[string]stringset.Set{
					"chrome/infra/custom/builds/count": stringset.NewFromSlice([]string{"chromium/try/linux", "chromium/try/mac"}...),
				}
				newEnt, err := prepareBuilderMetricsToPut(ctx, bldrMetrics)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, newEnt.LastUpdate, should.Match(now))
				expected := &modeldefs.CustomBuilderMetrics{
					Metrics: []*modeldefs.CustomBuilderMetric{
						{
							Name: "chrome/infra/custom/builds/count",
							Builders: []*pb.BuilderID{
								{
									Project: "chromium",
									Bucket:  "try",
									Builder: "linux",
								},
								{
									Project: "chromium",
									Bucket:  "try",
									Builder: "mac",
								},
							},
						},
					},
				}
				assert.Loosely(t, newEnt.Metrics, should.Resemble(expected))
			})
			t.Run("add builder", func(t *ftt.Test) {
				bldrMetrics := map[string]stringset.Set{
					"chrome/infra/custom/builds/count":   stringset.NewFromSlice([]string{"chromium/try/linux", "chromium/try/mac"}...),
					"chrome/infra/custom/builds/max_age": stringset.NewFromSlice([]string{"chromium/try/linux", "chromium/try/mac"}...),
				}
				newEnt, err := prepareBuilderMetricsToPut(ctx, bldrMetrics)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, newEnt.LastUpdate, should.Match(now))
				expected := &modeldefs.CustomBuilderMetrics{
					Metrics: []*modeldefs.CustomBuilderMetric{
						{
							Name: "chrome/infra/custom/builds/count",
							Builders: []*pb.BuilderID{
								{
									Project: "chromium",
									Bucket:  "try",
									Builder: "linux",
								},
								{
									Project: "chromium",
									Bucket:  "try",
									Builder: "mac",
								},
							},
						},
						{
							Name: "chrome/infra/custom/builds/max_age",
							Builders: []*pb.BuilderID{
								{
									Project: "chromium",
									Bucket:  "try",
									Builder: "linux",
								},
								{
									Project: "chromium",
									Bucket:  "try",
									Builder: "mac",
								},
							},
						},
					},
				}
				assert.Loosely(t, newEnt.Metrics, should.Resemble(expected))
			})
			t.Run("remove builder", func(t *ftt.Test) {
				bldrMetrics := map[string]stringset.Set{
					"chrome/infra/custom/builds/count":   stringset.NewFromSlice([]string{"chromium/try/linux", "chromium/try/mac"}...),
					"chrome/infra/custom/builds/max_age": stringset.New(0),
				}
				newEnt, err := prepareBuilderMetricsToPut(ctx, bldrMetrics)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, newEnt.LastUpdate, should.Match(now))
				expected := &modeldefs.CustomBuilderMetrics{
					Metrics: []*modeldefs.CustomBuilderMetric{
						{
							Name: "chrome/infra/custom/builds/count",
							Builders: []*pb.BuilderID{
								{
									Project: "chromium",
									Bucket:  "try",
									Builder: "linux",
								},
								{
									Project: "chromium",
									Bucket:  "try",
									Builder: "mac",
								},
							},
						},
					},
				}
				assert.Loosely(t, newEnt.Metrics, should.Resemble(expected))
			})
		})
	})
}
