// Copyright 2019 The LUCI Authors.
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

package monitoring

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
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
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

			cfg, err := monitoringConfig(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg, should.BeNil)
		})

		t.Run("Broken config", func(t *ftt.Test) {
			mockConfig("broken")
			assert.Loosely(t, ImportConfig(ctx), should.ErrLike("validation errors"))
		})

		t.Run("Good config", func(t *ftt.Test) {
			mockConfig(`
				client_monitoring_config {
					ip_whitelist: "ignored"
					label: "ignored-label"
				}
				client_monitoring_config {
					ip_whitelist: "bots"
					label: "bots-label"
				}
			`)
			assert.Loosely(t, ImportConfig(ctx), should.BeNil)

			t.Run("Has matching entry", func(t *ftt.Test) {
				e, err := monitoringConfig(auth.WithState(ctx, &authtest.FakeState{
					PeerIPAllowlist: []string{"bots"},
				}))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, e.Label, should.Equal("bots-label"))
			})

			t.Run("No matching entry", func(t *ftt.Test) {
				e, err := monitoringConfig(auth.WithState(ctx, &authtest.FakeState{
					PeerIPAllowlist: []string{"something-else"},
				}))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, e, should.BeNil)
			})
		})
	})
}
