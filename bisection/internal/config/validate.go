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
	"go.chromium.org/luci/config/validation"
)

// Validates the service-wide configuration for LUCI Bisection
func validateConfig(ctx *validation.Context, cfg *configpb.Config) {
	if cfg.GerritConfig == nil {
		ctx.Errorf("missing Gerrit config")
		return
	}
	validateGerritConfig(ctx, cfg.GerritConfig)
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
}

// Helper to validate the maximum revertible culprit age in a GerritConfig
func validateCulpritAge(ctx *validation.Context, age int64) {
	ctx.Enter("max_revertible_culprit_age")
	defer ctx.Exit()

	if age <= 0 {
		ctx.Errorf("invalid - must be positive number of seconds")
	}
}
