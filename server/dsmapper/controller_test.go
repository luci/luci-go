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

package dsmapper

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/server/dsmapper/dsmapperpb"
	"go.chromium.org/luci/server/dsmapper/internal/splitter"
	"go.chromium.org/luci/server/dsmapper/internal/tasks"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"
)

var (
	testTime        = testclock.TestRecentTimeUTC.Round(time.Millisecond)
	testTimeAsProto = timestamppb.New(testTime)
)

type intEnt struct {
	ID int64 `gae:"$id"`
}

func TestController(t *testing.T) {
	t.Parallel()

	ftt.Run("With controller", t, func(t *ftt.Test) {
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		ctx = gologger.StdConfig.Use(ctx)
		ctx, tc := testclock.UseTime(ctx, testTime)
		tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
			if testclock.HasTags(t, tqtesting.ClockTag) {
				tc.Add(d)
			}
		})

		dispatcher := &tq.Dispatcher{}
		ctx, sched := tq.TestingContext(ctx, dispatcher)

		ctl := Controller{
			MapperQueue:  "mapper-queue",
			ControlQueue: "control-queue",
		}
		ctl.Install(dispatcher)

		// mapperFunc is set by test cases.
		var mapperFunc func(params []byte, shardIdx int, keys []*datastore.Key) error

		const testMapperID ID = "test-mapper"
		ctl.RegisterFactory(testMapperID, func(_ context.Context, j *Job, idx int) (Mapper, error) {
			return func(_ context.Context, keys []*datastore.Key) error {
				if mapperFunc == nil {
					return nil
				}
				return mapperFunc(j.Config.Params, idx, keys)
			}, nil
		})

		spinUntilDone := func(expectErrors bool) (executed []proto.Message) {
			var succeeded tqtesting.TaskList
			sched.TaskSucceeded = tqtesting.TasksCollector(&succeeded)
			sched.TaskFailed = func(ctx context.Context, task *tqtesting.Task) {
				if !expectErrors {
					t.Fatalf("task %q %s failed unexpectedly", task.Name, task.Payload)
				}
			}
			sched.Run(ctx, tqtesting.StopWhenDrained())
			return succeeded.Payloads()
		}

		// Create a bunch of entities to run the mapper over.
		entities := make([]intEnt, 512)
		assert.Loosely(t, datastore.Put(ctx, entities), should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		t.Run("LaunchJob works", func(t *ftt.Test) {
			cfg := JobConfig{
				Query: Query{
					Kind: "intEnt",
				},
				Mapper:        testMapperID,
				Params:        []byte("zzz"),
				ShardCount:    4,
				PageSize:      33, // make it weird to trigger "incomplete" pages
				PagesPerTask:  2,  // to trigger multiple mapping tasks in a chain
				TrackProgress: true,
			}

			// Before we start, there' no job with ID 1.
			j, err := ctl.GetJob(ctx, 1)
			assert.Loosely(t, err, should.Equal(ErrNoSuchJob))
			assert.Loosely(t, j, should.BeNil)

			jobID, err := ctl.LaunchJob(ctx, &cfg)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, jobID, should.Equal(1))

			// In "starting" state.
			job, err := ctl.GetJob(ctx, jobID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, job, should.Resemble(&Job{
				ID:      jobID,
				Config:  cfg,
				State:   dsmapperpb.State_STARTING,
				Created: testTime,
				Updated: testTime,
			}))

			// No shards in the info yet.
			info, err := job.FetchInfo(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, info, should.Resemble(&dsmapperpb.JobInfo{
				Id:            int64(jobID),
				State:         dsmapperpb.State_STARTING,
				Created:       testTimeAsProto,
				Updated:       testTimeAsProto,
				TotalEntities: -1,
			}))

			// Roll TQ forward.
			sched.Run(ctx, tqtesting.StopBeforeTask("dsmapper-fan-out-shards"))

			// Switched into "running" state.
			job, err = ctl.GetJob(ctx, jobID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, job.State, should.Equal(dsmapperpb.State_RUNNING))

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
					State:         dsmapperpb.State_STARTING,
					Range:         rng,
					ExpectedCount: expected,
					Created:       testTime,
					Updated:       testTime,
				}
			}

			// Created the shard entities.
			shards, err := job.fetchShards(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, shards, should.Resemble([]shard{
				expectedShard(1, 0, -1, 136, 136),
				expectedShard(2, 1, 136, 268, 132),
				expectedShard(3, 2, 268, 399, 131),
				expectedShard(4, 3, 399, -1, 113),
			}))

			// Shards also appear in the info now.
			info, err = job.FetchInfo(ctx)
			assert.Loosely(t, err, should.BeNil)

			expectedShardInfo := func(idx, total int) *dsmapperpb.ShardInfo {
				return &dsmapperpb.ShardInfo{
					Index:         int32(idx),
					State:         dsmapperpb.State_STARTING,
					Created:       testTimeAsProto,
					Updated:       testTimeAsProto,
					TotalEntities: int64(total),
				}
			}
			assert.Loosely(t, info, should.Resemble(&dsmapperpb.JobInfo{
				Id:            int64(jobID),
				State:         dsmapperpb.State_RUNNING,
				Created:       testTimeAsProto,
				Updated:       testTimeAsProto,
				TotalEntities: 512,
				Shards: []*dsmapperpb.ShardInfo{
					expectedShardInfo(0, 136),
					expectedShardInfo(1, 132),
					expectedShardInfo(2, 131),
					expectedShardInfo(3, 113),
				},
			}))

			visitShards := func(cb func(s shard)) {
				visitedShards, err := job.fetchShards(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, visitedShards, should.HaveLength(cfg.ShardCount))
				for _, s := range visitedShards {
					cb(s)
				}
			}

			seen := make(map[int64]struct{}, len(entities))

			updateSeen := func(keys []*datastore.Key) {
				for _, k := range keys {
					_, ok := seen[k.IntID()]
					assert.Loosely(t, ok, should.BeFalse)
					seen[k.IntID()] = struct{}{}
				}
			}

			assertAllSeen := func() {
				assert.Loosely(t, len(seen), should.Equal(len(entities)))
				for _, e := range entities {
					_, ok := seen[e.ID]
					assert.Loosely(t, ok, should.BeTrue)
				}
			}

			t.Run("No errors when processing shards", func(t *ftt.Test) {
				mapperFunc = func(params []byte, shardIdx int, keys []*datastore.Key) error {
					assert.Loosely(t, len(keys), should.BeLessThanOrEqual(cfg.PageSize))
					assert.Loosely(t, params, should.Resemble(cfg.Params))
					updateSeen(keys)
					return nil
				}

				spinUntilDone(false)

				visitShards(func(s shard) {
					assert.Loosely(t, s.State, should.Equal(dsmapperpb.State_SUCCESS))
					assert.Loosely(t, s.ProcessTaskNum, should.Equal(2))
					assert.Loosely(t, s.ProcessedCount, should.Equal([]int64{
						136, 132, 131, 113,
					}[s.Index]))
				})

				assertAllSeen()

				job, err = ctl.GetJob(ctx, jobID)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, job.State, should.Equal(dsmapperpb.State_SUCCESS))

				info, err := job.FetchInfo(ctx)
				assert.Loosely(t, err, should.BeNil)

				expectedShardInfo := func(idx, total int) *dsmapperpb.ShardInfo {
					return &dsmapperpb.ShardInfo{
						Index:             int32(idx),
						State:             dsmapperpb.State_SUCCESS,
						Created:           testTimeAsProto,
						Updated:           testTimeAsProto,
						TotalEntities:     int64(total),
						ProcessedEntities: int64(total),
					}
				}
				assert.Loosely(t, info, should.Resemble(&dsmapperpb.JobInfo{
					Id:      int64(jobID),
					State:   dsmapperpb.State_SUCCESS,
					Created: testTimeAsProto,
					// There's 2 sec delay before UpdateJobState task.
					Updated:           timestamppb.New(testTime.Add(2 * time.Second)),
					TotalEntities:     512,
					ProcessedEntities: 512,
					EntitiesPerSec:    256,
					Shards: []*dsmapperpb.ShardInfo{
						expectedShardInfo(0, 136),
						expectedShardInfo(1, 132),
						expectedShardInfo(2, 131),
						expectedShardInfo(3, 113),
					},
				}))
			})

			t.Run("One shard fails", func(t *ftt.Test) {
				page := 0
				processed := 0

				mapperFunc = func(_ []byte, shardIdx int, keys []*datastore.Key) error {
					if shardIdx == 1 {
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
						assert.Loosely(t, s.State, should.Equal(dsmapperpb.State_FAIL))
						assert.Loosely(t, s.Error, should.Equal(`while mapping 33 keys: boom`))
					} else {
						assert.Loosely(t, s.State, should.Equal(dsmapperpb.State_SUCCESS))
						assert.Loosely(t, s.ProcessTaskNum, should.Equal(2))
					}
					assert.Loosely(t, s.ProcessedCount, should.Equal([]int64{
						136, 33, 131, 113, // the failed shard is incomplete
					}[s.Index]))
				})

				// There are 5 pages per shard. We aborted on second. So 3 are skipped.
				assert.Loosely(t, processed, should.Equal(len(entities)-3*cfg.PageSize))

				job, err = ctl.GetJob(ctx, jobID)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, job.State, should.Equal(dsmapperpb.State_FAIL))
			})

			t.Run("Job aborted midway", func(t *ftt.Test) {
				processed := 0

				mapperFunc = func(_ []byte, shardIdx int, keys []*datastore.Key) error {
					processed += len(keys)

					job, err = ctl.AbortJob(ctx, jobID)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, job.State, should.Equal(dsmapperpb.State_ABORTING))

					return nil
				}

				spinUntilDone(false)

				// All shards eventually discovered that the job was aborted.
				visitShards(func(s shard) {
					assert.Loosely(t, s.State, should.Equal(dsmapperpb.State_ABORTED))
				})

				// And the job itself eventually switched into ABORTED state.
				job, err = ctl.GetJob(ctx, jobID)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, job.State, should.Equal(dsmapperpb.State_ABORTED))

				// Processed 2 pages (instead of 1), since processShardHandler doesn't
				// check job state inside the processing loop (only at the beginning).
				assert.Loosely(t, processed, should.Equal(2*cfg.PageSize))
			})

			t.Run("processShardHandler saves state on transient errors", func(t *ftt.Test) {
				pages := 0

				mapperFunc = func(_ []byte, shardIdx int, keys []*datastore.Key) error {
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
				assert.Loosely(t, transient.Tag.In(err), should.BeTrue)

				// Shard's resume point is updated. Its taskNum is left unchanged, since
				// we are going to retry the task.
				sh, err := getActiveShard(ctx, shards[0].ID, shards[0].ProcessTaskNum)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, sh.ResumeFrom, should.NotBeNil)
				assert.Loosely(t, sh.ProcessedCount, should.Equal(33))
			})
		})

		t.Run("With simple starting job", func(t *ftt.Test) {
			cfg := JobConfig{
				Query:      Query{Kind: "intEnt"},
				Mapper:     testMapperID,
				ShardCount: 4,
				PageSize:   64,
			}

			jobID, err := ctl.LaunchJob(ctx, &cfg)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, jobID, should.Equal(1))

			// In "starting" state initially.
			job, err := ctl.GetJob(ctx, jobID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, job.State, should.Equal(dsmapperpb.State_STARTING))

			t.Run("Abort right after start", func(t *ftt.Test) {
				job, err := ctl.AbortJob(ctx, jobID)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, job.State, should.Equal(dsmapperpb.State_ABORTED)) // aborted right away

				// Didn't actually launch any shards.
				assert.Loosely(t, spinUntilDone(false), should.Resemble([]proto.Message{
					&tasks.SplitAndLaunch{JobId: int64(jobID)},
				}))
			})

			t.Run("Abort after shards are created", func(t *ftt.Test) {
				// Stop right after we created the shards, before we launch them.
				sched.Run(ctx, tqtesting.StopBeforeTask("dsmapper-fan-out-shards"))

				job, err := ctl.AbortJob(ctx, jobID)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, job.State, should.Equal(dsmapperpb.State_ABORTING)) // waits for shards to die

				spinUntilDone(false)

				job, err = ctl.AbortJob(ctx, jobID)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, job.State, should.Equal(dsmapperpb.State_ABORTED)) // all shards are dead now

				// Dead indeed.
				info, err := job.FetchInfo(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, info.Shards, should.HaveLength(4))
				for _, s := range info.Shards {
					assert.Loosely(t, s.State, should.Equal(dsmapperpb.State_ABORTED))
				}
			})
		})
	})
}
