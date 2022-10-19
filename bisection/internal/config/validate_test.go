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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"

	configpb "go.chromium.org/luci/bisection/proto/config"
	"go.chromium.org/luci/config/validation"
)

func TestValidateConfig(t *testing.T) {
	t.Parallel()

	validate := func(cfg *configpb.Config) error {
		ctx := validation.Context{Context: context.Background()}
		validateConfig(&ctx, cfg)
		return ctx.Finalize()
	}

	Convey("Incomplete config is invalid", t, func() {
		cfg := &configpb.Config{}
		So(validate(cfg), ShouldErrLike, "missing Gerrit config")
	})
}

func TestValidateGerritConfig(t *testing.T) {
	t.Parallel()

	validate := func(cfg *configpb.GerritConfig) error {
		ctx := validation.Context{Context: context.Background()}
		validateGerritConfig(&ctx, cfg)
		return ctx.Finalize()
	}

	Convey("Gerrit config structure", t, func() {
		Convey("must not be empty", func() {
			cfg := &configpb.GerritConfig{}
			So(validate(cfg), ShouldErrLike, "missing config for")
		})

		Convey("missing create revert settings is invalid", func() {
			cfg := &configpb.GerritConfig{
				MaxRevertibleCulpritAge: 21600,
				SubmitRevertSettings:    &configpb.GerritConfig_RevertActionSettings{},
			}
			So(validate(cfg), ShouldErrLike, "missing config for creating reverts")
		})

		Convey("missing submit revert settings is invalid", func() {
			cfg := &configpb.GerritConfig{
				MaxRevertibleCulpritAge: 21600,
				CreateRevertSettings:    &configpb.GerritConfig_RevertActionSettings{},
			}
			So(validate(cfg), ShouldErrLike, "missing config for submitting reverts")
		})
	})

	Convey("Gerrit config values", t, func() {
		cfg := &configpb.GerritConfig{
			MaxRevertibleCulpritAge: 21600,
			CreateRevertSettings:    &configpb.GerritConfig_RevertActionSettings{},
			SubmitRevertSettings:    &configpb.GerritConfig_RevertActionSettings{},
		}

		Convey("max revertible culprit age", func() {
			Convey("cannot be 0", func() {
				cfg.MaxRevertibleCulpritAge = 0
				So(validate(cfg), ShouldErrLike, "invalid - must be positive number of seconds")
			})

			Convey("cannot be negative", func() {
				cfg.MaxRevertibleCulpritAge = -21600
				So(validate(cfg), ShouldErrLike, "invalid - must be positive number of seconds")
			})

			Convey("can be positive", func() {
				cfg.MaxRevertibleCulpritAge = 21600
				So(validate(cfg), ShouldBeNil)
			})
		})
	})
}
