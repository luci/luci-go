// Copyright 2024 The LUCI Authors.
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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"

	configpb "go.chromium.org/luci/tree_status/proto/config"
)

func TestValidateConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("ValidateConfig", t, func(t *ftt.Test) {
		var cfg = TestConfig()

		validateCfg := func(cfg *configpb.Config) error {
			c := validation.Context{Context: context.Background()}
			validateConfig(&c, cfg)
			return c.Finalize()
		}

		t.Run("accept valid config", func(t *ftt.Test) {
			assert.Loosely(t, validateCfg(cfg), should.BeNil)
		})

		t.Run("reject invalid tree name", func(t *ftt.Test) {
			cfg.Trees[0].Name = "Invalid tree"
			err := validateCfg(cfg)
			assert.That(t, err, should.ErrLike("(trees / [0] / name): expected format"))
		})

		t.Run("reject invalid project", func(t *ftt.Test) {
			cfg.Trees[0].Projects = []string{"Invalid project"}
			err := validateCfg(cfg)
			assert.That(t, err, should.ErrLike("(trees / [0] / projects / [0]): expected format"))
		})

		t.Run("reject non-default acls with empty projects", func(t *ftt.Test) {
			cfg.Trees[0].Projects = []string{}
			cfg.Trees[0].UseDefaultAcls = false
			err := validateCfg(cfg)
			assert.That(t, err, should.ErrLike("(trees / [0] / use_default_acls): projects must not be empty when not using default ACL"))
		})

		t.Run("reject multiple trees with same name", func(t *ftt.Test) {
			cfg.Trees[1].Name = "chromium"
			err := validateCfg(cfg)
			assert.That(t, err, should.ErrLike("(trees): tree name \"chromium\" is reused at indices [0 1]"))
		})

		t.Run("reject project in multiple trees", func(t *ftt.Test) {
			cfg.Trees[1].Projects = []string{"chromium"}
			err := validateCfg(cfg)
			assert.That(t, err, should.ErrLike("(trees): project \"chromium\" is in more than one tree: [chromium v8]"))
		})

	})
}
