// Copyright 2020 The LUCI Authors.
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
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestCache(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		configs := map[config.Set]cfgmem.Files{
			"services/${appid}": {
				"services.cfg": `coordinator { admin_auth_group: "a" }`,
			},
			"projects/a": {
				"${appid}.cfg": `archive_gs_bucket: "a"`,
			},
		}

		ctx := context.Background()
		ctx = memory.Use(ctx)
		ctx, tc := testclock.UseTime(ctx, testclock.TestTimeUTC)
		ctx = cfgclient.Use(ctx, cfgmem.New(configs))
		ctx = WithStore(ctx, &Store{})

		sync := func() {
			assert.Loosely(t, Sync(ctx), should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
		}

		sync()

		t.Run("Service config cache", func(t *ftt.Test) {
			cfg, err := Config(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg.Coordinator.AdminAuthGroup, should.Equal("a"))

			configs["services/${appid}"]["services.cfg"] = `coordinator { admin_auth_group: "b" }`
			sync()

			// Still seeing the cached config.
			cfg, err = Config(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg.Coordinator.AdminAuthGroup, should.Equal("a"))

			tc.Add(2 * time.Minute)

			// The cache expired and we loaded a newer config.
			cfg, err = Config(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg.Coordinator.AdminAuthGroup, should.Equal("b"))
		})

		t.Run("Project config cache", func(t *ftt.Test) {
			cfg, err := ProjectConfig(ctx, "a")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg.ArchiveGsBucket, should.Equal("a"))

			configs["projects/a"]["${appid}.cfg"] = `archive_gs_bucket: "b"`
			sync()

			// Still seeing the cached config.
			cfg, err = ProjectConfig(ctx, "a")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg.ArchiveGsBucket, should.Equal("a"))

			tc.Add(2 * time.Minute)

			// The cache expired and we loaded a newer config.
			cfg, err = ProjectConfig(ctx, "a")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg.ArchiveGsBucket, should.Equal("b"))
		})

		t.Run("New project", func(t *ftt.Test) {
			// Missing initially.
			_, err := ProjectConfig(ctx, "new")
			assert.Loosely(t, err, should.Equal(config.ErrNoConfig))

			// Appears.
			configs["projects/new"] = cfgmem.Files{
				"${appid}.cfg": `archive_gs_bucket: "a"`,
			}
			sync()

			// Its absence is still cached.
			_, err = ProjectConfig(ctx, "new")
			assert.Loosely(t, err, should.Equal(config.ErrNoConfig))

			tc.Add(2 * time.Minute)

			// Appears now.
			cfg, err := ProjectConfig(ctx, "new")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg.ArchiveGsBucket, should.Equal("a"))
		})

		t.Run("Deleted project", func(t *ftt.Test) {
			cfg, err := ProjectConfig(ctx, "a")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg.ArchiveGsBucket, should.Equal("a"))

			// Gone.
			delete(configs, "projects/a")
			sync()

			// Old config is still cached.
			cfg, err = ProjectConfig(ctx, "a")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg.ArchiveGsBucket, should.Equal("a"))

			tc.Add(2 * time.Minute)

			// Gone now.
			_, err = ProjectConfig(ctx, "a")
			assert.Loosely(t, err, should.Equal(config.ErrNoConfig))
		})
	})
}
