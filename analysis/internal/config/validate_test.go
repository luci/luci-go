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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	configpb "go.chromium.org/luci/analysis/proto/config"
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

	ftt.Run("config template is valid", t, func(t *ftt.Test) {
		content, err := os.ReadFile(
			"../../configs/services/luci-analysis-dev/config-template.cfg",
		)
		assert.Loosely(t, err, should.BeNil)
		cfg := &configpb.Config{}
		assert.Loosely(t, prototext.Unmarshal(content, cfg), should.BeNil)
		assert.Loosely(t, validate(cfg), should.BeNil)
	})

	ftt.Run("valid config is valid", t, func(t *ftt.Test) {
		cfg, err := CreatePlaceholderConfig()
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, validate(cfg), should.BeNil)
	})

	ftt.Run("monorail hostname", t, func(t *ftt.Test) {
		cfg, err := CreatePlaceholderConfig()
		assert.Loosely(t, err, should.BeNil)

		t.Run("must be specified", func(t *ftt.Test) {
			cfg.MonorailHostname = ""
			assert.Loosely(t, validate(cfg), should.ErrLike("(monorail_hostname): unspecified"))
		})
		t.Run("must be correctly formed", func(t *ftt.Test) {
			cfg.MonorailHostname = "monorail host"
			assert.Loosely(t, validate(cfg), should.ErrLike(`(monorail_hostname): does not match pattern "^[a-z][a-z9-9\\-.]{0,62}[a-z]$"`))
		})
	})
	ftt.Run("chunk GCS bucket", t, func(t *ftt.Test) {
		cfg, err := CreatePlaceholderConfig()
		assert.Loosely(t, err, should.BeNil)

		t.Run("must be specified", func(t *ftt.Test) {
			cfg.ChunkGcsBucket = ""
			assert.Loosely(t, validate(cfg), should.ErrLike(`(chunk_gcs_bucket): unspecified`))
		})
		t.Run("must be correctly formed", func(t *ftt.Test) {
			cfg, err := CreatePlaceholderConfig()
			assert.Loosely(t, err, should.BeNil)

			cfg.ChunkGcsBucket = "my bucket"
			assert.Loosely(t, validate(cfg), should.ErrLike(`(chunk_gcs_bucket): does not match pattern "^[a-z0-9][a-z0-9\\-_.]{1,220}[a-z0-9]$"`))
		})
	})
	ftt.Run("reclustering workers", t, func(t *ftt.Test) {
		cfg, err := CreatePlaceholderConfig()
		assert.Loosely(t, err, should.BeNil)

		t.Run("zero", func(t *ftt.Test) {
			cfg.ReclusteringWorkers = 0
			assert.Loosely(t, validate(cfg), should.ErrLike(`(reclustering_workers): must be specified`))
		})
		t.Run("less than one", func(t *ftt.Test) {
			cfg.ReclusteringWorkers = -1
			assert.Loosely(t, validate(cfg), should.ErrLike(`(reclustering_workers): must be in the range [1, 1000]`))
		})
		t.Run("too large", func(t *ftt.Test) {
			cfg.ReclusteringWorkers = 1001
			assert.Loosely(t, validate(cfg), should.ErrLike(`(reclustering_workers): must be in the range [1, 1000]`))
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

	ftt.Run("config template is valid", t, func(t *ftt.Test) {
		content, err := os.ReadFile(
			"../../configs/projects/chromium/luci-analysis-dev-template.cfg",
		)
		assert.Loosely(t, err, should.BeNil)
		cfg := &configpb.ProjectConfig{}
		assert.Loosely(t, prototext.Unmarshal(content, cfg), should.BeNil)
		assert.Loosely(t, validate(project, cfg), should.BeNil)
	})

	ftt.Run("clustering", t, func(t *ftt.Test) {
		cfg := CreateConfigWithBothBuganizerAndMonorail(configpb.BugSystem_MONORAIL)

		clustering := cfg.Clustering

		t.Run("may not be specified", func(t *ftt.Test) {
			cfg.Clustering = nil
			assert.Loosely(t, validate(project, cfg), should.BeNil)
		})
		t.Run("test name rules", func(t *ftt.Test) {
			rule := clustering.TestNameRules[0]
			path := `clustering / test_name_rules / [0]`
			t.Run("name", func(t *ftt.Test) {
				t.Run("unset", func(t *ftt.Test) {
					rule.Name = ""
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / name): unspecified`))
				})
				t.Run("invalid", func(t *ftt.Test) {
					rule.Name = "<script>evil()</script>"
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / name): does not match pattern "^[a-zA-Z0-9\\-(), ]+$"`))
				})
			})
			t.Run("pattern", func(t *ftt.Test) {
				t.Run("unset", func(t *ftt.Test) {
					rule.Pattern = ""
					// Make sure the like template does not refer to capture
					// groups in the pattern, to avoid other errors in this test.
					rule.LikeTemplate = "%blah%"
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / pattern): unspecified`))
				})
				t.Run("invalid", func(t *ftt.Test) {
					rule.Pattern = "["
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): pattern: error parsing regexp: missing closing ]`))
				})
			})
			t.Run("like pattern", func(t *ftt.Test) {
				t.Run("unset", func(t *ftt.Test) {
					rule.LikeTemplate = ""
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / like_template): unspecified`))
				})
				t.Run("invalid", func(t *ftt.Test) {
					rule.LikeTemplate = "blah${broken"
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): like_template: invalid use of the $ operator at position 4 in "blah${broken"`))
				})
			})
		})
		t.Run("failure reason masks", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				clustering.ReasonMaskPatterns = nil
				assert.Loosely(t, validate(project, cfg), should.BeNil)
			})
			t.Run("pattern is not specified", func(t *ftt.Test) {
				clustering.ReasonMaskPatterns[0] = ""
				assert.Loosely(t, validate(project, cfg), should.ErrLike("empty pattern is not allowed"))
			})
			t.Run("pattern is invalid", func(t *ftt.Test) {
				clustering.ReasonMaskPatterns[0] = "["
				assert.Loosely(t, validate(project, cfg), should.ErrLike("could not compile pattern: error parsing regexp: missing closing ]"))
			})
			t.Run("pattern has multiple subexpressions", func(t *ftt.Test) {
				clustering.ReasonMaskPatterns[0] = `(a)(b)`
				assert.Loosely(t, validate(project, cfg), should.ErrLike("pattern must contain exactly one parenthesised capturing subexpression indicating the text to mask"))
			})
			t.Run("non-capturing subexpressions does not count", func(t *ftt.Test) {
				clustering.ReasonMaskPatterns[0] = `^(?:\[Fixture failure\]) ([a-zA-Z0-9_]+)(?:[:])`
				assert.Loosely(t, validate(project, cfg), should.BeNil)
			})
		})
	})
	ftt.Run("metrics", t, func(t *ftt.Test) {
		cfg := CreateConfigWithBothBuganizerAndMonorail(configpb.BugSystem_MONORAIL)

		metrics := cfg.Metrics

		t.Run("may be left unspecified", func(t *ftt.Test) {
			cfg.Metrics = nil
			assert.Loosely(t, validate(project, cfg), should.BeNil)
		})
		t.Run("overrides must be valid", func(t *ftt.Test) {
			override := metrics.Overrides[0]
			t.Run("metric ID is not specified", func(t *ftt.Test) {
				override.MetricId = ""
				assert.Loosely(t, validate(project, cfg), should.ErrLike(`no metric with ID ""`))
			})
			t.Run("metric ID is invalid", func(t *ftt.Test) {
				override.MetricId = "not-exists"
				assert.Loosely(t, validate(project, cfg), should.ErrLike(`no metric with ID "not-exists"`))
			})
			t.Run("metric ID is repeated", func(t *ftt.Test) {
				metrics.Overrides[0].MetricId = "failures"
				metrics.Overrides[1].MetricId = "failures"
				assert.Loosely(t, validate(project, cfg), should.ErrLike(`metric with ID "failures" appears in collection more than once`))
			})
			t.Run("sort priority is invalid", func(t *ftt.Test) {
				override.SortPriority = proto.Int32(0)
				assert.Loosely(t, validate(project, cfg), should.ErrLike(`value must be positive`))
			})
		})
	})
	ftt.Run("bug management", t, func(t *ftt.Test) {
		cfg := CreateConfigWithBothBuganizerAndMonorail(configpb.BugSystem_BUGANIZER)
		bm := cfg.BugManagement

		t.Run("may be unspecified", func(t *ftt.Test) {
			// E.g. if project does not want to use bug management capabilities.
			cfg.BugManagement = nil
			assert.Loosely(t, validate(project, cfg), should.BeNil)
		})
		t.Run("may be empty", func(t *ftt.Test) {
			// E.g. if project does not want to use bug management capabilities.
			cfg.BugManagement = &configpb.BugManagement{}
			assert.Loosely(t, validate(project, cfg), should.BeNil)
		})
		t.Run("default bug system must be set if monorail or buganizer configured", func(t *ftt.Test) {
			bm.DefaultBugSystem = configpb.BugSystem_BUG_SYSTEM_UNSPECIFIED
			assert.Loosely(t, validate(project, cfg), should.ErrLike(`(bug_management / default_bug_system): must be specified`))
		})
		t.Run("buganizer", func(t *ftt.Test) {
			b := bm.Buganizer
			t.Run("may be unset", func(t *ftt.Test) {
				bm.DefaultBugSystem = configpb.BugSystem_MONORAIL
				bm.Buganizer = nil
				assert.Loosely(t, validate(project, cfg), should.BeNil)

				t.Run("but not if buganizer is default bug system", func(t *ftt.Test) {
					bm.DefaultBugSystem = configpb.BugSystem_BUGANIZER
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(bug_management): buganizer section is required when the default_bug_system is Buganizer`))
				})
			})
			t.Run("default component", func(t *ftt.Test) {
				path := `bug_management / buganizer / default_component`
				t.Run("must be set", func(t *ftt.Test) {
					b.DefaultComponent = nil
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be specified`))
				})
				t.Run("id must be set", func(t *ftt.Test) {
					b.DefaultComponent.Id = 0
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / id): must be specified`))
				})
				t.Run("id is non-positive", func(t *ftt.Test) {
					b.DefaultComponent.Id = -1
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / id): must be positive`))
				})
			})
		})
		t.Run("monorail", func(t *ftt.Test) {
			m := bm.Monorail
			path := `bug_management / monorail`
			t.Run("may be unset", func(t *ftt.Test) {
				bm.DefaultBugSystem = configpb.BugSystem_BUGANIZER
				bm.Monorail = nil
				assert.Loosely(t, validate(project, cfg), should.BeNil)

				t.Run("but not if monorail is default bug system", func(t *ftt.Test) {
					bm.DefaultBugSystem = configpb.BugSystem_MONORAIL
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(bug_management): monorail section is required when the default_bug_system is Monorail`))
				})
			})
			t.Run("project", func(t *ftt.Test) {
				path := path + ` / project`
				t.Run("unset", func(t *ftt.Test) {
					m.Project = ""
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): unspecified`))
				})
				t.Run("invalid", func(t *ftt.Test) {
					m.Project = "<>"
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): does not match pattern "^[a-z0-9][-a-z0-9]{0,61}[a-z0-9]$"`))
				})
			})
			t.Run("monorail hostname", func(t *ftt.Test) {
				path := path + ` / monorail_hostname`
				t.Run("unset", func(t *ftt.Test) {
					m.MonorailHostname = ""
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): unspecified`))
				})
				t.Run("invalid", func(t *ftt.Test) {
					m.MonorailHostname = "<>"
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): does not match pattern "^[a-z][a-z9-9\\-.]{0,62}[a-z]$"`))
				})
			})
			t.Run("display prefix", func(t *ftt.Test) {
				path := path + ` / display_prefix`
				t.Run("unset", func(t *ftt.Test) {
					m.DisplayPrefix = ""
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): unspecified`))
				})
				t.Run("invalid", func(t *ftt.Test) {
					m.DisplayPrefix = "<>"
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): does not match pattern "^[a-z0-9\\-.]{0,64}$"`))
				})
			})
			t.Run("priority field id", func(t *ftt.Test) {
				path := path + ` / priority_field_id`
				t.Run("unset", func(t *ftt.Test) {
					m.PriorityFieldId = 0
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be specified`))
				})
				t.Run("invalid", func(t *ftt.Test) {
					m.PriorityFieldId = -1
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be positive`))
				})
			})
			t.Run("default field values", func(t *ftt.Test) {
				path := path + ` / default_field_values`
				fieldValue := m.DefaultFieldValues[0]
				t.Run("empty", func(t *ftt.Test) {
					// Valid to have no default values.
					m.DefaultFieldValues = nil
					assert.Loosely(t, validate(project, cfg), should.BeNil)
				})
				t.Run("too many", func(t *ftt.Test) {
					m.DefaultFieldValues = make([]*configpb.MonorailFieldValue, 0, 51)
					for i := range 51 {
						m.DefaultFieldValues = append(m.DefaultFieldValues, &configpb.MonorailFieldValue{
							FieldId: int64(i + 1),
							Value:   "value",
						})
					}
					m.DefaultFieldValues[0].Value = `\0`
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): at most 50 field values may be specified`))
				})
				t.Run("unset", func(t *ftt.Test) {
					m.DefaultFieldValues[0] = nil
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / [0]): must be specified`))
				})
				t.Run("invalid - unset field ID", func(t *ftt.Test) {
					fieldValue.FieldId = 0
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / [0] / field_id): must be specified`))
				})
				t.Run("invalid - bad field value", func(t *ftt.Test) {
					fieldValue.Value = "\x00"
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / [0] / value): does not match pattern "^[[:print:]]+$"`))
				})
			})
		})
		t.Run("policies", func(t *ftt.Test) {
			policy := bm.Policies[0]
			path := "bug_management / policies"
			t.Run("may be empty", func(t *ftt.Test) {
				bm.Policies = nil
				assert.Loosely(t, validate(project, cfg), should.BeNil)
			})
			// but may have non-duplicate IDs.
			t.Run("may have multiple", func(t *ftt.Test) {
				bm.Policies = []*configpb.BugManagementPolicy{
					CreatePlaceholderBugManagementPolicy("policy-a"),
					CreatePlaceholderBugManagementPolicy("policy-b"),
				}
				assert.Loosely(t, validate(project, cfg), should.BeNil)

				t.Run("duplicate policy IDs", func(t *ftt.Test) {
					bm.Policies[1].Id = bm.Policies[0].Id
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / [1] / id): policy with ID "policy-a" appears in the collection more than once`))
				})
			})
			t.Run("too many", func(t *ftt.Test) {
				bm.Policies = []*configpb.BugManagementPolicy{}
				for i := range 51 {
					policy := CreatePlaceholderBugManagementPolicy(fmt.Sprintf("extra-%v", i))
					bm.Policies = append(bm.Policies, policy)
				}
				assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): exceeds maximum of 50 policies`))
			})
			t.Run("unset", func(t *ftt.Test) {
				bm.Policies[0] = nil
				assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / [0]): must be specified`))
			})
			t.Run("id", func(t *ftt.Test) {
				path := path + " / [0] / id"
				t.Run("unset", func(t *ftt.Test) {
					policy.Id = ""
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): unspecified`))
				})
				t.Run("invalid", func(t *ftt.Test) {
					policy.Id = "-a-"
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): does not match pattern "^[a-z]([a-z0-9-]{0,62}[a-z0-9])?$"`))
				})
				t.Run("too long", func(t *ftt.Test) {
					policy.Id = strings.Repeat("a", 65)
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): exceeds maximum allowed length of 64 bytes`))
				})
			})
			t.Run("human readable name", func(t *ftt.Test) {
				path := path + " / [0] / human_readable_name"
				t.Run("unset", func(t *ftt.Test) {
					policy.HumanReadableName = ""
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): unspecified`))
				})
				t.Run("invalid", func(t *ftt.Test) {
					policy.HumanReadableName = "\x00"
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): does not match pattern "^[[:print:]]{1,100}$"`))
				})
				t.Run("too long", func(t *ftt.Test) {
					policy.HumanReadableName = strings.Repeat("a", 101)
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): exceeds maximum allowed length of 100 bytes`))
				})
			})
			t.Run("owners", func(t *ftt.Test) {
				path := path + " / [0] / owners"
				t.Run("unset", func(t *ftt.Test) {
					policy.Owners = nil
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): at least one owner must be specified`))
				})
				t.Run("too many", func(t *ftt.Test) {
					policy.Owners = []string{}
					for range 11 {
						policy.Owners = append(policy.Owners, "blah@google.com")
					}
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): exceeds maximum of 10 owners`))
				})
				t.Run("invalid - empty", func(t *ftt.Test) {
					// Must have a @google.com owner.
					policy.Owners = []string{""}
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / [0]): unspecified`))
				})
				t.Run("invalid - non @google.com", func(t *ftt.Test) {
					// Must have a @google.com owner.
					policy.Owners = []string{"blah@blah.com"}
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+" / [0]): does not match pattern \"^[A-Za-z0-9!#$%&'*+-/=?^_`.{|}~]{1,64}@google\\\\.com$\""))
				})
				t.Run("invalid - too long", func(t *ftt.Test) {
					policy.Owners = []string{strings.Repeat("a", 65) + "@google.com"}
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / [0]): exceeds maximum allowed length of 75 bytes`))
				})
			})
			t.Run("priority", func(t *ftt.Test) {
				path := path + " / [0] / priority"
				t.Run("unset", func(t *ftt.Test) {
					policy.Priority = configpb.BuganizerPriority_BUGANIZER_PRIORITY_UNSPECIFIED
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be specified`))
				})
			})
			t.Run("metrics", func(t *ftt.Test) {
				metric := policy.Metrics[0]
				path := path + " / [0] / metrics"
				t.Run("unset", func(t *ftt.Test) {
					policy.Metrics = nil
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): at least one metric must be specified`))
				})
				t.Run("multiple", func(t *ftt.Test) {
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
							MetricId: metrics.BuildsWithTestRunsFailedDueToFlakyTests.ID.String(),
							ActivationThreshold: &configpb.MetricThreshold{
								OneDay: proto.Int64(50),
							},
							DeactivationThreshold: &configpb.MetricThreshold{
								ThreeDay: proto.Int64(1),
							},
						},
					}
					// Valid
					assert.Loosely(t, validate(project, cfg), should.BeNil)

					t.Run("duplicate IDs", func(t *ftt.Test) {
						// Invalid.
						policy.Metrics[1].MetricId = policy.Metrics[0].MetricId
						assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / [1] / metric_id): metric with ID "critical-failures-exonerated" appears in collection more than once`))
					})
					t.Run("too many", func(t *ftt.Test) {
						policy.Metrics = []*configpb.BugManagementPolicy_Metric{}
						for i := range 11 {
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
						assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): exceeds maximum of 10 metrics`))
					})
				})
				t.Run("metric ID", func(t *ftt.Test) {
					t.Run("unset", func(t *ftt.Test) {
						metric.MetricId = ""
						assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / [0] / metric_id): no metric with ID ""`))
					})
					t.Run("invalid - metric not defined", func(t *ftt.Test) {
						metric.MetricId = "not-exists"
						assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / [0] / metric_id): no metric with ID "not-exists"`))
					})
				})
				t.Run("activation threshold", func(t *ftt.Test) {
					path := path + " / [0] / activation_threshold"
					t.Run("unset", func(t *ftt.Test) {
						// An activation threshold is not required, e.g. in case of
						// policies which are paused or being removed, but where
						// existing policy activations are to be kept.
						metric.ActivationThreshold = nil
						assert.Loosely(t, validate(project, cfg), should.BeNil)
					})
					t.Run("may be empty", func(t *ftt.Test) {
						// An activation threshold is not required, e.g. in case of
						// policies which are paused or being removed, but where
						// existing policy activations are to be kept.
						metric.ActivationThreshold = &configpb.MetricThreshold{}
						assert.Loosely(t, validate(project, cfg), should.BeNil)
					})
					t.Run("invalid - non-positive threshold", func(t *ftt.Test) {
						metric.ActivationThreshold.ThreeDay = proto.Int64(0)
						assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / three_day): value must be positive`))
					})
					t.Run("invalid - too large threshold", func(t *ftt.Test) {
						metric.ActivationThreshold.SevenDay = proto.Int64(1000 * 1000 * 1000)
						assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / seven_day): value must be less than one million`))
					})
				})
				t.Run("deactivation threshold", func(t *ftt.Test) {
					path := path + " / [0] / deactivation_threshold"
					t.Run("unset", func(t *ftt.Test) {
						metric.DeactivationThreshold = nil
						assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be specified`))
					})
					t.Run("empty", func(t *ftt.Test) {
						// There must always be a way for a policy to deactivate.
						metric.DeactivationThreshold = &configpb.MetricThreshold{}
						assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): at least one of one_day, three_day and seven_day must be set`))
					})
					t.Run("invalid - non-positive threshold", func(t *ftt.Test) {
						metric.DeactivationThreshold.OneDay = proto.Int64(0)
						assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / one_day): value must be positive`))
					})
					t.Run("invalid - too large threshold", func(t *ftt.Test) {
						metric.DeactivationThreshold.ThreeDay = proto.Int64(1000 * 1000 * 1000)
						assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / three_day): value must be less than one million`))
					})
				})
			})
			t.Run("explanation", func(t *ftt.Test) {
				path := path + " / [0] / explanation"
				t.Run("unset", func(t *ftt.Test) {
					policy.Explanation = nil
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be specified`))
				})
				explanation := policy.Explanation
				t.Run("problem html", func(t *ftt.Test) {
					path := path + " / problem_html"
					t.Run("unset", func(t *ftt.Test) {
						explanation.ProblemHtml = ""
						assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be specified`))
					})
					t.Run("invalid UTF-8", func(t *ftt.Test) {
						explanation.ProblemHtml = "\xc3\x28"
						assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): not a valid UTF-8 string`))
					})
					t.Run("invalid rune", func(t *ftt.Test) {
						explanation.ProblemHtml = "a\x00"
						assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): unicode rune '\x00' at index 1 is not graphic or newline character`))
					})
					t.Run("too long", func(t *ftt.Test) {
						explanation.ProblemHtml = strings.Repeat("a", 10001)
						assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): exceeds maximum allowed length of 10000 bytes`))
					})
				})
				t.Run("action html", func(t *ftt.Test) {
					path := path + " / action_html"
					t.Run("unset", func(t *ftt.Test) {
						explanation.ActionHtml = ""
						assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be specified`))
					})
					t.Run("invalid UTF-8", func(t *ftt.Test) {
						explanation.ActionHtml = "\xc3\x28"
						assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): not a valid UTF-8 string`))
					})
					t.Run("invalid", func(t *ftt.Test) {
						explanation.ActionHtml = "a\x00"
						assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): unicode rune '\x00' at index 1 is not graphic or newline character`))
					})
					t.Run("too long", func(t *ftt.Test) {
						explanation.ActionHtml = strings.Repeat("a", 10001)
						assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): exceeds maximum allowed length of 10000 bytes`))
					})
				})
			})
			t.Run("bug template", func(t *ftt.Test) {
				path := path + " / [0] / bug_template"
				t.Run("unset", func(t *ftt.Test) {
					policy.BugTemplate = nil
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be specified`))
				})
				bugTemplate := policy.BugTemplate
				t.Run("comment template", func(t *ftt.Test) {
					path := path + " / comment_template"
					t.Run("unset", func(t *ftt.Test) {
						// May be left blank to post no comment.
						bugTemplate.CommentTemplate = ""
						assert.Loosely(t, validate(project, cfg), should.BeNil)
					})
					t.Run("too long", func(t *ftt.Test) {
						bugTemplate.CommentTemplate = strings.Repeat("a", 10001)
						assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): exceeds maximum allowed length of 10000 bytes`))
					})
					t.Run("invalid - not valid UTF-8", func(t *ftt.Test) {
						bugTemplate.CommentTemplate = "\xc3\x28"
						assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): not a valid UTF-8 string`))
					})
					t.Run("invalid - non-ASCII characters", func(t *ftt.Test) {
						bugTemplate.CommentTemplate = "a\x00"
						assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): unicode rune '\x00' at index 1 is not graphic or newline character`))
					})
					t.Run("invalid - bad field reference", func(t *ftt.Test) {
						bugTemplate.CommentTemplate = "{{.FieldNotExisting}}"

						err := validate(project, cfg)
						assert.Loosely(t, err, should.ErrLike(`(`+path+`): validate template: `))
						assert.Loosely(t, err, should.ErrLike(`can't evaluate field FieldNotExisting`))
					})
					t.Run("invalid - bad function reference", func(t *ftt.Test) {
						bugTemplate.CommentTemplate = "{{call SomeFunc}}"

						err := validate(project, cfg)
						assert.Loosely(t, err, should.ErrLike(`(`+path+`): parsing template: `))
						assert.Loosely(t, err, should.ErrLike(`function "SomeFunc" not defined`))
					})
					t.Run("invalid - output too long on simulated examples", func(t *ftt.Test) {
						// Produces 10100 letter 'a's through nested templates, which
						// exceeds the output length limit.
						bugTemplate.CommentTemplate =
							`{{define "T1"}}` + strings.Repeat("a", 100) + `{{end}}` +
								`{{define "T2"}}` + strings.Repeat(`{{template "T1"}}`, 101) + `{{end}}` +
								`{{template "T2"}}`

						err := validate(project, cfg)
						assert.Loosely(t, err, should.ErrLike(`(`+path+`): validate template: `))
						assert.Loosely(t, err, should.ErrLike(`template produced 10100 bytes of output, which exceeds the limit of 10000 bytes`))
					})
					t.Run("invalid - does not handle monorail bug", func(t *ftt.Test) {
						// Unqualified access of Buganizer Bug ID without checking bug type.
						bugTemplate.CommentTemplate = "{{.BugID.BuganizerBugID}}"

						err := validate(project, cfg)
						assert.Loosely(t, err, should.ErrLike(`(`+path+`): validate template: test case "monorail"`))
						assert.Loosely(t, err, should.ErrLike(`error calling BuganizerBugID: not a buganizer bug`))
					})
					t.Run("invalid - does not handle buganizer bug", func(t *ftt.Test) {
						// Unqualified access of Monorail Bug ID without checking bug type.
						bugTemplate.CommentTemplate = "{{.BugID.MonorailBugID}}"

						err := validate(project, cfg)
						assert.Loosely(t, err, should.ErrLike(`(`+path+`): validate template: test case "buganizer"`))
						assert.Loosely(t, err, should.ErrLike(`error calling MonorailBugID: not a monorail bug`))
					})
					t.Run("invalid - does not handle reserved bug system", func(t *ftt.Test) {
						// Access of Buganizer Bug ID based on assumption that
						// absence of monorail Bug ID implies Buganizer, without
						// considering that the system may be extended in future.
						bugTemplate.CommentTemplate = "{{if .BugID.IsMonorail}}{{.BugID.MonorailBugID}}{{else}}{{.BugID.BuganizerBugID}}{{end}}"

						err := validate(project, cfg)
						assert.Loosely(t, err, should.ErrLike(`(`+path+`): validate template: test case "neither buganizer nor monorail"`))
						assert.Loosely(t, err, should.ErrLike(`error calling BuganizerBugID: not a buganizer bug`))
					})
				})
				t.Run("buganizer", func(t *ftt.Test) {
					path := path + " / buganizer"
					t.Run("may be unset", func(t *ftt.Test) {
						// Not all policies need to avail themselves of buganizer-specific
						// features.
						bugTemplate.Buganizer = nil
						assert.Loosely(t, validate(project, cfg), should.BeNil)
					})
					buganizer := bugTemplate.Buganizer
					t.Run("hotlists", func(t *ftt.Test) {
						path := path + " / hotlists"
						t.Run("empty", func(t *ftt.Test) {
							buganizer.Hotlists = nil
							assert.Loosely(t, validate(project, cfg), should.BeNil)
						})
						t.Run("too many", func(t *ftt.Test) {
							buganizer.Hotlists = make([]int64, 0, 11)
							for range 11 {
								buganizer.Hotlists = append(buganizer.Hotlists, 1)
							}
							assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): exceeds maximum of 5 hotlists`))
						})
						t.Run("duplicate IDs", func(t *ftt.Test) {
							buganizer.Hotlists = []int64{1, 1}
							assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / [1]): ID 1 appears in collection more than once`))
						})
						t.Run("invalid - non-positive ID", func(t *ftt.Test) {
							buganizer.Hotlists[0] = 0
							assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / [0]): ID must be positive`))
						})
					})
				})
				t.Run("monorail", func(t *ftt.Test) {
					path := path + " / monorail"
					t.Run("may be unset", func(t *ftt.Test) {
						bugTemplate.Monorail = nil
						assert.Loosely(t, validate(project, cfg), should.BeNil)
					})
					monorail := bugTemplate.Monorail
					t.Run("labels", func(t *ftt.Test) {
						path := path + " / labels"
						t.Run("empty", func(t *ftt.Test) {
							monorail.Labels = nil
							assert.Loosely(t, validate(project, cfg), should.BeNil)
						})
						t.Run("too many", func(t *ftt.Test) {
							monorail.Labels = make([]string, 0, 11)
							for i := range 11 {
								monorail.Labels = append(monorail.Labels, fmt.Sprintf("label-%v", i))
							}
							assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): exceeds maximum of 5 labels`))
						})
						t.Run("duplicate labels", func(t *ftt.Test) {
							monorail.Labels = []string{"a", "A"}
							assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / [1]): label "a" appears in collection more than once`))
						})
						t.Run("invalid - empty label", func(t *ftt.Test) {
							monorail.Labels[0] = ""
							assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / [0]): unspecified`))
						})
						t.Run("invalid - bad label", func(t *ftt.Test) {
							monorail.Labels[0] = "!test"
							assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / [0]): does not match pattern "^[a-zA-Z0-9\\-]+$"`))
						})
						t.Run("invalid - too long label", func(t *ftt.Test) {
							monorail.Labels[0] = strings.Repeat("a", 61)
							assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+` / [0]): exceeds maximum allowed length of 60 bytes`))
						})
					})
				})
			})
		})
	})
	ftt.Run("test stability criteria", t, func(t *ftt.Test) {
		cfg := CreateConfigWithBothBuganizerAndMonorail(configpb.BugSystem_BUGANIZER)

		path := "test_stability_criteria"
		tsc := cfg.TestStabilityCriteria

		t.Run("may be left unset", func(t *ftt.Test) {
			cfg.TestStabilityCriteria = nil
			assert.Loosely(t, validate(project, cfg), should.BeNil)
		})
		t.Run("failure rate", func(t *ftt.Test) {
			path := path + " / failure_rate"
			fr := tsc.FailureRate
			t.Run("unset", func(t *ftt.Test) {
				tsc.FailureRate = nil
				assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be specified`))
			})
			t.Run("consecutive failure threshold", func(t *ftt.Test) {
				path := path + " / consecutive_failure_threshold"
				t.Run("unset", func(t *ftt.Test) {
					fr.ConsecutiveFailureThreshold = 0
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be specified`))
				})
				t.Run("invalid - more than ten", func(t *ftt.Test) {
					fr.ConsecutiveFailureThreshold = 11
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be in the range [1, 10]`))
				})
				t.Run("invalid - less than zero", func(t *ftt.Test) {
					fr.ConsecutiveFailureThreshold = -1
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be in the range [1, 10]`))
				})
			})
			t.Run("failure threshold", func(t *ftt.Test) {
				path := path + " / failure_threshold"
				t.Run("unset", func(t *ftt.Test) {
					fr.FailureThreshold = 0
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be specified`))
				})
				t.Run("invalid - more than ten", func(t *ftt.Test) {
					fr.FailureThreshold = 11
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be in the range [1, 10]`))
				})
				t.Run("invalid - less than zero", func(t *ftt.Test) {
					fr.FailureThreshold = -1
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be in the range [1, 10]`))
				})
			})
		})
		t.Run("flake rate", func(t *ftt.Test) {
			path := path + " / flake_rate"
			fr := tsc.FlakeRate
			t.Run("unset", func(t *ftt.Test) {
				tsc.FlakeRate = nil
				assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be specified`))
			})
			t.Run("min window", func(t *ftt.Test) {
				path := path + " / min_window"
				t.Run("may be unset", func(t *ftt.Test) {
					fr.MinWindow = 0
					assert.Loosely(t, validate(project, cfg), should.BeNil)
				})
				t.Run("invalid - too large", func(t *ftt.Test) {
					fr.MinWindow = 1_000_001
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be in the range [0, 1000000]`))
				})
				t.Run("invalid - less than zero", func(t *ftt.Test) {
					fr.MinWindow = -1
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be in the range [0, 1000000]`))
				})
			})
			t.Run("flake threshold", func(t *ftt.Test) {
				path := path + " / flake_threshold"
				t.Run("unset", func(t *ftt.Test) {
					fr.FlakeThreshold = 0
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be specified`))
				})
				t.Run("invalid - too large", func(t *ftt.Test) {
					fr.FlakeThreshold = 1_000_001
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be in the range [1, 1000000]`))
				})
				t.Run("invalid - less than zero", func(t *ftt.Test) {
					fr.FlakeThreshold = -1
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be in the range [1, 1000000]`))
				})
			})
			t.Run("flake rate threshold", func(t *ftt.Test) {
				path := path + " / flake_rate_threshold"
				t.Run("may be unset", func(t *ftt.Test) {
					fr.FlakeRateThreshold = 0
					assert.Loosely(t, validate(project, cfg), should.BeNil)
				})
				t.Run("invalid - NaN", func(t *ftt.Test) {
					fr.FlakeRateThreshold = math.NaN()
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be a finite number`))
				})
				t.Run("invalid - infinity", func(t *ftt.Test) {
					fr.FlakeRateThreshold = math.Inf(1)
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be a finite number`))
				})
				t.Run("invalid - too large", func(t *ftt.Test) {
					fr.FlakeRateThreshold = 1.0001
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be in the range [0.000000, 1.000000]`))
				})
				t.Run("invalid - less than zero", func(t *ftt.Test) {
					fr.FlakeRateThreshold = -0.0001
					assert.Loosely(t, validate(project, cfg), should.ErrLike(`(`+path+`): must be in the range [0.000000, 1.000000]`))
				})
			})
		})
	})
}
