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
	configpb "go.chromium.org/luci/bisection/proto/config"
)

func CreatePlaceholderProjectConfig() *configpb.ProjectConfig {
	return &configpb.ProjectConfig{
		CompileAnalysisConfig: &configpb.CompileAnalysisConfig{
			BuildConfig:                createPlaceHolderCompileBuildConfig(),
			CulpritVerificationEnabled: true,
			NthsectionEnabled:          true,
			GerritConfig:               createPlaceHolderGerritConfig(),
			FailureIngestionFilter: &configpb.FailureIngestionFilter{
				ExcludedBuilderGroups: []string{"chromium.clang"},
			},
		},
		TestAnalysisConfig: &configpb.TestAnalysisConfig{
			BuildConfig:     createPlaceHolderTestBuildConfig(),
			DetectorEnabled: true,
			BisectorEnabled: true,
			DailyLimit:      10,
			GerritConfig:    createPlaceHolderGerritConfig(),
		},
	}
}

func createPlaceHolderGerritConfig() *configpb.GerritConfig {
	return &configpb.GerritConfig{
		ActionsEnabled: true,
		CreateRevertSettings: &configpb.GerritConfig_RevertActionSettings{
			Enabled:    true,
			DailyLimit: 1,
		},
		SubmitRevertSettings: &configpb.GerritConfig_RevertActionSettings{
			Enabled:    true,
			DailyLimit: 1,
		},
		MaxRevertibleCulpritAge: 1,
		NthsectionSettings: &configpb.GerritConfig_NthSectionSettings{
			Enabled:                     true,
			ActionWhenVerificationError: false,
		},
	}
}

func createPlaceHolderCompileBuildConfig() *configpb.BuildConfig {
	return &configpb.BuildConfig{
		Builder: &configpb.Builder{
			Project: "chromium",
			Bucket:  "findit",
			Builder: "gofindit-culprit-verification",
		},
	}
}

func createPlaceHolderTestBuildConfig() *configpb.BuildConfig {
	return &configpb.BuildConfig{
		Builder: &configpb.Builder{
			Project: "chromium",
			Bucket:  "findit",
			Builder: "test-single-revision",
		},
	}
}
