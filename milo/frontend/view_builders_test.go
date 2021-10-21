// Copyright 2017 The LUCI Authors.
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

package frontend

import (
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/milo/common/model/milostatus"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetBuilderHistories(t *testing.T) {
	t.Parallel()

	Convey(`TestGetBuilderHistories`, t, func() {
		c := gaetesting.TestingContextWithAppID("luci-milo-dev")
		c = caching.WithRequestCache(c)

		datastore.GetTestable(c).AddIndexes(&datastore.IndexDefinition{
			Kind: "BuildSummary",
			SortBy: []datastore.IndexColumn{
				{Property: "BuilderID"},
				{Property: "Created", Descending: true},
			},
		})
		datastore.GetTestable(c).AddIndexes(&datastore.IndexDefinition{
			Kind: "BuildSummary",
			SortBy: []datastore.IndexColumn{
				{Property: "BuilderID"},
				{Property: "Summary.Status"},
			},
		})
		datastore.GetTestable(c).CatchupIndexes()
		datastore.GetTestable(c).Consistent(true)

		p := "proj"
		proj := datastore.MakeKey(c, "Project", p)

		// Populate consoles.
		err := datastore.Put(c, &common.Console{
			Parent: proj,
			ID:     "console",
			Builders: []string{
				"buildbucket/bucket/b1",
				"buildbucket/bucket/b2",
				"buildbucket/bucket/b4",
				"buildbucket/bucket/b5",
			},
		})
		So(err, ShouldBeNil)
		err = datastore.Put(c, &common.Console{
			Parent:   proj,
			ID:       "console2",
			Builders: []string{"buildbucket/bucket/b4"},
		})
		So(err, ShouldBeNil)
		err = datastore.Put(c, &model.BuilderSummary{
			BuilderID: "buildbucket/bucket/b5",
			ProjectID: "private",
		})
		So(err, ShouldBeNil)

		// Populate builds.
		var builds []*model.BuildSummary
		addBuilds := func(builder, project string, statuses ...milostatus.Status) {
			buildPrefix := "buildbucket/bucket/"
			for i, status := range statuses {
				buildID := fmt.Sprintf("%s%s/%d", buildPrefix, builder, i)
				builds = append(builds, &model.BuildSummary{
					BuildKey:  datastore.MakeKey(c, "build", buildID),
					ProjectID: project,
					BuilderID: buildPrefix + builder,
					BuildID:   buildID,
					Created:   testclock.TestRecentTimeUTC.Add(time.Duration(len(builds)) * time.Hour),
					Summary:   model.Summary{Status: status},
				})
			}
		}

		// One builder has lots of builds.
		addBuilds("b2", p,
			milostatus.Running,
			milostatus.Success,
			milostatus.Running,
			milostatus.Exception,
			milostatus.Running,
			milostatus.InfraFailure)
		// One builder is not on any project's consoles.
		addBuilds("b3", p, milostatus.Success)
		// One builder is on two consoles.
		addBuilds("b4", p, milostatus.Success)
		// One builder is on the console of a different project.
		addBuilds("b5", "private", milostatus.Success)
		err = datastore.Put(c, builds)
		So(err, ShouldBeNil)

		Convey("Getting recent history for existing project", func() {
			Convey("across all consoles", func() {
				Convey("with limit less than number of finished builds works", func() {
					builders, err := getBuildersForProject(c, p, "")
					So(err, ShouldBeNil)
					hists, err := getBuilderHistories(c, builders, p, 2)
					So(err, ShouldBeNil)
					So(hists, ShouldHaveLength, 4)

					So(hists[0], ShouldResemble, &builderHistory{
						BuilderID:    "buildbucket/bucket/b1",
						BuilderLink:  "/p/proj/builders/bucket/b1",
						RecentBuilds: []*model.BuildSummary{},
					})

					b2Hist := hists[1]
					So(b2Hist.BuilderID, ShouldEqual, "buildbucket/bucket/b2")
					So(b2Hist.BuilderLink, ShouldEqual, "/p/proj/builders/bucket/b2")
					So(b2Hist.NumRunning, ShouldEqual, 3)
					So(b2Hist.RecentBuilds, ShouldHaveLength, 2)
					So(b2Hist.RecentBuilds[0].BuildID, ShouldEqual, "buildbucket/bucket/b2/5")
					So(b2Hist.RecentBuilds[1].BuildID, ShouldEqual, "buildbucket/bucket/b2/3")

					b4Hist := hists[2]
					So(b4Hist.BuilderID, ShouldEqual, "buildbucket/bucket/b4")
					So(b4Hist.BuilderLink, ShouldEqual, "/p/proj/builders/bucket/b4")
					So(b4Hist.NumRunning, ShouldEqual, 0)
					So(b4Hist.RecentBuilds, ShouldHaveLength, 1)
					So(b4Hist.RecentBuilds[0].BuildID, ShouldEqual, "buildbucket/bucket/b4/0")

					b5Hist := hists[3]
					So(b5Hist.BuilderID, ShouldEqual, "buildbucket/bucket/b5")
					So(b5Hist.BuilderLink, ShouldEqual, "/p/private/builders/bucket/b5")
					So(b5Hist.NumRunning, ShouldEqual, 0)
					So(b5Hist.RecentBuilds, ShouldHaveLength, 1)
					So(b5Hist.RecentBuilds[0].BuildID, ShouldEqual, "buildbucket/bucket/b5/0")
				})

				Convey("with limit greater than number of finished builds works", func() {
					builders, err := getBuildersForProject(c, p, "")
					So(err, ShouldBeNil)
					hists, err := getBuilderHistories(c, builders, p, 5)
					So(err, ShouldBeNil)
					So(hists, ShouldHaveLength, 4)

					So(hists[0], ShouldResemble, &builderHistory{
						BuilderID:    "buildbucket/bucket/b1",
						BuilderLink:  "/p/proj/builders/bucket/b1",
						RecentBuilds: []*model.BuildSummary{},
					})

					b2Hist := hists[1]
					So(b2Hist.BuilderID, ShouldEqual, "buildbucket/bucket/b2")
					So(b2Hist.BuilderLink, ShouldEqual, "/p/proj/builders/bucket/b2")
					So(b2Hist.NumRunning, ShouldEqual, 3)
					So(b2Hist.RecentBuilds, ShouldHaveLength, 3)
					So(b2Hist.RecentBuilds[0].BuildID, ShouldEqual, "buildbucket/bucket/b2/5")
					So(b2Hist.RecentBuilds[1].BuildID, ShouldEqual, "buildbucket/bucket/b2/3")
					So(b2Hist.RecentBuilds[2].BuildID, ShouldEqual, "buildbucket/bucket/b2/1")

					b4Hist := hists[2]
					So(b4Hist.BuilderID, ShouldEqual, "buildbucket/bucket/b4")
					So(b4Hist.BuilderLink, ShouldEqual, "/p/proj/builders/bucket/b4")
					So(b4Hist.NumRunning, ShouldEqual, 0)
					So(b4Hist.RecentBuilds, ShouldHaveLength, 1)
					So(b4Hist.RecentBuilds[0].BuildID, ShouldEqual, "buildbucket/bucket/b4/0")
				})
			})

			Convey("across a specific console", func() {
				Convey("for a valid console works", func() {
					builders, err := getBuildersForProject(c, p, "console2")
					So(err, ShouldBeNil)
					hists, err := getBuilderHistories(c, builders, p, 2)
					So(err, ShouldBeNil)
					So(hists, ShouldHaveLength, 1)

					So(hists[0].BuilderID, ShouldEqual, "buildbucket/bucket/b4")
					So(hists[0].BuilderLink, ShouldEqual, "/p/proj/builders/bucket/b4")
					So(hists[0].RecentBuilds, ShouldHaveLength, 1)
					So(hists[0].RecentBuilds[0].BuildID, ShouldResemble, "buildbucket/bucket/b4/0")
				})

				Convey("for an invalid console works", func() {
					_, err := getBuildersForProject(c, p, "bad_console")
					So(err, ShouldNotBeNil)
				})
			})
		})

		Convey("Getting recent history for nonexisting project", func() {
			builders, err := getBuildersForProject(c, "no_proj", "")
			So(err, ShouldBeNil)
			So(builders, ShouldHaveLength, 0)
			hists, err := getBuilderHistories(c, builders, "no_proj", 3)
			So(err, ShouldBeNil)
			So(hists, ShouldHaveLength, 0)
		})
	})
}
