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

package aggrmetrics

import (
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/caching"

	commonpb "go.chromium.org/luci/cv/api/common/v1"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRunAggregator(t *testing.T) {
	t.Parallel()

	Convey("runAggregator works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		// Truncate current time to seconds to deal with integer delays.
		ct.Clock.Set(ct.Clock.Now().UTC().Add(time.Second).Truncate(time.Second))

		ctx = caching.WithEmptyProcessCache(ctx)

		// TODO(tandrii): move this to cvtesting.Test.SetUp().
		ctx, _, _ = tsmon.WithFakes(ctx)
		tstore := store.NewInMemory(&target.Task{})
		tsmon.GetState(ctx).SetStore(tstore)

		runsSent := func(project, status string) interface{} {
			return tstore.Get(ctx, metricActiveRunsCount, time.Time{}, []interface{}{project, status})
		}
		durationsSent := func(project string) *distribution.Distribution {
			d := tstore.Get(ctx, metricActiveRunsDurationsS, time.Time{}, []interface{}{project})
			if d == nil {
				return nil
			}
			return d.(*distribution.Distribution)
		}

		putRun := func(i byte, p string, s commonpb.Run_Status, ct time.Time) {
			err := datastore.Put(ctx, &run.Run{
				ID:         common.MakeRunID(p, ct, 1, []byte{i}),
				CreateTime: ct,
				Status:     s,
			})
			if err != nil {
				panic(err)
			}
		}

		prepareAndReport := func(active ...string) {
			ra := runsAggregator{}
			f, err := ra.prepare(ctx, stringset.NewFromSlice(active...))
			So(err, ShouldBeNil)
			f(ctx)
		}

		Convey("Active projects get data reported even when there are no active Runs", func() {
			prepareAndReport("v8")
			So(runsSent("v8", "RUNNING"), ShouldEqual, 0)
			So(durationsSent("v8").Count(), ShouldEqual, 0)
			So(tstore.GetAll(ctx), ShouldHaveLength, 2)
		})

		Convey("Active projects with various run kinds", func() {
			putRun(1, "v8", commonpb.Run_RUNNING, ct.Clock.Now().Add(-time.Second))
			putRun(2, "v8", commonpb.Run_RUNNING, ct.Clock.Now().Add(-time.Minute))
			putRun(3, "v8", commonpb.Run_SUBMITTING, ct.Clock.Now().Add(-time.Hour))
			putRun(4, "fuchsia", commonpb.Run_WAITING_FOR_SUBMISSION, ct.Clock.Now().Add(-time.Second))
			putRun(5, "fuchsia", commonpb.Run_WAITING_FOR_SUBMISSION, ct.Clock.Now().Add(-time.Second))
			prepareAndReport("v8", "fuchsia")

			So(runsSent("v8", "RUNNING"), ShouldEqual, 2)
			So(runsSent("v8", "SUBMITTING"), ShouldEqual, 1)
			So(durationsSent("v8").Sum(), ShouldEqual, (time.Second + time.Minute + time.Hour).Seconds())
			So(durationsSent("v8").Count(), ShouldEqual, 3)

			So(runsSent("fuchsia", "RUNNING"), ShouldEqual, 0)
			So(runsSent("fuchsia", "WAITING_FOR_SUBMISSION"), ShouldEqual, 2)
			So(durationsSent("fuchsia").Sum(), ShouldEqual, 2)
			So(durationsSent("fuchsia").Count(), ShouldEqual, 2)

			So(tstore.GetAll(ctx), ShouldHaveLength, 6)
		})

		putManyRuns := func(n int) map[string]int {
			projects := map[string]int{}
			for i := 0; i < n; i++ {
				id := byte(i % 128)
				project := fmt.Sprintf("p-%04d", i/128)
				created := ct.Clock.Now().Add(-time.Duration(id) * time.Second)
				projects[project]++
				putRun(id, project, commonpb.Run_RUNNING, created)
			}
			return projects
		}

		Convey("Can handle a lot of Runs and projects", func() {
			projects := putManyRuns(maxRuns)
			prepareAndReport()

			// First project should have all its Runs reported.
			So(runsSent("p-0000", "RUNNING"), ShouldEqual, 128)
			So(durationsSent("p-0000").Sum(), ShouldEqual, 127*128/2) // 0 + 1 + 2 + ... + 127
			So(durationsSent("p-0000").Count(), ShouldEqual, 128)

			// But projects at the end of the lexicographic order may have nothing
			// reported due to maxRuns limit.
			reportedCnt := 0
			for p := range projects {
				v := runsSent(p, "RUNNING")
				if v != nil {
					reportedCnt += int(v.(int64))
				}
			}
			So(reportedCnt, ShouldEqual, maxRuns)
		})

		Convey("Refuses to report anything if there are too many active Runs", func() {
			putManyRuns(maxRuns + 1)
			ra := runsAggregator{}
			_, err := ra.prepare(ctx, stringset.Set{})
			So(err, ShouldErrLike, "too many active Runs")
			So(tstore.GetAll(ctx), ShouldBeEmpty)
		})
	})
}
