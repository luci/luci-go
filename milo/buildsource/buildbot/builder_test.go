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
	"context"
	"testing"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBuilder(t *testing.T) {
	c := memory.UseWithAppID(context.Background(), "dev~luci-milo")
	c, _ = testclock.UseTime(c, testclock.TestTimeUTC)
	fakeTime := float64(123)
	fakeResult := int(0)
	fakeFailure := int(2)

	// Seed a builder with 10 builds.
	for i := 1; i < 10; i++ {
		datastore.Put(c, &buildbotBuild{
			Master:      "fake",
			Buildername: "fake",
			Number:      i,
			Internal:    false,
			Times:       []*float64{&fakeTime, &fakeTime},
			Sourcestamp: &buildbotSourceStamp{},
			Results:     &fakeResult,
			Finished:    true,
		})
	}
	// Failed build
	datastore.Put(c, &buildbotBuild{
		Master:      "fake",
		Buildername: "fake",
		Number:      10,
		Internal:    false,
		Times:       []*float64{&fakeTime, &fakeTime},
		Sourcestamp: &buildbotSourceStamp{},
		Results:     &fakeFailure,
		Finished:    true,
		Text:        []string{"failed", "stuff"},
	})
	putDSMasterJSON(c, &buildbotMaster{
		Name:     "fake",
		Builders: map[string]*buildbotBuilder{"fake": {}},
	}, false)
	datastore.GetTestable(c).Consistent(true)
	datastore.GetTestable(c).AutoIndex(true)
	datastore.GetTestable(c).CatchupIndexes()
	Convey(`A test Environment`, t, func() {

		Convey(`Invalid builder`, func() {
			_, err := GetBuilder(c, "fake", "not real builder", 2, nil)
			So(err.Error(), ShouldResemble, "Cannot find builder \"not real builder\" in master \"fake\".\nAvailable builders: \nfake")
		})
		Convey(`Basic 3 build builder`, func() {
			Convey(`Fetch 2`, func() {
				response, err := GetBuilder(c, "fake", "fake", 2, nil)
				So(err, ShouldBeNil)
				So(len(response.FinishedBuilds), ShouldEqual, 2)
				So(response.NextCursor, ShouldNotEqual, "")
				So(response.PrevCursor, ShouldEqual, "")
				So(response.FinishedBuilds[0].Link.Label, ShouldEqual, "#10")
				So(response.FinishedBuilds[0].Text, ShouldResemble, []string{"failed stuff"})

				cursor, err := datastore.DecodeCursor(c, response.NextCursor)
				So(err, ShouldBeNil)

				Convey(`Fetch another 2`, func() {
					response2, err := GetBuilder(c, "fake", "fake", 2, cursor)
					So(err, ShouldBeNil)
					So(len(response2.FinishedBuilds), ShouldEqual, 2)
					So(response2.PrevCursor, ShouldEqual, "EMPTY")

					cursor, err := datastore.DecodeCursor(c, response2.NextCursor)
					So(err, ShouldBeNil)

					Convey(`Fetch another 2`, func() {
						response3, err := GetBuilder(c, "fake", "fake", 2, cursor)
						So(err, ShouldBeNil)
						So(len(response3.FinishedBuilds), ShouldEqual, 2)
						So(response3.PrevCursor, ShouldNotEqual, "")
						So(response3.PrevCursor, ShouldNotEqual, "EMPTY")

						cursor, err := datastore.DecodeCursor(c, response3.NextCursor)
						So(err, ShouldBeNil)

						Convey(`Fetch the rest`, func() {
							response4, err := GetBuilder(c, "fake", "fake", 20, cursor)
							So(err, ShouldBeNil)
							So(len(response4.FinishedBuilds), ShouldEqual, 4)
						})
					})
				})
			})
		})
	})
}
