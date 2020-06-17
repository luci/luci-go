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

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"

	"go.chromium.org/luci/logdog/api/config/svcconfig"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSync(t *testing.T) {
	t.Parallel()

	Convey("With initial configs", t, func() {
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
				So(Sync(ctx), ShouldBeNil)
			} else {
				So(Sync(ctx), ShouldErrLike, expectedErr)
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
		So(err, ShouldBeNil)
		So(svc, ShouldEqual, "a")

		prj, err := projectCfg("proj1")
		So(err, ShouldBeNil)
		So(prj, ShouldEqual, "a")

		Convey("No changes", func() {
			sync("")

			svc, err := serviceCfg()
			So(err, ShouldBeNil)
			So(svc, ShouldEqual, "a")

			prj, err := projectCfg("proj1")
			So(err, ShouldBeNil)
			So(prj, ShouldEqual, "a")
		})

		Convey("Service config change", func() {
			configs["services/${appid}"]["services.cfg"] = `coordinator { admin_auth_group: "b" }`

			sync("")

			svc, err := serviceCfg()
			So(err, ShouldBeNil)
			So(svc, ShouldEqual, "b")
		})

		Convey("Broken service config", func() {
			configs["services/${appid}"]["services.cfg"] = `wat`

			sync("bad service config")

			svc, err := serviceCfg()
			So(err, ShouldBeNil)
			So(svc, ShouldEqual, "a") // unchanged
		})

		Convey("Project config change", func() {
			configs["projects/proj1"]["${appid}.cfg"] = `archive_gs_bucket: "b"`

			sync("")

			prj, err := projectCfg("proj1")
			So(err, ShouldBeNil)
			So(prj, ShouldEqual, "b")
		})

		Convey("Broken project config", func() {
			configs["projects/proj1"]["${appid}.cfg"] = `wat`

			sync("bad project config")

			prj, err := projectCfg("proj1")
			So(err, ShouldBeNil)
			So(prj, ShouldEqual, "a") // unchanged
		})

		Convey("New project", func() {
			configs["projects/proj2"] = cfgmem.Files{
				"${appid}.cfg": `archive_gs_bucket: "new"`,
			}

			sync("")

			prj1, err := projectCfg("proj1")
			So(err, ShouldBeNil)
			So(prj1, ShouldEqual, "a") // still there

			prj2, err := projectCfg("proj2")
			So(err, ShouldBeNil)
			So(prj2, ShouldEqual, "new")
		})

		Convey("Removed project", func() {
			delete(configs, "projects/proj1")

			sync("")

			_, err = projectCfg("proj1")
			So(err, ShouldEqual, datastore.ErrNoSuchEntity)
		})

		Convey("Broken project doesn't block updated", func() {
			configs["projects/proj1"]["${appid}.cfg"] = `wat`
			configs["projects/proj2"] = cfgmem.Files{
				"${appid}.cfg": `archive_gs_bucket: "new"`,
			}

			sync("bad project config")

			prj1, err := projectCfg("proj1")
			So(err, ShouldBeNil)
			So(prj1, ShouldEqual, "a") // unchanged

			prj2, err := projectCfg("proj2")
			So(err, ShouldBeNil)
			So(prj2, ShouldEqual, "new")

			delete(configs, "projects/proj2")

			sync("bad project config")

			prj1, err = projectCfg("proj1")
			So(err, ShouldBeNil)
			So(prj1, ShouldEqual, "a") // still unchanged

			_, err = projectCfg("proj2")
			So(err, ShouldEqual, datastore.ErrNoSuchEntity)
		})
	})
}
