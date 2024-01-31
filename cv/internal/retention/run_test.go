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
	"testing"
	"time"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestScheduleWipeoutRuns(t *testing.T) {
	t.Parallel()

	Convey("Schedule wipeout runs tasks", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()
		registerWipeoutRunsTask(ct.TQDispatcher)

		// Test Scenario: Create a lot of runs under 2 LUCI Projects (1 disabled).
		// Making sure the tasks are scheduled for all runs that are out of
		// retention period.

		cfg := &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{{
				Name: "main",
			}},
		}
		const projFoo = "foo"
		const projDisabled = "disabled"
		prjcfgtest.Create(ctx, projFoo, cfg)
		prjcfgtest.Create(ctx, projDisabled, cfg)
		prjcfgtest.Disable(ctx, projDisabled)

		createNRunsBetween := func(proj string, n int, start, end time.Time) []*run.Run {
			So(n, ShouldBeGreaterThan, 0)
			So(end, ShouldHappenAfter, start)
			runs := make([]*run.Run, n)
			for i := range runs {
				createTime := start.Add(time.Duration(mathrand.Int63n(ctx, int64(end.Sub(start))))).Truncate(time.Millisecond)
				runs[i] = &run.Run{
					ID:         common.MakeRunID(proj, createTime, 1, []byte("deadbeef")),
					CreateTime: createTime,
				}
			}
			So(datastore.Put(ctx, runs), ShouldBeNil)
			return runs
		}

		cutOff := ct.Clock.Now().UTC().Add(-retentionPeriod)
		var allRuns []*run.Run
		allRuns = append(allRuns, createNRunsBetween(projFoo, 1000, cutOff.Add(-time.Hour), cutOff.Add(time.Hour))...)
		allRuns = append(allRuns, createNRunsBetween(projDisabled, 500, cutOff.Add(-time.Minute), cutOff.Add(time.Minute))...)

		var expectedRuns common.RunIDs
		for _, r := range allRuns {
			if r.CreateTime.Before(cutOff) {
				expectedRuns.InsertSorted(r.ID)
			}
		}

		err := scheduleWipeoutRuns(ctx, ct.TQDispatcher)
		So(err, ShouldBeNil)
		var actualRuns common.RunIDs
		for _, task := range ct.TQ.Tasks() {
			So(task.ETA, ShouldHappenWithin, 16*time.Hour, ct.Clock.Now())
			ids := task.Payload.(*WipeoutRunsTask).GetIds()
			So(len(ids), ShouldBeLessThanOrEqualTo, runsPerTask)
			for _, id := range ids {
				actualRuns.InsertSorted(common.RunID(id))
			}
		}
		So(actualRuns, ShouldResemble, expectedRuns)
	})
}

func TestWipeoutRuns(t *testing.T) {
	t.Parallel()

	Convey("Wipeout", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		const lProject = "infra"
		makeRun := func(createTime time.Time) *run.Run {
			r := &run.Run{
				ID:         common.MakeRunID(lProject, createTime, 1, []byte("deadbeef")),
				CreateTime: createTime,
			}
			So(datastore.Put(ctx, r), ShouldBeNil)
			return r
		}

		Convey("wipeout Run and children", func() {
			r := makeRun(ct.Clock.Now().Add(-2 * retentionPeriod).UTC())
			cl1 := &run.RunCL{
				ID:  1,
				Run: datastore.KeyForObj(ctx, r),
			}
			cl2 := &run.RunCL{
				ID:  2,
				Run: datastore.KeyForObj(ctx, r),
			}
			log := &run.RunLog{
				ID:  555,
				Run: datastore.KeyForObj(ctx, r),
			}
			So(datastore.Put(ctx, cl1, cl2, log), ShouldBeNil)
			So(wipeoutRuns(ctx, common.RunIDs{r.ID}), ShouldBeNil)
			So(datastore.Get(ctx, r), ShouldErrLike, datastore.ErrNoSuchEntity)
			So(datastore.Get(ctx, cl1), ShouldErrLike, datastore.ErrNoSuchEntity)
			So(datastore.Get(ctx, cl2), ShouldErrLike, datastore.ErrNoSuchEntity)
			So(datastore.Get(ctx, log), ShouldErrLike, datastore.ErrNoSuchEntity)
		})

		Convey("handle run doesn't exist", func() {
			createTime := ct.Clock.Now().Add(-2 * retentionPeriod).UTC()
			rid := common.MakeRunID(lProject, createTime, 1, []byte("deadbeef"))
			So(wipeoutRuns(ctx, common.RunIDs{rid}), ShouldBeNil)
		})

		Convey("handle run should still be retained", func() {
			r := makeRun(ct.Clock.Now().Add(-retentionPeriod / 2).UTC())
			So(wipeoutRuns(ctx, common.RunIDs{r.ID}), ShouldBeNil)
			So(datastore.Get(ctx, r), ShouldBeNil)
		})
	})
}
