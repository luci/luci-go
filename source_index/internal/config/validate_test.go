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

	configpb "go.chromium.org/luci/source_index/proto/config"
)

func TestValidateConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("ValidateConfig", t, func(t *ftt.Test) {
		var cfg = &configpb.Config{
			Hosts: []*configpb.Config_Host{
				{
					Host: "chromium.googlesource.com",
					Repositories: []*configpb.Config_Host_Repository{
						{
							Name:              "repo1",
							IncludeRefRegexes: []string{"^main$", "^main2$"},
						},
						{
							Name:              "repo2",
							IncludeRefRegexes: []string{"^main3$", "^main4$"},
						},
					},
				},
				{
					Host: "another-host.googlesource.com",
					Repositories: []*configpb.Config_Host_Repository{
						{
							Name:              "valid/repo",
							IncludeRefRegexes: []string{"^main$", "^main2$"},
						},
						{
							Name:              "another-repo2",
							IncludeRefRegexes: []string{"^main3$", "^main4$"},
						},
					},
				},
			},
		}

		validateCfg := func(cfg *configpb.Config) error {
			c := validation.Context{Context: context.Background()}
			validateConfig(&c, cfg)
			return c.Finalize()
		}

		t.Run("accept valid config", func(t *ftt.Test) {
			assert.Loosely(t, validateCfg(cfg), should.BeNil)
		})

		t.Run("reject invalid gitiles host", func(t *ftt.Test) {
			cfg.Hosts[1].Host = "chromium.not-googlesource.com"

			err := validateCfg(cfg)

			assert.That(t, err, should.ErrLike("hosts #2 / host"))
			assert.That(t, err, should.ErrLike("is not a valid Gitiles host"))
		})

		t.Run("reject invalid repo", func(t *ftt.Test) {
			cfg.Hosts[0].Repositories[0].Name = "not-a-valid-repo/"

			err := validateCfg(cfg)

			assert.That(t, err, should.ErrLike("hosts #1 / repositories #1 / name"))
			assert.That(t, err, should.ErrLike("does not match pattern"))
		})

		t.Run("reject invalid regex", func(t *ftt.Test) {
			cfg.Hosts[0].Repositories[0].IncludeRefRegexes[1] = "^main($"

			err := validateCfg(cfg)

			assert.That(t, err, should.ErrLike("hosts #1 / repositories #1 / include_ref_regexes #2"))
			assert.That(t, err, should.ErrLike("error parsing regexp"))
		})

		t.Run("reject empty regex", func(t *ftt.Test) {
			cfg.Hosts[0].Repositories[0].IncludeRefRegexes[1] = ""

			err := validateCfg(cfg)

			assert.That(t, err, should.ErrLike("hosts #1 / repositories #1 / include_ref_regexes #2"))
			assert.That(t, err, should.ErrLike("regex should not be empty"))
		})
	})
}
