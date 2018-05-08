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

package buildbot

import (
	"testing"

	"golang.org/x/net/context"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/milo/api/buildbot"
	"go.chromium.org/luci/milo/buildsource/buildbot/buildstore"
	"go.chromium.org/luci/server/caching"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBuilder(t *testing.T) {
	t.Parallel()

	Convey(`TestBuilder`, t, func() {
		c := memory.UseWithAppID(context.Background(), "dev~luci-milo")
		c, _ = testclock.UseTime(c, testclock.TestTimeUTC)
		c = caching.WithRequestCache(c)
		fakeTime := unixTime(123)

		// Seed a builder with 10 builds.
		for i := 0; i < 10; i++ {
			importBuild(c, &buildbot.Build{
				Master:      "fake",
				Buildername: "fake",
				Number:      i,
				Internal:    false,
				Times:       buildbot.MkTimeRange(fakeTime, fakeTime),
				Sourcestamp: &buildbot.SourceStamp{},
				Results:     buildbot.Success,
				Finished:    true,
			})
		}
		// Failed build
		importBuild(c, &buildbot.Build{
			Master:      "fake",
			Buildername: "fake",
			Number:      10,
			Internal:    false,
			Times:       buildbot.MkTimeRange(fakeTime, fakeTime),
			Sourcestamp: &buildbot.SourceStamp{},
			Results:     buildbot.Failure,
			Finished:    true,
			Text:        []string{"failed", "stuff"},
		})
		buildstore.SaveMaster(c, &buildbot.Master{
			Name:     "fake",
			Builders: map[string]*buildbot.Builder{"fake": {}},
		}, false, nil)
		datastore.GetTestable(c).Consistent(true)
		datastore.GetTestable(c).AutoIndex(true)
		datastore.GetTestable(c).CatchupIndexes()

		Convey(`Invalid builder`, func() {
			_, err := GetBuilder(c, "fake", "not real builder", 2, "")
			So(err.Error(), ShouldResemble, "Cannot find builder \"not real builder\" in master \"fake\".\nAvailable builders: \nfake")
		})
		Convey(`Basic 3 build builder`, func() {
			Convey(`Fetch 2`, func() {
				response, err := GetBuilder(c, "fake", "fake", 2, "")
				So(err, ShouldBeNil)
				So(len(response.FinishedBuilds), ShouldEqual, 2)
				So(response.NextCursor, ShouldEqual, "-9") // numbers < 9
				So(response.PrevCursor, ShouldEqual, "")   // no prev cursor for a non-cursor query
				So(response.FinishedBuilds[0].Link.Label, ShouldEqual, "#10")
				So(response.FinishedBuilds[0].Text, ShouldResemble, []string{"failed stuff"})

				Convey(`Fetch another 2`, func() {
					response2, err := GetBuilder(c, "fake", "fake", 2, response.NextCursor)
					So(err, ShouldBeNil)
					So(len(response2.FinishedBuilds), ShouldEqual, 2)
					So(response2.PrevCursor, ShouldEqual, "9")  // numbers >= 9
					So(response2.NextCursor, ShouldEqual, "-7") // numbers < 7

					Convey(`Fetch another 2`, func() {
						response3, err := GetBuilder(c, "fake", "fake", 2, response2.NextCursor)
						So(err, ShouldBeNil)
						So(len(response3.FinishedBuilds), ShouldEqual, 2)
						So(response3.PrevCursor, ShouldEqual, "7")  // numbers >= 7
						So(response3.NextCursor, ShouldEqual, "-5") // numbers < 5

						Convey(`Fetch the rest`, func() {
							response4, err := GetBuilder(c, "fake", "fake", 20, response3.NextCursor)
							So(err, ShouldBeNil)
							So(len(response4.FinishedBuilds), ShouldEqual, 5)
							So(response4.PrevCursor, ShouldEqual, "5") // numbers >= 5
							So(response4.NextCursor, ShouldEqual, "")  // EOF
						})
					})
				})
			})
		})
	})
}
