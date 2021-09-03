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

package config

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/config/validation"
	v2 "go.chromium.org/luci/cv/api/config/v2"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidationRules(t *testing.T) {
	t.Parallel()

	Convey("Validation Rules", t, func() {
		r := validation.NewRuleSet()
		r.Vars.Register("appid", func(context.Context) (string, error) { return "commit-queue", nil })

		addRules(r)

		patterns, err := r.ConfigPatterns(context.Background())
		So(err, ShouldBeNil)
		So(len(patterns), ShouldEqual, 2)
		Convey("project-scope cq.cfg", func() {
			So(patterns[0].ConfigSet.Match("projects/xyz"), ShouldBeTrue)
			So(patterns[0].ConfigSet.Match("projects/xyz/refs/heads/master"), ShouldBeFalse)
			So(patterns[0].Path.Match("commit-queue.cfg"), ShouldBeTrue)
		})
		Convey("service-scope migration-settings.cfg", func() {
			So(patterns[1].ConfigSet.Match("services/commit-queue"), ShouldBeTrue)
			So(patterns[1].ConfigSet.Match("projects/xyz/refs/heads/master"), ShouldBeFalse)
			So(patterns[1].Path.Match("migration-settings.cfg"), ShouldBeTrue)
		})
	})
}

const validConfigTextPB = `
	cq_status_host: "example.com"
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

func TestValidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("Validate Config", t, func() {
		vctx := &validation.Context{Context: ctx}
		configSet := "projects/foo"
		path := "cq.cfg"

		Convey("Loading bad proto", func() {
			content := []byte(` bad: "config" `)
			So(validateProject(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "unknown field")
		})

		// It's easier to manipulate Go struct than text.
		cfg := v2.Config{}
		So(proto.UnmarshalText(validConfigTextPB, &cfg), ShouldBeNil)

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
			Convey("Bad cq_status_host", func() {
				cfg.CqStatusHost = "h://@test:123//not//://@adsfhost."
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldNotBeNil)
			})
			Convey("cq_status_host not just host", func() {
				cfg.CqStatusHost = "example.com/path#fragment"
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, `should be just a host "example.com"`)
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
					cfg.ConfigGroups = append(cfg.ConfigGroups, &v2.ConfigGroup{
						Name: fmt.Sprintf("group-%d", len(cfg.ConfigGroups)),
						Gerrit: []*v2.ConfigGroup_Gerrit{
							{
								Url: orig.Gerrit[0].Url,
								Projects: []*v2.ConfigGroup_Gerrit_Project{
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
					cfg.ConfigGroups[0].Fallback = v2.Toggle_YES
					add("refs/branch-heads/.+")
					cfg.ConfigGroups[1].Fallback = v2.Toggle_YES
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
						So(vctx.Finalize(), ShouldErrLike, "duplicate config_group `name` \"bbb\" not allowed")
					})
					Convey("empty dups for now allowed", func() {
						cfg.ConfigGroups[1].Name = ""
						cfg.ConfigGroups[2].Name = ""
						validateProjectConfig(vctx, &cfg)
						So(mustWarn(vctx.Finalize()), ShouldErrLike, "specify `name`")
					})
				})
			})
		})

		Convey("ConfigGroups", func() {
			Convey("with no Name", func() {
				cfg.ConfigGroups[0].Name = ""
				validateProjectConfig(vctx, &cfg)
				So(mustWarn(vctx.Finalize()), ShouldErrLike, "please, specify `name`")
			})
			Convey("with valid Name", func() {
				cfg.ConfigGroups[0].Name = "!invalid!"
				validateProjectConfig(vctx, &cfg)
				So(mustWarn(vctx.Finalize()), ShouldErrLike, "`name` must match")
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
				cfg.ConfigGroups[0].CombineCls = &v2.CombineCLs{}
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
			Convey("Additional modes", func() {
				mode := &v2.Mode{
					Name:            "TEST_RUN",
					CqLabelValue:    1,
					TriggeringLabel: "TEST_RUN_LABEL",
					TriggeringValue: 2,
				}
				cfg.ConfigGroups[0].AdditionalModes = []*v2.Mode{mode}
				Convey("OK", func() {
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldBeNil)
				})
				Convey("Requires name", func() {
					mode.Name = ""
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, "`name` is required")
				})
				Convey("Uses reserved mode name", func() {
					for _, m := range []string{"DRY_RUN", "FULL_RUN"} {
						mode.Name = m
						validateProjectConfig(vctx, &cfg)
						So(vctx.Finalize(), ShouldErrLike, "`name` MUST not be DRY_RUN or FULL_RUN")
					}
				})
				Convey("Invalid name", func() {
					mode.Name = "~!Invalid Run Mode!~"
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, "`name` must match")
				})
				Convey("Duplicate modes", func() {
					cfg.ConfigGroups[0].AdditionalModes = []*v2.Mode{mode, mode}
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, "duplicate `name` \"TEST_RUN\" not allowed")
				})
				Convey("CQ label value out of range", func() {
					for _, val := range []int32{-1, 0, 3, 10} {
						mode.CqLabelValue = val
						validateProjectConfig(vctx, &cfg)
						So(vctx.Finalize(), ShouldErrLike, "`cq_label_value` must be either 1 or 2")
					}
				})
				Convey("Requires triggering_label", func() {
					mode.TriggeringLabel = ""
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, "`triggering_label` is required")
				})
				Convey("triggering_label must not be Commit-Queue", func() {
					mode.TriggeringLabel = "Commit-Queue"
					validateProjectConfig(vctx, &cfg)
					So(vctx.Finalize(), ShouldErrLike, "`triggering_label` MUST not be \"Commit-Queue\"")
				})
				Convey("triggering_value out of range", func() {
					for _, val := range []int32{-1, 0} {
						mode.TriggeringValue = val
						validateProjectConfig(vctx, &cfg)
						So(vctx.Finalize(), ShouldErrLike, "`triggering_value` must be > 0")
					}
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
				So(err, ShouldErrLike, "and 4 other errors")
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
				v.Fake = &v2.Verifiers_Fake{}
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, "fake verifier is not allowed")
			})
			Convey("deprecator not allowed", func() {
				v.Cqlinter = &v2.Verifiers_CQLinter{}
				validateProjectConfig(vctx, &cfg)
				So(vctx.Finalize(), ShouldErrLike, "cqlinter verifier is not allowed")
			})
			Convey("tree_status", func() {
				v.TreeStatus = &v2.Verifiers_TreeStatus{}
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
						v2.Verifiers_GerritCQAbility_UNSET)
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
					v.GerritCqAbility.AllowOwnerIfSubmittable = v2.Verifiers_GerritCQAbility_COMMIT
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
	})
}

func TestTryjobValidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("Validate Tryjob Verifier Config", t, func() {
		validate := func(textPB string) error {
			vctx := &validation.Context{Context: ctx}
			cfg := v2.Verifiers_Tryjob{}
			if err := proto.UnmarshalText(textPB, &cfg); err != nil {
				panic(err)
			}
			validateTryjobVerifier(vctx, &cfg, standardModes)
			return vctx.Finalize()
		}

		So(validate(``), ShouldErrLike, "at least 1 builder required")

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

		Convey("location_regexps", func() {
			So(validate(`builders {name: "a/b/c" location_regexp: ""}`),
				ShouldErrLike, "must not be empty")
			So(validate(`builders {name: "a/b/c" location_regexp_exclude: "*"}`),
				ShouldErrLike, "error parsing regexp: missing argument")
			So(validate(`
				builders {
					name: "a/b/c"
					location_regexp: ".+"
					location_regexp: ".+"
				}`), ShouldErrLike, "duplicate")

			So(validate(`
				builders {
					name: "a/b/c"
					location_regexp: "a/.+"
					location_regexp: "b"
					location_regexp_exclude: "redundant/but/not/caught"
				}`), ShouldBeNil)

			So(validate(`
				builders {
					name: "a/b/c"
					experiment_percentage: 50
					location_regexp: "a/.+"
					location_regexp: "b"
					location_regexp_exclude: "redundant/but/not/caught"
				}`), ShouldBeNil)

			So(mustWarn(validate(`
				builders {
					name: "a/b/c"
					location_regexp: "https://x.googlesource.com/my/repo/[+]/*.cpp"
				}`)), ShouldErrLike, `did you mean "https://x-review.googlesource.com/my/repo/[+]/*.cpp"?`)
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

			Convey("contains ANALYZER_RUN", func() {
				So(validate(`
				builders {
					name: "a/b/c"
					mode_allowlist: "DRY_RUN"
					mode_allowlist: "ANALYZER_RUN"
				}`), ShouldErrLike,
					"ANALYZER_RUN must be the only element in mode_allowlist")
				So(validate(`
					builders {
						name: "a/b/c"
						location_regexp: ".*"
						mode_allowlist: "ANALYZER_RUN"
					}`), ShouldErrLike,
					`location_regexp of an analyzer MUST either be in the format of`)
				So(validate(`
					builders {
						name: "a/b/c"
						location_regexp: "chromium-review.googlesource.com/proj/.+\\.go"
						mode_allowlist: "ANALYZER_RUN"
					}`), ShouldErrLike,
					`location_regexp of an analyzer MUST either be in the format of`)
				So(validate(`
					builders {
						name: "a/b/c"
						location_regexp_exclude: ".+\\.py"
						mode_allowlist: "ANALYZER_RUN"
					}`), ShouldErrLike,
					`location_regexp_exclude is not combinable with tryjob run in ANALYZER_RUN mode`)
				So(validate(`
				builders {
					name: "a/b/c"
					mode_allowlist: "ANALYZER_RUN"
				}`), ShouldErrLike,
					"must have at least one non-analyzer tryjob builder")
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
					location_regexp: ".+\\.go"
				}`), ShouldBeNil)
				So(validate(`
				builders {
					name: "x/y/z"
				}
				builders {
					name: "a/b/c"
					mode_allowlist: "ANALYZER_RUN"
					location_regexp: "https://chromium-review.googlesource.com/infra/[+]/.+\\.go"
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
					location_regexp: ".+\\.cpp"
					triggered_by: "c/d/e"
				}
				builders {
					name: "c/d/e"
					location_regexp: ".+\\.cpp"
				} `),
				ShouldBeNil)
			So(validate(`
				builders {name: "pa/re/nt"}
				builders {
					name: "a/b/c"
					triggered_by: "pa/re/nt"
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
					location_regexp_exclude: ".+\\.cpp"
					includable_only: true
				}`),
				ShouldErrLike,
				"includable_only is not combinable with location_regexp[_exclude]")
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

		Convey("triggered_by", func() {
			So(validate(`
				builders {name: "a/b/0" }
				builders {name: "a/b/1" triggered_by: "a/b/0"}
				builders {name: "a/b/21" triggered_by: "a/b/1"}
				builders {name: "a/b/22" triggered_by: "a/b/1"}
			`), ShouldBeNil)

			So(validate(`builders {name: "a/b/1" triggered_by: "a/b/0"}`),
				ShouldErrLike, `triggered_by must refer to an existing builder, but "a/b/0" given`)

			So(validate(`
				builders {name: "a/b/0" experiment_percentage: 10}
				builders {name: "a/b/1" triggered_by: "a/b/0"}
			`), ShouldErrLike,
				`builder a/b/1): triggered_by must refer to an existing builder without`)

			So(validate(`
				builders {
					name: "a/b/c"
					mode_allowlist: "DRY_RUN"
					triggered_by: "a/b/0"
				}`), ShouldErrLike,
				"triggered_by is not combinable with mode_allowlist")

			Convey("doesn't form loops", func() {
				So(validate(`
					builders {name: "l/oo/p" triggered_by: "l/oo/p"}
				`), ShouldErrLike, `triggered_by must refer to an existing builder without`)

				So(validate(`
					builders {name: "tri/gger/able"}
					builders {name: "l/oo/p1" triggered_by: "l/oo/p2"}
					builders {name: "l/oo/p2" triggered_by: "l/oo/p1"}
				`), ShouldErrLike, `triggered_by must refer to an existing builder without`)
			})

			Convey("avoids less restrictive location_regexp[_exclude] in children", func() {
				So(validate(`
					builders {
						name: "a/a/parent"
						location_regexp: ".+/dog/.+"
						location_regexp: ".+/snake/.+"
					}
					builders {
						name: "a/a/child" triggered_by: "a/a/parent"
						location_regexp: ".+/dog/.+"
						location_regexp: ".+/cat/.+"
					}
				`), ShouldErrLike, `but these are not in parent: .+/cat/.+`)

				So(validate(`
					builders {
						name: "a/a/parent"
						location_regexp_exclude: ".+/dog/poodle"
						location_regexp_exclude: ".+/dog/corgi"
					}
					builders {
						name: "a/a/child" triggered_by: "a/a/parent"
						location_regexp_exclude: ".+/dog/corgi"
					}
				`), ShouldErrLike, `these are only in parent: .+/dog/poodle`)

				So(validate(`
					builders {
						name: "a/a/parent"
						location_regexp:         ".+/dog/.+"
						location_regexp_exclude: ".+/dog/poodle"
						location_regexp:          ".+/cat/.+"
					}
					builders {
						name: "a/a/child" triggered_by: "a/a/parent"
						location_regexp_exclude: ".+/dog/poodle"  # necessary to comply with checks, only.
						location_regexp:         ".+/cat/.+"
						location_regexp_exclude: ".+/cat/siamese"
					}
				`), ShouldBeNil)
			})
		})
	})
}

func TestMigrationConfigValidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("Validate Config", t, func() {
		vctx := &validation.Context{Context: ctx}
		configSet := "services/commit-queue"
		path := "migration-settings.cfg"

		Convey("Loading bad proto", func() {
			content := []byte(` bad: "config" `)
			So(validateMigrationSettings(vctx, configSet, path, content), ShouldBeNil)
			So(vctx.Finalize().Error(), ShouldContainSubstring, "unknown field")
		})

		const okConfig = `
        # Realistic config.
				api_hosts {
				  host: "luci-change-verifier-dev.appspot.com"
				  project_regexp: "cq-test"
				}

				api_hosts {
				  host: "luci-change-verifier.appspot.com"
				  project_regexp: "infra(-internal)?"
				  project_regexp_exclude: "cq-test.+"
				}

				use_cv_status {
				  project_regexp: "cq-test.+"
				  project_regexp_exclude: "cq-test-bad"
				}
	  `

		Convey("OK", func() {
			Convey("fully loaded", func() {
				So(validateMigrationSettings(vctx, configSet, path, []byte(okConfig)), ShouldBeNil)
				So(vctx.Finalize(), ShouldBeNil)
			})
			Convey("empty", func() {
				So(validateMigrationSettings(vctx, configSet, path, []byte{}), ShouldBeNil)
				So(vctx.Finalize(), ShouldBeNil)
			})
		})

		Convey("Catches regexp bugs", func() {
			badConfig := strings.Replace(okConfig, `project_regexp_exclude: "cq-test-bad"`,
				`project_regexp_exclude: "(where is closing bracket?"`, 1)
			So(validateMigrationSettings(vctx, configSet, path, []byte(badConfig)), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, "error parsing regexp")
		})
	})
}

func mustHaveOnlySeverity(err error, severity validation.Severity) error {
	So(err, ShouldNotBeNil)
	for _, e := range err.(*validation.Error).Errors {
		s, ok := validation.SeverityTag.In(e)
		So(ok, ShouldBeTrue)
		So(s, ShouldEqual, severity)
	}
	return err
}

func mustWarn(err error) error {
	return mustHaveOnlySeverity(err, validation.Warning)
}

func mustError(err error) error {
	return mustHaveOnlySeverity(err, validation.Blocking)
}
