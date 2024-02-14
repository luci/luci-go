// Copyright 2024 The LUCI Authors.
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

package retention

import (
	"slices"
	"testing"
	"time"

	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestScheduleWipeoutTryjobs(t *testing.T) {
	t.Parallel()

	Convey("Schedule wipeout tryjobs tasks", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()
		registerWipeoutTryjobsTask(ct.TQDispatcher)

		// create tryjobs with 1 minute interval.
		tryjobs := make([]*tryjob.Tryjob, 3*tryjobsPerTask)
		for i := range tryjobs {
			tryjobs[i] = tryjob.MustBuildbucketID("bb.example.com", int64(i+1000)).
				MustCreateIfNotExists(ctx)
			ct.Clock.Add(1 * time.Minute)
		}

		// Make half of the tryjobs eligible for wipeout
		ct.Clock.Set(tryjobs[len(tryjobs)/2].EntityUpdateTime.Add(retentionPeriod))

		So(scheduleWipeoutTryjobsTasks(ctx, ct.TQDispatcher), ShouldBeNil)
		var expectedTryjobIDs common.TryjobIDs
		for _, tj := range tryjobs[:len(tryjobs)/2] {
			expectedTryjobIDs = append(expectedTryjobIDs, tj.ID)
		}

		var actualTryjobIDs common.TryjobIDs
		for _, task := range ct.TQ.Tasks() {
			So(task.ETA, ShouldHappenWithin, 7*time.Hour, ct.Clock.Now())
			ids := task.Payload.(*WipeoutTryjobsTask).GetIds()
			So(len(ids), ShouldBeLessThanOrEqualTo, tryjobsPerTask)
			for _, id := range ids {
				actualTryjobIDs = append(actualTryjobIDs, common.TryjobID(id))
			}
		}
		slices.Sort(expectedTryjobIDs)
		slices.Sort(actualTryjobIDs)
		So(actualTryjobIDs, ShouldResemble, expectedTryjobIDs)
	})
}
func TestWipeoutTryjobs(t *testing.T) {
	t.Parallel()

	Convey("Wipeout Tryjobs", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		tj := tryjob.MustBuildbucketID("bb.example.com", 12345).
			MustCreateIfNotExists(ctx)
		ct.Clock.Add(2 * retentionPeriod) // make tryjob eligible for wipeout

		Convey("Can wipeout tryjob", func() {
			Convey("when there's no watching run", func() {
				So(wipeoutTryjobs(ctx, common.TryjobIDs{tj.ID}), ShouldBeNil)
				So(datastore.Get(ctx, &tryjob.Tryjob{ID: tj.ID}), ShouldErrLike, datastore.ErrNoSuchEntity)
			})
			Convey("when all watching run no longer exists", func() {
				tj.LaunchedBy = common.MakeRunID("infra", tj.EntityCreateTime, 1, []byte("deadbeef"))
				tj.ReusedBy = append(tj.ReusedBy, common.MakeRunID("infra", tj.EntityCreateTime.Add(1*time.Minute), 1, []byte("deadbeef")))
				So(wipeoutTryjobs(ctx, common.TryjobIDs{tj.ID}), ShouldBeNil)
				So(datastore.Get(ctx, &tryjob.Tryjob{ID: tj.ID}), ShouldErrLike, datastore.ErrNoSuchEntity)
			})
		})

		Convey("Don't wipeout tryjob", func() {
			Convey("When a run that use this tryjob still exists", func() {
				r := &run.Run{
					ID: common.MakeRunID("infra", tj.EntityCreateTime, 1, []byte("deadbeef")),
				}
				tj.LaunchedBy = r.ID
				tj.ReusedBy = append(tj.ReusedBy, common.MakeRunID("infra", tj.EntityCreateTime.Add(-1*time.Minute), 1, []byte("deadbeef")))
				So(datastore.Put(ctx, r, tj), ShouldBeNil)
				So(wipeoutTryjobs(ctx, common.TryjobIDs{tj.ID}), ShouldBeNil)
				So(datastore.Get(ctx, &tryjob.Tryjob{ID: tj.ID}), ShouldBeNil)
			})

			Convey("When Tryjob doesn't exist", func() {
				So(wipeoutTryjobs(ctx, common.TryjobIDs{tj.ID + 1}), ShouldBeNil)
			})
		})
	})
}
