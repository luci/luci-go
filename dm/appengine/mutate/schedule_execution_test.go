// Copyright 2015 The LUCI Authors.
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

package mutate

import (
	"fmt"
	"testing"
	"time"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/dm/api/service/v1"
	"go.chromium.org/luci/dm/appengine/distributor/fake"
	"go.chromium.org/luci/dm/appengine/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestScheduleExecution(t *testing.T) {
	t.Parallel()

	Convey("ScheduleExecution", t, func() {
		_, c, dist := fake.Setup(FinishExecutionFn)

		qdesc := fake.QuestDesc("quest")
		qdesc.Meta.Timeouts.Start = google.NewDuration(time.Minute * 5)
		qid := qdesc.QuestID()
		se := &ScheduleExecution{dm.NewAttemptID(qid, 1)}

		Convey("Root", func() {
			So(se.Root(c).String(), ShouldEqual, fmt.Sprintf(`dev~app::/Attempt,"%s|fffffffe"`, qid))
		})

		Convey("RollForward", func() {
			q := &model.Quest{ID: qid, Desc: *qdesc}
			a := &model.Attempt{
				ID:    *se.For,
				State: dm.Attempt_SCHEDULING,
			}
			e := model.ExecutionFromID(c, dm.NewExecutionID(qid, 1, 1))
			So(ds.Put(c, q, a), ShouldBeNil)

			Convey("basic", func() {
				muts, err := se.RollForward(c)
				So(err, ShouldBeNil)
				So(muts, ShouldBeNil)

				So(ds.Get(c, a, e), ShouldBeNil)
				Convey("distributor information is saved", func() {
					tok := fake.MakeToken(dm.NewExecutionID(qid, 1, 1))

					So(a.State, ShouldEqual, dm.Attempt_EXECUTING)
					So(e.State, ShouldEqual, dm.Execution_SCHEDULING)
					So(e.DistributorConfigName, ShouldEqual, "fakeDistributor")
					So(e.DistributorToken, ShouldEqual, tok)
					So(e.DistributorConfigVersion, ShouldEqual, "testing")
					So(e.TimeToStart, ShouldEqual, time.Minute*5)
				})
				Convey("a timeout is set", func() {
					ex, err := ds.Exists(c, ds.MakeKey(c,
						"Attempt", a.ID.DMEncoded(),
						"Execution", 1,
						"tumble.Mutation", "n:timeout"))
					So(err, ShouldBeNil)
					So(ex.All(), ShouldBeTrue)
				})
			})

			Convey("transient", func() {
				dist.RunError = errors.New("transient failure", transient.Tag)

				muts, err := se.RollForward(c)
				So(err, ShouldErrLike, "transient")
				So(muts, ShouldBeNil)
			})

			Convey("rejection", func() {
				dist.RunError = errors.New("no soup for you")

				muts, err := se.RollForward(c)
				So(err, ShouldBeNil)
				So(muts, ShouldBeNil)

				So(ds.Get(c, a, e), ShouldBeNil)
				So(a.State, ShouldEqual, dm.Attempt_ABNORMAL_FINISHED)
				So(a.Result.AbnormalFinish.Status, ShouldEqual, dm.AbnormalFinish_REJECTED)
				So(a.Result.AbnormalFinish.Reason, ShouldContainSubstring, "non-transient")

				So(e.State, ShouldEqual, dm.Execution_ABNORMAL_FINISHED)
				So(e.Result.AbnormalFinish.Status, ShouldEqual, dm.AbnormalFinish_REJECTED)
				So(e.Result.AbnormalFinish.Reason, ShouldContainSubstring, "non-transient")
			})

		})
	})
}
