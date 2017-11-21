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

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"

	. "github.com/smartystreets/goconvey/convey"
)

// addBuildSummary populates the given BuildSummary array at specified index, returning the next
// index.
func addBuildSummary(c context.Context, builds *[]*model.BuildSummary, project, builder string, summary model.Summary) {
	i := len(*builds)
	*builds = append(*builds, &model.BuildSummary{
		BuildKey:  datastore.MakeKey(c, "build", i+1),
		ProjectID: project,
		BuilderID: builder,
		BuildID:   fmt.Sprintf("%s/%d", builder, i),
		Created:   testclock.TestRecentTimeUTC.Add(time.Duration(i) * time.Hour),
		Summary:   summary,
	})
}

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

		nBuilds := 10
		statuses := []model.Summary{
			{Status: model.Running},      /* b2 */
			{Status: model.Success},      /* b2 */
			{Status: model.Running},      /* b2 */
			{Status: model.Exception},    /* b2 */
			{Status: model.Running},      /* b2 */
			{Status: model.InfraFailure}, /* b2 */
			{Status: model.NotRun},       /* b2 */
			{Status: model.NotRun},       /* b2 */
			{Status: model.Success},      /* b3 */
			{Status: model.Success},      /* b4 */
		}
		So(statuses, ShouldHaveLength, nBuilds)

		p := "proj"
		proj := datastore.MakeKey(c, "Project", p)

		// Populate consoles.
		err := datastore.Put(c, &common.Console{
			Parent:   proj,
			ID:       "console",
			Builders: []string{"buildbot/master/b1", "buildbot/master/b2", "buildbot/master/b4"},
		})
		So(err, ShouldBeNil)
		err = datastore.Put(c, &common.Console{
			Parent:   proj,
			ID:       "console2",
			Builders: []string{"buildbot/master/b4"},
		})
		So(err, ShouldBeNil)

		// Populate builds.
		builds := make([]*model.BuildSummary, 0, nBuilds)

		// One builder is on a console but has no builds.

		// One builder has lots of builds.
		builder := "buildbot/master/b2"
		for i := 0; i < 8; i++ {
			addBuildSummary(c, &builds, p, builder, statuses[i])
		}

		// One builder is not on any project's consoles.
		addBuildSummary(c, &builds, p, "buildbot/master/b3", statuses[8])

		// One builder is on two consoles.
		addBuildSummary(c, &builds, p, "buildbot/master/b4", statuses[9])

		err = datastore.Put(c, builds)
		So(err, ShouldBeNil)

		Convey("Getting recent history for existing project", func() {
			Convey("across all consoles", func() {
				Convey("with limit less than number of finished builds works", func() {
					hists, err := getBuilderHistories(c, p, "", 2)
					So(err, ShouldBeNil)
					So(hists, ShouldHaveLength, 3)

					So(*hists[0], ShouldResemble, builderHistory{
						BuilderID:    "buildbot/master/b1",
						BuilderLink:  "/buildbot/master/b1",
						RecentBuilds: []*model.BuildSummary{},
					})

					So(hists[1].BuilderID, ShouldEqual, "buildbot/master/b2")
					So(hists[1].BuilderLink, ShouldEqual, "/buildbot/master/b2")
					So(hists[1].NumPending, ShouldEqual, 2)
					So(hists[1].NumRunning, ShouldEqual, 3)
					So(hists[1].RecentBuilds, ShouldHaveLength, 2)
					So(hists[1].RecentBuilds[0].BuildID, ShouldEqual, builds[5].BuildID)
					So(hists[1].RecentBuilds[1].BuildID, ShouldEqual, builds[3].BuildID)

					So(hists[2].BuilderID, ShouldEqual, "buildbot/master/b4")
					So(hists[2].BuilderLink, ShouldEqual, "/buildbot/master/b4")
					So(hists[2].NumPending, ShouldEqual, 0)
					So(hists[2].NumRunning, ShouldEqual, 0)
					So(hists[2].RecentBuilds, ShouldHaveLength, 1)
					So(hists[2].RecentBuilds[0].BuildID, ShouldEqual, builds[9].BuildID)
				})

				Convey("with limit greater than number of finished builds works", func() {
					hists, err := getBuilderHistories(c, p, "", 5)
					So(err, ShouldBeNil)
					So(hists, ShouldHaveLength, 3)

					So(*hists[0], ShouldResemble, builderHistory{
						BuilderID:    "buildbot/master/b1",
						BuilderLink:  "/buildbot/master/b1",
						RecentBuilds: []*model.BuildSummary{},
					})

					So(hists[1].BuilderID, ShouldEqual, "buildbot/master/b2")
					So(hists[1].BuilderLink, ShouldEqual, "/buildbot/master/b2")
					So(hists[1].NumPending, ShouldEqual, 2)
					So(hists[1].NumRunning, ShouldEqual, 3)
					So(hists[1].RecentBuilds, ShouldHaveLength, 3)
					So(hists[1].RecentBuilds[0].BuildID, ShouldEqual, builds[5].BuildID)
					So(hists[1].RecentBuilds[1].BuildID, ShouldEqual, builds[3].BuildID)
					So(hists[1].RecentBuilds[2].BuildID, ShouldEqual, builds[1].BuildID)

					So(hists[2].BuilderID, ShouldEqual, "buildbot/master/b4")
					So(hists[2].BuilderLink, ShouldEqual, "/buildbot/master/b4")
					So(hists[2].NumPending, ShouldEqual, 0)
					So(hists[2].NumRunning, ShouldEqual, 0)
					So(hists[2].RecentBuilds, ShouldHaveLength, 1)
					So(hists[2].RecentBuilds[0].BuildID, ShouldEqual, builds[9].BuildID)
				})
			})

			Convey("across a specific console", func() {
				Convey("for a valid console works", func() {
					hists, err := getBuilderHistories(c, p, "console2", 2)
					So(err, ShouldBeNil)
					So(hists, ShouldHaveLength, 1)

					So(hists[0].BuilderID, ShouldEqual, "buildbot/master/b4")
					So(hists[0].BuilderLink, ShouldEqual, "/buildbot/master/b4")
					So(hists[0].RecentBuilds, ShouldHaveLength, 1)
					So(hists[0].RecentBuilds[0].BuildID, ShouldResemble, builds[9].BuildID)
				})

				Convey("for an invalid console works", func() {
					_, err := getBuilderHistories(c, p, "bad_console", 2)
					So(err, ShouldNotBeNil)
				})
			})
		})

		Convey("Getting recent history for nonexisting project", func() {
			hists, err := getBuilderHistories(c, "no_proj", "", 3)
			So(err, ShouldBeNil)
			So(hists, ShouldHaveLength, 0)
		})
	})
}
