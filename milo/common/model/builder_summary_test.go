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

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
)

type fakeBuild struct {}

func TestUpdateBuilder(t *testing.T) {
	c := gaetesting.TestingContextWithAppID("luci-milo-dev")

	// Populate a few BuildSummaries. For convenience, ordered by creation time.
	builds := make([]*BuildSummary, 10)
	for i := 0; i < 10; i++ {
		bk := datastore.MakeKey(c, "buildbucket.Build", fmt.Sprintf("test:%d", i))
		builds[i] = &BuildSummary{
			BuildKey: bk,
			BuilderID: "fake",
			Created: testclock.TestRecentTimeUTC.Add(time.Duration(i) * time.Hour),
		}
		datastore.Put(c, builds[i])
	}

	Convey("Updating appropriate builder", t, func() {
		builder := &BuilderSummary{
			BuilderID: "fake",
			LastFinishedStatus: Success,
			LastFinishedID: builds[5].BuildKey,
		}

		Convey("having no pending builds", func() {
			Convey("with finished build should panic", func() {
				builds[6].Summary.Status = Failure
				So(func() {builder.Update(c, builds[6])}, ShouldPanic)
			})

			Convey("with pending build should update InProgress but not last finished build info", func() {
				Convey("for build created earlier than last finished", func() {
					builds[4].Summary.Status = NotRun
					err := builder.Update(c, builds[4])
					So(err, ShouldBeNil)
					So(builder.LastFinishedStatus, ShouldEqual, Success)
					So(builder.LastFinishedID, ShouldEqual, builds[5].BuildKey)
					So(builder.InProgress, ShouldResemble,
						[]pendingBuild{pendingBuild{builds[4].BuildKey, NotRun}})
				})

				Convey("for build created later than last finished", func() {
					builds[6].Summary.Status = NotRun
					err := builder.Update(c, builds[6])
					So(err, ShouldBeNil)
					So(builder.LastFinishedStatus, ShouldEqual, Success)
					So(builder.LastFinishedID, ShouldEqual, builds[5].BuildKey)
					So(builder.InProgress, ShouldResemble,
						[]pendingBuild{pendingBuild{builds[6].BuildKey, NotRun}})
				})
			})
		})

		Convey("having unrelated pending build", func() {
			builder.InProgress = []pendingBuild{
				pendingBuild{builds[3].BuildKey,NotRun},
			}

			Convey("with finished build should panic", func() {
				builds[6].Summary.Status = Failure
				So(func() {builder.Update(c, builds[6])}, ShouldPanic)
			})

			Convey("with pending build should update InProgress but not last finished build info", func() {
				Convey("for build created earlier than last finished", func() {
					builds[4].Summary.Status = NotRun
					err := builder.Update(c, builds[4])
					So(err, ShouldBeNil)
					So(builder.LastFinishedStatus, ShouldEqual, Success)
					So(builder.LastFinishedID, ShouldEqual, builds[5].BuildKey)
					So(builder.InProgress, ShouldResemble,
						[]pendingBuild{
							pendingBuild{builds[3].BuildKey, NotRun},
							pendingBuild{builds[4].BuildKey, NotRun},
					})
				})

				Convey("for build created later than last finished", func() {
					builds[6].Summary.Status = NotRun
					err := builder.Update(c, builds[6])
					So(err, ShouldBeNil)
					So(builder.LastFinishedStatus, ShouldEqual, Success)
					So(builder.LastFinishedID, ShouldEqual, builds[5].BuildKey)
					So(builder.InProgress, ShouldResemble,
						[]pendingBuild{
							pendingBuild{builds[3].BuildKey, NotRun},
							pendingBuild{builds[6].BuildKey, NotRun},
					})
				})
			})
		})

		Convey("having several pending builds", func() {
			builder.InProgress = []pendingBuild{
				pendingBuild{builds[3].BuildKey, NotRun},
				pendingBuild{builds[4].BuildKey, NotRun},
				pendingBuild{builds[7].BuildKey, NotRun},
			}

			Convey("with finished build", func() {
				Convey("created earlier than last finished should update InProgress but not last finished build info", func() {
					builds[4].Summary.Status = Failure
					err := builder.Update(c, builds[4])
					So(err, ShouldBeNil)
					So(builder.LastFinishedStatus, ShouldEqual, Success)
					So(builder.LastFinishedID, ShouldResemble, builds[5].BuildKey)
					So(builder.InProgress, ShouldResemble,
						[]pendingBuild{
							pendingBuild{builds[3].BuildKey, NotRun},
							pendingBuild{builds[7].BuildKey, NotRun},
					})
				})

				Convey("created later than last finished should update InProgress and last finished build info", func() {
					builds[7].Summary.Status = Failure
					err := builder.Update(c, builds[7])
					So(err, ShouldBeNil)
					So(builder.LastFinishedStatus, ShouldEqual, Failure)
					So(builder.LastFinishedID, ShouldResemble, builds[7].BuildKey)
					So(builder.InProgress, ShouldResemble,
						[]pendingBuild{
							pendingBuild{builds[3].BuildKey, NotRun},
							pendingBuild{builds[4].BuildKey, NotRun},
					})
				})
			})

			Convey("with pending build should update InProgress but not last finished build info", func() {
				Convey("for newly pending build", func() {
					Convey("created earlier than last finished", func() {
						builds[2].Summary.Status = NotRun
						err := builder.Update(c, builds[2])
						So(err, ShouldBeNil)
						So(builder.LastFinishedStatus, ShouldEqual, Success)
						So(builder.LastFinishedID, ShouldResemble, builds[5].BuildKey)
						So(builder.InProgress, ShouldResemble,
							[]pendingBuild{
								pendingBuild{builds[3].BuildKey, NotRun},
								pendingBuild{builds[4].BuildKey, NotRun},
								pendingBuild{builds[7].BuildKey, NotRun},
								pendingBuild{builds[2].BuildKey, NotRun},
						})
					})

					Convey("created later than last finished", func() {
						builds[8].Summary.Status = NotRun
						err := builder.Update(c, builds[8])
						So(err, ShouldBeNil)
						So(builder.LastFinishedStatus, ShouldEqual, Success)
						So(builder.LastFinishedID, ShouldResemble, builds[5].BuildKey)
						So(builder.InProgress, ShouldResemble,
							[]pendingBuild{
								pendingBuild{builds[3].BuildKey, NotRun},
								pendingBuild{builds[4].BuildKey, NotRun},
								pendingBuild{builds[7].BuildKey, NotRun},
								pendingBuild{builds[8].BuildKey, NotRun},
						})
					})
				})

				Convey("for existing pending build", func() {
					Convey("created earlier than last finished", func() {
						builds[3].Summary.Status = Running
						err := builder.Update(c, builds[3])
						So(err, ShouldBeNil)
						So(builder.LastFinishedStatus, ShouldEqual, Success)
						So(builder.LastFinishedID, ShouldResemble, builds[5].BuildKey)
						So(builder.InProgress, ShouldResemble,
							[]pendingBuild{
								pendingBuild{builds[3].BuildKey, Running},
								pendingBuild{builds[4].BuildKey, NotRun},
								pendingBuild{builds[7].BuildKey, NotRun},
						})
					})

					Convey("created later than last finished", func() {
						builds[7].Summary.Status = Running
						err := builder.Update(c, builds[7])
						So(err, ShouldBeNil)
						So(builder.LastFinishedStatus, ShouldEqual, Success)
						So(builder.LastFinishedID, ShouldResemble, builds[5].BuildKey)
						So(builder.InProgress, ShouldResemble,
							[]pendingBuild{
								pendingBuild{builds[3].BuildKey, NotRun},
								pendingBuild{builds[4].BuildKey, NotRun},
								pendingBuild{builds[7].BuildKey, Running},
						})
					})
				})
			})
		})
	})

	Convey("Updating wrong builder should panic", t, func() {
		builder := &BuilderSummary{BuilderID: "wrong"}
		datastore.Put(c, builder)
		So(func() {builder.Update(c, builds[5])}, ShouldPanic)
	})
}
