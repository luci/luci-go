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
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/appengine/tq/tqtesting"

	"go.chromium.org/luci/appengine/mapper/internal/tasks"
	"go.chromium.org/luci/appengine/mapper/splitter"

	. "github.com/smartystreets/goconvey/convey"
)

var testTime = testclock.TestRecentTimeUTC.Round(time.Millisecond)

// shardIndex extracts the index of the shard processed by the mapper.
func shardIndex(c context.Context) int {
	return c.Value(&shardIndexKey).(int)
}

type testMapper struct {
	process func(context.Context, Params, []*datastore.Key) error
}

func (tm *testMapper) MapperID() ID { return "test-mapper" }
func (tm *testMapper) Process(c context.Context, p Params, k []*datastore.Key) error {
	if tm.process == nil {
		return nil
	}
	return tm.process(c, p, k)
}

type intEnt struct {
	ID int64 `gae:"$id"`
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

		Convey("LaunchJob works", func() {
			cfg := JobConfig{
				Query: Query{
					Kind: "intEnt",
				},
				Mapper: mapper.MapperID(),
				Params: Params{
					"k1": "v1",
				},
				ShardCount:    4,
				PageSize:      33, // make it weird to trigger "incomplete" pages
				PagesPerTask:  2,  // to trigger multiple mapping tasks in a chain
				TrackProgress: true,
			}

			jobID, err := ctl.LaunchJob(ctx, &cfg)
			So(err, ShouldBeNil)
			So(jobID, ShouldEqual, 1)

			// In "starting" state.
			job, err := getJob(ctx, jobID)
			So(err, ShouldBeNil)
			So(job, ShouldResemble, &Job{
				ID:      jobID,
				Config:  cfg,
				State:   State_STARTING,
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
			job, err = getJob(ctx, jobID)
			So(err, ShouldBeNil)
			So(job.State, ShouldEqual, State_RUNNING)

			expectedShard := func(id int64, idx int, l, r, expected int64) shard {
				rng := splitter.Range{}
				if l != -1 {
					rng.Start = datastore.KeyForObj(ctx, &intEnt{ID: l})
				}
				if r != -1 {
					rng.End = datastore.KeyForObj(ctx, &intEnt{ID: r})
				}
				return shard{
					ID:            id,
					JobID:         jobID,
					Index:         idx,
					State:         State_STARTING,
					Range:         rng,
					ExpectedCount: expected,
					Created:       testTime,
					Updated:       testTime,
				}
			}

			// Created the shard entities.
			shards, err := job.fetchShards(ctx)
			So(err, ShouldBeNil)
			So(shards, ShouldResemble, []shard{
				expectedShard(1, 0, -1, 136, 136),
				expectedShard(2, 1, 136, 268, 132),
				expectedShard(3, 2, 268, 399, 131),
				expectedShard(4, 3, 399, -1, 113),
			})

			spinUntilDone := func(expectErrors bool) {
				for {
					_, _, err := tqt.RunSimulation(ctx, nil)
					if err == nil {
						return
					}
					if !expectErrors {
						So(err, ShouldBeNil)
					}
				}
			}

			visitShards := func(cb func(s shard)) {
				shards, err := job.fetchShards(ctx)
				So(err, ShouldBeNil)
				So(shards, ShouldHaveLength, cfg.ShardCount)
				for _, s := range shards {
					cb(s)
				}
			}

			seen := make(map[int64]struct{}, len(entities))

			updateSeen := func(keys []*datastore.Key) {
				for _, k := range keys {
					_, ok := seen[k.IntID()]
					So(ok, ShouldBeFalse)
					seen[k.IntID()] = struct{}{}
				}
			}

			assertAllSeen := func() {
				So(len(seen), ShouldEqual, len(entities))
				for _, e := range entities {
					_, ok := seen[e.ID]
					So(ok, ShouldBeTrue)
				}
			}

			Convey("No errors when processing shards", func() {
				mapper.process = func(c context.Context, p Params, keys []*datastore.Key) error {
					So(len(keys), ShouldBeLessThanOrEqualTo, cfg.PageSize)
					updateSeen(keys)
					return nil
				}

				spinUntilDone(false)

				visitShards(func(s shard) {
					So(s.State, ShouldEqual, State_SUCCESS)
					So(s.ProcessTaskNum, ShouldEqual, 2)
					So(s.ProcessedCount, ShouldEqual, []int64{
						136, 132, 131, 113,
					}[s.Index])
				})

				assertAllSeen()

				job, err := getJob(ctx, jobID)
				So(err, ShouldBeNil)
				So(job.State, ShouldEqual, State_SUCCESS)
			})

			Convey("One shard fails", func() {
				page := 0
				processed := 0

				mapper.process = func(c context.Context, p Params, keys []*datastore.Key) error {
					if shardIndex(c) == 1 {
						page++
						if page == 2 {
							return errors.New("boom")
						}
					}
					processed += len(keys)
					return nil
				}

				spinUntilDone(true)

				visitShards(func(s shard) {
					if s.Index == 1 {
						So(s.State, ShouldEqual, State_FAIL)
						So(s.Error, ShouldEqual, `in the mapper "test-mapper": boom`)
					} else {
						So(s.State, ShouldEqual, State_SUCCESS)
						So(s.ProcessTaskNum, ShouldEqual, 2)
					}
					So(s.ProcessedCount, ShouldEqual, []int64{
						136, 33, 131, 113, // the failed shard is incomplete
					}[s.Index])
				})

				// There are 5 pages per shard. We aborted on second. So 3 are skipped.
				So(processed, ShouldEqual, len(entities)-3*cfg.PageSize)

				job, err := getJob(ctx, jobID)
				So(err, ShouldBeNil)
				So(job.State, ShouldEqual, State_FAIL)
			})

			Convey("Job aborted midway", func() {
				processed := 0

				mapper.process = func(c context.Context, p Params, keys []*datastore.Key) error {
					processed += len(keys)

					job, err := getJob(ctx, jobID)
					So(err, ShouldBeNil)
					job.State = State_FAIL
					So(datastore.Put(c, job), ShouldBeNil)

					return nil
				}

				spinUntilDone(false)

				// All shards are left in whatever state they were in. We don't bother
				// changing their state: Shards owned by an aborted job are
				// automatically considered aborted.
				visitShards(func(s shard) {
					if s.Index == 0 {
						// Zeroth shard did manage to run for a bit.
						So(s.State, ShouldEqual, State_RUNNING)
						So(s.ProcessedCount, ShouldEqual, 66)
					} else {
						So(s.State, ShouldEqual, State_STARTING)
						So(s.ProcessedCount, ShouldEqual, 0)
					}
				})

				// Processed 2 pages (instead of 1), since processShardHandler doesn't
				// check job state inside the processing loop (only at the beginning).
				So(processed, ShouldEqual, 2*cfg.PageSize)
			})

			Convey("processShardHandler saves state on transient errors", func() {
				pages := 0

				mapper.process = func(c context.Context, p Params, keys []*datastore.Key) error {
					pages++
					if pages == 2 {
						return errors.New("boom", transient.Tag)
					}
					return nil
				}

				err := ctl.processShardHandler(ctx, &tasks.ProcessShard{
					JobId:   int64(job.ID),
					ShardId: shards[0].ID,
				})
				So(transient.Tag.In(err), ShouldBeTrue)

				// Shard's resume point is updated. Its taskNum is left unchanged, since
				// we are going to retry the task.
				sh, err := getActiveShard(ctx, shards[0].ID, shards[0].ProcessTaskNum)
				So(err, ShouldBeNil)
				So(sh.ResumeFrom, ShouldNotBeNil)
				So(sh.ProcessedCount, ShouldEqual, 33)
			})
		})
	})
}
