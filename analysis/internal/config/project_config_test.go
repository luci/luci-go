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
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/caching"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	configpb "go.chromium.org/luci/analysis/proto/config"
)

var textPBMultiline = prototext.MarshalOptions{
	Multiline: true,
}

func TestProjectConfig(t *testing.T) {
	t.Parallel()

	Convey("SetTestProjectConfig updates context config", t, func() {
		projectA := CreateConfigWithBothBuganizerAndMonorail(configpb.BugSystem_MONORAIL)
		projectA.LastUpdated = timestamppb.New(time.Now())
		configs := make(map[string]*configpb.ProjectConfig)
		configs["a"] = projectA

		ctx := memory.Use(context.Background())
		SetTestProjectConfig(ctx, configs)

		cfg, err := Projects(ctx)

		So(err, ShouldBeNil)
		So(len(cfg.Keys()), ShouldEqual, 1)
		So(cfg.Project("a"), ShouldResembleProto, projectA)
	})

	Convey("With mocks", t, WithBothBugSystems(func(system configpb.BugSystem, name string) {
		projectA := CreateConfigWithBothBuganizerAndMonorail(system)
		projectB := CreateConfigWithBothBuganizerAndMonorail(system)
		projectB.BugManagement.Monorail.PriorityFieldId = 1

		configs := map[config.Set]cfgmem.Files{
			"projects/a": {"${appid}.cfg": textPBMultiline.Format(projectA)},
			"projects/b": {"${appid}.cfg": textPBMultiline.Format(projectB)},
		}

		ctx := memory.Use(context.Background())
		ctx, tc := testclock.UseTime(ctx, testclock.TestTimeUTC)
		ctx = cfgclient.Use(ctx, cfgmem.New(configs))
		ctx = caching.WithEmptyProcessCache(ctx)

		Convey(fmt.Sprintf("%s - Update works", name), func() {
			// Initial update.
			creationTime := clock.Now(ctx)
			err := updateProjects(ctx)
			So(err, ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Get works.
			projects, err := Projects(ctx)
			So(err, ShouldBeNil)
			So(len(projects.Keys()), ShouldEqual, 2)
			So(projects.Project("a"), ShouldResembleProto, withLastUpdated(projectA, creationTime))
			So(projects.Project("b"), ShouldResembleProto, withLastUpdated(projectB, creationTime))

			tc.Add(1 * time.Second)

			// Noop update.
			err = updateProjects(ctx)
			So(err, ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			tc.Add(1 * time.Second)

			// Real update.
			projectC := CreateConfigWithBothBuganizerAndMonorail(system)
			newProjectB := CreateConfigWithBothBuganizerAndMonorail(system)
			newProjectB.BugManagement.Monorail.PriorityFieldId = 2
			delete(configs, "projects/a")
			configs["projects/b"]["${appid}.cfg"] = textPBMultiline.Format(newProjectB)
			configs["projects/c"] = cfgmem.Files{
				"${appid}.cfg": textPBMultiline.Format(projectC),
			}
			newProjectA := &configpb.ProjectConfig{}
			updateTime := clock.Now(ctx)
			err = updateProjects(ctx)
			So(err, ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Fetch returns the new value right away.
			projects, err = fetchProjects(ctx)
			So(err, ShouldBeNil)
			So(len(projects.Keys()), ShouldEqual, 3)
			So(projects.Project("a"), ShouldResembleProto, withLastUpdated(newProjectA, updateTime)) // Retained.
			So(projects.Project("b"), ShouldResembleProto, withLastUpdated(newProjectB, updateTime))
			So(projects.Project("c"), ShouldResembleProto, withLastUpdated(projectC, updateTime))

			// Get still uses in-memory cached copy.
			projects, err = Projects(ctx)
			So(err, ShouldBeNil)
			So(len(projects.Keys()), ShouldEqual, 2)
			So(projects.Project("a"), ShouldResembleProto, withLastUpdated(projectA, creationTime)) // Retained.
			So(projects.Project("b"), ShouldResembleProto, withLastUpdated(projectB, creationTime))

			// Noop update again.
			err = updateProjects(ctx)
			So(err, ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// No change for all projects.
			projects, err = fetchProjects(ctx)
			So(err, ShouldBeNil)
			So(len(projects.Keys()), ShouldEqual, 3)
			So(projects.Project("a"), ShouldResembleProto, withLastUpdated(newProjectA, updateTime)) // Retained.
			So(projects.Project("b"), ShouldResembleProto, withLastUpdated(newProjectB, updateTime))
			So(projects.Project("c"), ShouldResembleProto, withLastUpdated(projectC, updateTime))

			Convey("Expedited cache eviction", func() {
				projectB, err = ProjectWithMinimumVersion(ctx, "b", updateTime)
				So(err, ShouldBeNil)
				So(projectB, ShouldResembleProto, withLastUpdated(newProjectB, updateTime))
			})
			Convey("Natural cache eviction", func() {
				// Time passes, in-memory cached copy expires.
				tc.Add(2 * time.Minute)

				// Get returns the new value now too.
				projects, err = Projects(ctx)
				So(err, ShouldBeNil)
				So(len(projects.Keys()), ShouldEqual, 3)
				So(projects.Project("a"), ShouldResembleProto, withLastUpdated(newProjectA, updateTime)) // Retained.
				So(projects.Project("b"), ShouldResembleProto, withLastUpdated(newProjectB, updateTime))
				So(projects.Project("c"), ShouldResembleProto, withLastUpdated(projectC, updateTime))

				// Time passes, in-memory cached copy expires.
				tc.Add(2 * time.Minute)

				// Get returns the same value.
				projects, err = Projects(ctx)
				So(err, ShouldBeNil)
				So(len(projects.Keys()), ShouldEqual, 3)
				So(projects.Project("a"), ShouldResembleProto, withLastUpdated(newProjectA, updateTime))
				So(projects.Project("b"), ShouldResembleProto, withLastUpdated(newProjectB, updateTime))
				So(projects.Project("c"), ShouldResembleProto, withLastUpdated(projectC, updateTime))
			})

			Convey("Add config for previously deleted project", func() {
				tc.Add(2 * time.Second)
				configs["projects/a"] = cfgmem.Files{
					"${appid}.cfg": textPBMultiline.Format(projectA),
				}
				newUpdateTime := clock.Now(ctx)
				err = updateProjects(ctx)
				So(err, ShouldBeNil)
				datastore.GetTestable(ctx).CatchupIndexes()

				// Fetch returns the new value right away.
				projects, err = fetchProjects(ctx)
				So(err, ShouldBeNil)
				So(len(projects.Keys()), ShouldEqual, 3)
				So(projects.Project("a"), ShouldResembleProto, withLastUpdated(projectA, newUpdateTime))
				So(projects.Project("b"), ShouldResembleProto, withLastUpdated(newProjectB, updateTime))
				So(projects.Project("c"), ShouldResembleProto, withLastUpdated(projectC, updateTime))

			})
		})

		Convey(fmt.Sprintf("%s - Validation works", name), func() {
			configs["projects/b"]["${appid}.cfg"] = `bad data`
			creationTime := clock.Now(ctx)
			err := updateProjects(ctx)
			datastore.GetTestable(ctx).CatchupIndexes()
			So(err, ShouldErrLike, "validation errors")

			// Validation for project A passed and project is
			// available, validation for project B failed
			// as is not available.
			projects, err := Projects(ctx)
			So(err, ShouldBeNil)
			So(len(projects.Keys()), ShouldEqual, 1)
			So(projects.Project("a"), ShouldResembleProto, withLastUpdated(projectA, creationTime))
		})

		Convey(fmt.Sprintf("%s - Update retains existing config if new config is invalid", name), func() {
			// Initial update.
			creationTime := clock.Now(ctx)
			err := updateProjects(ctx)
			So(err, ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Get works.
			projects, err := Projects(ctx)
			So(err, ShouldBeNil)
			So(len(projects.Keys()), ShouldEqual, 2)
			So(projects.Project("a"), ShouldResembleProto, withLastUpdated(projectA, creationTime))
			So(projects.Project("b"), ShouldResembleProto, withLastUpdated(projectB, creationTime))

			tc.Add(1 * time.Second)

			// Attempt to update with an invalid config for project B.
			newProjectA := CreateConfigWithBothBuganizerAndMonorail(system)
			newProjectA.BugManagement.Monorail.Project = "new-project-a"
			newProjectB := CreateConfigWithBothBuganizerAndMonorail(system)
			newProjectB.BugManagement.Monorail.Project = ""
			configs["projects/a"]["${appid}.cfg"] = textPBMultiline.Format(newProjectA)
			configs["projects/b"]["${appid}.cfg"] = textPBMultiline.Format(newProjectB)
			updateTime := clock.Now(ctx)
			err = updateProjects(ctx)
			So(err, ShouldErrLike, "validation errors")
			datastore.GetTestable(ctx).CatchupIndexes()

			// Time passes, in-memory cached copy expires.
			tc.Add(2 * time.Minute)

			// Get returns the new configuration A and the old
			// configuration for B. This ensures an attempt to push an invalid
			// config does not result in a service outage for that project.
			projects, err = Projects(ctx)
			So(err, ShouldBeNil)
			So(len(projects.Keys()), ShouldEqual, 2)
			So(projects.Project("a"), ShouldResembleProto, withLastUpdated(newProjectA, updateTime))
			So(projects.Project("b"), ShouldResembleProto, withLastUpdated(projectB, creationTime))
		})
	}))
}

// withLastUpdated returns a copy of the given ProjectConfig with the
// specified LastUpdated time set.
func withLastUpdated(cfg *configpb.ProjectConfig, lastUpdated time.Time) *configpb.ProjectConfig {
	result := proto.Clone(cfg).(*configpb.ProjectConfig)
	result.LastUpdated = timestamppb.New(lastUpdated)
	return result
}

func TestProject(t *testing.T) {
	t.Parallel()

	Convey("Project", t, WithBothBugSystems(func(system configpb.BugSystem, name string) {
		pjChromium := CreateConfigWithBothBuganizerAndMonorail(system)
		configs := map[string]*configpb.ProjectConfig{
			"chromium": pjChromium,
		}

		ctx := memory.Use(context.Background())
		SetTestProjectConfig(ctx, configs)

		Convey(fmt.Sprintf("%s - success", name), func() {
			pj, err := Project(ctx, "chromium")
			So(err, ShouldBeNil)
			So(pj, ShouldResembleProto, pjChromium)
		})

		Convey(fmt.Sprintf("%s - not found", name), func() {
			pj, err := Project(ctx, "random")
			So(err, ShouldBeNil)
			So(pj, ShouldResembleProto, &configpb.ProjectConfig{LastUpdated: timestamppb.New(StartingEpoch)})
		})
	}))
}

func TestRealm(t *testing.T) {
	t.Parallel()

	Convey("Realm", t, WithBothBugSystems(func(system configpb.BugSystem, name string) {
		pj := CreateConfigWithBothBuganizerAndMonorail(system)
		configs := map[string]*configpb.ProjectConfig{
			"chromium": pj,
		}

		ctx := memory.Use(context.Background())
		SetTestProjectConfig(ctx, configs)

		Convey(fmt.Sprintf("%s - success", name), func() {
			rj, err := Realm(ctx, "chromium:ci")
			So(err, ShouldBeNil)
			So(rj, ShouldResembleProto, pj.Realms[0])
		})

		Convey(fmt.Sprintf("%s - not found", name), func() {
			rj, err := Realm(ctx, "chromium:random")
			So(err, ShouldEqual, RealmNotExistsErr)
			So(rj, ShouldBeNil)
		})
	}))
}
