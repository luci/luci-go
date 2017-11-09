// Copyright 2016 The LUCI Authors.
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
	"go.chromium.org/luci/server/caching"

	. "github.com/smartystreets/goconvey/convey"
)

func TestUpdateBuilder(t *testing.T) {
	t.Parallel()

	Convey(`TestUpdateBuilder`, t, func() {
		c := gaetesting.TestingContextWithAppID("luci-milo-dev")

		// Populate a few BuildSummaries. For convenience, ordered by creation time.
		builds := make([]*BuildSummary, 10)
		for i := 0; i < 10; i++ {
			bk := datastore.MakeKey(c, "fakeBuild", i)
			builds[i] = &BuildSummary{
				BuildKey:  bk,
				BuilderID: "fake",
				BuildID:   fmt.Sprintf("build_id/%d", i),
				Created:   testclock.TestRecentTimeUTC.Add(time.Duration(i) * time.Hour),
			}
		}

		c = caching.WithRequestCache(c)

		Convey("Updating appropriate builder having existing last finished build", func() {
			builder := &BuilderSummary{
				BuilderID:           "fake",
				LastFinishedCreated: builds[5].Created,
				LastFinishedStatus:  Success,
				LastFinishedBuildID: builds[5].BuildID,
			}

			Convey("and no pending builds", func() {
				Convey("with finished build should error but update last finished build info", func() {
					builds[6].Summary.Status = Failure
					err := builder.Update(c, builds[6])
					So(BuildMessageOutOfOrderTag.In(err), ShouldEqual, true)
					So(builder.LastFinishedStatus, ShouldEqual, Failure)
					So(builder.LastFinishedBuildID, ShouldEqual, builds[6].BuildID)
				})

				Convey("with pending build should update InProgress but not last finished build info", func() {
					Convey("for build created earlier than last finished", func() {
						builds[4].Summary.Status = NotRun
						err := builder.Update(c, builds[4])
						So(err, ShouldBeNil)
						So(builder.LastFinishedStatus, ShouldEqual, Success)
						So(builder.LastFinishedBuildID, ShouldEqual, builds[5].BuildID)
						So(builder.GetInProgress(), ShouldResemble,
							map[string]Status{builds[4].BuildID: NotRun})
					})

					Convey("for build created later than last finished", func() {
						builds[6].Summary.Status = NotRun
						err := builder.Update(c, builds[6])
						So(err, ShouldBeNil)
						So(builder.LastFinishedStatus, ShouldEqual, Success)
						So(builder.LastFinishedBuildID, ShouldEqual, builds[5].BuildID)
						So(builder.GetInProgress(), ShouldResemble,
							map[string]Status{builds[6].BuildID: NotRun})
					})
				})
			})

			Convey("and unrelated pending build", func() {
				builder.InProgress = []pendingBuild{
					{builds[3].BuildID, NotRun},
				}

				Convey("with finished build should error", func() {
					Convey("and not update last finished build info for build created earlier than last finished", func() {
						builds[4].Summary.Status = Failure
						err := builder.Update(c, builds[4])
						So(BuildMessageOutOfOrderTag.In(err), ShouldEqual, true)
						So(builder.LastFinishedStatus, ShouldEqual, Success)
						So(builder.LastFinishedBuildID, ShouldEqual, builds[5].BuildID)
					})

					Convey("but update last finished build info for build created later than last finished", func() {
						builds[6].Summary.Status = Failure
						err := builder.Update(c, builds[6])
						So(BuildMessageOutOfOrderTag.In(err), ShouldEqual, true)
						So(builder.LastFinishedStatus, ShouldEqual, Failure)
						So(builder.LastFinishedBuildID, ShouldEqual, builds[6].BuildID)
					})
				})

				Convey("with pending build should update InProgress but not last finished build info", func() {
					Convey("for build created earlier than last finished", func() {
						builds[4].Summary.Status = NotRun
						err := builder.Update(c, builds[4])
						So(err, ShouldBeNil)
						So(builder.LastFinishedStatus, ShouldEqual, Success)
						So(builder.LastFinishedBuildID, ShouldEqual, builds[5].BuildID)
						So(builder.GetInProgress(), ShouldResemble,
							map[string]Status{
								builds[3].BuildID: NotRun,
								builds[4].BuildID: NotRun,
							})
					})

					Convey("for build created later than last finished", func() {
						builds[6].Summary.Status = NotRun
						err := builder.Update(c, builds[6])
						So(err, ShouldBeNil)
						So(builder.LastFinishedStatus, ShouldEqual, Success)
						So(builder.LastFinishedBuildID, ShouldEqual, builds[5].BuildID)
						So(builder.GetInProgress(), ShouldResemble,
							map[string]Status{
								builds[3].BuildID: NotRun,
								builds[6].BuildID: NotRun,
							})
					})
				})
			})

			Convey("and several pending builds", func() {
				builder.InProgress = []pendingBuild{
					{builds[3].BuildID, NotRun},
					{builds[4].BuildID, NotRun},
					{builds[7].BuildID, NotRun},
				}

				Convey("with finished build", func() {
					Convey("created earlier than last finished should update InProgress but not last finished build info", func() {
						builds[4].Summary.Status = Failure
						err := builder.Update(c, builds[4])
						So(err, ShouldBeNil)
						So(builder.LastFinishedStatus, ShouldEqual, Success)
						So(builder.LastFinishedBuildID, ShouldResemble, builds[5].BuildID)
						So(builder.GetInProgress(), ShouldResemble,
							map[string]Status{
								builds[3].BuildID: NotRun,
								builds[7].BuildID: NotRun,
							})
					})

					Convey("created later than last finished should update InProgress and last finished build info", func() {
						builds[7].Summary.Status = Failure
						err := builder.Update(c, builds[7])
						So(err, ShouldBeNil)
						So(builder.LastFinishedStatus, ShouldEqual, Failure)
						So(builder.LastFinishedBuildID, ShouldResemble, builds[7].BuildID)
						So(builder.GetInProgress(), ShouldResemble,
							map[string]Status{
								builds[3].BuildID: NotRun,
								builds[4].BuildID: NotRun,
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
							So(builder.LastFinishedBuildID, ShouldResemble, builds[5].BuildID)
							So(builder.GetInProgress(), ShouldResemble,
								map[string]Status{
									builds[3].BuildID: NotRun,
									builds[4].BuildID: NotRun,
									builds[7].BuildID: NotRun,
									builds[2].BuildID: NotRun,
								})
						})

						Convey("created later than last finished", func() {
							builds[8].Summary.Status = NotRun
							err := builder.Update(c, builds[8])
							So(err, ShouldBeNil)
							So(builder.LastFinishedStatus, ShouldEqual, Success)
							So(builder.LastFinishedBuildID, ShouldResemble, builds[5].BuildID)
							So(builder.GetInProgress(), ShouldResemble,
								map[string]Status{
									builds[3].BuildID: NotRun,
									builds[4].BuildID: NotRun,
									builds[7].BuildID: NotRun,
									builds[8].BuildID: NotRun,
								})
						})
					})

					Convey("for existing pending build", func() {
						Convey("created earlier than last finished", func() {
							builds[3].Summary.Status = Running
							err := builder.Update(c, builds[3])
							So(err, ShouldBeNil)
							So(builder.LastFinishedStatus, ShouldEqual, Success)
							So(builder.LastFinishedBuildID, ShouldResemble, builds[5].BuildID)
							So(builder.GetInProgress(), ShouldResemble,
								map[string]Status{
									builds[3].BuildID: Running,
									builds[4].BuildID: NotRun,
									builds[7].BuildID: NotRun,
								})
						})

						Convey("created later than last finished", func() {
							builds[7].Summary.Status = Running
							err := builder.Update(c, builds[7])
							So(err, ShouldBeNil)
							So(builder.LastFinishedStatus, ShouldEqual, Success)
							So(builder.LastFinishedBuildID, ShouldResemble, builds[5].BuildID)
							So(builder.GetInProgress(), ShouldResemble,
								map[string]Status{
									builds[3].BuildID: NotRun,
									builds[4].BuildID: NotRun,
									builds[7].BuildID: Running,
								})
						})
					})
				})
			})
		})

		Convey("Updating appropriate builder with no last finished builder should initialize", func() {
			builder := &BuilderSummary{BuilderID: "fake"}

			Convey("last finished build info with finished build", func() {
				builds[5].Summary.Status = Failure
				err := builder.Update(c, builds[5])
				So(BuildMessageOutOfOrderTag.In(err), ShouldEqual, true)
				So(builder.LastFinishedStatus, ShouldEqual, Failure)
				So(builder.LastFinishedBuildID, ShouldEqual, builds[5].BuildID)
			})

			Convey("InProgress with pending build", func() {
				builds[5].Summary.Status = NotRun
				err := builder.Update(c, builds[5])
				So(err, ShouldBeNil)
				So(builder.LastFinishedBuildID, ShouldEqual, "")
				So(builder.GetInProgress(), ShouldResemble,
					map[string]Status{builds[5].BuildID: NotRun})
			})
		})

		Convey("Updating wrong builder should error", func() {
			builder := &BuilderSummary{BuilderID: "wrong"}
			err := builder.Update(c, builds[5])
			So(err.Error(), ShouldContainSubstring, "updating wrong builder")
		})

		Convey("Updating builder with build with consoles should overwrite builder consoles", func() {
			builder := &BuilderSummary{BuilderID: "consoles"}
			build := &BuildSummary{
				BuilderID: "consoles",
			}
			build.AddManifestKey("tok", "id0", "MANIFEST_NAME", "https://repo.example.com", []byte("dead"))
			build.AddManifestKey("tok", "id0", "OTHER_MANIFEST_KIND", "https://repo.example.com", []byte("dead"))
			build.AddManifestKey("tok", "id1", "MANIFEST_NAME", "https://repo.example.com", []byte("beef"))

			err := builder.Update(c, build)
			So(err, ShouldBeNil)
			So(builder.Consoles, ShouldResemble, []string{"tok/id0", "tok/id1"})
		})
	})

}
