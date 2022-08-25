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

	atvpb "go.chromium.org/luci/analysis/proto/analyzedtestvariant"
	configpb "go.chromium.org/luci/analysis/proto/config"
)

// createPlaceholderMonorailProject Creates a placeholder Monorail project
// with default values.
func createPlaceholderMonorailProject() *configpb.MonorailProject {
	return &configpb.MonorailProject{
		Project:         "chromium",
		PriorityFieldId: 10,
		Priorities: []*configpb.MonorailPriority{
			{
				Priority: "0",
				Threshold: &configpb.ImpactThreshold{
					TestResultsFailed: &configpb.MetricThreshold{
						OneDay: proto.Int64(1500),
					},
				},
			},
			{
				Priority: "1",
				Threshold: &configpb.ImpactThreshold{
					TestResultsFailed: &configpb.MetricThreshold{
						OneDay: proto.Int64(500),
					},
				},
			},
		},
	}
}

// Creates a placeholder impact threshold config
func createPlaceholderImpactThreshold() *configpb.ImpactThreshold {
	return &configpb.ImpactThreshold{
		TestResultsFailed: &configpb.MetricThreshold{
			OneDay: proto.Int64(1000),
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

// Creates a placeholder project config with key "chromium".
func CreatePlaceholderProjectConfig() *configpb.ProjectConfig {
	return &configpb.ProjectConfig{
		Monorail:           createPlaceholderMonorailProject(),
		BugFilingThreshold: createPlaceholderImpactThreshold(),
		Realms:             createPlaceholderRealms(),
		Clustering:         createPlaceholderClustering(),
	}
}
