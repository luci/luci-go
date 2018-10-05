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

			// Roll TQ forward.
			_, _, err = tqt.RunSimulation(ctx, &tqtesting.SimulationParams{
				ShouldStopBefore: func(t tqtesting.Task) bool {
					_, yep := t.Payload.(*tasks.FanOutShards)
					return yep
				},
			})
			So(err, ShouldBeNil)

			// Switched into "running" state.
			job, err = ctl.getJob(ctx, jobID)
			So(err, ShouldBeNil)
			So(job.State, ShouldEqual, JobStateRunning)

			expectedShard := func(id int64, idx, l, r int) shard {
				rng := splitter.Range{}
				if l != -1 {
					rng.Start = datastore.KeyForObj(ctx, &intEnt{ID: l})
				}
				if r != -1 {
					rng.End = datastore.KeyForObj(ctx, &intEnt{ID: r})
				}
				return shard{
					ID:      id,
					JobID:   jobID,
					Index:   idx,
					State:   shardStateStarting,
					Range:   rng,
					Created: testTime,
					Updated: testTime,
				}
			}

			// Created the shard entities.
			shards, err := job.fetchShards(ctx)
			So(err, ShouldBeNil)
			So(shards, ShouldResemble, []shard{
				expectedShard(1, 0, -1, 136),
				expectedShard(2, 1, 136, 268),
				expectedShard(3, 2, 268, 399),
				expectedShard(4, 3, 399, -1),
			})

			// Roll TQ forward.
			_, _, err = tqt.RunSimulation(ctx, &tqtesting.SimulationParams{
				ShouldStopAfter: func(t tqtesting.Task) bool {
					_, yep := t.Payload.(*tasks.FanOutShards)
					return yep
				},
			})
			So(err, ShouldBeNil)
		})
	})
}
