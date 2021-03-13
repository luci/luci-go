// Copyright 2021 The LUCI Authors.
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

package submit

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/clock"
	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestQueue(t *testing.T) {
	t.Parallel()

	Convey("Queue", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "lProject"
		run1 := common.MakeRunID(lProject, clock.Now(ctx), 1, []byte("deadbeef"))
		run2 := common.MakeRunID(lProject, clock.Now(ctx), 1, []byte("cafecafe"))
		run3 := common.MakeRunID(lProject, clock.Now(ctx), 1, []byte("badbadbad"))
		submitOpts := &cfgpb.SubmitOptions{
			MaxBurst:   1,
			BurstDelay: durationpb.New(time.Second),
		}
		mustLoadQueue := func(luciProject string) *queue {
			q, err := loadQueue(ctx, luciProject)
			So(err, ShouldBeNil)
			return q
		}

		So(CreateQueue(ctx, lProject, submitOpts), ShouldBeNil)

		Convey("Creation", func() {
			q, err := loadQueue(ctx, lProject)
			So(err, ShouldBeNil)
			So(q.ID, ShouldEqual, lProject)
			So(q.Opts, ShouldResembleProto, submitOpts)
		})

		Convey("Deletion", func() {
			So(DeleteQueue(ctx, lProject), ShouldBeNil)
			_, err := loadQueue(ctx, lProject)
			So(err, ShouldErrLike, `SubmitQueue "lProject" doesn't exist`)
		})

		Convey("Acquire", func() {
			Convey("When queue is empty", func() {
				waitlisted, err := TryAcquire(ctx, run1)
				So(err, ShouldBeNil)
				So(waitlisted, ShouldBeFalse)
				q := mustLoadQueue(lProject)
				So(q.InProgress, ShouldEqual, run1)
				So(q.Waitlist, ShouldBeEmpty)

				Convey("Acquiring another Run will be added to waitlist", func() {
					waitlisted, err := TryAcquire(ctx, run2)
					So(err, ShouldBeNil)
					So(waitlisted, ShouldBeTrue)
					q := mustLoadQueue(lProject)
					So(q.InProgress, ShouldEqual, run1)
					So(q.Waitlist, ShouldResemble, common.RunIDs{run2})

					Convey("Noop when acquiring an existing run in queue", func() {
						for _, r := range []common.RunID{run1, run2} {
							waitlisted, err := TryAcquire(ctx, r)
							So(err, ShouldBeNil)
							So(waitlisted, ShouldEqual, r == run2)
							q := mustLoadQueue(lProject)
							So(q.InProgress, ShouldEqual, run1)
							So(q.Waitlist, ShouldResemble, common.RunIDs{run2})
						}
					})

					Convey("Add to waitlist as long as waitlist is not empty", func() {
						q := mustLoadQueue(lProject)
						q.InProgress = ""
						So(datastore.Put(ctx, q), ShouldBeNil)
						waitlisted, err := TryAcquire(ctx, run3)
						So(err, ShouldBeNil)
						So(waitlisted, ShouldBeTrue)
						q = mustLoadQueue(lProject)
						So(q.InProgress, ShouldEqual, "")
						So(q.Waitlist, ShouldResemble, common.RunIDs{run2, run3})
					})
				})
			})
		})

		Convey("Release", func() {
			_, err := TryAcquire(ctx, run1)
			So(err, ShouldBeNil)
			var notified common.RunIDs
			notifyFn := func(ctx context.Context, runID common.RunID, eta time.Time) error {
				notified = append(notified, runID)
				return nil
			}
			Convey("Current slot and waitlist is empty", func() {
				err := Release(ctx, run1, notifyFn)
				So(err, ShouldBeNil)
				So(notified, ShouldBeEmpty)
				q := mustLoadQueue(lProject)
				So(q.InProgress, ShouldEqual, "")
				So(q.Waitlist, ShouldBeEmpty)
			})

			_, err = TryAcquire(ctx, run2)
			So(err, ShouldBeNil)

			Convey("Current Slot and notify first Run in waitlist", func() {
				err := Release(ctx, run1, notifyFn)
				So(err, ShouldBeNil)
				So(notified, ShouldResemble, common.RunIDs{run2})
				q := mustLoadQueue(lProject)
				So(q.InProgress, ShouldEqual, "")
				So(q.Waitlist, ShouldResemble, common.RunIDs{run2})
			})

			Convey("Run in waitlist", func() {
				err := Release(ctx, run2, notifyFn)
				So(err, ShouldBeNil)
				So(notified, ShouldBeEmpty)
				q := mustLoadQueue(lProject)
				So(q.InProgress, ShouldEqual, run1)
				So(q.Waitlist, ShouldBeEmpty)
			})

			Convey("Non-existing run", func() {
				err := Release(ctx, run3, notifyFn)
				So(err, ShouldBeNil)
				So(notified, ShouldBeEmpty)
				q := mustLoadQueue(lProject)
				So(q.InProgress, ShouldEqual, run1)
				So(q.Waitlist, ShouldResemble, common.RunIDs{run2})
			})
		})
	})
}
