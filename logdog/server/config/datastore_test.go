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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/logdog/api/config/svcconfig"
)

func TestSync(t *testing.T) {
	t.Parallel()

	ftt.Run("With initial configs", t, func(t *ftt.Test) {
		configs := map[config.Set]cfgmem.Files{
			"services/${appid}": {
				"services.cfg": `coordinator { admin_auth_group: "a" }`,
			},
			"projects/proj1": {
				"${appid}.cfg": `archive_gs_bucket: "a"`,
			},
		}

		ctx := context.Background()
		ctx = memory.Use(ctx)
		ctx = cfgclient.Use(ctx, cfgmem.New(configs))

		sync := func(expectedErr string) {
			if expectedErr == "" {
				assert.Loosely(t, Sync(ctx), should.BeNil)
			} else {
				assert.Loosely(t, Sync(ctx), should.ErrLike(expectedErr))
			}
			datastore.GetTestable(ctx).CatchupIndexes()
		}

		serviceCfg := func() (string, error) {
			var cfg svcconfig.Config
			err := fromDatastore(ctx, serviceConfigKind, serviceConfigPath, &cfg)
			return cfg.Coordinator.AdminAuthGroup, err
		}

		projectCfg := func(proj string) (string, error) {
			var cfg svcconfig.ProjectConfig
			err := fromDatastore(ctx, projectConfigKind, proj, &cfg)
			return cfg.ArchiveGsBucket, err
		}

		sync("")

		svc, err := serviceCfg()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, svc, should.Equal("a"))

		prj, err := projectCfg("proj1")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, prj, should.Equal("a"))

		t.Run("No changes", func(t *ftt.Test) {
			sync("")

			svc, err := serviceCfg()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, svc, should.Equal("a"))

			prj, err := projectCfg("proj1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, prj, should.Equal("a"))
		})

		t.Run("Service config change", func(t *ftt.Test) {
			configs["services/${appid}"]["services.cfg"] = `coordinator { admin_auth_group: "b" }`

			sync("")

			svc, err := serviceCfg()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, svc, should.Equal("b"))
		})

		t.Run("Broken service config", func(t *ftt.Test) {
			configs["services/${appid}"]["services.cfg"] = `wat`

			sync("bad service config")

			svc, err := serviceCfg()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, svc, should.Equal("a")) // unchanged
		})

		t.Run("Project config change", func(t *ftt.Test) {
			configs["projects/proj1"]["${appid}.cfg"] = `archive_gs_bucket: "b"`

			sync("")

			prj, err := projectCfg("proj1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, prj, should.Equal("b"))
		})

		t.Run("Broken project config", func(t *ftt.Test) {
			configs["projects/proj1"]["${appid}.cfg"] = `wat`

			sync("bad project config")

			prj, err := projectCfg("proj1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, prj, should.Equal("a")) // unchanged
		})

		t.Run("New project", func(t *ftt.Test) {
			configs["projects/proj2"] = cfgmem.Files{
				"${appid}.cfg": `archive_gs_bucket: "new"`,
			}

			sync("")

			prj1, err := projectCfg("proj1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, prj1, should.Equal("a")) // still there

			prj2, err := projectCfg("proj2")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, prj2, should.Equal("new"))
		})

		t.Run("Removed project", func(t *ftt.Test) {
			delete(configs, "projects/proj1")

			sync("")

			_, err = projectCfg("proj1")
			assert.Loosely(t, err, should.Equal(datastore.ErrNoSuchEntity))
		})

		t.Run("Broken project doesn't block updated", func(t *ftt.Test) {
			configs["projects/proj1"]["${appid}.cfg"] = `wat`
			configs["projects/proj2"] = cfgmem.Files{
				"${appid}.cfg": `archive_gs_bucket: "new"`,
			}

			sync("bad project config")

			prj1, err := projectCfg("proj1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, prj1, should.Equal("a")) // unchanged

			prj2, err := projectCfg("proj2")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, prj2, should.Equal("new"))

			delete(configs, "projects/proj2")

			sync("bad project config")

			prj1, err = projectCfg("proj1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, prj1, should.Equal("a")) // still unchanged

			_, err = projectCfg("proj2")
			assert.Loosely(t, err, should.Equal(datastore.ErrNoSuchEntity))
		})
	})
}
