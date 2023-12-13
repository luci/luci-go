// Copyright 2022 The LUCI Authors.
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
	"math"
	"os"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	configpb "go.chromium.org/luci/analysis/proto/config"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

const project = "fakeproject"
const chromiumMilestoneProject = "chrome-m101"

func TestServiceConfigValidator(t *testing.T) {
	t.Parallel()

	validate := func(cfg *configpb.Config) error {
		c := validation.Context{Context: context.Background()}
		validateConfig(&c, cfg)
		return c.Finalize()
	}

	Convey("config template is valid", t, func() {
		content, err := os.ReadFile(
			"../../configs/services/luci-analysis-dev/config-template.cfg",
		)
		So(err, ShouldBeNil)
		cfg := &configpb.Config{}
		So(prototext.Unmarshal(content, cfg), ShouldBeNil)
		So(validate(cfg), ShouldBeNil)
	})

	Convey("valid config is valid", t, func() {
		cfg, err := CreatePlaceholderConfig()
		So(err, ShouldBeNil)

		So(validate(cfg), ShouldBeNil)
	})

	Convey("monorail hostname", t, func() {
		cfg, err := CreatePlaceholderConfig()
		So(err, ShouldBeNil)

		Convey("must be specified", func() {
			cfg.MonorailHostname = ""
			So(validate(cfg), ShouldErrLike, "(monorail_hostname): must be specified")
		})
		Convey("must be correctly formed", func() {
			cfg.MonorailHostname = "monorail host"
			So(validate(cfg), ShouldErrLike, `(monorail_hostname): does not match pattern "^[a-z][a-z9-9\\-.]{0,62}[a-z]$"`)
		})
	})
	Convey("chunk GCS bucket", t, func() {
		cfg, err := CreatePlaceholderConfig()
		So(err, ShouldBeNil)

		Convey("must be specified", func() {
			cfg.ChunkGcsBucket = ""
			So(validate(cfg), ShouldErrLike, `(chunk_gcs_bucket): must be specified`)
		})
		Convey("must be correctly formed", func() {
			cfg, err := CreatePlaceholderConfig()
			So(err, ShouldBeNil)

			cfg.ChunkGcsBucket = "my bucket"
			So(validate(cfg), ShouldErrLike, `(chunk_gcs_bucket): does not match pattern "^[a-z0-9][a-z0-9\\-_.]{1,220}[a-z0-9]$"`)
		})
	})
	Convey("reclustering workers", t, func() {
		cfg, err := CreatePlaceholderConfig()
		So(err, ShouldBeNil)

		Convey("zero", func() {
			cfg.ReclusteringWorkers = 0
			So(validate(cfg), ShouldErrLike, `(reclustering_workers): must be specified`)
		})
		Convey("less than one", func() {
			cfg.ReclusteringWorkers = -1
			So(validate(cfg), ShouldErrLike, `(reclustering_workers): must be in the range [1, 1000]`)
		})
		Convey("too large", func() {
			cfg.ReclusteringWorkers = 1001
			So(validate(cfg), ShouldErrLike, `(reclustering_workers): must be in the range [1, 1000]`)
		})
	})
}

func TestProjectConfigValidator(t *testing.T) {
	t.Parallel()

	validate := func(project string, cfg *configpb.ProjectConfig) error {
		c := validation.Context{Context: context.Background()}
		ValidateProjectConfig(&c, project, cfg)
		return c.Finalize()
	}

	Convey("config template is valid", t, func() {
		content, err := os.ReadFile(
			"../../configs/projects/chromium/luci-analysis-dev-template.cfg",
		)
		So(err, ShouldBeNil)
		cfg := &configpb.ProjectConfig{}
		So(prototext.Unmarshal(content, cfg), ShouldBeNil)
		So(validate(project, cfg), ShouldBeNil)
	})

	Convey("clustering", t, func() {
		cfg := CreateConfigWithBothBuganizerAndMonorail(configpb.BugSystem_MONORAIL)

		clustering := cfg.Clustering

		Convey("may not be specified", func() {
			cfg.Clustering = nil
			So(validate(project, cfg), ShouldBeNil)
		})
		Convey("test name rules", func() {
			rule := clustering.TestNameRules[0]
			path := `clustering / test_name_rules / [0]`
			Convey("name", func() {
				Convey("unset", func() {
					rule.Name = ""
					So(validate(project, cfg), ShouldErrLike, `(`+path+` / name): must be specified`)
				})
				Convey("invalid", func() {
					rule.Name = "<script>evil()</script>"
					So(validate(project, cfg), ShouldErrLike, `(`+path+` / name): does not match pattern "^[a-zA-Z0-9\\-(), ]+$"`)
				})
			})
			Convey("pattern", func() {
				Convey("unset", func() {
					rule.Pattern = ""
					// Make sure the like template does not refer to capture
					// groups in the pattern, to avoid other errors in this test.
					rule.LikeTemplate = "%blah%"
					So(validate(project, cfg), ShouldErrLike, `(`+path+` / pattern): must be specified`)
				})
				Convey("invalid", func() {
					rule.Pattern = "["
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): pattern: error parsing regexp: missing closing ]`)
				})
			})
			Convey("like pattern", func() {
				Convey("unset", func() {
					rule.LikeTemplate = ""
					So(validate(project, cfg), ShouldErrLike, `(`+path+` / like_template): must be specified`)
				})
				Convey("invalid", func() {
					rule.LikeTemplate = "blah${broken"
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): like_template: invalid use of the $ operator at position 4 in "blah${broken"`)
				})
			})
		})
		Convey("failure reason masks", func() {
			Convey("empty", func() {
				clustering.ReasonMaskPatterns = nil
				So(validate(project, cfg), ShouldBeNil)
			})
			Convey("pattern is not specified", func() {
				clustering.ReasonMaskPatterns[0] = ""
				So(validate(project, cfg), ShouldErrLike, "empty pattern is not allowed")
			})
			Convey("pattern is invalid", func() {
				clustering.ReasonMaskPatterns[0] = "["
				So(validate(project, cfg), ShouldErrLike, "could not compile pattern: error parsing regexp: missing closing ]")
			})
			Convey("pattern has multiple subexpressions", func() {
				clustering.ReasonMaskPatterns[0] = `(a)(b)`
				So(validate(project, cfg), ShouldErrLike, "pattern must contain exactly one parenthesised capturing subexpression indicating the text to mask")
			})
			Convey("non-capturing subexpressions does not count", func() {
				clustering.ReasonMaskPatterns[0] = `^(?:\[Fixture failure\]) ([a-zA-Z0-9_]+)(?:[:])`
				So(validate(project, cfg), ShouldBeNil)
			})
		})
	})
	Convey("metrics", t, func() {
		cfg := CreateConfigWithBothBuganizerAndMonorail(configpb.BugSystem_MONORAIL)

		metrics := cfg.Metrics

		Convey("may be left unspecified", func() {
			cfg.Metrics = nil
			So(validate(project, cfg), ShouldBeNil)
		})
		Convey("overrides must be valid", func() {
			override := metrics.Overrides[0]
			Convey("metric ID is not specified", func() {
				override.MetricId = ""
				So(validate(project, cfg), ShouldErrLike, `no metric with ID ""`)
			})
			Convey("metric ID is invalid", func() {
				override.MetricId = "not-exists"
				So(validate(project, cfg), ShouldErrLike, `no metric with ID "not-exists"`)
			})
			Convey("metric ID is repeated", func() {
				metrics.Overrides[0].MetricId = "failures"
				metrics.Overrides[1].MetricId = "failures"
				So(validate(project, cfg), ShouldErrLike, `metric with ID "failures" appears in collection more than once`)
			})
			Convey("sort priority is invalid", func() {
				override.SortPriority = proto.Int32(0)
				So(validate(project, cfg), ShouldErrLike, `value must be positive`)
			})
		})
	})
	Convey("bug management", t, func() {
		So(printableASCIIRE.MatchString("ninja:${target}/%${suite}.${case}%"), ShouldBeTrue)
		cfg := CreateConfigWithBothBuganizerAndMonorail(configpb.BugSystem_BUGANIZER)
		bm := cfg.BugManagement

		Convey("may be unspecified", func() {
			// E.g. if project does not want to use bug management capabilities.
			cfg.BugManagement = nil
			So(validate(project, cfg), ShouldBeNil)
		})
		Convey("may be empty", func() {
			// E.g. if project does not want to use bug management capabilities.
			cfg.BugManagement = &configpb.BugManagement{}
			So(validate(project, cfg), ShouldBeNil)
		})
		Convey("default bug system must be set if monorail or buganizer configured", func() {
			bm.DefaultBugSystem = configpb.BugSystem_BUG_SYSTEM_UNSPECIFIED
			So(validate(project, cfg), ShouldErrLike, `(bug_management / default_bug_system): must be specified`)
		})
		Convey("buganizer", func() {
			b := bm.Buganizer
			Convey("may be unset", func() {
				bm.DefaultBugSystem = configpb.BugSystem_MONORAIL
				bm.Buganizer = nil
				So(validate(project, cfg), ShouldBeNil)

				Convey("but not if buganizer is default bug system", func() {
					bm.DefaultBugSystem = configpb.BugSystem_BUGANIZER
					So(validate(project, cfg), ShouldErrLike, `(bug_management): buganizer section is required when the default_bug_system is Buganizer`)
				})
			})
			Convey("default component", func() {
				path := `bug_management / buganizer / default_component`
				Convey("must be set", func() {
					b.DefaultComponent = nil
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be specified`)
				})
				Convey("id must be set", func() {
					b.DefaultComponent.Id = 0
					So(validate(project, cfg), ShouldErrLike, `(`+path+` / id): must be specified`)
				})
				Convey("id is non-positive", func() {
					b.DefaultComponent.Id = -1
					So(validate(project, cfg), ShouldErrLike, `(`+path+` / id): must be positive`)
				})
			})
		})
		Convey("monorail", func() {
			m := bm.Monorail
			path := `bug_management / monorail`
			Convey("may be unset", func() {
				bm.DefaultBugSystem = configpb.BugSystem_BUGANIZER
				bm.Monorail = nil
				So(validate(project, cfg), ShouldBeNil)

				Convey("but not if monorail is default bug system", func() {
					bm.DefaultBugSystem = configpb.BugSystem_MONORAIL
					So(validate(project, cfg), ShouldErrLike, `(bug_management): monorail section is required when the default_bug_system is Monorail`)
				})
			})
			Convey("project", func() {
				path := path + ` / project`
				Convey("unset", func() {
					m.Project = ""
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be specified`)
				})
				Convey("invalid", func() {
					m.Project = "<>"
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): does not match pattern "^[a-z0-9][-a-z0-9]{0,61}[a-z0-9]$"`)
				})
			})
			Convey("monorail hostname", func() {
				path := path + ` / monorail_hostname`
				Convey("unset", func() {
					m.MonorailHostname = ""
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be specified`)
				})
				Convey("invalid", func() {
					m.MonorailHostname = "<>"
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): does not match pattern "^[a-z][a-z9-9\\-.]{0,62}[a-z]$"`)
				})
			})
			Convey("display prefix", func() {
				path := path + ` / display_prefix`
				Convey("unset", func() {
					m.DisplayPrefix = ""
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be specified`)
				})
				Convey("invalid", func() {
					m.DisplayPrefix = "<>"
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): does not match pattern "^[a-z0-9\\-.]{0,64}$"`)
				})
			})
			Convey("priority field id", func() {
				path := path + ` / priority_field_id`
				Convey("unset", func() {
					m.PriorityFieldId = 0
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be specified`)
				})
				Convey("invalid", func() {
					m.PriorityFieldId = -1
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be positive`)
				})
			})
			Convey("default field values", func() {
				path := path + ` / default_field_values`
				fieldValue := m.DefaultFieldValues[0]
				Convey("empty", func() {
					// Valid to have no default values.
					m.DefaultFieldValues = nil
					So(validate(project, cfg), ShouldBeNil)
				})
				Convey("too many", func() {
					m.DefaultFieldValues = make([]*configpb.MonorailFieldValue, 0, 51)
					for i := 0; i < 51; i++ {
						m.DefaultFieldValues = append(m.DefaultFieldValues, &configpb.MonorailFieldValue{
							FieldId: int64(i + 1),
							Value:   "value",
						})
					}
					m.DefaultFieldValues[0].Value = `\0`
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): at most 50 field values may be specified`)
				})
				Convey("unset", func() {
					m.DefaultFieldValues[0] = nil
					So(validate(project, cfg), ShouldErrLike, `(`+path+` / [0]): must be specified`)
				})
				Convey("invalid - unset field ID", func() {
					fieldValue.FieldId = 0
					So(validate(project, cfg), ShouldErrLike, `(`+path+` / [0] / field_id): must be specified`)
				})
				Convey("invalid - bad field value", func() {
					fieldValue.Value = "\x00"
					So(validate(project, cfg), ShouldErrLike, `(`+path+` / [0] / value): does not match pattern "^[[:print:]]+$"`)
				})
			})
		})
		Convey("policies", func() {
			policy := bm.Policies[0]
			path := "bug_management / policies"
			Convey("may be empty", func() {
				bm.Policies = nil
				So(validate(project, cfg), ShouldBeNil)
			})
			// but may have non-duplicate IDs.
			Convey("may have multiple", func() {
				bm.Policies = []*configpb.BugManagementPolicy{
					CreatePlaceholderBugManagementPolicy("policy-a"),
					CreatePlaceholderBugManagementPolicy("policy-b"),
				}
				So(validate(project, cfg), ShouldBeNil)

				Convey("duplicate policy IDs", func() {
					bm.Policies[1].Id = bm.Policies[0].Id
					So(validate(project, cfg), ShouldErrLike, `(`+path+` / [1] / id): policy with ID "policy-a" appears in the collection more than once`)
				})
			})
			Convey("too many", func() {
				bm.Policies = []*configpb.BugManagementPolicy{}
				for i := 0; i < 51; i++ {
					policy := CreatePlaceholderBugManagementPolicy(fmt.Sprintf("extra-%v", i))
					bm.Policies = append(bm.Policies, policy)
				}
				So(validate(project, cfg), ShouldErrLike, `(`+path+`): exceeds maximum of 50 policies`)
			})
			Convey("unset", func() {
				bm.Policies[0] = nil
				So(validate(project, cfg), ShouldErrLike, `(`+path+` / [0]): must be specified`)
			})
			Convey("id", func() {
				path := path + " / [0] / id"
				Convey("unset", func() {
					policy.Id = ""
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be specified`)
				})
				Convey("invalid", func() {
					policy.Id = "-a-"
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): does not match pattern "^[a-z]([a-z0-9-]{0,62}[a-z0-9])?$"`)
				})
				Convey("too long", func() {
					policy.Id = strings.Repeat("a", 65)
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): exceeds maximum allowed length of 64 bytes`)
				})
			})
			Convey("human readable name", func() {
				path := path + " / [0] / human_readable_name"
				Convey("unset", func() {
					policy.HumanReadableName = ""
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be specified`)
				})
				Convey("invalid", func() {
					policy.HumanReadableName = "\x00"
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): does not match pattern "^[[:print:]]{1,100}$"`)
				})
				Convey("too long", func() {
					policy.HumanReadableName = strings.Repeat("a", 101)
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): exceeds maximum allowed length of 100 bytes`)
				})
			})
			Convey("owners", func() {
				path := path + " / [0] / owners"
				Convey("unset", func() {
					policy.Owners = nil
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): at least one owner must be specified`)
				})
				Convey("too many", func() {
					policy.Owners = []string{}
					for i := 0; i < 11; i++ {
						policy.Owners = append(policy.Owners, "blah@google.com")
					}
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): exceeds maximum of 10 owners`)
				})
				Convey("invalid - empty", func() {
					// Must have a @google.com owner.
					policy.Owners = []string{""}
					So(validate(project, cfg), ShouldErrLike, `(`+path+` / [0]): must be specified`)
				})
				Convey("invalid - non @google.com", func() {
					// Must have a @google.com owner.
					policy.Owners = []string{"blah@blah.com"}
					So(validate(project, cfg), ShouldErrLike, `(`+path+" / [0]): does not match pattern \"^[A-Za-z0-9!#$%&'*+-/=?^_`.{|}~]{1,64}@google\\\\.com$\"")
				})
				Convey("invalid - too long", func() {
					policy.Owners = []string{strings.Repeat("a", 65) + "@google.com"}
					So(validate(project, cfg), ShouldErrLike, `(`+path+` / [0]): exceeds maximum allowed length of 75 bytes`)
				})
			})
			Convey("priority", func() {
				path := path + " / [0] / priority"
				Convey("unset", func() {
					policy.Priority = configpb.BuganizerPriority_BUGANIZER_PRIORITY_UNSPECIFIED
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be specified`)
				})
			})
			Convey("metrics", func() {
				metric := policy.Metrics[0]
				path := path + " / [0] / metrics"
				Convey("unset", func() {
					policy.Metrics = nil
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): at least one metric must be specified`)
				})
				Convey("multiple", func() {
					policy.Metrics = []*configpb.BugManagementPolicy_Metric{
						{
							MetricId: metrics.CriticalFailuresExonerated.ID.String(),
							ActivationThreshold: &configpb.MetricThreshold{
								OneDay: proto.Int64(50),
							},
							DeactivationThreshold: &configpb.MetricThreshold{
								ThreeDay: proto.Int64(1),
							},
						},
						{
							MetricId: metrics.BuildsFailedDueToFlakyTests.ID.String(),
							ActivationThreshold: &configpb.MetricThreshold{
								OneDay: proto.Int64(50),
							},
							DeactivationThreshold: &configpb.MetricThreshold{
								ThreeDay: proto.Int64(1),
							},
						},
					}
					// Valid
					So(validate(project, cfg), ShouldBeNil)

					Convey("duplicate IDs", func() {
						// Invalid.
						policy.Metrics[1].MetricId = policy.Metrics[0].MetricId
						So(validate(project, cfg), ShouldErrLike, `(`+path+` / [1] / metric_id): metric with ID "critical-failures-exonerated" appears in collection more than once`)
					})
					Convey("too many", func() {
						policy.Metrics = []*configpb.BugManagementPolicy_Metric{}
						for i := 0; i < 11; i++ {
							policy.Metrics = append(policy.Metrics, &configpb.BugManagementPolicy_Metric{
								MetricId: fmt.Sprintf("metric-%v", i),
								ActivationThreshold: &configpb.MetricThreshold{
									OneDay: proto.Int64(50),
								},
								DeactivationThreshold: &configpb.MetricThreshold{
									ThreeDay: proto.Int64(1),
								},
							})
						}
						So(validate(project, cfg), ShouldErrLike, `(`+path+`): exceeds maximum of 10 metrics`)
					})
				})
				Convey("metric ID", func() {
					Convey("unset", func() {
						metric.MetricId = ""
						So(validate(project, cfg), ShouldErrLike, `(`+path+` / [0] / metric_id): no metric with ID ""`)
					})
					Convey("invalid - metric not defined", func() {
						metric.MetricId = "not-exists"
						So(validate(project, cfg), ShouldErrLike, `(`+path+` / [0] / metric_id): no metric with ID "not-exists"`)
					})
				})
				Convey("activation threshold", func() {
					path := path + " / [0] / activation_threshold"
					Convey("unset", func() {
						// An activation threshold is not required, e.g. in case of
						// policies which are paused or being removed, but where
						// existing policy activations are to be kept.
						metric.ActivationThreshold = nil
						So(validate(project, cfg), ShouldBeNil)
					})
					Convey("may be empty", func() {
						// An activation threshold is not required, e.g. in case of
						// policies which are paused or being removed, but where
						// existing policy activations are to be kept.
						metric.ActivationThreshold = &configpb.MetricThreshold{}
						So(validate(project, cfg), ShouldBeNil)
					})
					Convey("invalid - non-positive threshold", func() {
						metric.ActivationThreshold.ThreeDay = proto.Int64(0)
						So(validate(project, cfg), ShouldErrLike, `(`+path+` / three_day): value must be positive`)
					})
					Convey("invalid - too large threshold", func() {
						metric.ActivationThreshold.SevenDay = proto.Int64(1000 * 1000 * 1000)
						So(validate(project, cfg), ShouldErrLike, `(`+path+` / seven_day): value must be less than one million`)
					})
				})
				Convey("deactivation threshold", func() {
					path := path + " / [0] / deactivation_threshold"
					Convey("unset", func() {
						metric.DeactivationThreshold = nil
						So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be specified`)
					})
					Convey("empty", func() {
						// There must always be a way for a policy to deactivate.
						metric.DeactivationThreshold = &configpb.MetricThreshold{}
						So(validate(project, cfg), ShouldErrLike, `(`+path+`): at least one of one_day, three_day and seven_day must be set`)
					})
					Convey("invalid - non-positive threshold", func() {
						metric.DeactivationThreshold.OneDay = proto.Int64(0)
						So(validate(project, cfg), ShouldErrLike, `(`+path+` / one_day): value must be positive`)
					})
					Convey("invalid - too large threshold", func() {
						metric.DeactivationThreshold.ThreeDay = proto.Int64(1000 * 1000 * 1000)
						So(validate(project, cfg), ShouldErrLike, `(`+path+` / three_day): value must be less than one million`)
					})
				})
			})
			Convey("explanation", func() {
				path := path + " / [0] / explanation"
				Convey("unset", func() {
					policy.Explanation = nil
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be specified`)
				})
				explanation := policy.Explanation
				Convey("problem html", func() {
					path := path + " / problem_html"
					Convey("unset", func() {
						explanation.ProblemHtml = ""
						So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be specified`)
					})
					Convey("invalid UTF-8", func() {
						explanation.ProblemHtml = "\xc3\x28"
						So(validate(project, cfg), ShouldErrLike, `(`+path+`): not a valid UTF-8 string`)
					})
					Convey("invalid rune", func() {
						explanation.ProblemHtml = "a\x00"
						So(validate(project, cfg), ShouldErrLike, `(`+path+`): unicode rune '\x00' at index 1 is not graphic or newline character`)
					})
					Convey("too long", func() {
						explanation.ProblemHtml = strings.Repeat("a", 10001)
						So(validate(project, cfg), ShouldErrLike, `(`+path+`): exceeds maximum allowed length of 10000 bytes`)
					})
				})
				Convey("action html", func() {
					path := path + " / action_html"
					Convey("unset", func() {
						explanation.ActionHtml = ""
						So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be specified`)
					})
					Convey("invalid UTF-8", func() {
						explanation.ActionHtml = "\xc3\x28"
						So(validate(project, cfg), ShouldErrLike, `(`+path+`): not a valid UTF-8 string`)
					})
					Convey("invalid", func() {
						explanation.ActionHtml = "a\x00"
						So(validate(project, cfg), ShouldErrLike, `(`+path+`): unicode rune '\x00' at index 1 is not graphic or newline character`)
					})
					Convey("too long", func() {
						explanation.ActionHtml = strings.Repeat("a", 10001)
						So(validate(project, cfg), ShouldErrLike, `(`+path+`): exceeds maximum allowed length of 10000 bytes`)
					})
				})
			})
			Convey("bug template", func() {
				path := path + " / [0] / bug_template"
				Convey("unset", func() {
					policy.BugTemplate = nil
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be specified`)
				})
				bugTemplate := policy.BugTemplate
				Convey("comment template", func() {
					path := path + " / comment_template"
					Convey("unset", func() {
						// May be left blank to post no comment.
						bugTemplate.CommentTemplate = ""
						So(validate(project, cfg), ShouldBeNil)
					})
					Convey("too long", func() {
						bugTemplate.CommentTemplate = strings.Repeat("a", 10001)
						So(validate(project, cfg), ShouldErrLike, `(`+path+`): exceeds maximum allowed length of 10000 bytes`)
					})
					Convey("invalid - not valid UTF-8", func() {
						bugTemplate.CommentTemplate = "\xc3\x28"
						So(validate(project, cfg), ShouldErrLike, `(`+path+`): not a valid UTF-8 string`)
					})
					Convey("invalid - non-ASCII characters", func() {
						bugTemplate.CommentTemplate = "a\x00"
						So(validate(project, cfg), ShouldErrLike, `(`+path+`): unicode rune '\x00' at index 1 is not graphic or newline character`)
					})
					Convey("invalid - bad field reference", func() {
						bugTemplate.CommentTemplate = "{{.FieldNotExisting}}"

						err := validate(project, cfg)
						So(err, ShouldErrLike, `(`+path+`): validate template: `)
						So(err, ShouldErrLike, `can't evaluate field FieldNotExisting`)
					})
					Convey("invalid - bad function reference", func() {
						bugTemplate.CommentTemplate = "{{call SomeFunc}}"

						err := validate(project, cfg)
						So(err, ShouldErrLike, `(`+path+`): parsing template: `)
						So(err, ShouldErrLike, `function "SomeFunc" not defined`)
					})
					Convey("invalid - output too long on simulated examples", func() {
						// Produces 10100 letter 'a's through nested templates, which
						// exceeds the output length limit.
						bugTemplate.CommentTemplate =
							`{{define "T1"}}` + strings.Repeat("a", 100) + `{{end}}` +
								`{{define "T2"}}` + strings.Repeat(`{{template "T1"}}`, 101) + `{{end}}` +
								`{{template "T2"}}`

						err := validate(project, cfg)
						So(err, ShouldErrLike, `(`+path+`): validate template: `)
						So(err, ShouldErrLike, `template produced 10100 bytes of output, which exceeds the limit of 10000 bytes`)
					})
					Convey("invalid - does not handle monorail bug", func() {
						// Unqualified access of Buganizer Bug ID without checking bug type.
						bugTemplate.CommentTemplate = "{{.BugID.BuganizerBugID}}"

						err := validate(project, cfg)
						So(err, ShouldErrLike, `(`+path+`): validate template: test case "monorail"`)
						So(err, ShouldErrLike, `error calling BuganizerBugID: not a buganizer bug`)
					})
					Convey("invalid - does not handle buganizer bug", func() {
						// Unqualified access of Monorail Bug ID without checking bug type.
						bugTemplate.CommentTemplate = "{{.BugID.MonorailBugID}}"

						err := validate(project, cfg)
						So(err, ShouldErrLike, `(`+path+`): validate template: test case "buganizer"`)
						So(err, ShouldErrLike, `error calling MonorailBugID: not a monorail bug`)
					})
					Convey("invalid - does not handle reserved bug system", func() {
						// Access of Buganizer Bug ID based on assumption that
						// absence of monorail Bug ID implies Buganizer, without
						// considering that the system may be extended in future.
						bugTemplate.CommentTemplate = "{{if .BugID.IsMonorail}}{{.BugID.MonorailBugID}}{{else}}{{.BugID.BuganizerBugID}}{{end}}"

						err := validate(project, cfg)
						So(err, ShouldErrLike, `(`+path+`): validate template: test case "neither buganizer nor monorail"`)
						So(err, ShouldErrLike, `error calling BuganizerBugID: not a buganizer bug`)
					})
				})
				Convey("buganizer", func() {
					path := path + " / buganizer"
					Convey("may be unset", func() {
						// Not all policies need to avail themselves of buganizer-specific
						// features.
						bugTemplate.Buganizer = nil
						So(validate(project, cfg), ShouldBeNil)
					})
					buganizer := bugTemplate.Buganizer
					Convey("hotlists", func() {
						path := path + " / hotlists"
						Convey("empty", func() {
							buganizer.Hotlists = nil
							So(validate(project, cfg), ShouldBeNil)
						})
						Convey("too many", func() {
							buganizer.Hotlists = make([]int64, 0, 11)
							for i := 0; i < 11; i++ {
								buganizer.Hotlists = append(buganizer.Hotlists, 1)
							}
							So(validate(project, cfg), ShouldErrLike, `(`+path+`): exceeds maximum of 5 hotlists`)
						})
						Convey("duplicate IDs", func() {
							buganizer.Hotlists = []int64{1, 1}
							So(validate(project, cfg), ShouldErrLike, `(`+path+` / [1]): ID 1 appears in collection more than once`)
						})
						Convey("invalid - non-positive ID", func() {
							buganizer.Hotlists[0] = 0
							So(validate(project, cfg), ShouldErrLike, `(`+path+` / [0]): ID must be positive`)
						})
					})
				})
				Convey("monorail", func() {
					path := path + " / monorail"
					Convey("may be unset", func() {
						bugTemplate.Monorail = nil
						So(validate(project, cfg), ShouldBeNil)
					})
					monorail := bugTemplate.Monorail
					Convey("labels", func() {
						path := path + " / labels"
						Convey("empty", func() {
							monorail.Labels = nil
							So(validate(project, cfg), ShouldBeNil)
						})
						Convey("too many", func() {
							monorail.Labels = make([]string, 0, 11)
							for i := 0; i < 11; i++ {
								monorail.Labels = append(monorail.Labels, fmt.Sprintf("label-%v", i))
							}
							So(validate(project, cfg), ShouldErrLike, `(`+path+`): exceeds maximum of 5 labels`)
						})
						Convey("duplicate labels", func() {
							monorail.Labels = []string{"a", "A"}
							So(validate(project, cfg), ShouldErrLike, `(`+path+` / [1]): label "a" appears in collection more than once`)
						})
						Convey("invalid - empty label", func() {
							monorail.Labels[0] = ""
							So(validate(project, cfg), ShouldErrLike, `(`+path+` / [0]): must be specified`)
						})
						Convey("invalid - bad label", func() {
							monorail.Labels[0] = "!test"
							So(validate(project, cfg), ShouldErrLike, `(`+path+` / [0]): does not match pattern "^[a-zA-Z0-9\\-]+$"`)
						})
						Convey("invalid - too long label", func() {
							monorail.Labels[0] = strings.Repeat("a", 61)
							So(validate(project, cfg), ShouldErrLike, `(`+path+` / [0]): exceeds maximum allowed length of 60 bytes`)
						})
					})
				})
			})
		})
	})
	Convey("test stability criteria", t, func() {
		cfg := CreateConfigWithBothBuganizerAndMonorail(configpb.BugSystem_BUGANIZER)

		path := "test_stability_criteria"
		tsc := cfg.TestStabilityCriteria

		Convey("may be left unset", func() {
			cfg.TestStabilityCriteria = nil
			So(validate(project, cfg), ShouldBeNil)
		})
		Convey("failure rate", func() {
			path := path + " / failure_rate"
			fr := tsc.FailureRate
			Convey("unset", func() {
				tsc.FailureRate = nil
				So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be specified`)
			})
			Convey("consecutive failure threshold", func() {
				path := path + " / consecutive_failure_threshold"
				Convey("unset", func() {
					fr.ConsecutiveFailureThreshold = 0
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be specified`)
				})
				Convey("invalid - more than ten", func() {
					fr.ConsecutiveFailureThreshold = 11
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be in the range [1, 10]`)
				})
				Convey("invalid - less than zero", func() {
					fr.ConsecutiveFailureThreshold = -1
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be in the range [1, 10]`)
				})
			})
			Convey("failure threshold", func() {
				path := path + " / failure_threshold"
				Convey("unset", func() {
					fr.FailureThreshold = 0
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be specified`)
				})
				Convey("invalid - more than ten", func() {
					fr.FailureThreshold = 11
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be in the range [1, 10]`)
				})
				Convey("invalid - less than zero", func() {
					fr.FailureThreshold = -1
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be in the range [1, 10]`)
				})
			})
		})
		Convey("flake rate", func() {
			path := path + " / flake_rate"
			fr := tsc.FlakeRate
			Convey("unset", func() {
				tsc.FlakeRate = nil
				So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be specified`)
			})
			Convey("min window", func() {
				path := path + " / min_window"
				Convey("may be unset", func() {
					fr.MinWindow = 0
					So(validate(project, cfg), ShouldBeNil)
				})
				Convey("invalid - too large", func() {
					fr.MinWindow = 1_000_001
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be in the range [0, 1000000]`)
				})
				Convey("invalid - less than zero", func() {
					fr.MinWindow = -1
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be in the range [0, 1000000]`)
				})
			})
			Convey("flake threshold", func() {
				path := path + " / flake_threshold"
				Convey("unset", func() {
					fr.FlakeThreshold = 0
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be specified`)
				})
				Convey("invalid - too large", func() {
					fr.FlakeThreshold = 1_000_001
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be in the range [1, 1000000]`)
				})
				Convey("invalid - less than zero", func() {
					fr.FlakeThreshold = -1
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be in the range [1, 1000000]`)
				})
			})
			Convey("flake rate threshold", func() {
				path := path + " / flake_rate_threshold"
				Convey("may be unset", func() {
					fr.FlakeRateThreshold = 0
					So(validate(project, cfg), ShouldBeNil)
				})
				Convey("invalid - NaN", func() {
					fr.FlakeRateThreshold = math.NaN()
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be a finite number`)
				})
				Convey("invalid - infinity", func() {
					fr.FlakeRateThreshold = math.Inf(1)
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be a finite number`)
				})
				Convey("invalid - too large", func() {
					fr.FlakeRateThreshold = 1.0001
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be in the range [0.000000, 1.000000]`)
				})
				Convey("invalid - less than zero", func() {
					fr.FlakeRateThreshold = -0.0001
					So(validate(project, cfg), ShouldErrLike, `(`+path+`): must be in the range [0.000000, 1.000000]`)
				})
			})
		})
	})
}
