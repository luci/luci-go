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

package tasks

import (
	"context"
	"sort"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/metrics"
	"go.chromium.org/luci/swarming/server/model"
)

func generateEntities(ctx context.Context, reqKey *datastore.Key, state apipb.TaskState, now time.Time) (*model.TaskRequest, *model.TaskResultSummary) {
	tr := &model.TaskRequest{
		Key: reqKey,
		TaskSlices: []model.TaskSlice{
			{
				Properties: model.TaskProperties{
					Dimensions: model.TaskDimensions{
						"d1": {"v1", "v2"},
					},
				},
			},
		},
		PubSubTopic: "pubsub-topic",
		RBEInstance: "rbe-instance",
	}

	trs := &model.TaskResultSummary{
		Key: model.TaskResultSummaryKey(ctx, reqKey),
		TaskResultCommon: model.TaskResultCommon{
			Modified: now.Add(-time.Minute),
			State:    state,
		},
		Created: now.Add(-2 * time.Hour),
		Tags:    []string{"spec_name:spec_name", "pool:test_pool", "device_type:wobblyeye"},
	}
	return tr, trs
}

func TestCancel(t *testing.T) {
	t.Parallel()

	ftt.Run("TestRun", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		now := time.Date(2024, time.January, 1, 2, 3, 4, 0, time.UTC)
		ctx, _ = testclock.UseTime(ctx, now)
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		globalStore := tsmon.Store(ctx)

		tID := "65aba3a3e6b99310"
		reqKey, err := model.TaskIDToRequestKey(ctx, tID)
		assert.Loosely(t, err, should.BeNil)
		tr, trs := generateEntities(ctx, reqKey, apipb.TaskState_PENDING, now)
		assert.Loosely(t, datastore.Put(ctx, tr), should.BeNil)

		t.Run("no task id specied", func(t *ftt.Test) {
			c := &Cancellation{}
			_, _, err := c.Run(ctx)
			assert.Loosely(t, err, should.ErrLike("no task id specified for cancellation"))
		})

		lt := MockTQTasks()
		c := &Cancellation{
			TaskID:         tID,
			KillRunning:    true,
			LifecycleTasks: lt,
		}

		t.Run("bot id and kill runing uncompitable", func(t *ftt.Test) {
			c.KillRunning = false
			c.BotID = "bot"
			_, _, err := c.Run(ctx)
			assert.Loosely(t, err, should.ErrLike("can only specify bot id in cancellation if can kill a running task"))
		})

		t.Run("failed to get some entity", func(t *ftt.Test) {
			_, _, err := c.Run(ctx)
			assert.Loosely(t, err, should.ErrLike("datastore error fetching entities for task 65aba3a3e6b99310"))
		})

		t.Run("mismatcked task id and request", func(t *ftt.Test) {
			wrongID := "65aba3a3e6b99500"
			wrongKey, _ := model.TaskIDToRequestKey(ctx, wrongID)
			c.TaskRequest = &model.TaskRequest{
				Key: wrongKey,
			}
			_, _, err := c.Run(ctx)
			assert.Loosely(t, err, should.ErrLike("mismatched TaskID 65aba3a3e6b99310 and TaskRequest with id 65aba3a3e6b99500"))
		})

		t.Run("cancel ended task", func(t *ftt.Test) {
			trs.State = apipb.TaskState_COMPLETED
			assert.Loosely(t, datastore.Put(ctx, trs), should.BeNil)
			canceled, wasRunning, err := c.Run(ctx)
			assert.Loosely(t, canceled, should.BeFalse)
			assert.Loosely(t, wasRunning, should.BeFalse)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("cancel pending task", func(t *ftt.Test) {
			trs.State = apipb.TaskState_PENDING
			toRunKey, err := model.TaskRequestToToRunKey(ctx, tr, 0)
			assert.Loosely(t, err, should.BeNil)
			ttr := &model.TaskToRun{
				Key:            toRunKey,
				QueueNumber:    datastore.NewIndexedOptional(int64(2)),
				Expiration:     datastore.NewIndexedOptional(now.Add(time.Hour)),
				RBEReservation: "reservation",
			}
			assert.Loosely(t, datastore.Put(ctx, trs, ttr), should.BeNil)
			canceled, wasRunning, err := c.Run(ctx)
			assert.Loosely(t, canceled, should.BeTrue)
			assert.Loosely(t, wasRunning, should.BeFalse)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, datastore.Get(ctx, trs, ttr), should.BeNil)
			assert.Loosely(t, trs.Abandoned.Get(), should.Match(now))
			assert.Loosely(t, trs.Modified, should.Match(now))
			assert.Loosely(t, trs.State, should.Equal(apipb.TaskState_CANCELED))
			assert.Loosely(t, ttr.IsReapable(), should.BeFalse)
			assert.Loosely(t, lt.PopTask("rbe-cancel"), should.Equal("rbe-instance/reservation"))
			assert.Loosely(t, lt.PopTask("pubsub-go"), should.Equal("65aba3a3e6b99310"))
			val := globalStore.Get(ctx, metrics.TaskStatusChangeSchedulerLatency, []any{"test_pool", "spec_name", "CANCELED", "wobblyeye"})
			assert.Loosely(t, val.(*distribution.Distribution).Sum(), should.Equal(float64(2*time.Hour.Milliseconds())))
		})

		t.Run("cancel running task", func(t *ftt.Test) {
			trs.State = apipb.TaskState_RUNNING
			assert.Loosely(t, datastore.Put(ctx, trs), should.BeNil)
			t.Run("don't kill running", func(t *ftt.Test) {
				c.KillRunning = false
				canceled, wasRunning, err := c.Run(ctx)
				assert.Loosely(t, canceled, should.BeFalse)
				assert.Loosely(t, wasRunning, should.BeTrue)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("wrong bot id", func(t *ftt.Test) {
				c.BotID = "bot"
				canceled, wasRunning, err := c.Run(ctx)
				assert.Loosely(t, canceled, should.BeFalse)
				assert.Loosely(t, wasRunning, should.BeTrue)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, datastore.Get(ctx, trs), should.BeNil)
				assert.Loosely(t, trs.Modified, should.Match(now.Add(-time.Minute)))
			})

			t.Run("kill running", func(t *ftt.Test) {
				trr := &model.TaskRunResult{
					TaskResultCommon: model.TaskResultCommon{
						State: apipb.TaskState_RUNNING,
					},
					Key: model.TaskRunResultKey(ctx, reqKey),
				}
				assert.Loosely(t, datastore.Put(ctx, trr), should.BeNil)
				canceled, wasRunning, err := c.Run(ctx)
				assert.Loosely(t, canceled, should.BeTrue)
				assert.Loosely(t, wasRunning, should.BeTrue)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, datastore.Get(ctx, trr, trs), should.BeNil)
				assert.Loosely(t, trs.Abandoned.Get(), should.Match(trr.Abandoned.Get()))
				assert.Loosely(t, trs.Abandoned.Get(), should.Match(now))
				assert.Loosely(t, trs.Modified, should.Match(trr.Modified))
				assert.Loosely(t, trs.Modified, should.Match(now))
				assert.Loosely(t, trr.Killing, should.BeTrue)
				assert.Loosely(t, trs.State, should.Equal(apipb.TaskState_RUNNING))
				assert.Loosely(t, lt.PopTask("cancel-children-tasks-go"), should.Equal(tID))
			})

			t.Run("kill running does nothing if the task has started cancellation", func(t *ftt.Test) {
				previousModified := now.Add(-time.Minute)
				trr := &model.TaskRunResult{
					TaskResultCommon: model.TaskResultCommon{
						State:    apipb.TaskState_RUNNING,
						Modified: previousModified,
					},
					Key:     model.TaskRunResultKey(ctx, reqKey),
					Killing: true,
				}
				trs.Modified = previousModified
				assert.Loosely(t, datastore.Put(ctx, trr, trs), should.BeNil)
				canceled, wasRunning, err := c.Run(ctx)
				assert.Loosely(t, canceled, should.BeFalse)
				assert.Loosely(t, wasRunning, should.BeTrue)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, datastore.Get(ctx, trr, trs), should.BeNil)
				assert.Loosely(t, trs.Modified, should.Match(trr.Modified))
				assert.Loosely(t, trs.Modified, should.Match(previousModified))
			})
		})
	})
}

func TestCancelChildren(t *testing.T) {
	t.Parallel()

	ftt.Run("TestCancelChildren", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		pID := "65aba3a3e6b99310"
		pReqKey, err := model.TaskIDToRequestKey(ctx, pID)
		assert.Loosely(t, err, should.BeNil)

		lt := MockTQTasks()
		cc := &childCancellation{
			parentID:       pID,
			batchSize:      300,
			lifecycleTasks: lt,
		}

		t.Run("task has no children", func(t *ftt.Test) {
			assert.Loosely(t, cc.queryToCancel(ctx), should.BeNil)
			assert.Loosely(t, lt.PopNTasks("cancel-tasks-go", 10), should.HaveLength(0))
		})

		t.Run("task has no active children to cancel", func(t *ftt.Test) {
			cID := "65aba3a3e6b99100"
			cReqKey, err := model.TaskIDToRequestKey(ctx, cID)
			assert.Loosely(t, err, should.BeNil)
			childReq := &model.TaskRequest{
				Key:          cReqKey,
				ParentTaskID: datastore.NewIndexedNullable(model.RequestKeyToTaskID(pReqKey, model.AsRunResult)),
			}
			childRes := &model.TaskResultSummary{
				Key: model.TaskResultSummaryKey(ctx, cReqKey),
				TaskResultCommon: model.TaskResultCommon{
					State: apipb.TaskState_COMPLETED,
				},
			}
			assert.Loosely(t, datastore.Put(ctx, childReq, childRes), should.BeNil)
			assert.Loosely(t, cc.queryToCancel(ctx), should.BeNil)
			assert.Loosely(t, lt.PopNTasks("cancel-tasks-go", 10), should.HaveLength(0))
		})

		t.Run("task has active children to cancel", func(t *ftt.Test) {
			cIDs := []string{"65aba3a3e6b99100", "65aba3a3e6b99200", "65aba3a3e6b99300", "65aba3a3e6b99400"}
			for i, cID := range cIDs {
				cReqKey, _ := model.TaskIDToRequestKey(ctx, cID)
				childReq := &model.TaskRequest{
					Key:          cReqKey,
					ParentTaskID: datastore.NewIndexedNullable(model.RequestKeyToTaskID(pReqKey, model.AsRunResult)),
				}
				childRes := &model.TaskResultSummary{
					Key: model.TaskResultSummaryKey(ctx, cReqKey),
					TaskResultCommon: model.TaskResultCommon{
						State: apipb.TaskState_RUNNING,
					},
				}
				if i == 1 {
					childRes.State = apipb.TaskState_COMPLETED
				}
				assert.Loosely(t, datastore.Put(ctx, childReq, childRes), should.BeNil)
			}

			t.Run("one task for all children", func(t *ftt.Test) {
				assert.Loosely(t, cc.queryToCancel(ctx), should.BeNil)
				assert.Loosely(t, lt.PopTask("cancel-tasks-go"), should.Equal(`["65aba3a3e6b99100" "65aba3a3e6b99300" "65aba3a3e6b99400"], purpose: cancel children for 65aba3a3e6b99310 batch 0, retry # 0`))
			})

			t.Run("multiple tasks", func(t *ftt.Test) {
				cc.batchSize = 2
				assert.Loosely(t, cc.queryToCancel(ctx), should.BeNil)
				res := lt.PopNTasks("cancel-tasks-go", 2)
				assert.Loosely(t, res, should.HaveLength(2))
				assert.Loosely(t, res[0], should.Equal(`["65aba3a3e6b99300" "65aba3a3e6b99400"], purpose: cancel children for 65aba3a3e6b99310 batch 0, retry # 0`))
				assert.Loosely(t, res[1], should.Equal(`["65aba3a3e6b99100"], purpose: cancel children for 65aba3a3e6b99310 batch 1, retry # 0`))
			})
		})
	})
}

func TestBatchCancellation(t *testing.T) {
	t.Parallel()

	ftt.Run("TestBatchCancellation", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		now := time.Date(2024, time.January, 1, 2, 3, 4, 0, time.UTC)
		ctx, _ = testclock.UseTime(ctx, now)
		ctx, _ = tsmon.WithDummyInMemory(ctx)

		lt := MockTQTasks()
		bc := &batchCancellation{
			killRunning:    true,
			lifecycleTasks: lt,
			workers:        10,
			purpose:        "CancelTasks request",
		}

		t.Run("no tasks to cancel", func(t *ftt.Test) {
			assert.Loosely(t, bc.run(ctx), should.ErrLike("no tasks specified for cancellation"))
			assert.Loosely(t, lt.PopNTasks("pubsub-go", 10), should.HaveLength(0))
		})

		tID1, tID2 := "65aba3a3e6b99100", "65aba3a3e6b99200"
		bc.tasks = []string{tID1, tID2}
		reqKey1, _ := model.TaskIDToRequestKey(ctx, tID1)
		tr1, trs1 := generateEntities(ctx, reqKey1, apipb.TaskState_PENDING, now)
		reqKey2, _ := model.TaskIDToRequestKey(ctx, tID2)
		tr2, trs2 := generateEntities(ctx, reqKey2, apipb.TaskState_RUNNING, now)
		assert.Loosely(t, datastore.Put(ctx, tr1, tr2, trs1, trs2), should.BeNil)

		t.Run("all fail for unknown tasks, no retry", func(t *ftt.Test) {
			assert.Loosely(t, bc.run(ctx), should.BeNil)
			// No new retry is enqueued.
			assert.Loosely(t, lt.PopNTasks("cancel-tasks-go", 10), should.HaveLength(0))
		})

		toRunKey, err := model.TaskRequestToToRunKey(ctx, tr1, 0)
		assert.Loosely(t, err, should.BeNil)
		ttr := &model.TaskToRun{
			Key:            toRunKey,
			QueueNumber:    datastore.NewIndexedOptional(int64(2)),
			Expiration:     datastore.NewIndexedOptional(now.Add(time.Hour)),
			RBEReservation: "reservation",
		}
		trr := &model.TaskRunResult{
			TaskResultCommon: model.TaskResultCommon{
				State: apipb.TaskState_RUNNING,
			},
			Key: model.TaskRunResultKey(ctx, reqKey2),
		}
		assert.Loosely(t, datastore.Put(ctx, ttr, trr), should.BeNil)

		t.Run("all failures are retriable", func(t *ftt.Test) {
			tr1.PubSubTopic = "fail-the-task"
			tr2.PubSubTopic = "fail-the-task"
			assert.Loosely(t, datastore.Put(ctx, tr1, tr2), should.BeNil)
			assert.Loosely(t, bc.run(ctx), should.BeNil)
			// A new task is enqueued to retry both tasks.
			assert.Loosely(t, lt.PopTask("cancel-tasks-go"), should.Equal(`["65aba3a3e6b99100" "65aba3a3e6b99200"], purpose: CancelTasks request, retry # 1`))
		})

		t.Run("partial fail", func(t *ftt.Test) {
			tr1.PubSubTopic = "fail-the-task"
			assert.Loosely(t, datastore.Put(ctx, tr1), should.BeNil)
			assert.Loosely(t, bc.run(ctx), should.BeNil)
			// A new task is enqueued to retry t1 only.
			assert.Loosely(t, lt.PopTask("cancel-tasks-go"), should.Equal(`["65aba3a3e6b99100"], purpose: CancelTasks request, retry # 1`))
			// t2 is fine.
			assert.Loosely(t, lt.PopTask("cancel-children-tasks-go"), should.Equal(tID2))
			assert.Loosely(t, datastore.Get(ctx, trr, trs2), should.BeNil)
			assert.Loosely(t, trs2.Modified, should.Match(trr.Modified))
			assert.Loosely(t, trs2.Modified, should.Match(now))
			assert.Loosely(t, trr.Killing, should.BeTrue)
		})

		t.Run("give up after sufficient retries", func(t *ftt.Test) {
			tr1.PubSubTopic = "fail-the-task"
			assert.Loosely(t, datastore.Put(ctx, tr1), should.BeNil)
			bc.tasks = []string{tID1}
			bc.retries = maxBatchCancellationRetries
			assert.Loosely(t, bc.run(ctx), should.BeNil)
			// No new retry is enqueued.
			assert.Loosely(t, lt.PopNTasks("cancel-tasks-go", 10), should.HaveLength(0))
		})

		t.Run("success", func(t *ftt.Test) {
			assert.Loosely(t, bc.run(ctx), should.BeNil)
			assert.Loosely(t, lt.PopTask("rbe-cancel"), should.Equal("rbe-instance/reservation"))
			res := lt.PopNTasks("pubsub-go", 2)
			sort.Strings(res)
			assert.Loosely(t, res, should.Match([]string{tID1, tID2}))
			assert.Loosely(t, lt.PopTask("cancel-children-tasks-go"), should.Equal(tID2))

			assert.Loosely(t, datastore.Get(ctx, ttr, trr, trs1, trs2), should.BeNil)
			assert.Loosely(t, trs1.Modified, should.Match(now))
			assert.Loosely(t, trs1.State, should.Equal(apipb.TaskState_CANCELED))
			assert.Loosely(t, ttr.IsReapable(), should.BeFalse)
			assert.Loosely(t, trs2.Modified, should.Match(trr.Modified))
			assert.Loosely(t, trs2.Modified, should.Match(now))
			assert.Loosely(t, trr.Killing, should.BeTrue)
		})

	})
}
