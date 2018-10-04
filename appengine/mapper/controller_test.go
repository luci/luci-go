// Copyright 2018 The LUCI Authors.
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

package mapper

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/clock/testclock"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/appengine/tq/tqtesting"

	"go.chromium.org/luci/appengine/mapper/internal/tasks"
	"go.chromium.org/luci/appengine/mapper/splitter"

	. "github.com/smartystreets/goconvey/convey"
)

var testTime = testclock.TestRecentTimeUTC.Round(time.Millisecond)

type testMapper struct{}

func (tm *testMapper) MapperID() ID                                            { return "test-mapper" }
func (tm *testMapper) Process(context.Context, Params, []*datastore.Key) error { return nil }

type intEnt struct {
	ID int `gae:"$id"`
}

func TestController(t *testing.T) {
	t.Parallel()

	Convey("With controller", t, func() {
		ctx := gaetesting.TestingContext()
		ctx, _ = testclock.UseTime(ctx, testTime)

		dispatcher := &tq.Dispatcher{}
		mapper := &testMapper{}

		ctl := Controller{
			MapperQueue:  "mapper-queue",
			ControlQueue: "control-queue",
		}
		ctl.RegisterMapper(mapper)
		ctl.Install(dispatcher)

		tqt := tqtesting.GetTestable(ctx, dispatcher)
		tqt.CreateQueues()

		// Create a bunch of entities to run the mapper over.
		entities := make([]intEnt, 512)
		So(datastore.Put(ctx, entities), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		Convey("LaunchJob splits key range", func() {
			cfg := JobConfig{
				Query: Query{
					Kind: "intEnt",
				},
				Mapper:     mapper.MapperID(),
				ShardCount: 4,
				PageSize:   64,
				Params: Params{
					"k1": "v1",
				},
			}

			jobID, err := ctl.LaunchJob(ctx, &cfg)
			So(err, ShouldBeNil)
			So(jobID, ShouldEqual, 1)

			// In "starting" state.
			job, err := ctl.getJob(ctx, jobID)
			So(err, ShouldBeNil)
			So(job, ShouldResemble, &Job{
				ID:      jobID,
				Config:  cfg,
				State:   JobStateStarting,
				Created: testTime,
				Updated: testTime,
			})

			ranges := make([][]int, 0, cfg.ShardCount)
			ctl.testingRecordSplit = func(rng splitter.Range) {
				l, r := -1, -1
				if rng.Start != nil {
					l = int(rng.Start.IntID())
				}
				if rng.End != nil {
					r = int(rng.End.IntID())
				}
				ranges = append(ranges, []int{l, r})
			}

			// Roll TQ forward.
			_, _, err = tqt.RunSimulation(ctx, &tqtesting.SimulationParams{
				ShouldStopAfter: func(t tqtesting.Task) bool {
					_, yep := t.Payload.(*tasks.SplitAndLaunch)
					return yep
				},
			})
			So(err, ShouldBeNil)

			// Correctly split into 4 shards.
			So(ranges, ShouldResemble, [][]int{
				{-1, 136}, {136, 268}, {268, 399}, {399, -1},
			})

			// TODO: add more.
		})
	})
}
