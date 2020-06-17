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

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCache(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func() {
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
			So(Sync(ctx), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
		}

		sync()

		Convey("Service config cache", func() {
			cfg, err := Config(ctx)
			So(err, ShouldBeNil)
			So(cfg.Coordinator.AdminAuthGroup, ShouldEqual, "a")

			configs["services/${appid}"]["services.cfg"] = `coordinator { admin_auth_group: "b" }`
			sync()

			// Still seeing the cached config.
			cfg, err = Config(ctx)
			So(err, ShouldBeNil)
			So(cfg.Coordinator.AdminAuthGroup, ShouldEqual, "a")

			tc.Add(6 * time.Minute)

			// The cache expired and we loaded a newer config.
			cfg, err = Config(ctx)
			So(err, ShouldBeNil)
			So(cfg.Coordinator.AdminAuthGroup, ShouldEqual, "b")
		})

		Convey("Project config cache", func() {
			cfg, err := ProjectConfig(ctx, "a")
			So(err, ShouldBeNil)
			So(cfg.ArchiveGsBucket, ShouldEqual, "a")

			configs["projects/a"]["${appid}.cfg"] = `archive_gs_bucket: "b"`
			sync()

			// Still seeing the cached config.
			cfg, err = ProjectConfig(ctx, "a")
			So(err, ShouldBeNil)
			So(cfg.ArchiveGsBucket, ShouldEqual, "a")

			tc.Add(6 * time.Minute)

			// The cache expired and we loaded a newer config.
			cfg, err = ProjectConfig(ctx, "a")
			So(err, ShouldBeNil)
			So(cfg.ArchiveGsBucket, ShouldEqual, "b")
		})

		Convey("New project", func() {
			// Missing initially.
			_, err := ProjectConfig(ctx, "new")
			So(err, ShouldEqual, config.ErrNoConfig)

			// Appears.
			configs["projects/new"] = cfgmem.Files{
				"${appid}.cfg": `archive_gs_bucket: "a"`,
			}
			sync()

			// Its absence is still cached.
			_, err = ProjectConfig(ctx, "new")
			So(err, ShouldEqual, config.ErrNoConfig)

			tc.Add(2 * time.Minute)

			// Appears now.
			cfg, err := ProjectConfig(ctx, "new")
			So(err, ShouldBeNil)
			So(cfg.ArchiveGsBucket, ShouldEqual, "a")
		})

		Convey("Deleted project", func() {
			cfg, err := ProjectConfig(ctx, "a")
			So(err, ShouldBeNil)
			So(cfg.ArchiveGsBucket, ShouldEqual, "a")

			// Gone.
			delete(configs, "projects/a")
			sync()

			// Old config is still cached.
			cfg, err = ProjectConfig(ctx, "a")
			So(err, ShouldBeNil)
			So(cfg.ArchiveGsBucket, ShouldEqual, "a")

			tc.Add(6 * time.Minute)

			// Gone now.
			_, err = ProjectConfig(ctx, "a")
			So(err, ShouldEqual, config.ErrNoConfig)
		})
	})
}
