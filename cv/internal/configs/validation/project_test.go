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

	"go.chromium.org/luci/config/validation"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	apipb "go.chromium.org/luci/cv/api/v1"
	"go.chromium.org/luci/cv/internal/configs/srvcfg"
	"go.chromium.org/luci/cv/internal/cvtesting"
	listenerpb "go.chromium.org/luci/cv/settings/listener"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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

	Convey("ValidateProject works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		cfg := cfgpb.Config{}
		vctx := &validation.Context{Context: ctx}
		So(prototext.Unmarshal([]byte(validConfigTextPB), &cfg), ShouldBeNil)
		So(mockListenerSettings(ctx, "chromium-review.googlesource.com"), ShouldBeNil)

		Convey("OK", func() {
			So(ValidateProject(vctx, &cfg, project), ShouldBeNil)
			So(vctx.Finalize(), ShouldBeNil)
		})
		Convey("Error", func() {
			cfg.GetConfigGroups()[0].Name = "!invalid! name"
			So(ValidateProject(vctx, &cfg, project), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, "must match")
		})
	})

	Convey("ValidateProjectConfig works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		cfg := cfgpb.Config{}
		vctx := &validation.Context{Context: ctx}
		So(prototext.Unmarshal([]byte(validConfigTextPB), &cfg), ShouldBeNil)

		Convey("OK", func() {
			So(ValidateProjectConfig(vctx, &cfg), ShouldBeNil)
			So(vctx.Finalize(), ShouldBeNil)
		})
		Convey("Error", func() {
			cfg.GetConfigGroups()[0].Name = "!invalid! name"
			So(ValidateProject(vctx, &cfg, project), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, "must match")
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

	Convey("Validate Config", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()
		vctx := &validation.Context{Context: ctx}
		validateProjectConfig := func(vctx *validation.Context, cfg *cfgpb.Config) {
			vd, err := makeProjectConfigValidator(vctx, project)
			So(err, ShouldBeNil)
			vd.validateProjectConfig(cfg)
		}

		Convey("Loading bad proto", func() {
			content := []byte(` bad: "config" `)
			So(validateProject(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "unknown field")
		})

		// It's easier to manipulate Go struct than text.
		cfg := cfgpb.Config{}
		So(prototext.Unmarshal([]byte(validConfigTextPB), &cfg), ShouldBeNil)
		So(mockListenerSettings(ctx, "chromium-review.googlesource.com"), ShouldBeNil)

		Convey("OK", func() {
			Convey("good proto, good config", func() {
				So(validateProject(vctx, configSet, path, []byte(validConfigTextPB)), ShouldBeNil)
				So(vctx.Finalize(), ShouldBeNil)
			})
			Convey("good config", func() {
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldBeNil)
			})
		})

		Convey("Missing gerrit subscription", func() {
			// reset the listener settings to make the validation fail.
			So(mockListenerSettings(ctx), ShouldBeNil)

			Convey("validation fails", func() {
				So(validateProject(vctx, configSet, path, []byte(validConfigTextPB)), ShouldBeNil)
				So(vctx.Finalize(), ShouldErrLike, "Gerrit pub/sub")
			})
			Convey("OK if the project is disabled in listener settings", func() {
				ct.DisableProjectInGerritListener(ctx, project)
				So(validateProject(vctx, configSet, path, []byte(validConfigTextPB)), ShouldBeNil)
			})
		})
		So(mockListenerSettings(ctx, "chromium-review.googlesource.com"), ShouldBeNil)

		Convey("Top-level config", func() {
			Convey("Top level opts can be omitted", func() {
				cfg.CqStatusHost = ""
				cfg.SubmitOptions = nil
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldBeNil)
			})
			Convey("draining time not allowed crbug/1208569", func() {
				cfg.DrainingStartTime = "2017-12-23T15:47:58Z"
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, `https://crbug.com/1208569`)
			})
			Convey("CQ status host can be internal", func() {
				cfg.CqStatusHost = CQStatusHostInternal
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldBeNil)
			})
			Convey("CQ status host can be empty", func() {
				cfg.CqStatusHost = ""
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldBeNil)
			})
			Convey("CQ status host can be public", func() {
				cfg.CqStatusHost = CQStatusHostPublic
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldBeNil)
			})
			Convey("CQ status host can not be something else", func() {
				cfg.CqStatusHost = "nope.example.com"
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, "cq_status_host must be")
			})
			Convey("Bad max_burst", func() {
				cfg.SubmitOptions.MaxBurst = -1
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldNotBeNil)
			})
			Convey("Bad burst_delay ", func() {
				cfg.SubmitOptions.BurstDelay.Seconds = -1
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldNotBeNil)
			})
			Convey("config_groups", func() {
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

				Convey("at least 1 Config Group", func() {
					cfg.ConfigGroups = nil
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, "at least 1 config_group is required")
				})

				Convey("at most 1 fallback", func() {
					cfg.ConfigGroups = nil
					add("refs/heads/.+")
					cfg.ConfigGroups[0].Fallback = cfgpb.Toggle_YES
					add("refs/branch-heads/.+")
					cfg.ConfigGroups[1].Fallback = cfgpb.Toggle_YES
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, "At most 1 config_group with fallback=YES allowed")
				})

				Convey("with unique names", func() {
					cfg.ConfigGroups = nil
					add("refs/heads/.+")
					add("refs/branch-heads/.+")
					add("refs/other-heads/.+")
					Convey("dups not allowed", func() {
						cfg.ConfigGroups[0].Name = "aaa"
						cfg.ConfigGroups[1].Name = "bbb"
						cfg.ConfigGroups[2].Name = "bbb"
						validateProjectConfig(vctx, &cfg)
						So(vctx.Finalize(), ShouldErrLike, "duplicate config_group name \"bbb\" not allowed")
					})
				})
			})
		})

		Convey("ConfigGroups", func() {
			Convey("with no Name", func() {
				cfg.ConfigGroups[0].Name = ""
				validateProjectConfig(vctx, &cfg)
				So(mustError(vctx.Finalize()), ShouldErrLike, "name is required")
			})
			Convey("with valid Name", func() {
				cfg.ConfigGroups[0].Name = "!invalid!"
				validateProjectConfig(vctx, &cfg)
				So(mustError(vctx.Finalize()), ShouldErrLike, "name must match")
			})
			Convey("with Gerrit", func() {
				cfg.ConfigGroups[0].Gerrit = nil
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, "at least 1 gerrit is required")
			})
			Convey("with Verifiers", func() {
				cfg.ConfigGroups[0].Verifiers = nil
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, "verifiers are required")
			})
			Convey("no dup Gerrit blocks", func() {
				cfg.ConfigGroups[0].Gerrit = append(cfg.ConfigGroups[0].Gerrit, cfg.ConfigGroups[0].Gerrit[0])
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, "duplicate gerrit url in the same config_group")
			})
			Convey("CombineCLs", func() {
				cfg.ConfigGroups[0].CombineCls = &cfgpb.CombineCLs{}
				Convey("Needs stabilization_delay", func() {
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, "stabilization_delay is required")
				})
				cfg.ConfigGroups[0].CombineCls.StabilizationDelay = &durationpb.Duration{}
				Convey("Needs stabilization_delay > 10s", func() {
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, "stabilization_delay must be at least 10 seconds")
				})
				cfg.ConfigGroups[0].CombineCls.StabilizationDelay.Seconds = 20
				Convey("OK", func() {
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldBeNil)
				})
				Convey("Can't use with allow_submit_with_open_deps", func() {
					cfg.ConfigGroups[0].Verifiers.GerritCqAbility.AllowSubmitWithOpenDeps = true
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, "allow_submit_with_open_deps=true")
				})
			})

			mode := &cfgpb.Mode{
				Name:            "QUICK_DRY_RUN",
				CqLabelValue:    1,
				TriggeringLabel: "TEST_RUN_LABEL",
				TriggeringValue: 2,
			}
			Convey("Mode", func() {
				cfg.ConfigGroups[0].AdditionalModes = []*cfgpb.Mode{mode}
				Convey("OK", func() {
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldBeNil)
				})
				Convey("name", func() {
					Convey("empty", func() { mode.Name = "" })
					Convey("with invalid chars", func() { mode.Name = "~!Invalid Run Mode!~" })

					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, "does not match regex pattern")
				})
				Convey("cq_label_value", func() {
					Convey("with -1", func() { mode.CqLabelValue = -1 })
					Convey("with 0", func() { mode.CqLabelValue = 0 })
					Convey("with 3", func() { mode.CqLabelValue = 3 })
					Convey("with 10", func() { mode.CqLabelValue = 10 })

					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, "must be in list [1 2]")
				})
				Convey("triggering_label", func() {
					Convey("empty", func() {
						mode.TriggeringLabel = ""
						validateProjectConfig(vctx, &cfg)
						So(vctx.Finalize(), ShouldErrLike, "length must be at least 1 runes")
					})
					Convey("with Commit-Queue", func() {
						mode.TriggeringLabel = "Commit-Queue"
						validateProjectConfig(vctx, &cfg)
						So(vctx.Finalize(), ShouldErrLike, "must not be in list [Commit-Queue]")
					})
				})
				Convey("triggering_value", func() {
					Convey("with 0", func() { mode.TriggeringValue = 0 })
					Convey("with -1", func() { mode.TriggeringValue = -1 })

					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, "must be greater than 0")
				})
			})

			// Tests for additional mode specific verifiers.
			Convey("additional_modes", func() {
				cfg.ConfigGroups[0].AdditionalModes = []*cfgpb.Mode{mode}
				Convey("reserved names", func() {
					Convey("DRY_RUN", func() { mode.Name = "DRY_RUN" })
					Convey("FULL_RUN", func() { mode.Name = "FULL_RUN" })

					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, "MUST be `QUICK_DRY_RUN`")
				})
				Convey("not QUICK_DRY_RUN", func() {
					mode.Name = "TEST_RUN"
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, "MUST be `QUICK_DRY_RUN`")
				})
				Convey("duplicate names", func() {
					cfg.ConfigGroups[0].AdditionalModes = []*cfgpb.Mode{mode, mode}
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, `"QUICK_DRY_RUN" is already in use`)
				})
			})

			Convey("post_actions", func() {
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

				Convey("works", func() {
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldBeNil)
				})

				Convey("name", func() {
					Convey("missing", func() {
						pa.Name = ""
						validateProjectConfig(vctx, &cfg)
						So(vctx.Finalize(), ShouldErrLike, "Name: value length must be at least 1")
					})

					Convey("duplicate", func() {
						cfg.ConfigGroups[0].PostActions = append(cfg.ConfigGroups[0].PostActions,
							cfg.ConfigGroups[0].PostActions[0])
						validateProjectConfig(vctx, &cfg)
						So(vctx.Finalize(), ShouldErrLike, `"CQ verified"' is already in use`)
					})
				})

				Convey("action", func() {
					Convey("missing", func() {
						pa.Action = nil
						validateProjectConfig(vctx, &cfg)
						So(vctx.Finalize(), ShouldErrLike, `Action: value is required`)
					})
					Convey("vote_gerrit_labels", func() {
						w := pa.GetAction().(*cfgpb.ConfigGroup_PostAction_VoteGerritLabels_).VoteGerritLabels
						Convey("empty pairs", func() {
							w.Votes = nil
							validateProjectConfig(vctx, &cfg)
							So(vctx.Finalize(), ShouldErrLike, "Votes: value must contain")
						})
						Convey("a pair with an empty name", func() {
							w.Votes[0].Name = ""
							validateProjectConfig(vctx, &cfg)
							So(vctx.Finalize(), ShouldErrLike, "Name: value length must be")
						})
						Convey("pairs with duplicate names", func() {
							w.Votes = append(w.Votes, w.Votes[0])
							validateProjectConfig(vctx, &cfg)
							So(vctx.Finalize(), ShouldErrLike, `"CQ-verified" already specified`)

						})
					})
				})

				Convey("triggering_conditions", func() {
					tc := pa.Conditions[0]
					Convey("missing", func() {
						pa.Conditions = nil
						validateProjectConfig(vctx, &cfg)
						So(vctx.Finalize(), ShouldErrLike, `Conditions: value must contain at least 1`)
					})
					Convey("mode", func() {
						Convey("missing", func() {
							tc.Mode = ""
							validateProjectConfig(vctx, &cfg)
							So(vctx.Finalize(), ShouldErrLike, `Mode: value length must be at least 1`)
						})

						cfg.ConfigGroups[0].AdditionalModes = []*cfgpb.Mode{mode}
						Convey("with an existing additional mode", func() {
							tc.Mode = "QUICK_DRY_RUN"
							validateProjectConfig(vctx, &cfg)
							So(vctx.Finalize(), ShouldBeNil)
						})

						Convey("with an non-existing additional mode", func() {
							tc.Mode = "SLOW_DRY_RUN"
							validateProjectConfig(vctx, &cfg)
							So(vctx.Finalize(), ShouldErrLike, `invalid mode "SLOW_DRY_RUN"`)
						})
					})
					Convey("statuses", func() {
						Convey("missing", func() {
							tc.Statuses = nil
							validateProjectConfig(vctx, &cfg)
							So(vctx.Finalize(), ShouldErrLike, `Statuses: value must contain at least 1`)
						})
						Convey("non-terminal status", func() {
							tc.Statuses = []apipb.Run_Status{
								apipb.Run_SUCCEEDED,
								apipb.Run_PENDING,
							}
							validateProjectConfig(vctx, &cfg)
							So(vctx.Finalize(), ShouldErrLike, `"PENDING" is not a terminal status`)
						})
						Convey("duplicates", func() {
							tc.Statuses = []apipb.Run_Status{
								apipb.Run_SUCCEEDED,
								apipb.Run_SUCCEEDED,
							}
							validateProjectConfig(vctx, &cfg)
							So(vctx.Finalize(), ShouldErrLike, `"SUCCEEDED" was specified already`)
						})
					})
				})
			})
		})

		Convey("tryjob_experiments", func() {
			exp := &cfgpb.ConfigGroup_TryjobExperiment{
				Name: "infra.experiment.foo",
				Condition: &cfgpb.ConfigGroup_TryjobExperiment_Condition{
					OwnerGroupAllowlist: []string{"googlers"},
				},
			}
			cfg.ConfigGroups[0].TryjobExperiments = []*cfgpb.ConfigGroup_TryjobExperiment{exp}

			Convey("works", func() {
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldBeNil)
			})

			Convey("name", func() {
				Convey("missing", func() {
					exp.Name = ""
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, "Name: value length must be at least 1")
				})

				Convey("duplicate", func() {
					cfg.ConfigGroups[0].TryjobExperiments = []*cfgpb.ConfigGroup_TryjobExperiment{exp, exp}
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, `duplicate name "infra.experiment.foo"`)
				})

				Convey("invalid name", func() {
					exp.Name = "^&*()"
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, `"^&*()" does not match`)
				})
			})

			Convey("Condition", func() {
				Convey("owner_group_allowlist has empty string", func() {
					exp.Condition.OwnerGroupAllowlist = []string{"infra.chromium.foo", ""}
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, "OwnerGroupAllowlist[1]: value length must be at least 1 ")
				})
			})
		})

		Convey("Gerrit", func() {
			g := cfg.ConfigGroups[0].Gerrit[0]
			Convey("needs valid URL", func() {
				g.Url = ""
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, "url is required")

				g.Url = ":badscheme, bad URL"
				vctx = &validation.Context{Context: ctx}
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, "failed to parse url")
			})

			Convey("without fancy URL components", func() {
				g.Url = "bad://ok/path-not-good?query=too#neither-is-fragment"
				validateProjectConfig(vctx, &cfg)
				err := vctx.Finalize()
				So(err, ShouldErrLike, "path component not yet allowed in url")
				So(err, ShouldErrLike, "and 5 other errors")
			})

			Convey("current limitations", func() {
				g.Url = "https://not.yet.allowed.com"
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, "only *.googlesource.com hosts supported for now")

				vctx = &validation.Context{Context: ctx}
				g.Url = "new-scheme://chromium-review.googlesource.com"
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, "only 'https' scheme supported for now")
			})
			Convey("at least 1 project required", func() {
				g.Projects = nil
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, "at least 1 project is required")
			})
			Convey("no dup project blocks", func() {
				g.Projects = append(g.Projects, g.Projects[0])
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, "duplicate project in the same gerrit")
			})
		})

		Convey("Gerrit Project", func() {
			p := cfg.ConfigGroups[0].Gerrit[0].Projects[0]
			Convey("project name required", func() {
				p.Name = ""
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, "name is required")
			})
			Convey("incorrect project names", func() {
				p.Name = "a/prefix-not-allowed/so-is-.git-suffix/.git"
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldNotBeNil)

				vctx = &validation.Context{Context: ctx}
				p.Name = "/prefix-not-allowed/so-is-/-suffix/"
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldNotBeNil)
			})
			Convey("bad regexp", func() {
				p.RefRegexp = []string{"refs/heads/master", "*is-bad-regexp"}
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, "ref_regexp #2): error parsing regexp:")
			})
			Convey("bad regexp_exclude", func() {
				p.RefRegexpExclude = []string{"*is-bad-regexp"}
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, "ref_regexp_exclude #1): error parsing regexp:")
			})
			Convey("duplicate regexp", func() {
				p.RefRegexp = []string{"refs/heads/master", "refs/heads/master"}
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, "ref_regexp #2): duplicate regexp:")
			})
			Convey("duplicate regexp include/exclude", func() {
				p.RefRegexp = []string{"refs/heads/.+"}
				p.RefRegexpExclude = []string{"refs/heads/.+"}
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, "ref_regexp_exclude #1): duplicate regexp:")
			})
		})

		Convey("Verifiers", func() {
			v := cfg.ConfigGroups[0].Verifiers

			Convey("fake not allowed", func() {
				v.Fake = &cfgpb.Verifiers_Fake{}
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, "fake verifier is not allowed")
			})
			Convey("deprecator not allowed", func() {
				v.Cqlinter = &cfgpb.Verifiers_CQLinter{}
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, "cqlinter verifier is not allowed")
			})
			Convey("tree_status", func() {
				v.TreeStatus = &cfgpb.Verifiers_TreeStatus{}
				Convey("needs URL", func() {
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, "url is required")
				})
				Convey("needs https URL", func() {
					v.TreeStatus.Url = "http://example.com/test"
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, "url scheme must be 'https'")
				})
			})
			Convey("gerrit_cq_ability", func() {
				Convey("sane defaults", func() {
					So(v.GerritCqAbility.AllowSubmitWithOpenDeps, ShouldBeFalse)
					So(v.GerritCqAbility.AllowOwnerIfSubmittable, ShouldEqual,
						cfgpb.Verifiers_GerritCQAbility_UNSET)
				})
				Convey("is required", func() {
					v.GerritCqAbility = nil
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, "gerrit_cq_ability verifier is required")
				})
				Convey("needs committer_list", func() {
					v.GerritCqAbility.CommitterList = nil
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, "committer_list is required")
				})
				Convey("no empty committer_list", func() {
					v.GerritCqAbility.CommitterList = []string{""}
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, "must not be empty")
				})
				Convey("no empty dry_run_access_list", func() {
					v.GerritCqAbility.DryRunAccessList = []string{""}
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, "must not be empty")
				})
				Convey("may grant CL owners extra rights", func() {
					v.GerritCqAbility.AllowOwnerIfSubmittable = cfgpb.Verifiers_GerritCQAbility_COMMIT
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldBeNil)
				})
			})
		})

		Convey("Tryjob", func() {
			v := cfg.ConfigGroups[0].Verifiers.Tryjob

			Convey("really bad retry config", func() {
				v.RetryConfig.SingleQuota = -1
				v.RetryConfig.GlobalQuota = -1
				v.RetryConfig.FailureWeight = -1
				v.RetryConfig.TransientFailureWeight = -1
				v.RetryConfig.TimeoutWeight = -1
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike,
					"negative single_quota not allowed (-1 given) (and 4 other errors)")
			})
		})

		Convey("UserLimits and UserLimitDefault", func() {
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
			So(vctx.Finalize(), ShouldBeNil)

			Convey("UserLimits doesn't allow nil", func() {
				cg.UserLimits[1] = nil
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, "user_limits #2): cannot be nil")
			})
			Convey("Names in UserLimits should be unique", func() {
				cg.UserLimits[0].Name = cg.UserLimits[1].Name
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, "user_limits #2 / name): duplicate name")
			})
			Convey("UserLimitDefault.Name should be unique", func() {
				cg.UserLimitDefault.Name = cg.UserLimits[0].Name
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, "user_limit_default / name): duplicate name")
			})
			Convey("Limit names must be valid", func() {
				ok := func(n string) {
					vctx := &validation.Context{Context: ctx}
					cg.UserLimits[0].Name = n
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldBeNil)
				}
				fail := func(n string) {
					vctx := &validation.Context{Context: ctx}
					cg.UserLimits[0].Name = n
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, "does not match")
				}
				ok("UserLimits")
				ok("User-_@.+Limits")
				ok("1User.Limits")
				ok("User5.Limits-3")
				fail("")
				fail("user limit #1")
			})
			Convey("UserLimits require principals", func() {
				cg.UserLimits[0].Principals = nil
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, "user_limits #1 / principals): must have at least one")
			})
			Convey("UserLimitDefault require no principals", func() {
				cg.UserLimitDefault.Principals = []string{"group:committers"}
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, "user_limit_default / principals): must not have any")
			})
			Convey("principals must be valid", func() {
				ok := func(id string) {
					vctx := &validation.Context{Context: ctx}
					cg.UserLimits[0].Principals[0] = id
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldBeNil)
				}
				fail := func(id, msg string) {
					vctx := &validation.Context{Context: ctx}
					cg.UserLimits[0].Principals[0] = id
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, msg)
				}
				ok("user:test@example.org")
				ok("group:committers")
				fail("user:", `"user:" doesn't look like a principal id`)
				fail("user1", `"user1" doesn't look like a principal id`)
				fail("group:", `"group:" doesn't look like a principal id`)
				fail("bot:linux-123", `unknown principal type "bot"`)
				fail("user:foo", `bad value "foo" for identity kind "user"`)
			})
			Convey("limits are required", func() {
				fail := func(msg string) {
					vctx := &validation.Context{Context: ctx}
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, msg)
				}

				cg.UserLimits[0].Run = nil
				fail("run): missing; set all limits with `unlimited` if there are no limits")
				cg.UserLimits[0].Run = &cfgpb.UserLimit_Run{}
				fail("run / max_active): missing; set `unlimited` if there is no limit")
			})
			Convey("limits are > 0 or unlimited", func() {
				ok := func(l *cfgpb.UserLimit_Limit, val int64, unlimited bool) {
					vctx := &validation.Context{Context: ctx}
					if unlimited {
						l.Limit = &cfgpb.UserLimit_Limit_Unlimited{Unlimited: true}
					} else {
						l.Limit = &cfgpb.UserLimit_Limit_Value{Value: val}
					}
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldBeNil)
				}
				fail := func(l *cfgpb.UserLimit_Limit, val int64, unlimited bool, msg string) {
					vctx := &validation.Context{Context: ctx}
					l.Limit = &cfgpb.UserLimit_Limit_Unlimited{Unlimited: true}
					if !unlimited {
						l.Limit = &cfgpb.UserLimit_Limit_Value{Value: val}
					}
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, msg)
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

	Convey("Validate Tryjob Verifier Config", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		validate := func(textPB string, parentPB ...string) error {
			vctx := &validation.Context{Context: ctx}
			vd, err := makeProjectConfigValidator(vctx, "prj")
			So(err, ShouldBeNil)
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

		So(validate(``), ShouldBeNil) // allow empty builders.

		So(mustError(validate(`
			cancel_stale_tryjobs: YES
			builders {name: "a/b/c"}`)), ShouldErrLike, "please remove")
		So(mustError(validate(`
			cancel_stale_tryjobs: NO
			builders {name: "a/b/c"}`)), ShouldErrLike, "use per-builder `cancel_stale` instead")

		Convey("builder name", func() {
			So(validate(`builders {}`), ShouldErrLike, "name is required")
			So(validate(`builders {name: ""}`), ShouldErrLike, "name is required")
			So(validate(`builders {name: "a"}`), ShouldErrLike,
				`name "a" doesn't match required format`)
			So(validate(`builders {name: "a/b/c" equivalent_to {name: "z"}}`), ShouldErrLike,
				`name "z" doesn't match required format`)
			So(validate(`builders {name: "b/luci.b.try/c"}`), ShouldErrLike,
				`name "b/luci.b.try/c" is highly likely malformed;`)

			So(validate(`
			  builders {name: "a/b/c"}
			  builders {name: "a/b/c"}
			`), ShouldErrLike, "duplicate")

			So(validate(`
				builders {name: "m/n/o"}
			  builders {name: "a/b/c" equivalent_to {name: "x/y/z"}}
			`), ShouldBeNil)

			So(validate(`builders {name: "123/b/c"}`), ShouldErrLike,
				`first part of "123/b/c" is not a valid LUCI project name`)
		})

		Convey("result_visibility", func() {
			So(validate(`
				builders {name: "a/b/c" result_visibility: COMMENT_LEVEL_UNSET}
			`), ShouldBeNil)
			So(validate(`
				builders {name: "a/b/c" result_visibility: COMMENT_LEVEL_FULL}
			`), ShouldBeNil)
			So(validate(`
				builders {name: "a/b/c" result_visibility: COMMENT_LEVEL_RESTRICTED}
			`), ShouldBeNil)
		})

		Convey("experiment", func() {
			So(validate(`builders {name: "a/b/c" experiment_percentage: 1}`), ShouldBeNil)
			So(validate(`builders {name: "a/b/c" experiment_percentage: -1}`), ShouldNotBeNil)
			So(validate(`builders {name: "a/b/c" experiment_percentage: 101}`), ShouldNotBeNil)
		})

		Convey("location_filters", func() {
			So(validate(`
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
				}`), ShouldBeNil)

			So(validate(`
				builders {
					name: "a/b/c"
					location_filters: {
						gerrit_host_regexp: "bad \\c regexp"
					}
				}`), ShouldErrLike, "gerrit_host_regexp", "invalid regexp")

			So(validate(`
				builders {
					name: "a/b/c"
					location_filters: {
						gerrit_host_regexp: "https://chromium-review.googlesource.com"
					}
				}`), ShouldErrLike, "gerrit_host_regexp", "scheme", "not needed")

			So(validate(`
				builders {
					name: "a/b/c"
					location_filters: {
						gerrit_project_regexp: "bad \\c regexp"
					}
				}`), ShouldErrLike, "gerrit_project_regexp", "invalid regexp")

			So(validate(`
				builders {
					name: "a/b/c"
					location_filters: {
						path_regexp: "bad \\c regexp"
					}
				}`), ShouldErrLike, "path_regexp", "invalid regexp")
		})

		Convey("equivalent_to", func() {
			So(validate(`
				builders {
					name: "a/b/c"
					equivalent_to {name: "x/y/z" percentage: 10 owner_whitelist_group: "group"}
				}`),
				ShouldBeNil)

			So(validate(`
				builders {
					name: "a/b/c"
					equivalent_to {name: "x/y/z" percentage: -1 owner_whitelist_group: "group"}
				}`),
				ShouldErrLike, "percentage must be between 0 and 100")
			So(validate(`
				builders {
					name: "a/b/c"
					equivalent_to {name: "a/b/c"}
				}`),
				ShouldErrLike,
				`equivalent_to.name must not refer to already defined "a/b/c" builder`)
			So(validate(`
				builders {
					name: "a/b/c"
					equivalent_to {name: "c/d/e"}
				}
				builders {
					name: "x/y/z"
					equivalent_to {name: "c/d/e"}
				}`),
				ShouldErrLike,
				`duplicate name "c/d/e"`)
		})

		Convey("owner_whitelist_group", func() {
			So(validate(`builders { name: "a/b/c" owner_whitelist_group: "ok" }`), ShouldBeNil)
			So(validate(`
				builders {
					name: "a/b/c"
					owner_whitelist_group: "ok"
				}`), ShouldBeNil)
			So(validate(`
				builders {
					name: "a/b/c"
					owner_whitelist_group: "ok"
					owner_whitelist_group: ""
					owner_whitelist_group: "also-ok"
				}`), ShouldErrLike,
				"must not be empty string")
		})

		Convey("mode_allowlist", func() {
			So(validate(`builders {name: "a/b/c" mode_allowlist: "DRY_RUN"}`), ShouldBeNil)
			So(validate(`
				builders {
					name: "a/b/c"
					mode_allowlist: "DRY_RUN"
					mode_allowlist: "FULL_RUN"
				}`), ShouldBeNil)

			So(validate(`
				builders {
					name: "a/b/c"
					mode_allowlist: "DRY"
					mode_allowlist: "FULL_RUN"
				}`), ShouldErrLike,
				"must be one of")

			So(validate(`
				builders {
					name: "a/b/c"
					mode_allowlist: "NEW_PATCHSET_RUN"
				}`), ShouldErrLike,
				"cannot be used unless a new_patchset_run_access_list is set")

			Convey("contains ANALYZER_RUN", func() {
				So(validate(`
					builders {
						name: "a/b/c"
						location_filters: {
							path_regexp: ".*"
						}
						mode_allowlist: "ANALYZER_RUN"
					}`), ShouldErrLike,
					`analyzer location filter path pattern must match`)
				So(validate(`
					builders {
						name: "a/b/c"
						location_filters: {
							gerrit_project_regexp: "proj"
							path_regexp: ".+\\.go"
						}
						mode_allowlist: "ANALYZER_RUN"
					}`), ShouldErrLike,
					`analyzer location filter must include both host and project or neither`)
				So(validate(`
					builders {
						name: "a/b/c"
						location_filters: {
							path_regexp: ".+\\.py"
							exclude: True
						}
						mode_allowlist: "ANALYZER_RUN"
					}`), ShouldErrLike,
					`location_filters exclude filters are not combinable`)
				So(validate(`
				builders {
					name: "x/y/z"
				}
				builders {
					name: "a/b/c"
					mode_allowlist: "ANALYZER_RUN"
				}`), ShouldBeNil)
				So(validate(`
				builders {
					name: "x/y/z"
				}
				builders {
					name: "a/b/c"
					mode_allowlist: "ANALYZER_RUN"
					location_filters: {
						path_regexp: ".+\\.go"
					}
				}`), ShouldBeNil)
				So(validate(`
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
				}`), ShouldBeNil)
			})
		})

		Convey("allowed combinations", func() {
			So(validate(`
				builders {
					name: "a/b/c"
					experiment_percentage: 1
					owner_whitelist_group: "owners"
				}`),
				ShouldBeNil)
			So(validate(`
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
				ShouldBeNil)
			So(validate(`
				builders {name: "pa/re/nt"}
				builders {
					name: "a/b/c"
					includable_only: true
				}`),
				ShouldBeNil)
		})

		Convey("disallowed combinations", func() {
			So(validate(`
				builders {
					name: "a/b/c"
					experiment_percentage: 1
					equivalent_to {name: "c/d/e"}}`),
				ShouldErrLike,
				"experiment_percentage is not combinable with equivalent_to")
		})

		Convey("includable_only", func() {
			So(validate(`
				builders {
					name: "a/b/c"
					experiment_percentage: 1
					includable_only: true
				}`),
				ShouldErrLike,
				"includable_only is not combinable with experiment_percentage")
			So(validate(`
				builders {
					name: "a/b/c"
					location_filters: {
						path_regexp: ".+\\.cpp"
					}
					includable_only: true
				}`),
				ShouldErrLike,
				"includable_only is not combinable with location_filters")
			So(validate(`
				builders {
					name: "a/b/c"
					mode_allowlist: "DRY_RUN"
					includable_only: true
				}`),
				ShouldErrLike,
				"includable_only is not combinable with mode_allowlist")

			So(validate(`builders {name: "one/is/enough" includable_only: true}`), ShouldBeNil)
		})
	})
}
