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

func CreatePlaceholderServiceConfig() *configpb.Config {
	return &configpb.Config{
		BqArtifactExportConfig: &configpb.BqArtifactExportConfig{
			Enabled:       true,
			ExportPercent: 50,
		},
		Schemes: []*configpb.Scheme{
			{
				Id:                "flat",
				HumanReadableName: "Flat",
				Case: &configpb.Scheme_Level{
					HumanReadableName: "Test case",
				},
			},
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
		ProducerSystems: []*configpb.ProducerSystem{
			{
				System:           "buildbucket",
				NamePattern:      `^builds/(?P<build_id>[0-9]+)$`,
				DataRealmPattern: `^test|prod$`,
				ValidateCallers:  true,
				UrlTemplate:      "https://milo-prod/ui/b/${build_id}",
				UrlTemplateByDataRealm: map[string]string{
					"test": "https://milo-test/ui/b/${build_id}",
				},
			},
		},
		AndroidBuild: &configpb.AndroidBuild{
			DataRealmPattern: "^(prod|qual-staging)$",
			DataRealms: map[string]*configpb.AndroidBuild_ByDataRealmConfig{
				"prod": {
					FullBuildUrlTemplate: "https://android-build.googleplex.com/build_explorer/build_details/${build_id}/${build_target}/",
				},
			},
		},
	}
}
