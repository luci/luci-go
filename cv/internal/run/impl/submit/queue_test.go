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
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run/runtest"

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
		run1 := common.MakeRunID(lProject, clock.Now(ctx), 1, []byte("deaddead"))
		run2 := common.MakeRunID(lProject, clock.Now(ctx), 1, []byte("beefbeef"))
		run3 := common.MakeRunID(lProject, clock.Now(ctx), 1, []byte("cafecafe"))
		submitOpts := &cfgpb.SubmitOptions{
			MaxBurst:   2,
			BurstDelay: durationpb.New(1 * time.Minute),
		}
		mustLoadQueue := func() *queue {
			q, err := loadQueue(ctx, lProject)
			So(err, ShouldBeNil)
			return q
		}

		Convey("Creation", func() {
			So(CreateQueue(ctx, lProject, submitOpts), ShouldBeNil)
			q := mustLoadQueue()
			So(q.ID, ShouldEqual, lProject)
			So(q.Opts, ShouldResembleProto, submitOpts)
		})

		Convey("Deletion", func() {
			So(CreateQueue(ctx, lProject, submitOpts), ShouldBeNil)
			So(DeleteQueue(ctx, lProject), ShouldBeNil)
			_, err := loadQueue(ctx, lProject)
			So(err, ShouldErrLike, `SubmitQueue "lProject" doesn't exist`)
		})

		initQueue := func(current common.RunID, waitlist common.RunIDs) {
			So(CreateQueue(ctx, lProject, submitOpts), ShouldBeNil)
			q := mustLoadQueue()
			q.Current = current
			q.Waitlist = waitlist
			So(datastore.Put(ctx, q), ShouldBeNil)
		}

		verifyQueue := func(expectedCurrent common.RunID, expectedWaitlist common.RunIDs) {
			q := mustLoadQueue()
			So(q.Current, ShouldEqual, expectedCurrent)
			So(q.Waitlist, ShouldResemble, expectedWaitlist)
		}

		Convey("Acquire", func() {
			Convey("When queue is empty", func() {
				initQueue("", nil)
				waitlisted, eta, err := TryAcquire(ctx, run1)
				So(err, ShouldBeNil)
				So(waitlisted, ShouldBeFalse)
				So(eta.IsZero(), ShouldBeTrue)
				verifyQueue(run1, nil)

				Convey("And acquire same Run again", func() {
					initQueue(run1, nil)
					waitlisted, eta, err := TryAcquire(ctx, run1)
					So(err, ShouldBeNil)
					So(waitlisted, ShouldBeFalse)
					So(eta.IsZero(), ShouldBeTrue)
					verifyQueue(run1, nil)
				})
			})

			Convey("Waitlisted", func() {
				initQueue(run1, nil)
				waitlisted, eta, err := TryAcquire(ctx, run2)
				So(err, ShouldBeNil)
				So(waitlisted, ShouldBeTrue)
				So(eta.IsZero(), ShouldBeTrue)
				verifyQueue(run1, common.RunIDs{run2})
				waitlisted, eta, err = TryAcquire(ctx, run3)
				So(err, ShouldBeNil)
				So(waitlisted, ShouldBeTrue)
				So(eta.IsZero(), ShouldBeTrue)
				verifyQueue(run1, common.RunIDs{run2, run3})

				Convey("And acquire same Run in waitlist again", func() {
					waitlisted, eta, err := TryAcquire(ctx, run2)
					So(err, ShouldBeNil)
					So(waitlisted, ShouldBeTrue)
					So(eta.IsZero(), ShouldBeTrue)
					verifyQueue(run1, common.RunIDs{run2, run3})
				})
			})

			Convey("First in waitlist while Current is empty", func() {
				initQueue("", common.RunIDs{run2, run3})
				waitlisted, eta, err := TryAcquire(ctx, run2)
				So(err, ShouldBeNil)
				So(waitlisted, ShouldBeFalse)
				So(eta.IsZero(), ShouldBeTrue)
				verifyQueue(run2, common.RunIDs{run3})
			})
		})

		Convey("Release", func() {
			initQueue(run1, common.RunIDs{run2, run3})
			Convey("Current slot", func() {
				err := Release(ctx, run1)
				So(err, ShouldBeNil)
				verifyQueue("", common.RunIDs{run2, run3})
				runtest.AssertReceivedReadyForSubmission(ctx, run2, time.Time{})
				for _, r := range []common.RunID{run1, run3} {
					runtest.AssertEventboxEmpty(ctx, r)
				}
			})

			Convey("Run in waitlist", func() {
				err := Release(ctx, run2)
				So(err, ShouldBeNil)
				verifyQueue(run1, common.RunIDs{run3})
				for _, r := range []common.RunID{run1, run2, run3} {
					runtest.AssertEventboxEmpty(ctx, r)
				}
			})

			Convey("Non-existing run", func() {
				nonExisting := common.MakeRunID(lProject, clock.Now(ctx), 1, []byte("badbadbad"))
				err := Release(ctx, nonExisting)
				So(err, ShouldBeNil)
				verifyQueue(run1, common.RunIDs{run2, run3})
				for _, r := range []common.RunID{run1, run2, run3} {
					runtest.AssertEventboxEmpty(ctx, r)
				}
			})
		})
	})
}
