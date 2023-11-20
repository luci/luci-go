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
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	atvpb "go.chromium.org/luci/analysis/proto/analyzedtestvariant"
	configpb "go.chromium.org/luci/analysis/proto/config"
)

// createPlaceholderMonorailProject creates a Monorail project config with
// fake values. The returned proto is for use at
// ProjectConfig.bug_management.monorail.
func createPlaceholderMonorailProject() *configpb.MonorailProject {
	return &configpb.MonorailProject{
		Project:         "chromium",
		PriorityFieldId: 10,
		DefaultFieldValues: []*configpb.MonorailFieldValue{
			{
				FieldId: 1234,
				Value:   "Bug",
			},
		},
		DisplayPrefix:    "crbug.com",
		MonorailHostname: "bugs.chromium.org",
	}
}

// createPlaceholderBuganizerProject creates a Buganizer project config
// with fake values. The returned proto is for use at
// ProjectConfig.bug_management.buganizer.
func createPlaceholderBuganizerProject() *configpb.BuganizerProject {
	return &configpb.BuganizerProject{
		DefaultComponent: &configpb.BuganizerComponent{
			Id: 1,
		},
	}
}

// Creates a placeholder Clustering config with default values.
func createPlaceholderClustering() *configpb.Clustering {
	return &configpb.Clustering{
		TestNameRules: []*configpb.TestNameClusteringRule{
			{
				Name:         "Google Test (Value-parameterized)",
				Pattern:      `^ninja:(?P<target>[\w/]+:\w+)/` + `(\w+/)?(?P<suite>\w+)\.(?P<case>\w+)/\w+$`,
				LikeTemplate: `ninja:${target}/%${suite}.${case}%`,
			},
			{
				Name:         "Google Test (Type-parameterized)",
				Pattern:      `^ninja:(?P<target>[\w/]+:\w+)/` + `(\w+/)?(?P<suite>\w+)/\w+\.(?P<case>\w+)$`,
				LikeTemplate: `ninja:${target}/%${suite}/%.${case}`,
			},
		},
		ReasonMaskPatterns: []string{
			`^\[Fixture failure\] ([a-zA-Z0-9_]+)[:]`,
		},
	}
}

// Creates a placeholder realms config.
func createPlaceholderRealms() []*configpb.RealmConfig {
	return []*configpb.RealmConfig{
		{
			Name: "ci",
			TestVariantAnalysis: &configpb.TestVariantAnalysisConfig{
				UpdateTestVariantTask: &configpb.UpdateTestVariantTask{
					UpdateTestVariantTaskInterval:   durationpb.New(time.Hour),
					TestVariantStatusUpdateDuration: durationpb.New(6 * time.Hour),
				},
				BqExports: []*configpb.BigQueryExport{
					{
						Table: &configpb.BigQueryExport_BigQueryTable{
							CloudProject: "test-hrd",
							Dataset:      "chromium",
							Table:        "flaky_test_variants",
						},
						Predicate: &atvpb.Predicate{},
					},
				},
			},
		},
	}
}

func createPlaceholderMetrics() *configpb.Metrics {
	return &configpb.Metrics{
		Overrides: []*configpb.Metrics_MetricOverride{
			{
				MetricId:     string(metrics.HumanClsFailedPresubmit.ID),
				IsDefault:    proto.Bool(true),
				SortPriority: proto.Int32(1000),
			},
			{
				MetricId:     string(metrics.Failures.ID),
				IsDefault:    proto.Bool(false),
				SortPriority: proto.Int32(10),
			},
		},
	}
}

func createPlaceholderBugManagementPolicies() []*configpb.BugManagementPolicy {
	return []*configpb.BugManagementPolicy{
		CreatePlaceholderBugManagementPolicy("exoneration"),
	}
}

func CreatePlaceholderBugManagementPolicy(id string) *configpb.BugManagementPolicy {
	return &configpb.BugManagementPolicy{
		Id:                id,
		Owners:            []string{"username@google.com"},
		HumanReadableName: "test variant(s) are being exonerated in presubmit",
		Priority:          configpb.BuganizerPriority_P2,
		Metrics: []*configpb.BugManagementPolicy_Metric{
			{
				MetricId: "critical-failures-exonerated",
				ActivationThreshold: &configpb.MetricThreshold{
					OneDay: proto.Int64(50),
				},
				DeactivationThreshold: &configpb.MetricThreshold{
					ThreeDay: proto.Int64(20),
				},
			},
		},
		Explanation: &configpb.BugManagementPolicy_Explanation{
			ProblemHtml: "Test variant(s) in the cluster are being exonerated because they are too flaky or failing.",
			ActionHtml:  "<ul><li>View recent failures and fix them</li><li>Demote the test(s) from CQ</li></ul>",
		},
		BugTemplate: &configpb.BugManagementPolicy_BugTemplate{
			CommentTemplate: `Policy ID: ` + id + "\n" +
				`{{if .BugID.IsBuganizer }}Buganizer Bug ID: {{ .BugID.BuganizerBugID }}{{end}}` +
				`{{if .BugID.IsMonorail }}Monorail Project: {{ .BugID.MonorailProject }}; ID: {{ .BugID.MonorailBugID }}{{end}}` +
				`Rule URL: {{.RuleURL}}`,
			Monorail: &configpb.BugManagementPolicy_BugTemplate_Monorail{
				Labels: []string{"Test-Exonerated"},
			},
			Buganizer: &configpb.BugManagementPolicy_BugTemplate_Buganizer{
				Hotlists: []int64{1234},
			},
		},
	}
}

// CreateConfigWithBothBuganizerAndMonorail creates a placeholder config
// for a project that uses both Monorail and Buganizer.
func CreateConfigWithBothBuganizerAndMonorail(defaultBugSystem configpb.BugSystem) *configpb.ProjectConfig {
	return &configpb.ProjectConfig{
		Realms:     createPlaceholderRealms(),
		Clustering: createPlaceholderClustering(),
		Metrics:    createPlaceholderMetrics(),
		BugManagement: &configpb.BugManagement{
			Policies:         createPlaceholderBugManagementPolicies(),
			DefaultBugSystem: defaultBugSystem,
			Monorail:         createPlaceholderMonorailProject(),
			Buganizer:        createPlaceholderBuganizerProject(),
		},
	}
}
