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
	"strconv"
	"strings"
	"testing"

	"go.chromium.org/luci/config/validation"
)

const refConfigSet = "projects/foo/refs/heads/master"

func assertValidConfig(config string, t *testing.T) {
	ctx := &validation.Context{Context: context.Background()}
	if err := validateRefCfg(ctx, refConfigSet, "cq.cfg", []byte(config)); err != nil {
		t.Errorf("failed to validate %s", err)
		return
	}
	if err := ctx.Finalize(); err != nil {
		t.Errorf("unexpected validation error: %s", err)
	}
}

func assertConfigMessages(config string, t *testing.T, expectedMessages ...string) {
	ctx := &validation.Context{Context: context.Background()}
	if err := validateRefCfg(ctx, refConfigSet, "cq.cfg", []byte(config)); err != nil {
		t.Errorf("failed to validate %s", err)
		return
	}
	errs := ctx.Finalize()
	if errs == nil {
		t.Error("config is unexpectedly valid")
		return
	}
	verrs := errs.(*validation.Error).Errors

	errorf := func(fmt string, args ...interface{}) {
		args = append([]interface{}{config}, args...)
		t.Errorf("Config `%s`:\n"+fmt+".", args...)
	}

	for i, err := range verrs {
		if i >= len(expectedMessages) {
			t.Errorf("Extra actual message: `%s`", err)
			continue
		}
		if !strings.Contains(err.Error(), expectedMessages[i]) {
			errorf("Actual != expected:\n    `%s`\n  !=\n    `%s`", err.Error(), expectedMessages[i])
		}
	}
	if len(verrs) < len(expectedMessages) {
		missing := []string{}
		for _, exp := range expectedMessages[len(verrs):] {
			missing = append(missing, exp)
		}
		t.Errorf("Missing %d expected messages:\n  `%s`", len(missing), strings.Join(missing, "`\n  `"))
	}
}

func TestMissingRequiredFields(t *testing.T) {
	assertConfigMessages("", t,
		"version is a required field and must be 1",
		"verifiers is a required field",
		"gerrit is required",
		"git_repo_url is required",
	)
}

func TestLegacyAndInternalFields(t *testing.T) {
	ok := `
		version: 1
		gerrit {}
		git_repo_url: "https://x.googlesource.com/me.git"
		verifiers {
			gerrit_cq_ability { committer_list: "committers" }
			# PLACEHOLDER #
		}
	`
	assertValidConfig(ok, t)
	assertConfigMessages(ok+`cq_name: "foo"`, t,
		"cq_name is no longer used and can and should be removed. Please, do so now")
	assertConfigMessages(strings.Replace(ok, "# PLACEHOLDER #", `deprecator {}`, -1), t,
		"deprecator verifier is not allowed in customer configs. Please, remove.")
	assertConfigMessages(strings.Replace(ok, "# PLACEHOLDER #", `fake {}`, -1), t,
		"fake verifier is not allowed in customer configs. Please, remove.")

	assertConfigMessages(strings.Replace(ok, "gerrit {}", `
		gerrit{
			cq_verified_label: "cqv"
			dry_run_sets_cq_verified_label: true
		}`, -1), t,
		"gerrit.cq_verified_label is no longer supported.",
		"gerrit.dry_run_sets_cq_verified_label is no longer supported.",
	)
}

func TestGitRepoUrlWithGerrit(t *testing.T) {
	cfg := `
		version: 1
		gerrit {}
		verifiers { gerrit_cq_ability {committer_list: "whatever"} }
	`
	assertConfigMessages(cfg, t, "git_repo_url is required")
	assertConfigMessages(cfg+"\n"+`git_repo_url: "://who.uses:21/this.today"`, t,
		"git_repo_url must be a valid url: parse ://who.uses:21/this.today: missing protocol scheme")
	assertConfigMessages(cfg+"\n"+`git_repo_url: "http://a.b/c.git"`, t,
		"git_repo_url must match https://*.googlesource.com")
	assertConfigMessages(cfg+"\n"+`git_repo_url: "https://googlesource.com/c.git"`, t,
		"git_repo_url must match https://*.googlesource.com")
	assertValidConfig(cfg+"\n"+`git_repo_url: "https://example.googlesource.com/c.git"`, t)
}

func TestInvalidVersionNumber(t *testing.T) {
	assertConfigMessages(`
		version: -1
		gerrit {}
		verifiers { gerrit_cq_ability {committer_list: "whatever"} }
		git_repo_url: "https://example.googlesource.com/c.git"
	`, t, "version is a required field and must be 1")
}

func TestGerritNeedsCQAbility(t *testing.T) {
	cfg := `
		version: 1
		gerrit {}
		verifiers {}
		git_repo_url: "https://example.googlesource.com/c.git"
	`
	assertConfigMessages(cfg, t, "gerrit requires gerrit_cq_ability verifier to be used.")

	cfg = strings.Replace(cfg, "verifiers {}", `verifiers{ gerrit_cq_ability {committer_list: "w"} }`, 1)
	assertValidConfig(cfg, t)
}

func TestGerritCQAbilityVerifierNeedsCommittersList(t *testing.T) {
	cfg := `
		version: 1
		gerrit {}
		git_repo_url: "https://example.googlesource.com/c.git"
		verifiers {
			gerrit_cq_ability {}
		}
	`
	assertConfigMessages(cfg, t, "verifiers.gerrit_cq_ability requires committer_list to be set")

	cfg = strings.Replace(cfg, "gerrit_cq_ability {}", `gerrit_cq_ability {committer_list: "w"}`, 1)
	assertValidConfig(cfg, t)
}

func makeConfigWithTryJobs(try_job string) string {
	cfg := `
		version: 1
		gerrit {}
		git_repo_url: "https://example.googlesource.com/c.git"
		verifiers{
			gerrit_cq_ability {committer_list: "w"}
			try_job {}
		}
	`
	return strings.Replace(cfg, "try_job {}", try_job, -1)
}

func TestTryJobsBucketNames(t *testing.T) {
	cfg := makeConfigWithTryJobs(`
			try_job {
				buckets { name: "xyz" }
				buckets { name: "xyz" }
				buckets { name: "" }
				buckets { }
			}`)
	assertConfigMessages(cfg, t,
		"Bucket 'xyz' has been defined more than once",
		"Bucket name must be given",
		"Bucket name must be given",
	)
	cfg = makeConfigWithTryJobs(`
			try_job {
				buckets { name: "xyz" }
				buckets { name: "abc" }
			}`)
	assertValidConfig(cfg, t)
}

func TestTryJobsBuilderNames(t *testing.T) {
	cfg := makeConfigWithTryJobs(`
			try_job {
				buckets {
					name: "B"
					builders { }
					builders { name: "xyz" }
					builders { name: "xyz" }
					builders { name: "" }
				}
			}`)
	assertConfigMessages(cfg, t,
		"Bucket 'B' has builder without name",
		"Bucket 'B' builder 'xyz' has been defined more than once",
		"Bucket 'B' has builder without name",
	)
	cfg = makeConfigWithTryJobs(`
			try_job {
				buckets {
					name: "B"
					builders { name: "xyz" }
					builders { name: "abc" }
				}
			}`)
	assertValidConfig(cfg, t)
}

func TestTryJobsTooManyBuilders(t *testing.T) {
	// Ensure highly unscalable algorithms through CQ code still function.
	// aka tandrii@ says 193 builders is enough for everybody.
	mkcfg := func(builders []string) string {
		t.Logf("builders %d: %v", len(builders), builders)
		return makeConfigWithTryJobs(
			`try_job { buckets { name: "HUGE" ` + strings.Join(builders, " ") + `}}`)
	}
	builders := make([]string, 0, 194)
	for i := 1; i <= 194; i++ {
		builders = append(builders, `builders { name: "`+strconv.Itoa(i)+`" }`)
	}
	assertValidConfig(mkcfg(builders[0:193]), t)
	assertConfigMessages(mkcfg(builders), t,
		"CQ allows at most 193 builders (194 specified); contact CQ team with your use case")
}

func TestTryJobsLegacyPresubmit(t *testing.T) {
	cfg := makeConfigWithTryJobs(`
			try_job { buckets { name: "luci.infra.try"
			  builders { name: "redundant"        disable_reuse: false }
			  builders { name: "as-intended"      disable_reuse: true }
			  builders { name: "legacy-presubmit" disable_reuse: true }
			}}`)
	assertValidConfig(cfg, t)
}

func TestTryJobsValidTriggeredByNobody(t *testing.T) {
	cfg := makeConfigWithTryJobs(`
			try_job { buckets { name: "master"
				builders { name: "T" triggered_by: "C" }
				builders { name: "Q" triggered_by: "" }
			}}`)
	assertConfigMessages(cfg, t,
		"Bucket 'master' builder 'T' triggered_by non-existent builder 'C'",
		"Bucket 'master' builder 'Q' triggered_by non-existent builder ''",
	)
	cfg = makeConfigWithTryJobs(`
			try_job { buckets { name: "master"
				builders { name: "T" triggered_by: "C" }
				builders { name: "B" triggered_by: "T" }
				builders { name: "C" }
			}}`)
	assertValidConfig(cfg, t)
}

func TestTryJobsValidTriggeredByLoop(t *testing.T) {
	cfg := makeConfigWithTryJobs(`
			try_job { buckets { name: "master"
				builders { name: "C" triggered_by: "C" }
			}}`)
	assertConfigMessages(cfg, t,
		"Bucket 'master' builders ['C'] are triggered_by each other and neither can be triggered by CQ directly")

	cfg = makeConfigWithTryJobs(`
			try_job { buckets { name: "master"
				builders { name: "C" triggered_by: "T" }
				builders { name: "T" triggered_by: "C" }
				builders { name: "Q" triggered_by: "C" }
			}}`)
	assertConfigMessages(cfg, t,
		"Bucket 'master' builders ['C', 'Q', 'T'] are triggered_by each other "+
			"and neither can be triggered by CQ directly")
}

func TestTryJobsValidTriggeredByAndEquivalentTo(t *testing.T) {
	cfg := makeConfigWithTryJobs(`
			try_job { buckets { name: "master"
				builders {
					name: "C"
					triggered_by: "E"
					equivalent_to {
						bucket: "luci"
					}
				}
				builders {
					name: "D"
					equivalent_to {builder: "luci-D"}
				}
			}}`)
	assertConfigMessages(cfg, t,
		"Bucket 'master' builder 'C' has `equivalent_to` and `triggered_by`, which is not allowed",
		"Bucket 'master' builder 'D' `equivalent_to` needs a specified `bucket`",
	)
}

func TestTryJobsValidTriggeredByAndExperimental(t *testing.T) {
	cfg := makeConfigWithTryJobs(`
			try_job { buckets { name: "master"
				builders {
					name: "C"
					triggered_by: "D"
					experiment_percentage: 55
				}
				builders {name: "D"}
				builders {
					name: "E"
					triggered_by: "F"
				}
				builders {name: "F" experiment_percentage: 33}
			}}`)
	assertConfigMessages(cfg, t,
		"Bucket 'master' builder 'C' has `experiment_percentage` and `triggered_by`, which is not allowed",
	)
	cfg = makeConfigWithTryJobs(`
			try_job { buckets { name: "master"
				builders {
					name: "E"
					triggered_by: "F"
				}
				builders {name: "F" experiment_percentage: 33}
			}}`)
	assertConfigMessages(cfg, t,
		"Bucket 'master' builder 'E' is triggered by 'F', which has an `experiment_percentage` and this is not allowed",
	)
}

func TestTryJobsValidExperimentPercentage(t *testing.T) {
	cfg := makeConfigWithTryJobs(`
			try_job { buckets { name: "master"
				builders { name: "UNDER" experiment_percentage: -10 }
				builders { name: "MIN" experiment_percentage: 0 }
				builders { name: "MAX" experiment_percentage: 100 }
				builders { name: "ABOVE" experiment_percentage: 1249.5 }
			}}`)
	assertConfigMessages(cfg, t,
		"Bucket 'master' builder 'UNDER' `experimental_percentage` -10.000000 must be within 0..100",
		"Bucket 'master' builder 'ABOVE' `experimental_percentage` 1249.500000 must be within 0..100",
	)
	cfg = makeConfigWithTryJobs(`
			try_job { buckets { name: "master"
				builders { name: "C" experiment_percentage: 33 }
			}}`)
	assertValidConfig(cfg, t)
}

func TestTryJobsValidEquivalentGroupAndExperimental(t *testing.T) {
	cfg := makeConfigWithTryJobs(`
			try_job { buckets { name: "master"
				builders {
					name: "C"
					experiment_percentage: 55
					equivalent_to {
						bucket: "luci"
						builder: "C-luci"
					}
				}
			}}`)
	assertConfigMessages(cfg, t,
		"Bucket 'master' builder 'C' has `equivalent_to` and `experiment_percentage`, which is not allowed",
	)
}

func TestTryJobsValidTriggeredByEquivalentGroup(t *testing.T) {
	cfg := makeConfigWithTryJobs(`
			try_job { buckets { name: "master"
				builders { name: "C" triggered_by: "E" }
				builders { name: "E" equivalent_to {
					bucket: "luci"
					builder: "E-luci"
				}}
			}}`)
	assertConfigMessages(cfg, t,
		"Bucket 'master' builder 'C' is triggered by 'E', which has an `equivalent_to` and this is not allowed")
}

func TestTryJobsNoAliasingInEquivalentTo(t *testing.T) {
	cfg := makeConfigWithTryJobs(`
			try_job { buckets { name: "master"
				builders { name: "aliased1" }
				builders {
					name: "other"
					equivalent_to {
						bucket: "master"
						builder: "aliased1"
					}
				}

				builders {
					name: "note-the-order"
					equivalent_to {
						bucket: "master"
						builder: "aliased2"
					}
				}
				builders { name: "aliased2" }

				builders {
					name: "recursion"
					equivalent_to {
						bucket: "master"
						builder: "recursion"
					}
				}
			}}`)
	assertConfigMessages(cfg, t,
		"Bucket 'master' builder 'aliased1' should not be in main and equivalent_to places at the same time",
		"Bucket 'master' builder 'aliased2' should not be in main and equivalent_to places at the same time",
		"Bucket 'master' builder 'recursion' should not be in main and equivalent_to places at the same time",
	)
}

func TestTryJobsNoAliasingInEquivalentToOnly(t *testing.T) {
	cfg := makeConfigWithTryJobs(`
			try_job {
				buckets {
					name: "B1"
					builders {
						name: "first"
						equivalent_to {
							bucket: "master"
							builder: "two-refs"
						}
					}
				}
				buckets {
					name: "B2"
					builders {
						name: "second"
						equivalent_to {
							bucket: "master"
							builder: "two-refs"
						}
					}
				}
			}`)
	assertConfigMessages(cfg, t,
		"Bucket 'master' builder 'two-refs' should not be in more than one equivalent_to sections")
}

func TestTryJobsBadPathRegexp(t *testing.T) {
	cfg := makeConfigWithTryJobs(`
			try_job { buckets { name: "try"
			  builders {
					name: "P"
					path_regexp: "exact/is\\.fine"
				}
				builders {
					name: "EX_ONLY"
					path_regexp_exclude: "regex.is.also.fine"
				}
				# PLACEHOLDER #
			}}`)
	assertValidConfig(cfg, t)

	assertConfigMessages(strings.Replace(cfg, "# PLACEHOLDER #", `
	      builders {
					name: "P1"
					path_regexp: "*invalid-regexp"
					path_regexp_exclude: "*invalid-regexp"
					experiment_percentage: 55
				}`, -1), t,
		"Bucket 'try' builder 'P1' has `path_regexp` and `experiment_percentage`, which is not allowed",
		"Bucket 'try' builder 'P1' path_regexp=\"*invalid-regexp\" is invalid regexp: error parsing regexp: missing argument to repetition operator: `*`",
		"Bucket 'try' builder 'P1' path_regexp_exclude=\"*invalid-regexp\" is invalid regexp: error parsing regexp: missing argument to repetition operator: `*`")

	assertConfigMessages(strings.Replace(cfg, "# PLACEHOLDER #", `
	      builders {
					name: "P"
				}`, -1), t,
		"Bucket 'try' builder 'P' has been defined more than once")

	assertConfigMessages(strings.Replace(cfg, "# PLACEHOLDER #", `
				builders {
					name: "EP"
					equivalent_to { bucket: "try-ng" }
					path_regexp: ".+\\.yaml"
				}`, -1), t,
		"Bucket 'try' builder 'EP' has `path_regexp` and `equivalent_to`, which is not allowed")
}

func TestTryJobsValidComplex(t *testing.T) {
	cfg := makeConfigWithTryJobs(`
			try_job { buckets { name: "try"
				builders {
					name: "A"
					equivalent_to {
						bucket: "try-ng"
						builder: "Z"
						percentage: 42
						owner_whitelist_group: "dogfooders"
					}
				}
				builders {
					name: "B"
					equivalent_to {
						bucket: "try-ng"
						percentage: 0
					}
				}
				builders {
					name: "C"
					equivalent_to {
						bucket: "try-ng"
						builder: "C-ng"
					}
				}
				builders { name: "D" }
				builders { name: "E" triggered_by: "D"}

				builders {
					name: "P"
					path_regexp: "/never-match-but-valid"
					path_regexp: "maybe.*some\\.txt"
				}
			}}`)
	assertValidConfig(cfg, t)
}

func TestInvalidRepoUrl(t *testing.T) {
	assertConfigMessages(`
		version: 1
		gerrit {}
		verifiers { gerrit_cq_ability {committer_list: "w"} }
		git_repo_url: "abc"
	`, t, "git_repo_url must be a valid url: parse abc: invalid URI for request")
}
