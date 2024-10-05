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
	"testing"

	configpb "go.chromium.org/luci/bisection/proto/config"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"
)

func TestValidateProjectConfig(t *testing.T) {
	t.Parallel()

	validate := func(cfg *configpb.ProjectConfig) error {
		ctx := validation.Context{Context: context.Background()}
		validateProjectConfig(&ctx, cfg)
		return ctx.Finalize()
	}

	ftt.Run("missing test analysis config", t, func(t *ftt.Test) {
		cfg := &configpb.ProjectConfig{CompileAnalysisConfig: &configpb.CompileAnalysisConfig{
			GerritConfig: createPlaceHolderGerritConfig(),
			BuildConfig:  createPlaceHolderCompileBuildConfig(),
		}}
		assert.Loosely(t, validate(cfg), should.ErrLike("missing test analysis config"))
	})

	ftt.Run("missing compile analysis config", t, func(t *ftt.Test) {
		cfg := &configpb.ProjectConfig{TestAnalysisConfig: &configpb.TestAnalysisConfig{
			GerritConfig: createPlaceHolderGerritConfig(),
			BuildConfig:  createPlaceHolderTestBuildConfig(),
		}}
		assert.Loosely(t, validate(cfg), should.ErrLike("missing compile analysis config"))
	})
}

func TestValidateBuildConfig(t *testing.T) {
	t.Parallel()

	validate := func(cfg *configpb.BuildConfig) error {
		ctx := validation.Context{Context: context.Background()}
		validateBuildConfig(&ctx, cfg)
		return ctx.Finalize()
	}

	ftt.Run("build config is nil", t, func(t *ftt.Test) {
		assert.Loosely(t, validate(nil), should.ErrLike("missing build config"))
	})

	ftt.Run("missing builder config", t, func(t *ftt.Test) {
		cfg := &configpb.BuildConfig{}
		assert.Loosely(t, validate(cfg), should.ErrLike("missing builder config"))
	})

	ftt.Run("missing project", t, func(t *ftt.Test) {
		cfg := &configpb.BuildConfig{
			Builder: &configpb.Builder{},
		}
		assert.Loosely(t, validate(cfg), should.ErrLike("missing project"))
	})

	ftt.Run("missing bucket", t, func(t *ftt.Test) {
		cfg := &configpb.BuildConfig{
			Builder: &configpb.Builder{
				Project: "chromium",
			},
		}
		assert.Loosely(t, validate(cfg), should.ErrLike("missing bucket"))
	})

	ftt.Run("missing builder", t, func(t *ftt.Test) {
		cfg := &configpb.BuildConfig{
			Builder: &configpb.Builder{
				Project: "chromium",
				Bucket:  "findit",
			},
		}
		assert.Loosely(t, validate(cfg), should.ErrLike("missing builder"))
	})
}

func TestValidateGerritConfig(t *testing.T) {
	t.Parallel()

	validate := func(cfg *configpb.GerritConfig) error {
		ctx := validation.Context{Context: context.Background()}
		validateGerritConfig(&ctx, cfg)
		return ctx.Finalize()
	}

	ftt.Run("Gerrit config structure", t, func(t *ftt.Test) {
		t.Run("must not be empty", func(t *ftt.Test) {
			cfg := &configpb.GerritConfig{}
			assert.Loosely(t, validate(cfg), should.ErrLike("missing config for"))
		})

		t.Run("missing create revert settings is invalid", func(t *ftt.Test) {
			cfg := &configpb.GerritConfig{
				MaxRevertibleCulpritAge: 21600,
				SubmitRevertSettings:    &configpb.GerritConfig_RevertActionSettings{},
			}
			assert.Loosely(t, validate(cfg), should.ErrLike("missing config for creating reverts"))
		})

		t.Run("missing submit revert settings is invalid", func(t *ftt.Test) {
			cfg := &configpb.GerritConfig{
				MaxRevertibleCulpritAge: 21600,
				CreateRevertSettings:    &configpb.GerritConfig_RevertActionSettings{},
			}
			assert.Loosely(t, validate(cfg), should.ErrLike("missing config for submitting reverts"))
		})
	})

	ftt.Run("Gerrit config values", t, func(t *ftt.Test) {
		cfg := &configpb.GerritConfig{
			MaxRevertibleCulpritAge: 21600,
			CreateRevertSettings:    &configpb.GerritConfig_RevertActionSettings{},
			SubmitRevertSettings:    &configpb.GerritConfig_RevertActionSettings{},
		}

		t.Run("max revertible culprit age", func(t *ftt.Test) {
			t.Run("cannot be 0", func(t *ftt.Test) {
				cfg.MaxRevertibleCulpritAge = 0
				assert.Loosely(t, validate(cfg), should.ErrLike("invalid - must be positive number of seconds"))
			})

			t.Run("cannot be negative", func(t *ftt.Test) {
				cfg.MaxRevertibleCulpritAge = -21600
				assert.Loosely(t, validate(cfg), should.ErrLike("invalid - must be positive number of seconds"))
			})

			t.Run("can be positive", func(t *ftt.Test) {
				cfg.MaxRevertibleCulpritAge = 21600
				assert.Loosely(t, validate(cfg), should.BeNil)
			})
		})
	})
}
