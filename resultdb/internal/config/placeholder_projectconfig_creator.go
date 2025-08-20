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
	configpb "go.chromium.org/luci/resultdb/proto/config"
)

// Creates a placeholder `GcsAllowlist` config with default values.
func createPlaceholderGcsAllowlist() []*configpb.GcsAllowList {
	return []*configpb.GcsAllowList{
		{
			Users: []string{
				"user:test@test.com",
			},
			Buckets: []string{
				"bucket",
			},
		},
	}
}

// Creates a placeholder `RbeAllowlist` config with default values.
func createPlaceholderRbeAllowlist() []*configpb.RbeAllowList {
	return []*configpb.RbeAllowList{
		{
			Users: []string{
				"user:test@test.com",
			},
			Instances: []string{
				"projects/testproject/instances/default_instance",
			},
		},
	}
}

// CreatePlaceholderProjectConfig creates a placeholder project config with
// default values.
func CreatePlaceholderProjectConfig() *configpb.ProjectConfig {
	return &configpb.ProjectConfig{
		GcsAllowList: createPlaceholderGcsAllowlist(),
		RbeAllowList: createPlaceholderRbeAllowlist(),
	}
}

func CreatePlaceHolderServiceConfig() *configpb.Config {
	return &configpb.Config{
		BqArtifactExportConfig: &configpb.BqArtifactExportConfig{
			Enabled:       true,
			ExportPercent: 50,
		},
		Schemes: []*configpb.Scheme{
			{
				Id:                "gtest",
				HumanReadableName: "GTest",
				Fine: &configpb.Scheme_Level{
					HumanReadableName: "Suite",
					ValidationRegexp:  "^[^_]+$",
				},
				Case: &configpb.Scheme_Level{
					HumanReadableName: "Method",
				},
			},
			{
				Id:                "junit",
				HumanReadableName: "JUnit",
				Coarse: &configpb.Scheme_Level{
					HumanReadableName: "Package",
				},
				Fine: &configpb.Scheme_Level{
					HumanReadableName: "Class",
				},
				Case: &configpb.Scheme_Level{
					HumanReadableName: "Method",
				},
			},
		},
	}
}
