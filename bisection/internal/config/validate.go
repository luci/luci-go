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
	luciproto "go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/config/validation"

	configpb "go.chromium.org/luci/bisection/proto/config"
)

func validateTestAnalysisConfig(ctx *validation.Context, testAnalysisConfig *configpb.TestAnalysisConfig) {
	ctx.Enter("test_analysis_config")
	defer ctx.Exit()
	if testAnalysisConfig == nil {
		ctx.Errorf("missing test analysis config")
		return
	}
	validateGerritConfig(ctx, testAnalysisConfig.GerritConfig)
	validateBuildConfig(ctx, testAnalysisConfig.BuildConfig)
}

func validateCompileAnalysisConfig(ctx *validation.Context, compileAnalysisConfig *configpb.CompileAnalysisConfig) {
	ctx.Enter("compile_analysis_config")
	defer ctx.Exit()
	if compileAnalysisConfig == nil {
		ctx.Errorf("missing compile analysis config")
		return
	}
	validateBuildConfig(ctx, compileAnalysisConfig.BuildConfig)
	validateGerritConfig(ctx, compileAnalysisConfig.GerritConfig)
}

func validateBuildConfig(ctx *validation.Context, cfg *configpb.BuildConfig) {
	ctx.Enter("build_config")
	defer ctx.Exit()

	if cfg == nil {
		ctx.Errorf("missing build config")
		return
	}
	validateBuilder(ctx, cfg.Builder)
}

func validateBuilder(ctx *validation.Context, cfg *configpb.Builder) {
	ctx.Enter("builder")
	defer ctx.Exit()
	if cfg == nil {
		ctx.Errorf("missing builder config")
		return
	}
	if cfg.Project == "" {
		ctx.Errorf("missing project")
	}
	if cfg.Bucket == "" {
		ctx.Errorf("missing bucket")
	}
	if cfg.Builder == "" {
		ctx.Errorf("missing builder")
	}
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
func validateProjectConfigRaw(ctx *validation.Context, project, content string) *configpb.ProjectConfig {
	// We don't validate milestone projects.
	if IsMilestoneProject(project) {
		return nil
	}
	msg := &configpb.ProjectConfig{}
	if err := luciproto.UnmarshalTextML(content, msg); err != nil {
		ctx.Errorf("failed to unmarshal as text proto: %s", err)
		return nil
	}
	validateProjectConfig(ctx, msg)
	return msg
}

func validateProjectConfig(ctx *validation.Context, cfg *configpb.ProjectConfig) {
	validateTestAnalysisConfig(ctx, cfg.TestAnalysisConfig)
	validateCompileAnalysisConfig(ctx, cfg.CompileAnalysisConfig)
}
