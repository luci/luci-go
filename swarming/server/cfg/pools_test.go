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
			Name:         "d1",
			Prod:         goodTaskTemplate,
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
		},
	},
}

func TestNewPoolsConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		poolsCfg, err := newPoolsConfig(goodPoolsCfg)
		assert.Loosely(t, err, should.BeNil)
		pools := poolsCfg.pools
		assert.Loosely(t, pools, should.HaveLength(3))
		assert.Loosely(t, pools["a"].Realm, should.Equal("test:1"))
		assert.Loosely(t, pools["b"].Realm, should.Equal("test:2"))
		assert.Loosely(t, pools["c"].Realm, should.Equal("test:2"))

		assert.Loosely(t, pools["a"].DefaultTaskRealm, should.Equal("test:default"))
		assert.Loosely(t, pools["b"].DefaultTaskRealm, should.BeEmpty)
		assert.Loosely(t, pools["c"].DefaultTaskRealm, should.BeEmpty)
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
		t.Run("pool", func(t *ftt.Test) {
			onePool := func(p *configpb.Pool) *configpb.PoolsCfg {
				return &configpb.PoolsCfg{
					Pool: []*configpb.Pool{p},
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
							},
						},
					}),
					err: "(pool #1 (a) / task_template_deployment_inline): name cannot be specified",
				},
			}
			for _, cs := range testCases {
				assert.Loosely(t, call(cs.cfg), should.Resemble([]string{`in "pools.cfg" ` + cs.err}))
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
					err: []string{`(task_template / #2 (a)): template "a" was already declared`},
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
					err: []string{(`(task_template / #1 (a) / cipd_package): package 0 (a): path` +
						` "a/b" is mapped to a named cache and cannot be a target of CIPD installation`)},
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
				// task_template_deployment
				{
					name: "deployment_no_name",
					cfg: &configpb.PoolsCfg{
						TaskTemplateDeployment: []*configpb.TaskTemplateDeployment{
							{},
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
							},
							{
								Name: "a",
							},
						},
					},
					err: []string{`(task_template_deployment / #2 (a)): deployment "a" was already declared`},
				},
				{
					name: "deployment_canary_chance_without_canary",
					cfg: &configpb.PoolsCfg{
						TaskTemplateDeployment: []*configpb.TaskTemplateDeployment{
							{
								Name:         "a",
								CanaryChance: 1000,
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
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *ftt.Test) {
					for i := range tc.err {
						tc.err[i] = `in "pools.cfg" ` + tc.err[i]
					}
					assert.Loosely(t, call(tc.cfg), should.Resemble(tc.err))
				})
			}
		})
	})
}
