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
	"fmt"
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
			},
			Created: now.Add(-2 * time.Hour),
			Tags:    []string{"spec_name:spec_name", "pool:test_pool", "device_type:wobblyeye"},
		}
		assert.Loosely(t, datastore.Put(ctx, tr), should.BeNil)

		t.Run("no task id specied", func(t *ftt.Test) {
			c := &Cancellation{}
			_, err := c.Run(ctx)
			assert.Loosely(t, err, should.ErrLike("no task id specified for cancellation"))
		})

		c := &Cancellation{
			TaskID:      tID,
			KillRunning: true,
		}
		fakeTaskQueue := make(map[string][]string, 4)
		c.testEnqueueRBECancel = func(_ context.Context, tr *model.TaskRequest, ttr *model.TaskToRun) error {
			fakeTaskQueue["rbe-cancel"] = append(fakeTaskQueue["rbe-cancel"], fmt.Sprintf("%s/%s", tr.RBEInstance, ttr.RBEReservation))
			return nil
		}

		c.testSendOnTaskUpdate = func(_ context.Context, tr *model.TaskRequest, trs *model.TaskResultSummary) error {
			taskID := model.RequestKeyToTaskID(tr.Key, model.AsRequest)
			if tr.PubSubTopic != "" {
				fakeTaskQueue["pubsub-go"] = append(fakeTaskQueue["pubsub-go"], taskID)
			}
			if tr.HasBuildTask {
				fakeTaskQueue["buildbucket-notify-go"] = append(fakeTaskQueue["buildbucket-notify-go"], taskID)
			}

			return nil
		}

		c.testEnqueueChildCancellation = func(_ context.Context, taskID string) error {
			fakeTaskQueue["cancel-children-tasks-go"] = append(fakeTaskQueue["cancel-children-tasks-go"], taskID)
			return nil
		}

		t.Run("bot id and kill runing uncompitable", func(t *ftt.Test) {
			c.KillRunning = false
			c.BotID = "bot"
			_, err := c.Run(ctx)
			assert.Loosely(t, err, should.ErrLike("can only specify bot id in cancellation if can kill a running task"))
		})

		t.Run("failed to get some entity", func(t *ftt.Test) {
			_, err := c.Run(ctx)
			assert.Loosely(t, err, should.ErrLike("datastore error fetching entities for task 65aba3a3e6b99310"))
		})

		t.Run("cancel ended task", func(t *ftt.Test) {
			trs.State = apipb.TaskState_COMPLETED
			assert.Loosely(t, datastore.Put(ctx, trs), should.BeNil)
			wasRunning, err := c.Run(ctx)
			assert.Loosely(t, wasRunning, should.Equal(false))
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("cancel pending task", func(t *ftt.Test) {
			trs.State = apipb.TaskState_PENDING
			toRunKey, err := model.TaskRequestToToRunKey(ctx, tr, 0)
			assert.Loosely(t, err, should.BeNil)
			tr := &model.TaskToRun{
				Key:            toRunKey,
				QueueNumber:    datastore.NewIndexedOptional(int64(2)),
				Expiration:     datastore.NewIndexedOptional(now.Add(time.Hour)),
				RBEReservation: "reservation",
			}
			assert.Loosely(t, datastore.Put(ctx, trs, tr), should.BeNil)
			wasRunning, err := c.Run(ctx)
			assert.Loosely(t, wasRunning, should.Equal(false))
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, datastore.Get(ctx, trs, tr), should.BeNil)
			assert.Loosely(t, trs.Abandoned.Get(), should.Match(now))
			assert.Loosely(t, trs.Modified, should.Match(now))
			assert.Loosely(t, trs.State, should.Equal(apipb.TaskState_CANCELED))
			assert.Loosely(t, tr.IsReapable(), should.BeFalse)
			assert.Loosely(t, fakeTaskQueue["rbe-cancel"], should.Match([]string{
				"rbe-instance/reservation",
			}))
			assert.Loosely(t, fakeTaskQueue["pubsub-go"], should.Match([]string{
				"65aba3a3e6b99310",
			}))
			val := globalStore.Get(ctx, metrics.TaskStatusChangeSchedulerLatency, time.Time{}, []any{"test_pool", "spec_name", "CANCELED", "wobblyeye"})
			assert.Loosely(t, val.(*distribution.Distribution).Sum(), should.Equal(float64(2*time.Hour.Milliseconds())))
		})

		t.Run("cancel running task", func(t *ftt.Test) {
			trs.State = apipb.TaskState_RUNNING
			assert.Loosely(t, datastore.Put(ctx, trs), should.BeNil)
			t.Run("don't kill running", func(t *ftt.Test) {
				c.KillRunning = false
				wasRunning, err := c.Run(ctx)
				assert.Loosely(t, wasRunning, should.Equal(true))
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("wrong bot id", func(t *ftt.Test) {
				c.BotID = "bot"
				_, err := c.Run(ctx)
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
				wasRunning, err := c.Run(ctx)
				assert.Loosely(t, wasRunning, should.Equal(true))
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, datastore.Get(ctx, trr, trs), should.BeNil)
				assert.Loosely(t, trs.Abandoned.Get(), should.Match(trr.Abandoned.Get()))
				assert.Loosely(t, trs.Abandoned.Get(), should.Match(now))
				assert.Loosely(t, trs.Modified, should.Match(trr.Modified))
				assert.Loosely(t, trs.Modified, should.Match(now))
				assert.Loosely(t, trr.Killing, should.Equal(true))
				assert.Loosely(t, trs.State, should.Equal(apipb.TaskState_RUNNING))
				assert.Loosely(t, fakeTaskQueue["cancel-children-tasks-go"], should.Match([]string{tID}))
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
				wasRunning, err := c.Run(ctx)
				assert.Loosely(t, wasRunning, should.Equal(true))
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

		var fakeTaskQueue []string
		cc := &childCancellation{
			parentID:  pID,
			batchSize: 300,
			testEnqueueBatchCancel: func(_ context.Context, tasks []string) error {
				sort.Strings(tasks)
				fakeTaskQueue = append(fakeTaskQueue, fmt.Sprintf("cancel-tasks-go:%q", tasks))
				return nil
			},
		}

		t.Run("task has no children", func(t *ftt.Test) {
			assert.Loosely(t, cc.queryToCancel(ctx), should.BeNil)
			assert.Loosely(t, fakeTaskQueue, should.HaveLength(0))
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
			assert.Loosely(t, fakeTaskQueue, should.HaveLength(0))
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
				assert.Loosely(t, fakeTaskQueue, should.HaveLength(1))
				assert.Loosely(t, fakeTaskQueue[0], should.Equal(`cancel-tasks-go:["65aba3a3e6b99100" "65aba3a3e6b99300" "65aba3a3e6b99400"]`))
			})

			t.Run("multiple tasks", func(t *ftt.Test) {
				cc.batchSize = 2
				assert.Loosely(t, cc.queryToCancel(ctx), should.BeNil)
				assert.Loosely(t, fakeTaskQueue, should.HaveLength(2))
				assert.Loosely(t, fakeTaskQueue[0], should.Equal(`cancel-tasks-go:["65aba3a3e6b99300" "65aba3a3e6b99400"]`))
				assert.Loosely(t, fakeTaskQueue[1], should.Equal(`cancel-tasks-go:["65aba3a3e6b99100"]`))
			})
		})
	})
}
