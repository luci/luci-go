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
	"time"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/caching"

	configpb "go.chromium.org/luci/resultdb/proto/config"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var textPBMultiline = prototext.MarshalOptions{
	Multiline: true,
}

func TestProjectConfig(t *testing.T) {
	t.Parallel()

	Convey("SetTestProjectConfig updates context config", t, func() {
		projectA := CreatePlaceholderProjectConfig()
		configs := make(map[string]*configpb.ProjectConfig)
		configs["a"] = projectA

		ctx := memory.Use(context.Background())
		So(SetTestProjectConfig(ctx, configs), ShouldBeNil)

		cfg, err := Projects(ctx)

		So(err, ShouldBeNil)
		So(len(cfg), ShouldEqual, 1)
		So(cfg["a"], ShouldResembleProto, projectA)
	})

	Convey("With mocks", t, func() {
		projectA := CreatePlaceholderProjectConfig()
		projectB := CreatePlaceholderProjectConfig()
		So(len(projectB.GcsAllowList), ShouldEqual, 1)
		projectB.GcsAllowList[0].Users = []string{"user:b@test.com"}

		configs := map[config.Set]cfgmem.Files{
			"projects/a": {"${appid}.cfg": textPBMultiline.Format(projectA)},
			"projects/b": {"${appid}.cfg": textPBMultiline.Format(projectB)},
		}

		ctx := memory.Use(context.Background())
		ctx, tc := testclock.UseTime(ctx, testclock.TestTimeUTC)
		ctx = cfgclient.Use(ctx, cfgmem.New(configs))
		ctx = caching.WithEmptyProcessCache(ctx)

		Convey("Update works", func() {
			// Initial update.
			err := UpdateProjects(ctx)
			So(err, ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Get works.
			projects, err := Projects(ctx)
			So(err, ShouldBeNil)
			So(len(projects), ShouldEqual, 2)
			So(projects["a"], ShouldResembleProto, projectA)
			So(projects["b"], ShouldResembleProto, projectB)

			// Noop update.
			err = UpdateProjects(ctx)
			So(err, ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Real update.
			projectC := CreatePlaceholderProjectConfig()
			newProjectB := CreatePlaceholderProjectConfig()
			So(len(newProjectB.GcsAllowList), ShouldEqual, 1)
			newProjectB.GcsAllowList[0].Users = []string{"user:newb@test.com"}
			delete(configs, "projects/a")
			configs["projects/b"]["${appid}.cfg"] = textPBMultiline.Format(newProjectB)
			configs["projects/c"] = cfgmem.Files{
				"${appid}.cfg": textPBMultiline.Format(projectC),
			}
			err = UpdateProjects(ctx)
			So(err, ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Fetch returns the new value right away.
			projects, err = fetchProjects(ctx)
			So(err, ShouldBeNil)
			So(len(projects), ShouldEqual, 2)
			So(projects["b"], ShouldResembleProto, newProjectB)
			So(projects["c"], ShouldResembleProto, projectC)

			// Get still uses in-memory cached copy.
			projects, err = Projects(ctx)
			So(err, ShouldBeNil)
			So(len(projects), ShouldEqual, 2)
			So(projects["a"], ShouldResembleProto, projectA)
			So(projects["b"], ShouldResembleProto, projectB)

			// Time passes, in-memory cached copy expires.
			tc.Add(2 * time.Minute)

			// Get returns the new value now too.
			projects, err = Projects(ctx)
			So(err, ShouldBeNil)
			So(len(projects), ShouldEqual, 2)
			So(projects["b"], ShouldResembleProto, newProjectB)
			So(projects["c"], ShouldResembleProto, projectC)

			// Time passes, in-memory cached copy expires.
			tc.Add(2 * time.Minute)

			// Get returns the same value.
			projects, err = Projects(ctx)
			So(err, ShouldBeNil)
			So(len(projects), ShouldEqual, 2)
			So(projects["b"], ShouldResembleProto, newProjectB)
			So(projects["c"], ShouldResembleProto, projectC)
		})

		Convey("Validation works", func() {
			configs["projects/b"]["${appid}.cfg"] = `bad data`
			err := UpdateProjects(ctx)
			datastore.GetTestable(ctx).CatchupIndexes()
			So(err, ShouldErrLike, "validation errors")

			// Validation for project A passed and project is
			// available, validation for project B failed
			// as is not available.
			projects, err := Projects(ctx)
			So(err, ShouldBeNil)
			So(len(projects), ShouldEqual, 1)
			So(projects["a"], ShouldResembleProto, projectA)
		})

		Convey("Update retains existing config if new config is invalid", func() {
			// Initial update.
			err := UpdateProjects(ctx)
			So(err, ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Get works.
			projects, err := Projects(ctx)
			So(err, ShouldBeNil)
			So(len(projects), ShouldEqual, 2)
			So(projects["a"], ShouldResembleProto, projectA)
			So(projects["b"], ShouldResembleProto, projectB)

			// Attempt to update with an invalid config for project B.
			newProjectA := CreatePlaceholderProjectConfig()
			So(len(newProjectA.GcsAllowList), ShouldEqual, 1)
			newProjectA.GcsAllowList[0].Users = []string{"user:newa@test.com"}
			newProjectB := CreatePlaceholderProjectConfig()
			So(len(newProjectB.GcsAllowList), ShouldEqual, 1)
			newProjectB.GcsAllowList[0].Users = []string{""}
			configs["projects/a"]["${appid}.cfg"] = textPBMultiline.Format(newProjectA)
			configs["projects/b"]["${appid}.cfg"] = textPBMultiline.Format(newProjectB)
			err = UpdateProjects(ctx)
			So(err, ShouldErrLike, "validation errors")
			datastore.GetTestable(ctx).CatchupIndexes()

			// Time passes, in-memory cached copy expires.
			tc.Add(2 * time.Minute)

			// Get returns the new configuration A and the old
			// configuration for B. This ensures an attempt to push an invalid
			// config does not result in a service outage for that project.
			projects, err = Projects(ctx)
			So(err, ShouldBeNil)
			So(len(projects), ShouldEqual, 2)
			So(projects["a"], ShouldResembleProto, newProjectA)
			So(projects["b"], ShouldResembleProto, projectB)
		})
	})
}

func TestProject(t *testing.T) {
	t.Parallel()

	Convey("Project", t, func() {
		pjChromium := CreatePlaceholderProjectConfig()
		configs := map[string]*configpb.ProjectConfig{
			"chromium": pjChromium,
		}

		ctx := memory.Use(context.Background())
		So(SetTestProjectConfig(ctx, configs), ShouldBeNil)

		Convey("success", func() {
			pj, err := Project(ctx, "chromium")
			So(err, ShouldBeNil)
			So(pj, ShouldResembleProto, pjChromium)
		})

		Convey("not found", func() {
			pj, err := Project(ctx, "random")
			So(err, ShouldErrLike, ErrNotFoundProjectConfig)
			So(pj, ShouldBeNil)
		})
	})
}
