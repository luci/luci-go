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

type fakeBuild struct{}

func TestUpdateBuilder(t *testing.T) {
	c := gaetesting.TestingContextWithAppID("luci-milo-dev")

	// Populate a few BuildSummaries. For convenience, ordered by creation time.
	builds := make([]*BuildSummary, 10)
	for i := 0; i < 10; i++ {
		bk := datastore.MakeKey(c, "buildbucket.Build", fmt.Sprintf("test:%d", i))
		builds[i] = &BuildSummary{
			BuildKey:  bk,
			BuilderID: "fake",
			Created:   testclock.TestRecentTimeUTC.Add(time.Duration(i) * time.Hour),
		}
		datastore.Put(c, builds[i])
	}

	Convey("Updating appropriate builder having existing last finished build", t, func() {
		builder := &BuilderSummary{
			BuilderID:          "fake",
			LastFinishedStatus: Success,
			LastFinishedID:     builds[5].BuildKey,
		}

		Convey("and no pending builds", func() {
			Convey("with finished build should error but update last finished build info", func() {
				builds[6].Summary.Status = Failure
				err := builder.Update(c, builds[6])
				So(err, ShouldHaveSameTypeAs, ErrBuildMessageOutOfOrder{})
				So(builder.LastFinishedStatus, ShouldEqual, Failure)
				So(builder.LastFinishedID, ShouldEqual, builds[6].BuildKey)
			})

			Convey("with pending build should update InProgress but not last finished build info", func() {
				Convey("for build created earlier than last finished", func() {
					builds[4].Summary.Status = NotRun
					err := builder.Update(c, builds[4])
					So(err, ShouldBeNil)
					So(builder.LastFinishedStatus, ShouldEqual, Success)
					So(builder.LastFinishedID, ShouldEqual, builds[5].BuildKey)
					So(builder.GetInProgress(), ShouldResemble,
						map[string]Status{builds[4].BuildKey.String(): NotRun})
				})

				Convey("for build created later than last finished", func() {
					builds[6].Summary.Status = NotRun
					err := builder.Update(c, builds[6])
					So(err, ShouldBeNil)
					So(builder.LastFinishedStatus, ShouldEqual, Success)
					So(builder.LastFinishedID, ShouldEqual, builds[5].BuildKey)
					So(builder.GetInProgress(), ShouldResemble,
						map[string]Status{builds[6].BuildKey.String(): NotRun})
				})
			})
		})

		Convey("and unrelated pending build", func() {
			builder.InProgress = []pendingBuild{
				{builds[3].BuildKey.String(), NotRun},
			}

			Convey("with finished build should error", func() {
				Convey("and not update last finished build info for build created earlier than last finished", func() {
					builds[4].Summary.Status = Failure
					err := builder.Update(c, builds[4])
					So(err, ShouldHaveSameTypeAs, ErrBuildMessageOutOfOrder{})
					So(builder.LastFinishedStatus, ShouldEqual, Success)
					So(builder.LastFinishedID, ShouldEqual, builds[5].BuildKey)
				})

				Convey("but update last finished build info for build created later than last finished", func() {
					builds[6].Summary.Status = Failure
					err := builder.Update(c, builds[6])
					So(err, ShouldHaveSameTypeAs, ErrBuildMessageOutOfOrder{})
					So(builder.LastFinishedStatus, ShouldEqual, Failure)
					So(builder.LastFinishedID, ShouldEqual, builds[6].BuildKey)
				})
			})

			Convey("with pending build should update InProgress but not last finished build info", func() {
				Convey("for build created earlier than last finished", func() {
					builds[4].Summary.Status = NotRun
					err := builder.Update(c, builds[4])
					So(err, ShouldBeNil)
					So(builder.LastFinishedStatus, ShouldEqual, Success)
					So(builder.LastFinishedID, ShouldEqual, builds[5].BuildKey)
					So(builder.GetInProgress(), ShouldResemble,
						map[string]Status{
							builds[3].BuildKey.String(): NotRun,
							builds[4].BuildKey.String(): NotRun,
						})
				})

				Convey("for build created later than last finished", func() {
					builds[6].Summary.Status = NotRun
					err := builder.Update(c, builds[6])
					So(err, ShouldBeNil)
					So(builder.LastFinishedStatus, ShouldEqual, Success)
					So(builder.LastFinishedID, ShouldEqual, builds[5].BuildKey)
					So(builder.GetInProgress(), ShouldResemble,
						map[string]Status{
							builds[3].BuildKey.String(): NotRun,
							builds[6].BuildKey.String(): NotRun,
						})
				})
			})
		})

		Convey("and several pending builds", func() {
			builder.InProgress = []pendingBuild{
				{builds[3].BuildKey.String(), NotRun},
				{builds[4].BuildKey.String(), NotRun},
				{builds[7].BuildKey.String(), NotRun},
			}

			Convey("with finished build", func() {
				Convey("created earlier than last finished should update InProgress but not last finished build info", func() {
					builds[4].Summary.Status = Failure
					err := builder.Update(c, builds[4])
					So(err, ShouldBeNil)
					So(builder.LastFinishedStatus, ShouldEqual, Success)
					So(builder.LastFinishedID, ShouldResemble, builds[5].BuildKey)
					So(builder.GetInProgress(), ShouldResemble,
						map[string]Status{
							builds[3].BuildKey.String(): NotRun,
							builds[7].BuildKey.String(): NotRun,
						})
				})

				Convey("created later than last finished should update InProgress and last finished build info", func() {
					builds[7].Summary.Status = Failure
					err := builder.Update(c, builds[7])
					So(err, ShouldBeNil)
					So(builder.LastFinishedStatus, ShouldEqual, Failure)
					So(builder.LastFinishedID, ShouldResemble, builds[7].BuildKey)
					So(builder.GetInProgress(), ShouldResemble,
						map[string]Status{
							builds[3].BuildKey.String(): NotRun,
							builds[4].BuildKey.String(): NotRun,
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
						So(builder.GetInProgress(), ShouldResemble,
							map[string]Status{
								builds[3].BuildKey.String(): NotRun,
								builds[4].BuildKey.String(): NotRun,
								builds[7].BuildKey.String(): NotRun,
								builds[2].BuildKey.String(): NotRun,
							})
					})

					Convey("created later than last finished", func() {
						builds[8].Summary.Status = NotRun
						err := builder.Update(c, builds[8])
						So(err, ShouldBeNil)
						So(builder.LastFinishedStatus, ShouldEqual, Success)
						So(builder.LastFinishedID, ShouldResemble, builds[5].BuildKey)
						So(builder.GetInProgress(), ShouldResemble,
							map[string]Status{
								builds[3].BuildKey.String(): NotRun,
								builds[4].BuildKey.String(): NotRun,
								builds[7].BuildKey.String(): NotRun,
								builds[8].BuildKey.String(): NotRun,
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
						So(builder.GetInProgress(), ShouldResemble,
							map[string]Status{
								builds[3].BuildKey.String(): Running,
								builds[4].BuildKey.String(): NotRun,
								builds[7].BuildKey.String(): NotRun,
							})
					})

					Convey("created later than last finished", func() {
						builds[7].Summary.Status = Running
						err := builder.Update(c, builds[7])
						So(err, ShouldBeNil)
						So(builder.LastFinishedStatus, ShouldEqual, Success)
						So(builder.LastFinishedID, ShouldResemble, builds[5].BuildKey)
						So(builder.GetInProgress(), ShouldResemble,
							map[string]Status{
								builds[3].BuildKey.String(): NotRun,
								builds[4].BuildKey.String(): NotRun,
								builds[7].BuildKey.String(): Running,
							})
					})
				})
			})
		})
	})

	Convey("Updating appropriate builder with no last finished builder should initialize", t, func() {
		builder := &BuilderSummary{BuilderID: "fake"}

		Convey("last finished build info with finished build", func() {
			builds[5].Summary.Status = Failure
			err := builder.Update(c, builds[5])
			So(err, ShouldHaveSameTypeAs, ErrBuildMessageOutOfOrder{})
			So(builder.LastFinishedStatus, ShouldEqual, Failure)
			So(builder.LastFinishedID, ShouldEqual, builds[5].BuildKey)
		})

		Convey("InProgress with pending build", func() {
			builds[5].Summary.Status = NotRun
			err := builder.Update(c, builds[5])
			So(err, ShouldBeNil)
			So(builder.LastFinishedID, ShouldEqual, nil)
			So(builder.GetInProgress(), ShouldResemble,
				map[string]Status{builds[5].BuildKey.String(): NotRun})
		})
	})

	Convey("Updating wrong builder should error", t, func() {
		builder := &BuilderSummary{BuilderID: "wrong"}
		err := builder.Update(c, builds[5])
		So(err.Error(), ShouldContainSubstring, "updating wrong builder")
	})
}
