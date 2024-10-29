// Copyright 2018 The LUCI Authors.
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

package validation

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	apipb "go.chromium.org/luci/cv/api/v1"
	"go.chromium.org/luci/cv/internal/configs/srvcfg"
	"go.chromium.org/luci/cv/internal/cvtesting"
	listenerpb "go.chromium.org/luci/cv/settings/listener"
)

func mockListenerSettings(ctx context.Context, hosts ...string) error {
	var subs []*listenerpb.Settings_GerritSubscription
	for _, h := range hosts {
		subs = append(subs, &listenerpb.Settings_GerritSubscription{Host: h})
	}
	return srvcfg.SetTestListenerConfig(ctx, &listenerpb.Settings{GerritSubscriptions: subs}, nil)
}

func TestValidateProjectHighLevel(t *testing.T) {
	t.Parallel()
	const project = "proj"

	ftt.Run("ValidateProject works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		cfg := cfgpb.Config{}
		vctx := &validation.Context{Context: ctx}
		assert.Loosely(t, prototext.Unmarshal([]byte(validConfigTextPB), &cfg), should.BeNil)
		assert.Loosely(t, mockListenerSettings(ctx, "chromium-review.googlesource.com"), should.BeNil)

		t.Run("OK", func(t *ftt.Test) {
			assert.Loosely(t, ValidateProject(vctx, &cfg, project), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
		})
		t.Run("Error", func(t *ftt.Test) {
			cfg.GetConfigGroups()[0].Name = "!invalid! name"
			assert.Loosely(t, ValidateProject(vctx, &cfg, project), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike("must match"))
		})
	})

	ftt.Run("ValidateProjectConfig works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		cfg := cfgpb.Config{}
		vctx := &validation.Context{Context: ctx}
		assert.Loosely(t, prototext.Unmarshal([]byte(validConfigTextPB), &cfg), should.BeNil)

		t.Run("OK", func(t *ftt.Test) {
			assert.Loosely(t, ValidateProjectConfig(vctx, &cfg), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
		})
		t.Run("Error", func(t *ftt.Test) {
			cfg.GetConfigGroups()[0].Name = "!invalid! name"
			assert.Loosely(t, ValidateProject(vctx, &cfg, project), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike("must match"))
		})
	})
}

const validConfigTextPB = `
	cq_status_host: "chromium-cq-status.appspot.com"
	submit_options {
		max_burst: 2
		burst_delay { seconds: 120 }
	}
	config_groups {
		name: "test"
		gerrit {
			url: "https://chromium-review.googlesource.com"
			projects {
				name: "chromium/src"
				ref_regexp: "refs/heads/.+"
				ref_regexp_exclude: "refs/heads/excluded"
			}
		}
		verifiers {
			tree_status { url: "https://chromium-status.appspot.com" }
			gerrit_cq_ability { committer_list: "project-chromium-committers" }
			tryjob {
				retry_config {
					single_quota: 1
					global_quota: 2
					failure_weight: 1
					transient_failure_weight: 1
					timeout_weight: 1
				}
				builders {
					name: "chromium/try/linux"
					cancel_stale: NO
				}
			}
		}
	}
`

func TestValidateProjectDetailed(t *testing.T) {
	t.Parallel()

	const (
		configSet = "projects/foo"
		project   = "foo"
		path      = "cq.cfg"
	)

	ftt.Run("Validate Config", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		vctx := &validation.Context{Context: ctx}
		validateProjectConfig := func(vctx *validation.Context, cfg *cfgpb.Config) {
			vd, err := makeProjectConfigValidator(vctx, project)
			assert.NoErr(t, err)
			vd.validateProjectConfig(cfg)
		}

		t.Run("Loading bad proto", func(t *ftt.Test) {
			content := []byte(` bad: "config" `)
			assert.Loosely(t, validateProject(vctx, configSet, path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("unknown field"))
		})

		// It's easier to manipulate Go struct than text.
		cfg := cfgpb.Config{}
		assert.Loosely(t, prototext.Unmarshal([]byte(validConfigTextPB), &cfg), should.BeNil)
		assert.Loosely(t, mockListenerSettings(ctx, "chromium-review.googlesource.com"), should.BeNil)

		t.Run("OK", func(t *ftt.Test) {
			t.Run("good proto, good config", func(t *ftt.Test) {
				assert.Loosely(t, validateProject(vctx, configSet, path, []byte(validConfigTextPB)), should.BeNil)
				assert.Loosely(t, vctx.Finalize(), should.BeNil)
			})
			t.Run("good config", func(t *ftt.Test) {
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.BeNil)
			})
		})

		t.Run("Missing gerrit subscription", func(t *ftt.Test) {
			// reset the listener settings to make the validation fail.
			assert.Loosely(t, mockListenerSettings(ctx), should.BeNil)

			t.Run("validation fails", func(t *ftt.Test) {
				assert.Loosely(t, validateProject(vctx, configSet, path, []byte(validConfigTextPB)), should.BeNil)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike("Gerrit pub/sub"))
			})
			t.Run("OK if the project is disabled in listener settings", func(t *ftt.Test) {
				ct.DisableProjectInGerritListener(ctx, project)
				assert.Loosely(t, validateProject(vctx, configSet, path, []byte(validConfigTextPB)), should.BeNil)
			})
		})
		assert.Loosely(t, mockListenerSettings(ctx, "chromium-review.googlesource.com"), should.BeNil)

		t.Run("Top-level config", func(t *ftt.Test) {
			t.Run("Top level opts can be omitted", func(t *ftt.Test) {
				cfg.CqStatusHost = ""
				cfg.SubmitOptions = nil
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.BeNil)
			})
			t.Run("draining time not allowed crbug/1208569", func(t *ftt.Test) {
				cfg.DrainingStartTime = "2017-12-23T15:47:58Z"
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike(`https://crbug.com/1208569`))
			})
			t.Run("CQ status host can be internal", func(t *ftt.Test) {
				cfg.CqStatusHost = CQStatusHostInternal
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.BeNil)
			})
			t.Run("CQ status host can be empty", func(t *ftt.Test) {
				cfg.CqStatusHost = ""
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.BeNil)
			})
			t.Run("CQ status host can be public", func(t *ftt.Test) {
				cfg.CqStatusHost = CQStatusHostPublic
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.BeNil)
			})
			t.Run("CQ status host can not be something else", func(t *ftt.Test) {
				cfg.CqStatusHost = "nope.example.com"
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike("cq_status_host must be"))
			})
			t.Run("Bad max_burst", func(t *ftt.Test) {
				cfg.SubmitOptions.MaxBurst = -1
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.NotBeNil)
			})
			t.Run("Bad burst_delay ", func(t *ftt.Test) {
				cfg.SubmitOptions.BurstDelay.Seconds = -1
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.NotBeNil)
			})
			t.Run("config_groups", func(t *ftt.Test) {
				orig := cfg.ConfigGroups[0]
				add := func(refRegexps ...string) {
					// Add new regexps sequence with constant valid gerrit url and
					// project and the same valid verifiers.
					cfg.ConfigGroups = append(cfg.ConfigGroups, &cfgpb.ConfigGroup{
						Name: fmt.Sprintf("group-%d", len(cfg.ConfigGroups)),
						Gerrit: []*cfgpb.ConfigGroup_Gerrit{
							{
								Url: orig.Gerrit[0].Url,
								Projects: []*cfgpb.ConfigGroup_Gerrit_Project{
									{
										Name:      orig.Gerrit[0].Projects[0].Name,
										RefRegexp: refRegexps,
									},
								},
							},
						},
						Verifiers: orig.Verifiers,
					})
				}

				t.Run("at least 1 Config Group", func(t *ftt.Test) {
					cfg.ConfigGroups = nil
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.ErrLike("at least 1 config_group is required"))
				})

				t.Run("at most 1 fallback", func(t *ftt.Test) {
					cfg.ConfigGroups = nil
					add("refs/heads/.+")
					cfg.ConfigGroups[0].Fallback = cfgpb.Toggle_YES
					add("refs/branch-heads/.+")
					cfg.ConfigGroups[1].Fallback = cfgpb.Toggle_YES
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.ErrLike("At most 1 config_group with fallback=YES allowed"))
				})

				t.Run("with unique names", func(t *ftt.Test) {
					cfg.ConfigGroups = nil
					add("refs/heads/.+")
					add("refs/branch-heads/.+")
					add("refs/other-heads/.+")
					t.Run("dups not allowed", func(t *ftt.Test) {
						cfg.ConfigGroups[0].Name = "aaa"
						cfg.ConfigGroups[1].Name = "bbb"
						cfg.ConfigGroups[2].Name = "bbb"
						validateProjectConfig(vctx, &cfg)
						assert.Loosely(t, vctx.Finalize(), should.ErrLike("duplicate config_group name \"bbb\" not allowed"))
					})
				})
			})
		})

		t.Run("ConfigGroups", func(t *ftt.Test) {
			t.Run("with no Name", func(t *ftt.Test) {
				cfg.ConfigGroups[0].Name = ""
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, mustError(t, vctx.Finalize()), should.ErrLike("name is required"))
			})
			t.Run("with valid Name", func(t *ftt.Test) {
				cfg.ConfigGroups[0].Name = "!invalid!"
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, mustError(t, vctx.Finalize()), should.ErrLike("name must match"))
			})
			t.Run("with Gerrit", func(t *ftt.Test) {
				cfg.ConfigGroups[0].Gerrit = nil
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike("at least 1 gerrit is required"))
			})
			t.Run("with Verifiers", func(t *ftt.Test) {
				cfg.ConfigGroups[0].Verifiers = nil
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike("verifiers are required"))
			})
			t.Run("no dup Gerrit blocks", func(t *ftt.Test) {
				cfg.ConfigGroups[0].Gerrit = append(cfg.ConfigGroups[0].Gerrit, cfg.ConfigGroups[0].Gerrit[0])
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike("duplicate gerrit url in the same config_group"))
			})
			t.Run("CombineCLs", func(t *ftt.Test) {
				cfg.ConfigGroups[0].CombineCls = &cfgpb.CombineCLs{}
				t.Run("Needs stabilization_delay", func(t *ftt.Test) {
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.ErrLike("stabilization_delay is required"))
				})
				cfg.ConfigGroups[0].CombineCls.StabilizationDelay = &durationpb.Duration{}
				t.Run("Needs stabilization_delay > 10s", func(t *ftt.Test) {
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.ErrLike("stabilization_delay must be at least 10 seconds"))
				})
				cfg.ConfigGroups[0].CombineCls.StabilizationDelay.Seconds = 20
				t.Run("OK", func(t *ftt.Test) {
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.BeNil)
				})
				t.Run("Can't use with allow_submit_with_open_deps", func(t *ftt.Test) {
					cfg.ConfigGroups[0].Verifiers.GerritCqAbility.AllowSubmitWithOpenDeps = true
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.ErrLike("allow_submit_with_open_deps=true"))
				})
			})

			mode := &cfgpb.Mode{
				Name:            "TEST_RUN",
				CqLabelValue:    1,
				TriggeringLabel: "TEST_RUN_LABEL",
				TriggeringValue: 2,
			}
			t.Run("Mode", func(t *ftt.Test) {
				cfg.ConfigGroups[0].AdditionalModes = []*cfgpb.Mode{mode}
				t.Run("OK", func(t *ftt.Test) {
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.BeNil)
				})
				t.Run("name", func(t *ftt.Test) {
					check := func(t testing.TB) {
						validateProjectConfig(vctx, &cfg)
						assert.Loosely(t, vctx.Finalize(), should.ErrLike("does not match regex pattern"))
					}
					t.Run("empty", func(t *ftt.Test) {
						mode.Name = ""
						check(t)
					})
					t.Run("with invalid chars", func(t *ftt.Test) {
						mode.Name = "~!Invalid Run Mode!~"
						check(t)
					})
				})

				t.Run("cq_label_value", func(t *ftt.Test) {
					check := func(t testing.TB) {
						validateProjectConfig(vctx, &cfg)
						assert.Loosely(t, vctx.Finalize(), should.ErrLike("must be in list [1 2]"))
					}

					t.Run("with -1", func(t *ftt.Test) {
						mode.CqLabelValue = -1
						check(t)
					})
					t.Run("with 0", func(t *ftt.Test) {
						mode.CqLabelValue = 0
						check(t)
					})
					t.Run("with 3", func(t *ftt.Test) {
						mode.CqLabelValue = 3
						check(t)
					})
					t.Run("with 10", func(t *ftt.Test) {
						mode.CqLabelValue = 10
						check(t)
					})
				})

				t.Run("triggering_label", func(t *ftt.Test) {
					t.Run("empty", func(t *ftt.Test) {
						mode.TriggeringLabel = ""
						validateProjectConfig(vctx, &cfg)
						assert.Loosely(t, vctx.Finalize(), should.ErrLike("length must be at least 1 runes"))
					})
					t.Run("with Commit-Queue", func(t *ftt.Test) {
						mode.TriggeringLabel = "Commit-Queue"
						validateProjectConfig(vctx, &cfg)
						assert.Loosely(t, vctx.Finalize(), should.ErrLike("must not be in list [Commit-Queue]"))
					})
				})

				t.Run("triggering_value", func(t *ftt.Test) {
					check := func(t testing.TB) {
						validateProjectConfig(vctx, &cfg)
						assert.Loosely(t, vctx.Finalize(), should.ErrLike("must be greater than 0"))
					}

					t.Run("with 0", func(t *ftt.Test) {
						mode.TriggeringValue = 0
						check(t)
					})
					t.Run("with -1", func(t *ftt.Test) {
						mode.TriggeringValue = -1
						check(t)
					})
				})
			})

			// Tests for additional mode specific verifiers.
			t.Run("additional_modes", func(t *ftt.Test) {
				cfg.ConfigGroups[0].AdditionalModes = []*cfgpb.Mode{mode}
				t.Run("duplicate names", func(t *ftt.Test) {
					cfg.ConfigGroups[0].AdditionalModes = []*cfgpb.Mode{mode, mode}
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.ErrLike(`"TEST_RUN" is already in use`))
				})
			})

			t.Run("post_actions", func(t *ftt.Test) {
				pa := &cfgpb.ConfigGroup_PostAction{
					Name: "CQ verified",
					Action: &cfgpb.ConfigGroup_PostAction_VoteGerritLabels_{
						VoteGerritLabels: &cfgpb.ConfigGroup_PostAction_VoteGerritLabels{
							Votes: []*cfgpb.ConfigGroup_PostAction_VoteGerritLabels_Vote{
								{
									Name:  "CQ-verified",
									Value: 1,
								},
							},
						},
					},
					Conditions: []*cfgpb.ConfigGroup_PostAction_TriggeringCondition{
						{
							Mode:     "DRY_RUN",
							Statuses: []apipb.Run_Status{apipb.Run_SUCCEEDED},
						},
					},
				}
				cfg.ConfigGroups[0].PostActions = []*cfgpb.ConfigGroup_PostAction{pa}

				t.Run("works", func(t *ftt.Test) {
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.BeNil)
				})

				t.Run("name", func(t *ftt.Test) {
					t.Run("missing", func(t *ftt.Test) {
						pa.Name = ""
						validateProjectConfig(vctx, &cfg)
						assert.Loosely(t, vctx.Finalize(), should.ErrLike("Name: value length must be at least 1"))
					})

					t.Run("duplicate", func(t *ftt.Test) {
						cfg.ConfigGroups[0].PostActions = append(cfg.ConfigGroups[0].PostActions,
							cfg.ConfigGroups[0].PostActions[0])
						validateProjectConfig(vctx, &cfg)
						assert.Loosely(t, vctx.Finalize(), should.ErrLike(`"CQ verified"' is already in use`))
					})
				})

				t.Run("action", func(t *ftt.Test) {
					t.Run("missing", func(t *ftt.Test) {
						pa.Action = nil
						validateProjectConfig(vctx, &cfg)
						assert.Loosely(t, vctx.Finalize(), should.ErrLike(`Action: value is required`))
					})
					t.Run("vote_gerrit_labels", func(t *ftt.Test) {
						w := pa.GetAction().(*cfgpb.ConfigGroup_PostAction_VoteGerritLabels_).VoteGerritLabels
						t.Run("empty pairs", func(t *ftt.Test) {
							w.Votes = nil
							validateProjectConfig(vctx, &cfg)
							assert.Loosely(t, vctx.Finalize(), should.ErrLike("Votes: value must contain"))
						})
						t.Run("a pair with an empty name", func(t *ftt.Test) {
							w.Votes[0].Name = ""
							validateProjectConfig(vctx, &cfg)
							assert.Loosely(t, vctx.Finalize(), should.ErrLike("Name: value length must be"))
						})
						t.Run("pairs with duplicate names", func(t *ftt.Test) {
							w.Votes = append(w.Votes, w.Votes[0])
							validateProjectConfig(vctx, &cfg)
							assert.Loosely(t, vctx.Finalize(), should.ErrLike(`"CQ-verified" already specified`))

						})
					})
				})

				t.Run("triggering_conditions", func(t *ftt.Test) {
					tc := pa.Conditions[0]
					t.Run("missing", func(t *ftt.Test) {
						pa.Conditions = nil
						validateProjectConfig(vctx, &cfg)
						assert.Loosely(t, vctx.Finalize(), should.ErrLike(`Conditions: value must contain at least 1`))
					})
					t.Run("mode", func(t *ftt.Test) {
						t.Run("missing", func(t *ftt.Test) {
							tc.Mode = ""
							validateProjectConfig(vctx, &cfg)
							assert.Loosely(t, vctx.Finalize(), should.ErrLike(`Mode: value length must be at least 1`))
						})

						cfg.ConfigGroups[0].AdditionalModes = []*cfgpb.Mode{mode}
						t.Run("with an existing additional mode", func(t *ftt.Test) {
							tc.Mode = mode.Name
							validateProjectConfig(vctx, &cfg)
							assert.Loosely(t, vctx.Finalize(), should.BeNil)
						})

						t.Run("with an non-existing additional mode", func(t *ftt.Test) {
							tc.Mode = "NON_EXISTING_RUN"
							validateProjectConfig(vctx, &cfg)
							assert.Loosely(t, vctx.Finalize(), should.ErrLike(`invalid mode "NON_EXISTING_RUN"`))
						})
					})
					t.Run("statuses", func(t *ftt.Test) {
						t.Run("missing", func(t *ftt.Test) {
							tc.Statuses = nil
							validateProjectConfig(vctx, &cfg)
							assert.Loosely(t, vctx.Finalize(), should.ErrLike(`Statuses: value must contain at least 1`))
						})
						t.Run("non-terminal status", func(t *ftt.Test) {
							tc.Statuses = []apipb.Run_Status{
								apipb.Run_SUCCEEDED,
								apipb.Run_PENDING,
							}
							validateProjectConfig(vctx, &cfg)
							assert.Loosely(t, vctx.Finalize(), should.ErrLike(`"PENDING" is not a terminal status`))
						})
						t.Run("duplicates", func(t *ftt.Test) {
							tc.Statuses = []apipb.Run_Status{
								apipb.Run_SUCCEEDED,
								apipb.Run_SUCCEEDED,
							}
							validateProjectConfig(vctx, &cfg)
							assert.Loosely(t, vctx.Finalize(), should.ErrLike(`"SUCCEEDED" was specified already`))
						})
					})
				})
			})
		})

		t.Run("tryjob_experiments", func(t *ftt.Test) {
			exp := &cfgpb.ConfigGroup_TryjobExperiment{
				Name: "infra.experiment.foo",
				Condition: &cfgpb.ConfigGroup_TryjobExperiment_Condition{
					OwnerGroupAllowlist: []string{"googlers"},
				},
			}
			cfg.ConfigGroups[0].TryjobExperiments = []*cfgpb.ConfigGroup_TryjobExperiment{exp}

			t.Run("works", func(t *ftt.Test) {
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.BeNil)
			})

			t.Run("name", func(t *ftt.Test) {
				t.Run("missing", func(t *ftt.Test) {
					exp.Name = ""
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.ErrLike("Name: value length must be at least 1"))
				})

				t.Run("duplicate", func(t *ftt.Test) {
					cfg.ConfigGroups[0].TryjobExperiments = []*cfgpb.ConfigGroup_TryjobExperiment{exp, exp}
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.ErrLike(`duplicate name "infra.experiment.foo"`))
				})

				t.Run("invalid name", func(t *ftt.Test) {
					exp.Name = "^&*()"
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.ErrLike(`"^&*()" does not match`))
				})
			})

			t.Run("Condition", func(t *ftt.Test) {
				t.Run("owner_group_allowlist has empty string", func(t *ftt.Test) {
					exp.Condition.OwnerGroupAllowlist = []string{"infra.chromium.foo", ""}
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.ErrLike("OwnerGroupAllowlist[1]: value length must be at least 1 "))
				})
			})
		})

		t.Run("Gerrit", func(t *ftt.Test) {
			g := cfg.ConfigGroups[0].Gerrit[0]
			t.Run("needs valid URL", func(t *ftt.Test) {
				g.Url = ""
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike("url is required"))

				g.Url = ":badscheme, bad URL"
				vctx = &validation.Context{Context: ctx}
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike("failed to parse url"))
			})

			t.Run("without fancy URL components", func(t *ftt.Test) {
				g.Url = "bad://ok/path-not-good?query=too#neither-is-fragment"
				validateProjectConfig(vctx, &cfg)
				err := vctx.Finalize()
				assert.Loosely(t, err, should.ErrLike("path component not yet allowed in url"))
				assert.Loosely(t, err, should.ErrLike("and 5 other errors"))
			})

			t.Run("current limitations", func(t *ftt.Test) {
				g.Url = "https://not.yet.allowed.com"
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike("only *.googlesource.com hosts supported for now"))

				vctx = &validation.Context{Context: ctx}
				g.Url = "new-scheme://chromium-review.googlesource.com"
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike("only 'https' scheme supported for now"))
			})
			t.Run("at least 1 project required", func(t *ftt.Test) {
				g.Projects = nil
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike("at least 1 project is required"))
			})
			t.Run("no dup project blocks", func(t *ftt.Test) {
				g.Projects = append(g.Projects, g.Projects[0])
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike("duplicate project in the same gerrit"))
			})
		})

		t.Run("Gerrit Project", func(t *ftt.Test) {
			p := cfg.ConfigGroups[0].Gerrit[0].Projects[0]
			t.Run("project name required", func(t *ftt.Test) {
				p.Name = ""
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike("name is required"))
			})
			t.Run("incorrect project names", func(t *ftt.Test) {
				p.Name = "a/prefix-not-allowed/so-is-.git-suffix/.git"
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.NotBeNil)

				vctx = &validation.Context{Context: ctx}
				p.Name = "/prefix-not-allowed/so-is-/-suffix/"
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.NotBeNil)
			})
			t.Run("bad regexp", func(t *ftt.Test) {
				p.RefRegexp = []string{"refs/heads/master", "*is-bad-regexp"}
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike("ref_regexp #2): error parsing regexp:"))
			})
			t.Run("bad regexp_exclude", func(t *ftt.Test) {
				p.RefRegexpExclude = []string{"*is-bad-regexp"}
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike("ref_regexp_exclude #1): error parsing regexp:"))
			})
			t.Run("duplicate regexp", func(t *ftt.Test) {
				p.RefRegexp = []string{"refs/heads/master", "refs/heads/master"}
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike("ref_regexp #2): duplicate regexp:"))
			})
			t.Run("duplicate regexp include/exclude", func(t *ftt.Test) {
				p.RefRegexp = []string{"refs/heads/.+"}
				p.RefRegexpExclude = []string{"refs/heads/.+"}
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike("ref_regexp_exclude #1): duplicate regexp:"))
			})
		})

		t.Run("Verifiers", func(t *ftt.Test) {
			v := cfg.ConfigGroups[0].Verifiers

			t.Run("fake not allowed", func(t *ftt.Test) {
				v.Fake = &cfgpb.Verifiers_Fake{}
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike("fake verifier is not allowed"))
			})
			t.Run("deprecator not allowed", func(t *ftt.Test) {
				v.Cqlinter = &cfgpb.Verifiers_CQLinter{}
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike("cqlinter verifier is not allowed"))
			})
			t.Run("tree_status", func(t *ftt.Test) {
				v.TreeStatus = &cfgpb.Verifiers_TreeStatus{}
				t.Run("require tree name", func(t *ftt.Test) {
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.ErrLike("tree name is required"))
				})
				t.Run("needs https URL", func(t *ftt.Test) {
					v.TreeStatus.Url = "http://example.com/test"
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.ErrLike("url scheme must be 'https'"))
				})
			})
			t.Run("gerrit_cq_ability", func(t *ftt.Test) {
				t.Run("sane defaults", func(t *ftt.Test) {
					assert.Loosely(t, v.GerritCqAbility.AllowSubmitWithOpenDeps, should.BeFalse)
					assert.Loosely(t, v.GerritCqAbility.AllowOwnerIfSubmittable, should.Equal(
						cfgpb.Verifiers_GerritCQAbility_UNSET))
				})
				t.Run("is required", func(t *ftt.Test) {
					v.GerritCqAbility = nil
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.ErrLike("gerrit_cq_ability verifier is required"))
				})
				t.Run("needs committer_list", func(t *ftt.Test) {
					v.GerritCqAbility.CommitterList = nil
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.ErrLike("committer_list is required"))
				})
				t.Run("no empty committer_list", func(t *ftt.Test) {
					v.GerritCqAbility.CommitterList = []string{""}
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.ErrLike("must not be empty"))
				})
				t.Run("no empty dry_run_access_list", func(t *ftt.Test) {
					v.GerritCqAbility.DryRunAccessList = []string{""}
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.ErrLike("must not be empty"))
				})
				t.Run("may grant CL owners extra rights", func(t *ftt.Test) {
					v.GerritCqAbility.AllowOwnerIfSubmittable = cfgpb.Verifiers_GerritCQAbility_COMMIT
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.BeNil)
				})
			})
		})

		t.Run("Tryjob", func(t *ftt.Test) {
			v := cfg.ConfigGroups[0].Verifiers.Tryjob

			t.Run("really bad retry config", func(t *ftt.Test) {
				v.RetryConfig.SingleQuota = -1
				v.RetryConfig.GlobalQuota = -1
				v.RetryConfig.FailureWeight = -1
				v.RetryConfig.TransientFailureWeight = -1
				v.RetryConfig.TimeoutWeight = -1
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike(
					"negative single_quota not allowed (-1 given) (and 4 other errors)"))
			})
		})

		t.Run("UserLimits and UserLimitDefault", func(t *ftt.Test) {
			cg := cfg.ConfigGroups[0]
			cg.UserLimits = []*cfgpb.UserLimit{
				{
					Name:       "user_limit",
					Principals: []string{"user:foo@example.org"},
					Run: &cfgpb.UserLimit_Run{
						MaxActive: &cfgpb.UserLimit_Limit{
							Limit: &cfgpb.UserLimit_Limit_Value{Value: 123},
						},
					},
					Tryjob: &cfgpb.UserLimit_Tryjob{
						MaxActive: &cfgpb.UserLimit_Limit{
							Limit: &cfgpb.UserLimit_Limit_Unlimited{
								Unlimited: true,
							},
						},
					},
				},
				{
					Name:       "group_limit",
					Principals: []string{"group:bar"},
					Run: &cfgpb.UserLimit_Run{
						MaxActive: &cfgpb.UserLimit_Limit{
							Limit: &cfgpb.UserLimit_Limit_Unlimited{
								Unlimited: true,
							},
						},
					},
					Tryjob: &cfgpb.UserLimit_Tryjob{
						MaxActive: &cfgpb.UserLimit_Limit{
							Limit: &cfgpb.UserLimit_Limit_Value{Value: 456},
						},
					},
				},
			}
			cg.UserLimitDefault = &cfgpb.UserLimit{
				Name: "user_limit_default_limit",
				Run: &cfgpb.UserLimit_Run{
					MaxActive: &cfgpb.UserLimit_Limit{
						Limit: &cfgpb.UserLimit_Limit_Unlimited{
							Unlimited: true,
						},
					},
				},
				Tryjob: &cfgpb.UserLimit_Tryjob{
					MaxActive: &cfgpb.UserLimit_Limit{
						Limit: &cfgpb.UserLimit_Limit_Unlimited{
							Unlimited: true,
						},
					},
				},
			}
			validateProjectConfig(vctx, &cfg)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)

			t.Run("UserLimits doesn't allow nil", func(t *ftt.Test) {
				cg.UserLimits[1] = nil
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike("user_limits #2): cannot be nil"))
			})
			t.Run("Names in UserLimits should be unique", func(t *ftt.Test) {
				cg.UserLimits[0].Name = cg.UserLimits[1].Name
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike("user_limits #2 / name): duplicate name"))
			})
			t.Run("UserLimitDefault.Name should be unique", func(t *ftt.Test) {
				cg.UserLimitDefault.Name = cg.UserLimits[0].Name
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike("user_limit_default / name): duplicate name"))
			})
			t.Run("Limit names must be valid", func(t *ftt.Test) {
				ok := func(n string) {
					vctx := &validation.Context{Context: ctx}
					cg.UserLimits[0].Name = n
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.BeNil)
				}
				fail := func(n string) {
					vctx := &validation.Context{Context: ctx}
					cg.UserLimits[0].Name = n
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.ErrLike("does not match"))
				}
				ok("UserLimits")
				ok("User-_@.+Limits")
				ok("1User.Limits")
				ok("User5.Limits-3")
				fail("")
				fail("user limit #1")
			})
			t.Run("UserLimits require principals", func(t *ftt.Test) {
				cg.UserLimits[0].Principals = nil
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike("user_limits #1 / principals): must have at least one"))
			})
			t.Run("UserLimitDefault require no principals", func(t *ftt.Test) {
				cg.UserLimitDefault.Principals = []string{"group:committers"}
				validateProjectConfig(vctx, &cfg)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike("user_limit_default / principals): must not have any"))
			})
			t.Run("principals must be valid", func(t *ftt.Test) {
				ok := func(id string) {
					vctx := &validation.Context{Context: ctx}
					cg.UserLimits[0].Principals[0] = id
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.BeNil)
				}
				fail := func(id, msg string) {
					vctx := &validation.Context{Context: ctx}
					cg.UserLimits[0].Principals[0] = id
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.ErrLike(msg))
				}
				ok("user:test@example.org")
				ok("group:committers")
				fail("user:", `"user:" doesn't look like a principal id`)
				fail("user1", `"user1" doesn't look like a principal id`)
				fail("group:", `"group:" doesn't look like a principal id`)
				fail("bot:linux-123", `unknown principal type "bot"`)
				fail("user:foo", `bad value "foo" for identity kind "user"`)
			})
			t.Run("limits are required", func(t *ftt.Test) {
				fail := func(msg string) {
					vctx := &validation.Context{Context: ctx}
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.ErrLike(msg))
				}

				cg.UserLimits[0].Run = nil
				fail("run): missing; set all limits with `unlimited` if there are no limits")
				cg.UserLimits[0].Run = &cfgpb.UserLimit_Run{}
				fail("run / max_active): missing; set `unlimited` if there is no limit")
			})
			t.Run("limits are > 0 or unlimited", func(t *ftt.Test) {
				ok := func(l *cfgpb.UserLimit_Limit, val int64, unlimited bool) {
					vctx := &validation.Context{Context: ctx}
					if unlimited {
						l.Limit = &cfgpb.UserLimit_Limit_Unlimited{Unlimited: true}
					} else {
						l.Limit = &cfgpb.UserLimit_Limit_Value{Value: val}
					}
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.BeNil)
				}
				fail := func(l *cfgpb.UserLimit_Limit, val int64, unlimited bool, msg string) {
					vctx := &validation.Context{Context: ctx}
					l.Limit = &cfgpb.UserLimit_Limit_Unlimited{Unlimited: true}
					if !unlimited {
						l.Limit = &cfgpb.UserLimit_Limit_Value{Value: val}
					}
					validateProjectConfig(vctx, &cfg)
					assert.Loosely(t, vctx.Finalize(), should.ErrLike(msg))
				}

				// run limits
				ulimit := cg.UserLimits[0]
				fail(ulimit.Run.MaxActive, 0, false, "invalid limit 0;")
				ok(ulimit.Run.MaxActive, 3, false)
				ok(ulimit.Run.MaxActive, 0, true)
			})
		})
	})
}

func TestTryjobValidation(t *testing.T) {
	t.Parallel()

	ftt.Run("Validate Tryjob Verifier Config", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		validate := func(textPB string, parentPB ...string) error {
			vctx := &validation.Context{Context: ctx}
			vd, err := makeProjectConfigValidator(vctx, "prj")
			assert.NoErr(t, err)
			v := cfgpb.Verifiers{}
			switch len(parentPB) {
			case 0:
			case 1:
				if err := prototext.Unmarshal([]byte(parentPB[0]), &v); err != nil {
					panic(err)
				}
			default:
				panic("expected at most one parentPB")
			}
			cfg := cfgpb.Verifiers_Tryjob{}
			switch err := prototext.Unmarshal([]byte(textPB), &cfg); {
			case err != nil:
				panic(err)
			case v.Tryjob == nil:
				v.Tryjob = &cfg
			default:
				proto.Merge(v.Tryjob, &cfg)
			}

			vd.validateTryjobVerifier(&v, standardModes)
			return vctx.Finalize()
		}

		assert.Loosely(t, validate(``), should.BeNil) // allow empty builders.

		assert.Loosely(t, mustError(t, validate(`
			cancel_stale_tryjobs: YES
			builders {name: "a/b/c"}`)), should.ErrLike("please remove"))
		assert.Loosely(t, mustError(t, validate(`
			cancel_stale_tryjobs: NO
			builders {name: "a/b/c"}`)), should.ErrLike("use per-builder `cancel_stale` instead"))

		t.Run("builder name", func(t *ftt.Test) {
			assert.Loosely(t, validate(`builders {}`), should.ErrLike("name is required"))
			assert.Loosely(t, validate(`builders {name: ""}`), should.ErrLike("name is required"))
			assert.Loosely(t, validate(`builders {name: "a"}`), should.ErrLike(
				`name "a" doesn't match required format`))
			assert.Loosely(t, validate(`builders {name: "a/b/c" equivalent_to {name: "z"}}`), should.ErrLike(
				`name "z" doesn't match required format`))
			assert.Loosely(t, validate(`builders {name: "b/luci.b.try/c"}`), should.ErrLike(
				`name "b/luci.b.try/c" is highly likely malformed;`))

			assert.Loosely(t, validate(`
			  builders {name: "a/b/c"}
			  builders {name: "a/b/c"}
			`), should.ErrLike("duplicate"))

			assert.Loosely(t, validate(`
				builders {name: "m/n/o"}
			  builders {name: "a/b/c" equivalent_to {name: "x/y/z"}}
			`), should.BeNil)

			assert.Loosely(t, validate(`builders {name: "123/b/c"}`), should.ErrLike(
				`first part of "123/b/c" is not a valid LUCI project name`))
		})

		t.Run("result_visibility", func(t *ftt.Test) {
			assert.Loosely(t, validate(`
				builders {name: "a/b/c" result_visibility: COMMENT_LEVEL_UNSET}
			`), should.BeNil)
			assert.Loosely(t, validate(`
				builders {name: "a/b/c" result_visibility: COMMENT_LEVEL_FULL}
			`), should.BeNil)
			assert.Loosely(t, validate(`
				builders {name: "a/b/c" result_visibility: COMMENT_LEVEL_RESTRICTED}
			`), should.BeNil)
		})

		t.Run("experiment", func(t *ftt.Test) {
			assert.Loosely(t, validate(`builders {name: "a/b/c" experiment_percentage: 1}`), should.BeNil)
			assert.Loosely(t, validate(`builders {name: "a/b/c" experiment_percentage: -1}`), should.NotBeNil)
			assert.Loosely(t, validate(`builders {name: "a/b/c" experiment_percentage: 101}`), should.NotBeNil)
		})

		t.Run("location_filters", func(t *ftt.Test) {
			assert.Loosely(t, validate(`
				builders {
					name: "a/b/c"
					location_filters: {
						gerrit_host_regexp: ""
						gerrit_project_regexp: ""
						path_regexp: ".*"
						exclude: false
					}
					location_filters: {
						gerrit_host_regexp: "chromium-review.googlesource.com"
						gerrit_project_regexp: "chromium/src"
						path_regexp: "README.md"
						exclude: true
					}
				}`), should.BeNil)

			err := validate(`
				builders {
					name: "a/b/c"
					location_filters: {
						gerrit_host_regexp: "bad \\c regexp"
					}
				}`)
			assert.Loosely(t, err, should.ErrLike("gerrit_host_regexp"))
			assert.Loosely(t, err, should.ErrLike("invalid regexp"))

			err = validate(`
				builders {
					name: "a/b/c"
					location_filters: {
						gerrit_host_regexp: "https://chromium-review.googlesource.com"
					}
				}`)
			assert.Loosely(t, err, should.ErrLike("gerrit_host_regexp"))
			assert.Loosely(t, err, should.ErrLike("scheme"))
			assert.Loosely(t, err, should.ErrLike("not needed"))

			err = validate(`
				builders {
					name: "a/b/c"
					location_filters: {
						gerrit_project_regexp: "bad \\c regexp"
					}
				}`)
			assert.Loosely(t, err, should.ErrLike("gerrit_project_regexp"))
			assert.Loosely(t, err, should.ErrLike("invalid regexp"))

			err = validate(`
				builders {
					name: "a/b/c"
					location_filters: {
						path_regexp: "bad \\c regexp"
					}
				}`)
			assert.Loosely(t, err, should.ErrLike("path_regexp"))
			assert.Loosely(t, err, should.ErrLike("invalid regexp"))
		})

		t.Run("equivalent_to", func(t *ftt.Test) {
			assert.Loosely(t, validate(`
				builders {
					name: "a/b/c"
					equivalent_to {name: "x/y/z" percentage: 10 owner_whitelist_group: "group"}
				}`),
				should.BeNil)

			assert.Loosely(t, validate(`
				builders {
					name: "a/b/c"
					equivalent_to {name: "x/y/z" percentage: -1 owner_whitelist_group: "group"}
				}`),
				should.ErrLike("percentage must be between 0 and 100"))
			assert.Loosely(t, validate(`
				builders {
					name: "a/b/c"
					equivalent_to {name: "a/b/c"}
				}`),
				should.ErrLike(
					`equivalent_to.name must not refer to already defined "a/b/c" builder`))
			assert.Loosely(t, validate(`
				builders {
					name: "a/b/c"
					equivalent_to {name: "c/d/e"}
				}
				builders {
					name: "x/y/z"
					equivalent_to {name: "c/d/e"}
				}`),
				should.ErrLike(
					`duplicate name "c/d/e"`))
		})

		t.Run("owner_whitelist_group", func(t *ftt.Test) {
			assert.Loosely(t, validate(`builders { name: "a/b/c" owner_whitelist_group: "ok" }`), should.BeNil)
			assert.Loosely(t, validate(`
				builders {
					name: "a/b/c"
					owner_whitelist_group: "ok"
				}`), should.BeNil)
			assert.Loosely(t, validate(`
				builders {
					name: "a/b/c"
					owner_whitelist_group: "ok"
					owner_whitelist_group: ""
					owner_whitelist_group: "also-ok"
				}`), should.ErrLike(
				"must not be empty string"))
		})

		t.Run("mode_allowlist", func(t *ftt.Test) {
			assert.Loosely(t, validate(`builders {name: "a/b/c" mode_allowlist: "DRY_RUN"}`), should.BeNil)
			assert.Loosely(t, validate(`
				builders {
					name: "a/b/c"
					mode_allowlist: "DRY_RUN"
					mode_allowlist: "FULL_RUN"
				}`), should.BeNil)

			assert.Loosely(t, validate(`
				builders {
					name: "a/b/c"
					mode_allowlist: "DRY"
					mode_allowlist: "FULL_RUN"
				}`), should.ErrLike(
				"must be one of"))

			assert.Loosely(t, validate(`
				builders {
					name: "a/b/c"
					mode_allowlist: "NEW_PATCHSET_RUN"
				}`), should.ErrLike(
				"cannot be used unless a new_patchset_run_access_list is set"))

			t.Run("contains ANALYZER_RUN", func(t *ftt.Test) {
				assert.Loosely(t, validate(`
					builders {
						name: "a/b/c"
						location_filters: {
							path_regexp: ".*"
						}
						mode_allowlist: "ANALYZER_RUN"
					}`), should.ErrLike(
					`analyzer location filter path pattern must match`))
				assert.Loosely(t, validate(`
					builders {
						name: "a/b/c"
						location_filters: {
							gerrit_project_regexp: "proj"
							path_regexp: ".+\\.go"
						}
						mode_allowlist: "ANALYZER_RUN"
					}`), should.ErrLike(
					`analyzer location filter must include both host and project or neither`))
				assert.Loosely(t, validate(`
					builders {
						name: "a/b/c"
						location_filters: {
							path_regexp: ".+\\.py"
							exclude: True
						}
						mode_allowlist: "ANALYZER_RUN"
					}`), should.ErrLike(
					`location_filters exclude filters are not combinable`))
				assert.Loosely(t, validate(`
				builders {
					name: "x/y/z"
				}
				builders {
					name: "a/b/c"
					mode_allowlist: "ANALYZER_RUN"
				}`), should.BeNil)
				assert.Loosely(t, validate(`
				builders {
					name: "x/y/z"
				}
				builders {
					name: "a/b/c"
					mode_allowlist: "ANALYZER_RUN"
					location_filters: {
						path_regexp: ".+\\.go"
					}
				}`), should.BeNil)
				assert.Loosely(t, validate(`
				builders {
					name: "x/y/z"
				}
				builders {
					name: "a/b/c"
					mode_allowlist: "ANALYZER_RUN"
					location_filters: {
						gerrit_host_regexp: "chromium-review.googlesource.com"
						gerrit_project_regexp: "proj"
						path_regexp: ".+\\.go"
					}
				}`), should.BeNil)
			})
		})

		t.Run("allowed combinations", func(t *ftt.Test) {
			assert.Loosely(t, validate(`
				builders {
					name: "a/b/c"
					experiment_percentage: 1
					owner_whitelist_group: "owners"
				}`),
				should.BeNil)
			assert.Loosely(t, validate(`
				builders {
					name: "a/b/c"
					location_filters: {
						path_regexp: ".+\\.cpp"
					}
				}
				builders {
					name: "c/d/e"
					location_filters: {
						path_regexp: ".+\\.cpp"
					}
				} `),
				should.BeNil)
			assert.Loosely(t, validate(`
				builders {name: "pa/re/nt"}
				builders {
					name: "a/b/c"
					includable_only: true
				}`),
				should.BeNil)
		})

		t.Run("disallowed combinations", func(t *ftt.Test) {
			assert.Loosely(t, validate(`
				builders {
					name: "a/b/c"
					experiment_percentage: 1
					equivalent_to {name: "c/d/e"}}`),
				should.ErrLike(
					"experiment_percentage is not combinable with equivalent_to"))
		})

		t.Run("includable_only", func(t *ftt.Test) {
			assert.Loosely(t, validate(`
				builders {
					name: "a/b/c"
					experiment_percentage: 1
					includable_only: true
				}`),
				should.ErrLike(
					"includable_only is not combinable with experiment_percentage"))
			assert.Loosely(t, validate(`
				builders {
					name: "a/b/c"
					location_filters: {
						path_regexp: ".+\\.cpp"
					}
					includable_only: true
				}`),
				should.ErrLike(
					"includable_only is not combinable with location_filters"))
			assert.Loosely(t, validate(`
				builders {
					name: "a/b/c"
					mode_allowlist: "DRY_RUN"
					includable_only: true
				}`),
				should.ErrLike(
					"includable_only is not combinable with mode_allowlist"))

			assert.Loosely(t, validate(`builders {name: "one/is/enough" includable_only: true}`), should.BeNil)
		})
	})
}
