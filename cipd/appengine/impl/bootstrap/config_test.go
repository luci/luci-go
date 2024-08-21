// Copyright 2021 The LUCI Authors.
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

package bootstrap

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/impl/memory"
	gae "go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/caching"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		configs := map[config.Set]memory.Files{
			"services/${appid}": map[string]string{},
		}
		mockConfig := func(body string) {
			configs["services/${appid}"][cachedCfg.Path] = body
		}

		ctx := gae.Use(context.Background())
		ctx = cfgclient.Use(ctx, memory.New(configs))
		ctx = caching.WithEmptyProcessCache(ctx)

		t.Run("No config", func(t *ftt.Test) {
			assert.Loosely(t, ImportConfig(ctx), should.BeNil)

			cfg, err := BootstrapConfig(ctx, "some/pkg")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg, should.BeNil)
		})

		t.Run("Broken config", func(t *ftt.Test) {
			mockConfig("broken")
			assert.Loosely(t, ImportConfig(ctx), should.ErrLike("validation errors"))
		})

		t.Run("Good config", func(t *ftt.Test) {
			mockConfig(`
				bootstrap_config {
					prefix: "pkg/a/specific"
				}
				bootstrap_config {
					prefix: "pkg/a"
				}
			`)
			assert.Loosely(t, ImportConfig(ctx), should.BeNil)

			t.Run("Scans in order", func(t *ftt.Test) {
				cfg, err := BootstrapConfig(ctx, "pkg/a/specific/zzz")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cfg.Prefix, should.Equal("pkg/a/specific"))

				cfg, err = BootstrapConfig(ctx, "pkg/a/specific")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cfg.Prefix, should.Equal("pkg/a/specific"))

				cfg, err = BootstrapConfig(ctx, "pkg/a/another")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cfg.Prefix, should.Equal("pkg/a"))
			})

			t.Run("No matching entry", func(t *ftt.Test) {
				cfg, err := BootstrapConfig(ctx, "pkg/b/another")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cfg, should.BeNil)
			})
		})
	})
}
