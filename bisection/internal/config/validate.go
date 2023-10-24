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

// Package config implements config validation for LUCI Bisection
package config

import (
	configpb "go.chromium.org/luci/bisection/proto/config"
	luciproto "go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/config/validation"
)

// Validates the service-wide configuration for LUCI Bisection
func validateConfig(ctx *validation.Context, cfg *configpb.Config) {
	if cfg.GerritConfig == nil {
		ctx.Errorf("missing Gerrit config")
		return
	}
	if cfg.AnalysisConfig == nil {
		ctx.Errorf("missing Analysis config")
		return
	}
	validateGerritConfig(ctx, cfg.GerritConfig)
	// TODO(beining@): validate test analysis config.
}

func validateTestAnalysisConfig(ctx *validation.Context, testAnalysisConfig *configpb.TestAnalysisConfig) {
	ctx.Enter("test_analysis_config")
	defer ctx.Exit()
	if testAnalysisConfig == nil {
		ctx.Errorf("missing test analysis config")
		return
	}
	validateGerritConfig(ctx, testAnalysisConfig.GerritConfig)
}

func validateCompileAnalysisConfig(ctx *validation.Context, compileAnalysisConfig *configpb.CompileAnalysisConfig) {
	ctx.Enter("compile_analysis_config")
	defer ctx.Exit()
	if compileAnalysisConfig == nil {
		ctx.Errorf("missing compile analysis config")
		return
	}
	validateGerritConfig(ctx, compileAnalysisConfig.GerritConfig)
}

// Validates the settings in a GerritConfig
func validateGerritConfig(ctx *validation.Context, cfg *configpb.GerritConfig) {
	ctx.Enter("gerrit_config")
	defer ctx.Exit()

	if cfg.CreateRevertSettings == nil {
		ctx.Errorf("missing config for creating reverts")
		return
	}
	if cfg.SubmitRevertSettings == nil {
		ctx.Errorf("missing config for submitting reverts")
		return
	}
	validateCulpritAge(ctx, cfg.MaxRevertibleCulpritAge)
	// TODO (nqmtuan): validate nthsection_config when we have it
}

// Helper to validate the maximum revertible culprit age in a GerritConfig
func validateCulpritAge(ctx *validation.Context, age int64) {
	ctx.Enter("max_revertible_culprit_age")
	defer ctx.Exit()

	if age <= 0 {
		ctx.Errorf("invalid - must be positive number of seconds")
	}
}

// validateProjectConfigRaw deserializes the project-level config message
// and passes it through the validator.
func validateProjectConfigRaw(ctx *validation.Context, content string) *configpb.ProjectConfig {
	msg := &configpb.ProjectConfig{}
	if err := luciproto.UnmarshalTextML(content, msg); err != nil {
		ctx.Errorf("failed to unmarshal as text proto: %s", err)
		return nil
	}
	validateProjectConfig(ctx, msg)
	return msg
}

func validateProjectConfig(ctx *validation.Context, cfg *configpb.ProjectConfig) {
	// TODO(beining@): invalidate empty test analysis config and compile analysis config.
	if cfg.TestAnalysisConfig != nil {
		validateTestAnalysisConfig(ctx, cfg.TestAnalysisConfig)
	}
	if cfg.CompileAnalysisConfig != nil {
		validateCompileAnalysisConfig(ctx, cfg.CompileAnalysisConfig)
	}
}
