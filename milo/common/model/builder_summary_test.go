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

package model

import (
	"fmt"
	"testing"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/caching"

	. "github.com/smartystreets/goconvey/convey"
)

func TestUpdateBuilder(t *testing.T) {
	t.Parallel()

	Convey(`TestUpdateBuilder`, t, func() {
		c := gaetesting.TestingContextWithAppID("luci-milo-dev")

		builder := &BuilderSummary{BuilderID: "fake"}

		// Populate a few BuildSummaries. For convenience, ordered by creation time.
		builds := make([]*BuildSummary, 10)
		for i := 0; i < 10; i++ {
			builds[i] = &BuildSummary{
				BuildKey:  datastore.MakeKey(c, "fakeBuild", i),
				BuilderID: builder.BuilderID,
				BuildID:   fmt.Sprintf("build_id/%d", i),
				Created:   testclock.TestRecentTimeUTC.Add(time.Duration(i) * time.Hour),
			}
		}

		c = caching.WithRequestCache(c)

		updateBuilder := func(build *BuildSummary) {
			err := datastore.RunInTransaction(c, func(c context.Context) error {
				return UpdateBuilderForBuild(c, build)
			}, nil)
			So(err, ShouldBeNil)
			err = datastore.Get(c, builder)
			So(err, ShouldBeNil)
		}

		Convey("Updating appropriate builder having existing last finished build", func() {
			builder.LastFinishedCreated = builds[5].Created
			builder.LastFinishedStatus = Success
			builder.LastFinishedBuildID = builds[5].BuildID
			err := datastore.Put(c, builder)
			So(err, ShouldBeNil)

			Convey("with finished build should not update last finished build info", func() {
				builds[6].Summary.Status = Failure
				updateBuilder(builds[6])
				So(builder.LastFinishedStatus, ShouldEqual, Failure)
				So(builder.LastFinishedBuildID, ShouldEqual, builds[6].BuildID)
			})

			Convey("for build created earlier than last finished", func() {
				builds[4].Summary.Status = Failure
				updateBuilder(builds[4])
				So(builder.LastFinishedStatus, ShouldEqual, Success)
				So(builder.LastFinishedBuildID, ShouldEqual, builds[5].BuildID)
			})

			Convey("for build created later than last finished", func() {
				builds[6].Summary.Status = NotRun
				updateBuilder(builds[6])
				So(builder.LastFinishedStatus, ShouldEqual, Success)
				So(builder.LastFinishedBuildID, ShouldEqual, builds[5].BuildID)
			})
		})

		Convey("Updating appropriate builder with no last finished build should initialize it", func() {
			builds[5].Summary.Status = Failure
			updateBuilder(builds[5])
			So(builder.LastFinishedStatus, ShouldEqual, Failure)
			So(builder.LastFinishedBuildID, ShouldEqual, builds[5].BuildID)
		})
	})
}
