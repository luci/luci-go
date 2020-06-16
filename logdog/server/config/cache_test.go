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
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/config/server/cfgclient/backend/testconfig"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCache(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func() {
		configs := map[config.Set]memory.Files{
			"services/app-id": {
				"services.cfg": `coordinator { admin_auth_group: "a" }`,
			},
			"projects/a": {
				"app-id.cfg": `archive_gs_bucket: "a"`,
			},
		}

		ctx := context.Background()
		ctx, tc := testclock.UseTime(ctx, testclock.TestTimeUTC)
		ctx = testconfig.WithCommonClient(ctx, memory.New(configs))
		ctx = WithStore(ctx, &Store{
			ServiceID: func(context.Context) string { return "app-id" },
		})

		Convey("Service config cache", func() {
			cfg, err := Config(ctx)
			So(err, ShouldBeNil)
			So(cfg.Coordinator.AdminAuthGroup, ShouldEqual, "a")

			configs["services/app-id"]["services.cfg"] = `coordinator { admin_auth_group: "b" }`

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

			configs["projects/a"]["app-id.cfg"] = `archive_gs_bucket: "b"`

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
			configs["projects/new"] = memory.Files{
				"app-id.cfg": `archive_gs_bucket: "a"`,
			}

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
