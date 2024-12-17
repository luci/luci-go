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
	"fmt"
	"strings"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"

	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/validate"
)

var goodTaskTemplate = &configpb.TaskTemplate{
	Cache: []*configpb.TaskTemplate_CacheEntry{
		{
			Name: "c1",
			Path: "a/b",
		},
	},
	CipdPackage: []*configpb.TaskTemplate_CipdPackage{
		{
			Pkg:     "a",
			Version: "latest",
			Path:    "c/d",
		},
	},
	Env: []*configpb.TaskTemplate_Env{
		{
			Var:   "ev",
			Value: "b",
			Prefix: []string{
				"e/f",
			},
			Soft: true,
		},
	},
}

var goodPoolsCfg = &configpb.PoolsCfg{
	TaskTemplate: []*configpb.TaskTemplate{
		{
			Name:        "t1",
			Cache:       goodTaskTemplate.Cache,
			CipdPackage: goodTaskTemplate.CipdPackage,
			Env:         goodTaskTemplate.Env,
		},
	},
	TaskTemplateDeployment: []*configpb.TaskTemplateDeployment{
		{
			Name: "d1",
			Prod: &configpb.TaskTemplate{
				Include: []string{
					"t1",
				},
			},
			Canary:       goodTaskTemplate,
			CanaryChance: 1000,
		},
	},
	Pool: []*configpb.Pool{
		{
			Name:             []string{"a"},
			Realm:            "test:1",
			DefaultTaskRealm: "test:default",
			TaskDeploymentScheme: &configpb.Pool_TaskTemplateDeployment{
				TaskTemplateDeployment: "d1",
			},
		},
		{
			Name:  []string{"b", "c"},
			Realm: "test:2",
			TaskDeploymentScheme: &configpb.Pool_TaskTemplateDeploymentInline{
				TaskTemplateDeploymentInline: &configpb.TaskTemplateDeployment{
					Prod:         goodTaskTemplate,
					Canary:       goodTaskTemplate,
					CanaryChance: 100,
				},
			},
			RbeMigration: &configpb.Pool_RBEMigration{
				RbeInstance:    "some-instance",
				RbeModePercent: 66,
				BotModeAllocation: []*configpb.Pool_RBEMigration_BotModeAllocation{
					{Mode: configpb.Pool_RBEMigration_BotModeAllocation_SWARMING, Percent: 5},
					{Mode: configpb.Pool_RBEMigration_BotModeAllocation_HYBRID, Percent: 10},
					{Mode: configpb.Pool_RBEMigration_BotModeAllocation_RBE, Percent: 85},
				},
			},
		},
	},
	DefaultExternalServices: &configpb.ExternalServices{
		Cipd: &configpb.ExternalServices_CIPD{
			Server: "https://cipd.example.com",
			ClientPackage: &configpb.CipdPackage{
				PackageName: "client/pkg",
				Version:     "latest",
			},
		},
	},
}

func TestNewPoolsConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		pools, err := newPoolsConfig(goodPoolsCfg)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pools, should.HaveLength(3))
		assert.Loosely(t, pools["a"].Realm, should.Equal("test:1"))
		assert.Loosely(t, pools["b"].Realm, should.Equal("test:2"))
		assert.Loosely(t, pools["c"].Realm, should.Equal("test:2"))

		assert.Loosely(t, pools["a"].DefaultTaskRealm, should.Equal("test:default"))
		assert.Loosely(t, pools["b"].DefaultTaskRealm, should.BeEmpty)
		assert.Loosely(t, pools["c"].DefaultTaskRealm, should.BeEmpty)

		expectedInline := &configpb.TaskTemplateDeployment{
			Prod:         goodTaskTemplate,
			Canary:       goodTaskTemplate,
			CanaryChance: 100,
		}
		assert.Loosely(t, pools["b"].Deployment, should.Match(expectedInline))

		expectedShared := &configpb.TaskTemplateDeployment{
			Name:         "d1",
			Prod:         goodTaskTemplate,
			Canary:       goodTaskTemplate,
			CanaryChance: 1000,
		}
		assert.Loosely(t, pools["a"].Deployment, should.Match(expectedShared))
	})
}

func TestPoolsValidation(t *testing.T) {
	t.Parallel()

	call := func(cfg *configpb.PoolsCfg) []string {
		ctx := validation.Context{Context: context.Background()}
		ctx.SetFile("pools.cfg")
		validatePoolsCfg(&ctx, cfg)
		if err := ctx.Finalize(); err != nil {
			var verr *validation.Error
			errors.As(err, &verr)
			out := make([]string, len(verr.Errors))
			for i, err := range verr.Errors {
				out[i] = err.Error()
			}
			return out
		}
		return nil
	}

	ftt.Run("Empty", t, func(t *ftt.Test) {
		assert.Loosely(t, call(&configpb.PoolsCfg{}), should.BeNil)
	})

	ftt.Run("Good", t, func(t *ftt.Test) {
		assert.Loosely(t, call(goodPoolsCfg), should.BeNil)
	})

	ftt.Run("Errors", t, func(t *ftt.Test) {
		t.Run("default_cipd", func(t *ftt.Test) {
			testCases := []struct {
				name string
				cfg  *configpb.PoolsCfg
				err  []string
			}{
				{
					name: "no_server",
					cfg: &configpb.PoolsCfg{
						DefaultExternalServices: &configpb.ExternalServices{
							Cipd: &configpb.ExternalServices_CIPD{
								ClientPackage: &configpb.CipdPackage{
									PackageName: "client/pkg",
									Version:     "latest",
								},
							},
						},
					},
					err: []string{`(default_cipd / server): required`},
				},
				{
					name: "no_client_package",
					cfg: &configpb.PoolsCfg{
						DefaultExternalServices: &configpb.ExternalServices{
							Cipd: &configpb.ExternalServices_CIPD{
								Server: "https://cipd.example.com",
							},
						},
					},
					err: []string{`(default_cipd / client_package): required`},
				},
				{
					name: "no_package_name",
					cfg: &configpb.PoolsCfg{
						DefaultExternalServices: &configpb.ExternalServices{
							Cipd: &configpb.ExternalServices_CIPD{
								Server: "https://cipd.example.com",
								ClientPackage: &configpb.CipdPackage{
									Version: "latest",
								},
							},
						},
					},
					err: []string{`(default_cipd / client_package / name): required`},
				},
				{
					name: "no_package_version",
					cfg: &configpb.PoolsCfg{
						DefaultExternalServices: &configpb.ExternalServices{
							Cipd: &configpb.ExternalServices_CIPD{
								Server: "https://cipd.example.com",
								ClientPackage: &configpb.CipdPackage{
									PackageName: "client/pkg",
								},
							},
						},
					},
					err: []string{`(default_cipd / client_package / version): required`},
				},
			}
			for _, tc := range testCases {
				t.Run(tc.name, func(t *ftt.Test) {
					for i := range tc.err {
						tc.err[i] = `in "pools.cfg" ` + tc.err[i]
					}
					assert.Loosely(t, call(tc.cfg), should.Match(tc.err))
				})
			}
		})
		t.Run("pool", func(t *ftt.Test) {
			onePool := func(p *configpb.Pool) *configpb.PoolsCfg {
				return &configpb.PoolsCfg{
					Pool: []*configpb.Pool{p},
					DefaultExternalServices: &configpb.ExternalServices{
						Cipd: &configpb.ExternalServices_CIPD{
							Server: "https://cipd.example.com",
							ClientPackage: &configpb.CipdPackage{
								PackageName: "client/pkg",
								Version:     "latest",
							},
						},
					},
				}
			}

			testCases := []struct {
				cfg *configpb.PoolsCfg
				err string
			}{
				// pool
				{
					cfg: onePool(&configpb.Pool{
						Name:       []string{"a"},
						Realm:      "test:1",
						Schedulers: &configpb.Schedulers{},
					}),
					err: "(pool #1 (a)): setting deprecated field `schedulers`",
				},
				{
					cfg: onePool(&configpb.Pool{
						Name:          []string{"a"},
						Realm:         "test:1",
						BotMonitoring: "bzzz",
					}),
					err: "(pool #1 (a)): setting deprecated field `bot_monitoring`",
				},
				{
					cfg: onePool(&configpb.Pool{
						Realm: "test:1",
					}),
					err: "(pool #1 (unnamed)): at least one pool name must be given",
				},
				{
					cfg: onePool(&configpb.Pool{
						Name:  []string{"a", ""},
						Realm: "test:1",
					}),
					err: `(pool #1 (a,)): bad pool name "": the value cannot be empty`,
				},
				{
					cfg: onePool(&configpb.Pool{
						Name:  []string{"a", "a"},
						Realm: "test:1",
					}),
					err: "(pool #1 (a,a)): pool \"a\" was already declared",
				},
				{
					cfg: onePool(&configpb.Pool{
						Name: []string{"a"},
					}),
					err: "(pool #1 (a)): missing required `realm` field",
				},
				{
					cfg: onePool(&configpb.Pool{
						Name:  []string{"a"},
						Realm: "not-global",
					}),
					err: "(pool #1 (a)): bad `realm` field: bad global realm name \"not-global\" - should be <project>:<realm>",
				},
				{
					cfg: onePool(&configpb.Pool{
						Name:             []string{"a"},
						Realm:            "test:1",
						DefaultTaskRealm: "not-global",
					}),
					err: "(pool #1 (a)): bad `default_task_realm` field: bad global realm name \"not-global\" - should be <project>:<realm>",
				},
				{
					cfg: onePool(&configpb.Pool{
						Name:  []string{"a"},
						Realm: "test:1",
						TaskDeploymentScheme: &configpb.Pool_TaskTemplateDeployment{
							TaskTemplateDeployment: "b",
						},
					}),
					err: "(pool #1 (a)): unknown `task_template_deployment`: \"b\"",
				},
				{
					cfg: onePool(&configpb.Pool{
						Name:  []string{"a"},
						Realm: "test:1",
						TaskDeploymentScheme: &configpb.Pool_TaskTemplateDeploymentInline{
							TaskTemplateDeploymentInline: &configpb.TaskTemplateDeployment{
								Name: "b",
								Prod: &configpb.TaskTemplate{},
							},
						},
					}),
					err: "(pool #1 (a) / task_template_deployment_inline): name cannot be specified",
				},
				// rbe_migration
				{
					cfg: onePool(&configpb.Pool{
						Name:  []string{"a"},
						Realm: "test:1",
						RbeMigration: &configpb.Pool_RBEMigration{
							BotModeAllocation: []*configpb.Pool_RBEMigration_BotModeAllocation{
								{Mode: configpb.Pool_RBEMigration_BotModeAllocation_RBE, Percent: 100},
							},
						},
					}),
					err: "(pool #1 (a) / rbe_migration): rbe_instance is required",
				},
				{
					cfg: onePool(&configpb.Pool{
						Name:  []string{"a"},
						Realm: "test:1",
						RbeMigration: &configpb.Pool_RBEMigration{
							RbeInstance:    "some-instance",
							RbeModePercent: 101,
							BotModeAllocation: []*configpb.Pool_RBEMigration_BotModeAllocation{
								{Mode: configpb.Pool_RBEMigration_BotModeAllocation_RBE, Percent: 100},
							},
						},
					}),
					err: "(pool #1 (a) / rbe_migration): rbe_mode_percent should be in [0; 100]",
				},
				{
					cfg: onePool(&configpb.Pool{
						Name:  []string{"a"},
						Realm: "test:1",
						RbeMigration: &configpb.Pool_RBEMigration{
							RbeInstance: "some-instance",
							BotModeAllocation: []*configpb.Pool_RBEMigration_BotModeAllocation{
								{Mode: configpb.Pool_RBEMigration_BotModeAllocation_UNKNOWN, Percent: 100},
							},
						},
					}),
					err: "(pool #1 (a) / rbe_migration / bot_mode_allocation #0): mode is required",
				},
				{
					cfg: onePool(&configpb.Pool{
						Name:  []string{"a"},
						Realm: "test:1",
						RbeMigration: &configpb.Pool_RBEMigration{
							RbeInstance: "some-instance",
							BotModeAllocation: []*configpb.Pool_RBEMigration_BotModeAllocation{
								{Mode: configpb.Pool_RBEMigration_BotModeAllocation_SWARMING, Percent: 20},
								{Mode: configpb.Pool_RBEMigration_BotModeAllocation_RBE, Percent: 80},
								{Mode: configpb.Pool_RBEMigration_BotModeAllocation_SWARMING, Percent: 10},
							},
						},
					}),
					err: "(pool #1 (a) / rbe_migration / bot_mode_allocation #2): allocation for mode SWARMING was already defined",
				},
				{
					cfg: onePool(&configpb.Pool{
						Name:  []string{"a"},
						Realm: "test:1",
						RbeMigration: &configpb.Pool_RBEMigration{
							RbeInstance: "some-instance",
							BotModeAllocation: []*configpb.Pool_RBEMigration_BotModeAllocation{
								{Mode: configpb.Pool_RBEMigration_BotModeAllocation_SWARMING, Percent: 101},
							},
						},
					}),
					err: "(pool #1 (a) / rbe_migration / bot_mode_allocation #0): percent should be in [0; 100]",
				},
				{
					cfg: onePool(&configpb.Pool{
						Name:  []string{"a"},
						Realm: "test:1",
						RbeMigration: &configpb.Pool_RBEMigration{
							RbeInstance: "some-instance",
							BotModeAllocation: []*configpb.Pool_RBEMigration_BotModeAllocation{
								{Mode: configpb.Pool_RBEMigration_BotModeAllocation_SWARMING, Percent: 20},
								{Mode: configpb.Pool_RBEMigration_BotModeAllocation_RBE, Percent: 80},
								{Mode: configpb.Pool_RBEMigration_BotModeAllocation_HYBRID, Percent: 10},
							},
						},
					}),
					err: "(pool #1 (a) / rbe_migration): bot_mode_allocation percents should sum up to 100",
				},
			}
			for _, cs := range testCases {
				assert.Loosely(t, call(cs.cfg), should.Match([]string{`in "pools.cfg" ` + cs.err}))
			}
		})

		t.Run("task_template_and_deployment", func(t *ftt.Test) {
			testCases := []struct {
				name string
				cfg  *configpb.PoolsCfg
				err  []string
			}{
				// task_template
				{
					name: "template_no_name",
					cfg: &configpb.PoolsCfg{
						TaskTemplate: []*configpb.TaskTemplate{
							{},
						},
					},
					err: []string{"(task_template / #1 ()): name is empty"},
				},
				{
					name: "template_cache",
					cfg: &configpb.PoolsCfg{
						TaskTemplate: []*configpb.TaskTemplate{
							{
								Name: "a",
								Cache: []*configpb.TaskTemplate_CacheEntry{
									{
										Name: "a",
									},
								},
							},
						},
					},
					err: []string{"(task_template / #1 (a) / cache): cache path 0: cannot be empty"},
				},
				{
					name: "template_cipd_package",
					cfg: &configpb.PoolsCfg{
						TaskTemplate: []*configpb.TaskTemplate{
							{
								Name: "a",
								CipdPackage: []*configpb.TaskTemplate_CipdPackage{
									{
										Pkg:  "a",
										Path: "c/d",
									},
								},
							},
						},
					},
					err: []string{"(task_template / #1 (a) / cipd_package): package 0 (a): version: required"},
				},
				{
					name: "template_cache_package_conflict_on_path",
					cfg: &configpb.PoolsCfg{
						TaskTemplate: []*configpb.TaskTemplate{
							{
								Name: "a",
								Cache: []*configpb.TaskTemplate_CacheEntry{
									{
										Name: "a",
										Path: "a/b",
									},
								},
								CipdPackage: []*configpb.TaskTemplate_CipdPackage{
									{
										Pkg:     "a",
										Version: "latest",
										Path:    "a/b",
									},
								},
							},
						},
					},
					err: []string{(`(task_template / #1 (a) / cipd_package):` +
						` "a/b": directory has conflicting owners: task_template_cache:a[]` +
						` and task_template_cipd_package[a:latest]`)},
				},
				{
					name: "template_env_var_empty",
					cfg: &configpb.PoolsCfg{
						TaskTemplate: []*configpb.TaskTemplate{
							{
								Name: "a",
								Env: []*configpb.TaskTemplate_Env{
									{
										Value: "a",
									},
								},
							},
						},
					},
					err: []string{"(task_template / #1 (a) / env / #1 ): var is empty"},
				},
				{
					name: "template_env_too_many",
					cfg: &configpb.PoolsCfg{
						TaskTemplate: []*configpb.TaskTemplate{
							{
								Name: "a",
								Env: []*configpb.TaskTemplate_Env{
									{
										Var:   "a",
										Value: strings.Repeat("a", validate.MaxEnvValueLength+1),
									},
								},
							},
						},
					},
					err: []string{fmt.Sprintf("(task_template / #1 (a) / env / #1 a): value: too long %q: 1025 > 1024", strings.Repeat("a", validate.MaxEnvValueLength+1))},
				},
				{
					name: "template_env_prefix",
					cfg: &configpb.PoolsCfg{
						TaskTemplate: []*configpb.TaskTemplate{
							{
								Name: "a",
								Env: []*configpb.TaskTemplate_Env{
									{
										Var:    "a",
										Prefix: []string{"a/../b"},
									},
								},
							},
						},
					},
					err: []string{`(task_template / #1 (a) / env / #1 a): prefix: "a/../b" is not normalized. Normalized is "b"`},
				},
				// task template inclusions
				{
					name: "template_duplicate",
					cfg: &configpb.PoolsCfg{
						TaskTemplate: []*configpb.TaskTemplate{
							{
								Name: "a",
							},
							{
								Name: "a",
							},
						},
					},
					err: []string{`(task_template / resolve inclusion): template "a" was already declared`},
				},
				{
					name: "template_include_self",
					cfg: &configpb.PoolsCfg{
						TaskTemplate: []*configpb.TaskTemplate{
							{
								Name: "a",
								Include: []string{
									"a",
								},
							},
						},
					},
					err: []string{`(task_template / resolve inclusion): template "a" includes self`},
				},
				{
					name: "template_include_unknown",
					cfg: &configpb.PoolsCfg{
						TaskTemplate: []*configpb.TaskTemplate{
							{
								Name: "a",
								Include: []string{
									"b",
								},
							},
						},
					},
					err: []string{`(task_template / resolve inclusion): unknown template "b"`},
				},
				{
					name: "template_include_duplicated",
					cfg: &configpb.PoolsCfg{
						TaskTemplate: []*configpb.TaskTemplate{
							{
								Name: "a",
								Include: []string{
									"b",
									"b",
								},
							},
							{
								Name: "b",
							},
						},
					},
					err: []string{`(task_template / resolve inclusion): template "a" already includes "b"`},
				},
				{
					name: "template_include_cycle",
					cfg: &configpb.PoolsCfg{
						TaskTemplate: []*configpb.TaskTemplate{
							{
								Name: "a",
								Include: []string{
									"b",
								},
							},
							{
								Name: "b",
								Include: []string{
									"c",
								},
							},
							{
								Name: "c",
								Include: []string{
									"a",
								},
							},
						},
					},
					err: []string{`(task_template / resolve inclusion): encounter inclusion cycle for template "a"`},
				},
				{
					name: "template_diamond_inclusion",
					cfg: &configpb.PoolsCfg{
						TaskTemplate: []*configpb.TaskTemplate{
							{
								Name: "a",
								Include: []string{
									"b",
									"c",
								},
							},
							{
								Name: "b",
								Include: []string{
									"c",
								},
							},
							{
								Name: "c",
							},
						},
					},
					err: []string{`(task_template / resolve inclusion): template "a" already includes "c"`},
				},
				// conflicts from inclusion
				{
					name: "template_invalid_after_resolve",
					cfg: &configpb.PoolsCfg{
						TaskTemplate: []*configpb.TaskTemplate{
							{
								Name: "a",
								Include: []string{
									"b",
								},
								Cache: []*configpb.TaskTemplate_CacheEntry{
									{
										Name: "a",
										Path: "a/b",
									},
								},
							},
							{
								Name: "b",
								CipdPackage: []*configpb.TaskTemplate_CipdPackage{
									{
										Pkg:     "a",
										Version: "latest",
										Path:    "a/b",
									},
								},
							},
						},
					},
					err: []string{
						`(task_template / resolve inclusion / #1 (a) / ` +
							`cipd_package): "a/b": directory has conflicting owners:` +
							` task_template_cache:a[] and task_template_cipd_package[a:latest]`},
				},
				// task_template_deployment
				{
					name: "deployment_no_name",
					cfg: &configpb.PoolsCfg{
						TaskTemplateDeployment: []*configpb.TaskTemplateDeployment{
							{
								Prod: &configpb.TaskTemplate{},
							},
						},
					},
					err: []string{"(task_template_deployment / #1 ()): name is empty"},
				},
				{
					name: "deployment_duplicate",
					cfg: &configpb.PoolsCfg{
						TaskTemplateDeployment: []*configpb.TaskTemplateDeployment{
							{
								Name: "a",
								Prod: &configpb.TaskTemplate{},
							},
							{
								Name: "a",
								Prod: &configpb.TaskTemplate{},
							},
						},
					},
					err: []string{`(task_template_deployment / #2 (a)): deployment "a" was already declared`},
				},
				{
					name: "deployment_no_prod",
					cfg: &configpb.PoolsCfg{
						TaskTemplateDeployment: []*configpb.TaskTemplateDeployment{
							{
								Name: "a",
							},
						},
					},
					err: []string{"(task_template_deployment / #1 (a) / prod): required"},
				},
				{
					name: "deployment_canary_chance_without_canary",
					cfg: &configpb.PoolsCfg{
						TaskTemplateDeployment: []*configpb.TaskTemplateDeployment{
							{
								Name:         "a",
								CanaryChance: 1000,
								Prod:         &configpb.TaskTemplate{},
							},
						},
					},
					err: []string{`(task_template_deployment / #1 (a)): canary_chance specified without a canary`},
				},
				{
					name: "deployment_canary_chance_out_of_range",
					cfg: &configpb.PoolsCfg{
						TaskTemplateDeployment: []*configpb.TaskTemplateDeployment{
							{
								Name:         "a",
								CanaryChance: 10000,
								Prod:         &configpb.TaskTemplate{},
								Canary:       &configpb.TaskTemplate{},
							},
						},
					},
					err: []string{`(task_template_deployment / #1 (a)): canary_chance out of range [0,9999]`},
				},
				{
					name: "deployment_prod_with_name",
					cfg: &configpb.PoolsCfg{
						TaskTemplateDeployment: []*configpb.TaskTemplateDeployment{
							{
								Name: "a",
								Prod: &configpb.TaskTemplate{
									Name: "b",
								},
							},
						},
					},
					err: []string{`(task_template_deployment / #1 (a) / prod): name cannot be specified`},
				},
				{
					name: "deployment_prod_include_unknown_template",
					cfg: &configpb.PoolsCfg{
						TaskTemplateDeployment: []*configpb.TaskTemplateDeployment{
							{
								Name: "a",
								Prod: &configpb.TaskTemplate{
									Include: []string{
										"unknown",
									},
								},
							},
						},
					},
					err: []string{`(task_template_deployment / #1 (a) / prod): includes unknown template "unknown"`},
				},
				{
					name: "deployment_prod_include_duplicated_template",
					cfg: &configpb.PoolsCfg{
						TaskTemplate: []*configpb.TaskTemplate{
							{
								Name: "t1",
								Include: []string{
									"t2",
								},
							},
							{
								Name: "t2",
							},
						},
						TaskTemplateDeployment: []*configpb.TaskTemplateDeployment{
							{
								Name: "a",
								Prod: &configpb.TaskTemplate{
									Include: []string{
										"t1",
										"t2",
									},
								},
							},
						},
					},
					err: []string{`(task_template_deployment / #1 (a) / prod): template already includes "t2"`},
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *ftt.Test) {
					tc.cfg.DefaultExternalServices = &configpb.ExternalServices{
						Cipd: &configpb.ExternalServices_CIPD{
							Server: "https://cipd.example.com",
							ClientPackage: &configpb.CipdPackage{
								PackageName: "client/pkg",
								Version:     "latest",
							},
						},
					}
					for i := range tc.err {
						tc.err[i] = `in "pools.cfg" ` + tc.err[i]
					}
					assert.Loosely(t, call(tc.cfg), should.Match(tc.err))
				})
			}
		})
	})
}

func TestPoolRBEConfig(t *testing.T) {
	t.Parallel()

	call := func(botID string, cfg *configpb.Pool_RBEMigration) RBEConfig {
		pool, err := newPool(&configpb.Pool{
			Name:         []string{"pool"},
			RbeMigration: cfg,
		}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return pool.rbeConfig(botID)
	}

	t.Run("edge cases", func(t *testing.T) {
		assert.That(t, call("bot", nil), should.Equal(RBEConfig{
			Mode: configpb.Pool_RBEMigration_BotModeAllocation_SWARMING,
		}))

		allRBE := call("bot", &configpb.Pool_RBEMigration{
			RbeInstance: "some-instance",
			BotModeAllocation: []*configpb.Pool_RBEMigration_BotModeAllocation{
				{Mode: configpb.Pool_RBEMigration_BotModeAllocation_RBE, Percent: 100},
			},
		})
		assert.That(t, allRBE, should.Equal(RBEConfig{
			Instance: "some-instance",
			Mode:     configpb.Pool_RBEMigration_BotModeAllocation_RBE,
		}))
	})

	t.Run("distribution", func(t *testing.T) {
		perMode := map[configpb.Pool_RBEMigration_BotModeAllocation_BotMode]int{}
		for i := range 1000 {
			cfg := call(fmt.Sprintf("bot-%d", i), &configpb.Pool_RBEMigration{
				RbeInstance: "some-instance",
				BotModeAllocation: []*configpb.Pool_RBEMigration_BotModeAllocation{
					{Mode: configpb.Pool_RBEMigration_BotModeAllocation_SWARMING, Percent: 20},
					{Mode: configpb.Pool_RBEMigration_BotModeAllocation_HYBRID, Percent: 10},
					{Mode: configpb.Pool_RBEMigration_BotModeAllocation_RBE, Percent: 70},
				},
			})
			perMode[cfg.Mode] += 1
		}
		assert.That(t, perMode, should.Match(map[configpb.Pool_RBEMigration_BotModeAllocation_BotMode]int{
			configpb.Pool_RBEMigration_BotModeAllocation_SWARMING: 216,
			configpb.Pool_RBEMigration_BotModeAllocation_HYBRID:   108,
			configpb.Pool_RBEMigration_BotModeAllocation_RBE:      676,
		}))
	})
}

func TestNewInclusionGraph(t *testing.T) {
	t.Parallel()

	ftt.Run("newInclusionGraph", t, func(t *ftt.Test) {
		inclusions := func(g *inclusionGraph) map[string][]string {
			res := make(map[string][]string, len(g.flattened))
			for name, node := range g.flattened {
				res[name] = node.tmp.Include
			}
			return res
		}

		testCases := []struct {
			name string
			cfg  *configpb.PoolsCfg
			res  map[string][]string
		}{
			{
				name: "empty",
				cfg:  &configpb.PoolsCfg{},
				res:  map[string][]string{},
			},
			{
				name: "single",
				cfg: &configpb.PoolsCfg{
					TaskTemplate: []*configpb.TaskTemplate{
						{
							Name: "t",
						},
					},
				},
				res: map[string][]string{
					"t": {"t"},
				},
			},
			{
				name: "simple",
				cfg: &configpb.PoolsCfg{
					TaskTemplate: []*configpb.TaskTemplate{
						{
							Name: "t1",
							Include: []string{
								"t2",
							},
						},
						{
							Name: "t2",
						},
					},
				},
				res: map[string][]string{
					"t1": {"t1", "t2"},
					"t2": {"t2"},
				},
			},
			{
				name: "tree",
				cfg: &configpb.PoolsCfg{
					TaskTemplate: []*configpb.TaskTemplate{
						{
							Name: "t1",
							Include: []string{
								"t2",
								"t3",
							},
						},
						{
							Name: "t2",
							Include: []string{
								"t5",
							},
						},
						{
							Name: "t3",
							Include: []string{
								"t4",
							},
						},
						{
							Name: "t4",
						},
						{
							Name: "t5",
						},
					},
				},
				res: map[string][]string{
					"t1": {"t1", "t2", "t5", "t3", "t4"},
					"t2": {"t2", "t5"},
					"t3": {"t3", "t4"},
					"t4": {"t4"},
					"t5": {"t5"},
				},
			},
			{
				name: "forest",
				cfg: &configpb.PoolsCfg{
					TaskTemplate: []*configpb.TaskTemplate{
						{
							Name: "t1",
							Include: []string{
								"t3",
							},
						},
						{
							Name: "t2",
							Include: []string{
								"t4",
							},
						},
						{
							Name: "t3",
							Include: []string{
								"t4",
							},
						},
						{
							Name: "t4",
						},
					},
				},
				res: map[string][]string{
					"t1": {"t1", "t3", "t4"},
					"t2": {"t2", "t4"},
					"t3": {"t3", "t4"},
					"t4": {"t4"},
				},
			},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *ftt.Test) {
				graph, merr := newInclusionGraph(tc.cfg.TaskTemplate)
				assert.That(t, merr.AsError(), should.ErrLike(nil))
				assert.Loosely(t, inclusions(graph), should.Match(tc.res))
			})
		}
	})
}

func TestResolveTemplates(t *testing.T) {
	t.Parallel()
	ftt.Run("Works", t, func(t *ftt.Test) {
		t.Run("simple inclusion", func(t *ftt.Test) {
			cfg := &configpb.PoolsCfg{
				TaskTemplate: []*configpb.TaskTemplate{
					{
						Name: "a",
						Include: []string{
							"b",
						},
					},
					{
						Name:        "b",
						Cache:       goodTaskTemplate.Cache,
						CipdPackage: goodTaskTemplate.CipdPackage,
						Env:         goodTaskTemplate.Env,
					},
					{
						Name: "c",
						Include: []string{
							"b",
						},
					},
				},
			}
			graph, merr := newInclusionGraph(cfg.TaskTemplate)
			assert.That(t, merr.AsError(), should.ErrLike(nil))
			err := graph.flattenTaskTemplates()
			assert.That(t, err, should.ErrLike(nil))
			tmpMap := graph.flattened
			assert.That(t, tmpMap["a"].tmp.Cache, should.Match(goodTaskTemplate.Cache))
			assert.That(t, tmpMap["a"].tmp.CipdPackage, should.Match(goodTaskTemplate.CipdPackage))
			assert.That(t, tmpMap["a"].tmp.Env, should.Match(goodTaskTemplate.Env))
			assert.That(t, tmpMap["c"].tmp.Cache, should.Match(goodTaskTemplate.Cache))
			assert.That(t, tmpMap["c"].tmp.CipdPackage, should.Match(goodTaskTemplate.CipdPackage))
			assert.That(t, tmpMap["c"].tmp.Env, should.Match(goodTaskTemplate.Env))
		})

		t.Run("Add fields from included templates", func(t *ftt.Test) {
			cfg := &configpb.PoolsCfg{
				TaskTemplate: []*configpb.TaskTemplate{
					{
						Name: "a",
						Include: []string{
							"b",
						},
						Cache: []*configpb.TaskTemplate_CacheEntry{
							{
								Name: "c1",
								Path: "a/b",
							},
						},
						CipdPackage: []*configpb.TaskTemplate_CipdPackage{
							{
								Pkg:     "p1",
								Version: "latest",
								Path:    "c/d",
							},
						},
						Env: []*configpb.TaskTemplate_Env{
							{
								Var:   "ev1",
								Value: "ev1",
								Prefix: []string{
									"e/f",
								},
								Soft: true,
							},
						},
					},
					{
						Name: "b",
						Cache: []*configpb.TaskTemplate_CacheEntry{
							{
								Name: "c2",
								Path: "a/c",
							},
						},
						CipdPackage: []*configpb.TaskTemplate_CipdPackage{
							{
								Pkg:     "p2",
								Version: "latest",
								Path:    "c/f",
							},
						},
						Env: []*configpb.TaskTemplate_Env{
							{
								Var:   "ev2",
								Value: "ev2",
								Prefix: []string{
									"e/f",
								},
								Soft: true,
							},
						},
					},
				},
			}
			graph, merr := newInclusionGraph(cfg.TaskTemplate)
			assert.That(t, merr.AsError(), should.ErrLike(nil))
			err := graph.flattenTaskTemplates()
			assert.That(t, err, should.ErrLike(nil))
			tmpMap := graph.flattened
			expected := &configpb.TaskTemplate{
				Name: "a",
				Include: []string{
					"a",
					"b",
				},
				Cache: []*configpb.TaskTemplate_CacheEntry{
					{
						Name: "c1",
						Path: "a/b",
					},
					{
						Name: "c2",
						Path: "a/c",
					},
				},
				CipdPackage: []*configpb.TaskTemplate_CipdPackage{
					{
						Pkg:     "p1",
						Version: "latest",
						Path:    "c/d",
					},
					{
						Pkg:     "p2",
						Version: "latest",
						Path:    "c/f",
					},
				},
				Env: []*configpb.TaskTemplate_Env{
					{
						Var:   "ev1",
						Value: "ev1",
						Prefix: []string{
							"e/f",
						},
						Soft: true,
					},
					{
						Var:   "ev2",
						Value: "ev2",
						Prefix: []string{
							"e/f",
						},
						Soft: true,
					},
				},
			}
			assert.That(t, tmpMap["a"].tmp, should.Match(expected))
			assert.That(t, tmpMap["b"].tmp.Include, should.Match([]string{"b"}))
		})
		t.Run("override fields from included templates", func(t *ftt.Test) {
			cfg := &configpb.PoolsCfg{
				TaskTemplate: []*configpb.TaskTemplate{
					{
						Name: "a",
						Include: []string{
							"b",
						},
						Cache: []*configpb.TaskTemplate_CacheEntry{
							{
								Name: "c1",
								Path: "a/b",
							},
						},
						CipdPackage: []*configpb.TaskTemplate_CipdPackage{
							{
								Pkg:     "p1",
								Version: "prod",
								Path:    "c/d",
							},
							{
								Pkg:     "p2",
								Version: "prod",
								Path:    "c/f",
							},
						},
						Env: []*configpb.TaskTemplate_Env{
							{
								Var:   "ev1",
								Value: "ev1",
								Prefix: []string{
									"e/f",
								},
								Soft: true,
							},
						},
					},
					{
						Name: "b",
						Cache: []*configpb.TaskTemplate_CacheEntry{
							{
								Name: "c1",
								Path: "a/c",
							},
						},
						CipdPackage: []*configpb.TaskTemplate_CipdPackage{
							// Same (Pkg, Path) as base, will be overridden.
							{
								Pkg:     "p1",
								Version: "latest",
								Path:    "c/d",
							},
							// Same Pkg different Path from base, will not be
							// overridden.
							{
								Pkg:     "p2",
								Version: "prod",
								Path:    "c/g",
							},
						},
						Env: []*configpb.TaskTemplate_Env{
							{
								Var:   "ev1",
								Value: "ev2",
								Prefix: []string{
									"e/g",
								},
								Soft: true,
							},
						},
					},
				},
			}
			graph, merr := newInclusionGraph(cfg.TaskTemplate)
			assert.That(t, merr.AsError(), should.ErrLike(nil))
			err := graph.flattenTaskTemplates()
			assert.That(t, err, should.ErrLike(nil))
			tmpMap := graph.flattened
			expected := &configpb.TaskTemplate{
				Name: "a",
				Include: []string{
					"a",
					"b",
				},
				Cache: []*configpb.TaskTemplate_CacheEntry{
					{
						Name: "c1",
						Path: "a/b",
					},
				},
				CipdPackage: []*configpb.TaskTemplate_CipdPackage{
					{
						Pkg:     "p1",
						Version: "prod",
						Path:    "c/d",
					},
					{
						Pkg:     "p2",
						Version: "prod",
						Path:    "c/f",
					},
					{
						Pkg:     "p2",
						Version: "prod",
						Path:    "c/g",
					},
				},
				Env: []*configpb.TaskTemplate_Env{
					{
						Var:   "ev1",
						Value: "ev1",
						Prefix: []string{
							"e/f",
							"e/g",
						},
						Soft: true,
					},
				},
			}
			assert.That(t, tmpMap["a"].tmp, should.Match(expected))
		})
		t.Run("merge_from_multiple_templates", func(t *ftt.Test) {
			cfg := &configpb.PoolsCfg{
				TaskTemplate: []*configpb.TaskTemplate{
					{
						Name: "a",
						Include: []string{
							"b",
							"c",
						},
					},
					{
						Name: "b",
						Include: []string{
							"d",
						},
						Cache: []*configpb.TaskTemplate_CacheEntry{
							{
								Name: "c1",
								Path: "a/b",
							},
						},
					},
					{
						Name: "c",
						Cache: []*configpb.TaskTemplate_CacheEntry{
							{
								Name: "c1",
								Path: "a/c",
							},
						},
					},
					{
						Name: "d",
						Cache: []*configpb.TaskTemplate_CacheEntry{
							{
								Name: "c1",
								Path: "a/d",
							},
							{
								Name: "c2",
								Path: "a/d2",
							},
						},
					},
				},
			}
			graph, merr := newInclusionGraph(cfg.TaskTemplate)
			assert.That(t, merr.AsError(), should.ErrLike(nil))
			err := graph.flattenTaskTemplates()
			assert.That(t, err, should.ErrLike(nil))
			tmpMap := graph.flattened
			expected := &configpb.TaskTemplate{
				Name: "a",
				Include: []string{
					"a",
					"b",
					"d",
					"c",
				},
				Cache: []*configpb.TaskTemplate_CacheEntry{
					{
						Name: "c1",
						Path: "a/b",
					},
					{
						Name: "c2",
						Path: "a/d2",
					},
				},
			}
			assert.That(t, tmpMap["a"].tmp, should.Match(expected))
		})
	})
}
